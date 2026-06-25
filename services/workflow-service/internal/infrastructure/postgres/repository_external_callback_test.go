package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/workflow-service/internal/domain"
	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

func TestRepositoryExternalCallbackDeliveryLifecycleIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openWorkflowTestPool(t)
	resetWorkflowTables(t, ctx, pool)
	repository := NewRepository(pool)

	workflow := createExternalCallbackWorkflow(t, ctx, repository, "wf_external_callback_1", "wfs_external_callback_1")
	delivery := externalCallbackDeliveryForWorkflow(workflow, "wfecd_lifecycle_1")
	registered, replayed, err := repository.RegisterExternalCallbackDelivery(ctx, delivery)
	if err != nil {
		t.Fatalf("register external callback delivery: %v", err)
	}
	if replayed || registered.Status != types.WorkflowExternalCallbackDeliveryStatusPending {
		t.Fatalf("unexpected registered delivery replayed=%v %+v", replayed, registered)
	}
	replayedDelivery, replayed, err := repository.RegisterExternalCallbackDelivery(ctx, delivery)
	if err != nil {
		t.Fatalf("replay external callback delivery: %v", err)
	}
	if !replayed || replayedDelivery.DeliveryID != delivery.DeliveryID {
		t.Fatalf("unexpected replay replayed=%v %+v", replayed, replayedDelivery)
	}

	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	claimed, err := repository.ClaimReadyExternalCallbackDeliveries(ctx, now, 10, time.Minute)
	if err != nil {
		t.Fatalf("claim external callback delivery: %v", err)
	}
	if len(claimed) != 1 || claimed[0].DeliveryID != delivery.DeliveryID ||
		claimed[0].Status != types.WorkflowExternalCallbackDeliveryStatusInFlight ||
		claimed[0].AttemptCount != 1 {
		t.Fatalf("unexpected claimed deliveries: %+v", claimed)
	}
	completed, err := repository.MarkExternalCallbackDeliveryDelivered(ctx, claimed[0], types.WorkflowExternalCallbackDeliveryResult{
		DeliveryResultRef: "provider-status:202",
	})
	if err != nil {
		t.Fatalf("mark delivered: %v", err)
	}
	if completed.Status != types.WorkflowExternalCallbackDeliveryStatusDelivered || completed.DeliveredAt.IsZero() {
		t.Fatalf("unexpected delivered state: %+v", completed)
	}
	assertExternalCallbackOutbox(t, ctx, pool, delivery.DeliveryID, types.WorkflowEventExternalCallbackDelivered)
}

func TestRepositoryExternalCallbackDeliveryRetryAndDLQIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openWorkflowTestPool(t)
	resetWorkflowTables(t, ctx, pool)
	repository := NewRepository(pool)

	workflow := createExternalCallbackWorkflow(t, ctx, repository, "wf_external_callback_retry", "wfs_external_callback_retry")
	delivery := externalCallbackDeliveryForWorkflow(workflow, "wfecd_retry_1")
	delivery.MaxAttempts = 2
	if _, _, err := repository.RegisterExternalCallbackDelivery(ctx, delivery); err != nil {
		t.Fatalf("register external callback delivery: %v", err)
	}

	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	claimed, err := repository.ClaimReadyExternalCallbackDeliveries(ctx, now, 10, time.Minute)
	if err != nil {
		t.Fatalf("claim first attempt: %v", err)
	}
	retryAt := now.Add(time.Minute)
	retryPending, err := repository.MarkExternalCallbackDeliveryFailed(ctx, claimed[0], types.WorkflowExternalCallbackDeliveryResult{
		FailureClass: "provider_unavailable",
	}, retryAt)
	if err != nil {
		t.Fatalf("mark retry pending: %v", err)
	}
	if retryPending.Status != types.WorkflowExternalCallbackDeliveryStatusRetryPending ||
		retryPending.LastFailureClass != "provider_unavailable" ||
		!retryPending.AvailableAt.Equal(retryAt) {
		t.Fatalf("unexpected retry pending state: %+v", retryPending)
	}

	claimed, err = repository.ClaimReadyExternalCallbackDeliveries(ctx, retryAt.Add(time.Second), 10, time.Minute)
	if err != nil {
		t.Fatalf("claim second attempt: %v", err)
	}
	if len(claimed) != 1 || claimed[0].AttemptCount != 2 {
		t.Fatalf("unexpected second claim: %+v", claimed)
	}
	dlq, err := repository.MarkExternalCallbackDeliveryFailed(ctx, claimed[0], types.WorkflowExternalCallbackDeliveryResult{
		FailureClass: "provider_unavailable",
	}, retryAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("mark dlq: %v", err)
	}
	if dlq.Status != types.WorkflowExternalCallbackDeliveryStatusDLQ {
		t.Fatalf("expected DLQ, got %+v", dlq)
	}
	assertExternalCallbackOutbox(t, ctx, pool, delivery.DeliveryID, types.WorkflowEventExternalCallbackDLQ)
}

