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
	if _, err := pool.Exec(ctx, `TRUNCATE action_executor_provider_failures, action_executor_tool_results, action_executor_execution_audits`); err != nil {
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
	if err := repository.RecordExecution(ctx, audit, projection, nil); err != nil {
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

func TestRepositoryInsertProviderFailureProjectionIntegration(t *testing.T) {
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
	if _, err := pool.Exec(ctx, `TRUNCATE action_executor_provider_failures, action_executor_tool_results, action_executor_execution_audits`); err != nil {
		t.Fatalf("reset action executor tables: %v", err)
	}

	repository := NewRepository(pool)
	audit := types.ExecutionAudit{
		TenantID:          "tenant-1",
		ExecutionID:       "exec-provider-failure-1",
		ProposalID:        "proposal-provider-failure-1",
		ApprovalID:        "approval-provider-failure-1",
		PreparedAuditID:   "mcp-audit-provider-failure-1",
		UserID:            "user-1",
		DeviceID:          "device-1",
		SkillID:           "skill-1",
		ToolName:          "conversation.reply.send",
		Action:            types.ToolActionExecute,
		ResourceType:      "conversation",
		ResourceID:        "conv-1",
		RiskLevel:         "LOW",
		Intent:            "send approved reply",
		IdempotencyKey:    "idem-provider-failure-1",
		InputSHA256:       "abc",
		Allowed:           true,
		RequiresApproval:  true,
		PermissionVersion: 7,
		Classification:    "TOOL_PROVIDER_UNAVAILABLE",
		Reason:            "tool provider unavailable",
		DecisionSource:    "action-executor",
		Status:            types.ExecutionStatusFailed,
		Executed:          false,
	}
	projection := types.ToolResultProjection{
		TenantID:        "tenant-1",
		ResultID:        "result-provider-failure-1",
		ExecutionID:     "exec-provider-failure-1",
		ProposalID:      "proposal-provider-failure-1",
		ApprovalID:      "approval-provider-failure-1",
		PreparedAuditID: "mcp-audit-provider-failure-1",
		UserID:          "user-1",
		SkillID:         "skill-1",
		ToolName:        "conversation.reply.send",
		ResourceType:    "conversation",
		ResourceID:      "conv-1",
		Status:          types.ResultStatusFailed,
		Executed:        false,
		ResultRef:       "action-executor://executions/exec-provider-failure-1/results/result-provider-failure-1",
	}
	failure := types.ProviderFailureProjection{
		TenantID:          "tenant-1",
		ProviderFailureID: "provider-failure-1",
		ExecutionID:       "exec-provider-failure-1",
		ResultID:          "result-provider-failure-1",
		ProposalID:        "proposal-provider-failure-1",
		ApprovalID:        "approval-provider-failure-1",
		PreparedAuditID:   "mcp-audit-provider-failure-1",
		UserID:            "user-1",
		SkillID:           "skill-1",
		ToolName:          "conversation.reply.send",
		ResourceType:      "conversation",
		ResourceID:        "conv-1",
		Classification:    "TOOL_PROVIDER_UNAVAILABLE",
		Status:            types.ProviderFailureStatusRetryPending,
		Retryable:         true,
		RetryCount:        0,
		NextRetryAt:       time.Now().UTC().Add(time.Minute),
		FailureRef:        "action-executor://executions/exec-provider-failure-1/provider-failures/provider-failure-1",
		CreatedAt:         time.Now().UTC(),
	}
	if err := repository.RecordExecution(ctx, audit, projection, &failure); err != nil {
		t.Fatalf("record execution provider failure: %v", err)
	}

	var storedStatus string
	var storedClassification string
	var storedRetryable bool
	var storedRetryCount int
	var storedFailureRef string
	var nextRetryAt time.Time
	var deadLetteredAtIsNull bool
	err = pool.QueryRow(ctx, `
SELECT status, classification, retryable, retry_count, failure_ref, next_retry_at, dead_lettered_at IS NULL
FROM action_executor_provider_failures
WHERE tenant_id = $1 AND provider_failure_id = $2`, "tenant-1", "provider-failure-1").Scan(
		&storedStatus,
		&storedClassification,
		&storedRetryable,
		&storedRetryCount,
		&storedFailureRef,
		&nextRetryAt,
		&deadLetteredAtIsNull,
	)
	if err != nil {
		t.Fatalf("query provider failure: %v", err)
	}
	if storedStatus != types.ProviderFailureStatusRetryPending ||
		storedClassification != "TOOL_PROVIDER_UNAVAILABLE" ||
		!storedRetryable ||
		storedRetryCount != 0 ||
		storedFailureRef == "" ||
		nextRetryAt.IsZero() ||
		!deadLetteredAtIsNull {
		t.Fatalf("unexpected provider failure row: status=%s class=%s retryable=%v count=%d ref=%s next=%v dlq=%v",
			storedStatus,
			storedClassification,
			storedRetryable,
			storedRetryCount,
			storedFailureRef,
			nextRetryAt,
			deadLetteredAtIsNull,
		)
	}
}

func TestRepositoryProcessDueProviderFailuresIntegration(t *testing.T) {
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
	if _, err := pool.Exec(ctx, `TRUNCATE action_executor_provider_failures, action_executor_tool_results, action_executor_execution_audits`); err != nil {
		t.Fatalf("reset action executor tables: %v", err)
	}

	repository := NewRepository(pool)
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	recordProviderFailureFixture(t, ctx, repository, "reschedule", 0, now.Add(-time.Minute))
	recordProviderFailureFixture(t, ctx, repository, "deadletter", 2, now.Add(-time.Minute))
	recordProviderFailureFixture(t, ctx, repository, "future", 0, now.Add(time.Hour))

	stats, err := repository.ProcessDueProviderFailures(ctx, 10, 3, time.Second, now)
	if err != nil {
		t.Fatalf("process due provider failures: %v", err)
	}
	if stats.Fetched != 2 || stats.Rescheduled != 1 || stats.DeadLettered != 1 {
		t.Fatalf("unexpected retry stats: %+v", stats)
	}

	rescheduled := queryProviderFailureState(t, ctx, pool, "provider-failure-reschedule", now)
	if rescheduled.Status != types.ProviderFailureStatusRetryPending ||
		rescheduled.RetryCount != 1 ||
		!rescheduled.NextRetryAfterNow ||
		rescheduled.NextRetryNull ||
		rescheduled.DeadLettered {
		t.Fatalf("unexpected rescheduled state: %+v", rescheduled)
	}
	deadletter := queryProviderFailureState(t, ctx, pool, "provider-failure-deadletter", now)
	if deadletter.Status != types.ProviderFailureStatusDLQ ||
		deadletter.Retryable ||
		deadletter.RetryCount != 3 ||
		!deadletter.NextRetryNull ||
		!deadletter.DeadLettered {
		t.Fatalf("unexpected deadletter state: %+v", deadletter)
	}
	future := queryProviderFailureState(t, ctx, pool, "provider-failure-future", now)
	if future.Status != types.ProviderFailureStatusRetryPending ||
		!future.Retryable ||
		future.RetryCount != 0 ||
		!future.NextRetryAfterNow ||
		future.NextRetryNull ||
		future.DeadLettered {
		t.Fatalf("unexpected future state: %+v", future)
	}
}

func applyMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for _, name := range []string{
		"000001_action_executor_core.sql",
		"000002_action_executor_tool_results.sql",
		"000003_action_executor_provider_failures.sql",
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

type providerFailureState struct {
	Status            string
	Retryable         bool
	RetryCount        int
	NextRetryAfterNow bool
	NextRetryNull     bool
	DeadLettered      bool
}

func queryProviderFailureState(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	providerFailureID string,
	now time.Time,
) providerFailureState {
	t.Helper()
	var state providerFailureState
	err := pool.QueryRow(ctx, `
SELECT status,
       retryable,
       retry_count,
       COALESCE(next_retry_at > $3, false),
       next_retry_at IS NULL,
       dead_lettered_at IS NOT NULL
FROM action_executor_provider_failures
WHERE tenant_id = $1 AND provider_failure_id = $2`,
		"tenant-1",
		providerFailureID,
		now,
	).Scan(
		&state.Status,
		&state.Retryable,
		&state.RetryCount,
		&state.NextRetryAfterNow,
		&state.NextRetryNull,
		&state.DeadLettered,
	)
	if err != nil {
		t.Fatalf("query provider failure state %s: %v", providerFailureID, err)
	}
	return state
}

func recordProviderFailureFixture(
	t *testing.T,
	ctx context.Context,
	repository Repository,
	suffix string,
	retryCount int,
	nextRetryAt time.Time,
) {
	t.Helper()
	audit := types.ExecutionAudit{
		TenantID:          "tenant-1",
		ExecutionID:       "exec-" + suffix,
		ProposalID:        "proposal-" + suffix,
		ApprovalID:        "approval-" + suffix,
		PreparedAuditID:   "mcp-audit-" + suffix,
		UserID:            "user-1",
		DeviceID:          "device-1",
		SkillID:           "skill-1",
		ToolName:          "conversation.reply.send",
		Action:            types.ToolActionExecute,
		ResourceType:      "conversation",
		ResourceID:        "conv-1",
		RiskLevel:         "LOW",
		Intent:            "send approved reply",
		IdempotencyKey:    "idem-" + suffix,
		InputSHA256:       "abc",
		Allowed:           true,
		RequiresApproval:  true,
		PermissionVersion: 7,
		Classification:    "TOOL_PROVIDER_UNAVAILABLE",
		Reason:            "tool provider unavailable",
		DecisionSource:    "action-executor",
		Status:            types.ExecutionStatusFailed,
		Executed:          false,
	}
	projection := types.ToolResultProjection{
		TenantID:        "tenant-1",
		ResultID:        "result-" + suffix,
		ExecutionID:     "exec-" + suffix,
		ProposalID:      "proposal-" + suffix,
		ApprovalID:      "approval-" + suffix,
		PreparedAuditID: "mcp-audit-" + suffix,
		UserID:          "user-1",
		SkillID:         "skill-1",
		ToolName:        "conversation.reply.send",
		ResourceType:    "conversation",
		ResourceID:      "conv-1",
		Status:          types.ResultStatusFailed,
		Executed:        false,
		ResultRef:       "action-executor://executions/exec-" + suffix + "/results/result-" + suffix,
	}
	failure := types.ProviderFailureProjection{
		TenantID:          "tenant-1",
		ProviderFailureID: "provider-failure-" + suffix,
		ExecutionID:       "exec-" + suffix,
		ResultID:          "result-" + suffix,
		ProposalID:        "proposal-" + suffix,
		ApprovalID:        "approval-" + suffix,
		PreparedAuditID:   "mcp-audit-" + suffix,
		UserID:            "user-1",
		SkillID:           "skill-1",
		ToolName:          "conversation.reply.send",
		ResourceType:      "conversation",
		ResourceID:        "conv-1",
		Classification:    "TOOL_PROVIDER_UNAVAILABLE",
		Status:            types.ProviderFailureStatusRetryPending,
		Retryable:         true,
		RetryCount:        retryCount,
		NextRetryAt:       nextRetryAt,
		FailureRef:        "action-executor://executions/exec-" + suffix + "/provider-failures/provider-failure-" + suffix,
		CreatedAt:         nextRetryAt,
	}
	if err := repository.RecordExecution(ctx, audit, projection, &failure); err != nil {
		t.Fatalf("record provider failure fixture %s: %v", suffix, err)
	}
}
