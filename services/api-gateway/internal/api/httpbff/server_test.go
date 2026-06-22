package httpbff

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	contactsv1 "github.com/qsyy0921/IM/api/proto/nexusim/contacts/v1"
	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
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

func TestRegisterEndpointForwardsPublicIdentityRequest(t *testing.T) {
	gateway := &fakeGateway{
		registerUser: func(_ context.Context, request *identityv1.RegisterUserRequest) (*identityv1.RegisterUserResponse, error) {
			if request.GetTenantId() != "tenant-1" || request.GetUserId() != "user-new" || request.GetPassword() != "pw" {
				t.Fatalf("unexpected register request: %+v", request)
			}
			return &identityv1.RegisterUserResponse{
				TenantId:        request.GetTenantId(),
				UserId:          request.GetUserId(),
				Status:          identityv1.UserStatus_USER_STATUS_ACTIVE,
				CreatedAtUnixMs: 11,
			}, nil
		},
	}
	handler := NewServer(Config{Gateway: gateway})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{
		"tenant_id":"tenant-1",
		"user_id":"user-new",
		"password":"pw"
	}`))

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"user_id":"user-new"`) {
		t.Fatalf("expected register response, got %s", response.Body.String())
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

func TestCreateConversationEndpointForwardsRequest(t *testing.T) {
	gateway := &fakeGateway{
		createConversation: func(ctx context.Context, request *conversationv1.CreateConversationRequest) (*conversationv1.CreateConversationResponse, error) {
			md, ok := metadata.FromIncomingContext(ctx)
			if !ok || firstMetadata(md, "authorization") != "Bearer token-1" {
				t.Fatalf("expected auth metadata, got %+v", md)
			}
			if request.GetConversationId() != "group-1" ||
				request.GetConversationType() != conversationv1.ConversationType_CONVERSATION_TYPE_GROUP ||
				request.GetIdempotencyKey() != "idem-1" {
				t.Fatalf("unexpected create conversation request: %+v", request)
			}
			return &conversationv1.CreateConversationResponse{
				TenantId:          "tenant-1",
				ConversationId:    request.GetConversationId(),
				ConversationType:  request.GetConversationType(),
				BoundarySeq:       1,
				MemberVersion:     1,
				PermissionVersion: 1,
			}, nil
		},
	}
	handler := NewServer(Config{Gateway: gateway})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/conversations/create", strings.NewReader(`{
		"conversation_id":"group-1",
		"conversation_type":"CONVERSATION_TYPE_GROUP",
		"idempotency_key":"idem-1"
	}`))
	request.Header.Set("Authorization", "Bearer token-1")

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"boundary_seq":"1"`) {
		t.Fatalf("expected create conversation response, got %s", response.Body.String())
	}
}

func TestOpenDirectConversationRequiresActiveContact(t *testing.T) {
	authenticator := &fakeAuthenticator{auth: gatewayauth.AuthContext{
		TenantID:  "tenant-1",
		UserID:    "user-a",
		DeviceID:  "web-1",
		SessionID: "session-1",
	}}
	expectedConversationID := directConversationID("tenant-1", "user-a", "user-b")
	gateway := &fakeGateway{
		getContactState: func(ctx context.Context, request *contactsv1.GetContactStateRequest) (*contactsv1.GetContactStateResponse, error) {
			md, ok := metadata.FromIncomingContext(ctx)
			if !ok || firstMetadata(md, "authorization") != "Bearer token-1" {
				t.Fatalf("expected forwarded auth metadata, got %+v", md)
			}
			if request.GetOtherUserId() != "user-b" {
				t.Fatalf("unexpected contact state request: %+v", request)
			}
			return &contactsv1.GetContactStateResponse{
				TenantId:      "tenant-1",
				OwnerUserId:   "user-a",
				ContactUserId: "user-b",
				Status:        contactsv1.ContactEdgeStatus_CONTACT_EDGE_STATUS_ACTIVE,
			}, nil
		},
		createConversation: func(ctx context.Context, request *conversationv1.CreateConversationRequest) (*conversationv1.CreateConversationResponse, error) {
			md, ok := metadata.FromIncomingContext(ctx)
			if !ok || firstMetadata(md, "authorization") != "Bearer token-1" {
				t.Fatalf("expected forwarded auth metadata, got %+v", md)
			}
			if request.GetConversationId() != expectedConversationID ||
				request.GetConversationType() != conversationv1.ConversationType_CONVERSATION_TYPE_DIRECT ||
				request.GetDirectPeerUserId() != "user-b" ||
				request.GetIdempotencyKey() != "idem-direct-1" {
				t.Fatalf("unexpected direct create request: %+v", request)
			}
			return &conversationv1.CreateConversationResponse{
				TenantId:          "tenant-1",
				ConversationId:    expectedConversationID,
				ConversationType:  request.GetConversationType(),
				DirectPeerUserId:  request.GetDirectPeerUserId(),
				BoundarySeq:       2,
				MemberVersion:     2,
				PermissionVersion: 2,
			}, nil
		},
	}
	handler := NewServer(Config{Gateway: gateway, Authenticator: authenticator})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/conversations/direct", strings.NewReader(`{
		"peer_user_id":"user-b",
		"idempotency_key":"idem-direct-1"
	}`))
	request.Header.Set("Authorization", "Bearer token-1")

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"conversation_id":"`+expectedConversationID+`"`) ||
		!strings.Contains(response.Body.String(), `"direct_peer_user_id":"user-b"`) {
		t.Fatalf("expected direct conversation response, got %s", response.Body.String())
	}
}

