package postgres

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

const (
	MessageLegalHoldStatusActive   = "ACTIVE"
	MessageLegalHoldStatusReleased = "RELEASED"
)

type MessageLegalHoldMutationOptions struct {
	TenantID       string
	ConversationID string
	MessageID      string
	HoldID         string
	OperatorID     string
	Reason         string
	Now            time.Time
}

type MessageLegalHoldMutationResult struct {
	TenantID       string
	ConversationID string
	MessageID      string
	HoldID         string
	Status         string
	ReasonPresent  bool
	CreatedBy      string
	CreatedAt      time.Time
	ReleasedBy     string
	ReleasedAt     *time.Time
	UpdatedAt      time.Time
}

type MessageLegalHoldAuditOptions struct {
	TenantID       string
	ConversationID string
	MessageID      string
	HoldID         string
	Status         string
	Limit          int
}

type MessageLegalHoldAuditRow = MessageLegalHoldMutationResult

func (r *MessageRepository) SetMessageLegalHold(ctx context.Context, options MessageLegalHoldMutationOptions) (MessageLegalHoldMutationResult, error) {
	if r.pool == nil {
		return MessageLegalHoldMutationResult{}, ErrRepositoryNotConfigured
	}
	options = normalizeMessageLegalHoldMutationOptions(options, r.now())
	if err := validateMessageLegalHoldMutationOptions(options, true); err != nil {
		return MessageLegalHoldMutationResult{}, err
	}

	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return MessageLegalHoldMutationResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer conn.Release()
	tx, err := conn.Begin(ctx)
	if err != nil {
		return MessageLegalHoldMutationResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := ensureMessageExistsForLegalHold(ctx, tx, options); err != nil {
		return MessageLegalHoldMutationResult{}, err
	}
	if err := assertLegalHoldIDMatchesMessage(ctx, tx, options); err != nil {
		return MessageLegalHoldMutationResult{}, err
	}
	row := tx.QueryRow(ctx, `
INSERT INTO message_legal_holds (
    tenant_id,
    conversation_id,
    message_id,
    hold_id,
    status,
    reason,
    created_by,
    created_at,
    released_by,
    released_at,
    updated_at
) VALUES ($1, $2, $3, $4, 'ACTIVE', $5, $6, $7, '', NULL, $7)
ON CONFLICT (tenant_id, hold_id) DO UPDATE
SET status = 'ACTIVE',
    reason = EXCLUDED.reason,
    created_by = EXCLUDED.created_by,
    created_at = EXCLUDED.created_at,
    released_by = '',
    released_at = NULL,
    updated_at = EXCLUDED.updated_at
RETURNING tenant_id, conversation_id, message_id, hold_id, status, reason <> '', created_by, created_at, released_by, released_at, updated_at
`, options.TenantID, options.ConversationID, options.MessageID, options.HoldID, options.Reason, options.OperatorID, options.Now)
	result, err := scanMessageLegalHoldRow(row)
	if err != nil {
		return MessageLegalHoldMutationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return MessageLegalHoldMutationResult{}, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func (r *MessageRepository) ReleaseMessageLegalHold(ctx context.Context, options MessageLegalHoldMutationOptions) (MessageLegalHoldMutationResult, error) {
	if r.pool == nil {
		return MessageLegalHoldMutationResult{}, ErrRepositoryNotConfigured
	}
	options = normalizeMessageLegalHoldMutationOptions(options, r.now())
	if err := validateMessageLegalHoldMutationOptions(options, false); err != nil {
		return MessageLegalHoldMutationResult{}, err
	}

	row := r.pool.QueryRow(ctx, `
UPDATE message_legal_holds
SET status = 'RELEASED',
    released_by = $3,
    released_at = $4,
    updated_at = $4
WHERE tenant_id = $1
  AND hold_id = $2
  AND status = 'ACTIVE'
RETURNING tenant_id, conversation_id, message_id, hold_id, status, reason <> '', created_by, created_at, released_by, released_at, updated_at
`, options.TenantID, options.HoldID, options.OperatorID, options.Now)
	result, err := scanMessageLegalHoldRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MessageLegalHoldMutationResult{}, types.NewMessageNotFound("active legal hold not found")
		}
		return MessageLegalHoldMutationResult{}, err
	}
	return result, nil
}

func (r *MessageRepository) AuditMessageLegalHolds(ctx context.Context, options MessageLegalHoldAuditOptions) ([]MessageLegalHoldAuditRow, error) {
	if r.pool == nil {
		return nil, ErrRepositoryNotConfigured
	}
	limit := options.Limit
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	status := strings.ToUpper(strings.TrimSpace(options.Status))
	if status != "" && status != MessageLegalHoldStatusActive && status != MessageLegalHoldStatusReleased {
		return nil, errors.New("unsupported message legal hold status")
	}

	args := []any{}
	clauses := []string{"1 = 1"}
	addClause := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, clause)
	}
	if value := strings.TrimSpace(options.TenantID); value != "" {
		addClause("tenant_id = $"+strconv.Itoa(len(args)+1), value)
	}
	if value := strings.TrimSpace(options.ConversationID); value != "" {
		addClause("conversation_id = $"+strconv.Itoa(len(args)+1), value)
	}
	if value := strings.TrimSpace(options.MessageID); value != "" {
		addClause("message_id = $"+strconv.Itoa(len(args)+1), value)
	}
	if value := strings.TrimSpace(options.HoldID); value != "" {
		addClause("hold_id = $"+strconv.Itoa(len(args)+1), value)
	}
	if status != "" {
		addClause("status = $"+strconv.Itoa(len(args)+1), status)
	}
	args = append(args, limit)

	rows, err := r.pool.Query(ctx, `
SELECT tenant_id, conversation_id, message_id, hold_id, status, reason <> '', created_by, created_at, released_by, released_at, updated_at
FROM message_legal_holds
WHERE `+strings.Join(clauses, " AND ")+`
ORDER BY updated_at DESC, id DESC
LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer rows.Close()

	result := make([]MessageLegalHoldAuditRow, 0, limit)
	for rows.Next() {
		row, err := scanMessageLegalHoldRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func ensureMessageExistsForLegalHold(ctx context.Context, tx pgx.Tx, options MessageLegalHoldMutationOptions) error {
	var exists bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM message_log
    WHERE tenant_id = $1
      AND conversation_id = $2
      AND message_id = $3
)
`, options.TenantID, options.ConversationID, options.MessageID).Scan(&exists); err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if !exists {
		return types.NewMessageNotFound("message not found")
	}
	return nil
}

