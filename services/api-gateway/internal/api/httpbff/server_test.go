package httpbff

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	contactsv1 "github.com/qsyy0921/IM/api/proto/nexusim/contacts/v1"
	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	identityv1 "github.com/qsyy0921/IM/api/proto/nexusim/identity/v1"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	receiptv1 "github.com/qsyy0921/IM/api/proto/nexusim/receipt/v1"
	gatewayauth "github.com/qsyy0921/IM/internal/gatewayauth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestLoginEndpointUsesProtoJSON(t *testing.T) {
	gateway := &fakeGateway{
		login: func(_ context.Context, request *identityv1.LoginRequest) (*identityv1.LoginResponse, error) {
			if request.GetTenantId() != "tenant-1" || request.GetUserId() != "user-1" || request.GetDeviceId() != "web-1" {
				t.Fatalf("unexpected login request: %+v", request)
			}
			if request.GetAudience() != "api-gateway" {
				t.Fatalf("expected BFF login to force api-gateway audience, got %q", request.GetAudience())
			}
			return &identityv1.LoginResponse{
				TenantId:     request.GetTenantId(),
				UserId:       request.GetUserId(),
				DeviceId:     request.GetDeviceId(),
				SessionId:    "session-1",
				Audience:     request.GetAudience(),
				TokenType:    "Bearer",
				GatewayToken: "gateway-token",
				RefreshToken: "refresh-token",
			}, nil
		},
	}
	handler := NewServer(Config{Gateway: gateway, PushTokens: NewHMACPushTokenIssuer("push-secret", time.Minute)})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{
		"tenant_id":"tenant-1",
		"user_id":"user-1",
		"password":"pw",
		"device_id":"web-1"
	}`))

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"gateway_token":"gateway-token"`) {
		t.Fatalf("expected proto json response, got %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"push_gateway_token"`) ||
		!strings.Contains(response.Body.String(), `"push_gateway_audience":"push-gateway"`) {
		t.Fatalf("expected push gateway token fields, got %s", response.Body.String())
	}
}

func TestSendMessageForwardsAuthMetadata(t *testing.T) {
	gateway := &fakeGateway{
		sendMessage: func(ctx context.Context, request *messagev1.SendMessageRequest) (*messagev1.SendMessageResponse, error) {
			if request.GetConversationId() != "conv-1" || request.GetClientMsgId() != "client-1" {
				t.Fatalf("unexpected send request: %+v", request)
			}
			md, ok := metadata.FromIncomingContext(ctx)
			if !ok {
				t.Fatalf("expected incoming metadata")
			}
			if got := firstMetadata(md, "authorization"); got != "Bearer token-1" {
				t.Fatalf("authorization metadata=%q", got)
			}
			if got := firstMetadata(md, "x-nexusim-request-id"); got != "request-1" {
				t.Fatalf("request id metadata=%q", got)
			}
			return &messagev1.SendMessageResponse{
				MessageId:       "msg-1",
				ConversationId:  "conv-1",
				ConversationSeq: 7,
			}, nil
		},
	}
	handler := NewServer(Config{Gateway: gateway})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/messages/send", strings.NewReader(`{
		"conversation_id":"conv-1",
		"client_msg_id":"client-1",
		"message_type":"TEXT",
		"payload":{"text":"hello"}
	}`))
	request.Header.Set("Authorization", "Bearer token-1")
	request.Header.Set("X-NexusIM-Request-ID", "request-1")

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"conversation_seq":"7"`) {
		t.Fatalf("expected int64 proto json string response, got %s", response.Body.String())
	}
}

func TestConversationMessagesMapsToPullInbox(t *testing.T) {
	gateway := &fakeGateway{
		pullInbox: func(_ context.Context, request *deliveryv1.PullInboxRequest) (*deliveryv1.PullInboxResponse, error) {
			if request.GetConversationId() != "conv/1" || request.GetAfterSeq() != 3 || request.GetLimit() != 20 {
				t.Fatalf("unexpected pull request: %+v", request)
			}
			return &deliveryv1.PullInboxResponse{
				NextSeq: 4,
				HasMore: false,
			}, nil
		},
	}
	handler := NewServer(Config{Gateway: gateway})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/conversations/conv%2F1/messages?after_seq=3&limit=20", nil)

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"next_seq":"4"`) {
		t.Fatalf("expected pull response, got %s", response.Body.String())
	}
}

func TestMeEndpointAuthenticatesToken(t *testing.T) {
	authenticator := &fakeAuthenticator{
		auth: gatewayauth.AuthContext{
			TenantID:  "tenant-1",
			UserID:    "user-1",
			DeviceID:  "android-1",
			SessionID: "session-1",
		},
	}
	handler := NewServer(Config{Authenticator: authenticator})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	request.Header.Set("X-NexusIM-Gateway-Token", "token-1")

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if authenticator.authorization != "Bearer token-1" {
		t.Fatalf("authorization passed to authenticator=%q", authenticator.authorization)
	}
	if !strings.Contains(response.Body.String(), `"device_id":"android-1"`) {
		t.Fatalf("expected me response, got %s", response.Body.String())
	}
}

func TestCORSPreflightRequiresAllowedOrigin(t *testing.T) {
	handler := NewServer(Config{AllowedOrigins: []string{"http://localhost:5173"}})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/api/auth/login", nil)
	request.Header.Set("Origin", "http://evil.example")

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden preflight, got status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCORSPreflightAllowsConfiguredOrigin(t *testing.T) {
	handler := NewServer(Config{AllowedOrigins: []string{"http://localhost:5173"}})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/api/auth/login", nil)
	request.Header.Set("Origin", "http://localhost:5173")

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected no content preflight, got status=%d body=%s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("allow origin=%q", got)
	}
}

func TestGatewayErrorMapsToHTTPStatus(t *testing.T) {
	gateway := &fakeGateway{
		listContacts: func(context.Context, *contactsv1.ListContactsRequest) (*contactsv1.ListContactsResponse, error) {
			return nil, status.Error(codes.Unauthenticated, "gateway auth failed")
		},
	}
	handler := NewServer(Config{Gateway: gateway})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/contacts", nil)

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"code":"Unauthenticated"`) {
		t.Fatalf("expected public error code, got %s", response.Body.String())
	}
}

