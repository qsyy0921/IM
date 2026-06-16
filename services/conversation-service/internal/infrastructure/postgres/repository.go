package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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

type listMembersPageToken struct {
	Version    int    `json:"v"`
	UserID     string `json:"user_id"`
	RoleFilter string `json:"role_filter"`
}

func (r *Repository) ListConversationMembers(
	ctx context.Context,
	command types.ListConversationMembersCommand,
) (types.ListConversationMembersResult, error) {
	if r.pool == nil {
		return types.ListConversationMembersResult{}, types.NewDBReadFailed("repository is not configured")
	}
	lastUserID, err := decodeListMembersPageToken(command.PageToken, command.RoleFilter)
	if err != nil {
		return types.ListConversationMembersResult{}, err
	}

	var conversationStatus types.ConversationStatus
	var authStatus types.MemberStatus
	result := types.ListConversationMembersResult{
		TenantID:       command.AuthContext.TenantID,
		ConversationID: command.ConversationID,
	}
	if err := r.pool.QueryRow(ctx, `
SELECT
    c.status,
    c.member_version,
    c.permission_version,
    COALESCE(auth_member.status, '')
FROM conversations c
LEFT JOIN conversation_members auth_member
  ON auth_member.tenant_id = c.tenant_id
 AND auth_member.conversation_id = c.conversation_id
 AND auth_member.user_id = $3
WHERE c.tenant_id = $1
  AND c.conversation_id = $2
`, command.AuthContext.TenantID, command.ConversationID, command.AuthContext.UserID).Scan(
		&conversationStatus,
		&result.MemberVersion,
		&result.PermissionVersion,
		&authStatus,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.ListConversationMembersResult{}, types.NewConversationNotFound("conversation not found")
		}
		return types.ListConversationMembersResult{}, types.NewDBReadFailed(err.Error())
	}
	if conversationStatus != types.ConversationStatusActive {
		return types.ListConversationMembersResult{}, types.NewConversationNotFound("conversation not found")
	}
	if authStatus != types.MemberStatusActive {
		return types.ListConversationMembersResult{}, types.NewMemberNotActive("conversation member is not active")
	}

	pageSize := command.EffectivePageSize()
	rows, err := r.pool.Query(ctx, `
SELECT
    user_id,
    role,
    status,
    COALESCE(join_seq, 0),
    COALESCE(leave_seq, 0),
    member_version,
    permission_version,
    updated_at
FROM conversation_members
WHERE tenant_id = $1
  AND conversation_id = $2
  AND status = 'ACTIVE'
  AND ($3 = '' OR role = $3)
  AND ($4 = '' OR user_id > $4)
ORDER BY user_id ASC
LIMIT $5
`, command.AuthContext.TenantID, command.ConversationID, command.RoleFilter, lastUserID, pageSize+1)
	if err != nil {
		return types.ListConversationMembersResult{}, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	members := make([]types.ConversationMember, 0, pageSize)
	for rows.Next() {
		var member types.ConversationMember
		if err := rows.Scan(
			&member.UserID,
			&member.Role,
			&member.Status,
			&member.JoinSeq,
			&member.LeaveSeq,
			&member.MemberVersion,
			&member.PermissionVersion,
			&member.UpdatedAt,
		); err != nil {
			return types.ListConversationMembersResult{}, types.NewDBReadFailed(err.Error())
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return types.ListConversationMembersResult{}, types.NewDBReadFailed(err.Error())
	}
	if len(members) > pageSize {
		page := members[:pageSize]
		nextToken, err := encodeListMembersPageToken(page[len(page)-1].UserID, command.RoleFilter)
		if err != nil {
			return types.ListConversationMembersResult{}, types.NewDBReadFailed(err.Error())
		}
		result.Members = page
		result.NextPageToken = nextToken
		return result, nil
	}
	result.Members = members
	return result, nil
}

func decodeListMembersPageToken(token string, roleFilter types.MemberRole) (string, error) {
	if token == "" {
		return "", nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", types.NewInvalidArgument("page_token is invalid")
	}
	var decoded listMembersPageToken
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return "", types.NewInvalidArgument("page_token is invalid")
	}
	if decoded.Version == 1 {
		if roleFilter != "" || decoded.UserID == "" {
			return "", types.NewInvalidArgument("page_token is invalid")
		}
		return decoded.UserID, nil
	}
	if decoded.Version != 2 || decoded.UserID == "" || decoded.RoleFilter != string(roleFilter) {
		return "", types.NewInvalidArgument("page_token is invalid")
	}
	return decoded.UserID, nil
}

