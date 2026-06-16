package postgres

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositoryRepairMemberWindowsClearsActiveLeaveSeqIntegration(t *testing.T) {
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	defer pool.Close()

	ensureMemberWindowRepairAuditSchema(t, ctx, pool)
	resetConversationTables(t, ctx, pool)
	truncateMemberWindowRepairAudit(t, ctx, pool)
	seedMemberWindowRepairFixtures(t, ctx, pool)

	repository := NewRepository(pool)
	dryRunStats, err := repository.RepairMemberWindows(ctx, MemberWindowRepairOptions{
		TenantID:       "tenant-window-repair",
		ConversationID: "conv-window-repair",
		OperatorID:     "operator-1",
		Reason:         "dry run stale leave_seq",
		DryRun:         true,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("dry-run repair member windows: %v", err)
	}
	if dryRunStats.Requested != 1 || dryRunStats.Repaired != 0 || dryRunStats.Skipped != 1 || !dryRunStats.DryRun {
		t.Fatalf("unexpected dry-run stats: %+v", dryRunStats)
	}
	assertMemberLeaveSeq(t, ctx, pool, "tenant-window-repair", "conv-window-repair", "stale-active", true)

	mutateStats, err := repository.RepairMemberWindows(ctx, MemberWindowRepairOptions{
		TenantID:       "tenant-window-repair",
		ConversationID: "conv-window-repair",
		OperatorID:     "operator-1",
		Reason:         "clear stale leave_seq",
		DryRun:         false,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("mutating repair member windows: %v", err)
	}
	if mutateStats.Requested != 1 || mutateStats.Repaired != 1 || mutateStats.Skipped != 0 || mutateStats.DryRun {
		t.Fatalf("unexpected mutate stats: %+v", mutateStats)
	}
	assertMemberLeaveSeq(t, ctx, pool, "tenant-window-repair", "conv-window-repair", "stale-active", false)
	assertMemberLeaveSeq(t, ctx, pool, "tenant-window-repair", "conv-window-repair", "leave-before-join", true)

	rows, err := repository.AuditMemberWindowRepairs(ctx, MemberWindowRepairAuditOptions{
		TenantID:       "tenant-window-repair",
		ConversationID: "conv-window-repair",
		IssueClass:     "active_with_leave_seq",
		Outcome:        "mutated",
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("audit member window repairs: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one mutated audit row, got %d: %+v", len(rows), rows)
	}
	row := rows[0]
	if row.UserID != "stale-active" ||
		row.RepairAction != memberWindowRepairActionClearActiveLeaveSeq ||
		row.RepairOutcome != memberWindowRepairOutcomeMutated ||
		row.OperatorID != "operator-1" ||
		row.Reason != "clear stale leave_seq" ||
		row.DryRun ||
		!row.HasJoinSeq ||
		!row.HasLeaveSeq ||
		row.HasNewLeaveSeq {
		t.Fatalf("unexpected mutated audit row: %+v", row)
	}

	if _, err := repository.RepairMemberWindows(ctx, MemberWindowRepairOptions{IssueClass: "leave_before_join"}); err == nil {
		t.Fatalf("expected unsupported repair issue class to fail")
	}
	if _, err := repository.AuditMemberWindowRepairs(ctx, MemberWindowRepairAuditOptions{Outcome: "unknown"}); err == nil {
		t.Fatalf("expected unsupported repair outcome to fail")
	}
}

func ensureMemberWindowRepairAuditSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	path := filepath.Clean(filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "conversation", "000005_member_window_repair_audit.sql"))
	ddl, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read member window repair migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(ddl)); err != nil {
		t.Fatalf("apply member window repair migration: %v", err)
	}
}

func truncateMemberWindowRepairAudit(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `TRUNCATE TABLE conversation_member_window_repair_audit`); err != nil {
		t.Fatalf("truncate member window repair audit: %v", err)
	}
}

func seedMemberWindowRepairFixtures(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO conversations (
    tenant_id, conversation_id, conversation_type, status, conversation_mode,
    fanout_mode, fanout_policy_version, member_version, permission_version, current_seq_shard
) VALUES ('tenant-window-repair', 'conv-window-repair', 'GROUP', 'ACTIVE', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 1, 10, 20, 'local');

INSERT INTO conversation_members (
    tenant_id, conversation_id, user_id, role, status, join_seq, leave_seq,
    member_version, permission_version, updated_at
) VALUES
    ('tenant-window-repair', 'conv-window-repair', 'stale-active', 'MEMBER', 'ACTIVE', 4, 9, 9, 19, now() - interval '1 minute'),
    ('tenant-window-repair', 'conv-window-repair', 'leave-before-join', 'MEMBER', 'ACTIVE', 8, 7, 9, 19, now() - interval '2 minutes'),
    ('tenant-window-repair', 'conv-window-repair', 'healthy-active', 'MEMBER', 'ACTIVE', 5, NULL, 9, 19, now() - interval '3 minutes');
`)
	if err != nil {
		t.Fatalf("seed member window repair fixtures: %v", err)
	}
}

func assertMemberLeaveSeq(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID string, conversationID string, userID string, wantValid bool) {
	t.Helper()
	var leaveSeq *int64
	if err := pool.QueryRow(ctx, `
SELECT leave_seq
FROM conversation_members
WHERE tenant_id = $1
  AND conversation_id = $2
  AND user_id = $3
`, tenantID, conversationID, userID).Scan(&leaveSeq); err != nil {
		t.Fatalf("query member leave_seq: %v", err)
	}
	if (leaveSeq != nil) != wantValid {
		t.Fatalf("leave_seq valid = %t, want %t for user %s", leaveSeq != nil, wantValid, userID)
	}
}
