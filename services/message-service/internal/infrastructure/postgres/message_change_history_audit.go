package postgres

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

type MessageChangeHistoryAuditOptions struct {
	TenantID       string
	ConversationID string
	MessageID      string
	ChangeType     string
	ChangedBy      string
	Limit          int
}

type MessageChangeHistoryAuditRow struct {
	TenantID             string
	ConversationID       string
	MessageID            string
	ChangeVersion        int
	ChangeType           string
	BeforePayloadPresent bool
	AfterPayloadPresent  bool
	BeforeStatus         string
	AfterStatus          string
	ChangedBy            string
	ReasonPresent        bool
	TraceID              string
	ChangedAt            time.Time
}

func (r *MessageRepository) AuditMessageChangeHistory(ctx context.Context, options MessageChangeHistoryAuditOptions) ([]MessageChangeHistoryAuditRow, error) {
	if r.pool == nil {
		return nil, types.NewDBWriteFailed("message repository is not configured")
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
	if tenantID := strings.TrimSpace(options.TenantID); tenantID != "" {
		args = append(args, tenantID)
		clauses = append(clauses, "tenant_id = $"+strconv.Itoa(len(args)))
	}
	if conversationID := strings.TrimSpace(options.ConversationID); conversationID != "" {
		args = append(args, conversationID)
		clauses = append(clauses, "conversation_id = $"+strconv.Itoa(len(args)))
	}
	if messageID := strings.TrimSpace(options.MessageID); messageID != "" {
		args = append(args, messageID)
		clauses = append(clauses, "message_id = $"+strconv.Itoa(len(args)))
	}
	if rawChangeType := strings.TrimSpace(options.ChangeType); rawChangeType != "" {
		changeType := normalizeMessageChangeHistoryAuditType(rawChangeType)
		if changeType == "" {
			return nil, errors.New("unsupported message change history type")
		}
		args = append(args, changeType)
		clauses = append(clauses, "change_type = $"+strconv.Itoa(len(args)))
	}
	if changedBy := strings.TrimSpace(options.ChangedBy); changedBy != "" {
		args = append(args, changedBy)
		clauses = append(clauses, "changed_by = $"+strconv.Itoa(len(args)))
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	args = append(args, limit)

	rows, err := r.pool.Query(ctx, `
SELECT
    tenant_id,
    conversation_id,
    message_id,
    change_version,
    change_type,
    before_payload_json IS NOT NULL,
    after_payload_json IS NOT NULL,
    before_status,
    after_status,
    changed_by,
    COALESCE(reason, '') <> '',
    trace_id,
    changed_at
FROM message_change_history
`+where+`
ORDER BY changed_at DESC, tenant_id, conversation_id, message_id, change_version DESC
LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer rows.Close()

	result := make([]MessageChangeHistoryAuditRow, 0, limit)
	for rows.Next() {
		var row MessageChangeHistoryAuditRow
		if err := rows.Scan(
			&row.TenantID,
			&row.ConversationID,
			&row.MessageID,
			&row.ChangeVersion,
			&row.ChangeType,
			&row.BeforePayloadPresent,
			&row.AfterPayloadPresent,
			&row.BeforeStatus,
			&row.AfterStatus,
			&row.ChangedBy,
			&row.ReasonPresent,
			&row.TraceID,
			&row.ChangedAt,
		); err != nil {
			return nil, types.NewDBWriteFailed(err.Error())
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func normalizeMessageChangeHistoryAuditType(value string) string {
	changeType := strings.ToUpper(strings.TrimSpace(value))
	switch changeType {
	case "EDIT", "REVOKE", "DELETE":
		return changeType
	default:
		return ""
	}
}
