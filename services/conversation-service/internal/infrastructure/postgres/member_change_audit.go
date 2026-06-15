package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type MemberChangeAuditOptions struct {
	ChangeID       string
	TenantID       string
	ConversationID string
	Status         string
	OutboxEventID  string
	Limit          int
}

type MemberChangeAuditRow struct {
	ChangeID        string
	TenantID        string
	ConversationID  string
	TargetUserID    string
	OperatorUserID  string
	ChangeType      string
	Status          string
	BoundarySeq     int64
	TimelineEventID string
	OutboxEventID   string
	RetryCount      int
	LastError       string
	NextRetryAt     *time.Time
	DeadLetteredAt  *time.Time
	CompletedAt     *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (r *Repository) AuditMemberChanges(ctx context.Context, options MemberChangeAuditOptions) ([]MemberChangeAuditRow, error) {
	if r.pool == nil {
		return nil, types.NewDBReadFailed("repository is not configured")
	}
	limit := options.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	var args []any
	clauses := make([]string, 0, 5)
	if changeID := strings.TrimSpace(options.ChangeID); changeID != "" {
		args = append(args, changeID)
		clauses = append(clauses, "change_id = $"+strconv.Itoa(len(args)))
	}
	if tenantID := strings.TrimSpace(options.TenantID); tenantID != "" {
		args = append(args, tenantID)
		clauses = append(clauses, "tenant_id = $"+strconv.Itoa(len(args)))
	}
	if conversationID := strings.TrimSpace(options.ConversationID); conversationID != "" {
		args = append(args, conversationID)
		clauses = append(clauses, "conversation_id = $"+strconv.Itoa(len(args)))
	}
	if rawStatus := strings.TrimSpace(options.Status); rawStatus != "" {
		status := normalizeMemberChangeAuditStatus(rawStatus)
		if status == "" {
			return nil, errors.New("unsupported member change status")
		}
		args = append(args, status)
		clauses = append(clauses, "status = $"+strconv.Itoa(len(args)))
	}
	if outboxEventID := strings.TrimSpace(options.OutboxEventID); outboxEventID != "" {
		args = append(args, outboxEventID)
		clauses = append(clauses, "outbox_event_id = $"+strconv.Itoa(len(args)))
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	args = append(args, limit)
	rows, err := r.pool.Query(ctx, `
SELECT
    change_id,
    tenant_id,
    conversation_id,
    user_id,
    operator_id,
    change_type,
    status,
    COALESCE(boundary_seq, 0),
    COALESCE(timeline_event_id, ''),
    COALESCE(outbox_event_id, ''),
    retry_count,
    CASE WHEN COALESCE(last_error, '') = '' THEN '' ELSE 'member change processing failed' END,
    next_retry_at,
    dead_lettered_at,
    completed_at,
    created_at,
    updated_at
FROM member_change_saga
`+where+`
ORDER BY updated_at DESC, created_at DESC, change_id DESC
LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	result := make([]MemberChangeAuditRow, 0, limit)
	for rows.Next() {
		var row MemberChangeAuditRow
		var nextRetryAt sql.NullTime
		var deadLetteredAt sql.NullTime
		var completedAt sql.NullTime
		if err := rows.Scan(
			&row.ChangeID,
			&row.TenantID,
			&row.ConversationID,
			&row.TargetUserID,
			&row.OperatorUserID,
			&row.ChangeType,
			&row.Status,
			&row.BoundarySeq,
			&row.TimelineEventID,
			&row.OutboxEventID,
			&row.RetryCount,
			&row.LastError,
			&nextRetryAt,
			&deadLetteredAt,
			&completedAt,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		row.NextRetryAt = nullableTimePtr(nextRetryAt)
		row.DeadLetteredAt = nullableTimePtr(deadLetteredAt)
		row.CompletedAt = nullableTimePtr(completedAt)
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return result, nil
}

func normalizeMemberChangeAuditStatus(value string) string {
	status := strings.ToUpper(strings.TrimSpace(value))
	switch types.MemberChangeStatus(status) {
	case types.MemberChangeStatusPendingBoundary,
		types.MemberChangeStatusBoundaryAllocated,
		types.MemberChangeStatusMemberUpdated,
		types.MemberChangeStatusOutboxEnqueued,
		types.MemberChangeStatusEventPublished,
		types.MemberChangeStatusDone,
		types.MemberChangeStatusFailedCompensated:
		return status
	default:
		return ""
	}
}

func nullableTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}