func TestRepositoryExternalCallbackDeliveryRejectsClosedWorkflowIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openWorkflowTestPool(t)
	resetWorkflowTables(t, ctx, pool)
	repository := NewRepository(pool)

	workflow := createExternalCallbackWorkflow(t, ctx, repository, "wf_external_callback_closed", "wfs_external_callback_closed")
	decision := prepareDecision(t, workflow.WorkflowID, workflow.CurrentStepID, "wfd_external_callback_closed", "operator:approver", "decision-external-callback-closed")
	if _, _, _, err := repository.RecordWorkflowDecision(ctx, decision); err != nil {
		t.Fatalf("approve workflow: %v", err)
	}
	delivery := externalCallbackDeliveryForWorkflow(workflow, "wfecd_closed_1")
	if _, _, err := repository.RegisterExternalCallbackDelivery(ctx, delivery); !errors.Is(err, types.ErrFailedPrecondition) {
		t.Fatalf("expected closed workflow to fail callback delivery registration, got %v", err)
	}
}

func createExternalCallbackWorkflow(
	t *testing.T,
	ctx context.Context,
	repository *Repository,
	workflowID string,
	stepID string,
) types.Workflow {
	t.Helper()
	prepared := prepareWorkflow(t, "idem-"+workflowID, workflowID, stepID)
	prepared.Command.WorkflowType = types.WorkflowTypeRepairApproval
	prepared.Command.RequesterService = "admin-service"
	prepared.Command.TargetService = "action-executor"
	prepared.Command.TargetOperation = "PROVIDER_REPLAY_REQUEST"
	prepared.Command.TargetRefHash = "sha256:provider-replay-target"
	prepared.Command.PayloadSchemaVersion = "admin.provider_replay_request.v1"
	prepared.Command.PayloadRefHash = "sha256:provider-replay-payload"
	prepared.Command.ApprovalPolicyRef = "admin.workflow.provider_replay.v1"
	prepared.CommandHash = domain.HashRef("external-callback-" + workflowID)
	workflow, replayed, err := repository.CreateWorkflow(ctx, prepared)
	if err != nil {
		t.Fatalf("create external callback workflow: %v", err)
	}
	if replayed {
		t.Fatal("external callback workflow should not replay")
	}
	return workflow
}

func externalCallbackDeliveryForWorkflow(
	workflow types.Workflow,
	deliveryID string,
) types.WorkflowExternalCallbackDelivery {
	return types.WorkflowExternalCallbackDelivery{
		TenantID:                     workflow.TenantID,
		WorkflowID:                   workflow.WorkflowID,
		DeliveryID:                   deliveryID,
		DeliveryPlanSha256:           "sha256:delivery-plan-" + deliveryID,
		SourceDecisionManifestSha256: "sha256:decision-manifest-" + deliveryID,
		StepID:                       workflow.CurrentStepID,
		WorkflowType:                 workflow.WorkflowType,
		TargetService:                workflow.TargetService,
		TargetOperation:              workflow.TargetOperation,
		TargetRefHash:                workflow.TargetRefHash,
		PayloadSchemaVersion:         workflow.PayloadSchemaVersion,
		PayloadRefHash:               workflow.PayloadRefHash,
		ApprovalPolicyRef:            workflow.ApprovalPolicyRef,
		DecisionPolicyRef:            "workflow.external-decision.v1",
		CallbackProviderRef:          "callback-provider:ops",
		CallbackEndpointRef:          "callback-endpoint:ops-approval",
		DeliveryQueueRef:             "callback-queue:ops-approval",
		RetryPolicyRef:               "retry-policy:external-callback",
		BackoffPolicyRef:             "backoff-policy:external-callback",
		CallbackTimeoutPolicyRef:     "timeout-policy:external-callback",
		CallbackPayloadSchemaVersion: "nexusim.workflow.external_decision_manifest.v1",
		CallbackPayloadRefHash:       "sha256:callback-payload-" + deliveryID,
		Status:                       types.WorkflowExternalCallbackDeliveryStatusPending,
		MaxAttempts:                  3,
	}
}

func assertExternalCallbackOutbox(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	deliveryID string,
	eventType string,
) {
	t.Helper()
	var count int
	var payload string
	if err := pool.QueryRow(ctx, `
SELECT count(*), COALESCE(max(payload_json::text), '')
FROM workflow_outbox
WHERE event_type = $1 AND payload_json->>'delivery_id' = $2
`, eventType, deliveryID).Scan(&count, &payload); err != nil {
		t.Fatalf("query external callback outbox: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one external callback outbox row, got %d payload=%s", count, payload)
	}
	for _, want := range []string{deliveryID, "sha256:", "callback_provider_ref", "callback_endpoint_ref"} {
		if !strings.Contains(payload, want) {
			t.Fatalf("external callback outbox missing %q: %s", want, payload)
		}
	}
	for _, forbidden := range []string{"http://", "https://", "provider body", "raw:", "secret", "token"} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("external callback outbox leaked forbidden value %q: %s", forbidden, payload)
		}
	}
}