func TestOpenDirectConversationRejectsInactiveContact(t *testing.T) {
	authenticator := &fakeAuthenticator{auth: gatewayauth.AuthContext{
		TenantID:  "tenant-1",
		UserID:    "user-a",
		DeviceID:  "web-1",
		SessionID: "session-1",
	}}
	createCalled := false
	gateway := &fakeGateway{
		getContactState: func(context.Context, *contactsv1.GetContactStateRequest) (*contactsv1.GetContactStateResponse, error) {
			return &contactsv1.GetContactStateResponse{
				TenantId:      "tenant-1",
				OwnerUserId:   "user-a",
				ContactUserId: "user-b",
				Status:        contactsv1.ContactEdgeStatus_CONTACT_EDGE_STATUS_DELETED,
			}, nil
		},
		createConversation: func(context.Context, *conversationv1.CreateConversationRequest) (*conversationv1.CreateConversationResponse, error) {
			createCalled = true
			return &conversationv1.CreateConversationResponse{}, nil
		},
	}
	handler := NewServer(Config{Gateway: gateway, Authenticator: authenticator})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/conversations/direct", strings.NewReader(`{
		"peer_user_id":"user-b"
	}`))
	request.Header.Set("Authorization", "Bearer token-1")

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden, got status=%d body=%s", response.Code, response.Body.String())
	}
	if createCalled {
		t.Fatalf("create conversation should not be called for inactive contact")
	}
}

func TestOpenDirectConversationRequiresIdempotencyKey(t *testing.T) {
	authenticator := &fakeAuthenticator{auth: gatewayauth.AuthContext{
		TenantID:  "tenant-1",
		UserID:    "user-a",
		DeviceID:  "web-1",
		SessionID: "session-1",
	}}
	createCalled := false
	gateway := &fakeGateway{
		getContactState: func(context.Context, *contactsv1.GetContactStateRequest) (*contactsv1.GetContactStateResponse, error) {
			return &contactsv1.GetContactStateResponse{
				TenantId:      "tenant-1",
				OwnerUserId:   "user-a",
				ContactUserId: "user-b",
				Status:        contactsv1.ContactEdgeStatus_CONTACT_EDGE_STATUS_ACTIVE,
			}, nil
		},
		createConversation: func(context.Context, *conversationv1.CreateConversationRequest) (*conversationv1.CreateConversationResponse, error) {
			createCalled = true
			return nil, status.Error(codes.Internal, "should not create direct conversation")
		},
	}
	handler := NewServer(Config{Gateway: gateway, Authenticator: authenticator})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/conversations/direct", strings.NewReader(`{
		"peer_user_id":"user-b"
	}`))
	request.Header.Set("Authorization", "Bearer token-1")

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got status=%d body=%s", response.Code, response.Body.String())
	}
	if createCalled {
		t.Fatalf("create conversation should not be called without idempotency key")
	}
	if !strings.Contains(response.Body.String(), "idempotency_key is required") {
		t.Fatalf("expected idempotency error, got %s", response.Body.String())
	}
}

func TestInviteConversationMemberForwardsJoinCommand(t *testing.T) {
	authenticator := &fakeAuthenticator{auth: gatewayauth.AuthContext{
		TenantID:  "tenant-1",
		UserID:    "owner-1",
		DeviceID:  "web-1",
		SessionID: "session-1",
	}}
	gateway := &fakeGateway{
		createMemberChange: func(ctx context.Context, request *conversationv1.CreateMemberChangeRequest) (*conversationv1.CreateMemberChangeResponse, error) {
			md, ok := metadata.FromIncomingContext(ctx)
			if !ok || firstMetadata(md, "authorization") != "Bearer token-1" {
				t.Fatalf("expected forwarded auth metadata, got %+v", md)
			}
			if request.GetConversationId() != "conv/group" ||
				request.GetTargetUserId() != "user-b" ||
				request.GetChangeType() != conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_JOIN ||
				request.GetTargetRole() != conversationv1.MemberRole_MEMBER_ROLE_MEMBER ||
				request.GetExpectedMemberVersion() != 7 ||
				request.GetIdempotencyKey() != "idem-join-1" ||
				request.GetConflictPolicy() != conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_REJECT ||
				request.GetReason() != "invite from client" {
				t.Fatalf("unexpected member join request: %+v", request)
			}
			return &conversationv1.CreateMemberChangeResponse{
				ChangeId:          "change-1",
				TenantId:          "tenant-1",
				ConversationId:    request.GetConversationId(),
				TargetUserId:      request.GetTargetUserId(),
				ChangeType:        request.GetChangeType(),
				Status:            conversationv1.MemberChangeStatus_MEMBER_CHANGE_STATUS_OUTBOX_ENQUEUED,
				BoundarySeq:       8,
				MemberVersion:     8,
				PermissionVersion: 2,
			}, nil
		},
	}
	handler := NewServer(Config{Gateway: gateway, Authenticator: authenticator})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/conversations/conv%2Fgroup/members/invite", strings.NewReader(`{
		"target_user_id":"user-b",
		"expected_member_version":7,
		"idempotency_key":"idem-join-1",
		"reason":"invite from client"
	}`))
	request.Header.Set("Authorization", "Bearer token-1")

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"change_id":"change-1"`) ||
		!strings.Contains(response.Body.String(), `"member_version":"8"`) {
		t.Fatalf("expected member change response, got %s", response.Body.String())
	}
}

