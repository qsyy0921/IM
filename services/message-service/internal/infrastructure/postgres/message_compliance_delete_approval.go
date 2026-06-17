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
	MessageComplianceApprovalStatusApproved = "APPROVED"
	MessageComplianceApprovalStatusConsumed = "CONSUMED"
	MessageComplianceApprovalStatusCanceled = "CANCELED"
)

type MessageComplianceDeleteApprovalMutationOptions struct {
	TenantID         string
	ConversationID   string
	MessageID        string
	ApprovalID       string
	ExternalProofRef string
	OperatorID       string
	Reason           string
	Now              time.Time
}

type MessageComplianceDeleteApprovalResult struct {
	TenantID         string
	ConversationID   string
	MessageID        string
	ApprovalID       string
	Status           string
	ExternalProofRef string
	ReasonPresent    bool
	ApprovedBy       string
	ApprovedAt       time.Time
	ConsumedBy       string
	ConsumedEventID  string
	ConsumedAt       *time.Time
	CanceledBy       string
	CanceledAt       *time.Time
	UpdatedAt        time.Time
}

type MessageComplianceDeleteApprovalAuditOptions struct {
	TenantID       string
	ConversationID string
	MessageID      string
	ApprovalID     string
	Status         string
	UpdatedAfter   *time.Time
	UpdatedBefore  *time.Time
	Limit          int
}

type MessageComplianceDeleteApprovalAuditRow = MessageComplianceDeleteApprovalResult

