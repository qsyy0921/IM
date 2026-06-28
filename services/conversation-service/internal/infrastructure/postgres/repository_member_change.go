package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/conversation-service/internal/domain"
	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

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
    conversation_type,
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
		&conversation.ConversationType,
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

func applyConversationScalePolicyAfterMemberChange(
	ctx context.Context,
	tx pgx.Tx,
	conversation domain.Conversation,
) (domain.ConversationScalePolicy, error) {
	currentPolicy := domain.ConversationScalePolicy{
		Runtime:             domain.ConversationScaleRuntimeActive,
		ConversationMode:    conversation.ConversationMode,
		FanoutMode:          conversation.FanoutMode,
		FanoutPolicyVersion: conversation.FanoutPolicyVersion,
		CurrentSeqShard:     conversation.CurrentSeqShard,
	}
	if conversation.ConversationType != types.ConversationTypeGroup {
		return currentPolicy, nil
	}
	activeMemberCount, err := countActiveConversationMembers(ctx, tx, conversation.TenantID, conversation.ConversationID)
	if err != nil {
		return domain.ConversationScalePolicy{}, err
	}
	resolvedPolicy, err := domain.ResolveConversationScalePolicy(conversation.ConversationType, activeMemberCount)
	if err != nil {
		return domain.ConversationScalePolicy{}, err
	}
	if resolvedPolicy.Runtime != domain.ConversationScaleRuntimeActive {
		return domain.ConversationScalePolicy{}, types.NewSequencerUnavailable("conversation scale policy is not active")
	}
	if resolvedPolicy.FanoutPolicyVersion <= conversation.FanoutPolicyVersion {
		return currentPolicy, nil
	}
	if err := promoteConversationScalePolicy(ctx, tx, conversation, resolvedPolicy); err != nil {
		return domain.ConversationScalePolicy{}, err
	}
	return resolvedPolicy, nil
}

func countActiveConversationMembers(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	conversationID types.ConversationID,
) (int64, error) {
	var activeMemberCount int64
	err := tx.QueryRow(ctx, `
SELECT COUNT(*)
FROM conversation_members
WHERE tenant_id = $1
  AND conversation_id = $2
  AND status = 'ACTIVE'
`, tenantID, conversationID).Scan(&activeMemberCount)
	if err != nil {
		return 0, types.NewDBReadFailed(err.Error())
	}
	return activeMemberCount, nil
}

func promoteConversationScalePolicy(
	ctx context.Context,
	tx pgx.Tx,
	conversation domain.Conversation,
	policy domain.ConversationScalePolicy,
) error {
	_, err := tx.Exec(ctx, `
UPDATE conversations
SET conversation_mode = $3,
    fanout_mode = $4,
    fanout_policy_version = $5,
    current_seq_shard = $6,
    updated_at = now()
WHERE tenant_id = $1
  AND conversation_id = $2
`, conversation.TenantID, conversation.ConversationID, policy.ConversationMode, policy.FanoutMode, policy.FanoutPolicyVersion, policy.CurrentSeqShard)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
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