func TestLeaveConversationTargetsAuthenticatedUser(t *testing.T) {
	authenticator := &fakeAuthenticator{auth: gatewayauth.AuthContext{
		TenantID:  "tenant-1",
		UserID:    "user-a",
		DeviceID:  "web-1",
		SessionID: "session-1",
	}}
	gateway := &fakeGateway{
		createMemberChange: func(ctx context.Context, request *conversationv1.CreateMemberChangeRequest) (*conversationv1.CreateMemberChangeResponse, error) {
			md, ok := metadata.FromIncomingContext(ctx)
			if !ok || firstMetadata(md, "authorization") != "Bearer token-1" {
				t.Fatalf("expected forwarded auth metadata, got %+v", md)
			}
			if request.GetConversationId() != "group-1" ||
				request.GetTargetUserId() != "user-a" ||
				request.GetChangeType() != conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_LEAVE ||
				request.GetTargetRole() != conversationv1.MemberRole_MEMBER_ROLE_UNSPECIFIED ||
				request.GetIdempotencyKey() != "idem-leave-1" ||
				request.GetConflictPolicy() != conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_REJECT {
				t.Fatalf("unexpected member leave request: %+v", request)
			}
			return &conversationv1.CreateMemberChangeResponse{
				ChangeId:       "change-leave-1",
				TenantId:       "tenant-1",
				ConversationId: request.GetConversationId(),
				TargetUserId:   request.GetTargetUserId(),
				ChangeType:     request.GetChangeType(),
				Status:         conversationv1.MemberChangeStatus_MEMBER_CHANGE_STATUS_OUTBOX_ENQUEUED,
			}, nil
		},
	}
	handler := NewServer(Config{Gateway: gateway, Authenticator: authenticator})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/conversations/group-1/members/leave", strings.NewReader(`{
		"target_user_id":"user-b",
		"idempotency_key":"idem-leave-1"
	}`))
	request.Header.Set("Authorization", "Bearer token-1")

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"target_user_id":"user-a"`) {
		t.Fatalf("expected leave response for authenticated user, got %s", response.Body.String())
	}
}

func TestListConversationMembersForwardsQuery(t *testing.T) {
	authenticator := &fakeAuthenticator{auth: gatewayauth.AuthContext{
		TenantID:  "tenant-1",
		UserID:    "owner-1",
		DeviceID:  "web-1",
		SessionID: "session-1",
	}}
	gateway := &fakeGateway{
		listConversationMembers: func(ctx context.Context, request *conversationv1.ListConversationMembersRequest) (*conversationv1.ListConversationMembersResponse, error) {
			md, ok := metadata.FromIncomingContext(ctx)
			if !ok || firstMetadata(md, "authorization") != "Bearer token-1" {
				t.Fatalf("expected forwarded auth metadata, got %+v", md)
			}
			if request.GetConversationId() != "group/1" ||
				request.GetPageSize() != 20 ||
				request.GetPageToken() != "cursor-1" ||
				request.GetRoleFilter() != conversationv1.MemberRole_MEMBER_ROLE_ADMIN ||
				request.GetUserIdPrefix() != "user-" ||
				request.GetSort() != conversationv1.ConversationMemberListSort_CONVERSATION_MEMBER_LIST_SORT_ROLE_USER_ID_ASC {
				t.Fatalf("unexpected list members request: %+v", request)
			}
			return &conversationv1.ListConversationMembersResponse{
				TenantId:          "tenant-1",
				ConversationId:    request.GetConversationId(),
				MemberVersion:     9,
				PermissionVersion: 3,
				Members: []*conversationv1.ConversationMember{
					{
						UserId:            "owner-1",
						Role:              conversationv1.MemberRole_MEMBER_ROLE_OWNER,
						Status:            conversationv1.MemberStatus_MEMBER_STATUS_ACTIVE,
						JoinSeq:           1,
						MemberVersion:     9,
						PermissionVersion: 3,
					},
				},
			}, nil
		},
	}
	handler := NewServer(Config{Gateway: gateway, Authenticator: authenticator})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/conversations/group%2F1/members?page_size=20&page_token=cursor-1&role=ADMIN&user_id_prefix=user-", nil)
	request.Header.Set("Authorization", "Bearer token-1")

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"user_id":"owner-1"`) ||
		!strings.Contains(response.Body.String(), `"member_version":"9"`) {
		t.Fatalf("expected members response, got %s", response.Body.String())
	}
}

func TestRemoveConversationMemberForwardsRemoveCommand(t *testing.T) {
	authenticator := &fakeAuthenticator{auth: gatewayauth.AuthContext{
		TenantID:  "tenant-1",
		UserID:    "owner-1",
		DeviceID:  "web-1",
		SessionID: "session-1",
	}}
	gateway := &fakeGateway{
		createMemberChange: func(_ context.Context, request *conversationv1.CreateMemberChangeRequest) (*conversationv1.CreateMemberChangeResponse, error) {
			if request.GetConversationId() != "group-1" ||
				request.GetTargetUserId() != "user-b" ||
				request.GetChangeType() != conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_REMOVE ||
				request.GetExpectedMemberVersion() != 9 ||
				request.GetIdempotencyKey() != "idem-remove-1" ||
				request.GetConflictPolicy() != conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_REJECT {
				t.Fatalf("unexpected remove request: %+v", request)
			}
			return &conversationv1.CreateMemberChangeResponse{
				ChangeId:       "change-remove-1",
				TenantId:       "tenant-1",
				ConversationId: request.GetConversationId(),
				TargetUserId:   request.GetTargetUserId(),
				ChangeType:     request.GetChangeType(),
				Status:         conversationv1.MemberChangeStatus_MEMBER_CHANGE_STATUS_OUTBOX_ENQUEUED,
				MemberVersion:  10,
			}, nil
		},
	}
	handler := NewServer(Config{Gateway: gateway, Authenticator: authenticator})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/conversations/group-1/members/remove", strings.NewReader(`{
		"target_user_id":"user-b",
		"expected_member_version":9,
		"idempotency_key":"idem-remove-1"
	}`))
	request.Header.Set("Authorization", "Bearer token-1")

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"change_type":"MEMBER_CHANGE_TYPE_REMOVE"`) {
		t.Fatalf("expected remove response, got %s", response.Body.String())
	}
}

