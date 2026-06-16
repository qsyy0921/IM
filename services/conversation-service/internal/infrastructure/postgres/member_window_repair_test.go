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

	if _, err := repository.RepairMemberWindows(ctx, MemberWindowRepairOptions{IssueClass: "unknown"}); err == nil {
		t.Fatalf("expected unsupported repair issue class to fail")
	}
	if _, err := repository.AuditMemberWindowRepairs(ctx, MemberWindowRepairAuditOptions{Outcome: "unknown"}); err == nil {
		t.Fatalf("expected unsupported repair outcome to fail")
	}
}

func TestRepositoryRepairMemberWindowsSetsInactiveLeaveSeqIntegration(t *testing.T) {
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
		IssueClass:     MemberWindowIssueInactiveWithoutLeaveSeq,
		OperatorID:     "operator-2",
		Reason:         "dry run inactive missing leave_seq",
		DryRun:         true,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("dry-run inactive repair member windows: %v", err)
	}
	if dryRunStats.Requested != 2 || dryRunStats.Repaired != 0 || dryRunStats.Skipped != 2 || !dryRunStats.DryRun {
		t.Fatalf("unexpected inactive dry-run stats: %+v", dryRunStats)
	}
	assertMemberLeaveSeqValue(t, ctx, pool, "tenant-window-repair", "conv-window-repair", "left-missing-leave", nil)

	mutateStats, err := repository.RepairMemberWindows(ctx, MemberWindowRepairOptions{
		TenantID:       "tenant-window-repair",
		ConversationID: "conv-window-repair",
		IssueClass:     "inactive_without_leave_seq",
		OperatorID:     "operator-2",
		Reason:         "set inactive leave_seq",
		DryRun:         false,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("mutating inactive repair member windows: %v", err)
	}
	if mutateStats.Requested != 2 || mutateStats.Repaired != 2 || mutateStats.Skipped != 0 || mutateStats.DryRun {
		t.Fatalf("unexpected inactive mutate stats: %+v", mutateStats)
	}
	assertMemberLeaveSeqValue(t, ctx, pool, "tenant-window-repair", "conv-window-repair", "left-missing-leave", ptrInt64(9))
	assertMemberLeaveSeqValue(t, ctx, pool, "tenant-window-repair", "conv-window-repair", "banned-zero-leave", ptrInt64(10))
	assertMemberLeaveSeqValue(t, ctx, pool, "tenant-window-repair", "conv-window-repair", "leave-before-join", ptrInt64(7))

	rows, err := repository.AuditMemberWindowRepairs(ctx, MemberWindowRepairAuditOptions{
		TenantID:       "tenant-window-repair",
		ConversationID: "conv-window-repair",
		IssueClass:     "inactive_without_leave_seq",
		Outcome:        "mutated",
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("audit inactive member window repairs: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two inactive mutated audit rows, got %d: %+v", len(rows), rows)
	}
	for _, row := range rows {
		if row.RepairAction != memberWindowRepairActionSetInactiveLeaveSeq ||
			row.RepairOutcome != memberWindowRepairOutcomeMutated ||
			row.OperatorID != "operator-2" ||
			row.Reason != "set inactive leave_seq" ||
			row.DryRun ||
			!row.HasJoinSeq ||
			!row.HasNewLeaveSeq {
			t.Fatalf("unexpected inactive mutated audit row: %+v", row)
		}
	}
}

func TestRepositoryRepairMemberWindowsClampsLeaveBeforeJoinIntegration(t *testing.T) {
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
		IssueClass:     MemberWindowIssueLeaveBeforeJoin,
		OperatorID:     "operator-3",
		Reason:         "dry run leave before join",
		DryRun:         true,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("dry-run leave-before-join repair member windows: %v", err)
	}
	if dryRunStats.Requested != 1 || dryRunStats.Repaired != 0 || dryRunStats.Skipped != 1 || !dryRunStats.DryRun {
		t.Fatalf("unexpected leave-before-join dry-run stats: %+v", dryRunStats)
	}
	assertMemberLeaveSeqValue(t, ctx, pool, "tenant-window-repair", "conv-window-repair", "inactive-leave-before-join", ptrInt64(7))

	mutateStats, err := repository.RepairMemberWindows(ctx, MemberWindowRepairOptions{
		TenantID:       "tenant-window-repair",
		ConversationID: "conv-window-repair",
		IssueClass:     "leave_before_join",
		OperatorID:     "operator-3",
		Reason:         "clamp leave_seq to join_seq",
		DryRun:         false,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("mutating leave-before-join repair member windows: %v", err)
	}
	if mutateStats.Requested != 1 || mutateStats.Repaired != 1 || mutateStats.Skipped != 0 || mutateStats.DryRun {
		t.Fatalf("unexpected leave-before-join mutate stats: %+v", mutateStats)
	}
	assertMemberLeaveSeqValue(t, ctx, pool, "tenant-window-repair", "conv-window-repair", "inactive-leave-before-join", ptrInt64(8))

	rows, err := repository.AuditMemberWindowRepairs(ctx, MemberWindowRepairAuditOptions{
		TenantID:       "tenant-window-repair",
		ConversationID: "conv-window-repair",
		IssueClass:     "leave_before_join",
		Outcome:        "mutated",
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("audit leave-before-join member window repairs: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one leave-before-join mutated audit row, got %d: %+v", len(rows), rows)
	}
	row := rows[0]
	if row.UserID != "inactive-leave-before-join" ||
		row.RepairAction != memberWindowRepairActionClampLeaveToJoinSeq ||
		row.RepairOutcome != memberWindowRepairOutcomeMutated ||
		row.OperatorID != "operator-3" ||
		row.Reason != "clamp leave_seq to join_seq" ||
		row.DryRun ||
		!row.HasJoinSeq ||
		!row.HasLeaveSeq ||
		!row.HasNewLeaveSeq ||
		row.PreviousJoinSeq != 8 ||
		row.PreviousLeaveSeq != 7 ||
		row.NewLeaveSeq != 8 {
		t.Fatalf("unexpected leave-before-join audit row: %+v", row)
	}
}

func TestRepositoryRepairMemberWindowsRaisesConversationVersionsIntegration(t *testing.T) {
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
	memberStats, err := repository.RepairMemberWindows(ctx, MemberWindowRepairOptions{
		TenantID:       "tenant-window-repair",
		ConversationID: "conv-window-repair",
		IssueClass:     MemberWindowIssueMemberVersionAhead,
		OperatorID:     "operator-4",
		Reason:         "raise conversation member version",
		DryRun:         false,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("repair member version ahead: %v", err)
	}
	if memberStats.Requested != 1 || memberStats.Repaired != 1 || memberStats.Skipped != 0 || memberStats.DryRun {
		t.Fatalf("unexpected member version stats: %+v", memberStats)
	}
	assertConversationVersions(t, ctx, pool, "tenant-window-repair", "conv-window-repair", 12, 20)

	permissionStats, err := repository.RepairMemberWindows(ctx, MemberWindowRepairOptions{
		TenantID:       "tenant-window-repair",
		ConversationID: "conv-window-repair",
		IssueClass:     "permission_version_ahead_conversation",
		OperatorID:     "operator-5",
		Reason:         "raise conversation permission version",
		DryRun:         false,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("repair permission version ahead: %v", err)
	}
	if permissionStats.Requested != 1 || permissionStats.Repaired != 1 || permissionStats.Skipped != 0 || permissionStats.DryRun {
		t.Fatalf("unexpected permission version stats: %+v", permissionStats)
	}
	assertConversationVersions(t, ctx, pool, "tenant-window-repair", "conv-window-repair", 12, 25)

	rows, err := repository.AuditMemberWindowRepairs(ctx, MemberWindowRepairAuditOptions{
		TenantID:       "tenant-window-repair",
		ConversationID: "conv-window-repair",
		IssueClass:     "member_version_ahead_conversation",
		Outcome:        "mutated",
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("audit member version repair: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one member version audit row, got %d: %+v", len(rows), rows)
	}
	memberRow := rows[0]
	if memberRow.UserID != "member-version-ahead-high" ||
		memberRow.RepairAction != memberWindowRepairActionRaiseMemberVersion ||
		!memberRow.HasPreviousMemberVersion ||
		!memberRow.HasNewMemberVersion ||
		memberRow.PreviousMemberVersion != 10 ||
		memberRow.NewMemberVersion != 12 {
		t.Fatalf("unexpected member version audit row: %+v", memberRow)
	}

	rows, err = repository.AuditMemberWindowRepairs(ctx, MemberWindowRepairAuditOptions{
		TenantID:       "tenant-window-repair",
		ConversationID: "conv-window-repair",
		IssueClass:     "permission_version_ahead_conversation",
		Outcome:        "mutated",
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("audit permission version repair: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one permission version audit row, got %d: %+v", len(rows), rows)
	}
	permissionRow := rows[0]
	if permissionRow.UserID != "permission-version-ahead" ||
		permissionRow.RepairAction != memberWindowRepairActionRaisePermVersion ||
		!permissionRow.HasPreviousPermissionVersion ||
		!permissionRow.HasNewPermissionVersion ||
		permissionRow.PreviousPermissionVersion != 20 ||
		permissionRow.NewPermissionVersion != 25 {
		t.Fatalf("unexpected permission version audit row: %+v", permissionRow)
	}
}

func ensureMemberWindowRepairAuditSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for _, filename := range []string{
		"000005_member_window_repair_audit.sql",
		"000006_member_window_repair_inactive_leave_seq.sql",
		"000007_member_window_repair_leave_before_join.sql",
		"000008_member_window_repair_version_ahead.sql",
	} {
		path := filepath.Clean(filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "conversation", filename))
		ddl, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read member window repair migration %s: %v", filename, err)
		}
		if _, err := pool.Exec(ctx, string(ddl)); err != nil {
			t.Fatalf("apply member window repair migration %s: %v", filename, err)
		}
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
    ('tenant-window-repair', 'conv-window-repair', 'healthy-active', 'MEMBER', 'ACTIVE', 5, NULL, 9, 19, now() - interval '3 minutes'),
    ('tenant-window-repair', 'conv-window-repair', 'left-missing-leave', 'MEMBER', 'LEFT', 6, NULL, 9, 19, now() - interval '4 minutes'),
    ('tenant-window-repair', 'conv-window-repair', 'banned-zero-leave', 'MEMBER', 'BANNED', 7, 0, 10, 19, now() - interval '5 minutes'),
    ('tenant-window-repair', 'conv-window-repair', 'inactive-leave-before-join', 'MEMBER', 'LEFT', 8, 7, 10, 19, now() - interval '6 minutes'),
    ('tenant-window-repair', 'conv-window-repair', 'member-version-ahead-low', 'MEMBER', 'ACTIVE', 9, NULL, 11, 20, now() - interval '7 minutes'),
    ('tenant-window-repair', 'conv-window-repair', 'member-version-ahead-high', 'MEMBER', 'ACTIVE', 10, NULL, 12, 20, now() - interval '8 minutes'),
    ('tenant-window-repair', 'conv-window-repair', 'permission-version-ahead', 'MEMBER', 'ACTIVE', 11, NULL, 10, 25, now() - interval '9 minutes');
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

func assertMemberLeaveSeqValue(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID string, conversationID string, userID string, want *int64) {
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
	if want == nil {
		if leaveSeq != nil {
			t.Fatalf("leave_seq = %d, want NULL for user %s", *leaveSeq, userID)
		}
		return
	}
	if leaveSeq == nil || *leaveSeq != *want {
		if leaveSeq == nil {
			t.Fatalf("leave_seq = NULL, want %d for user %s", *want, userID)
		}
		t.Fatalf("leave_seq = %d, want %d for user %s", *leaveSeq, *want, userID)
	}
}

func assertConversationVersions(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID string, conversationID string, wantMemberVersion int64, wantPermissionVersion int64) {
	t.Helper()
	var memberVersion int64
	var permissionVersion int64
	if err := pool.QueryRow(ctx, `
SELECT member_version, permission_version
FROM conversations
WHERE tenant_id = $1
  AND conversation_id = $2
`, tenantID, conversationID).Scan(&memberVersion, &permissionVersion); err != nil {
		t.Fatalf("query conversation versions: %v", err)
	}
	if memberVersion != wantMemberVersion || permissionVersion != wantPermissionVersion {
		t.Fatalf("conversation versions = member:%d permission:%d, want member:%d permission:%d", memberVersion, permissionVersion, wantMemberVersion, wantPermissionVersion)
	}
}

func ptrInt64(value int64) *int64 {
	return &value
}
