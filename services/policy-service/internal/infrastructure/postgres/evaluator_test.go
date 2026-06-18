package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/policy-service/internal/domain"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

func TestMessagePolicyEvaluatorUsesPostgresRuleIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           false,
		PermissionVersion: 1,
		Classification:    "STATIC_DENY",
		Reason:            "static deny",
	})
	command := testPolicyCommand(types.MessageActionSend)
	seedPolicyRule(t, ctx, pool, command, true, 42, "PG_ALLOW", "")

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if !decision.Allowed || decision.PermissionVersion != 42 || decision.Classification != "PG_ALLOW" {
		t.Fatalf("expected postgres allow rule, got %+v", decision)
	}
	assertPolicyDecisionSource(t, decision, types.PolicyDecisionSourceExactRule)
	if decision.TenantID != command.AuthContext.TenantID ||
		decision.UserID != command.AuthContext.UserID ||
		decision.ConversationID != command.ConversationID ||
		decision.Action != command.Action {
		t.Fatalf("decision did not echo command identity: %+v", decision)
	}
}

func TestMessagePolicyEvaluatorUsesPostgresDenyRuleIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 1,
		Classification:    "STATIC_ALLOW",
	})
	command := testPolicyCommand(types.MessageActionDelete)
	command.MessageID = "msg-policy-delete"
	seedPolicyRule(t, ctx, pool, command, false, 77, "PG_DENY", "owner only")

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if decision.Allowed || decision.PermissionVersion != 77 || decision.Classification != "PG_DENY" || decision.Reason != "owner only" {
		t.Fatalf("expected postgres deny rule, got %+v", decision)
	}
}

func TestMessagePolicyEvaluatorFallsBackWhenNoRuleIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 9,
		Classification:    "STATIC_ALLOW",
	})

	decision, err := evaluator.DecideMessageAction(ctx, testPolicyCommand(types.MessageActionSend))
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if !decision.Allowed || decision.PermissionVersion != 9 || decision.Classification != "STATIC_ALLOW" {
		t.Fatalf("expected static fallback decision, got %+v", decision)
	}
	assertPolicyDecisionSource(t, decision, types.PolicyDecisionSourceFallback)
}

func TestMessagePolicyEvaluatorUsesTenantRuleIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           false,
		PermissionVersion: 1,
		Classification:    "STATIC_DENY",
		Reason:            "static deny",
	})
	command := testPolicyCommand(types.MessageActionSend)
	seedTenantPolicyRule(t, ctx, pool, command.AuthContext.TenantID, command.Action, true, 88, "TENANT_ALLOW", "")

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if !decision.Allowed ||
		decision.PermissionVersion != 88 ||
		decision.Classification != "TENANT_ALLOW" ||
		decision.TenantID != command.AuthContext.TenantID ||
		decision.UserID != command.AuthContext.UserID ||
		decision.ConversationID != command.ConversationID ||
		decision.Action != command.Action {
		t.Fatalf("expected tenant allow rule, got %+v", decision)
	}
	assertPolicyDecisionSource(t, decision, types.PolicyDecisionSourceTenantRule)
}

func TestMessagePolicyEvaluatorUsesTenantDenyDefaultReasonIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 1,
		Classification:    "STATIC_ALLOW",
	})
	command := testPolicyCommand(types.MessageActionDelete)
	seedTenantPolicyRule(t, ctx, pool, command.AuthContext.TenantID, command.Action, false, 89, "TENANT_DENY", "")

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if decision.Allowed || decision.PermissionVersion != 89 || decision.Classification != "TENANT_DENY" || decision.Reason != "policy denied" {
		t.Fatalf("expected tenant deny default reason, got %+v", decision)
	}
}

func TestMessagePolicyEvaluatorExactRuleOverridesTenantRuleIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 1,
		Classification:    "STATIC_ALLOW",
	})
	command := testPolicyCommand(types.MessageActionSend)
	seedTenantPolicyRule(t, ctx, pool, command.AuthContext.TenantID, command.Action, false, 88, "TENANT_DENY", "tenant default deny")
	seedPolicyRule(t, ctx, pool, command, true, 99, "EXACT_ALLOW", "")

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if !decision.Allowed || decision.PermissionVersion != 99 || decision.Classification != "EXACT_ALLOW" || decision.Reason != "" {
		t.Fatalf("expected exact rule to override tenant rule, got %+v", decision)
	}
}

