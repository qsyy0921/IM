package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/policy-service/internal/domain"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

func TestToolPolicyEvaluatorUsesPostgresRuleIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewToolPolicyEvaluator(pool, domain.StaticToolPolicy{
		Allowed:           false,
		PermissionVersion: 1,
		Classification:    "TOOL_STATIC_DENY",
		Reason:            "static deny",
	})
	command := testToolPolicyCommand()
	seedToolPolicyRule(t, ctx, pool, command.AuthContext.TenantID, command.ToolName, command.Action, command.ResourceType, string(command.RiskLevel), true, true, 42, "TOOL_APPROVAL_REQUIRED", "operator approval required", 10)

	decision, err := evaluator.DecideToolAction(ctx, command)
	if err != nil {
		t.Fatalf("decide tool action: %v", err)
	}
	if !decision.Allowed ||
		!decision.RequiresApproval ||
		decision.PermissionVersion != 42 ||
		decision.Classification != "TOOL_APPROVAL_REQUIRED" ||
		decision.DecisionSource != types.PolicyDecisionSourceToolRule {
		t.Fatalf("expected postgres tool rule, got %+v", decision)
	}
}

func TestToolPolicyEvaluatorSpecificRuleOverridesWildcardIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewToolPolicyEvaluator(pool, domain.StaticToolPolicy{
		Allowed:           false,
		PermissionVersion: 1,
		Classification:    "TOOL_STATIC_DENY",
	})
	command := testToolPolicyCommand()
	seedToolPolicyRule(t, ctx, pool, command.AuthContext.TenantID, "*", command.Action, "*", "ANY", false, false, 11, "TOOL_WILDCARD_DENY", "tool denied", 1)
	seedToolPolicyRule(t, ctx, pool, command.AuthContext.TenantID, command.ToolName, command.Action, command.ResourceType, string(command.RiskLevel), true, false, 12, "TOOL_EXACT_ALLOW", "", 100)

	decision, err := evaluator.DecideToolAction(ctx, command)
	if err != nil {
		t.Fatalf("decide tool action: %v", err)
	}
	if !decision.Allowed || decision.PermissionVersion != 12 || decision.Classification != "TOOL_EXACT_ALLOW" {
		t.Fatalf("expected exact tool rule to override wildcard, got %+v", decision)
	}
}

func TestToolPolicyEvaluatorFallsBackWhenNoRuleIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewToolPolicyEvaluator(pool, domain.StaticToolPolicy{
		Allowed:           false,
		PermissionVersion: 9,
		Classification:    "TOOL_STATIC_DENY",
		Reason:            "tool policy denied",
	})

	decision, err := evaluator.DecideToolAction(ctx, testToolPolicyCommand())
	if err != nil {
		t.Fatalf("decide tool action: %v", err)
	}
	if decision.Allowed || decision.PermissionVersion != 9 || decision.Classification != "TOOL_STATIC_DENY" || decision.DecisionSource != types.PolicyDecisionSourceFallback {
		t.Fatalf("expected static fallback deny, got %+v", decision)
	}
}

func TestToolDecisionAuditRecordsLowSensitiveFieldsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	clock := time.Date(2026, 6, 18, 1, 2, 3, 0, time.UTC)
	audit := NewToolDecisionAudit(
		pool,
		WithToolDecisionAuditEventID(func() (string, error) { return "tool-audit-event-1", nil }),
		WithToolDecisionAuditClock(func() time.Time { return clock }),
	)
	command := testToolPolicyCommand()
	command.AuthContext.TraceID = "trace-tool"
	command.AuthContext.RequestID = "request-tool"
	decision := types.ToolActionDecision{
		TenantID:          command.AuthContext.TenantID,
		UserID:            command.AuthContext.UserID,
		ToolName:          command.ToolName,
		Action:            command.Action,
		ResourceType:      command.ResourceType,
		ResourceID:        command.ResourceID,
		RiskLevel:         command.RiskLevel,
		Allowed:           true,
		RequiresApproval:  true,
		PermissionVersion: 42,
		Classification:    "TOOL_APPROVAL_REQUIRED",
		Reason:            "operator approval required",
		DecisionSource:    types.PolicyDecisionSourceToolRule,
	}

	if err := audit.RecordToolDecision(ctx, command, decision); err != nil {
		t.Fatalf("record tool decision: %v", err)
	}

	var actorUserKey string
	var resourceIDPresent bool
	var intentColumnExists int
	var classification string
	err := pool.QueryRow(ctx, `
SELECT actor_user_key, resource_id_present, classification,
    (
        SELECT COUNT(*)
        FROM information_schema.columns
        WHERE table_name = 'policy_tool_decision_audit'
          AND column_name = 'intent'
    )
FROM policy_tool_decision_audit
WHERE event_id = 'tool-audit-event-1'
`).Scan(&actorUserKey, &resourceIDPresent, &classification, &intentColumnExists)
	if err != nil {
		t.Fatalf("query tool decision audit: %v", err)
	}
	if actorUserKey == "" || actorUserKey == string(command.AuthContext.UserID) {
		t.Fatalf("expected hashed actor key, got %q", actorUserKey)
	}
	if !resourceIDPresent || intentColumnExists != 0 || classification != "TOOL_APPROVAL_REQUIRED" {
		t.Fatalf("unexpected audit row: resource_present=%t intent_column=%d classification=%s", resourceIDPresent, intentColumnExists, classification)
	}
}

func testToolPolicyCommand() types.CheckToolActionCommand {
	return types.CheckToolActionCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-policy",
			UserID:   "user-policy",
			DeviceID: "device-policy",
		},
		ToolName:     "conversation.owner_transfer",
		Action:       types.ToolActionExecute,
		ResourceType: "conversation",
		ResourceID:   "conv-policy",
		RiskLevel:    types.ToolRiskLevelHigh,
		Intent:       "transfer owner",
	}
}

func seedToolPolicyRule(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	tenantID types.TenantID,
	toolName string,
	action types.ToolAction,
	resourceType string,
	riskLevel string,
	allowed bool,
	requiresApproval bool,
	permissionVersion int64,
	classification string,
	reason string,
	priority int,
) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO policy_tool_action_rules (
    tenant_id,
    tool_name,
    action,
    resource_type,
    risk_level,
    allowed,
    requires_approval,
    permission_version,
    classification,
    reason,
    priority
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
`, tenantID, toolName, action, resourceType, riskLevel, allowed, requiresApproval, permissionVersion, classification, reason, priority)
	if err != nil {
		t.Fatalf("seed tool policy rule: %v", err)
	}
}
