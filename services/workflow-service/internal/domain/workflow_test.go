package domain

import (
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

func TestPrepareWorkflowRejectsSensitiveRefs(t *testing.T) {
	_, err := PrepareWorkflow(validCreateCommand(func(command *types.CreateWorkflowCommand) {
		command.PayloadRefHash = "raw:proposal body should not be accepted"
	}), "wf_1", "wfs_1", time.Now())
	if err == nil {
		t.Fatal("expected sensitive payload ref to fail")
	}
}

func TestPrepareWorkflowNormalizesAndBuildsWaitingDecision(t *testing.T) {
	prepared, err := PrepareWorkflow(validCreateCommand(nil), "wf_1", "wfs_1", time.Now())
	if err != nil {
		t.Fatalf("prepare workflow: %v", err)
	}
	workflow := WorkflowFromPrepared(prepared)
	if workflow.Status != types.WorkflowStatusWaitingDecision || workflow.CurrentStepID != "wfs_1" {
		t.Fatalf("unexpected workflow: %+v", workflow)
	}
	step := StepFromPrepared(prepared)
	if step.StepType != types.WorkflowStepTypeApproval || step.Status != types.WorkflowStepStatusReady {
		t.Fatalf("unexpected first step: %+v", step)
	}
}

func TestPrepareWorkflowAllowsAdminOperationType(t *testing.T) {
	prepared, err := PrepareWorkflow(validCreateCommand(func(command *types.CreateWorkflowCommand) {
		command.RequesterService = "admin-service"
		command.WorkflowType = types.WorkflowTypeAdminOperation
		command.RiskLevel = types.RiskLevelCritical
		command.TargetService = "admin-service"
		command.TargetOperation = "CONFIG_PUBLISH"
		command.PayloadSchemaVersion = "admin.config_publish.v1"
	}), "wf_admin_1", "wfs_admin_1", time.Now())
	if err != nil {
		t.Fatalf("prepare admin operation workflow: %v", err)
	}
	workflow := WorkflowFromPrepared(prepared)
	if workflow.WorkflowType != types.WorkflowTypeAdminOperation ||
		workflow.RiskLevel != types.RiskLevelCritical ||
		workflow.TargetOperation != "CONFIG_PUBLISH" {
		t.Fatalf("unexpected admin workflow: %+v", workflow)
	}
}

func TestPrepareWorkflowAllowsCompensationRequestType(t *testing.T) {
	prepared, err := PrepareWorkflow(validCreateCommand(func(command *types.CreateWorkflowCommand) {
		command.RequesterService = "admin-service"
		command.WorkflowType = types.WorkflowTypeCompensationRequest
		command.RiskLevel = types.RiskLevelHigh
		command.TargetService = "control-plane-service"
		command.TargetOperation = "CONFIG_ROLLBACK"
		command.ApprovalPolicyRef = "admin.workflow.compensation.v1"
		command.CompensationPolicyRef = "admin.compensation.control_plane.v1"
		command.PayloadSchemaVersion = "admin.config_rollback.v1"
	}), "wf_comp_1", "wfs_comp_1", time.Now())
	if err != nil {
		t.Fatalf("prepare compensation workflow: %v", err)
	}
	workflow := WorkflowFromPrepared(prepared)
	if workflow.WorkflowType != types.WorkflowTypeCompensationRequest ||
		workflow.CompensationPolicyRef != "admin.compensation.control_plane.v1" ||
		workflow.TargetOperation != "CONFIG_ROLLBACK" {
		t.Fatalf("unexpected compensation workflow: %+v", workflow)
	}
}

func TestStatusAfterDecision(t *testing.T) {
	status, terminal := StatusAfterDecision(types.DecisionTypeApprove)
	if status != types.WorkflowStatusApproved || !terminal {
		t.Fatalf("approve should be terminal approved, got %s %v", status, terminal)
	}
	status, terminal = StatusAfterDecision(types.DecisionTypeRequestChanges)
	if status != types.WorkflowStatusWaitingDecision || terminal {
		t.Fatalf("request changes should remain waiting, got %s %v", status, terminal)
	}
}

func validCreateCommand(mutator func(*types.CreateWorkflowCommand)) types.CreateWorkflowCommand {
	command := types.CreateWorkflowCommand{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-workflow-test",
			ServiceName: "agent-service",
		},
		RequesterRef:         "service:agent-service",
		RequesterService:     "agent-service",
		WorkflowType:         types.WorkflowTypeActionApproval,
		RiskLevel:            types.RiskLevelHigh,
		TargetRefHash:        "sha256:target",
		TargetService:        "action-executor",
		TargetOperation:      "execute_tool",
		ApprovalPolicyRef:    "policy:approval/high",
		PayloadSchemaVersion: "action-approval.v1",
		PayloadRefHash:       "sha256:payload",
		ReasonRef:            "reason:approval-1",
		EvidenceRefs:         []string{"evidence:pack-1"},
		IdempotencyKey:       "idem-create",
	}
	if mutator != nil {
		mutator(&command)
	}
	return command
}
