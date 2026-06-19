package grpc

import (
	"context"
	"errors"

	mcpgatewayv1 "github.com/qsyy0921/IM/api/proto/nexusim/mcpgateway/v1"
	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	"github.com/qsyy0921/IM/services/mcp-gateway/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PrepareToolCallExecutor interface {
	Execute(context.Context, types.PrepareToolCallCommand) (types.PrepareToolCallResult, error)
}

type Server struct {
	mcpgatewayv1.UnimplementedMCPGatewayServiceServer
	prepareToolCall PrepareToolCallExecutor
}

func NewServer(prepareToolCall PrepareToolCallExecutor) *Server {
	return &Server{prepareToolCall: prepareToolCall}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	mcpgatewayv1.RegisterMCPGatewayServiceServer(registrar, server)
}

func (server *Server) PrepareToolCall(
	ctx context.Context,
	request *mcpgatewayv1.PrepareToolCallRequest,
) (*mcpgatewayv1.PrepareToolCallResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.prepareToolCall.Execute(ctx, types.PrepareToolCallCommand{
		AuthContext:    auth,
		SkillID:        request.GetSkillId(),
		ToolName:       request.GetToolName(),
		Action:         toolActionFromProto(request.GetAction()),
		ResourceType:   request.GetResourceType(),
		ResourceID:     request.GetResourceId(),
		RiskLevel:      request.GetRiskLevel(),
		Intent:         request.GetIntent(),
		InputJSON:      request.GetInputJson(),
		IdempotencyKey: request.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return resultToProto(result), nil
}

func authFromProto(ctx context.Context, auth *mcpgatewayv1.AuthContext) (types.AuthContext, bool) {
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

func resultToProto(result types.PrepareToolCallResult) *mcpgatewayv1.PrepareToolCallResponse {
	return &mcpgatewayv1.PrepareToolCallResponse{
		TenantId:          string(result.TenantID),
		UserId:            string(result.UserID),
		SkillId:           result.SkillID,
		ToolName:          result.ToolName,
		Action:            toolActionToProto(result.Action),
		ResourceType:      result.ResourceType,
		ResourceId:        result.ResourceID,
		RiskLevel:         result.RiskLevel,
		Allowed:           result.Allowed,
		RequiresApproval:  result.RequiresApproval,
		PermissionVersion: result.PermissionVersion,
		Classification:    result.Classification,
		Reason:            result.Reason,
		DecisionSource:    result.DecisionSource,
		AuditId:           result.AuditID,
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
	case errors.Is(err, types.ErrAuditWriteFailed):
		return status.Error(codes.Unavailable, "mcp audit unavailable")
	default:
		return status.Error(codes.Internal, "mcp gateway internal error")
	}
}
