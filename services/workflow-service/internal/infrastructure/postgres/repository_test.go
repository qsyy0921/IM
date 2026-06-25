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

func TestRepositoryListWorkflowsProviderReplayQueueIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openWorkflowTestPool(t)
	resetWorkflowTables(t, ctx, pool)
	repository := NewRepository(pool)

	providerReplay := prepareWorkflow(t, "wf-provider-replay-idem-1", "wf_provider_replay_1", "wfs_provider_replay_1")
	providerReplay.Command.WorkflowType = types.WorkflowTypeRepairApproval
	providerReplay.Command.RiskLevel = types.RiskLevelHigh
	providerReplay.Command.RequesterService = "admin-service"
	providerReplay.Command.TargetService = "action-executor"
	providerReplay.Command.TargetOperation = "PROVIDER_REPLAY_REQUEST"
	providerReplay.Command.TargetRefHash = "sha256:provider-failure"
	providerReplay.Command.ApprovalPolicyRef = "admin.workflow.provider_replay.v1"
	providerReplay.Command.PayloadSchemaVersion = "admin.provider_replay_request.v1"
	providerReplay.Command.PayloadRefHash = "sha256:provider-replay-payload"
	providerReplay.CommandHash = domain.HashRef("provider-replay-workflow")
	if _, _, err := repository.CreateWorkflow(ctx, providerReplay); err != nil {
		t.Fatalf("create provider replay workflow: %v", err)
	}

	compensation := prepareWorkflow(t, "wf-compensation-list-idem-1", "wf_compensation_list_1", "wfs_compensation_list_1")
	compensation.Command.WorkflowType = types.WorkflowTypeCompensationRequest
	compensation.Command.RiskLevel = types.RiskLevelHigh
	compensation.Command.RequesterService = "admin-service"
	compensation.Command.TargetService = "control-plane-service"
	compensation.Command.TargetOperation = "CONFIG_ROLLBACK"
	compensation.Command.ApprovalPolicyRef = "admin.workflow.compensation.v1"
	compensation.Command.PayloadSchemaVersion = "admin.config_rollback.v1"
	compensation.CommandHash = domain.HashRef("compensation-list-workflow")
	if _, _, err := repository.CreateWorkflow(ctx, compensation); err != nil {
		t.Fatalf("create compensation workflow: %v", err)
	}

	listed, err := repository.ListWorkflows(ctx, types.ListWorkflowsCommand{
		AuthContext:       types.AuthContext{TenantID: "tenant-workflow-test", ServiceName: "admin-service"},
		WorkflowType:      types.WorkflowTypeRepairApproval,
		Status:            types.WorkflowStatusWaitingDecision,
		TargetService:     "action-executor",
		TargetOperation:   "PROVIDER_REPLAY_REQUEST",
		ApprovalPolicyRef: "admin.workflow.provider_replay.v1",
		PageSize:          10,
	})
	if err != nil {
		t.Fatalf("list provider replay workflows: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected one provider replay workflow, got %d: %+v", len(listed), listed)
	}
	if listed[0].WorkflowID != "wf_provider_replay_1" ||
		listed[0].TargetService != "action-executor" ||
		listed[0].TargetOperation != "PROVIDER_REPLAY_REQUEST" ||
		listed[0].PayloadRefHash != "sha256:provider-replay-payload" {
		t.Fatalf("unexpected provider replay workflow: %+v", listed[0])
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
		compensation.TargetRefHash != "sha256:admin-operation" ||
		compensation.PayloadRefHash != "sha256:rollback-payload" ||
		compensation.CompensationPolicyRef != "admin.compensation.control_plane.v1" {
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

func TestRepositoryExecuteWorkflowCompensationIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openWorkflowTestPool(t)
	resetWorkflowTables(t, ctx, pool)
	repository := NewRepository(pool)

	workflow := createApprovedCompensationWorkflow(t, ctx, repository)
	requested, err := repository.RequestApprovedCompensations(ctx, 10)
	if err != nil {
		t.Fatalf("request approved compensation: %v", err)
	}
	if len(requested) != 1 {
		t.Fatalf("expected one requested compensation, got %d", len(requested))
	}
	claimed, err := repository.ClaimRequestedCompensations(ctx, 10, time.Minute)
	if err != nil {
		t.Fatalf("claim requested compensation: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected one claimed compensation, got %d", len(claimed))
	}
	if claimed[0].Status != types.WorkflowCompensationStatusExecuting {
		t.Fatalf("unexpected claimed status: %+v", claimed[0])
	}

	completed, err := repository.CompleteWorkflowCompensation(ctx, claimed[0], types.WorkflowCompensationExecutionResult{
		DownstreamService:    "control-plane-service",
		DownstreamRequestRef: "config-rollback:prod:API_GATEWAY_TENANT_QUOTA:tenant-a:v1",
		Status:               types.WorkflowCompensationStatusSucceeded,
	})
	if err != nil {
		t.Fatalf("complete workflow compensation: %v", err)
	}
	if completed.Status != types.WorkflowCompensationStatusSucceeded ||
		completed.DownstreamRequestRef != "config-rollback:prod:API_GATEWAY_TENANT_QUOTA:tenant-a:v1" ||
		completed.CompletedAt.IsZero() {
		t.Fatalf("unexpected completed compensation: %+v", completed)
	}
	loaded, _, err := repository.GetWorkflow(ctx, types.GetWorkflowCommand{
		AuthContext: types.AuthContext{TenantID: "tenant-workflow-test", ServiceName: "admin-service"},
		WorkflowID:  workflow.WorkflowID,
	})
	if err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	if loaded.Status != types.WorkflowStatusCompensated || loaded.CompletedAt.IsZero() {
		t.Fatalf("workflow should be compensated: %+v", loaded)
	}
	assertWorkflowCompensationResultOutbox(t, ctx, pool, workflow.WorkflowID, completed.CompensationID, types.WorkflowEventCompensationSucceeded)
}

func TestRepositoryWorkflowCompensationInstructionRegistryIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openWorkflowTestPool(t)
	resetWorkflowTables(t, ctx, pool)
	repository := NewRepository(pool)
	workflow := createApprovedCompensationWorkflow(t, ctx, repository)
	if _, err := repository.RequestApprovedCompensations(ctx, 10); err != nil {
		t.Fatalf("request approved compensation: %v", err)
	}

	instructions := []types.WorkflowCompensationInstruction{{
		TenantID:        "tenant-workflow-test",
		InstructionID:   "wfci_test_1",
		WorkflowID:      workflow.WorkflowID,
		PayloadRefHash:  "sha256:rollback-payload",
		TargetService:   "control-plane-service",
		TargetOperation: "CONFIG_ROLLBACK",
		InstructionType: types.WorkflowCompensationInstructionTypeControlPlaneRollback,
		Environment:     "local",
		ConfigKind:      "API_GATEWAY_TENANT_QUOTA",
		BundleKey:       "tenant-a",
		TargetVersion:   "v1",
		OperatorRef:     "operator:rollback",
		ReasonRef:       "reason-sha256:rollback",
		Status:          types.WorkflowCompensationInstructionStatusActive,
	}}
	if count, err := repository.UpsertWorkflowCompensationInstructions(ctx, instructions); err != nil || count != 1 {
		t.Fatalf("upsert compensation instruction: count=%d err=%v", count, err)
	}
	instructions[0].TargetVersion = "v2"
	if count, err := repository.UpsertWorkflowCompensationInstructions(ctx, instructions); err != nil || count != 1 {
		t.Fatalf("replay compensation instruction: count=%d err=%v", count, err)
	}

	resolved, ok, err := repository.ResolveControlPlaneRollbackInstruction(ctx, types.WorkflowCompensation{
		TenantID:        "tenant-workflow-test",
		WorkflowID:      workflow.WorkflowID,
		PayloadRefHash:  "sha256:rollback-payload",
		TargetService:   "control-plane-service",
		TargetOperation: "CONFIG_ROLLBACK",
	})
	if err != nil {
		t.Fatalf("resolve compensation instruction: %v", err)
	}
	if !ok || resolved.TargetVersion != "v2" || resolved.OperatorRef != "operator:rollback" {
		t.Fatalf("unexpected resolved instruction ok=%v %+v", ok, resolved)
	}
	if _, ok, err := repository.ResolveControlPlaneRollbackInstruction(ctx, types.WorkflowCompensation{
		TenantID:        "tenant-workflow-test",
		WorkflowID:      "wf_other",
		PayloadRefHash:  "sha256:rollback-payload",
		TargetService:   "control-plane-service",
		TargetOperation: "CONFIG_ROLLBACK",
	}); err != nil || ok {
		t.Fatalf("wrong workflow should not resolve ok=%v err=%v", ok, err)
	}

	instructions[0].Status = types.WorkflowCompensationInstructionStatusDisabled
	if _, err := repository.UpsertWorkflowCompensationInstructions(ctx, instructions); err != nil {
		t.Fatalf("disable compensation instruction: %v", err)
	}
	if _, ok, err := repository.ResolveControlPlaneRollbackInstruction(ctx, types.WorkflowCompensation{
		TenantID:        "tenant-workflow-test",
		WorkflowID:      workflow.WorkflowID,
		PayloadRefHash:  "sha256:rollback-payload",
		TargetService:   "control-plane-service",
		TargetOperation: "CONFIG_ROLLBACK",
	}); err != nil || ok {
		t.Fatalf("disabled instruction should not resolve ok=%v err=%v", ok, err)
	}
	instructions[0].Status = types.WorkflowCompensationInstructionStatusActive
	instructions[0].PayloadRefHash = "sha256:different"
	if _, err := repository.UpsertWorkflowCompensationInstructions(ctx, instructions); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected mismatched refs to fail, got %v", err)
	}
}