func TestUpdateConversationMemberRoleRejectsOwnerRole(t *testing.T) {
	authenticator := &fakeAuthenticator{auth: gatewayauth.AuthContext{UserID: "owner-1"}}
	gateway := &fakeGateway{
		createMemberChange: func(context.Context, *conversationv1.CreateMemberChangeRequest) (*conversationv1.CreateMemberChangeResponse, error) {
			t.Fatalf("create member change should not be called for owner role change")
			return nil, nil
		},
	}
	handler := NewServer(Config{Gateway: gateway, Authenticator: authenticator})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/conversations/group-1/members/role", strings.NewReader(`{
		"target_user_id":"user-b",
		"target_role":"OWNER"
	}`))
	request.Header.Set("Authorization", "Bearer token-1")

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestUpdateConversationMemberRoleForwardsRoleChange(t *testing.T) {
	authenticator := &fakeAuthenticator{auth: gatewayauth.AuthContext{UserID: "owner-1"}}
	gateway := &fakeGateway{
		createMemberChange: func(_ context.Context, request *conversationv1.CreateMemberChangeRequest) (*conversationv1.CreateMemberChangeResponse, error) {
			if request.GetConversationId() != "group-1" ||
				request.GetTargetUserId() != "user-b" ||
				request.GetChangeType() != conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_ROLE_CHANGED ||
				request.GetTargetRole() != conversationv1.MemberRole_MEMBER_ROLE_ADMIN ||
				request.GetExpectedMemberVersion() != 10 ||
				request.GetIdempotencyKey() != "idem-role-1" {
				t.Fatalf("unexpected role request: %+v", request)
			}
			return &conversationv1.CreateMemberChangeResponse{
				ChangeId:       "change-role-1",
				TenantId:       "tenant-1",
				ConversationId: request.GetConversationId(),
				TargetUserId:   request.GetTargetUserId(),
				ChangeType:     request.GetChangeType(),
				Status:         conversationv1.MemberChangeStatus_MEMBER_CHANGE_STATUS_OUTBOX_ENQUEUED,
				MemberVersion:  11,
			}, nil
		},
	}
	handler := NewServer(Config{Gateway: gateway, Authenticator: authenticator})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/conversations/group-1/members/role", strings.NewReader(`{
		"target_user_id":"user-b",
		"target_role":"ADMIN",
		"expected_member_version":10,
		"idempotency_key":"idem-role-1"
	}`))
	request.Header.Set("Authorization", "Bearer token-1")

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"change_type":"MEMBER_CHANGE_TYPE_ROLE_CHANGED"`) {
		t.Fatalf("expected role response, got %s", response.Body.String())
	}
}

