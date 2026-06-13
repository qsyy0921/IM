package grpc

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	gatewayv1 "github.com/qsyy0921/IM/api/proto/nexusim/gateway/v1"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	receiptv1 "github.com/qsyy0921/IM/api/proto/nexusim/receipt/v1"
	gatewayauth "github.com/qsyy0921/IM/internal/gatewayauth"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	metadataTenantID  = "x-nexusim-tenant-id"
	metadataUserID    = "x-nexusim-user-id"
	metadataDeviceID  = "x-nexusim-device-id"
	metadataSessionID = "x-nexusim-session-id"
	metadataTraceID   = "x-nexusim-trace-id"
	metadataRequestID = "x-nexusim-request-id"
	metadataToken     = "x-nexusim-gateway-token"
)

type Authenticator interface {
	Authenticate(*http.Request) (gatewayauth.AuthContext, error)
}

type Server struct {
	gatewayv1.UnimplementedGatewayServiceServer
	conversationv1.UnimplementedConversationServiceServer
	messagev1.UnimplementedMessageServiceServer
	deliveryv1.UnimplementedDeliveryServiceServer
	receiptv1.UnimplementedReceiptServiceServer

	auth         Authenticator
	conversation conversationv1.ConversationServiceClient
	message      messagev1.MessageServiceClient
	delivery     deliveryv1.DeliveryServiceClient
	receipt      receiptv1.ReceiptServiceClient
}

type Config struct {
	Authenticator Authenticator
	Conversation  conversationv1.ConversationServiceClient
	Message       messagev1.MessageServiceClient
	Delivery      deliveryv1.DeliveryServiceClient
	Receipt       receiptv1.ReceiptServiceClient
}

type RegisterConfig struct {
	RegisterLegacyDescriptors bool
}

func NewServer(config Config) *Server {
	return &Server{
		auth:         config.Authenticator,
		conversation: config.Conversation,
		message:      config.Message,
		delivery:     config.Delivery,
		receipt:      config.Receipt,
	}
}

func Register(server grpcgo.ServiceRegistrar, gateway *Server) {
	RegisterWithConfig(server, gateway, RegisterConfig{RegisterLegacyDescriptors: true})
}

func RegisterWithConfig(server grpcgo.ServiceRegistrar, gateway *Server, config RegisterConfig) {
	gatewayv1.RegisterGatewayServiceServer(server, gateway)
	if !config.RegisterLegacyDescriptors {
		return
	}
	conversationv1.RegisterConversationServiceServer(server, gateway)
	messagev1.RegisterMessageServiceServer(server, gateway)
	deliveryv1.RegisterDeliveryServiceServer(server, gateway)
	receiptv1.RegisterReceiptServiceServer(server, gateway)
}

func (server *Server) GetSendContext(context.Context, *conversationv1.GetSendContextRequest) (*conversationv1.GetSendContextResponse, error) {
	return nil, status.Error(codes.Unimplemented, "GetSendContext is service-internal")
}

func (server *Server) CreateMemberChange(ctx context.Context, request *conversationv1.CreateMemberChangeRequest) (*conversationv1.CreateMemberChangeResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*conversationv1.CreateMemberChangeRequest)
	cloned.AuthContext = conversationAuth(auth)
	return server.conversation.CreateMemberChange(outgoing, cloned)
}

func (server *Server) GetMemberChange(ctx context.Context, request *conversationv1.GetMemberChangeRequest) (*conversationv1.GetMemberChangeResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*conversationv1.GetMemberChangeRequest)
	cloned.AuthContext = conversationAuth(auth)
	return server.conversation.GetMemberChange(outgoing, cloned)
}

func (server *Server) ListConversationMembers(ctx context.Context, request *conversationv1.ListConversationMembersRequest) (*conversationv1.ListConversationMembersResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*conversationv1.ListConversationMembersRequest)
	cloned.AuthContext = conversationAuth(auth)
	return server.conversation.ListConversationMembers(outgoing, cloned)
}

func (server *Server) TransferConversationOwner(ctx context.Context, request *conversationv1.TransferConversationOwnerRequest) (*conversationv1.TransferConversationOwnerResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*conversationv1.TransferConversationOwnerRequest)
	cloned.AuthContext = conversationAuth(auth)
	return server.conversation.TransferConversationOwner(outgoing, cloned)
}

