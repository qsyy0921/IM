package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/conversation-service/internal/domain"
	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

func (r *Repository) CreateConversation(
	ctx context.Context,
	command types.CreateConversationCommand,
) (types.CreateConversationResult, error) {
	if r.pool == nil {
		return types.CreateConversationResult{}, types.NewDBWriteFailed("repository is not configured")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.CreateConversationResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := lockCreateConversationIdempotency(ctx, tx, command); err != nil {
		return types.CreateConversationResult{}, err
	}
	if existing, ok, err := findExistingConversationCreate(ctx, tx, command); err != nil {
		return types.CreateConversationResult{}, err
	} else if ok {
		return replayCreateConversation(ctx, tx, existing)
	}

	const boundarySeq int64 = 1
	eventIDs, err := r.createConversationEventIDs(command)
	if err != nil {
		return types.CreateConversationResult{}, err
	}
	record, err := domain.NewConversationCreateRecord(command, eventIDs, boundarySeq, r.now())
	if err != nil {
		return types.CreateConversationResult{}, err
	}
	if err := insertConversation(ctx, tx, record.Conversation); err != nil {
		return types.CreateConversationResult{}, err
	}
	if err := insertConversationSeq(ctx, tx, command.AuthContext.TenantID, command.ConversationID, record.BoundarySeq); err != nil {
		return types.CreateConversationResult{}, err
	}
	for _, member := range record.Members {
		if err := upsertMemberMutation(ctx, tx, command.AuthContext.TenantID, command.ConversationID, member); err != nil {
			return types.CreateConversationResult{}, err
		}
	}
	for _, event := range record.Timeline {
		if err := insertMemberTimelineEvent(ctx, tx, event); err != nil {
			return types.CreateConversationResult{}, err
		}
	}
	for _, event := range record.Outbox {
		if err := insertMemberOutboxEvent(ctx, tx, event); err != nil {
			return types.CreateConversationResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return types.CreateConversationResult{}, types.NewDBWriteFailed(err.Error())
	}
	return createConversationResult(record, false), nil
}

func (r *Repository) createConversationEventIDs(command types.CreateConversationCommand) ([]types.EventID, error) {
	count := 1
	if command.ConversationType == types.ConversationTypeDirect {
		count = 2
	}
	eventIDs := make([]types.EventID, 0, count)
	for i := 0; i < count; i++ {
		eventID, err := r.eventID()
		if err != nil {
			return nil, types.NewDBWriteFailed(err.Error())
		}
		eventIDs = append(eventIDs, eventID)
	}
	return eventIDs, nil
}

type existingConversationCreate struct {
	TenantID          types.TenantID
	ConversationID    types.ConversationID
	ConversationType  types.ConversationType
	DirectPeerUserID  types.UserID
	BoundarySeq       int64
	MemberVersion     int64
	PermissionVersion int64
}

func lockCreateConversationIdempotency(ctx context.Context, tx pgx.Tx, command types.CreateConversationCommand) error {
	_, err := tx.Exec(ctx, `
SELECT pg_advisory_xact_lock(hashtextextended($1, 0))
`, fmt.Sprintf("%s\x1f%s\x1f%s", command.AuthContext.TenantID, command.ConversationID, command.IdempotencyKey))
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func findExistingConversationCreate(
	ctx context.Context,
	tx pgx.Tx,
	command types.CreateConversationCommand,
) (existingConversationCreate, bool, error) {
	row := tx.QueryRow(ctx, `
SELECT
    c.tenant_id,
    c.conversation_id,
    c.conversation_type,
    CASE
        WHEN c.conversation_type = 'DIRECT' THEN COALESCE(seq.current_seq, c.member_version)
        ELSE COALESCE(m.join_seq, seq.current_seq, c.member_version)
    END,
    c.member_version,
    c.permission_version,
    COALESCE(peer.direct_peer_user_id, '')
FROM conversations c
JOIN conversation_members m
  ON m.tenant_id = c.tenant_id
 AND m.conversation_id = c.conversation_id
 AND m.user_id = $3
 AND m.status = 'ACTIVE'
LEFT JOIN conversation_seq seq
  ON seq.tenant_id = c.tenant_id
 AND seq.conversation_id = c.conversation_id
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
  AND c.status = 'ACTIVE'
FOR UPDATE OF c, m
`, command.AuthContext.TenantID, command.ConversationID, command.AuthContext.UserID)
	var existing existingConversationCreate
	if err := row.Scan(
		&existing.TenantID,
		&existing.ConversationID,
		&existing.ConversationType,
		&existing.BoundarySeq,
		&existing.MemberVersion,
		&existing.PermissionVersion,
		&existing.DirectPeerUserID,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return existingConversationCreate{}, false, nil
		}
		return existingConversationCreate{}, false, types.NewDBReadFailed(err.Error())
	}
	if existing.ConversationType != command.ConversationType {
		return existingConversationCreate{}, false, types.NewMemberConflict("conversation_id reused with different type")
	}
	if command.ConversationType == types.ConversationTypeGroup {
		if err := ensureExistingCreatorIsOwner(ctx, tx, command); err != nil {
			return existingConversationCreate{}, false, err
		}
	}
	if command.ConversationType == types.ConversationTypeDirect &&
		existing.DirectPeerUserID != command.DirectPeerUserID {
		return existingConversationCreate{}, false, types.NewMemberConflict("direct conversation peer mismatch")
	}
	return existing, true, nil
}

func ensureExistingCreatorIsOwner(ctx context.Context, tx pgx.Tx, command types.CreateConversationCommand) error {
	var role types.MemberRole
	err := tx.QueryRow(ctx, `
SELECT role
FROM conversation_members
WHERE tenant_id = $1
  AND conversation_id = $2
  AND user_id = $3
  AND status = 'ACTIVE'
`, command.AuthContext.TenantID, command.ConversationID, command.AuthContext.UserID).Scan(&role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.NewPermissionDenied("creator is not active")
		}
		return types.NewDBReadFailed(err.Error())
	}
	if role != types.MemberRoleOwner {
		return types.NewMemberConflict("conversation_id already exists")
	}
	return nil
}

func replayCreateConversation(ctx context.Context, tx pgx.Tx, existing existingConversationCreate) (types.CreateConversationResult, error) {
	if err := tx.Commit(ctx); err != nil {
		return types.CreateConversationResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.CreateConversationResult{
		TenantID:          existing.TenantID,
		ConversationID:    existing.ConversationID,
		ConversationType:  existing.ConversationType,
		DirectPeerUserID:  existing.DirectPeerUserID,
		BoundarySeq:       existing.BoundarySeq,
		MemberVersion:     existing.MemberVersion,
		PermissionVersion: existing.PermissionVersion,
		IdempotentReplay:  true,
	}, nil
}

func insertConversation(ctx context.Context, tx pgx.Tx, conversation domain.Conversation) error {
	tag, err := tx.Exec(ctx, `
INSERT INTO conversations (
    tenant_id,
    conversation_id,
    conversation_type,
    status,
    conversation_mode,
    fanout_mode,
    fanout_policy_version,
    member_version,
    permission_version,
    current_seq_shard
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (tenant_id, conversation_id) DO NOTHING
`,
		conversation.TenantID,
		conversation.ConversationID,
		conversation.ConversationType,
		conversation.Status,
		conversation.ConversationMode,
		conversation.FanoutMode,
		conversation.FanoutPolicyVersion,
		conversation.MemberVersion,
		conversation.PermissionVersion,
		conversation.CurrentSeqShard,
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if tag.RowsAffected() == 0 {
		return types.NewMemberConflict("conversation_id already exists")
	}
	return nil
}

func insertConversationSeq(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	conversationID types.ConversationID,
	seq int64,
) error {
	_, err := tx.Exec(ctx, `
INSERT INTO conversation_seq (tenant_id, conversation_id, current_seq)
VALUES ($1, $2, $3)
`, tenantID, conversationID, seq)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func createConversationResult(record domain.ConversationCreateRecord, replay bool) types.CreateConversationResult {
	return types.CreateConversationResult{
		TenantID:          record.Conversation.TenantID,
		ConversationID:    record.Conversation.ConversationID,
		ConversationType:  record.Conversation.ConversationType,
		DirectPeerUserID:  record.Conversation.DirectPeerUserID,
		BoundarySeq:       record.BoundarySeq,
		MemberVersion:     record.Conversation.MemberVersion,
		PermissionVersion: record.Conversation.PermissionVersion,
		IdempotentReplay:  replay,
	}
}
