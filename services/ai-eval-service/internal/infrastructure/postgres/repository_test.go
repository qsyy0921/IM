package postgres

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/ai-eval-service/internal/types"
)

func TestRepositoryEvalRunCatalogIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openAIEvalTestPool(t)
	resetAIEvalTables(t, ctx, pool)
	repository := NewRepository(pool)

	run := types.EvalRun{
		TenantID:     "tenant-1",
		RunID:        "run-profile-001",
		SuiteID:      "profile-agent-safety",
		Stage:        "profile-overgeneralization",
		Adapter:      "local-fixture",
		Status:       types.EvalRunStatusRunning,
		CaseCount:    2,
		SummaryRef:   "H:/NexusIM/loadtest-results/profile-agent-safety/summary.json",
		ReportRef:    "docs/runbook/loadtest/ai-eval/profile-agent-safety.md",
		MetadataJSON: `{"scope":"low-sensitive"}`,
	}
	created, err := repository.RecordEvalRun(ctx, run)
	if err != nil {
		t.Fatalf("record eval run: %v", err)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("expected timestamps: %+v", created)
	}
	if !created.CompletedAt.IsZero() {
		t.Fatalf("running run should not be completed: %+v", created)
	}

	passed := run
	passed.Status = types.EvalRunStatusPassed
	passed.PassedCount = 2
	completed, err := repository.RecordEvalRun(ctx, passed)
	if err != nil {
		t.Fatalf("complete eval run: %v", err)
	}
	if completed.CompletedAt.IsZero() {
		t.Fatalf("expected completed_at for final status: %+v", completed)
	}

	got, err := repository.GetEvalRun(ctx, "tenant-1", "run-profile-001")
	if err != nil {
		t.Fatalf("get eval run: %v", err)
	}
	if got.Status != types.EvalRunStatusPassed || got.PassedCount != 2 {
		t.Fatalf("unexpected eval run: %+v", got)
	}

	if _, err := repository.GetEvalRun(ctx, "tenant-2", "run-profile-001"); err == nil {
		t.Fatal("expected tenant boundary to hide eval run")
	}

	second := run
	second.RunID = "run-profile-002"
	second.Status = types.EvalRunStatusFailed
	second.PassedCount = 1
	second.FailedCount = 1
	if _, err := repository.RecordEvalRun(ctx, second); err != nil {
		t.Fatalf("record second eval run: %v", err)
	}

	listed, err := repository.ListEvalRuns(ctx, types.ListEvalRunsCommand{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"},
		SuiteID:     "profile-agent-safety",
		Status:      types.EvalRunStatusPassed,
	}, 10)
	if err != nil {
		t.Fatalf("list eval runs: %v", err)
	}
	if len(listed) != 1 || listed[0].RunID != "run-profile-001" {
		t.Fatalf("unexpected list: %+v", listed)
	}

	after, err := repository.ListEvalRuns(ctx, types.ListEvalRunsCommand{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"},
		AfterRunID:  "run-profile-001",
	}, 10)
	if err != nil {
		t.Fatalf("list after cursor: %v", err)
	}
	if len(after) != 1 || after[0].RunID != "run-profile-002" {
		t.Fatalf("unexpected cursor list: %+v", after)
	}
}

func openAIEvalTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	t.Cleanup(pool.Close)
	applyAIEvalMigration(t, context.Background(), pool)
	return pool
}

func applyAIEvalMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "ai-eval-service", "000001_ai_eval_core.sql")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(content)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
}

func resetAIEvalTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `TRUNCATE ai_eval_runs`); err != nil {
		t.Fatalf("reset ai eval tables: %v", err)
	}
}