func TestMessagePolicyEvaluatorExactDenyOverridesTenantAllowIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 1,
		Classification:    "STATIC_ALLOW",
	})
	command := testPolicyCommand(types.MessageActionRevoke)
	seedTenantPolicyRule(t, ctx, pool, command.AuthContext.TenantID, command.Action, true, 88, "TENANT_ALLOW", "")
	seedPolicyRule(t, ctx, pool, command, false, 99, "EXACT_DENY", "moderator only")

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if decision.Allowed || decision.PermissionVersion != 99 || decision.Classification != "EXACT_DENY" || decision.Reason != "moderator only" {
		t.Fatalf("expected exact deny to override tenant allow, got %+v", decision)
	}
}

func TestMessagePolicyEvaluatorUserRestrictionOverridesExactAllowIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 1,
		Classification:    "STATIC_ALLOW",
	})
	command := testPolicyCommand(types.MessageActionSend)
	seedTenantPolicyRule(t, ctx, pool, command.AuthContext.TenantID, command.Action, true, 88, "TENANT_ALLOW", "")
	seedPolicyRule(t, ctx, pool, command, true, 99, "EXACT_ALLOW", "")
	seedUserRestriction(t, ctx, pool, command, 123, "USER_MUTED", "user muted")

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if decision.Allowed ||
		decision.PermissionVersion != 123 ||
		decision.Classification != "USER_MUTED" ||
		decision.Reason != "user muted" {
		t.Fatalf("expected user restriction to override exact allow, got %+v", decision)
	}
	assertPolicyDecisionSource(t, decision, types.PolicyDecisionSourceUserRestriction)
}

func TestMessagePolicyEvaluatorTenantQuotaDeniesBeforeExactAllowIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 1,
		Classification:    "STATIC_ALLOW",
	})
	command := testPolicyCommand(types.MessageActionSend)
	seedTenantActionQuota(t, ctx, pool, command.AuthContext.TenantID, command.Action, 2, 3600, 301, "TENANT_SEND_QUOTA", "")
	seedPolicyRule(t, ctx, pool, command, true, 99, "EXACT_ALLOW", "")
	seedPolicyDecisionAuditRow(t, ctx, pool, "quota-audit-1", command.AuthContext.TenantID, command.Action, true, time.Now().UTC().Add(-2*time.Minute))
	seedPolicyDecisionAuditRow(t, ctx, pool, "quota-audit-2", command.AuthContext.TenantID, command.Action, true, time.Now().UTC().Add(-time.Minute))

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if decision.Allowed ||
		decision.PermissionVersion != 301 ||
		decision.Classification != "TENANT_SEND_QUOTA" ||
		decision.Reason != "tenant quota exceeded" {
		t.Fatalf("expected tenant quota deny before exact allow, got %+v", decision)
	}
	assertPolicyDecisionSource(t, decision, types.PolicyDecisionSourceTenantQuota)
}

func TestMessagePolicyEvaluatorTenantQuotaAllowsBelowLimitIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           false,
		PermissionVersion: 1,
		Classification:    "STATIC_DENY",
	})
	command := testPolicyCommand(types.MessageActionEdit)
	command.MessageID = "msg-policy-edit"
	seedTenantActionQuota(t, ctx, pool, command.AuthContext.TenantID, command.Action, 2, 3600, 302, "TENANT_EDIT_QUOTA", "tenant edit quota exceeded")
	seedTenantPolicyRule(t, ctx, pool, command.AuthContext.TenantID, command.Action, true, 88, "TENANT_ALLOW", "")
	seedPolicyDecisionAuditRow(t, ctx, pool, "quota-edit-allowed", command.AuthContext.TenantID, command.Action, true, time.Now().UTC().Add(-time.Minute))
	seedPolicyDecisionAuditRow(t, ctx, pool, "quota-edit-denied", command.AuthContext.TenantID, command.Action, false, time.Now().UTC().Add(-30*time.Second))
	seedPolicyDecisionAuditRow(t, ctx, pool, "quota-edit-expired", command.AuthContext.TenantID, command.Action, true, time.Now().UTC().Add(-2*time.Hour))

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if !decision.Allowed ||
		decision.PermissionVersion != 88 ||
		decision.Classification != "TENANT_ALLOW" {
		t.Fatalf("expected quota below limit to fall through to tenant allow, got %+v", decision)
	}
}

