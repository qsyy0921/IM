package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/workflow-service/internal/domain"
	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

func TestRepositoryWorkflowFirstPathIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openWorkflowTestPool(t)
	resetWorkflowTables(t, ctx, pool)
	repository := NewRepository(pool)

	prepared := prepareWorkflow(t, "wf-idem-1", "wf_test_1", "wfs_test_1")
	workflow, replayed, err := repository.CreateWorkflow(ctx, prepared)
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if replayed || workflow.Status != types.WorkflowStatusWaitingDecision {
		t.Fatalf("unexpected workflow create: replayed=%v %+v", replayed, workflow)
	}
	replayedWorkflow, replayed, err := repository.CreateWorkflow(ctx, prepared)
	if err != nil {
		t.Fatalf("replay workflow: %v", err)
	}
	if !replayed || replayedWorkflow.WorkflowID != workflow.WorkflowID {
		t.Fatalf("unexpected replay: replayed=%v %+v", replayed, replayedWorkflow)
	}
	conflict := prepareWorkflow(t, "wf-idem-1", "wf_conflict", "wfs_conflict")
	conflict.Command.TargetRefHash = "sha256:different"
	conflict.CommandHash = "sha256:different"
	if _, _, err := repository.CreateWorkflow(ctx, conflict); !errors.Is(err, types.ErrAlreadyExists) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}

	denied := prepareDecision(t, workflow.WorkflowID, workflow.CurrentStepID, "wfd_denied", "operator:requester", "decision-denied")
	if _, _, _, err := repository.RecordWorkflowDecision(ctx, denied); !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected separation-of-duty denial, got %v", err)
	}

	decisionPrepared := prepareDecision(t, workflow.WorkflowID, workflow.CurrentStepID, "wfd_test_1", "operator:approver", "decision-idem-1")
	approved, decision, replayed, err := repository.RecordWorkflowDecision(ctx, decisionPrepared)
	if err != nil {
		t.Fatalf("record decision: %v", err)
	}
	if replayed || approved.Status != types.WorkflowStatusApproved || decision.DecisionType != types.DecisionTypeApprove {
		t.Fatalf("unexpected decision result: replayed=%v workflow=%+v decision=%+v", replayed, approved, decision)
	}
	if _, _, _, err := repository.RecordWorkflowDecision(ctx, prepareDecision(t, workflow.WorkflowID, workflow.CurrentStepID, "wfd_late", "operator:other", "decision-late")); !errors.Is(err, types.ErrFailedPrecondition) {
		t.Fatalf("expected closed workflow to reject new decision, got %v", err)
	}
	loaded, decisions, err := repository.GetWorkflow(ctx, types.GetWorkflowCommand{
		AuthContext: types.AuthContext{TenantID: "tenant-workflow-test", ServiceName: "admin-service"},
		WorkflowID:  workflow.WorkflowID,
	})
	if err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	if loaded.Status != types.WorkflowStatusApproved || len(decisions) != 1 {
		t.Fatalf("unexpected get workflow result: %+v decisions=%d", loaded, len(decisions))
	}
	assertWorkflowOutboxLowSensitive(t, ctx, pool)
}

func TestRepositoryCreateAdminOperationWorkflowIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openWorkflowTestPool(t)
	resetWorkflowTables(t, ctx, pool)
	repository := NewRepository(pool)

	prepared := prepareWorkflow(t, "wf-admin-operation-idem-1", "wf_admin_operation_1", "wfs_admin_operation_1")
	prepared.Command.WorkflowType = types.WorkflowTypeAdminOperation
	prepared.Command.RiskLevel = types.RiskLevelCritical
	prepared.Command.RequesterService = "admin-service"
	prepared.Command.TargetService = "admin-service"
	prepared.Command.TargetOperation = "CONFIG_PUBLISH"
	prepared.Command.PayloadSchemaVersion = "admin.config_publish.v1"
	prepared.CommandHash = domain.HashRef("admin-operation-workflow")

	workflow, replayed, err := repository.CreateWorkflow(ctx, prepared)
	if err != nil {
		t.Fatalf("create admin operation workflow: %v", err)
	}
	if replayed || workflow.WorkflowType != types.WorkflowTypeAdminOperation ||
		workflow.RiskLevel != types.RiskLevelCritical ||
		workflow.TargetOperation != "CONFIG_PUBLISH" {
		t.Fatalf("unexpected admin operation workflow: replayed=%v %+v", replayed, workflow)
	}
}