func (r *MessageRepository) ApproveComplianceDelete(ctx context.Context, options MessageComplianceDeleteApprovalMutationOptions) (MessageComplianceDeleteApprovalResult, error) {
	if r.pool == nil {
		return MessageComplianceDeleteApprovalResult{}, ErrRepositoryNotConfigured
	}
	options = normalizeComplianceDeleteApprovalOptions(options, r.now())
	if err := validateComplianceDeleteApprovalOptions(options, true, true); err != nil {
		return MessageComplianceDeleteApprovalResult{}, err
	}

	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return MessageComplianceDeleteApprovalResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer conn.Release()
	tx, err := conn.Begin(ctx)
	if err != nil {
		return MessageComplianceDeleteApprovalResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := ensureMessageExistsForComplianceApproval(ctx, tx, options); err != nil {
		return MessageComplianceDeleteApprovalResult{}, err
	}
	if err := assertComplianceApprovalIDReusableForMessage(ctx, tx, options); err != nil {
		return MessageComplianceDeleteApprovalResult{}, err
	}
	if err := lockVerifiedComplianceExternalProof(ctx, tx, options.TenantID, options.ExternalProofRef); err != nil {
		return MessageComplianceDeleteApprovalResult{}, err
	}
	row := tx.QueryRow(ctx, `
INSERT INTO message_compliance_delete_approvals (
    tenant_id,
    conversation_id,
    message_id,
    approval_id,
    status,
    external_proof_ref,
    reason,
    approved_by,
    approved_at,
    updated_at
) VALUES ($1, $2, $3, $4, 'APPROVED', $5, $6, $7, $8, $8)
ON CONFLICT (tenant_id, approval_id) DO UPDATE
SET status = 'APPROVED',
    external_proof_ref = EXCLUDED.external_proof_ref,
    reason = EXCLUDED.reason,
    approved_by = EXCLUDED.approved_by,
    approved_at = EXCLUDED.approved_at,
    consumed_by = '',
    consumed_event_id = '',
    consumed_at = NULL,
    canceled_by = '',
    canceled_at = NULL,
    updated_at = EXCLUDED.updated_at
RETURNING tenant_id, conversation_id, message_id, approval_id, status, external_proof_ref, reason <> '', approved_by, approved_at, consumed_by, consumed_event_id, consumed_at, canceled_by, canceled_at, updated_at
`, options.TenantID, options.ConversationID, options.MessageID, options.ApprovalID, options.ExternalProofRef, options.Reason, options.OperatorID, options.Now)
	result, err := scanComplianceDeleteApprovalRow(row)
	if err != nil {
		return MessageComplianceDeleteApprovalResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return MessageComplianceDeleteApprovalResult{}, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func (r *MessageRepository) CancelComplianceDeleteApproval(ctx context.Context, options MessageComplianceDeleteApprovalMutationOptions) (MessageComplianceDeleteApprovalResult, error) {
	if r.pool == nil {
		return MessageComplianceDeleteApprovalResult{}, ErrRepositoryNotConfigured
	}
	options = normalizeComplianceDeleteApprovalOptions(options, r.now())
	if err := validateComplianceDeleteApprovalOptions(options, false, false); err != nil {
		return MessageComplianceDeleteApprovalResult{}, err
	}

	row := r.pool.QueryRow(ctx, `
UPDATE message_compliance_delete_approvals
SET status = 'CANCELED',
    canceled_by = $3,
    canceled_at = $4,
    updated_at = $4
WHERE tenant_id = $1
  AND approval_id = $2
  AND status = 'APPROVED'
RETURNING tenant_id, conversation_id, message_id, approval_id, status, external_proof_ref, reason <> '', approved_by, approved_at, consumed_by, consumed_event_id, consumed_at, canceled_by, canceled_at, updated_at
`, options.TenantID, options.ApprovalID, options.OperatorID, options.Now)
	result, err := scanComplianceDeleteApprovalRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MessageComplianceDeleteApprovalResult{}, types.NewMessageNotFound("approved compliance delete approval not found")
		}
		return MessageComplianceDeleteApprovalResult{}, err
	}
	return result, nil
}

func (r *MessageRepository) AuditComplianceDeleteApprovals(ctx context.Context, options MessageComplianceDeleteApprovalAuditOptions) ([]MessageComplianceDeleteApprovalAuditRow, error) {
	if r.pool == nil {
		return nil, ErrRepositoryNotConfigured
	}
	limit := options.Limit
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	status := strings.ToUpper(strings.TrimSpace(options.Status))
	if status != "" &&
		status != MessageComplianceApprovalStatusApproved &&
		status != MessageComplianceApprovalStatusConsumed &&
		status != MessageComplianceApprovalStatusCanceled {
		return nil, errors.New("unsupported message compliance delete approval status")
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
	if value := strings.TrimSpace(options.ApprovalID); value != "" {
		addClause("approval_id = $"+strconv.Itoa(len(args)+1), value)
	}
	if status != "" {
		addClause("status = $"+strconv.Itoa(len(args)+1), status)
	}
	if options.UpdatedAfter != nil {
		addClause("updated_at >= $"+strconv.Itoa(len(args)+1), options.UpdatedAfter.UTC())
	}
	if options.UpdatedBefore != nil {
		addClause("updated_at < $"+strconv.Itoa(len(args)+1), options.UpdatedBefore.UTC())
	}
	if options.UpdatedAfter != nil && options.UpdatedBefore != nil && !options.UpdatedAfter.Before(*options.UpdatedBefore) {
		return nil, types.NewInvalidArgument("updated_after must be before updated_before")
	}
	args = append(args, limit)
	rows, err := r.pool.Query(ctx, `
SELECT tenant_id, conversation_id, message_id, approval_id, status, external_proof_ref, reason <> '', approved_by, approved_at, consumed_by, consumed_event_id, consumed_at, canceled_by, canceled_at, updated_at
FROM message_compliance_delete_approvals
WHERE `+strings.Join(clauses, " AND ")+`
ORDER BY updated_at DESC, id DESC
LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer rows.Close()
	result := make([]MessageComplianceDeleteApprovalAuditRow, 0, limit)
	for rows.Next() {
		row, err := scanComplianceDeleteApprovalRow(rows)
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

func ensureMessageExistsForComplianceApproval(ctx context.Context, tx pgx.Tx, options MessageComplianceDeleteApprovalMutationOptions) error {
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

func assertComplianceApprovalIDReusableForMessage(ctx context.Context, tx pgx.Tx, options MessageComplianceDeleteApprovalMutationOptions) error {
	var conversationID, messageID, status string
	err := tx.QueryRow(ctx, `
SELECT conversation_id, message_id, status
FROM message_compliance_delete_approvals
WHERE tenant_id = $1
  AND approval_id = $2
FOR UPDATE
`, options.TenantID, options.ApprovalID).Scan(&conversationID, &messageID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if conversationID != options.ConversationID || messageID != options.MessageID {
		return types.NewIdempotencyConflict("compliance approval id belongs to another message")
	}
	if status == MessageComplianceApprovalStatusConsumed || status == MessageComplianceApprovalStatusCanceled {
		return types.NewInvalidMessageState("terminal compliance delete approval cannot be re-approved")
	}
	return nil
}

func normalizeComplianceDeleteApprovalOptions(options MessageComplianceDeleteApprovalMutationOptions, fallbackNow time.Time) MessageComplianceDeleteApprovalMutationOptions {
	options.TenantID = strings.TrimSpace(options.TenantID)
	options.ConversationID = strings.TrimSpace(options.ConversationID)
	options.MessageID = strings.TrimSpace(options.MessageID)
	options.ApprovalID = strings.TrimSpace(options.ApprovalID)
	options.ExternalProofRef = strings.TrimSpace(options.ExternalProofRef)
	options.OperatorID = strings.TrimSpace(options.OperatorID)
	options.Reason = strings.TrimSpace(options.Reason)
	if options.Now.IsZero() {
		options.Now = fallbackNow
	}
	return options
}

func validateComplianceDeleteApprovalOptions(options MessageComplianceDeleteApprovalMutationOptions, requireMessage bool, requireProof bool) error {
	if options.TenantID == "" {
		return errors.New("tenant_id is required")
	}
	if requireMessage && (options.ConversationID == "" || options.MessageID == "") {
		return errors.New("conversation_id and message_id are required")
	}
	if options.ApprovalID == "" {
		return errors.New("approval_id is required")
	}
	if requireProof && options.ExternalProofRef == "" {
		return errors.New("external_proof_ref is required")
	}
	if options.OperatorID == "" {
		return errors.New("operator_id is required")
	}
	return nil
}

type complianceDeleteApprovalScanner interface {
	Scan(dest ...any) error
}

func scanComplianceDeleteApprovalRow(scanner complianceDeleteApprovalScanner) (MessageComplianceDeleteApprovalResult, error) {
	var row MessageComplianceDeleteApprovalResult
	if err := scanner.Scan(
		&row.TenantID,
		&row.ConversationID,
		&row.MessageID,
		&row.ApprovalID,
		&row.Status,
		&row.ExternalProofRef,
		&row.ReasonPresent,
		&row.ApprovedBy,
		&row.ApprovedAt,
		&row.ConsumedBy,
		&row.ConsumedEventID,
		&row.ConsumedAt,
		&row.CanceledBy,
		&row.CanceledAt,
		&row.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MessageComplianceDeleteApprovalResult{}, pgx.ErrNoRows
		}
		return MessageComplianceDeleteApprovalResult{}, types.NewDBWriteFailed(err.Error())
	}
	return row, nil
}