func TestMessagePolicyEvaluatorReBACDirectContactActiveDeniesBeforeExactAllowIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 1,
		Classification:    "STATIC_ALLOW",
	})
	command := testPolicyCommand(types.MessageActionSend)
	command.DirectPeerUserID = "peer-policy"
	seedReBACRelationRule(
		t,
		ctx,
		pool,
		command.AuthContext.TenantID,
		command.Action,
		types.ReBACRelationDirectContactActive,
		types.ReBACConversationScopeDirect,
		401,
		"DIRECT_CONTACT_REQUIRED",
		"direct contact required",
	)
	seedPolicyRule(t, ctx, pool, command, true, 99, "EXACT_ALLOW", "")

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if decision.Allowed ||
		decision.PermissionVersion != 401 ||
		decision.Classification != "DIRECT_CONTACT_REQUIRED" ||
		decision.Reason != "direct contact required" {
		t.Fatalf("expected rebac direct-contact deny before exact allow, got %+v", decision)
	}
	assertPolicyDecisionSource(t, decision, types.PolicyDecisionSourceReBACRelation)
}

func TestMessagePolicyEvaluatorReBACDirectContactActiveAllowsExactRuleWhenSatisfiedIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           false,
		PermissionVersion: 1,
		Classification:    "STATIC_DENY",
	})
	command := testPolicyCommand(types.MessageActionSend)
	command.DirectPeerUserID = "peer-policy"
	seedReBACRelationRule(
		t,
		ctx,
		pool,
		command.AuthContext.TenantID,
		command.Action,
		types.ReBACRelationDirectContactActive,
		types.ReBACConversationScopeDirect,
		401,
		"DIRECT_CONTACT_REQUIRED",
		"direct contact required",
	)
	seedContactEdge(t, ctx, pool, string(command.AuthContext.UserID), string(command.DirectPeerUserID), types.ContactEdgeStatusActive, 12)
	seedContactEdge(t, ctx, pool, string(command.DirectPeerUserID), string(command.AuthContext.UserID), types.ContactEdgeStatusActive, 13)
	seedPolicyRule(t, ctx, pool, command, true, 99, "EXACT_ALLOW", "")

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if !decision.Allowed || decision.PermissionVersion != 99 || decision.Classification != "EXACT_ALLOW" {
		t.Fatalf("expected exact allow after satisfied rebac relation, got %+v", decision)
	}
	assertPolicyDecisionSource(t, decision, types.PolicyDecisionSourceExactRule)
}

func TestMessagePolicyEvaluatorReBACConversationMemberActiveDeniesLeftMemberIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 1,
		Classification:    "STATIC_ALLOW",
	})
	command := testPolicyCommand(types.MessageActionSend)
	seedReBACRelationRule(
		t,
		ctx,
		pool,
		command.AuthContext.TenantID,
		command.Action,
		types.ReBACRelationConversationMemberActive,
		types.ReBACConversationScopeGroup,
		402,
		"ACTIVE_MEMBER_REQUIRED",
		"",
	)
	seedConversationMember(
		t,
		ctx,
		pool,
		command.ConversationID,
		command.AuthContext.UserID,
		types.ConversationMemberRoleMember,
		types.ConversationMemberStatusLeft,
		5,
		command.ConversationPermissionVersion,
	)
	seedTenantPolicyRule(t, ctx, pool, command.AuthContext.TenantID, command.Action, true, 88, "TENANT_ALLOW", "")

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if decision.Allowed ||
		decision.PermissionVersion != 402 ||
		decision.Classification != "ACTIVE_MEMBER_REQUIRED" ||
		decision.Reason != "relationship policy denied" {
		t.Fatalf("expected rebac active-member deny before tenant allow, got %+v", decision)
	}
	assertPolicyDecisionSource(t, decision, types.PolicyDecisionSourceReBACRelation)
}

func TestMessagePolicyEvaluatorDisabledTenantQuotaFallsThroughIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 7,
		Classification:    "STATIC_ALLOW",
	})
	command := testPolicyCommand(types.MessageActionSend)
	seedTenantActionQuota(t, ctx, pool, command.AuthContext.TenantID, command.Action, 1, 3600, 303, "TENANT_SEND_QUOTA", "")
	if _, err := pool.Exec(ctx, `
UPDATE policy_tenant_message_action_quotas
SET enabled = false
WHERE tenant_id = $1 AND action = $2
`, command.AuthContext.TenantID, command.Action); err != nil {
		t.Fatalf("disable tenant quota: %v", err)
	}
	seedPolicyDecisionAuditRow(t, ctx, pool, "quota-disabled-allowed", command.AuthContext.TenantID, command.Action, true, time.Now().UTC().Add(-time.Minute))

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if !decision.Allowed ||
		decision.PermissionVersion != 7 ||
		decision.Classification != "STATIC_ALLOW" {
		t.Fatalf("expected disabled quota to fall through, got %+v", decision)
	}
}

