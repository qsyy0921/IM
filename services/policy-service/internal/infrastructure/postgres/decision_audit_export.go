package postgres

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type DecisionAuditExportOptions struct {
	EventID        string
	TenantID       string
	Action         string
	Allowed        *bool
	Classification string
	ReasonCode     string
	DecisionSource string
	Status         string
	CreatedAfter   *time.Time
	CreatedBefore  *time.Time
	Limit          int
}

type DecisionAuditExportRow struct {
	EventID                  string
	TenantID                 string
	ActorUserKey             string
	DeviceKey                string
	ConversationKey          string
	MessageKey               string
	Action                   string
	MessageIDPresent         bool
	DirectPeerContextPresent bool
	DirectPeerKey            string
	Allowed                  bool
	PermissionVersion        int64
	Classification           string
	ReasonCode               string
	DecisionSource           string
	Status                   string
	EventType                string
	EventVersion             string
	Producer                 string
	PartitionKey             string
	CorrelationID            string
	TraceID                  string
	CreatedAt                time.Time
	PublishedAt              *time.Time
}

func (store *OutboxStore) ExportDecisionAudit(ctx context.Context, options DecisionAuditExportOptions) ([]DecisionAuditExportRow, error) {
	if store == nil || store.pool == nil {
		return nil, errors.New("policy decision audit export store is not configured")
	}
	limit := options.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	var args []any
	clauses := make([]string, 0, 7)
	if eventID := strings.TrimSpace(options.EventID); eventID != "" {
		args = append(args, eventID)
		clauses = append(clauses, "event_id = $"+strconv.Itoa(len(args)))
	}
	if tenantID := strings.TrimSpace(options.TenantID); tenantID != "" {
		args = append(args, tenantID)
		clauses = append(clauses, "tenant_id = $"+strconv.Itoa(len(args)))
	}
	if action := normalizePolicyMessageAction(options.Action); action != "" {
		args = append(args, action)
		clauses = append(clauses, "action = $"+strconv.Itoa(len(args)))
	} else if strings.TrimSpace(options.Action) != "" {
		return nil, types.NewInvalidArgument("unsupported policy decision audit action")
	}
	if options.Allowed != nil {
		args = append(args, *options.Allowed)
		clauses = append(clauses, "allowed = $"+strconv.Itoa(len(args)))
	}
	if classification := strings.TrimSpace(options.Classification); classification != "" {
		args = append(args, classification)
		clauses = append(clauses, "classification = $"+strconv.Itoa(len(args)))
	}
	if reasonCode := strings.TrimSpace(options.ReasonCode); reasonCode != "" {
		args = append(args, reasonCode)
		clauses = append(clauses, "reason_code = $"+strconv.Itoa(len(args)))
	}
	if decisionSource := strings.TrimSpace(options.DecisionSource); decisionSource != "" {
		args = append(args, decisionSource)
		clauses = append(clauses, "decision_source = $"+strconv.Itoa(len(args)))
	}
	if rawStatus := strings.TrimSpace(options.Status); rawStatus != "" {
		status := normalizePolicyOutboxStatus(rawStatus)
		if status == "" {
			return nil, types.NewInvalidArgument("unsupported policy outbox status")
		}
		args = append(args, status)
		clauses = append(clauses, "status = $"+strconv.Itoa(len(args)))
	}
	if options.CreatedAfter != nil {
		args = append(args, options.CreatedAfter.UTC())
		clauses = append(clauses, "created_at >= $"+strconv.Itoa(len(args)))
	}
	if options.CreatedBefore != nil {
		args = append(args, options.CreatedBefore.UTC())
		clauses = append(clauses, "created_at < $"+strconv.Itoa(len(args)))
	}
	if options.CreatedAfter != nil && options.CreatedBefore != nil && !options.CreatedAfter.Before(*options.CreatedBefore) {
		return nil, types.NewInvalidArgument("created_after must be before created_before")
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	args = append(args, limit)
	rows, err := store.pool.Query(ctx, `
SELECT
    event_id,
    tenant_id,
    actor_user_key,
    device_key,
    conversation_key,
    message_key,
    action,
    message_id_present,
    direct_peer_context_present,
    direct_peer_key,
    allowed,
    permission_version,
    classification,
    reason_code,
    decision_source,
    status,
    event_type,
    event_version,
    producer,
    partition_key,
    correlation_id,
    trace_id,
    created_at,
    published_at
FROM policy_decision_audit_outbox
`+where+`
ORDER BY created_at DESC, id DESC
LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer rows.Close()

	result := make([]DecisionAuditExportRow, 0, limit)
	for rows.Next() {
		var row DecisionAuditExportRow
		if err := rows.Scan(
			&row.EventID,
			&row.TenantID,
			&row.ActorUserKey,
			&row.DeviceKey,
			&row.ConversationKey,
			&row.MessageKey,
			&row.Action,
			&row.MessageIDPresent,
			&row.DirectPeerContextPresent,
			&row.DirectPeerKey,
			&row.Allowed,
			&row.PermissionVersion,
			&row.Classification,
			&row.ReasonCode,
			&row.DecisionSource,
			&row.Status,
			&row.EventType,
			&row.EventVersion,
			&row.Producer,
			&row.PartitionKey,
			&row.CorrelationID,
			&row.TraceID,
			&row.CreatedAt,
			&row.PublishedAt,
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

func normalizePolicyMessageAction(action string) string {
	switch strings.ToUpper(strings.TrimSpace(action)) {
	case string(types.MessageActionSend):
		return string(types.MessageActionSend)
	case string(types.MessageActionEdit):
		return string(types.MessageActionEdit)
	case string(types.MessageActionRevoke):
		return string(types.MessageActionRevoke)
	case string(types.MessageActionDelete):
		return string(types.MessageActionDelete)
	default:
		return ""
	}
}