func TestRepositoryCreateCompensationRequestWorkflowIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openWorkflowTestPool(t)
	resetWorkflowTables(t, ctx, pool)
	repository := NewRepository(pool)

	prepared := prepareWorkflow(t, "wf-compensation-idem-1", "wf_compensation_1", "wfs_compensation_1")
	prepared.Command.WorkflowType = types.WorkflowTypeCompensationRequest
	prepared.Command.RiskLevel = types.RiskLevelHigh
	prepared.Command.RequesterService = "admin-service"
	prepared.Command.TargetService = "control-plane-service"
	prepared.Command.TargetOperation = "CONFIG_ROLLBACK"
	prepared.Command.ApprovalPolicyRef = "admin.workflow.compensation.v1"
	prepared.Command.CompensationPolicyRef = "admin.compensation.control_plane.v1"
	prepared.Command.PayloadSchemaVersion = "admin.config_rollback.v1"
	prepared.CommandHash = domain.HashRef("compensation-request-workflow")

	workflow, replayed, err := repository.CreateWorkflow(ctx, prepared)
	if err != nil {
		t.Fatalf("create compensation workflow: %v", err)
	}
	if replayed || workflow.WorkflowType != types.WorkflowTypeCompensationRequest ||
		workflow.CompensationPolicyRef != "admin.compensation.control_plane.v1" ||
		workflow.Status != types.WorkflowStatusWaitingDecision {
		t.Fatalf("unexpected compensation workflow: replayed=%v %+v", replayed, workflow)
	}
}

func TestRepositoryRequestApprovedCompensationsIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openWorkflowTestPool(t)
	resetWorkflowTables(t, ctx, pool)
	repository := NewRepository(pool)

	prepared := prepareWorkflow(t, "wf-compensation-worker-idem-1", "wf_compensation_worker_1", "wfs_compensation_worker_1")
	prepared.Command.WorkflowType = types.WorkflowTypeCompensationRequest
	prepared.Command.RiskLevel = types.RiskLevelHigh
	prepared.Command.RequesterService = "admin-service"
	prepared.Command.TargetService = "control-plane-service"
	prepared.Command.TargetOperation = "CONFIG_ROLLBACK"
	prepared.Command.TargetRefHash = "sha256:admin-operation"
	prepared.Command.ApprovalPolicyRef = "admin.workflow.compensation.v1"
	prepared.Command.CompensationPolicyRef = "admin.compensation.control_plane.v1"
	prepared.Command.PayloadSchemaVersion = "admin.config_rollback.v1"
	prepared.Command.PayloadRefHash = "sha256:rollback-payload"
	prepared.Command.ReasonRef = "reason-sha256:compensation"
	prepared.CommandHash = domain.HashRef("compensation-worker-workflow")

	workflow, replayed, err := repository.CreateWorkflow(ctx, prepared)
	if err != nil {
		t.Fatalf("create compensation workflow: %v", err)
	}
	if replayed {
		t.Fatal("new compensation workflow should not replay")
	}
	decisionPrepared := prepareDecision(t, workflow.WorkflowID, workflow.CurrentStepID, "wfd_compensation_worker_1", "operator:approver", "decision-compensation-worker")
	if _, _, _, err := repository.RecordWorkflowDecision(ctx, decisionPrepared); err != nil {
		t.Fatalf("approve compensation workflow: %v", err)
	}

	compensations, err := repository.RequestApprovedCompensations(ctx, 10)
	if err != nil {
		t.Fatalf("request approved compensations: %v", err)
	}
	if len(compensations) != 1 {
		t.Fatalf("expected one compensation, got %d", len(compensations))
	}
	compensation := compensations[0]
	if compensation.CompensationID != "wfc_"+workflow.WorkflowID ||
		compensation.Status != types.WorkflowCompensationStatusRequested ||
		compensation.TargetService != "control-plane-service" ||
		compensation.TargetOperation != "CONFIG_ROLLBACK" ||
		compensation.TargetRefHash != "sha256:admin-operation" {
		t.Fatalf("unexpected compensation: %+v", compensation)
	}

	loaded, _, err := repository.GetWorkflow(ctx, types.GetWorkflowCommand{
		AuthContext: types.AuthContext{TenantID: "tenant-workflow-test", ServiceName: "admin-service"},
		WorkflowID:  workflow.WorkflowID,
	})
	if err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	if loaded.Status != types.WorkflowStatusCompensationPending || !loaded.CompletedAt.IsZero() {
		t.Fatalf("workflow should be compensation pending with empty completed_at: %+v", loaded)
	}

	assertWorkflowCompensationRequested(t, ctx, pool, workflow.WorkflowID, compensation.CompensationID)
	replayedCompensations, err := repository.RequestApprovedCompensations(ctx, 10)
	if err != nil {
		t.Fatalf("replay compensation worker: %v", err)
	}
	if len(replayedCompensations) != 0 {
		t.Fatalf("expected no compensations on replay, got %d", len(replayedCompensations))
	}
	assertWorkflowCompensationRequested(t, ctx, pool, workflow.WorkflowID, compensation.CompensationID)
}