func TestMessagePolicyEvaluatorExpiredUserRestrictionFallsThroughIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           false,
		PermissionVersion: 1,
		Classification:    "STATIC_DENY",
		Reason:            "static deny",
	})
	command := testPolicyCommand(types.MessageActionSend)
	seedExpiredUserRestriction(t, ctx, pool, command, 123, "USER_MUTED", "user muted")
	seedTenantPolicyRule(t, ctx, pool, command.AuthContext.TenantID, command.Action, true, 88, "TENANT_ALLOW", "")

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if !decision.Allowed ||
		decision.PermissionVersion != 88 ||
		decision.Classification != "TENANT_ALLOW" ||
		decision.Reason != "" {
		t.Fatalf("expected expired restriction to fall through to tenant allow, got %+v", decision)
	}
}

func TestMessagePolicyEvaluatorUsesConversationRoleRuleIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           false,
		PermissionVersion: 1,
		Classification:    "STATIC_DENY",
		Reason:            "static deny",
	})
	command := testPolicyCommand(types.MessageActionSend)
	seedConversationRoleRule(t, ctx, pool, command.AuthContext.TenantID, command.Action, types.ConversationMemberRoleAdmin, "ROLE_ADMIN", "")
	seedConversationMember(t, ctx, pool, command.ConversationID, command.AuthContext.UserID, types.ConversationMemberRoleAdmin, types.ConversationMemberStatusActive, 3, command.ConversationPermissionVersion)
	seedTenantPolicyRule(t, ctx, pool, command.AuthContext.TenantID, command.Action, true, command.ConversationPermissionVersion, "TENANT_ALLOW", "")

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if !decision.Allowed || decision.PermissionVersion != command.ConversationPermissionVersion || decision.Classification != "TENANT_ALLOW" || decision.Reason != "" {
		t.Fatalf("expected role gate to allow tenant decision, got %+v", decision)
	}
}

func TestMessagePolicyEvaluatorConversationRoleRuleDeniesLowRoleIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 1,
		Classification:    "STATIC_ALLOW",
	})
	command := testPolicyCommand(types.MessageActionDelete)
	seedConversationRoleRule(t, ctx, pool, command.AuthContext.TenantID, command.Action, types.ConversationMemberRoleAdmin, "ROLE_ADMIN", "")
	seedConversationMember(t, ctx, pool, command.ConversationID, command.AuthContext.UserID, types.ConversationMemberRoleMember, types.ConversationMemberStatusActive, 4, command.ConversationPermissionVersion)

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if decision.Allowed || decision.PermissionVersion != command.ConversationPermissionVersion || decision.Classification != "ROLE_ADMIN" || decision.Reason != "conversation role policy denied" {
		t.Fatalf("expected role deny decision, got %+v", decision)
	}
	assertPolicyDecisionSource(t, decision, types.PolicyDecisionSourceConversationRole)
}

func TestMessagePolicyEvaluatorConversationRoleRuleDeniesInactiveMemberIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 1,
		Classification:    "STATIC_ALLOW",
	})
	command := testPolicyCommand(types.MessageActionEdit)
	seedConversationRoleRule(t, ctx, pool, command.AuthContext.TenantID, command.Action, types.ConversationMemberRoleMember, "ROLE_MEMBER", "active member required")
	seedConversationMember(t, ctx, pool, command.ConversationID, command.AuthContext.UserID, types.ConversationMemberRoleOwner, types.ConversationMemberStatusLeft, 5, command.ConversationPermissionVersion)

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if decision.Allowed || decision.PermissionVersion != command.ConversationPermissionVersion || decision.Classification != "ROLE_MEMBER" || decision.Reason != "active member required" {
		t.Fatalf("expected inactive role deny decision, got %+v", decision)
	}
}

func TestMessagePolicyEvaluatorConversationRoleRuleRequiresFreshProjectionIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 1,
		Classification:    "STATIC_ALLOW",
	})
	command := testPolicyCommand(types.MessageActionSend)
	seedConversationRoleRule(t, ctx, pool, command.AuthContext.TenantID, command.Action, types.ConversationMemberRoleMember, "ROLE_MEMBER", "")
	seedConversationMember(t, ctx, pool, command.ConversationID, command.AuthContext.UserID, types.ConversationMemberRoleMember, types.ConversationMemberStatusActive, 3, command.ConversationPermissionVersion-1)

	_, err := evaluator.DecideMessageAction(ctx, command)
	if !errors.Is(err, types.ErrDependencyUnavailable) {
		t.Fatalf("expected stale role projection to fail closed, got %v", err)
	}
}

