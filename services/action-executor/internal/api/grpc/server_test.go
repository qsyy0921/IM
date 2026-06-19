package grpc

import (
	"context"
	"testing"

	actionexecutorv1 "github.com/qsyy0921/IM/api/proto/nexusim/actionexecutor/v1"
	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	"github.com/qsyy0921/IM/services/action-executor/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestExecuteApprovedActionMapsRequestAndResponse(t *testing.T) {
	executor := &fakeExecutor{result: types.ExecuteApprovedActionResult{
		TenantID:          "tenant-1",
		UserID:            "user-1",
		ExecutionID:       "exec-1",
		ProposalID:        "proposal-1",
		ApprovalID:        "approval-1",
		PreparedAuditID:   "mcp-audit-1",
		SkillID:           "skill-1",
		ToolName:          "conversation.reply.send",
		Action:            types.ToolActionExecute,
		ResourceType:      "conversation",
		ResourceID:        "conv-1",
		RiskLevel:         "LOW",
		Status:            types.ExecutionStatusRecorded,
		Allowed:           true,
		RequiresApproval:  true,
		PermissionVersion: 7,
		Classification:    "TOOL_ALLOWED",
		Reason:            "allowed",
		DecisionSource:    "TOOL_RULE",
		Executed:          false,
		OutputJSON:        "{}",
		ResultID:          "result-1",
		ResultStatus:      types.ResultStatusNotExecuted,
		ResultRef:         "action-executor://executions/exec-1/results/result-1",
	}}
	server := NewServer(executor)
	response, err := server.ExecuteApprovedAction(context.Background(), &actionexecutorv1.ExecuteApprovedActionRequest{
		AuthContext: &actionexecutorv1.AuthContext{
			TenantId: "tenant-1",
			UserId:   "user-1",
			DeviceId: "device-1",
		},
		ProposalId:      "proposal-1",
		ApprovalId:      "approval-1",
		PreparedAuditId: "mcp-audit-1",
		SkillId:         "skill-1",
		ToolName:        "conversation.reply.send",
		Action:          policyv1.ToolAction_TOOL_ACTION_EXECUTE,
		InputJson:       `{}`,
	})
	if err != nil {
		t.Fatalf("execute approved action: %v", err)
	}
	if executor.command.Action != types.ToolActionExecute || executor.command.ApprovalID != "approval-1" {
		t.Fatalf("unexpected command: %+v", executor.command)
	}
	if response.GetStatus() != actionexecutorv1.ActionExecutionStatus_ACTION_EXECUTION_STATUS_RECORDED ||
		response.GetExecutionId() != "exec-1" ||
		response.GetResultId() != "result-1" ||
		response.GetResultStatus() != types.ResultStatusNotExecuted ||
		response.GetResultRef() == "" ||
		response.GetExecuted() {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestExecuteApprovedActionPermissionDeniedMapsToPermissionDenied(t *testing.T) {
	server := NewServer(&fakeExecutor{err: types.ErrToolPolicyDenied})
	_, err := server.ExecuteApprovedAction(context.Background(), &actionexecutorv1.ExecuteApprovedActionRequest{
		AuthContext: &actionexecutorv1.AuthContext{TenantId: "tenant-1", UserId: "user-1", DeviceId: "device-1"},
		Action:      policyv1.ToolAction_TOOL_ACTION_EXECUTE,
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got %v (%v)", status.Code(err), err)
	}
}

type fakeExecutor struct {
	command types.ExecuteApprovedActionCommand
	result  types.ExecuteApprovedActionResult
	err     error
}

func (executor *fakeExecutor) Execute(_ context.Context, command types.ExecuteApprovedActionCommand) (types.ExecuteApprovedActionResult, error) {
	executor.command = command
	if executor.err != nil {
		return types.ExecuteApprovedActionResult{}, executor.err
	}
	return executor.result, nil
}
