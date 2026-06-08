package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type SendMessageExecutor interface {
	Execute(ctx context.Context, command types.SendMessageCommand) (types.SendMessageResult, error)
}

type Server struct {
	messagev1.UnimplementedMessageServiceServer

	sendMessage SendMessageExecutor
	now         func() time.Time
}

type Option func(*Server)

func WithClock(clock func() time.Time) Option {
	return func(server *Server) {
		if clock != nil {
			server.now = clock
		}
	}
}

func NewServer(sendMessage SendMessageExecutor, opts ...Option) *Server {
	server := &Server{
		sendMessage: sendMessage,
		now:         func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(server)
	}
	return server
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	messagev1.RegisterMessageServiceServer(registrar, server)
}

func (s *Server) SendMessage(ctx context.Context, req *messagev1.SendMessageRequest) (*messagev1.SendMessageResponse, error) {
	command, err := s.toSendMessageCommand(req)
	if err != nil {
		return nil, grpcError(err, reqCorrelationID(req))
	}

	result, err := s.sendMessage.Execute(ctx, command)
	if err != nil {
		return nil, grpcError(err, reqCorrelationID(req))
	}

	return &messagev1.SendMessageResponse{
		MessageId:        string(result.MessageID),
		ConversationId:   string(result.ConversationID),
		ConversationSeq:  result.ConversationSeq,
		AcceptedAt:       timestamppb.New(result.AcceptedAt),
		IdempotentReplay: result.IdempotentReplay,
	}, nil
}

func (s *Server) toSendMessageCommand(req *messagev1.SendMessageRequest) (types.SendMessageCommand, error) {
	if s.sendMessage == nil {
		return types.SendMessageCommand{}, errors.New("send message use case is not configured")
	}
	if req == nil {
		return types.SendMessageCommand{}, newInvalidArgument("request is required")
	}
	auth := req.GetAuthContext()
	if auth == nil {
		return types.SendMessageCommand{}, newInvalidArgument("auth_context is required")
	}

	payloadJSON, err := payloadToJSON(req.GetPayload())
	if err != nil {
		return types.SendMessageCommand{}, newInvalidArgument(err.Error())
	}

	command := types.SendMessageCommand{
		AuthContext: types.AuthContext{
			TenantID:  types.TenantID(auth.GetTenantId()),
			UserID:    types.UserID(auth.GetUserId()),
			DeviceID:  types.DeviceID(auth.GetDeviceId()),
			SessionID: types.SessionID(auth.GetSessionId()),
			TraceID:   auth.GetTraceId(),
			RequestID: auth.GetRequestId(),
		},
		ConversationID: types.ConversationID(req.GetConversationId()),
		ClientMsgID:    types.ClientMsgID(req.GetClientMsgId()),
		MessageType:    types.MessageType(req.GetMessageType()),
		PayloadJSON:    payloadJSON,
		AttachmentIDs:  append([]string(nil), req.GetAttachmentIds()...),
		ReceivedAt:     s.now(),
	}
	if err := command.Validate(); err != nil {
		return types.SendMessageCommand{}, newInvalidArgument(err.Error())
	}
	if command.MessageType != types.MessageTypeText {
		return types.SendMessageCommand{}, types.NewUnsupportedMessageType("message_type is not supported in phase 1")
	}
	return command, nil
}

func payloadToJSON(payload *structpb.Struct) ([]byte, error) {
	if payload == nil {
		return []byte("{}"), nil
	}
	encoded, err := json.Marshal(payload.AsMap())
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func reqCorrelationID(req *messagev1.SendMessageRequest) string {
	if req == nil {
		return ""
	}
	auth := req.GetAuthContext()
	if auth == nil {
		return req.GetClientMsgId()
	}
	if auth.GetRequestId() != "" {
		return auth.GetRequestId()
	}
	if auth.GetTraceId() != "" {
		return auth.GetTraceId()
	}
	return req.GetClientMsgId()
}