func TestMessagePolicyEvaluatorExactRuleOverridesConversationRoleRuleIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 1,
		Classification:    "STATIC_ALLOW",
	})
	command := testPolicyCommand(types.MessageActionRevoke)
	seedConversationRoleRule(t, ctx, pool, command.AuthContext.TenantID, command.Action, types.ConversationMemberRoleMember, "ROLE_MEMBER", "member required")
	seedConversationMember(t, ctx, pool, command.ConversationID, command.AuthContext.UserID, types.ConversationMemberRoleMember, types.ConversationMemberStatusActive, 3, command.ConversationPermissionVersion)
	seedPolicyRule(t, ctx, pool, command, true, 99, "EXACT_ALLOW", "")

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if !decision.Allowed || decision.PermissionVersion != 99 || decision.Classification != "EXACT_ALLOW" {
		t.Fatalf("expected exact rule to override role rule, got %+v", decision)
	}
}

func TestMessagePolicyEvaluatorConversationRoleRuleOverridesTenantRuleIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 1,
		Classification:    "STATIC_ALLOW",
	})
	command := testPolicyCommand(types.MessageActionSend)
	seedTenantPolicyRule(t, ctx, pool, command.AuthContext.TenantID, command.Action, true, 88, "TENANT_ALLOW", "")
	seedConversationRoleRule(t, ctx, pool, command.AuthContext.TenantID, command.Action, types.ConversationMemberRoleAdmin, "ROLE_ADMIN", "admin only")
	seedConversationMember(t, ctx, pool, command.ConversationID, command.AuthContext.UserID, types.ConversationMemberRoleMember, types.ConversationMemberStatusActive, 3, command.ConversationPermissionVersion)

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if decision.Allowed || decision.PermissionVersion != command.ConversationPermissionVersion || decision.Classification != "ROLE_ADMIN" || decision.Reason != "admin only" {
		t.Fatalf("expected role gate to deny before tenant allow, got %+v", decision)
	}
}

func TestMessagePolicyEvaluatorAllowsMessageOwnershipOverrideIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           false,
		PermissionVersion: 1,
		Classification:    "STATIC_DENY",
	})
	command := testPolicyCommand(types.MessageActionDelete)
	command.AuthContext.UserID = "admin-policy"
	command.MessageSenderUserID = "sender-policy"
	seedOwnershipOverrideRule(t, ctx, pool, command.AuthContext.TenantID, command.Action, types.ConversationMemberRoleAdmin, "MESSAGE_OWNERSHIP_ROLE_OVERRIDE", "")
	seedConversationMember(t, ctx, pool, command.ConversationID, command.AuthContext.UserID, types.ConversationMemberRoleAdmin, types.ConversationMemberStatusActive, 8, command.ConversationPermissionVersion)

	decision, allowed, err := evaluator.DecideMessageOwnershipOverride(ctx, command)
	if err != nil {
		t.Fatalf("decide ownership override: %v", err)
	}
	if !allowed ||
		!decision.Allowed ||
		decision.PermissionVersion != command.ConversationPermissionVersion ||
		decision.Classification != "MESSAGE_OWNERSHIP_ROLE_OVERRIDE" ||
		decision.UserID != command.AuthContext.UserID ||
		decision.MessageID != command.MessageID {
		t.Fatalf("expected ownership override allow, allowed=%v decision=%+v", allowed, decision)
	}
	assertPolicyDecisionSource(t, decision, types.PolicyDecisionSourceOwnershipOverride)
}

func TestMessagePolicyEvaluatorMessageOwnershipOverrideIgnoresLowRoleIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           false,
		PermissionVersion: 1,
		Classification:    "STATIC_DENY",
	})
	command := testPolicyCommand(types.MessageActionRevoke)
	command.AuthContext.UserID = "member-policy"
	command.MessageSenderUserID = "sender-policy"
	seedOwnershipOverrideRule(t, ctx, pool, command.AuthContext.TenantID, command.Action, types.ConversationMemberRoleAdmin, "MESSAGE_OWNERSHIP_ROLE_OVERRIDE", "")
	seedConversationMember(t, ctx, pool, command.ConversationID, command.AuthContext.UserID, types.ConversationMemberRoleMember, types.ConversationMemberStatusActive, 8, command.ConversationPermissionVersion)

	decision, allowed, err := evaluator.DecideMessageOwnershipOverride(ctx, command)
	if err != nil {
		t.Fatalf("decide ownership override: %v", err)
	}
	if allowed || decision.Allowed || decision.Classification != "" {
		t.Fatalf("expected low role to skip override, allowed=%v decision=%+v", allowed, decision)
	}
}