func (server *Server) SendMessage(ctx context.Context, request *messagev1.SendMessageRequest) (*messagev1.SendMessageResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*messagev1.SendMessageRequest)
	cloned.AuthContext = messageAuth(auth)
	return server.message.SendMessage(outgoing, cloned)
}

func (server *Server) EditMessage(ctx context.Context, request *messagev1.EditMessageRequest) (*messagev1.MessageChangeResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*messagev1.EditMessageRequest)
	cloned.AuthContext = messageAuth(auth)
	return server.message.EditMessage(outgoing, cloned)
}

func (server *Server) RevokeMessage(ctx context.Context, request *messagev1.RevokeMessageRequest) (*messagev1.MessageChangeResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*messagev1.RevokeMessageRequest)
	cloned.AuthContext = messageAuth(auth)
	return server.message.RevokeMessage(outgoing, cloned)
}

func (server *Server) DeleteMessage(ctx context.Context, request *messagev1.DeleteMessageRequest) (*messagev1.MessageChangeResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*messagev1.DeleteMessageRequest)
	cloned.AuthContext = messageAuth(auth)
	return server.message.DeleteMessage(outgoing, cloned)
}

func (server *Server) PullInbox(ctx context.Context, request *deliveryv1.PullInboxRequest) (*deliveryv1.PullInboxResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*deliveryv1.PullInboxRequest)
	cloned.AuthContext = deliveryAuth(auth)
	return server.delivery.PullInbox(outgoing, cloned)
}

func (server *Server) AckDelivery(ctx context.Context, request *deliveryv1.AckDeliveryRequest) (*deliveryv1.AckDeliveryResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*deliveryv1.AckDeliveryRequest)
	cloned.AuthContext = deliveryAuth(auth)
	return server.delivery.AckDelivery(outgoing, cloned)
}

func (server *Server) MarkRead(ctx context.Context, request *receiptv1.MarkReadRequest) (*receiptv1.MarkReadResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*receiptv1.MarkReadRequest)
	cloned.AuthContext = receiptAuth(auth)
	return server.receipt.MarkRead(outgoing, cloned)
}

func (server *Server) GetReceiptState(ctx context.Context, request *receiptv1.GetReceiptStateRequest) (*receiptv1.GetReceiptStateResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*receiptv1.GetReceiptStateRequest)
	cloned.AuthContext = receiptAuth(auth)
	return server.receipt.GetReceiptState(outgoing, cloned)
}

func (server *Server) ListReceiptStates(ctx context.Context, request *receiptv1.ListReceiptStatesRequest) (*receiptv1.ListReceiptStatesResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*receiptv1.ListReceiptStatesRequest)
	cloned.AuthContext = receiptAuth(auth)
	return server.receipt.ListReceiptStates(outgoing, cloned)
}

func (server *Server) ListConversations(ctx context.Context, request *receiptv1.ListConversationsRequest) (*receiptv1.ListConversationsResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*receiptv1.ListConversationsRequest)
	cloned.AuthContext = receiptAuth(auth)
	return server.receipt.ListConversations(outgoing, cloned)
}

func (server *Server) ArchiveConversation(ctx context.Context, request *receiptv1.ArchiveConversationRequest) (*receiptv1.ArchiveConversationResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*receiptv1.ArchiveConversationRequest)
	cloned.AuthContext = receiptAuth(auth)
	return server.receipt.ArchiveConversation(outgoing, cloned)
}

func (server *Server) PinConversation(ctx context.Context, request *receiptv1.PinConversationRequest) (*receiptv1.PinConversationResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*receiptv1.PinConversationRequest)
	cloned.AuthContext = receiptAuth(auth)
	return server.receipt.PinConversation(outgoing, cloned)
}

func (server *Server) MuteConversation(ctx context.Context, request *receiptv1.MuteConversationRequest) (*receiptv1.MuteConversationResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*receiptv1.MuteConversationRequest)
	cloned.AuthContext = receiptAuth(auth)
	return server.receipt.MuteConversation(outgoing, cloned)
}