func prepareWorkflow(t *testing.T, idempotencyKey string, workflowID string, stepID string) domain.PreparedWorkflow {
	t.Helper()
	prepared, err := domain.PrepareWorkflow(types.CreateWorkflowCommand{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-workflow-test",
			ServiceName: "agent-service",
		},
		RequesterRef:         "operator:requester",
		RequesterService:     "agent-service",
		WorkflowType:         types.WorkflowTypeActionApproval,
		RiskLevel:            types.RiskLevelHigh,
		TargetRefHash:        "sha256:target-action",
		TargetService:        "action-executor",
		TargetOperation:      "execute_tool",
		ApprovalPolicyRef:    "policy:approval/high",
		PayloadSchemaVersion: "action-approval.v1",
		PayloadRefHash:       "sha256:payload-ref",
		ReasonRef:            "reason:ticket-123",
		EvidenceRefs:         []string{"evidence:pack-123"},
		IdempotencyKey:       idempotencyKey,
		CorrelationID:        "corr-workflow-test",
		TraceID:              "trace-workflow-test",
	}, workflowID, stepID, time.Now().UTC())
	if err != nil {
		t.Fatalf("prepare workflow: %v", err)
	}
	return prepared
}

func prepareDecision(t *testing.T, workflowID string, stepID string, decisionID string, deciderRef string, idempotencyKey string) domain.PreparedDecision {
	t.Helper()
	prepared, err := domain.PrepareDecision(types.RecordWorkflowDecisionCommand{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-workflow-test",
			ServiceName: "admin-service",
		},
		WorkflowID:        workflowID,
		StepID:            stepID,
		DecisionType:      types.DecisionTypeApprove,
		DeciderRef:        deciderRef,
		DecisionPolicyRef: "policy:approval/high",
		ReasonRef:         "reason:approval-456",
		EvidenceRefs:      []string{"evidence:approval-456"},
		IdempotencyKey:    idempotencyKey,
		CorrelationID:     "corr-decision-test",
	}, decisionID, time.Now().UTC())
	if err != nil {
		t.Fatalf("prepare decision: %v", err)
	}
	return prepared
}

func openWorkflowTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is required for workflow postgres integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	t.Cleanup(pool.Close)
	applyWorkflowMigration(t, context.Background(), pool)
	return pool
}

func applyWorkflowMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	clearWorkflowTablesIfPresent(ctx, pool)
	pattern := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "workflow", "*.sql")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("find workflow migrations: %v", err)
	}
	if len(paths) == 0 {
		t.Fatalf("no workflow migrations matched %s", pattern)
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read workflow migration %s: %v", path, err)
		}
		if _, err := pool.Exec(ctx, string(content)); err != nil {
			t.Fatalf("apply workflow migration %s: %v", path, err)
		}
	}
}

func clearWorkflowTablesIfPresent(ctx context.Context, pool *pgxpool.Pool) {
	_, _ = pool.Exec(ctx, `
TRUNCATE
    workflow_outbox,
    workflow_compensations,
    workflow_timers,
    workflow_decisions,
    workflow_steps,
    workflow_requests
CASCADE
`)
}

func resetWorkflowTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
TRUNCATE
    workflow_outbox,
    workflow_compensations,
    workflow_timers,
    workflow_decisions,
    workflow_steps,
    workflow_requests
CASCADE
`)
	if err != nil {
		t.Fatalf("reset workflow tables: %v", err)
	}
}

func assertWorkflowOutboxLowSensitive(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT aggregate_id, partition_key, payload_json::text FROM workflow_outbox WHERE tenant_id = 'tenant-workflow-test' ORDER BY event_type`)
	if err != nil {
		t.Fatalf("query workflow outbox: %v", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
		var aggregateID string
		var partitionKey string
		var payload string
		if err := rows.Scan(&aggregateID, &partitionKey, &payload); err != nil {
			t.Fatalf("scan workflow outbox: %v", err)
		}
		for _, forbidden := range []string{
			"proposal body",
			"EvidencePack text",
			"tool input",
			"provider body",
			"secret",
			"token",
			"raw:",
		} {
			if strings.Contains(payload, forbidden) || strings.Contains(aggregateID, forbidden) || strings.Contains(partitionKey, forbidden) {
				t.Fatalf("workflow outbox leaked forbidden value %q: aggregate=%s partition=%s payload=%s", forbidden, aggregateID, partitionKey, payload)
			}
		}
		if !strings.Contains(payload, "sha256:") {
			t.Fatalf("workflow outbox payload missing hash refs: %s", payload)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("workflow outbox rows: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected two workflow outbox rows, got %d", count)
	}
}

func assertWorkflowCompensationRequested(t *testing.T, ctx context.Context, pool *pgxpool.Pool, workflowID string, compensationID string) {
	t.Helper()
	var compensationCount int
	var compensationStatus string
	if err := pool.QueryRow(ctx, `
SELECT count(*), COALESCE(max(status), '')
FROM workflow_compensations
WHERE tenant_id = 'tenant-workflow-test' AND workflow_id = $1 AND compensation_id = $2
`, workflowID, compensationID).Scan(&compensationCount, &compensationStatus); err != nil {
		t.Fatalf("query workflow compensation: %v", err)
	}
	if compensationCount != 1 || compensationStatus != types.WorkflowCompensationStatusRequested {
		t.Fatalf("unexpected compensation row count=%d status=%s", compensationCount, compensationStatus)
	}

	var outboxCount int
	var payload string
	if err := pool.QueryRow(ctx, `
SELECT count(*), COALESCE(max(payload_json::text), '')
FROM workflow_outbox
WHERE event_type = $1 AND workflow_id = $2
`, types.WorkflowEventCompensationRequested, workflowID).Scan(&outboxCount, &payload); err != nil {
		t.Fatalf("query workflow compensation outbox: %v", err)
	}
	if outboxCount != 1 {
		t.Fatalf("expected one compensation outbox row, got %d", outboxCount)
	}
	for _, want := range []string{compensationID, types.WorkflowStatusCompensationPending, types.WorkflowCompensationStatusRequested, "sha256:admin-operation"} {
		if !strings.Contains(payload, want) {
			t.Fatalf("compensation outbox payload missing %q: %s", want, payload)
		}
	}
	for _, forbidden := range []string{"secret", "token", "raw:", "rollback plaintext", "operator:approver"} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("compensation outbox leaked forbidden value %q: %s", forbidden, payload)
		}
	}
}