func TestMessagePolicyEvaluatorMessageOwnershipOverrideRequiresFreshProjectionIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           false,
		PermissionVersion: 1,
		Classification:    "STATIC_DENY",
	})
	command := testPolicyCommand(types.MessageActionEdit)
	command.AuthContext.UserID = "admin-policy"
	command.MessageSenderUserID = "sender-policy"
	seedOwnershipOverrideRule(t, ctx, pool, command.AuthContext.TenantID, command.Action, types.ConversationMemberRoleAdmin, "MESSAGE_OWNERSHIP_ROLE_OVERRIDE", "")
	seedConversationMember(t, ctx, pool, command.ConversationID, command.AuthContext.UserID, types.ConversationMemberRoleAdmin, types.ConversationMemberStatusActive, 8, command.ConversationPermissionVersion-1)

	_, _, err := evaluator.DecideMessageOwnershipOverride(ctx, command)
	if !errors.Is(err, types.ErrDependencyUnavailable) {
		t.Fatalf("expected stale ownership override projection to fail closed, got %v", err)
	}
}

func TestMessagePolicyEvaluatorMessageOwnershipOverrideFallsThroughWithoutRuleIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           false,
		PermissionVersion: 1,
		Classification:    "STATIC_DENY",
	})
	command := testPolicyCommand(types.MessageActionDelete)
	command.AuthContext.UserID = "admin-policy"
	command.MessageSenderUserID = "sender-policy"
	seedConversationMember(t, ctx, pool, command.ConversationID, command.AuthContext.UserID, types.ConversationMemberRoleOwner, types.ConversationMemberStatusActive, 8, command.ConversationPermissionVersion)

	decision, allowed, err := evaluator.DecideMessageOwnershipOverride(ctx, command)
	if err != nil {
		t.Fatalf("decide ownership override: %v", err)
	}
	if allowed || decision.Allowed {
		t.Fatalf("expected missing rule to skip override, allowed=%v decision=%+v", allowed, decision)
	}
}

func TestMessagePolicyEvaluatorContactBlockOverridesTenantRuleIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 9,
		Classification:    "STATIC_ALLOW",
	})
	command := testPolicyCommand(types.MessageActionSend)
	command.DirectPeerUserID = "peer-policy"
	seedContactEdge(t, ctx, pool, "peer-policy", string(command.AuthContext.UserID), types.ContactEdgeStatusBlocked, 12)
	seedTenantPolicyRule(t, ctx, pool, command.AuthContext.TenantID, command.Action, true, 88, "TENANT_ALLOW", "")

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if decision.Allowed ||
		decision.PermissionVersion != 12 ||
		decision.Classification != "CONTACT_BLOCKED" ||
		decision.Reason != "contact blocked" {
		t.Fatalf("expected contact block to override tenant allow, got %+v", decision)
	}
	assertPolicyDecisionSource(t, decision, types.PolicyDecisionSourceContactProjection)
}

func TestMessagePolicyEvaluatorDeniesBlockedDirectPeerIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 9,
		Classification:    "STATIC_ALLOW",
	})
	command := testPolicyCommand(types.MessageActionSend)
	command.DirectPeerUserID = "peer-policy"
	seedContactEdge(t, ctx, pool, "peer-policy", string(command.AuthContext.UserID), types.ContactEdgeStatusBlocked, 12)
	seedPolicyRule(t, ctx, pool, command, true, 42, "PG_ALLOW", "")

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if decision.Allowed ||
		decision.PermissionVersion != 12 ||
		decision.Classification != "CONTACT_BLOCKED" ||
		decision.Reason != "contact blocked" {
		t.Fatalf("expected contact blocked deny, got %+v", decision)
	}
}

func TestMessagePolicyEvaluatorDeniesSenderBlockedPeerIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 9,
		Classification:    "STATIC_ALLOW",
	})
	command := testPolicyCommand(types.MessageActionSend)
	command.DirectPeerUserID = "peer-policy"
	seedContactEdge(t, ctx, pool, string(command.AuthContext.UserID), "peer-policy", types.ContactEdgeStatusBlocked, 13)

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if decision.Allowed ||
		decision.PermissionVersion != 13 ||
		decision.Classification != "CONTACT_BLOCKED" ||
		decision.Reason != "contact blocked" {
		t.Fatalf("expected sender contact block deny, got %+v", decision)
	}
}

func TestMessagePolicyEvaluatorIgnoresContactProjectionWithoutDirectPeerIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 9,
		Classification:    "STATIC_ALLOW",
	})
	command := testPolicyCommand(types.MessageActionSend)
	seedContactEdge(t, ctx, pool, "peer-policy", string(command.AuthContext.UserID), types.ContactEdgeStatusBlocked, 12)

	decision, err := evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		t.Fatalf("decide message action: %v", err)
	}
	if !decision.Allowed || decision.PermissionVersion != 9 || decision.Classification != "STATIC_ALLOW" {
		t.Fatalf("expected fallback allow without direct peer, got %+v", decision)
	}
}

