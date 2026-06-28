package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/conversation-service/internal/domain"
	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type Repository struct {
	pool     *pgxpool.Pool
	now      func() time.Time
	changeID func() (types.ChangeID, error)
	eventID  func() (types.EventID, error)
}

type RepositoryOption func(*Repository)

func NewRepository(pool *pgxpool.Pool, opts ...RepositoryOption) *Repository {
	repo := &Repository{
		pool: pool,
		now:  func() time.Time { return time.Now().UTC() },
		changeID: func() (types.ChangeID, error) {
			id, err := newUUIDString()
			if err != nil {
				return "", err
			}
			return types.ChangeID("change_" + id), nil
		},
		eventID: func() (types.EventID, error) {
			id, err := newUUIDString()
			if err != nil {
				return "", err
			}
			return types.EventID(id), nil
		},
	}
	for _, opt := range opts {
		opt(repo)
	}
	return repo
}

func WithClock(clock func() time.Time) RepositoryOption {
	return func(repo *Repository) {
		if clock != nil {
			repo.now = clock
		}
	}
}

func WithIDGenerators(
	changeID func() (types.ChangeID, error),
	eventID func() (types.EventID, error),
) RepositoryOption {
	return func(repo *Repository) {
		if changeID != nil {
			repo.changeID = changeID
		}
		if eventID != nil {
			repo.eventID = eventID
		}
	}
}

func (r *Repository) GetSendContext(
	ctx context.Context,
	command types.GetSendContextCommand,
) (types.ConversationSendContext, error) {
	row := r.pool.QueryRow(ctx, `
SELECT
    c.status,
    c.conversation_type,
    c.conversation_mode,
    c.fanout_mode,
    c.fanout_policy_version,
    c.member_version,
    c.permission_version,
    c.current_seq_shard,
    COALESCE(peer.direct_peer_user_id, ''),
    COALESCE(m.status, ''),
    COALESCE(m.member_version, 0),
    COALESCE(m.permission_version, 0)
FROM conversations c
LEFT JOIN conversation_members m
  ON m.tenant_id = c.tenant_id
 AND m.conversation_id = c.conversation_id
 AND m.user_id = $3
LEFT JOIN LATERAL (
    SELECT CASE WHEN COUNT(*) = 1 THEN MAX(peer.user_id) ELSE '' END AS direct_peer_user_id
    FROM conversation_members peer
    WHERE peer.tenant_id = c.tenant_id
      AND peer.conversation_id = c.conversation_id
      AND peer.user_id <> $3
      AND peer.status = 'ACTIVE'
) peer ON c.conversation_type = 'DIRECT'
WHERE c.tenant_id = $1
  AND c.conversation_id = $2
`, command.TenantID, command.ConversationID, command.UserID)

	var conversation domain.Conversation
	var member domain.Member
	conversation.TenantID = command.TenantID
	conversation.ConversationID = command.ConversationID
	member.UserID = command.UserID
	if err := row.Scan(
		&conversation.Status,
		&conversation.ConversationType,
		&conversation.ConversationMode,
		&conversation.FanoutMode,
		&conversation.FanoutPolicyVersion,
		&conversation.MemberVersion,
		&conversation.PermissionVersion,
		&conversation.CurrentSeqShard,
		&conversation.DirectPeerUserID,
		&member.Status,
		&member.MemberVersion,
		&member.PermissionVersion,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.ConversationSendContext{}, types.NewConversationNotFound("conversation not found")
		}
		return types.ConversationSendContext{}, types.NewDBReadFailed(err.Error())
	}
	return domain.BuildSendContext(conversation, member)
}