func TestTransferConversationOwnerForwardsTransfer(t *testing.T) {
	authenticator := &fakeAuthenticator{auth: gatewayauth.AuthContext{UserID: "owner-1"}}
	gateway := &fakeGateway{
		transferConversationOwner: func(_ context.Context, request *conversationv1.TransferConversationOwnerRequest) (*conversationv1.TransferConversationOwnerResponse, error) {
			if request.GetConversationId() != "group-1" ||
				request.GetNewOwnerUserId() != "user-b" ||
				request.GetExpectedMemberVersion() != 11 ||
				request.GetIdempotencyKey() != "idem-transfer-1" {
				t.Fatalf("unexpected transfer request: %+v", request)
			}
			return &conversationv1.TransferConversationOwnerResponse{
				ChangeId:            "change-transfer-1",
				TenantId:            "tenant-1",
				ConversationId:      request.GetConversationId(),
				PreviousOwnerUserId: "owner-1",
				NewOwnerUserId:      request.GetNewOwnerUserId(),
				Status:              conversationv1.MemberChangeStatus_MEMBER_CHANGE_STATUS_OUTBOX_ENQUEUED,
				MemberVersion:       12,
			}, nil
		},
	}
	handler := NewServer(Config{Gateway: gateway, Authenticator: authenticator})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/conversations/group-1/owner/transfer", strings.NewReader(`{
		"new_owner_user_id":"user-b",
		"expected_member_version":11,
		"idempotency_key":"idem-transfer-1"
	}`))
	request.Header.Set("Authorization", "Bearer token-1")

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"new_owner_user_id":"user-b"`) {
		t.Fatalf("expected transfer response, got %s", response.Body.String())
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

func TestLogoutRevokesCurrentAuthenticatedSession(t *testing.T) {
	authenticator := &fakeAuthenticator{
		auth: gatewayauth.AuthContext{
			TenantID:  "tenant-token",
			UserID:    "user-token",
			DeviceID:  "device-token",
			SessionID: "session-token",
			TraceID:   "trace-token",
			RequestID: "request-token",
		},
	}
	gateway := &fakeGateway{
		revokeSession: func(_ context.Context, request *identityv1.RevokeSessionRequest) (*identityv1.RevokeSessionResponse, error) {
			if request.GetAdminContext().GetTenantId() != "tenant-token" ||
				request.GetAdminContext().GetOperatorUserId() != "user-token" ||
				request.GetAdminContext().GetTraceId() != "trace-token" ||
				request.GetAdminContext().GetRequestId() != "request-token" {
				t.Fatalf("unexpected admin context: %+v", request.GetAdminContext())
			}
			if request.GetUserId() != "user-token" ||
				request.GetDeviceId() != "device-token" ||
				request.GetSessionId() != "session-token" {
				t.Fatalf("logout must target current authenticated session, got %+v", request)
			}
			if request.GetReason() != "client logout" {
				t.Fatalf("logout reason=%q", request.GetReason())
			}
			return &identityv1.RevokeSessionResponse{
				TenantId:        request.GetAdminContext().GetTenantId(),
				UserId:          request.GetUserId(),
				DeviceId:        request.GetDeviceId(),
				SessionId:       request.GetSessionId(),
				Status:          identityv1.SessionStatus_SESSION_STATUS_REVOKED,
				RevokedAtUnixMs: 1_800_000_000_000,
			}, nil
		},
	}
	handler := NewServer(Config{Gateway: gateway, Authenticator: authenticator})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/logout", strings.NewReader(`{
		"tenant_id":"tenant-body",
		"user_id":"user-body",
		"device_id":"device-body",
		"session_id":"session-body"
	}`))
	request.Header.Set("X-NexusIM-Gateway-Token", "token-1")

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if authenticator.authorization != "Bearer token-1" {
		t.Fatalf("authorization passed to authenticator=%q", authenticator.authorization)
	}
	if !strings.Contains(response.Body.String(), `"session_id":"session-token"`) ||
		!strings.Contains(response.Body.String(), `"status":"SESSION_STATUS_REVOKED"`) {
		t.Fatalf("expected revoke response, got %s", response.Body.String())
	}
}

func TestLogoutRequiresAuthenticatedSession(t *testing.T) {
	gatewayCalled := false
	handler := NewServer(Config{
		Authenticator: &fakeAuthenticator{err: gatewayauth.ErrPermissionDenied},
		Gateway: &fakeGateway{
			revokeSession: func(context.Context, *identityv1.RevokeSessionRequest) (*identityv1.RevokeSessionResponse, error) {
				gatewayCalled = true
				return &identityv1.RevokeSessionResponse{}, nil
			},
		},
	})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	request.Header.Set("Authorization", "Bearer invalid")

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized logout, got status=%d body=%s", response.Code, response.Body.String())
	}
	if gatewayCalled {
		t.Fatalf("gateway revoke should not be called when authentication fails")
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

func TestListContactRequestsMapsQueryEnums(t *testing.T) {
	gateway := &fakeGateway{
		listContactRequests: func(_ context.Context, request *contactsv1.ListContactRequestsRequest) (*contactsv1.ListContactRequestsResponse, error) {
			if request.GetDirection() != contactsv1.ContactRequestListDirection_CONTACT_REQUEST_LIST_DIRECTION_INCOMING {
				t.Fatalf("direction=%v", request.GetDirection())
			}
			if request.GetStatus() != contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_PENDING {
				t.Fatalf("status=%v", request.GetStatus())
			}
			if request.GetPageSize() != 20 || request.GetPageToken() != "cursor-1" {
				t.Fatalf("unexpected paging: %+v", request)
			}
			return &contactsv1.ListContactRequestsResponse{
				TenantId:  "tenant-1",
				UserId:    "user-1",
				Direction: request.GetDirection(),
				Status:    request.GetStatus(),
				Requests: []*contactsv1.ContactRequestItem{{
					RequestId:      "request-1",
					SenderUserId:   "user-b",
					ReceiverUserId: "user-1",
					Status:         request.GetStatus(),
				}},
			}, nil
		},
	}
	handler := NewServer(Config{Gateway: gateway})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/contact-requests?direction=incoming&status=pending&page_size=20&page_token=cursor-1", nil)

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"direction":"CONTACT_REQUEST_LIST_DIRECTION_INCOMING"`) ||
		!strings.Contains(response.Body.String(), `"request_id":"request-1"`) {
		t.Fatalf("expected contact request list response, got %s", response.Body.String())
	}
}