func (server *Server) authenticate(ctx context.Context) (gatewayauth.AuthContext, context.Context, error) {
	if server.auth == nil {
		return gatewayauth.AuthContext{}, nil, status.Error(codes.Internal, "gateway auth is not configured")
	}
	request, requestID := authRequestFromMetadata(ctx)
	auth, err := server.auth.Authenticate(request)
	if err != nil {
		return gatewayauth.AuthContext{}, nil, publicAuthError(err)
	}
	if auth.TenantID == "" || auth.UserID == "" || auth.DeviceID == "" {
		return gatewayauth.AuthContext{}, nil, status.Error(codes.Unauthenticated, "gateway auth metadata is required")
	}
	if auth.RequestID == "" {
		auth.RequestID = requestID
	}
	if auth.TraceID == "" {
		auth.TraceID = firstIncomingMetadata(ctx, metadataTraceID)
	}
	return auth, outgoingVerifiedContext(ctx, auth), nil
}

func authRequestFromMetadata(ctx context.Context) (*http.Request, string) {
	query := url.Values{}
	for _, pair := range []struct {
		metadata string
		query    string
	}{
		{metadataToken, "token"},
		{metadataTraceID, "trace_id"},
	} {
		if value := firstIncomingMetadata(ctx, pair.metadata); value != "" {
			query.Set(pair.query, value)
		}
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://api-gateway/auth?"+query.Encode(), nil)
	if authorization := firstIncomingMetadata(ctx, "authorization"); authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	return request, firstIncomingMetadata(ctx, metadataRequestID)
}

func outgoingVerifiedContext(ctx context.Context, auth gatewayauth.AuthContext) context.Context {
	pairs := []string{
		metadataTenantID, auth.TenantID,
		metadataUserID, auth.UserID,
		metadataDeviceID, auth.DeviceID,
	}
	if auth.SessionID != "" {
		pairs = append(pairs, metadataSessionID, auth.SessionID)
	}
	if auth.TraceID != "" {
		pairs = append(pairs, metadataTraceID, auth.TraceID)
	}
	if auth.RequestID != "" {
		pairs = append(pairs, metadataRequestID, auth.RequestID)
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...)
}

func firstIncomingMetadata(ctx context.Context, key string) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func publicAuthError(err error) error {
	switch {
	case errors.Is(err, gatewayauth.ErrAuthExpired):
		return status.Error(codes.Unauthenticated, "gateway token expired")
	case errors.Is(err, gatewayauth.ErrPermissionDenied), errors.Is(err, gatewayauth.ErrInvalidRequest):
		return status.Error(codes.Unauthenticated, "gateway auth failed")
	default:
		return status.Error(codes.Unauthenticated, "gateway auth failed")
	}
}

func conversationAuth(auth gatewayauth.AuthContext) *conversationv1.AuthContext {
	return &conversationv1.AuthContext{
		TenantId:  auth.TenantID,
		UserId:    auth.UserID,
		DeviceId:  auth.DeviceID,
		SessionId: auth.SessionID,
		TraceId:   auth.TraceID,
		RequestId: auth.RequestID,
	}
}

func messageAuth(auth gatewayauth.AuthContext) *messagev1.AuthContext {
	return &messagev1.AuthContext{
		TenantId:  auth.TenantID,
		UserId:    auth.UserID,
		DeviceId:  auth.DeviceID,
		SessionId: auth.SessionID,
		TraceId:   auth.TraceID,
		RequestId: auth.RequestID,
	}
}

func deliveryAuth(auth gatewayauth.AuthContext) *deliveryv1.AuthContext {
	return &deliveryv1.AuthContext{
		TenantId:  auth.TenantID,
		UserId:    auth.UserID,
		DeviceId:  auth.DeviceID,
		SessionId: auth.SessionID,
		TraceId:   auth.TraceID,
		RequestId: auth.RequestID,
	}
}

func receiptAuth(auth gatewayauth.AuthContext) *receiptv1.AuthContext {
	return &receiptv1.AuthContext{
		TenantId:  auth.TenantID,
		UserId:    auth.UserID,
		DeviceId:  auth.DeviceID,
		SessionId: auth.SessionID,
		TraceId:   auth.TraceID,
		RequestId: auth.RequestID,
	}
}
