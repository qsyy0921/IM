package postgres

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

type MessageRetentionProofAuditOptions struct {
	TenantID       string
	ConversationID string
	MessageID      string
	Status         string
	Limit          int
}

type MessageRetentionProofAuditRow struct {
	TenantID                   string
	ConversationID             string
	MessageID                  string
	ConversationSeq            int64
	SenderID                   string
	MessageType                string
	Status                     string
	CurrentPayloadPresent      bool
	CreatedAt                  time.Time
	DeletedAt                  *time.Time
	DeleteChangeVersion        *int
	DeleteChangedBy            string
	DeleteReasonPresent        bool
	DeleteBeforePayloadPresent bool
	DeleteAfterPayloadPresent  bool
	DeleteChangedAt            *time.Time
	DeleteTimelineEventPresent bool
	DeleteOutboxEventPresent   bool
}

func (r *MessageRepository) AuditMessageRetentionProof(ctx context.Context, options MessageRetentionProofAuditOptions) ([]MessageRetentionProofAuditRow, error) {
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
	clauses := make([]string, 0, 4)
	if tenantID := strings.TrimSpace(options.TenantID); tenantID != "" {
		args = append(args, tenantID)
		clauses = append(clauses, "ml.tenant_id = $"+strconv.Itoa(len(args)))
	}
	if conversationID := strings.TrimSpace(options.ConversationID); conversationID != "" {
		args = append(args, conversationID)
		clauses = append(clauses, "ml.conversation_id = $"+strconv.Itoa(len(args)))
	}
	if messageID := strings.TrimSpace(options.MessageID); messageID != "" {
		args = append(args, messageID)
		clauses = append(clauses, "ml.message_id = $"+strconv.Itoa(len(args)))
	}
	if rawStatus := strings.TrimSpace(options.Status); rawStatus != "" {
		status := normalizeMessageRetentionProofStatus(rawStatus)
		if status == "" {
			return nil, errors.New("unsupported message retention proof status")
		}
		args = append(args, status)
		clauses = append(clauses, "ml.status = $"+strconv.Itoa(len(args)))
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	args = append(args, limit)

	rows, err := r.pool.Query(ctx, `
SELECT
    ml.tenant_id,
    ml.conversation_id,
    ml.message_id,
    ml.conversation_seq,
    ml.sender_id,
    ml.message_type,
    ml.status,
    ml.payload_json IS NOT NULL,
    ml.created_at,
    ml.deleted_at,
    delete_history.change_version,
    COALESCE(delete_history.changed_by, ''),
    COALESCE(delete_history.reason, '') <> '',
    COALESCE(delete_history.before_payload_present, false),
    COALESCE(delete_history.after_payload_present, false),
    delete_history.changed_at,
    EXISTS (
        SELECT 1
        FROM conversation_timeline_events cte
        WHERE cte.tenant_id = ml.tenant_id
          AND cte.conversation_id = ml.conversation_id
          AND cte.message_id = ml.message_id
          AND cte.event_type = 'message.deleted.v1'
    ),
    EXISTS (
        SELECT 1
        FROM message_outbox mo
        WHERE mo.tenant_id = ml.tenant_id
          AND mo.conversation_id = ml.conversation_id
          AND mo.aggregate_version = ml.conversation_seq
          AND mo.event_type = 'message.deleted.v1'
    )
FROM message_log ml
LEFT JOIN LATERAL (
    SELECT
        mch.change_version,
        mch.changed_by,
        mch.reason,
        mch.before_payload_json IS NOT NULL AS before_payload_present,
        mch.after_payload_json IS NOT NULL AS after_payload_present,
        mch.changed_at
    FROM message_change_history mch
    WHERE mch.tenant_id = ml.tenant_id
      AND mch.conversation_id = ml.conversation_id
      AND mch.message_id = ml.message_id
      AND mch.change_type = 'DELETE'
    ORDER BY mch.change_version DESC
    LIMIT 1
) delete_history ON true
`+where+`
ORDER BY ml.deleted_at DESC NULLS LAST, ml.created_at DESC, ml.tenant_id, ml.conversation_id, ml.conversation_seq DESC
LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer rows.Close()

	result := make([]MessageRetentionProofAuditRow, 0, limit)
	for rows.Next() {
		var row MessageRetentionProofAuditRow
		if err := rows.Scan(
			&row.TenantID,
			&row.ConversationID,
			&row.MessageID,
			&row.ConversationSeq,
			&row.SenderID,
			&row.MessageType,
			&row.Status,
			&row.CurrentPayloadPresent,
			&row.CreatedAt,
			&row.DeletedAt,
			&row.DeleteChangeVersion,
			&row.DeleteChangedBy,
			&row.DeleteReasonPresent,
			&row.DeleteBeforePayloadPresent,
			&row.DeleteAfterPayloadPresent,
			&row.DeleteChangedAt,
			&row.DeleteTimelineEventPresent,
			&row.DeleteOutboxEventPresent,
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

func normalizeMessageRetentionProofStatus(value string) string {
	status := strings.ToUpper(strings.TrimSpace(value))
	switch status {
	case "NORMAL", "EDITED", "REVOKED", "DELETED":
		return status
	default:
		return ""
	}
}