func TestMessagePolicyEvaluatorDoesNotFallbackOnDatabaseErrorIntegration(t *testing.T) {
	pool := openTestPool(t)
	pool.Close()
	evaluator := NewMessagePolicyEvaluator(pool, domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 9,
		Classification:    "STATIC_ALLOW",
	})

	_, err := evaluator.DecideMessageAction(context.Background(), testPolicyCommand(types.MessageActionSend))
	if !errors.Is(err, types.ErrDependencyUnavailable) {
		t.Fatalf("expected dependency unavailable without static fallback, got %v", err)
	}
}

func TestIsUndefinedTable(t *testing.T) {
	if !isUndefinedTable(&pgconn.PgError{Code: "42P01"}) {
		t.Fatalf("expected undefined table error to match")
	}
	if isUndefinedTable(&pgconn.PgError{Code: "42703"}) {
		t.Fatalf("did not expect undefined column error to match")
	}
	if isUndefinedTable(errors.New("plain error")) {
		t.Fatalf("did not expect plain error to match")
	}
}

func assertPolicyDecisionSource(t *testing.T, decision types.MessageActionDecision, want types.PolicyDecisionSource) {
	t.Helper()
	if decision.DecisionSource != want {
		t.Fatalf("expected decision source %s, got %+v", want, decision)
	}
}

func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres pool: %v", err)
	}
	t.Cleanup(pool.Close)
	applyPolicyMigration(t, ctx, pool)
	return pool
}

func applyPolicyMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "policy")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read policy migrations: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".sql" {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		migration, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read policy migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(migration)); err != nil {
			t.Fatalf("apply policy migration %s: %v", name, err)
		}
	}
}

func resetPolicyTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `TRUNCATE policy_decision_audit_outbox_repair_audit, policy_decision_audit_outbox, policy_rebac_message_action_rules, policy_user_message_action_restrictions, policy_message_ownership_override_rules, policy_conversation_role_action_rules, policy_conversation_members_projection, policy_tenant_message_action_quotas, policy_tenant_message_action_rules, policy_message_action_rules, policy_contact_edges_projection, policy_kafka_checkpoints`); err != nil {
		t.Fatalf("reset policy tables: %v", err)
	}
}

func seedReBACRelationRule(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	tenantID types.TenantID,
	action types.MessageAction,
	relationType types.ReBACRelationType,
	scope types.ReBACConversationScope,
	permissionVersion int64,
	classification string,
	reason string,
) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO policy_rebac_message_action_rules (
    tenant_id,
    action,
    relation_type,
    conversation_scope,
    permission_version,
    classification,
    reason
) VALUES ($1, $2, $3, $4, $5, $6, $7)
`, tenantID, action, relationType, scope, permissionVersion, classification, reason)
	if err != nil {
		t.Fatalf("seed rebac relation rule: %v", err)
	}
}

func seedTenantActionQuota(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	tenantID types.TenantID,
	action types.MessageAction,
	maxDecisions int,
	windowSeconds int,
	permissionVersion int64,
	classification string,
	reason string,
) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO policy_tenant_message_action_quotas (
    tenant_id,
    action,
    max_decisions,
    window_seconds,
    permission_version,
    classification,
    reason
) VALUES ($1, $2, $3, $4, $5, $6, $7)
`, tenantID, action, maxDecisions, windowSeconds, permissionVersion, classification, reason)
	if err != nil {
		t.Fatalf("seed tenant action quota: %v", err)
	}
}

