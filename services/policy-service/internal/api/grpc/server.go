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

type Server struct {
	policyv1.UnimplementedPolicyServiceServer
	checkMessageAction CheckMessageActionExecutor
}

func NewServer(checkMessageAction CheckMessageActionExecutor) *Server {
	return &Server{checkMessageAction: checkMessageAction}
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
		AuthContext:      authFromProto(request.GetAuthContext()),
		ConversationID:   types.ConversationID(request.GetConversationId()),
		Action:           actionFromProto(request.GetAction()),
		MessageID:        types.MessageID(request.GetMessageId()),
		DirectPeerUserID: types.UserID(request.GetDirectPeerUserId()),
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
