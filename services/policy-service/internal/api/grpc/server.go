package grpc

import (
	"context"
	"errors"

	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CheckMessageActionExecutor interface {
	Execute(context.Context, types.CheckMessageActionCommand) (types.MessageActionDecision, error)
}

type CheckToolActionExecutor interface {
	Execute(context.Context, types.CheckToolActionCommand) (types.ToolActionDecision, error)
}

type Server struct {
	policyv1.UnimplementedPolicyServiceServer
	checkMessageAction CheckMessageActionExecutor
	checkToolAction    CheckToolActionExecutor
}

func NewServer(checkMessageAction CheckMessageActionExecutor, toolExecutors ...CheckToolActionExecutor) *Server {
	server := &Server{checkMessageAction: checkMessageAction}
	if len(toolExecutors) > 0 {
		server.checkToolAction = toolExecutors[0]
	}
	return server
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	policyv1.RegisterPolicyServiceServer(registrar, server)
}

func (s *Server) CheckMessageAction(
	ctx context.Context,
	request *policyv1.CheckMessageActionRequest,
) (*policyv1.CheckMessageActionResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.checkMessageAction == nil {
		return nil, status.Error(codes.Unimplemented, "check message action is not configured")
	}
	decision, err := s.checkMessageAction.Execute(ctx, types.CheckMessageActionCommand{
		AuthContext:                   authFromProto(request.GetAuthContext()),
		ConversationID:                types.ConversationID(request.GetConversationId()),
		Action:                        actionFromProto(request.GetAction()),
		MessageID:                     types.MessageID(request.GetMessageId()),
		DirectPeerUserID:              types.UserID(request.GetDirectPeerUserId()),
		MessageSenderUserID:           types.UserID(request.GetMessageSenderUserId()),
		MessageText:                   request.GetMessageText(),
		ConversationPermissionVersion: request.GetConversationPermissionVersion(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &policyv1.CheckMessageActionResponse{
		TenantId:          string(decision.TenantID),
		UserId:            string(decision.UserID),
		ConversationId:    string(decision.ConversationID),
		MessageId:         string(decision.MessageID),
		Action:            actionToProto(decision.Action),
		Allowed:           decision.Allowed,
		PermissionVersion: decision.PermissionVersion,
		Classification:    decision.Classification,
		Reason:            decision.Reason,
		OwnershipOverride: decision.OwnershipOverride,
		DecisionSource:    string(decision.DecisionSource),
	}, nil
}

func (s *Server) CheckToolAction(
	ctx context.Context,
	request *policyv1.CheckToolActionRequest,
) (*policyv1.CheckToolActionResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.checkToolAction == nil {
		return nil, status.Error(codes.Unimplemented, "check tool action is not configured")
	}
	decision, err := s.checkToolAction.Execute(ctx, types.CheckToolActionCommand{
		AuthContext:  authFromProto(request.GetAuthContext()),
		ToolName:     request.GetToolName(),
		Action:       toolActionFromProto(request.GetAction()),
		ResourceType: request.GetResourceType(),
		ResourceID:   request.GetResourceId(),
		RiskLevel:    types.ToolRiskLevel(request.GetRiskLevel()),
		Intent:       request.GetIntent(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &policyv1.CheckToolActionResponse{
		TenantId:          string(decision.TenantID),
		UserId:            string(decision.UserID),
		ToolName:          decision.ToolName,
		Action:            toolActionToProto(decision.Action),
		ResourceType:      decision.ResourceType,
		ResourceId:        decision.ResourceID,
		RiskLevel:         string(decision.RiskLevel),
		Allowed:           decision.Allowed,
		RequiresApproval:  decision.RequiresApproval,
		PermissionVersion: decision.PermissionVersion,
		Classification:    decision.Classification,
		Reason:            decision.Reason,
		DecisionSource:    string(decision.DecisionSource),
	}, nil
}

func authFromProto(auth *policyv1.AuthContext) types.AuthContext {
	if auth == nil {
		return types.AuthContext{}
	}
	return types.AuthContext{
		TenantID:  types.TenantID(auth.GetTenantId()),
		UserID:    types.UserID(auth.GetUserId()),
		DeviceID:  types.DeviceID(auth.GetDeviceId()),
		SessionID: auth.GetSessionId(),
		TraceID:   auth.GetTraceId(),
		RequestID: auth.GetRequestId(),
	}
}

func actionFromProto(action policyv1.MessageAction) types.MessageAction {
	switch action {
	case policyv1.MessageAction_MESSAGE_ACTION_SEND:
		return types.MessageActionSend
	case policyv1.MessageAction_MESSAGE_ACTION_EDIT:
		return types.MessageActionEdit
	case policyv1.MessageAction_MESSAGE_ACTION_REVOKE:
		return types.MessageActionRevoke
	case policyv1.MessageAction_MESSAGE_ACTION_DELETE:
		return types.MessageActionDelete
	default:
		return ""
	}
}

func actionToProto(action types.MessageAction) policyv1.MessageAction {
	switch action {
	case types.MessageActionSend:
		return policyv1.MessageAction_MESSAGE_ACTION_SEND
	case types.MessageActionEdit:
		return policyv1.MessageAction_MESSAGE_ACTION_EDIT
	case types.MessageActionRevoke:
		return policyv1.MessageAction_MESSAGE_ACTION_REVOKE
	case types.MessageActionDelete:
		return policyv1.MessageAction_MESSAGE_ACTION_DELETE
	default:
		return policyv1.MessageAction_MESSAGE_ACTION_UNSPECIFIED
	}
}

func toolActionFromProto(action policyv1.ToolAction) types.ToolAction {
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

func toolActionToProto(action types.ToolAction) policyv1.ToolAction {
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
		return status.Error(codes.InvalidArgument, "invalid policy request")
	case errors.Is(err, types.ErrDependencyUnavailable):
		return status.Error(codes.Unavailable, "policy unavailable")
	case errors.Is(err, types.ErrDBWriteFailed):
		return status.Error(codes.Unavailable, "policy unavailable")
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