func seedPolicyDecisionAuditRow(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	eventID string,
	tenantID types.TenantID,
	action types.MessageAction,
	allowed bool,
	createdAt time.Time,
) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO policy_decision_audit_outbox (
    event_id,
    tenant_id,
    aggregate_type,
    aggregate_id,
    mapping_version,
    actor_user_key,
    device_key,
    conversation_key,
    message_key,
    action,
    message_id_present,
    allowed,
    permission_version,
    classification,
    reason_code,
    partition_key,
    correlation_id,
    causation_id,
    trace_id,
    payload_json,
    created_at,
    available_at,
    updated_at
) VALUES (
    $1,
    $2,
    'policy_decision',
    'quota-test',
    1,
    'actor-key',
    'device-key',
    'conversation-key',
    'message-key',
    $3,
    true,
    $4,
    1,
    'QUOTA_TEST',
    '',
    'quota-test',
    'request-quota',
    'request-quota',
    'trace-quota',
    $5::jsonb,
    $6,
    $6,
    $6
)
`, eventID, tenantID, action, allowed, `{"event_id":"`+eventID+`"}`, createdAt)
	if err != nil {
		t.Fatalf("seed policy decision audit row: %v", err)
	}
}

func seedPolicyRule(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	command types.CheckMessageActionCommand,
	allowed bool,
	permissionVersion int64,
	classification string,
	reason string,
) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO policy_message_action_rules (
    tenant_id,
    user_id,
    conversation_id,
    action,
    allowed,
    permission_version,
    classification,
    reason
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID, command.Action, allowed, permissionVersion, classification, reason)
	if err != nil {
		t.Fatalf("seed policy rule: %v", err)
	}
}

func seedTenantPolicyRule(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	tenantID types.TenantID,
	action types.MessageAction,
	allowed bool,
	permissionVersion int64,
	classification string,
	reason string,
) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO policy_tenant_message_action_rules (
    tenant_id,
    action,
    allowed,
    permission_version,
    classification,
    reason
) VALUES ($1, $2, $3, $4, $5, $6)
`, tenantID, action, allowed, permissionVersion, classification, reason)
	if err != nil {
		t.Fatalf("seed tenant policy rule: %v", err)
	}
}

func seedUserRestriction(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	command types.CheckMessageActionCommand,
	permissionVersion int64,
	classification string,
	reason string,
) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO policy_user_message_action_restrictions (
    tenant_id,
    user_id,
    action,
    permission_version,
    classification,
    reason
) VALUES ($1, $2, $3, $4, $5, $6)
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.Action, permissionVersion, classification, reason)
	if err != nil {
		t.Fatalf("seed user restriction: %v", err)
	}
}

func seedExpiredUserRestriction(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	command types.CheckMessageActionCommand,
	permissionVersion int64,
	classification string,
	reason string,
) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO policy_user_message_action_restrictions (
    tenant_id,
    user_id,
    action,
    permission_version,
    classification,
    reason,
    expires_at
) VALUES ($1, $2, $3, $4, $5, $6, now() - interval '1 second')
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.Action, permissionVersion, classification, reason)
	if err != nil {
		t.Fatalf("seed expired user restriction: %v", err)
	}
}

func seedConversationRoleRule(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	tenantID types.TenantID,
	action types.MessageAction,
	minRole string,
	classification string,
	reason string,
) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO policy_conversation_role_action_rules (
    tenant_id,
    action,
    min_role,
    classification,
    reason
) VALUES ($1, $2, $3, $4, $5)
`, tenantID, action, minRole, classification, reason)
	if err != nil {
		t.Fatalf("seed conversation role rule: %v", err)
	}
}

func seedOwnershipOverrideRule(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	tenantID types.TenantID,
	action types.MessageAction,
	minRole string,
	classification string,
	reason string,
) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO policy_message_ownership_override_rules (
    tenant_id,
    action,
    min_role,
    classification,
    reason
) VALUES ($1, $2, $3, $4, $5)
`, tenantID, action, minRole, classification, reason)
	if err != nil {
		t.Fatalf("seed ownership override rule: %v", err)
	}
}

func seedConversationMember(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	conversationID types.ConversationID,
	userID types.UserID,
	role string,
	status string,
	memberVersion int64,
	permissionVersion int64,
) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO policy_conversation_members_projection (
    tenant_id,
    conversation_id,
    user_id,
    role,
    status,
    member_version,
    permission_version,
    updated_by_event_id
) VALUES ('tenant-policy', $1, $2, $3, $4, $5, $6, 'member-projection-test-event')
`, conversationID, userID, role, status, memberVersion, permissionVersion)
	if err != nil {
		t.Fatalf("seed conversation member projection: %v", err)
	}
}

func seedContactEdge(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	owner string,
	contact string,
	status string,
	edgeVersion int64,
) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO policy_contact_edges_projection (
    tenant_id,
    owner_user_id,
    contact_user_id,
    status,
    edge_version,
    updated_by_event_id
) VALUES ('tenant-policy', $1, $2, $3, $4, 'contact-edge-test-event')
`, owner, contact, status, edgeVersion)
	if err != nil {
		t.Fatalf("seed contact edge: %v", err)
	}
}

func testPolicyCommand(action types.MessageAction) types.CheckMessageActionCommand {
	command := types.CheckMessageActionCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-policy",
			UserID:   "user-policy",
			DeviceID: "device-policy",
		},
		ConversationID:                "conv-policy",
		Action:                        action,
		ConversationPermissionVersion: 7,
	}
	if action != types.MessageActionSend {
		command.MessageID = "msg-policy"
	}
	return command
}
