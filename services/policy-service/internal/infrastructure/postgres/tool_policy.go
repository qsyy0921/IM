package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type fallbackToolEvaluator interface {
	DecideToolAction(context.Context, types.CheckToolActionCommand) (types.ToolActionDecision, error)
}

type ToolPolicyEvaluator struct {
	pool     *pgxpool.Pool
	fallback fallbackToolEvaluator
}

func NewToolPolicyEvaluator(pool *pgxpool.Pool, fallback fallbackToolEvaluator) ToolPolicyEvaluator {
	return ToolPolicyEvaluator{pool: pool, fallback: fallback}
}

func (e ToolPolicyEvaluator) DecideToolAction(
	ctx context.Context,
	command types.CheckToolActionCommand,
) (types.ToolActionDecision, error) {
	if e.pool == nil {
		return e.fallbackDecision(ctx, command)
	}
	decision := types.ToolActionDecision{
		TenantID:       command.AuthContext.TenantID,
		UserID:         command.AuthContext.UserID,
		ToolName:       strings.TrimSpace(command.ToolName),
		Action:         command.Action,
		ResourceType:   strings.TrimSpace(command.ResourceType),
		ResourceID:     strings.TrimSpace(command.ResourceID),
		RiskLevel:      normalizeToolRisk(command.RiskLevel),
		DecisionSource: types.PolicyDecisionSourceToolRule,
	}
	err := e.pool.QueryRow(ctx, `
SELECT allowed, requires_approval, permission_version, classification, reason
FROM policy_tool_action_rules
WHERE tenant_id = $1
  AND enabled = true
  AND (tool_name = $2 OR tool_name = '*')
  AND action = $3
  AND (resource_type = $4 OR resource_type = '*')
  AND (risk_level = $5 OR risk_level = 'ANY')
ORDER BY
  CASE WHEN tool_name = $2 THEN 0 ELSE 1 END,
  CASE WHEN resource_type = $4 THEN 0 ELSE 1 END,
  CASE WHEN risk_level = $5 THEN 0 ELSE 1 END,
  priority ASC,
  updated_at ASC
LIMIT 1
`, command.AuthContext.TenantID, strings.TrimSpace(command.ToolName), command.Action, strings.TrimSpace(command.ResourceType), normalizeToolRisk(command.RiskLevel)).Scan(
		&decision.Allowed,
		&decision.RequiresApproval,
		&decision.PermissionVersion,
		&decision.Classification,
		&decision.Reason,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return e.fallbackDecision(ctx, command)
	}
	if isUndefinedTable(err) {
		return e.fallbackDecision(ctx, command)
	}
	if err != nil {
		return types.ToolActionDecision{}, types.NewDependencyUnavailable("tool policy rule lookup failed")
	}
	decision.Classification = strings.TrimSpace(decision.Classification)
	decision.Reason = strings.TrimSpace(decision.Reason)
	if decision.PermissionVersion <= 0 || decision.Classification == "" {
		return types.ToolActionDecision{}, types.NewDependencyUnavailable("tool policy rule is invalid")
	}
	if !decision.Allowed && decision.Reason == "" {
		decision.Reason = "tool policy denied"
	}
	if decision.RequiresApproval && decision.Reason == "" {
		decision.Reason = "tool action requires approval"
	}
	return decision, nil
}

func (e ToolPolicyEvaluator) fallbackDecision(
	ctx context.Context,
	command types.CheckToolActionCommand,
) (types.ToolActionDecision, error) {
	if e.fallback == nil {
		return types.ToolActionDecision{}, types.NewDependencyUnavailable("tool policy fallback is not configured")
	}
	return e.fallback.DecideToolAction(ctx, command)
}

type ToolDecisionAudit struct {
	pool    *pgxpool.Pool
	eventID func() (string, error)
	now     func() time.Time
}

type ToolDecisionAuditOption func(*ToolDecisionAudit)

func NewToolDecisionAudit(pool *pgxpool.Pool, opts ...ToolDecisionAuditOption) *ToolDecisionAudit {
	audit := &ToolDecisionAudit{
		pool:    pool,
		eventID: newPolicyAuditEventID,
		now:     func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(audit)
	}
	return audit
}

func WithToolDecisionAuditEventID(fn func() (string, error)) ToolDecisionAuditOption {
	return func(audit *ToolDecisionAudit) {
		if fn != nil {
			audit.eventID = fn
		}
	}
}

func WithToolDecisionAuditClock(clock func() time.Time) ToolDecisionAuditOption {
	return func(audit *ToolDecisionAudit) {
		if clock != nil {
			audit.now = clock
		}
	}
}

func (audit *ToolDecisionAudit) RecordToolDecision(
	ctx context.Context,
	command types.CheckToolActionCommand,
	decision types.ToolActionDecision,
) error {
	if audit == nil || audit.pool == nil {
		return types.NewDependencyUnavailable("tool decision audit is not configured")
	}
	if decision.PermissionVersion <= 0 || strings.TrimSpace(decision.Classification) == "" {
		return types.NewDependencyUnavailable("tool decision audit payload is invalid")
	}
	eventID, err := audit.eventID()
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	actorUserKey := policyAuditStableKey(string(decision.TenantID), "user", string(decision.UserID))
	deviceKey := policyAuditStableKey(string(decision.TenantID), "device", string(command.AuthContext.DeviceID))
	reasonCode := ""
	if !decision.Allowed || decision.RequiresApproval {
		reasonCode = truncateAuditField(strings.TrimSpace(decision.Classification), 128)
	}
	if reasonCode == "" && !decision.Allowed {
		reasonCode = "TOOL_POLICY_DENIED"
	}
	_, err = audit.pool.Exec(ctx, `
INSERT INTO policy_tool_decision_audit (
    event_id,
    tenant_id,
    actor_user_key,
    device_key,
    tool_name,
    action,
    resource_type,
    resource_id_present,
    risk_level,
    allowed,
    requires_approval,
    permission_version,
    classification,
    reason_code,
    decision_source,
    trace_id,
    request_id,
    created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
`, eventID,
		decision.TenantID,
		actorUserKey,
		deviceKey,
		truncateAuditField(strings.TrimSpace(decision.ToolName), 128),
		decision.Action,
		truncateAuditField(strings.TrimSpace(decision.ResourceType), 64),
		strings.TrimSpace(decision.ResourceID) != "",
		normalizeToolRisk(decision.RiskLevel),
		decision.Allowed,
		decision.RequiresApproval,
		decision.PermissionVersion,
		truncateAuditField(strings.TrimSpace(decision.Classification), 128),
		reasonCode,
		truncateAuditField(policyToolDecisionSource(decision), 128),
		truncateAuditField(strings.TrimSpace(command.AuthContext.TraceID), 128),
		truncateAuditField(strings.TrimSpace(command.AuthContext.RequestID), 128),
		audit.now(),
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func policyToolDecisionSource(decision types.ToolActionDecision) string {
	source := strings.TrimSpace(string(decision.DecisionSource))
	if source == "" {
		return "UNSPECIFIED"
	}
	return source
}

func normalizeToolRisk(risk types.ToolRiskLevel) types.ToolRiskLevel {
	if risk == "" {
		return types.ToolRiskLevelLow
	}
	return risk
}
