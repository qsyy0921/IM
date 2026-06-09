package grpc

import (
	"context"
	"errors"

	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type GetSendContextExecutor interface {
	Execute(context.Context, types.GetSendContextCommand) (types.ConversationSendContext, error)
}

type Server struct {
	conversationv1.UnimplementedConversationServiceServer
	getSendContext GetSendContextExecutor
}

func NewServer(getSendContext GetSendContextExecutor) *Server {
	return &Server{getSendContext: getSendContext}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	conversationv1.RegisterConversationServiceServer(registrar, server)
}

func (s *Server) GetSendContext(
	ctx context.Context,
	request *conversationv1.GetSendContextRequest,
) (*conversationv1.GetSendContextResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	result, err := s.getSendContext.Execute(ctx, types.GetSendContextCommand{
		TenantID:       types.TenantID(request.GetTenantId()),
		ConversationID: types.ConversationID(request.GetConversationId()),
		UserID:         types.UserID(request.GetUserId()),
		TraceID:        request.GetTraceId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &conversationv1.GetSendContextResponse{
		TenantId:            string(result.TenantID),
		ConversationId:      string(result.ConversationID),
		MemberVersion:       result.MemberVersion,
		PermissionVersion:   result.PermissionVersion,
		ConversationMode:    toProtoConversationMode(result.ConversationMode),
		FanoutMode:          toProtoFanoutMode(result.FanoutMode),
		FanoutPolicyVersion: result.FanoutPolicyVersion,
		CurrentSeqShard:     result.CurrentSeqShard,
	}, nil
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, types.ErrConversationNotFound):
		return status.Error(codes.NotFound, "conversation not found")
	case errors.Is(err, types.ErrMemberNotActive):
		return status.Error(codes.PermissionDenied, "conversation member is not active")
	case errors.Is(err, types.ErrDBReadFailed):
		return status.Error(codes.Unavailable, "conversation read failed")
	default:
		return status.Error(codes.Internal, "conversation service internal error")
	}
}

func toProtoConversationMode(mode types.ConversationMode) conversationv1.ConversationMode {
	switch mode {
	case types.ConversationModeLocalRowLock:
		return conversationv1.ConversationMode_CONVERSATION_MODE_LOCAL_ROW_LOCK
	case types.ConversationModeSequencerBlock:
		return conversationv1.ConversationMode_CONVERSATION_MODE_SEQUENCER_BLOCK
	default:
		return conversationv1.ConversationMode_CONVERSATION_MODE_UNSPECIFIED
	}
}

func toProtoFanoutMode(mode types.FanoutMode) conversationv1.FanoutMode {
	switch mode {
	case types.FanoutModeWriteFanout:
		return conversationv1.FanoutMode_FANOUT_MODE_WRITE_FANOUT
	case types.FanoutModeHybridFanout:
		return conversationv1.FanoutMode_FANOUT_MODE_HYBRID_FANOUT
	case types.FanoutModeReadFanout:
		return conversationv1.FanoutMode_FANOUT_MODE_READ_FANOUT
	case types.FanoutModeBroadcastSignal:
		return conversationv1.FanoutMode_FANOUT_MODE_BROADCAST_SIGNAL
	default:
		return conversationv1.FanoutMode_FANOUT_MODE_UNSPECIFIED
	}
}
