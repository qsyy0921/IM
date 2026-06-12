package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

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
	path := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "policy", "000001_policy_core.sql")
	migration, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read policy migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply policy migration: %v", err)
	}
}

func resetPolicyTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `TRUNCATE policy_message_action_rules`); err != nil {
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