func TestRepositoryListWorkflowCompensationInstructionsIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openWorkflowTestPool(t)
	resetWorkflowTables(t, ctx, pool)
	repository := NewRepository(pool)
	workflow := createApprovedCompensationWorkflow(t, ctx, repository)
	if _, err := repository.RequestApprovedCompensations(ctx, 10); err != nil {
		t.Fatalf("request approved compensation: %v", err)
	}

	instruction := types.WorkflowCompensationInstruction{
		TenantID:        "tenant-workflow-test",
		InstructionID:   "wfci_list_1",
		WorkflowID:      workflow.WorkflowID,
		PayloadRefHash:  "sha256:rollback-payload",
		TargetService:   "control-plane-service",
		TargetOperation: "CONFIG_ROLLBACK",
		InstructionType: types.WorkflowCompensationInstructionTypeControlPlaneRollback,
		Environment:     "local",
		ConfigKind:      "API_GATEWAY_TENANT_QUOTA",
		BundleKey:       "tenant-a",
		TargetVersion:   "v1",
		OperatorRef:     "operator:rollback",
		ReasonRef:       "reason-sha256:rollback",
		Status:          types.WorkflowCompensationInstructionStatusActive,
	}
	if _, err := repository.UpsertWorkflowCompensationInstructions(ctx, []types.WorkflowCompensationInstruction{instruction}); err != nil {
		t.Fatalf("upsert active instruction: %v", err)
	}
	instruction.InstructionID = "wfci_list_2"
	instruction.Status = types.WorkflowCompensationInstructionStatusDisabled
	instruction.TargetVersion = "v2"
	if _, err := repository.UpsertWorkflowCompensationInstructions(ctx, []types.WorkflowCompensationInstruction{instruction}); err != nil {
		t.Fatalf("upsert disabled instruction: %v", err)
	}

	listed, err := repository.ListWorkflowCompensationInstructions(ctx, types.ListWorkflowCompensationInstructionsCommand{
		AuthContext: types.AuthContext{TenantID: "tenant-workflow-test", ServiceName: "admin-service"},
		WorkflowID:  workflow.WorkflowID,
		PageSize:    10,
	})
	if err != nil {
		t.Fatalf("list instructions: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("expected two instructions, got %d: %+v", len(listed), listed)
	}
	for _, got := range listed {
		if got.WorkflowID != workflow.WorkflowID ||
			got.PayloadRefHash != "sha256:rollback-payload" ||
			got.TargetService != "control-plane-service" ||
			got.OperatorRef != "operator:rollback" {
			t.Fatalf("unexpected instruction: %+v", got)
		}
		for _, forbidden := range []string{"secret", "token", "raw:", "rollback plaintext"} {
			if strings.Contains(got.PayloadRefHash, forbidden) ||
				strings.Contains(got.ReasonRef, forbidden) ||
				strings.Contains(got.BundleKey, forbidden) {
				t.Fatalf("instruction leaked forbidden value %q: %+v", forbidden, got)
			}
		}
	}

	activeOnly, err := repository.ListWorkflowCompensationInstructions(ctx, types.ListWorkflowCompensationInstructionsCommand{
		AuthContext: types.AuthContext{TenantID: "tenant-workflow-test", ServiceName: "admin-service"},
		WorkflowID:  workflow.WorkflowID,
		Status:      types.WorkflowCompensationInstructionStatusActive,
		PageSize:    10,
	})
	if err != nil {
		t.Fatalf("list active instructions: %v", err)
	}
	if len(activeOnly) != 1 || activeOnly[0].InstructionID != "wfci_list_1" {
		t.Fatalf("unexpected active-only list: %+v", activeOnly)
	}
	otherWorkflow, err := repository.ListWorkflowCompensationInstructions(ctx, types.ListWorkflowCompensationInstructionsCommand{
		AuthContext: types.AuthContext{TenantID: "tenant-workflow-test", ServiceName: "admin-service"},
		WorkflowID:  "wf_other",
		PageSize:    10,
	})
	if err != nil {
		t.Fatalf("list other workflow instructions: %v", err)
	}
	if len(otherWorkflow) != 0 {
		t.Fatalf("other workflow should not see instructions: %+v", otherWorkflow)
	}
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

func createApprovedCompensationWorkflow(t *testing.T, ctx context.Context, repository *Repository) types.Workflow {
	t.Helper()
	prepared := prepareWorkflow(t, "wf-compensation-execution-idem-1", "wf_compensation_execution_1", "wfs_compensation_execution_1")
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
	prepared.CommandHash = domain.HashRef("compensation-execution-workflow")

	workflow, replayed, err := repository.CreateWorkflow(ctx, prepared)
	if err != nil {
		t.Fatalf("create compensation workflow: %v", err)
	}
	if replayed {
		t.Fatal("new compensation workflow should not replay")
	}
	decisionPrepared := prepareDecision(t, workflow.WorkflowID, workflow.CurrentStepID, "wfd_compensation_execution_1", "operator:approver", "decision-compensation-execution")
	if _, _, _, err := repository.RecordWorkflowDecision(ctx, decisionPrepared); err != nil {
		t.Fatalf("approve compensation workflow: %v", err)
	}
	return workflow
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
DROP TABLE IF EXISTS
    workflow_outbox,
    workflow_compensation_instructions,
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
    workflow_compensation_instructions,
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

func assertWorkflowCompensationResultOutbox(t *testing.T, ctx context.Context, pool *pgxpool.Pool, workflowID string, compensationID string, eventType string) {
	t.Helper()
	var outboxCount int
	var payload string
	if err := pool.QueryRow(ctx, `
SELECT count(*), COALESCE(max(payload_json::text), '')
FROM workflow_outbox
WHERE event_type = $1 AND workflow_id = $2
`, eventType, workflowID).Scan(&outboxCount, &payload); err != nil {
		t.Fatalf("query workflow compensation result outbox: %v", err)
	}
	if outboxCount != 1 {
		t.Fatalf("expected one compensation result outbox row, got %d", outboxCount)
	}
	for _, want := range []string{compensationID, types.WorkflowStatusCompensated, types.WorkflowCompensationStatusSucceeded, "control-plane-service", "config-rollback:"} {
		if !strings.Contains(payload, want) {
			t.Fatalf("compensation result outbox payload missing %q: %s", want, payload)
		}
	}
	for _, forbidden := range []string{"secret", "token", "raw:", "rollback plaintext", "operator:approver"} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("compensation result outbox leaked forbidden value %q: %s", forbidden, payload)
		}
	}
}
