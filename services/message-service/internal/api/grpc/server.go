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

type RevokeMessageExecutor interface {
	Execute(ctx context.Context, command types.RevokeMessageCommand) (types.MessageChangeResult, error)
}

type EditMessageExecutor interface {
	Execute(ctx context.Context, command types.EditMessageCommand) (types.MessageChangeResult, error)
}

type DeleteMessageExecutor interface {
	Execute(ctx context.Context, command types.DeleteMessageCommand) (types.MessageChangeResult, error)
}

type Server struct {
	messagev1.UnimplementedMessageServiceServer

	sendMessage   SendMessageExecutor
	editMessage   EditMessageExecutor
	revokeMessage RevokeMessageExecutor
	deleteMessage DeleteMessageExecutor
	now           func() time.Time
	metrics       types.LatencyRecorder
}

type Option func(*Server)

func WithClock(clock func() time.Time) Option {
	return func(server *Server) {
		if clock != nil {
			server.now = clock
		}
	}
}

func WithMetrics(metrics types.LatencyRecorder) Option {
	return func(server *Server) {
		if metrics != nil {
			server.metrics = metrics
		}
	}
}

func WithRevokeMessage(revokeMessage RevokeMessageExecutor) Option {
	return func(server *Server) {
		server.revokeMessage = revokeMessage
	}
}

func WithEditMessage(editMessage EditMessageExecutor) Option {
	return func(server *Server) {
		server.editMessage = editMessage
	}
}

func WithDeleteMessage(deleteMessage DeleteMessageExecutor) Option {
	return func(server *Server) {
		server.deleteMessage = deleteMessage
	}
}