func (r *Repository) CreateMemberChange(
	ctx context.Context,
	command types.CreateMemberChangeCommand,
) (types.MemberChangeResult, error) {
	if r.pool == nil {
		return types.MemberChangeResult{}, types.NewDBWriteFailed("repository is not configured")
	}
	commandHash, err := domain.ComputeMemberChangeCommandHash(command)
	if err != nil {
		return types.MemberChangeResult{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.MemberChangeResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := lockMemberChangeIdempotency(ctx, tx, command); err != nil {
		return types.MemberChangeResult{}, err
	}
	if existing, ok, err := findExistingMemberChange(ctx, tx, command); err != nil {
		return types.MemberChangeResult{}, err
	} else if ok {
		return replayMemberChange(ctx, tx, existing, commandHash)
	}

	conversation, err := lockConversation(ctx, tx, command)
	if err != nil {
		return types.MemberChangeResult{}, err
	}
	target, err := lockConversationMember(ctx, tx, command.AuthContext.TenantID, command.ConversationID, command.TargetUserID)
	if err != nil {
		return types.MemberChangeResult{}, err
	}
	operator := target
	if command.AuthContext.UserID != command.TargetUserID {
		operator, err = lockConversationMember(ctx, tx, command.AuthContext.TenantID, command.ConversationID, command.AuthContext.UserID)
		if err != nil {
			return types.MemberChangeResult{}, err
		}
	}

	if err := ensureConversationSeq(ctx, tx, command); err != nil {
		return types.MemberChangeResult{}, err
	}
	boundarySeq, err := allocateConversationSeq(ctx, tx, command)
	if err != nil {
		return types.MemberChangeResult{}, err
	}
	changeID, err := r.changeID()
	if err != nil {
		return types.MemberChangeResult{}, err
	}
	eventID, err := r.eventID()
	if err != nil {
		return types.MemberChangeResult{}, err
	}
	record, err := domain.NewMemberChangeRecord(domain.MemberChangeInput{
		Command:      command,
		Conversation: conversation,
		Target:       target,
		Operator:     operator,
	}, changeID, eventID, boundarySeq, r.now())
	if err != nil {
		return types.MemberChangeResult{}, err
	}

	if err := insertMemberChangeSaga(ctx, tx, record.Change); err != nil {
		return types.MemberChangeResult{}, err
	}
	if err := upsertConversationMember(ctx, tx, command, record); err != nil {
		return types.MemberChangeResult{}, err
	}
	scalePolicy, err := applyConversationScalePolicyAfterMemberChange(ctx, tx, conversation)
	if err != nil {
		return types.MemberChangeResult{}, err
	}
	record.Timeline.FanoutMode = scalePolicy.FanoutMode
	record.Timeline.FanoutPolicyVersion = scalePolicy.FanoutPolicyVersion
	if err := updateConversationVersions(ctx, tx, command, record); err != nil {
		return types.MemberChangeResult{}, err
	}
	if err := insertMemberTimelineEvent(ctx, tx, record.Timeline); err != nil {
		return types.MemberChangeResult{}, err
	}
	if err := insertMemberOutboxEvent(ctx, tx, record.Outbox); err != nil {
		return types.MemberChangeResult{}, err
	}
	if err := markMemberChangeOutboxEnqueued(ctx, tx, record.Change); err != nil {
		return types.MemberChangeResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.MemberChangeResult{}, types.NewDBWriteFailed(err.Error())
	}
	return memberChangeResult(record, false), nil
}

func (r *Repository) GetMemberChange(
	ctx context.Context,
	command types.GetMemberChangeCommand,
) (types.MemberChangeDetail, error) {
	if r.pool == nil {
		return types.MemberChangeDetail{}, types.NewDBReadFailed("repository is not configured")
	}
	row := r.pool.QueryRow(ctx, `
SELECT
    mcs.change_id,
    mcs.tenant_id,
    mcs.conversation_id,
    mcs.user_id,
    mcs.operator_id,
    mcs.change_type,
    mcs.status,
    COALESCE(mcs.boundary_seq, 0),
    COALESCE((metadata_json->>'member_version')::bigint, 0),
    COALESCE((metadata_json->>'permission_version')::bigint, 0),
    COALESCE(metadata_json->>'old_role', ''),
    COALESCE(metadata_json->>'new_role', ''),
    COALESCE(cte.payload_json->>'reason', ''),
    CASE
        WHEN COALESCE(last_error, '') = '' THEN ''
        ELSE 'member change processing failed'
    END,
    COALESCE(auth_member.role, ''),
    COALESCE(auth_member.status, '')
FROM member_change_saga mcs
LEFT JOIN conversation_timeline_events cte
  ON cte.tenant_id = mcs.tenant_id
 AND cte.event_id = mcs.timeline_event_id
LEFT JOIN conversation_members auth_member
  ON auth_member.tenant_id = mcs.tenant_id
 AND auth_member.conversation_id = mcs.conversation_id
 AND auth_member.user_id = $4
WHERE mcs.tenant_id = $1
  AND mcs.conversation_id = $2
  AND mcs.change_id = $3
`, command.AuthContext.TenantID, command.ConversationID, command.ChangeID, command.AuthContext.UserID)
	var result types.MemberChangeDetail
	var authRole types.MemberRole
	var authStatus types.MemberStatus
	if err := row.Scan(
		&result.ChangeID,
		&result.TenantID,
		&result.ConversationID,
		&result.TargetUserID,
		&result.OperatorUserID,
		&result.ChangeType,
		&result.Status,
		&result.BoundarySeq,
		&result.MemberVersion,
		&result.PermissionVersion,
		&result.OldRole,
		&result.NewRole,
		&result.Reason,
		&result.LastError,
		&authRole,
		&authStatus,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.MemberChangeDetail{}, types.NewMemberChangeNotFound("member change not found")
		}
		return types.MemberChangeDetail{}, types.NewDBReadFailed(err.Error())
	}
	if !canViewMemberChange(command.AuthContext.UserID, authRole, authStatus, result) {
		return types.MemberChangeDetail{}, types.NewPermissionDenied("member change is not visible to caller")
	}
	return result, nil
}

func (r *Repository) TransferConversationOwner(
	ctx context.Context,
	command types.TransferConversationOwnerCommand,
) (types.TransferConversationOwnerResult, error) {
	if r.pool == nil {
		return types.TransferConversationOwnerResult{}, types.NewDBWriteFailed("repository is not configured")
	}
	memberCommand := ownerTransferMemberChangeCommand(command)
	commandHash, err := domain.ComputeOwnerTransferCommandHash(command)
	if err != nil {
		return types.TransferConversationOwnerResult{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.TransferConversationOwnerResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := lockMemberChangeIdempotency(ctx, tx, memberCommand); err != nil {
		return types.TransferConversationOwnerResult{}, err
	}
	if existing, ok, err := findExistingMemberChange(ctx, tx, memberCommand); err != nil {
		return types.TransferConversationOwnerResult{}, err
	} else if ok {
		return replayOwnerTransfer(ctx, tx, existing, commandHash)
	}

	conversation, err := lockConversation(ctx, tx, memberCommand)
	if err != nil {
		return types.TransferConversationOwnerResult{}, err
	}
	previousOwner, err := lockConversationMember(ctx, tx, command.AuthContext.TenantID, command.ConversationID, command.AuthContext.UserID)
	if err != nil {
		return types.TransferConversationOwnerResult{}, err
	}
	newOwner, err := lockConversationMember(ctx, tx, command.AuthContext.TenantID, command.ConversationID, command.NewOwnerUserID)
	if err != nil {
		return types.TransferConversationOwnerResult{}, err
	}

	if err := ensureConversationSeq(ctx, tx, memberCommand); err != nil {
		return types.TransferConversationOwnerResult{}, err
	}
	boundarySeq, err := allocateConversationSeq(ctx, tx, memberCommand)
	if err != nil {
		return types.TransferConversationOwnerResult{}, err
	}
	changeID, err := r.changeID()
	if err != nil {
		return types.TransferConversationOwnerResult{}, err
	}
	eventID, err := r.eventID()
	if err != nil {
		return types.TransferConversationOwnerResult{}, err
	}
	record, err := domain.NewOwnerTransferRecord(domain.OwnerTransferInput{
		Command:       command,
		Conversation:  conversation,
		PreviousOwner: previousOwner,
		NewOwner:      newOwner,
	}, changeID, eventID, boundarySeq, r.now())
	if err != nil {
		return types.TransferConversationOwnerResult{}, err
	}

	if err := insertMemberChangeSaga(ctx, tx, record.Change); err != nil {
		return types.TransferConversationOwnerResult{}, err
	}
	if err := upsertMemberMutation(ctx, tx, record.Change.TenantID, record.Change.ConversationID, record.PreviousOwner); err != nil {
		return types.TransferConversationOwnerResult{}, err
	}
	if err := upsertMemberMutation(ctx, tx, record.Change.TenantID, record.Change.ConversationID, record.NewOwner); err != nil {
		return types.TransferConversationOwnerResult{}, err
	}
	if err := updateConversationVersionValues(ctx, tx, record.Change.TenantID, record.Change.ConversationID, record.Change.MemberVersion, record.Change.PermissionVersion); err != nil {
		return types.TransferConversationOwnerResult{}, err
	}
	if err := insertMemberTimelineEvent(ctx, tx, record.Timeline); err != nil {
		return types.TransferConversationOwnerResult{}, err
	}
	if err := insertMemberOutboxEvent(ctx, tx, record.Outbox); err != nil {
		return types.TransferConversationOwnerResult{}, err
	}
	if err := markMemberChangeOutboxEnqueued(ctx, tx, record.Change); err != nil {
		return types.TransferConversationOwnerResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.TransferConversationOwnerResult{}, types.NewDBWriteFailed(err.Error())
	}
	return ownerTransferResult(record, false), nil
}

func canViewMemberChange(
	userID types.UserID,
	authRole types.MemberRole,
	authStatus types.MemberStatus,
	change types.MemberChangeDetail,
) bool {
	if userID == change.OperatorUserID || userID == change.TargetUserID {
		return true
	}
	if authStatus != types.MemberStatusActive {
		return false
	}
	return authRole == types.MemberRoleOwner || authRole == types.MemberRoleAdmin
}
