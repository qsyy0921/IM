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
	server := NewServer(nil, nil, nil, executor)
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
	server := NewServer(nil, nil, nil, &fakeListInstructionsExecutor{err: types.ErrPermissionDenied})
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
