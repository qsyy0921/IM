package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
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
    c.conversation_mode,
    c.fanout_mode,
    c.fanout_policy_version,
    c.member_version,
    c.permission_version,
    c.current_seq_shard,
    COALESCE(m.status, ''),
    COALESCE(m.member_version, 0),
    COALESCE(m.permission_version, 0)
FROM conversations c
LEFT JOIN conversation_members m
  ON m.tenant_id = c.tenant_id
 AND m.conversation_id = c.conversation_id
 AND m.user_id = $3
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
		&conversation.ConversationMode,
		&conversation.FanoutMode,
		&conversation.FanoutPolicyVersion,
		&conversation.MemberVersion,
		&conversation.PermissionVersion,
		&conversation.CurrentSeqShard,
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

	if err := insertMemberChangeSaga(ctx, tx, record); err != nil {
		return types.MemberChangeResult{}, err
	}
	if err := upsertConversationMember(ctx, tx, command, record); err != nil {
		return types.MemberChangeResult{}, err
	}
	if err := updateConversationVersions(ctx, tx, command, record); err != nil {
		return types.MemberChangeResult{}, err
	}
	if err := insertMemberTimelineEvent(ctx, tx, record); err != nil {
		return types.MemberChangeResult{}, err
	}
	if err := insertMemberOutboxEvent(ctx, tx, record); err != nil {
		return types.MemberChangeResult{}, err
	}
	if err := markMemberChangeOutboxEnqueued(ctx, tx, record); err != nil {
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
    COALESCE(last_error, ''),
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

func (r *Repository) MarkPublishedMemberChanges(
	ctx context.Context,
	limit int,
) (types.MemberChangePublishProgressStats, error) {
	if r.pool == nil {
		return types.MemberChangePublishProgressStats{}, types.NewDBWriteFailed("repository is not configured")
	}
	if limit <= 0 {
		limit = 100
	}
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
WHERE mcs.status = $1
  AND mo.status = 'PUBLISHED'
  AND mo.published_at IS NOT NULL
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

func insertMemberChangeSaga(ctx context.Context, tx pgx.Tx, record domain.MemberChangeRecord) error {
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
		record.Change.ChangeID,
		record.Change.TenantID,
		record.Change.ConversationID,
		record.Change.TargetUserID,
		record.Change.ChangeType,
		record.Change.BoundarySeq,
		types.MemberChangeStatusBoundaryAllocated,
		record.Change.IdempotencyKey,
		record.Change.ExpectedMemberVersion,
		record.Change.CommandHash,
		record.Change.OperatorUserID,
		record.Change.ConflictPolicy,
		record.Change.TimelineEventID,
		record.Change.OutboxEventID,
		record.Change.MetadataJSON,
		record.Change.CreatedAt,
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func upsertConversationMember(ctx context.Context, tx pgx.Tx, command types.CreateMemberChangeCommand, record domain.MemberChangeRecord) error {
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
    leave_seq = COALESCE(EXCLUDED.leave_seq, conversation_members.leave_seq),
    member_version = EXCLUDED.member_version,
    permission_version = EXCLUDED.permission_version,
    updated_at = now()
`,
		command.AuthContext.TenantID,
		command.ConversationID,
		record.Target.UserID,
		record.Target.NewRole,
		record.Target.NewStatus,
		nullableInt64(record.Target.JoinSeq),
		nullableInt64(record.Target.LeaveSeq),
		record.Target.MemberVersion,
		record.Target.PermissionVersion,
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func updateConversationVersions(ctx context.Context, tx pgx.Tx, command types.CreateMemberChangeCommand, record domain.MemberChangeRecord) error {
	_, err := tx.Exec(ctx, `
UPDATE conversations
SET member_version = $3,
    permission_version = $4,
    updated_at = now()
WHERE tenant_id = $1
  AND conversation_id = $2
`, command.AuthContext.TenantID, command.ConversationID, record.Target.MemberVersion, record.Target.PermissionVersion)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertMemberTimelineEvent(ctx context.Context, tx pgx.Tx, record domain.MemberChangeRecord) error {
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
		record.Timeline.TenantID,
		record.Timeline.ConversationID,
		record.Timeline.ConversationSeq,
		record.Timeline.EventID,
		record.Timeline.EventType,
		record.Timeline.EventVersion,
		record.Timeline.ActorID,
		record.Timeline.FanoutMode,
		record.Timeline.FanoutPolicyVersion,
		record.Timeline.PermissionVersion,
		record.Timeline.Classification,
		record.Timeline.MappingVersion,
		record.Timeline.TraceID,
		record.Timeline.PayloadJSON,
		record.Timeline.CreatedAt,
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertMemberOutboxEvent(ctx context.Context, tx pgx.Tx, record domain.MemberChangeRecord) error {
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
		record.Outbox.EventID,
		record.Outbox.TenantID,
		record.Outbox.ConversationID,
		record.Outbox.AggregateVersion,
		record.Outbox.EventType,
		record.Outbox.EventVersion,
		record.Outbox.PartitionKey,
		record.Outbox.MappingVersion,
		record.Outbox.CorrelationID,
		record.Outbox.CausationID,
		record.Outbox.Producer,
		record.Outbox.PayloadJSON,
		record.Outbox.TraceID,
	)
	if err != nil {
		return types.NewOutboxWriteFailed(err.Error())
	}
	return nil
}

func markMemberChangeOutboxEnqueued(ctx context.Context, tx pgx.Tx, record domain.MemberChangeRecord) error {
	_, err := tx.Exec(ctx, `
UPDATE member_change_saga
SET status = $2,
    metadata_json = $3::jsonb,
    updated_at = now()
WHERE change_id = $1
`, record.Change.ChangeID, types.MemberChangeStatusOutboxEnqueued, record.Change.MetadataJSON)
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