func assertLegalHoldIDMatchesMessage(ctx context.Context, tx pgx.Tx, options MessageLegalHoldMutationOptions) error {
	var conversationID, messageID string
	err := tx.QueryRow(ctx, `
SELECT conversation_id, message_id
FROM message_legal_holds
WHERE tenant_id = $1
  AND hold_id = $2
FOR UPDATE
`, options.TenantID, options.HoldID).Scan(&conversationID, &messageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if conversationID != options.ConversationID || messageID != options.MessageID {
		return types.NewIdempotencyConflict("legal hold id belongs to another message")
	}
	return nil
}

type messageLegalHoldScanner interface {
	Scan(dest ...any) error
}

func scanMessageLegalHoldRow(scanner messageLegalHoldScanner) (MessageLegalHoldMutationResult, error) {
	var row MessageLegalHoldMutationResult
	if err := scanner.Scan(
		&row.TenantID,
		&row.ConversationID,
		&row.MessageID,
		&row.HoldID,
		&row.Status,
		&row.ReasonPresent,
		&row.CreatedBy,
		&row.CreatedAt,
		&row.ReleasedBy,
		&row.ReleasedAt,
		&row.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MessageLegalHoldMutationResult{}, pgx.ErrNoRows
		}
		return MessageLegalHoldMutationResult{}, types.NewDBWriteFailed(err.Error())
	}
	return row, nil
}

func normalizeMessageLegalHoldMutationOptions(options MessageLegalHoldMutationOptions, fallbackNow time.Time) MessageLegalHoldMutationOptions {
	options.TenantID = strings.TrimSpace(options.TenantID)
	options.ConversationID = strings.TrimSpace(options.ConversationID)
	options.MessageID = strings.TrimSpace(options.MessageID)
	options.HoldID = strings.TrimSpace(options.HoldID)
	options.OperatorID = strings.TrimSpace(options.OperatorID)
	options.Reason = strings.TrimSpace(options.Reason)
	if options.Now.IsZero() {
		options.Now = fallbackNow
	}
	return options
}

func validateMessageLegalHoldMutationOptions(options MessageLegalHoldMutationOptions, requireMessage bool) error {
	if options.TenantID == "" {
		return errors.New("tenant_id is required")
	}
	if requireMessage && (options.ConversationID == "" || options.MessageID == "") {
		return errors.New("conversation_id and message_id are required")
	}
	if options.HoldID == "" {
		return errors.New("hold_id is required")
	}
	if options.OperatorID == "" {
		return errors.New("operator_id is required")
	}
	return nil
}
