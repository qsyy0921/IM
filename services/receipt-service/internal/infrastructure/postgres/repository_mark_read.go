package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/receipt-service/internal/domain"
	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

func (repository *Repository) MarkRead(
	ctx context.Context,
	command types.MarkReadCommand,
) (types.MarkReadResult, error) {
	if err := validateAccessContext(command.AuthContext.TenantID, command.ConversationID, command.AccessContext); err != nil {
		return types.MarkReadResult{}, err
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.MarkReadResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := lockReadKey(ctx, tx, command); err != nil {
		return types.MarkReadResult{}, err
	}
	current, err := lockReadCursor(ctx, tx, command)
	if err != nil {
		return types.MarkReadResult{}, err
	}
	maxVisible, err := maxVisibleSeq(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID)
	if err != nil {
		return types.MarkReadResult{}, err
	}
	maxReceived, err := maxReceivedSeq(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID)
	if err != nil {
		return types.MarkReadResult{}, err
	}
	next, err := domain.MergeReadCursor(current, command.ReadSeq, maxVisible, maxReceived)
	if err != nil {
		return types.MarkReadResult{}, err
	}
	if next > current {
		if err := upsertReadCursor(ctx, tx, command, next); err != nil {
			return types.MarkReadResult{}, err
		}
		if err := markReadStates(ctx, tx, command, next); err != nil {
			return types.MarkReadResult{}, err
		}
		if err := insertReadOutbox(ctx, tx, command, next); err != nil {
			return types.MarkReadResult{}, err
		}
		if err := updateConversationSummaryAfterRead(ctx, tx, command, next); err != nil {
			return types.MarkReadResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return types.MarkReadResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.MarkReadResult{
		TenantID:       command.AuthContext.TenantID,
		UserID:         command.AuthContext.UserID,
		ConversationID: command.ConversationID,
		LastReadSeq:    next,
	}, nil
}

func lockReadKey(ctx context.Context, tx pgx.Tx, command types.MarkReadCommand) error {
	return lockConversationSummaryKey(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID)
}

func lockConversationSummaryKey(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	userID types.UserID,
	conversationID types.ConversationID,
) error {
	key := fmt.Sprintf("%s\x1f%s\x1f%s\x1fconversation_summary", tenantID, userID, conversationID)
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func lockReadCursor(ctx context.Context, tx pgx.Tx, command types.MarkReadCommand) (int64, error) {
	var current int64
	err := tx.QueryRow(ctx, `
SELECT COALESCE(last_read_seq, 0)
FROM user_read_cursors
WHERE tenant_id = $1
  AND user_id = $2
  AND conversation_id = $3
FOR UPDATE
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID).Scan(&current)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, types.NewDBReadFailed(err.Error())
	}
	return current, nil
}

func maxVisibleSeq(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, conversationID types.ConversationID) (int64, error) {
	var maxSeq int64
	err := tx.QueryRow(ctx, `
SELECT COALESCE(MAX(conversation_seq), 0)
FROM receipt_inbox_projection
WHERE tenant_id = $1
  AND user_id = $2
  AND conversation_id = $3
`, tenantID, userID, conversationID).Scan(&maxSeq)
	if err != nil {
		return 0, types.NewDBReadFailed(err.Error())
	}
	return maxSeq, nil
}

func maxReceivedSeq(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, conversationID types.ConversationID) (int64, error) {
	var maxSeq int64
	err := tx.QueryRow(ctx, `
SELECT COALESCE(last_received_seq, 0)
FROM user_received_cursors
WHERE tenant_id = $1
  AND user_id = $2
  AND conversation_id = $3
`, tenantID, userID, conversationID).Scan(&maxSeq)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, types.NewDBReadFailed(err.Error())
	}
	return maxSeq, nil
}

func upsertReadCursor(ctx context.Context, tx pgx.Tx, command types.MarkReadCommand, readSeq int64) error {
	_, err := tx.Exec(ctx, `
INSERT INTO user_read_cursors (
    tenant_id,
    user_id,
    conversation_id,
    last_read_seq,
    updated_at
) VALUES ($1, $2, $3, $4, now())
ON CONFLICT (tenant_id, user_id, conversation_id) DO UPDATE
SET last_read_seq = GREATEST(user_read_cursors.last_read_seq, EXCLUDED.last_read_seq),
    updated_at = now()
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID, readSeq)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func updateConversationSummaryAfterRead(ctx context.Context, tx pgx.Tx, command types.MarkReadCommand, readSeq int64) error {
	_, err := tx.Exec(ctx, `
UPDATE user_conversation_summaries
SET last_read_seq = GREATEST(last_read_seq, $4),
    unread_count = (
        SELECT COUNT(*)
        FROM receipt_inbox_projection
        WHERE tenant_id = $1
          AND user_id = $2
          AND conversation_id = $3
          AND source_event_type = 'message.persisted.v1'
          AND conversation_seq > $4
    ),
    updated_at = now()
WHERE tenant_id = $1
  AND user_id = $2
  AND conversation_id = $3
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID, readSeq)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func markReadStates(ctx context.Context, tx pgx.Tx, command types.MarkReadCommand, readSeq int64) error {
	_, err := tx.Exec(ctx, `
UPDATE message_receipt_states
SET read_at = COALESCE(read_at, now()),
    updated_at = now()
WHERE tenant_id = $1
  AND user_id = $2
  AND conversation_id = $3
  AND conversation_seq <= $4
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID, readSeq)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}