func TestSendContactRequestForwardsProtoJSON(t *testing.T) {
	gateway := &fakeGateway{
		sendContactRequest: func(ctx context.Context, request *contactsv1.SendContactRequestRequest) (*contactsv1.SendContactRequestResponse, error) {
			if request.GetTargetUserId() != "user-b" ||
				request.GetIdempotencyKey() != "idem-1" ||
				request.GetMessage() != "hello" ||
				request.GetSourceType() != contactsv1.ContactRequestSourceType_CONTACT_REQUEST_SOURCE_TYPE_DIRECT {
				t.Fatalf("unexpected contact request: %+v", request)
			}
			md, ok := metadata.FromIncomingContext(ctx)
			if !ok || firstMetadata(md, "authorization") != "Bearer token-1" {
				t.Fatalf("expected forwarded auth metadata, got %+v", md)
			}
			return &contactsv1.SendContactRequestResponse{
				RequestId:      "request-1",
				TenantId:       "tenant-1",
				SenderUserId:   "user-a",
				ReceiverUserId: "user-b",
				Status:         contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_PENDING,
				SourceType:     request.GetSourceType(),
			}, nil
		},
	}
	handler := NewServer(Config{Gateway: gateway})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/contact-requests/send", strings.NewReader(`{
		"target_user_id":"user-b",
		"idempotency_key":"idem-1",
		"message":"hello",
		"source_type":"CONTACT_REQUEST_SOURCE_TYPE_DIRECT"
	}`))
	request.Header.Set("Authorization", "Bearer token-1")

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"request_id":"request-1"`) ||
		!strings.Contains(response.Body.String(), `"status":"CONTACT_REQUEST_STATUS_PENDING"`) {
		t.Fatalf("expected send contact request response, got %s", response.Body.String())
	}
}

func TestHTTPBFFMetricsRecordsLowCardinalityRoute(t *testing.T) {
	metrics := &fakeMetricsRecorder{}
	handler := NewServer(Config{Metrics: metrics})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(metrics.records) != 1 {
		t.Fatalf("expected one metrics record, got %+v", metrics.records)
	}
	record := metrics.records[0]
	if record.route != "health" || record.method != http.MethodGet || record.statusCode != http.StatusOK {
		t.Fatalf("unexpected metrics record: %+v", record)
	}
}

