package grpc

import (
	"context"
	"net"
	"testing"
	"time"

	contactsv1 "github.com/qsyy0921/IM/api/proto/nexusim/contacts/v1"
	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	gatewayv1 "github.com/qsyy0921/IM/api/proto/nexusim/gateway/v1"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	receiptv1 "github.com/qsyy0921/IM/api/proto/nexusim/receipt/v1"
	gatewayauth "github.com/qsyy0921/IM/internal/gatewayauth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

var _ gatewayv1.GatewayServiceServer = (*Server)(nil)

func TestSendMessageInjectsVerifiedAuthAndOverridesBody(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	authenticator, err := gatewayauth.NewAuthenticator(gatewayauth.Config{
		Mode:     gatewayauth.ModeHMAC,
		Secret:   "secret",
		Audience: "push-gateway",
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	token, err := gatewayauth.SignGatewayToken("secret", map[string]string{
		"tenant_id":  "tenant-token",
		"user_id":    "user-token",
		"device_id":  "device-token",
		"session_id": "session-token",
		"trace_id":   "trace-token",
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("sign gateway token: %v", err)
	}
	fake := &fakeMessageClient{}
	server := NewServer(Config{Authenticator: authenticator, Message: fake})
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"authorization", "Bearer "+token,
		metadataDeviceID, "device-token",
		metadataTenantID, "tenant-spoofed",
		metadataUserID, "user-spoofed",
		metadataRequestID, "request-1",
	))

	_, err = server.SendMessage(ctx, &messagev1.SendMessageRequest{
		AuthContext: &messagev1.AuthContext{
			TenantId:  "tenant-body",
			UserId:    "user-body",
			DeviceId:  "device-body",
			SessionId: "session-body",
			TraceId:   "trace-body",
			RequestId: "request-body",
		},
		ConversationId: "conv-1",
		ClientMsgId:    "client-msg-1",
		MessageType:    "TEXT",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if fake.request.GetAuthContext().GetTenantId() != "tenant-token" ||
		fake.request.GetAuthContext().GetUserId() != "user-token" ||
		fake.request.GetAuthContext().GetDeviceId() != "device-token" ||
		fake.request.GetAuthContext().GetSessionId() != "session-token" ||
		fake.request.GetAuthContext().GetTraceId() != "trace-token" ||
		fake.request.GetAuthContext().GetRequestId() != "request-1" {
		t.Fatalf("expected body auth to be overwritten by token auth, got %+v", fake.request.GetAuthContext())
	}
	assertOutgoingMetadata(t, fake.ctx, map[string]string{
		metadataTenantID:  "tenant-token",
		metadataUserID:    "user-token",
		metadataDeviceID:  "device-token",
		metadataSessionID: "session-token",
		metadataTraceID:   "trace-token",
		metadataRequestID: "request-1",
	})
}

func TestListConversationsInjectsVerifiedAuth(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	authenticator, err := gatewayauth.NewAuthenticator(gatewayauth.Config{
		Mode:     gatewayauth.ModeHMAC,
		Secret:   "secret",
		Audience: "push-gateway",
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	token, err := gatewayauth.SignGatewayToken("secret", map[string]string{
		"tenant_id": "tenant-token",
		"user_id":   "user-token",
		"device_id": "device-token",
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("sign gateway token: %v", err)
	}
	fake := &fakeReceiptClient{}
	server := NewServer(Config{Authenticator: authenticator, Receipt: fake})
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"authorization", "Bearer "+token,
		metadataDeviceID, "device-token",
	))

	_, err = server.ListConversations(ctx, &receiptv1.ListConversationsRequest{
		AuthContext: &receiptv1.AuthContext{TenantId: "tenant-body", UserId: "user-body", DeviceId: "device-body"},
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if fake.listRequest.GetAuthContext().GetTenantId() != "tenant-token" ||
		fake.listRequest.GetAuthContext().GetUserId() != "user-token" ||
		fake.listRequest.GetAuthContext().GetDeviceId() != "device-token" {
		t.Fatalf("expected receipt auth to be overwritten, got %+v", fake.listRequest.GetAuthContext())
	}
	assertOutgoingMetadata(t, fake.ctx, map[string]string{
		metadataTenantID: "tenant-token",
		metadataUserID:   "user-token",
		metadataDeviceID: "device-token",
	})
}

func TestGatewayGeneratesMissingCorrelationIDsForDownstream(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	authenticator, err := gatewayauth.NewAuthenticator(gatewayauth.Config{
		Mode:     gatewayauth.ModeHMAC,
		Secret:   "secret",
		Audience: "api-gateway",
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	token, err := gatewayauth.SignGatewayToken("secret", map[string]string{
		"tenant_id": "tenant-token",
		"user_id":   "user-token",
		"device_id": "device-token",
		"aud":       "api-gateway",
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("sign gateway token: %v", err)
	}
	fake := &fakeMessageClient{}
	server := NewServer(Config{
		Authenticator: authenticator,
		Message:       fake,
		NewTraceID:    func() string { return "trace-generated" },
		NewRequestID:  func() string { return "request-generated" },
	})
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"authorization", "Bearer "+token,
	))

	_, err = server.SendMessage(ctx, &messagev1.SendMessageRequest{
		AuthContext:    &messagev1.AuthContext{TraceId: "trace-body", RequestId: "request-body"},
		ConversationId: "conv-1",
		ClientMsgId:    "client-msg-1",
		MessageType:    "TEXT",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if fake.request.GetAuthContext().GetTraceId() != "trace-generated" ||
		fake.request.GetAuthContext().GetRequestId() != "request-generated" {
		t.Fatalf("expected generated correlation ids in auth context, got %+v", fake.request.GetAuthContext())
	}
	assertOutgoingMetadata(t, fake.ctx, map[string]string{
		metadataTraceID:   "trace-generated",
		metadataRequestID: "request-generated",
	})
}

func TestGatewayReturnsGeneratedCorrelationHeaders(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	authenticator, err := gatewayauth.NewAuthenticator(gatewayauth.Config{
		Mode:     gatewayauth.ModeHMAC,
		Secret:   "secret",
		Audience: "api-gateway",
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	token, err := gatewayauth.SignGatewayToken("secret", map[string]string{
		"tenant_id": "tenant-token",
		"user_id":   "user-token",
		"device_id": "device-token",
		"aud":       "api-gateway",
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("sign gateway token: %v", err)
	}
	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	RegisterWithConfig(grpcServer, NewServer(Config{
		Authenticator: authenticator,
		Message:       &fakeMessageClient{},
		NewTraceID:    func() string { return "trace-generated" },
		NewRequestID:  func() string { return "request-generated" },
	}), RegisterConfig{RegisterLegacyDescriptors: false})
	go func() {
		_ = grpcServer.Serve(listener)
	}()
	defer grpcServer.Stop()

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	defer conn.Close()
	client := gatewayv1.NewGatewayServiceClient(conn)
	callCtx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+token))
	var header metadata.MD
	_, err = client.SendMessage(callCtx, &messagev1.SendMessageRequest{
		ConversationId: "conv-1",
		ClientMsgId:    "client-msg-1",
		MessageType:    "TEXT",
	}, grpc.Header(&header))
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	assertMetadataValue(t, header, metadataTraceID, "trace-generated")
	assertMetadataValue(t, header, metadataRequestID, "request-generated")
}

func TestSendContactRequestInjectsVerifiedAuthAndOverridesBody(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	authenticator, err := gatewayauth.NewAuthenticator(gatewayauth.Config{
		Mode:     gatewayauth.ModeHMAC,
		Secret:   "secret",
		Audience: "api-gateway",
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	token, err := gatewayauth.SignGatewayToken("secret", map[string]string{
		"tenant_id":  "tenant-token",
		"user_id":    "user-token",
		"device_id":  "device-token",
		"session_id": "session-token",
		"trace_id":   "trace-token",
		"aud":        "api-gateway",
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("sign gateway token: %v", err)
	}
	fake := &fakeContactsClient{}
	server := NewServer(Config{Authenticator: authenticator, Contacts: fake})
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"authorization", "Bearer "+token,
		metadataDeviceID, "device-token",
		metadataRequestID, "request-contacts-1",
	))

	_, err = server.SendContactRequest(ctx, &contactsv1.SendContactRequestRequest{
		AuthContext: &contactsv1.AuthContext{
			TenantId:  "tenant-body",
			UserId:    "user-body",
			DeviceId:  "device-body",
			SessionId: "session-body",
			TraceId:   "trace-body",
			RequestId: "request-body",
		},
		TargetUserId:   "target-user",
		IdempotencyKey: "idem-1",
	})
	if err != nil {
		t.Fatalf("send contact request: %v", err)
	}
	if fake.sendRequest.GetAuthContext().GetTenantId() != "tenant-token" ||
		fake.sendRequest.GetAuthContext().GetUserId() != "user-token" ||
		fake.sendRequest.GetAuthContext().GetDeviceId() != "device-token" ||
		fake.sendRequest.GetAuthContext().GetSessionId() != "session-token" ||
		fake.sendRequest.GetAuthContext().GetTraceId() != "trace-token" ||
		fake.sendRequest.GetAuthContext().GetRequestId() != "request-contacts-1" {
		t.Fatalf("expected contacts auth to be overwritten, got %+v", fake.sendRequest.GetAuthContext())
	}
	assertOutgoingMetadata(t, fake.ctx, map[string]string{
		metadataTenantID:  "tenant-token",
		metadataUserID:    "user-token",
		metadataDeviceID:  "device-token",
		metadataSessionID: "session-token",
		metadataTraceID:   "trace-token",
		metadataRequestID: "request-contacts-1",
	})
}

func TestGatewayRejectsMissingToken(t *testing.T) {
	authenticator, err := gatewayauth.NewAuthenticator(gatewayauth.Config{
		Mode:     gatewayauth.ModeHMAC,
		Secret:   "secret",
		Audience: "push-gateway",
		Now:      func() time.Time { return time.Unix(1_800_000_000, 0) },
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	server := NewServer(Config{Authenticator: authenticator, Message: &fakeMessageClient{}})
	_, err = server.SendMessage(context.Background(), &messagev1.SendMessageRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected unauthenticated, got %v", err)
	}
}

func TestGatewayRejectsMetadataDeviceWithoutTokenDevice(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	authenticator, err := gatewayauth.NewAuthenticator(gatewayauth.Config{
		Mode:     gatewayauth.ModeHMAC,
		Secret:   "secret",
		Audience: "push-gateway",
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	token, err := gatewayauth.SignGatewayToken("secret", map[string]string{
		"tenant_id": "tenant-token",
		"user_id":   "user-token",
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("sign gateway token: %v", err)
	}
	server := NewServer(Config{Authenticator: authenticator, Message: &fakeMessageClient{}})
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"authorization", "Bearer "+token,
		metadataDeviceID, "device-spoofed",
	))

	_, err = server.SendMessage(ctx, &messagev1.SendMessageRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected unauthenticated, got %v", err)
	}
}

func TestGatewayDoesNotExposeGetSendContext(t *testing.T) {
	server := NewServer(Config{})
	_, err := server.GetSendContext(context.Background(), &conversationv1.GetSendContextRequest{})
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("expected unimplemented, got %v", err)
	}
}

func TestGatewayDefaultRegistrationKeepsLegacyDescriptors(t *testing.T) {
	grpcServer := grpc.NewServer()
	Register(grpcServer, NewServer(Config{}))

	info := grpcServer.GetServiceInfo()
	assertServiceRegistered(t, info, "nexusim.gateway.v1.GatewayService")
	assertServiceRegistered(t, info, "nexusim.contacts.v1.ContactsService")
	assertServiceRegistered(t, info, "nexusim.conversation.v1.ConversationService")
	assertServiceRegistered(t, info, "nexusim.message.v1.MessageService")
	assertServiceRegistered(t, info, "nexusim.delivery.v1.DeliveryService")
	assertServiceRegistered(t, info, "nexusim.receipt.v1.ReceiptService")
	assertGatewayFacadeExcludesInternalMethods(t, info)
}

func TestGatewayCanDisableLegacyDescriptors(t *testing.T) {
	grpcServer := grpc.NewServer()
	RegisterWithConfig(grpcServer, NewServer(Config{}), RegisterConfig{RegisterLegacyDescriptors: false})

	info := grpcServer.GetServiceInfo()
	assertServiceRegistered(t, info, "nexusim.gateway.v1.GatewayService")
	assertServiceNotRegistered(t, info, "nexusim.contacts.v1.ContactsService")
	assertServiceNotRegistered(t, info, "nexusim.conversation.v1.ConversationService")
	assertServiceNotRegistered(t, info, "nexusim.message.v1.MessageService")
	assertServiceNotRegistered(t, info, "nexusim.delivery.v1.DeliveryService")
	assertServiceNotRegistered(t, info, "nexusim.receipt.v1.ReceiptService")
	assertGatewayFacadeExcludesInternalMethods(t, info)
}

func assertOutgoingMetadata(t *testing.T, ctx context.Context, expected map[string]string) {
	t.Helper()
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatalf("expected outgoing metadata")
	}
	for key, value := range expected {
		values := md.Get(key)
		if len(values) == 0 || values[0] != value {
			t.Fatalf("expected metadata %s=%q, got %v", key, value, values)
		}
	}
}

func assertMetadataValue(t *testing.T, md metadata.MD, key string, expected string) {
	t.Helper()
	values := md.Get(key)
	if len(values) == 0 || values[0] != expected {
		t.Fatalf("expected metadata %s=%q, got %v", key, expected, values)
	}
}

func assertGatewayFacadeExcludesInternalMethods(t *testing.T, info map[string]grpc.ServiceInfo) {
	t.Helper()
	facade, ok := info["nexusim.gateway.v1.GatewayService"]
	if !ok {
		t.Fatalf("expected gateway facade service to be registered")
	}
	for _, method := range facade.Methods {
		if method.Name == "GetSendContext" {
			t.Fatalf("gateway facade must not expose internal GetSendContext method")
		}
	}
}

func assertServiceRegistered(t *testing.T, info map[string]grpc.ServiceInfo, service string) {
	t.Helper()
	if _, ok := info[service]; !ok {
		t.Fatalf("expected %s to be registered", service)
	}
}

func assertServiceNotRegistered(t *testing.T, info map[string]grpc.ServiceInfo, service string) {
	t.Helper()
	if _, ok := info[service]; ok {
		t.Fatalf("expected %s not to be registered", service)
	}
}

type fakeMessageClient struct {
	ctx     context.Context
	request *messagev1.SendMessageRequest
}

func (client *fakeMessageClient) SendMessage(ctx context.Context, in *messagev1.SendMessageRequest, opts ...grpc.CallOption) (*messagev1.SendMessageResponse, error) {
	client.ctx = ctx
	client.request = in
	return &messagev1.SendMessageResponse{MessageId: "msg-1", ConversationId: in.GetConversationId(), ConversationSeq: 1}, nil
}

func (client *fakeMessageClient) EditMessage(context.Context, *messagev1.EditMessageRequest, ...grpc.CallOption) (*messagev1.MessageChangeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (client *fakeMessageClient) RevokeMessage(context.Context, *messagev1.RevokeMessageRequest, ...grpc.CallOption) (*messagev1.MessageChangeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (client *fakeMessageClient) DeleteMessage(context.Context, *messagev1.DeleteMessageRequest, ...grpc.CallOption) (*messagev1.MessageChangeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

type fakeContactsClient struct {
	ctx         context.Context
	sendRequest *contactsv1.SendContactRequestRequest
}

func (client *fakeContactsClient) SendContactRequest(ctx context.Context, in *contactsv1.SendContactRequestRequest, opts ...grpc.CallOption) (*contactsv1.SendContactRequestResponse, error) {
	client.ctx = ctx
	client.sendRequest = in
	return &contactsv1.SendContactRequestResponse{
		RequestId:      "contact-request-1",
		TenantId:       in.GetAuthContext().GetTenantId(),
		SenderUserId:   in.GetAuthContext().GetUserId(),
		ReceiverUserId: in.GetTargetUserId(),
		Status:         contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_PENDING,
	}, nil
}

func (client *fakeContactsClient) RespondContactRequest(context.Context, *contactsv1.RespondContactRequestRequest, ...grpc.CallOption) (*contactsv1.RespondContactRequestResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (client *fakeContactsClient) CancelContactRequest(context.Context, *contactsv1.CancelContactRequestRequest, ...grpc.CallOption) (*contactsv1.CancelContactRequestResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (client *fakeContactsClient) ListContactRequests(context.Context, *contactsv1.ListContactRequestsRequest, ...grpc.CallOption) (*contactsv1.ListContactRequestsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (client *fakeContactsClient) ListContacts(context.Context, *contactsv1.ListContactsRequest, ...grpc.CallOption) (*contactsv1.ListContactsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (client *fakeContactsClient) GetContactState(context.Context, *contactsv1.GetContactStateRequest, ...grpc.CallOption) (*contactsv1.GetContactStateResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (client *fakeContactsClient) DeleteContact(context.Context, *contactsv1.DeleteContactRequest, ...grpc.CallOption) (*contactsv1.DeleteContactResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (client *fakeContactsClient) BlockContact(context.Context, *contactsv1.BlockContactRequest, ...grpc.CallOption) (*contactsv1.BlockContactResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (client *fakeContactsClient) UnblockContact(context.Context, *contactsv1.UnblockContactRequest, ...grpc.CallOption) (*contactsv1.UnblockContactResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (client *fakeContactsClient) UpdateContactRemark(context.Context, *contactsv1.UpdateContactRemarkRequest, ...grpc.CallOption) (*contactsv1.UpdateContactRemarkResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

type fakeReceiptClient struct {
	ctx         context.Context
	listRequest *receiptv1.ListConversationsRequest
}

func (client *fakeReceiptClient) MarkRead(context.Context, *receiptv1.MarkReadRequest, ...grpc.CallOption) (*receiptv1.MarkReadResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (client *fakeReceiptClient) GetReceiptState(context.Context, *receiptv1.GetReceiptStateRequest, ...grpc.CallOption) (*receiptv1.GetReceiptStateResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (client *fakeReceiptClient) ListReceiptStates(context.Context, *receiptv1.ListReceiptStatesRequest, ...grpc.CallOption) (*receiptv1.ListReceiptStatesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (client *fakeReceiptClient) ListConversations(ctx context.Context, in *receiptv1.ListConversationsRequest, opts ...grpc.CallOption) (*receiptv1.ListConversationsResponse, error) {
	client.ctx = ctx
	client.listRequest = in
	return &receiptv1.ListConversationsResponse{}, nil
}

func (client *fakeReceiptClient) ArchiveConversation(context.Context, *receiptv1.ArchiveConversationRequest, ...grpc.CallOption) (*receiptv1.ArchiveConversationResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (client *fakeReceiptClient) PinConversation(context.Context, *receiptv1.PinConversationRequest, ...grpc.CallOption) (*receiptv1.PinConversationResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (client *fakeReceiptClient) MuteConversation(context.Context, *receiptv1.MuteConversationRequest, ...grpc.CallOption) (*receiptv1.MuteConversationResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}
