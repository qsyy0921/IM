package postgres

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/action-executor/internal/types"
)

func TestRepositoryInsertExecutionAuditIntegration(t *testing.T) {
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect pg: %v", err)
	}
	defer pool.Close()
	applyMigration(t, ctx, pool)
	if _, err := pool.Exec(ctx, `TRUNCATE action_executor_tool_results, action_executor_execution_audits`); err != nil {
		t.Fatalf("reset action executor tables: %v", err)
	}

	repository := NewRepository(pool)
	audit := types.ExecutionAudit{
		TenantID:          "tenant-1",
		ExecutionID:       "exec-1",
		ProposalID:        "proposal-1",
		ApprovalID:        "approval-1",
		PreparedAuditID:   "mcp-audit-1",
		UserID:            "user-1",
		DeviceID:          "device-1",
		SkillID:           "skill-1",
		ToolName:          "conversation.reply.send",
		Action:            types.ToolActionExecute,
		ResourceType:      "conversation",
		ResourceID:        "conv-1",
		RiskLevel:         "LOW",
		Intent:            "send approved reply",
		IdempotencyKey:    "idem-1",
		InputSHA256:       "abc",
		Allowed:           true,
		RequiresApproval:  true,
		PermissionVersion: 7,
		Classification:    "TOOL_ALLOWED",
		Reason:            "allowed",
		DecisionSource:    "TOOL_RULE",
		Status:            types.ExecutionStatusRecorded,
		Executed:          false,
	}
	projection := types.ToolResultProjection{
		TenantID:        "tenant-1",
		ResultID:        "result-1",
		ExecutionID:     "exec-1",
		ProposalID:      "proposal-1",
		ApprovalID:      "approval-1",
		PreparedAuditID: "mcp-audit-1",
		UserID:          "user-1",
		SkillID:         "skill-1",
		ToolName:        "conversation.reply.send",
		ResourceType:    "conversation",
		ResourceID:      "conv-1",
		Status:          types.ResultStatusNotExecuted,
		Executed:        false,
		ResultRef:       "action-executor://executions/exec-1/results/result-1",
	}
	if err := repository.RecordExecution(ctx, audit, projection); err != nil {
		t.Fatalf("record execution: %v", err)
	}

	var storedStatus string
	var storedInputHash string
	var storedExecuted bool
	err = pool.QueryRow(ctx, `
SELECT status, input_sha256, executed
FROM action_executor_execution_audits
WHERE tenant_id = $1 AND execution_id = $2`, "tenant-1", "exec-1").Scan(&storedStatus, &storedInputHash, &storedExecuted)
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if storedStatus != types.ExecutionStatusRecorded || storedInputHash != "abc" || storedExecuted {
		t.Fatalf("unexpected audit row: status=%s input=%s executed=%v", storedStatus, storedInputHash, storedExecuted)
	}
	var resultStatus string
	var resultRef string
	err = pool.QueryRow(ctx, `
SELECT status, result_ref
FROM action_executor_tool_results
WHERE tenant_id = $1 AND execution_id = $2`, "tenant-1", "exec-1").Scan(&resultStatus, &resultRef)
	if err != nil {
		t.Fatalf("query tool result projection: %v", err)
	}
	if resultStatus != types.ResultStatusNotExecuted || resultRef == "" {
		t.Fatalf("unexpected result projection: status=%s ref=%s", resultStatus, resultRef)
	}
}

func applyMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for _, name := range []string{
		"000001_action_executor_core.sql",
		"000002_action_executor_tool_results.sql",
	} {
		path := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "action-executor", name)
		sqlBytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
}