func TestRateLimiterRejectsBFFRequestBeforeGatewayCall(t *testing.T) {
	gatewayCalled := false
	limiter := &fakeRateLimiter{err: status.Error(codes.ResourceExhausted, "rate limit exceeded")}
	metrics := &fakeMetricsRecorder{}
	handler := NewServer(Config{
		RateLimiter: limiter,
		Metrics:     metrics,
		Gateway: &fakeGateway{
			sendMessage: func(context.Context, *messagev1.SendMessageRequest) (*messagev1.SendMessageResponse, error) {
				gatewayCalled = true
				return &messagev1.SendMessageResponse{}, nil
			},
		},
	})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/messages/send", strings.NewReader(`{
		"conversation_id":"conv-1",
		"client_msg_id":"client-1",
		"message_type":"TEXT",
		"payload":{"text":"hello"}
	}`))
	request.Header.Set("Authorization", "Bearer token-1")

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("expected rate limit status, got %d body=%s", response.Code, response.Body.String())
	}
	if gatewayCalled {
		t.Fatalf("gateway should not be called after BFF rate-limit rejection")
	}
	if len(limiter.methods) != 1 || limiter.methods[0] != "/nexusim.api_gateway.bff.HTTPBFF/messages.send" {
		t.Fatalf("unexpected rate-limit methods: %+v", limiter.methods)
	}
	if len(metrics.records) != 1 || metrics.records[0].route != "messages.send" || metrics.records[0].statusCode != http.StatusTooManyRequests {
		t.Fatalf("unexpected metrics records: %+v", metrics.records)
	}
	if !strings.Contains(response.Body.String(), `"code":"ResourceExhausted"`) {
		t.Fatalf("expected public rate-limit error body, got %s", response.Body.String())
	}
}

type fakeGateway struct {
	registerUser              func(context.Context, *identityv1.RegisterUserRequest) (*identityv1.RegisterUserResponse, error)
	login                     func(context.Context, *identityv1.LoginRequest) (*identityv1.LoginResponse, error)
	refresh                   func(context.Context, *identityv1.RefreshGatewayTokenRequest) (*identityv1.RefreshGatewayTokenResponse, error)
	issueGatewayToken         func(context.Context, *identityv1.IssueGatewayTokenRequest) (*identityv1.IssueGatewayTokenResponse, error)
	revokeSession             func(context.Context, *identityv1.RevokeSessionRequest) (*identityv1.RevokeSessionResponse, error)
	createConversation        func(context.Context, *conversationv1.CreateConversationRequest) (*conversationv1.CreateConversationResponse, error)
	createMemberChange        func(context.Context, *conversationv1.CreateMemberChangeRequest) (*conversationv1.CreateMemberChangeResponse, error)
	listConversationMembers   func(context.Context, *conversationv1.ListConversationMembersRequest) (*conversationv1.ListConversationMembersResponse, error)
	transferConversationOwner func(context.Context, *conversationv1.TransferConversationOwnerRequest) (*conversationv1.TransferConversationOwnerResponse, error)
	sendMessage               func(context.Context, *messagev1.SendMessageRequest) (*messagev1.SendMessageResponse, error)
	pullInbox                 func(context.Context, *deliveryv1.PullInboxRequest) (*deliveryv1.PullInboxResponse, error)
	ackDelivery               func(context.Context, *deliveryv1.AckDeliveryRequest) (*deliveryv1.AckDeliveryResponse, error)
	listConversations         func(context.Context, *receiptv1.ListConversationsRequest) (*receiptv1.ListConversationsResponse, error)
	listReceiptStates         func(context.Context, *receiptv1.ListReceiptStatesRequest) (*receiptv1.ListReceiptStatesResponse, error)
	sendContactRequest        func(context.Context, *contactsv1.SendContactRequestRequest) (*contactsv1.SendContactRequestResponse, error)
	respondContactRequest     func(context.Context, *contactsv1.RespondContactRequestRequest) (*contactsv1.RespondContactRequestResponse, error)
	cancelContactRequest      func(context.Context, *contactsv1.CancelContactRequestRequest) (*contactsv1.CancelContactRequestResponse, error)
	listContactRequests       func(context.Context, *contactsv1.ListContactRequestsRequest) (*contactsv1.ListContactRequestsResponse, error)
	listContacts              func(context.Context, *contactsv1.ListContactsRequest) (*contactsv1.ListContactsResponse, error)
	getContactState           func(context.Context, *contactsv1.GetContactStateRequest) (*contactsv1.GetContactStateResponse, error)
	deleteContact             func(context.Context, *contactsv1.DeleteContactRequest) (*contactsv1.DeleteContactResponse, error)
	blockContact              func(context.Context, *contactsv1.BlockContactRequest) (*contactsv1.BlockContactResponse, error)
	unblockContact            func(context.Context, *contactsv1.UnblockContactRequest) (*contactsv1.UnblockContactResponse, error)
	updateContactRemark       func(context.Context, *contactsv1.UpdateContactRemarkRequest) (*contactsv1.UpdateContactRemarkResponse, error)
	updateContactGroup        func(context.Context, *contactsv1.UpdateContactGroupRequest) (*contactsv1.UpdateContactGroupResponse, error)
}

func (gateway *fakeGateway) RegisterUser(ctx context.Context, request *identityv1.RegisterUserRequest) (*identityv1.RegisterUserResponse, error) {
	if gateway.registerUser == nil {
		return nil, status.Error(codes.Unimplemented, "register user not implemented")
	}
	return gateway.registerUser(ctx, request)
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

func (gateway *fakeGateway) RevokeSession(ctx context.Context, request *identityv1.RevokeSessionRequest) (*identityv1.RevokeSessionResponse, error) {
	if gateway.revokeSession == nil {
		return nil, status.Error(codes.Unimplemented, "revoke session not implemented")
	}
	return gateway.revokeSession(ctx, request)
}

func (gateway *fakeGateway) CreateConversation(ctx context.Context, request *conversationv1.CreateConversationRequest) (*conversationv1.CreateConversationResponse, error) {
	if gateway.createConversation == nil {
		return nil, status.Error(codes.Unimplemented, "create conversation not implemented")
	}
	return gateway.createConversation(ctx, request)
}

func (gateway *fakeGateway) CreateMemberChange(ctx context.Context, request *conversationv1.CreateMemberChangeRequest) (*conversationv1.CreateMemberChangeResponse, error) {
	if gateway.createMemberChange == nil {
		return nil, status.Error(codes.Unimplemented, "create member change not implemented")
	}
	return gateway.createMemberChange(ctx, request)
}

func (gateway *fakeGateway) ListConversationMembers(ctx context.Context, request *conversationv1.ListConversationMembersRequest) (*conversationv1.ListConversationMembersResponse, error) {
	if gateway.listConversationMembers == nil {
		return nil, status.Error(codes.Unimplemented, "list conversation members not implemented")
	}
	return gateway.listConversationMembers(ctx, request)
}

func (gateway *fakeGateway) TransferConversationOwner(ctx context.Context, request *conversationv1.TransferConversationOwnerRequest) (*conversationv1.TransferConversationOwnerResponse, error) {
	if gateway.transferConversationOwner == nil {
		return nil, status.Error(codes.Unimplemented, "transfer conversation owner not implemented")
	}
	return gateway.transferConversationOwner(ctx, request)
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

func (gateway *fakeGateway) SendContactRequest(ctx context.Context, request *contactsv1.SendContactRequestRequest) (*contactsv1.SendContactRequestResponse, error) {
	if gateway.sendContactRequest == nil {
		return nil, status.Error(codes.Unimplemented, "send contact request not implemented")
	}
	return gateway.sendContactRequest(ctx, request)
}

func (gateway *fakeGateway) RespondContactRequest(ctx context.Context, request *contactsv1.RespondContactRequestRequest) (*contactsv1.RespondContactRequestResponse, error) {
	if gateway.respondContactRequest == nil {
		return nil, status.Error(codes.Unimplemented, "respond contact request not implemented")
	}
	return gateway.respondContactRequest(ctx, request)
}

func (gateway *fakeGateway) CancelContactRequest(ctx context.Context, request *contactsv1.CancelContactRequestRequest) (*contactsv1.CancelContactRequestResponse, error) {
	if gateway.cancelContactRequest == nil {
		return nil, status.Error(codes.Unimplemented, "cancel contact request not implemented")
	}
	return gateway.cancelContactRequest(ctx, request)
}

func (gateway *fakeGateway) ListContactRequests(ctx context.Context, request *contactsv1.ListContactRequestsRequest) (*contactsv1.ListContactRequestsResponse, error) {
	if gateway.listContactRequests == nil {
		return nil, status.Error(codes.Unimplemented, "list contact requests not implemented")
	}
	return gateway.listContactRequests(ctx, request)
}

func (gateway *fakeGateway) ListContacts(ctx context.Context, request *contactsv1.ListContactsRequest) (*contactsv1.ListContactsResponse, error) {
	if gateway.listContacts == nil {
		return nil, status.Error(codes.Unimplemented, "list contacts not implemented")
	}
	return gateway.listContacts(ctx, request)
}

func (gateway *fakeGateway) GetContactState(ctx context.Context, request *contactsv1.GetContactStateRequest) (*contactsv1.GetContactStateResponse, error) {
	if gateway.getContactState == nil {
		return nil, status.Error(codes.Unimplemented, "get contact state not implemented")
	}
	return gateway.getContactState(ctx, request)
}

func (gateway *fakeGateway) DeleteContact(ctx context.Context, request *contactsv1.DeleteContactRequest) (*contactsv1.DeleteContactResponse, error) {
	if gateway.deleteContact == nil {
		return nil, status.Error(codes.Unimplemented, "delete contact not implemented")
	}
	return gateway.deleteContact(ctx, request)
}

func (gateway *fakeGateway) BlockContact(ctx context.Context, request *contactsv1.BlockContactRequest) (*contactsv1.BlockContactResponse, error) {
	if gateway.blockContact == nil {
		return nil, status.Error(codes.Unimplemented, "block contact not implemented")
	}
	return gateway.blockContact(ctx, request)
}

func (gateway *fakeGateway) UnblockContact(ctx context.Context, request *contactsv1.UnblockContactRequest) (*contactsv1.UnblockContactResponse, error) {
	if gateway.unblockContact == nil {
		return nil, status.Error(codes.Unimplemented, "unblock contact not implemented")
	}
	return gateway.unblockContact(ctx, request)
}

func (gateway *fakeGateway) UpdateContactRemark(ctx context.Context, request *contactsv1.UpdateContactRemarkRequest) (*contactsv1.UpdateContactRemarkResponse, error) {
	if gateway.updateContactRemark == nil {
		return nil, status.Error(codes.Unimplemented, "update contact remark not implemented")
	}
	return gateway.updateContactRemark(ctx, request)
}

func (gateway *fakeGateway) UpdateContactGroup(ctx context.Context, request *contactsv1.UpdateContactGroupRequest) (*contactsv1.UpdateContactGroupResponse, error) {
	if gateway.updateContactGroup == nil {
		return nil, status.Error(codes.Unimplemented, "update contact group not implemented")
	}
	return gateway.updateContactGroup(ctx, request)
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

type fakeMetricsRecorder struct {
	records []fakeMetricsRecord
}

type fakeMetricsRecord struct {
	route      string
	method     string
	statusCode int
}

func (recorder *fakeMetricsRecorder) RecordHTTPBFF(route string, method string, statusCode int, _ time.Duration) {
	recorder.records = append(recorder.records, fakeMetricsRecord{route: route, method: method, statusCode: statusCode})
}

type fakeRateLimiter struct {
	err     error
	methods []string
}

func (limiter *fakeRateLimiter) Check(_ context.Context, method string) error {
	limiter.methods = append(limiter.methods, method)
	return limiter.err
}
