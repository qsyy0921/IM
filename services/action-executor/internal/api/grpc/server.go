package grpc

import (
	"context"
	"errors"

	actionexecutorv1 "github.com/qsyy0921/IM/api/proto/nexusim/actionexecutor/v1"
	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	"github.com/qsyy0921/IM/services/action-executor/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ExecuteApprovedActionExecutor interface {
	Execute(context.Context, types.ExecuteApprovedActionCommand) (types.ExecuteApprovedActionResult, error)
}

type Server struct {
	actionexecutorv1.UnimplementedActionExecutorServiceServer
	executeApprovedAction ExecuteApprovedActionExecutor
}

func NewServer(executeApprovedAction ExecuteApprovedActionExecutor) *Server {
	return &Server{executeApprovedAction: executeApprovedAction}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	actionexecutorv1.RegisterActionExecutorServiceServer(registrar, server)
}

func (server *Server) ExecuteApprovedAction(
	ctx context.Context,
	request *actionexecutorv1.ExecuteApprovedActionRequest,
) (*actionexecutorv1.ExecuteApprovedActionResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.executeApprovedAction.Execute(ctx, types.ExecuteApprovedActionCommand{
		AuthContext:     auth,
		ProposalID:      request.GetProposalId(),
		ApprovalID:      request.GetApprovalId(),
		PreparedAuditID: request.GetPreparedAuditId(),
		SkillID:         request.GetSkillId(),
		ToolName:        request.GetToolName(),
		Action:          toolActionFromProto(request.GetAction()),
		ResourceType:    request.GetResourceType(),
		ResourceID:      request.GetResourceId(),
		RiskLevel:       request.GetRiskLevel(),
		Intent:          request.GetIntent(),
		InputJSON:       request.GetInputJson(),
		IdempotencyKey:  request.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return resultToProto(result), nil
}

func authFromProto(ctx context.Context, auth *actionexecutorv1.AuthContext) (types.AuthContext, bool) {
	if verified, ok := verifiedAuthFromContext(ctx); ok {
		if auth != nil {
			if verified.TraceID == "" {
				verified.TraceID = auth.GetTraceId()
			}
			if verified.RequestID == "" {
				verified.RequestID = auth.GetRequestId()
			}
		}
		return verified, true
	}
	if auth == nil {
		return types.AuthContext{}, false
	}
	return types.AuthContext{
		TenantID:  types.TenantID(auth.GetTenantId()),
		UserID:    types.UserID(auth.GetUserId()),
		DeviceID:  auth.GetDeviceId(),
		SessionID: auth.GetSessionId(),
		TraceID:   auth.GetTraceId(),
		RequestID: auth.GetRequestId(),
	}, true
}

func resultToProto(result types.ExecuteApprovedActionResult) *actionexecutorv1.ExecuteApprovedActionResponse {
	return &actionexecutorv1.ExecuteApprovedActionResponse{
		TenantId:          string(result.TenantID),
		UserId:            string(result.UserID),
		ExecutionId:       result.ExecutionID,
		ProposalId:        result.ProposalID,
		ApprovalId:        result.ApprovalID,
		PreparedAuditId:   result.PreparedAuditID,
		SkillId:           result.SkillID,
		ToolName:          result.ToolName,
		Action:            toolActionToProto(result.Action),
		ResourceType:      result.ResourceType,
		ResourceId:        result.ResourceID,
		RiskLevel:         result.RiskLevel,
		Status:            executionStatusToProto(result.Status),
		Allowed:           result.Allowed,
		RequiresApproval:  result.RequiresApproval,
		PermissionVersion: result.PermissionVersion,
		Classification:    result.Classification,
		Reason:            result.Reason,
		DecisionSource:    result.DecisionSource,
		Executed:          result.Executed,
		OutputJson:        result.OutputJSON,
	}
}

func toolActionFromProto(action policyv1.ToolAction) string {
	switch action {
	case policyv1.ToolAction_TOOL_ACTION_CALL:
		return types.ToolActionCall
	case policyv1.ToolAction_TOOL_ACTION_APPROVE:
		return types.ToolActionApprove
	case policyv1.ToolAction_TOOL_ACTION_EXECUTE:
		return types.ToolActionExecute
	default:
		return ""
	}
}

func toolActionToProto(action string) policyv1.ToolAction {
	switch action {
	case types.ToolActionCall:
		return policyv1.ToolAction_TOOL_ACTION_CALL
	case types.ToolActionApprove:
		return policyv1.ToolAction_TOOL_ACTION_APPROVE
	case types.ToolActionExecute:
		return policyv1.ToolAction_TOOL_ACTION_EXECUTE
	default:
		return policyv1.ToolAction_TOOL_ACTION_UNSPECIFIED
	}
}

func executionStatusToProto(status string) actionexecutorv1.ActionExecutionStatus {
	switch status {
	case types.ExecutionStatusRecorded:
		return actionexecutorv1.ActionExecutionStatus_ACTION_EXECUTION_STATUS_RECORDED
	case types.ExecutionStatusBlocked:
		return actionexecutorv1.ActionExecutionStatus_ACTION_EXECUTION_STATUS_BLOCKED
	case types.ExecutionStatusFailed:
		return actionexecutorv1.ActionExecutionStatus_ACTION_EXECUTION_STATUS_FAILED
	default:
		return actionexecutorv1.ActionExecutionStatus_ACTION_EXECUTION_STATUS_UNSPECIFIED
	}
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, types.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, "invalid argument")
	case errors.Is(err, types.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	case errors.Is(err, types.ErrSkillNotFound):
		return status.Error(codes.NotFound, "skill not found")
	case errors.Is(err, types.ErrSkillDisabled):
		return status.Error(codes.FailedPrecondition, "skill disabled")
	case errors.Is(err, types.ErrToolActionNotAllowed):
		return status.Error(codes.FailedPrecondition, "tool action not allowed")
	case errors.Is(err, types.ErrSkillCatalogUnavailable):
		return status.Error(codes.Unavailable, "skill catalog unavailable")
	case errors.Is(err, types.ErrToolPolicyUnavailable):
		return status.Error(codes.Unavailable, "tool policy unavailable")
	case errors.Is(err, types.ErrToolPolicyDenied):
		return status.Error(codes.PermissionDenied, "tool policy denied")
	case errors.Is(err, types.ErrProposalApprovalUnavailable):
		return status.Error(codes.Unavailable, "proposal approval unavailable")
	case errors.Is(err, types.ErrProposalNotApproved):
		return status.Error(codes.FailedPrecondition, "proposal not approved")
	case errors.Is(err, types.ErrProposalMismatch):
		return status.Error(codes.FailedPrecondition, "proposal mismatch")
	case errors.Is(err, types.ErrExecutionAuditFailed):
		return status.Error(codes.Unavailable, "action execution audit unavailable")
	default:
		return status.Error(codes.Internal, "action executor internal error")
	}
}
