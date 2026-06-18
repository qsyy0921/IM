package postgres

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type ReBACRelationRuleStore struct {
	pool *pgxpool.Pool
}

type ReBACRelationRuleAuditOptions struct {
	TenantID          string
	Action            string
	RelationType      string
	ConversationScope string
	Enabled           *bool
	Limit             int
}

type ReBACRelationRuleSetOptions struct {
	TenantID          string
	Action            string
	RelationType      string
	ConversationScope string
	PermissionVersion int64
	Classification    string
	Reason            string
	Priority          int
	Enabled           bool
	Source            string
}

type ReBACRelationRuleRow struct {
	TenantID          string
	Action            string
	RelationType      string
	ConversationScope string
	PermissionVersion int64
	Classification    string
	Reason            string
	Priority          int
	Enabled           bool
	Source            string
	UpdatedAt         time.Time
}

func NewReBACRelationRuleStore(pool *pgxpool.Pool) *ReBACRelationRuleStore {
	return &ReBACRelationRuleStore{pool: pool}
}

func (store *ReBACRelationRuleStore) AuditReBACRelationRules(ctx context.Context, options ReBACRelationRuleAuditOptions) ([]ReBACRelationRuleRow, error) {
	if store == nil || store.pool == nil {
		return nil, errors.New("policy rebac relation rule store is not configured")
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
	if action := normalizePolicyMessageAction(options.Action); action != "" {
		args = append(args, action)
		clauses = append(clauses, "action = $"+strconv.Itoa(len(args)))
	} else if strings.TrimSpace(options.Action) != "" {
		return nil, types.NewInvalidArgument("unsupported policy rebac relation action")
	}
	if relationType := normalizeReBACRelationType(options.RelationType); relationType != "" {
		args = append(args, relationType)
		clauses = append(clauses, "relation_type = $"+strconv.Itoa(len(args)))
	} else if strings.TrimSpace(options.RelationType) != "" {
		return nil, types.NewInvalidArgument("unsupported policy rebac relation type")
	}
	if scope := normalizeReBACConversationScope(options.ConversationScope); scope != "" {
		args = append(args, scope)
		clauses = append(clauses, "conversation_scope = $"+strconv.Itoa(len(args)))
	} else if strings.TrimSpace(options.ConversationScope) != "" {
		return nil, types.NewInvalidArgument("unsupported policy rebac conversation scope")
	}
	if options.Enabled != nil {
		args = append(args, *options.Enabled)
		clauses = append(clauses, "enabled = $"+strconv.Itoa(len(args)))
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	args = append(args, limit)
	rows, err := store.pool.Query(ctx, `
SELECT
    tenant_id,
    action,
    relation_type,
    conversation_scope,
    permission_version,
    classification,
    reason,
    priority,
    enabled,
    source,
    updated_at
FROM policy_rebac_message_action_rules
`+where+`
ORDER BY updated_at DESC, tenant_id, action, priority, relation_type, conversation_scope
LIMIT $`+strconv.Itoa(len(args)), args...)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if isUndefinedTable(err) {
		return nil, nil
	}
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer rows.Close()

	result := make([]ReBACRelationRuleRow, 0, limit)
	for rows.Next() {
		var row ReBACRelationRuleRow
		if err := rows.Scan(
			&row.TenantID,
			&row.Action,
			&row.RelationType,
			&row.ConversationScope,
			&row.PermissionVersion,
			&row.Classification,
			&row.Reason,
			&row.Priority,
			&row.Enabled,
			&row.Source,
			&row.UpdatedAt,
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

func (store *ReBACRelationRuleStore) SetReBACRelationRule(ctx context.Context, options ReBACRelationRuleSetOptions) (ReBACRelationRuleRow, error) {
	if store == nil || store.pool == nil {
		return ReBACRelationRuleRow{}, errors.New("policy rebac relation rule store is not configured")
	}
	tenantID := strings.TrimSpace(options.TenantID)
	action := normalizePolicyMessageAction(options.Action)
	relationType := normalizeReBACRelationType(options.RelationType)
	scope := normalizeReBACConversationScope(options.ConversationScope)
	classification := strings.TrimSpace(options.Classification)
	source := strings.TrimSpace(options.Source)
	if source == "" {
		source = "manual"
	}
	if tenantID == "" {
		return ReBACRelationRuleRow{}, types.NewInvalidArgument("tenant_id is required")
	}
	if action == "" {
		return ReBACRelationRuleRow{}, types.NewInvalidArgument("unsupported policy rebac relation action")
	}
	if relationType == "" {
		return ReBACRelationRuleRow{}, types.NewInvalidArgument("unsupported policy rebac relation type")
	}
	if scope == "" {
		return ReBACRelationRuleRow{}, types.NewInvalidArgument("unsupported policy rebac conversation scope")
	}
	if options.PermissionVersion <= 0 {
		return ReBACRelationRuleRow{}, types.NewInvalidArgument("permission_version must be positive")
	}
	if classification == "" {
		return ReBACRelationRuleRow{}, types.NewInvalidArgument("classification is required")
	}
	if options.Priority < 0 {
		return ReBACRelationRuleRow{}, types.NewInvalidArgument("priority must be non-negative")
	}
	if source == "" {
		return ReBACRelationRuleRow{}, types.NewInvalidArgument("source is required")
	}

	var row ReBACRelationRuleRow
	err := store.pool.QueryRow(ctx, `
INSERT INTO policy_rebac_message_action_rules (
    tenant_id,
    action,
    relation_type,
    conversation_scope,
    permission_version,
    classification,
    reason,
    priority,
    enabled,
    source,
    updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
ON CONFLICT (tenant_id, action, relation_type, conversation_scope) DO UPDATE
SET permission_version = EXCLUDED.permission_version,
    classification = EXCLUDED.classification,
    reason = EXCLUDED.reason,
    priority = EXCLUDED.priority,
    enabled = EXCLUDED.enabled,
    source = EXCLUDED.source,
    updated_at = now()
RETURNING tenant_id, action, relation_type, conversation_scope, permission_version, classification, reason, priority, enabled, source, updated_at
`,
		tenantID,
		action,
		relationType,
		scope,
		options.PermissionVersion,
		classification,
		strings.TrimSpace(options.Reason),
		options.Priority,
		options.Enabled,
		source,
	).Scan(
		&row.TenantID,
		&row.Action,
		&row.RelationType,
		&row.ConversationScope,
		&row.PermissionVersion,
		&row.Classification,
		&row.Reason,
		&row.Priority,
		&row.Enabled,
		&row.Source,
		&row.UpdatedAt,
	)
	if err != nil {
		return ReBACRelationRuleRow{}, types.NewDBWriteFailed(err.Error())
	}
	return row, nil
}

func normalizeReBACRelationType(relationType string) string {
	switch strings.ToUpper(strings.TrimSpace(relationType)) {
	case string(types.ReBACRelationDirectContactActive):
		return string(types.ReBACRelationDirectContactActive)
	case string(types.ReBACRelationConversationMemberActive):
		return string(types.ReBACRelationConversationMemberActive)
	default:
		return ""
	}
}

func normalizeReBACConversationScope(scope string) string {
	switch strings.ToUpper(strings.TrimSpace(scope)) {
	case string(types.ReBACConversationScopeAny):
		return string(types.ReBACConversationScopeAny)
	case string(types.ReBACConversationScopeDirect):
		return string(types.ReBACConversationScopeDirect)
	case string(types.ReBACConversationScopeGroup):
		return string(types.ReBACConversationScopeGroup)
	default:
		return ""
	}
}
