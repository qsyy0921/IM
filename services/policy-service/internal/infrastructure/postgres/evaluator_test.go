package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

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
	if _, err := pool.Exec(ctx, `TRUNCATE policy_decision_audit_outbox_repair_audit, policy_decision_audit_outbox, policy_tenant_message_action_rules, policy_message_action_rules, policy_contact_edges_projection, policy_kafka_checkpoints`); err != nil {
		t.Fatalf("reset policy tables: %v", err)
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
		ConversationID: "conv-policy",
		Action:         action,
	}
	if action != types.MessageActionSend {
		command.MessageID = "msg-policy"
	}
	return command
}