func encodeListMembersPageToken(userID types.UserID, roleFilter types.MemberRole) (string, error) {
	payload, err := json.Marshal(listMembersPageToken{
		Version:    2,
		UserID:     string(userID),
		RoleFilter: string(roleFilter),
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func (r *Repository) MarkPublishedMemberChanges(
	ctx context.Context,
	limit int,
) (types.MemberChangePublishProgressStats, error) {
	if r.pool == nil {
		return types.MemberChangePublishProgressStats{}, types.NewDBWriteFailed("repository is not configured")
	}
	limit = types.NormalizeMemberChangeProgressLimit(limit)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.MemberChangePublishProgressStats{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	rows, err := tx.Query(ctx, `
SELECT mcs.change_id
FROM member_change_saga mcs
JOIN message_outbox mo
  ON mo.tenant_id = mcs.tenant_id
 AND mo.event_id = mcs.outbox_event_id
 AND mo.conversation_id = mcs.conversation_id
WHERE mcs.status = $1
  AND mo.status = 'PUBLISHED'
  AND mo.published_at IS NOT NULL
  AND mo.producer = 'conversation-service'
  AND mo.event_type IN (
      'conversation.member.joined.v1',
      'conversation.member.left.v1',
      'conversation.member.removed.v1',
      'conversation.member.role_changed.v1',
      'conversation.member.owner_transferred.v1',
      'conversation.member.boundary_cancelled.v1'
  )
ORDER BY mcs.updated_at, mcs.change_id
LIMIT $2
FOR UPDATE OF mcs SKIP LOCKED
`,
		types.MemberChangeStatusOutboxEnqueued,
		limit,
	)
	if err != nil {
		return types.MemberChangePublishProgressStats{}, types.NewDBWriteFailed(err.Error())
	}

	changeIDs := make([]string, 0, limit)
	for rows.Next() {
		var changeID types.ChangeID
		if err := rows.Scan(&changeID); err != nil {
			rows.Close()
			return types.MemberChangePublishProgressStats{}, types.NewDBWriteFailed(err.Error())
		}
		changeIDs = append(changeIDs, string(changeID))
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return types.MemberChangePublishProgressStats{}, types.NewDBWriteFailed(err.Error())
	}
	rows.Close()
	if len(changeIDs) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return types.MemberChangePublishProgressStats{}, types.NewDBWriteFailed(err.Error())
		}
		return types.MemberChangePublishProgressStats{}, nil
	}

	if _, err := tx.Exec(ctx, `
UPDATE member_change_saga
SET status = $2,
    updated_at = now()
WHERE change_id = ANY($1)
`, changeIDs, types.MemberChangeStatusEventPublished); err != nil {
		return types.MemberChangePublishProgressStats{}, types.NewDBWriteFailed(err.Error())
	}
	if _, err := tx.Exec(ctx, `
UPDATE member_change_saga
SET status = $2,
    completed_at = COALESCE(completed_at, now()),
    updated_at = now()
WHERE change_id = ANY($1)
`, changeIDs, types.MemberChangeStatusDone); err != nil {
		return types.MemberChangePublishProgressStats{}, types.NewDBWriteFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return types.MemberChangePublishProgressStats{}, types.NewDBWriteFailed(err.Error())
	}
	return types.MemberChangePublishProgressStats{Advanced: len(changeIDs)}, nil
}

type existingMemberChange struct {
	ChangeID          types.ChangeID
	TenantID          types.TenantID
	ConversationID    types.ConversationID
	TargetUserID      types.UserID
	OperatorUserID    types.UserID
	ChangeType        types.MemberChangeType
	Status            types.MemberChangeStatus
	BoundarySeq       sql.NullInt64
	MemberVersion     sql.NullInt64
	PermissionVersion sql.NullInt64
	CommandHash       string
}

func lockMemberChangeIdempotency(ctx context.Context, tx pgx.Tx, command types.CreateMemberChangeCommand) error {
	_, err := tx.Exec(ctx, `
SELECT pg_advisory_xact_lock(hashtextextended($1, 0))
`, fmt.Sprintf("%s\x1f%s\x1f%s", command.AuthContext.TenantID, command.ConversationID, command.IdempotencyKey))
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func findExistingMemberChange(
	ctx context.Context,
	tx pgx.Tx,
	command types.CreateMemberChangeCommand,
) (existingMemberChange, bool, error) {
	row := tx.QueryRow(ctx, `
SELECT
    change_id,
    tenant_id,
    conversation_id,
    user_id,
    operator_id,
    change_type,
    status,
    boundary_seq,
    COALESCE((metadata_json->>'member_version')::bigint, 0),
    COALESCE((metadata_json->>'permission_version')::bigint, 0),
    command_hash
FROM member_change_saga
WHERE tenant_id = $1
  AND conversation_id = $2
  AND idempotency_key = $3
FOR UPDATE
`, command.AuthContext.TenantID, command.ConversationID, command.IdempotencyKey)
	var existing existingMemberChange
	if err := row.Scan(
		&existing.ChangeID,
		&existing.TenantID,
		&existing.ConversationID,
		&existing.TargetUserID,
		&existing.OperatorUserID,
		&existing.ChangeType,
		&existing.Status,
		&existing.BoundarySeq,
		&existing.MemberVersion.Int64,
		&existing.PermissionVersion.Int64,
		&existing.CommandHash,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return existingMemberChange{}, false, nil
		}
		return existingMemberChange{}, false, types.NewDBWriteFailed(err.Error())
	}
	existing.MemberVersion.Valid = existing.MemberVersion.Int64 > 0
	existing.PermissionVersion.Valid = existing.PermissionVersion.Int64 > 0
	return existing, true, nil
}

func replayMemberChange(
	ctx context.Context,
	tx pgx.Tx,
	existing existingMemberChange,
	commandHash string,
) (types.MemberChangeResult, error) {
	if existing.CommandHash != commandHash {
		return types.MemberChangeResult{}, types.NewMemberConflict("idempotency_key reused with different command")
	}
	if err := tx.Commit(ctx); err != nil {
		return types.MemberChangeResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.MemberChangeResult{
		ChangeID:          existing.ChangeID,
		TenantID:          existing.TenantID,
		ConversationID:    existing.ConversationID,
		TargetUserID:      existing.TargetUserID,
		OperatorUserID:    existing.OperatorUserID,
		ChangeType:        existing.ChangeType,
		Status:            existing.Status,
		BoundarySeq:       existing.BoundarySeq.Int64,
		MemberVersion:     existing.MemberVersion.Int64,
		PermissionVersion: existing.PermissionVersion.Int64,
		IdempotentReplay:  true,
	}, nil
}

func replayOwnerTransfer(
	ctx context.Context,
	tx pgx.Tx,
	existing existingMemberChange,
	commandHash string,
) (types.TransferConversationOwnerResult, error) {
	if existing.CommandHash != commandHash {
		return types.TransferConversationOwnerResult{}, types.NewMemberConflict("idempotency_key reused with different command")
	}
	if err := tx.Commit(ctx); err != nil {
		return types.TransferConversationOwnerResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.TransferConversationOwnerResult{
		ChangeID:            existing.ChangeID,
		TenantID:            existing.TenantID,
		ConversationID:      existing.ConversationID,
		PreviousOwnerUserID: existing.OperatorUserID,
		NewOwnerUserID:      existing.TargetUserID,
		Status:              existing.Status,
		BoundarySeq:         existing.BoundarySeq.Int64,
		MemberVersion:       existing.MemberVersion.Int64,
		PermissionVersion:   existing.PermissionVersion.Int64,
		IdempotentReplay:    true,
	}, nil
}

func ownerTransferMemberChangeCommand(command types.TransferConversationOwnerCommand) types.CreateMemberChangeCommand {
	return types.CreateMemberChangeCommand{
		AuthContext:           command.AuthContext,
		ConversationID:        command.ConversationID,
		TargetUserID:          command.NewOwnerUserID,
		ChangeType:            types.MemberChangeTypeOwnerTransfer,
		TargetRole:            types.MemberRoleOwner,
		ExpectedMemberVersion: command.ExpectedMemberVersion,
		IdempotencyKey:        command.IdempotencyKey,
		ConflictPolicy:        types.MemberChangeConflictPolicyReject,
		Reason:                command.Reason,
	}
}

func lockConversation(ctx context.Context, tx pgx.Tx, command types.CreateMemberChangeCommand) (domain.Conversation, error) {
	row := tx.QueryRow(ctx, `
SELECT
    status,
    conversation_mode,
    fanout_mode,
    fanout_policy_version,
    member_version,
    permission_version,
    current_seq_shard
FROM conversations
WHERE tenant_id = $1
  AND conversation_id = $2
FOR UPDATE
`, command.AuthContext.TenantID, command.ConversationID)
	conversation := domain.Conversation{
		TenantID:       command.AuthContext.TenantID,
		ConversationID: command.ConversationID,
	}
	if err := row.Scan(
		&conversation.Status,
		&conversation.ConversationMode,
		&conversation.FanoutMode,
		&conversation.FanoutPolicyVersion,
		&conversation.MemberVersion,
		&conversation.PermissionVersion,
		&conversation.CurrentSeqShard,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Conversation{}, types.NewConversationNotFound("conversation not found")
		}
		return domain.Conversation{}, types.NewDBWriteFailed(err.Error())
	}
	return conversation, nil
}

func lockConversationMember(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	conversationID types.ConversationID,
	userID types.UserID,
) (domain.Member, error) {
	row := tx.QueryRow(ctx, `
SELECT role, status, member_version, permission_version
FROM conversation_members
WHERE tenant_id = $1
  AND conversation_id = $2
  AND user_id = $3
FOR UPDATE
`, tenantID, conversationID, userID)
	member := domain.Member{UserID: userID}
	if err := row.Scan(
		&member.Role,
		&member.Status,
		&member.MemberVersion,
		&member.PermissionVersion,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return member, nil
		}
		return domain.Member{}, types.NewDBWriteFailed(err.Error())
	}
	return member, nil
}

func ensureConversationSeq(ctx context.Context, tx pgx.Tx, command types.CreateMemberChangeCommand) error {
	_, err := tx.Exec(ctx, `
INSERT INTO conversation_seq (tenant_id, conversation_id, current_seq)
VALUES ($1, $2, 0)
ON CONFLICT (tenant_id, conversation_id) DO NOTHING
`, command.AuthContext.TenantID, command.ConversationID)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func allocateConversationSeq(ctx context.Context, tx pgx.Tx, command types.CreateMemberChangeCommand) (int64, error) {
	row := tx.QueryRow(ctx, `
UPDATE conversation_seq
SET current_seq = current_seq + 1,
    updated_at = now()
WHERE tenant_id = $1
  AND conversation_id = $2
RETURNING current_seq
`, command.AuthContext.TenantID, command.ConversationID)
	var seq int64
	if err := row.Scan(&seq); err != nil {
		return 0, types.NewDBWriteFailed(err.Error())
	}
	return seq, nil
}

func insertMemberChangeSaga(ctx context.Context, tx pgx.Tx, change domain.MemberChange) error {
	_, err := tx.Exec(ctx, `
INSERT INTO member_change_saga (
    change_id,
    tenant_id,
    conversation_id,
    user_id,
    change_type,
    boundary_seq,
    status,
    idempotency_key,
    expected_member_version,
    command_hash,
    operator_id,
    conflict_policy,
    timeline_event_id,
    outbox_event_id,
    metadata_json,
    created_at,
    updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, 0), $10, $11, $12, $13, $14, $15::jsonb, $16, $16)
`,
		change.ChangeID,
		change.TenantID,
		change.ConversationID,
		change.TargetUserID,
		change.ChangeType,
		change.BoundarySeq,
		types.MemberChangeStatusBoundaryAllocated,
		change.IdempotencyKey,
		change.ExpectedMemberVersion,
		change.CommandHash,
		change.OperatorUserID,
		change.ConflictPolicy,
		change.TimelineEventID,
		change.OutboxEventID,
		change.MetadataJSON,
		change.CreatedAt,
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func upsertConversationMember(ctx context.Context, tx pgx.Tx, command types.CreateMemberChangeCommand, record domain.MemberChangeRecord) error {
	return upsertMemberMutation(ctx, tx, command.AuthContext.TenantID, command.ConversationID, record.Target)
}

func upsertMemberMutation(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	conversationID types.ConversationID,
	mutation domain.MemberMutation,
) error {
	_, err := tx.Exec(ctx, `
INSERT INTO conversation_members (
    tenant_id,
    conversation_id,
    user_id,
    role,
    status,
    join_seq,
    leave_seq,
    member_version,
    permission_version,
    created_at,
    updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now(), now())
ON CONFLICT (tenant_id, conversation_id, user_id) DO UPDATE
SET role = EXCLUDED.role,
    status = EXCLUDED.status,
    join_seq = COALESCE(EXCLUDED.join_seq, conversation_members.join_seq),
    leave_seq = CASE
        WHEN EXCLUDED.status = 'ACTIVE'
         AND EXCLUDED.join_seq IS NOT NULL
         AND EXCLUDED.leave_seq IS NULL THEN NULL
        ELSE COALESCE(EXCLUDED.leave_seq, conversation_members.leave_seq)
    END,
    member_version = EXCLUDED.member_version,
    permission_version = EXCLUDED.permission_version,
    updated_at = now()
`,
		tenantID,
		conversationID,
		mutation.UserID,
		mutation.NewRole,
		mutation.NewStatus,
		nullableInt64(mutation.JoinSeq),
		nullableInt64(mutation.LeaveSeq),
		mutation.MemberVersion,
		mutation.PermissionVersion,
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func updateConversationVersions(ctx context.Context, tx pgx.Tx, command types.CreateMemberChangeCommand, record domain.MemberChangeRecord) error {
	return updateConversationVersionValues(ctx, tx, command.AuthContext.TenantID, command.ConversationID, record.Target.MemberVersion, record.Target.PermissionVersion)
}

func updateConversationVersionValues(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	conversationID types.ConversationID,
	memberVersion int64,
	permissionVersion int64,
) error {
	_, err := tx.Exec(ctx, `
UPDATE conversations
SET member_version = $3,
    permission_version = $4,
    updated_at = now()
WHERE tenant_id = $1
  AND conversation_id = $2
`, tenantID, conversationID, memberVersion, permissionVersion)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertMemberTimelineEvent(ctx context.Context, tx pgx.Tx, event domain.TimelineEvent) error {
	_, err := tx.Exec(ctx, `
INSERT INTO conversation_timeline_events (
    tenant_id,
    conversation_id,
    seq,
    event_id,
    event_type,
    event_version,
    message_id,
    actor_id,
    fanout_mode,
    fanout_policy_version,
    permission_version,
    classification,
    mapping_version,
    trace_id,
    payload_json,
    created_at
) VALUES ($1, $2, $3, $4, $5, $6, NULL, $7, $8, $9, $10, $11, $12, $13, $14::jsonb, $15)
`,
		event.TenantID,
		event.ConversationID,
		event.ConversationSeq,
		event.EventID,
		event.EventType,
		event.EventVersion,
		event.ActorID,
		event.FanoutMode,
		event.FanoutPolicyVersion,
		event.PermissionVersion,
		event.Classification,
		event.MappingVersion,
		event.TraceID,
		event.PayloadJSON,
		event.CreatedAt,
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertMemberOutboxEvent(ctx context.Context, tx pgx.Tx, event domain.OutboxEvent) error {
	_, err := tx.Exec(ctx, `
INSERT INTO message_outbox (
    event_id,
    tenant_id,
    conversation_id,
    aggregate_version,
    event_type,
    event_version,
    partition_key,
    mapping_version,
    correlation_id,
    causation_id,
    producer,
    payload_json,
    trace_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13)
`,
		event.EventID,
		event.TenantID,
		event.ConversationID,
		event.AggregateVersion,
		event.EventType,
		event.EventVersion,
		event.PartitionKey,
		event.MappingVersion,
		event.CorrelationID,
		event.CausationID,
		event.Producer,
		event.PayloadJSON,
		event.TraceID,
	)
	if err != nil {
		return types.NewOutboxWriteFailed(err.Error())
	}
	return nil
}

func markMemberChangeOutboxEnqueued(ctx context.Context, tx pgx.Tx, change domain.MemberChange) error {
	_, err := tx.Exec(ctx, `
UPDATE member_change_saga
SET status = $2,
    metadata_json = $3::jsonb,
    updated_at = now()
WHERE change_id = $1
`, change.ChangeID, types.MemberChangeStatusOutboxEnqueued, change.MetadataJSON)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func memberChangeResult(record domain.MemberChangeRecord, replay bool) types.MemberChangeResult {
	return types.MemberChangeResult{
		ChangeID:          record.Change.ChangeID,
		TenantID:          record.Change.TenantID,
		ConversationID:    record.Change.ConversationID,
		TargetUserID:      record.Change.TargetUserID,
		OperatorUserID:    record.Change.OperatorUserID,
		ChangeType:        record.Change.ChangeType,
		Status:            record.Change.Status,
		BoundarySeq:       record.Change.BoundarySeq,
		MemberVersion:     record.Change.MemberVersion,
		PermissionVersion: record.Change.PermissionVersion,
		IdempotentReplay:  replay,
	}
}

func ownerTransferResult(record domain.OwnerTransferRecord, replay bool) types.TransferConversationOwnerResult {
	return types.TransferConversationOwnerResult{
		ChangeID:            record.Change.ChangeID,
		TenantID:            record.Change.TenantID,
		ConversationID:      record.Change.ConversationID,
		PreviousOwnerUserID: record.Change.OperatorUserID,
		NewOwnerUserID:      record.Change.TargetUserID,
		Status:              record.Change.Status,
		BoundarySeq:         record.Change.BoundarySeq,
		MemberVersion:       record.Change.MemberVersion,
		PermissionVersion:   record.Change.PermissionVersion,
		IdempotentReplay:    replay,
	}
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func newUUIDString() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate uuid: %w", err)
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		bytes[0:4],
		bytes[4:6],
		bytes[6:8],
		bytes[8:10],
		bytes[10:16],
	), nil
}
