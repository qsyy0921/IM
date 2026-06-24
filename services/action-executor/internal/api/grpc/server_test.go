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
	server := NewServer(executor, nil)
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
	server := NewServer(&fakeExecutor{err: types.ErrToolPolicyDenied}, nil)
	_, err := server.ExecuteApprovedAction(context.Background(), &actionexecutorv1.ExecuteApprovedActionRequest{
		AuthContext: &actionexecutorv1.AuthContext{TenantId: "tenant-1", UserId: "user-1", DeviceId: "device-1"},
		Action:      policyv1.ToolAction_TOOL_ACTION_EXECUTE,
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got %v (%v)", status.Code(err), err)
	}
}

func TestRedriveProviderFailureMapsRequestAndResponse(t *testing.T) {
	redrive := &fakeRedriveExecutor{result: types.RedriveProviderFailureResult{
		TenantID:          "tenant-1",
		UserID:            "user-1",
		ProviderFailureID: "provider-failure-1",
		SourceExecutionID: "exec-source-1",
		SourceResultID:    "result-source-1",
		RedriveResult: types.ExecuteApprovedActionResult{
			ExecutionID:     "exec-redrive-1",
			ResultID:        "result-redrive-1",
			ProposalID:      "proposal-redrive-1",
			ApprovalID:      "approval-redrive-1",
			PreparedAuditID: "mcp-audit-redrive-1",
			SkillID:         "skill-1",
			ToolName:        "conversation.reply.send",
			ResourceType:    "conversation",
			ResourceID:      "conv-1",
			Status:          types.ExecutionStatusRecorded,
			ResultStatus:    types.ResultStatusSucceeded,
			Executed:        true,
			Classification:  "TOOL_ALLOWED",
			Reason:          "approved",
			ResultRef:       "action-executor://executions/exec-redrive-1/results/result-redrive-1",
		},
	}}
	server := NewServer(&fakeExecutor{}, redrive)
	response, err := server.RedriveProviderFailure(context.Background(), &actionexecutorv1.RedriveProviderFailureRequest{
		AuthContext:       &actionexecutorv1.AuthContext{TenantId: "tenant-1", UserId: "user-1", DeviceId: "device-1"},
		ProviderFailureId: "provider-failure-1",
		ReasonSha256:      "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ProposalId:        "proposal-redrive-1",
		ApprovalId:        "approval-redrive-1",
		PreparedAuditId:   "mcp-audit-redrive-1",
		SkillId:           "skill-1",
		ToolName:          "conversation.reply.send",
		Action:            policyv1.ToolAction_TOOL_ACTION_EXECUTE,
		ResourceType:      "conversation",
		ResourceId:        "conv-1",
		RiskLevel:         "LOW",
		Intent:            "operator redrive",
		InputJson:         `{"body":"redrive"}`,
		IdempotencyKey:    "idem-redrive-1",
	})
	if err != nil {
		t.Fatalf("redrive provider failure: %v", err)
	}
	if redrive.command.ProviderFailureID != "provider-failure-1" ||
		redrive.command.Action != types.ToolActionExecute ||
		redrive.command.ReasonSHA256 == "" {
		t.Fatalf("unexpected redrive command: %+v", redrive.command)
	}
	if response.GetProviderFailureId() != "provider-failure-1" ||
		response.GetSourceExecutionId() != "exec-source-1" ||
		response.GetRedriveExecutionId() != "exec-redrive-1" ||
		!response.GetExecuted() ||
		response.GetStatus() != actionexecutorv1.ActionExecutionStatus_ACTION_EXECUTION_STATUS_RECORDED {
		t.Fatalf("unexpected redrive response: %+v", response)
	}
}

func TestRedriveProviderFailureNotRedrivableMapsToFailedPrecondition(t *testing.T) {
	server := NewServer(&fakeExecutor{}, &fakeRedriveExecutor{err: types.ErrProviderFailureNotRedrivable})
	_, err := server.RedriveProviderFailure(context.Background(), &actionexecutorv1.RedriveProviderFailureRequest{
		AuthContext: &actionexecutorv1.AuthContext{TenantId: "tenant-1", UserId: "user-1", DeviceId: "device-1"},
		Action:      policyv1.ToolAction_TOOL_ACTION_EXECUTE,
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %v (%v)", status.Code(err), err)
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

type fakeRedriveExecutor struct {
	command types.RedriveProviderFailureCommand
	result  types.RedriveProviderFailureResult
	err     error
}

func (executor *fakeRedriveExecutor) Execute(
	_ context.Context,
	command types.RedriveProviderFailureCommand,
) (types.RedriveProviderFailureResult, error) {
	executor.command = command
	if executor.err != nil {
		return types.RedriveProviderFailureResult{}, executor.err
	}
	return executor.result, nil
}