func NewServer(sendMessage SendMessageExecutor, opts ...Option) *Server {
	server := &Server{
		sendMessage: sendMessage,
		now:         func() time.Time { return time.Now().UTC() },
		metrics:     types.NoopLatencyRecorder{},
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
	started := time.Now()
	defer func() {
		s.metrics.ObserveSendMessage(time.Since(started))
	}()

	command, err := s.toSendMessageCommand(ctx, req)
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

func (s *Server) EditMessage(ctx context.Context, req *messagev1.EditMessageRequest) (*messagev1.MessageChangeResponse, error) {
	command, err := s.toEditMessageCommand(ctx, req)
	if err != nil {
		return nil, grpcError(err, editReqCorrelationID(req))
	}

	result, err := s.editMessage.Execute(ctx, command)
	if err != nil {
		return nil, grpcError(err, editReqCorrelationID(req))
	}

	return &messagev1.MessageChangeResponse{
		MessageId:        string(result.MessageID),
		ConversationId:   string(result.ConversationID),
		ConversationSeq:  result.ConversationSeq,
		ChangeVersion:    result.ChangeVersion,
		AcceptedAt:       timestamppb.New(result.AcceptedAt),
		IdempotentReplay: result.IdempotentReplay,
	}, nil
}

func (s *Server) RevokeMessage(ctx context.Context, req *messagev1.RevokeMessageRequest) (*messagev1.MessageChangeResponse, error) {
	command, err := s.toRevokeMessageCommand(ctx, req)
	if err != nil {
		return nil, grpcError(err, revokeReqCorrelationID(req))
	}

	result, err := s.revokeMessage.Execute(ctx, command)
	if err != nil {
		return nil, grpcError(err, revokeReqCorrelationID(req))
	}

	return &messagev1.MessageChangeResponse{
		MessageId:        string(result.MessageID),
		ConversationId:   string(result.ConversationID),
		ConversationSeq:  result.ConversationSeq,
		ChangeVersion:    result.ChangeVersion,
		AcceptedAt:       timestamppb.New(result.AcceptedAt),
		IdempotentReplay: result.IdempotentReplay,
	}, nil
}

func (s *Server) DeleteMessage(ctx context.Context, req *messagev1.DeleteMessageRequest) (*messagev1.MessageChangeResponse, error) {
	command, err := s.toDeleteMessageCommand(ctx, req)
	if err != nil {
		return nil, grpcError(err, deleteReqCorrelationID(req))
	}

	result, err := s.deleteMessage.Execute(ctx, command)
	if err != nil {
		return nil, grpcError(err, deleteReqCorrelationID(req))
	}

	return &messagev1.MessageChangeResponse{
		MessageId:        string(result.MessageID),
		ConversationId:   string(result.ConversationID),
		ConversationSeq:  result.ConversationSeq,
		ChangeVersion:    result.ChangeVersion,
		AcceptedAt:       timestamppb.New(result.AcceptedAt),
		IdempotentReplay: result.IdempotentReplay,
	}, nil
}

func (s *Server) toSendMessageCommand(ctx context.Context, req *messagev1.SendMessageRequest) (types.SendMessageCommand, error) {
	if s.sendMessage == nil {
		return types.SendMessageCommand{}, errors.New("send message use case is not configured")
	}
	if req == nil {
		return types.SendMessageCommand{}, newInvalidArgument("request is required")
	}
	auth, ok := authFromProto(ctx, req.GetAuthContext())
	if !ok {
		return types.SendMessageCommand{}, newInvalidArgument("auth_context is required")
	}

	payloadJSON, err := payloadToJSON(req.GetPayload())
	if err != nil {
		return types.SendMessageCommand{}, newInvalidArgument(err.Error())
	}

	command := types.SendMessageCommand{
		AuthContext:    auth,
		ConversationID: types.ConversationID(req.GetConversationId()),
		ClientMsgID:    types.ClientMsgID(req.GetClientMsgId()),
		MessageType:    types.MessageType(req.GetMessageType()),
		PayloadJSON:    payloadJSON,
		AttachmentIDs:  append([]string(nil), req.GetAttachmentIds()...),
		ReceivedAt:     s.now(),
	}
	if err := command.Validate(); err != nil {
		if errors.Is(err, types.ErrUnsupportedMessageType) {
			return types.SendMessageCommand{}, err
		}
		return types.SendMessageCommand{}, newInvalidArgument(err.Error())
	}
	return command, nil
}

func (s *Server) toRevokeMessageCommand(ctx context.Context, req *messagev1.RevokeMessageRequest) (types.RevokeMessageCommand, error) {
	if s.revokeMessage == nil {
		return types.RevokeMessageCommand{}, errors.New("revoke message use case is not configured")
	}
	if req == nil {
		return types.RevokeMessageCommand{}, newInvalidArgument("request is required")
	}
	auth, ok := authFromProto(ctx, req.GetAuthContext())
	if !ok {
		return types.RevokeMessageCommand{}, newInvalidArgument("auth_context is required")
	}
	command := types.RevokeMessageCommand{
		AuthContext:    auth,
		ConversationID: types.ConversationID(req.GetConversationId()),
		MessageID:      types.MessageID(req.GetMessageId()),
		IdempotencyKey: req.GetIdempotencyKey(),
		Reason:         req.GetReason(),
		ReceivedAt:     s.now(),
	}
	if err := command.Validate(); err != nil {
		return types.RevokeMessageCommand{}, newInvalidArgument(err.Error())
	}
	return command, nil
}

func (s *Server) toEditMessageCommand(ctx context.Context, req *messagev1.EditMessageRequest) (types.EditMessageCommand, error) {
	if s.editMessage == nil {
		return types.EditMessageCommand{}, errors.New("edit message use case is not configured")
	}
	if req == nil {
		return types.EditMessageCommand{}, newInvalidArgument("request is required")
	}
	auth, ok := authFromProto(ctx, req.GetAuthContext())
	if !ok {
		return types.EditMessageCommand{}, newInvalidArgument("auth_context is required")
	}
	payloadJSON, err := payloadToJSON(req.GetPayload())
	if err != nil {
		return types.EditMessageCommand{}, newInvalidArgument(err.Error())
	}
	command := types.EditMessageCommand{
		AuthContext:    auth,
		ConversationID: types.ConversationID(req.GetConversationId()),
		MessageID:      types.MessageID(req.GetMessageId()),
		IdempotencyKey: req.GetIdempotencyKey(),
		PayloadJSON:    payloadJSON,
		Reason:         req.GetReason(),
		ReceivedAt:     s.now(),
	}
	if err := command.Validate(); err != nil {
		return types.EditMessageCommand{}, newInvalidArgument(err.Error())
	}
	return command, nil
}

func (s *Server) toDeleteMessageCommand(ctx context.Context, req *messagev1.DeleteMessageRequest) (types.DeleteMessageCommand, error) {
	if s.deleteMessage == nil {
		return types.DeleteMessageCommand{}, errors.New("delete message use case is not configured")
	}
	if req == nil {
		return types.DeleteMessageCommand{}, newInvalidArgument("request is required")
	}
	auth, ok := authFromProto(ctx, req.GetAuthContext())
	if !ok {
		return types.DeleteMessageCommand{}, newInvalidArgument("auth_context is required")
	}
	scope, err := deleteScopeFromProto(req.GetDeleteScope())
	if err != nil {
		return types.DeleteMessageCommand{}, newInvalidArgument(err.Error())
	}
	command := types.DeleteMessageCommand{
		AuthContext:    auth,
		ConversationID: types.ConversationID(req.GetConversationId()),
		MessageID:      types.MessageID(req.GetMessageId()),
		IdempotencyKey: req.GetIdempotencyKey(),
		DeleteScope:    scope,
		Reason:         req.GetReason(),
		ReceivedAt:     s.now(),
	}
	if err := command.Validate(); err != nil {
		return types.DeleteMessageCommand{}, newInvalidArgument(err.Error())
	}
	return command, nil
}

func deleteScopeFromProto(scope messagev1.DeleteScope) (types.DeleteScope, error) {
	switch scope {
	case messagev1.DeleteScope_DELETE_SCOPE_CONVERSATION_VIEW:
		return types.DeleteScopeConversationView, nil
	case messagev1.DeleteScope_DELETE_SCOPE_COMPLIANCE_RETENTION:
		return types.DeleteScopeCompliance, nil
	default:
		return "", errors.New("delete_scope is required")
	}
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

func authFromProto(ctx context.Context, auth *messagev1.AuthContext) (types.AuthContext, bool) {
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
		DeviceID:  types.DeviceID(auth.GetDeviceId()),
		SessionID: types.SessionID(auth.GetSessionId()),
		TraceID:   auth.GetTraceId(),
		RequestID: auth.GetRequestId(),
	}, true
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

func editReqCorrelationID(req *messagev1.EditMessageRequest) string {
	if req == nil {
		return ""
	}
	auth := req.GetAuthContext()
	if auth == nil {
		return req.GetIdempotencyKey()
	}
	if auth.GetRequestId() != "" {
		return auth.GetRequestId()
	}
	if auth.GetTraceId() != "" {
		return auth.GetTraceId()
	}
	return req.GetIdempotencyKey()
}

func revokeReqCorrelationID(req *messagev1.RevokeMessageRequest) string {
	if req == nil {
		return ""
	}
	auth := req.GetAuthContext()
	if auth == nil {
		return req.GetIdempotencyKey()
	}
	if auth.GetRequestId() != "" {
		return auth.GetRequestId()
	}
	if auth.GetTraceId() != "" {
		return auth.GetTraceId()
	}
	return req.GetIdempotencyKey()
}

func deleteReqCorrelationID(req *messagev1.DeleteMessageRequest) string {
	if req == nil {
		return ""
	}
	auth := req.GetAuthContext()
	if auth == nil {
		return req.GetIdempotencyKey()
	}
	if auth.GetRequestId() != "" {
		return auth.GetRequestId()
	}
	if auth.GetTraceId() != "" {
		return auth.GetTraceId()
	}
	return req.GetIdempotencyKey()
}
