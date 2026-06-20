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
	path := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "workflow", "000001_workflow_core.sql")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read workflow migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(content)); err != nil {
		t.Fatalf("apply workflow migration: %v", err)
	}
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
