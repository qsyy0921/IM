package grpc

import (
	"context"
	"testing"
	"time"

	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	receiptv1 "github.com/qsyy0921/IM/api/proto/nexusim/receipt/v1"
	gatewayauth "github.com/qsyy0921/IM/internal/gatewayauth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

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
