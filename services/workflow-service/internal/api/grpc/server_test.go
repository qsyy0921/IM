package grpc

import (
	"context"
	"testing"
	"time"

	workflowv1 "github.com/qsyy0921/IM/api/proto/nexusim/workflow/v1"
	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestListWorkflowCompensationInstructions(t *testing.T) {
	executor := &fakeListInstructionsExecutor{instructions: []types.WorkflowCompensationInstruction{{
		TenantID:        "tenant-workflow-test",
		InstructionID:   "wfci_1",
		WorkflowID:      "wf_1",
		PayloadRefHash:  "sha256:payload",
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
		CreatedAt:       time.Unix(10, 0).UTC(),
		UpdatedAt:       time.Unix(20, 0).UTC(),
	}}}
	server := NewServer(nil, nil, nil, nil, nil, executor)
	response, err := server.ListWorkflowCompensationInstructions(context.Background(), &workflowv1.ListWorkflowCompensationInstructionsRequest{
		AuthContext: &workflowv1.AuthContext{TenantId: "tenant-workflow-test", ServiceName: "admin-service"},
		WorkflowId:  "wf_1",
		PageSize:    10,
	})
	if err != nil {
		t.Fatalf("list instructions: %v", err)
	}
	if executor.command.WorkflowID != "wf_1" || executor.command.PageSize != 10 {
		t.Fatalf("unexpected command: %+v", executor.command)
	}
	if len(response.GetInstructions()) != 1 {
		t.Fatalf("expected one instruction, got %d", len(response.GetInstructions()))
	}
	instruction := response.GetInstructions()[0]
	if instruction.GetInstructionId() != "wfci_1" ||
		instruction.GetPayloadRefHash() != "sha256:payload" ||
		instruction.GetTargetVersion() != "v1" ||
		instruction.GetCreatedAtUnixMs() == 0 ||
		instruction.GetUpdatedAtUnixMs() == 0 {
		t.Fatalf("unexpected instruction response: %+v", instruction)
	}
}

func TestListWorkflowCompensationInstructionsPermissionDenied(t *testing.T) {
	server := NewServer(nil, nil, nil, nil, nil, &fakeListInstructionsExecutor{err: types.ErrPermissionDenied})
	_, err := server.ListWorkflowCompensationInstructions(context.Background(), &workflowv1.ListWorkflowCompensationInstructionsRequest{
		AuthContext: &workflowv1.AuthContext{TenantId: "tenant-workflow-test", ServiceName: "admin-service"},
		WorkflowId:  "wf_1",
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected permission denied, got %v", err)
	}
}

type fakeListInstructionsExecutor struct {
	command      types.ListWorkflowCompensationInstructionsCommand
	instructions []types.WorkflowCompensationInstruction
	err          error
}

func (executor *fakeListInstructionsExecutor) Execute(_ context.Context, command types.ListWorkflowCompensationInstructionsCommand) ([]types.WorkflowCompensationInstruction, error) {
	executor.command = command
	if executor.err != nil {
		return nil, executor.err
	}
	return executor.instructions, nil
}

var _ ListWorkflowCompensationInstructionsExecutor = (*fakeListInstructionsExecutor)(nil)

func TestListWorkflowCompensations(t *testing.T) {
	executor := &fakeListCompensationsExecutor{compensations: []types.WorkflowCompensation{{
		TenantID:              "tenant-workflow-test",
		WorkflowID:            "wf_1",
		CompensationID:        "wfc_1",
		SourceStepID:          "wfs_1",
		TargetService:         "control-plane-service",
		TargetOperation:       "CONFIG_ROLLBACK",
		TargetRefHash:         "sha256:target",
		PayloadSchemaVersion:  "admin.config_rollback.v1",
		PayloadRefHash:        "sha256:payload",
		CompensationPolicyRef: "admin.compensation.control_plane.v1",
		ReasonRef:             "reason-sha256:rollback",
		DownstreamService:     "control-plane-service",
		DownstreamRequestRef:  "config-rollback:prod:API_GATEWAY_TENANT_QUOTA:tenant-a:v1",
		Status:                types.WorkflowCompensationStatusSucceeded,
		CreatedAt:             time.Unix(10, 0).UTC(),
		UpdatedAt:             time.Unix(20, 0).UTC(),
		CompletedAt:           time.Unix(30, 0).UTC(),
	}}}
	server := NewServer(nil, nil, nil, nil, executor, nil)
	response, err := server.ListWorkflowCompensations(context.Background(), &workflowv1.ListWorkflowCompensationsRequest{
		AuthContext: &workflowv1.AuthContext{TenantId: "tenant-workflow-test", ServiceName: "admin-service"},
		WorkflowId:  "wf_1",
		Status:      "SUCCEEDED",
		PageSize:    10,
	})
	if err != nil {
		t.Fatalf("list compensations: %v", err)
	}
	if executor.command.WorkflowID != "wf_1" || executor.command.Status != "SUCCEEDED" || executor.command.PageSize != 10 {
		t.Fatalf("unexpected command: %+v", executor.command)
	}
	if len(response.GetCompensations()) != 1 {
		t.Fatalf("expected one compensation, got %d", len(response.GetCompensations()))
	}
	compensation := response.GetCompensations()[0]
	if compensation.GetCompensationId() != "wfc_1" ||
		compensation.GetPayloadRefHash() != "sha256:payload" ||
		compensation.GetDownstreamRequestRef() != "config-rollback:prod:API_GATEWAY_TENANT_QUOTA:tenant-a:v1" ||
		compensation.GetStatus() != "SUCCEEDED" ||
		compensation.GetCompletedAtUnixMs() == 0 {
		t.Fatalf("unexpected compensation response: %+v", compensation)
	}
}

func TestListWorkflowCompensationsPermissionDenied(t *testing.T) {
	server := NewServer(nil, nil, nil, nil, &fakeListCompensationsExecutor{err: types.ErrPermissionDenied}, nil)
	_, err := server.ListWorkflowCompensations(context.Background(), &workflowv1.ListWorkflowCompensationsRequest{
		AuthContext: &workflowv1.AuthContext{TenantId: "tenant-workflow-test", ServiceName: "admin-service"},
		WorkflowId:  "wf_1",
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected permission denied, got %v", err)
	}
}

type fakeListCompensationsExecutor struct {
	command       types.ListWorkflowCompensationsCommand
	compensations []types.WorkflowCompensation
	err           error
}

func (executor *fakeListCompensationsExecutor) Execute(_ context.Context, command types.ListWorkflowCompensationsCommand) ([]types.WorkflowCompensation, error) {
	executor.command = command
	if executor.err != nil {
		return nil, executor.err
	}
	return executor.compensations, nil
}

var _ ListWorkflowCompensationsExecutor = (*fakeListCompensationsExecutor)(nil)

func TestListWorkflows(t *testing.T) {
	executor := &fakeListWorkflowsExecutor{workflows: []types.Workflow{{
		TenantID:             "tenant-workflow-test",
		WorkflowID:           "wf_provider_replay_1",
		WorkflowType:         types.WorkflowTypeRepairApproval,
		RiskLevel:            types.RiskLevelHigh,
		RequesterRef:         "admin-operation:provider-replay",
		RequesterService:     "admin-service",
		TargetService:        "action-executor",
		TargetOperation:      "PROVIDER_REPLAY_REQUEST",
		TargetRefHash:        "sha256:provider-failure",
		PayloadSchemaVersion: "admin.provider_replay_request.v1",
		PayloadRefHash:       "sha256:provider-replay-payload",
		ApprovalPolicyRef:    "admin.workflow.provider_replay.v1",
		ReasonRef:            "reason-sha256:provider-replay",
		Status:               types.WorkflowStatusWaitingDecision,
		CurrentStepID:        "wfs_provider_replay_1",
		CreatedAt:            time.Unix(10, 0).UTC(),
		UpdatedAt:            time.Unix(20, 0).UTC(),
	}}}
	server := NewServer(nil, nil, nil, executor, nil, nil)
	response, err := server.ListWorkflows(context.Background(), &workflowv1.ListWorkflowsRequest{
		AuthContext:       &workflowv1.AuthContext{TenantId: "tenant-workflow-test", ServiceName: "admin-service"},
		WorkflowType:      "REPAIR_APPROVAL",
		Status:            "WAITING_DECISION",
		TargetService:     "action-executor",
		TargetOperation:   "PROVIDER_REPLAY_REQUEST",
		ApprovalPolicyRef: "admin.workflow.provider_replay.v1",
		PageSize:          5,
	})
	if err != nil {
		t.Fatalf("list workflows: %v", err)
	}
	if executor.command.WorkflowType != "REPAIR_APPROVAL" ||
		executor.command.TargetService != "action-executor" ||
		executor.command.TargetOperation != "PROVIDER_REPLAY_REQUEST" ||
		executor.command.ApprovalPolicyRef != "admin.workflow.provider_replay.v1" ||
		executor.command.PageSize != 5 {
		t.Fatalf("unexpected command: %+v", executor.command)
	}
	if len(response.GetWorkflows()) != 1 {
		t.Fatalf("expected one workflow, got %d", len(response.GetWorkflows()))
	}
	workflow := response.GetWorkflows()[0]
	if workflow.GetWorkflowId() != "wf_provider_replay_1" ||
		workflow.GetTargetService() != "action-executor" ||
		workflow.GetPayloadRefHash() != "sha256:provider-replay-payload" ||
		workflow.GetCreatedAtUnixMs() == 0 {
		t.Fatalf("unexpected workflow response: %+v", workflow)
	}
}

func TestListWorkflowsPermissionDenied(t *testing.T) {
	server := NewServer(nil, nil, nil, &fakeListWorkflowsExecutor{err: types.ErrPermissionDenied}, nil, nil)
	_, err := server.ListWorkflows(context.Background(), &workflowv1.ListWorkflowsRequest{
		AuthContext: &workflowv1.AuthContext{TenantId: "tenant-workflow-test", ServiceName: "admin-service"},
		PageSize:    5,
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected permission denied, got %v", err)
	}
}

type fakeListWorkflowsExecutor struct {
	command   types.ListWorkflowsCommand
	workflows []types.Workflow
	err       error
}

func (executor *fakeListWorkflowsExecutor) Execute(_ context.Context, command types.ListWorkflowsCommand) ([]types.Workflow, error) {
	executor.command = command
	if executor.err != nil {
		return nil, executor.err
	}
	return executor.workflows, nil
}

var _ ListWorkflowsExecutor = (*fakeListWorkflowsExecutor)(nil)