type fakeGateway struct {
	login             func(context.Context, *identityv1.LoginRequest) (*identityv1.LoginResponse, error)
	refresh           func(context.Context, *identityv1.RefreshGatewayTokenRequest) (*identityv1.RefreshGatewayTokenResponse, error)
	issueGatewayToken func(context.Context, *identityv1.IssueGatewayTokenRequest) (*identityv1.IssueGatewayTokenResponse, error)
	sendMessage       func(context.Context, *messagev1.SendMessageRequest) (*messagev1.SendMessageResponse, error)
	pullInbox         func(context.Context, *deliveryv1.PullInboxRequest) (*deliveryv1.PullInboxResponse, error)
	ackDelivery       func(context.Context, *deliveryv1.AckDeliveryRequest) (*deliveryv1.AckDeliveryResponse, error)
	listConversations func(context.Context, *receiptv1.ListConversationsRequest) (*receiptv1.ListConversationsResponse, error)
	listReceiptStates func(context.Context, *receiptv1.ListReceiptStatesRequest) (*receiptv1.ListReceiptStatesResponse, error)
	listContacts      func(context.Context, *contactsv1.ListContactsRequest) (*contactsv1.ListContactsResponse, error)
}

func (gateway *fakeGateway) Login(ctx context.Context, request *identityv1.LoginRequest) (*identityv1.LoginResponse, error) {
	if gateway.login == nil {
		return nil, status.Error(codes.Unimplemented, "login not implemented")
	}
	return gateway.login(ctx, request)
}

func (gateway *fakeGateway) RefreshGatewayToken(ctx context.Context, request *identityv1.RefreshGatewayTokenRequest) (*identityv1.RefreshGatewayTokenResponse, error) {
	if gateway.refresh == nil {
		return nil, status.Error(codes.Unimplemented, "refresh not implemented")
	}
	return gateway.refresh(ctx, request)
}

func (gateway *fakeGateway) IssueGatewayToken(ctx context.Context, request *identityv1.IssueGatewayTokenRequest) (*identityv1.IssueGatewayTokenResponse, error) {
	if gateway.issueGatewayToken == nil {
		return nil, status.Error(codes.Unimplemented, "issue gateway token not implemented")
	}
	return gateway.issueGatewayToken(ctx, request)
}

func (gateway *fakeGateway) SendMessage(ctx context.Context, request *messagev1.SendMessageRequest) (*messagev1.SendMessageResponse, error) {
	if gateway.sendMessage == nil {
		return nil, status.Error(codes.Unimplemented, "send message not implemented")
	}
	return gateway.sendMessage(ctx, request)
}

func (gateway *fakeGateway) PullInbox(ctx context.Context, request *deliveryv1.PullInboxRequest) (*deliveryv1.PullInboxResponse, error) {
	if gateway.pullInbox == nil {
		return nil, status.Error(codes.Unimplemented, "pull inbox not implemented")
	}
	return gateway.pullInbox(ctx, request)
}

func (gateway *fakeGateway) AckDelivery(ctx context.Context, request *deliveryv1.AckDeliveryRequest) (*deliveryv1.AckDeliveryResponse, error) {
	if gateway.ackDelivery == nil {
		return nil, status.Error(codes.Unimplemented, "ack delivery not implemented")
	}
	return gateway.ackDelivery(ctx, request)
}

func (gateway *fakeGateway) ListConversations(ctx context.Context, request *receiptv1.ListConversationsRequest) (*receiptv1.ListConversationsResponse, error) {
	if gateway.listConversations == nil {
		return nil, status.Error(codes.Unimplemented, "list conversations not implemented")
	}
	return gateway.listConversations(ctx, request)
}

func (gateway *fakeGateway) ListReceiptStates(ctx context.Context, request *receiptv1.ListReceiptStatesRequest) (*receiptv1.ListReceiptStatesResponse, error) {
	if gateway.listReceiptStates == nil {
		return nil, status.Error(codes.Unimplemented, "list receipt states not implemented")
	}
	return gateway.listReceiptStates(ctx, request)
}

func (gateway *fakeGateway) ListContacts(ctx context.Context, request *contactsv1.ListContactsRequest) (*contactsv1.ListContactsResponse, error) {
	if gateway.listContacts == nil {
		return nil, status.Error(codes.Unimplemented, "list contacts not implemented")
	}
	return gateway.listContacts(ctx, request)
}

type fakeAuthenticator struct {
	auth          gatewayauth.AuthContext
	err           error
	authorization string
}

func (authenticator *fakeAuthenticator) Authenticate(request *http.Request) (gatewayauth.AuthContext, error) {
	authenticator.authorization = request.Header.Get("Authorization")
	if authenticator.err != nil {
		return gatewayauth.AuthContext{}, authenticator.err
	}
	auth := authenticator.auth
	if auth.TraceID == "" {
		auth.TraceID = "trace-test"
	}
	if auth.RequestID == "" {
		auth.RequestID = "request-test-" + time.Now().UTC().Format("150405")
	}
	return auth, nil
}

func firstMetadata(md metadata.MD, key string) string {
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
