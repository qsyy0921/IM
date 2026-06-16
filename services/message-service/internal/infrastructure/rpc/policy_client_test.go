package rpc

import (
	"context"
	"errors"
	"testing"

	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestPolicyClientCheckSendPermission(t *testing.T) {
	fake := &fakePolicyServiceClient{
		response: &policyv1.CheckMessageActionResponse{
			TenantId:          "tenant-1",
			UserId:            "user-1",
			ConversationId:    "conv-1",
			Action:            policyv1.MessageAction_MESSAGE_ACTION_SEND,
			Allowed:           true,
			PermissionVersion: 7,
			Classification:    "CONTACT",
		},
	}
	client := NewPolicyClient(fake, 0)
	decision, err := client.CheckSendPermission(context.Background(), testPolicyClientSendCommand(), testPolicyClientConversation())
	if err != nil {
		t.Fatalf("check send permission: %v", err)
	}
	if !decision.Allowed || decision.PermissionVersion != 7 || decision.Classification != "CONTACT" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	if fake.request.GetAction() != policyv1.MessageAction_MESSAGE_ACTION_SEND ||
		fake.request.GetConversationId() != "conv-1" ||
		fake.request.GetAuthContext().GetDeviceId() != "device-1" ||
		fake.request.GetDirectPeerUserId() != "user-2" ||
		fake.request.GetConversationPermissionVersion() != 7 ||
		fake.request.GetMessageText() != "hello policy" {
		t.Fatalf("unexpected request: %+v", fake.request)
	}
	if fake.outgoingMetadata.Get(policyMetadataTraceID)[0] != "trace-1" ||
		fake.outgoingMetadata.Get(policyMetadataRequestID)[0] != "request-1" {
		t.Fatalf("expected trace/request metadata, got %v", fake.outgoingMetadata)
	}
}

func TestPolicyClientCheckEditPermissionIncludesMessageID(t *testing.T) {
	fake := &fakePolicyServiceClient{
		response: &policyv1.CheckMessageActionResponse{
			TenantId:          "tenant-1",
			UserId:            "user-1",
			ConversationId:    "conv-1",
			MessageId:         "msg-1",
			Action:            policyv1.MessageAction_MESSAGE_ACTION_EDIT,
			Allowed:           true,
			PermissionVersion: 7,
			Classification:    "CONTACT",
			OwnershipOverride: true,
		},
	}
	client := NewPolicyClient(fake, 0)
	decision, err := client.CheckEditPermission(context.Background(), types.EditMessageCommand{
		AuthContext:    testPolicyClientAuth(),
		ConversationID: "conv-1",
		MessageID:      "msg-1",
		PayloadJSON:    []byte(`{"text":"edited policy"}`),
	}, testPolicyClientConversation(), types.MessagePolicyContext{SenderUserID: "sender-1"})
	if err != nil {
		t.Fatalf("check edit permission: %v", err)
	}
	if !decision.OwnershipOverride {
		t.Fatalf("expected ownership override to be propagated: %+v", decision)
	}
	if fake.request.GetMessageId() != "msg-1" ||
		fake.request.GetAction() != policyv1.MessageAction_MESSAGE_ACTION_EDIT ||
		fake.request.GetMessageSenderUserId() != "sender-1" ||
		fake.request.GetMessageText() != "edited policy" {
		t.Fatalf("unexpected request: %+v", fake.request)
	}
}

func TestPolicyClientDoesNotSendTextForRevoke(t *testing.T) {
	fake := &fakePolicyServiceClient{
		response: &policyv1.CheckMessageActionResponse{
			TenantId:          "tenant-1",
			UserId:            "user-1",
			ConversationId:    "conv-1",
			MessageId:         "msg-1",
			Action:            policyv1.MessageAction_MESSAGE_ACTION_REVOKE,
			Allowed:           true,
			PermissionVersion: 7,
			Classification:    "CONTACT",
		},
	}
	client := NewPolicyClient(fake, 0)
	_, err := client.CheckRevokePermission(context.Background(), types.RevokeMessageCommand{
		AuthContext:    testPolicyClientAuth(),
		ConversationID: "conv-1",
		MessageID:      "msg-1",
	}, testPolicyClientConversation(), types.MessagePolicyContext{SenderUserID: "sender-1"})
	if err != nil {
		t.Fatalf("check revoke permission: %v", err)
	}
	if fake.request.GetMessageText() != "" {
		t.Fatalf("expected revoke not to send message text, got %+v", fake.request)
	}
}

func TestPolicyClientRejectsMismatchedResponse(t *testing.T) {
	client := NewPolicyClient(&fakePolicyServiceClient{
		response: &policyv1.CheckMessageActionResponse{
			TenantId:          "other-tenant",
			UserId:            "user-1",
			ConversationId:    "conv-1",
			Action:            policyv1.MessageAction_MESSAGE_ACTION_SEND,
			Allowed:           true,
			PermissionVersion: 7,
			Classification:    "CONTACT",
		},
	}, 0)
	_, err := client.CheckSendPermission(context.Background(), testPolicyClientSendCommand(), testPolicyClientConversation())
	if !errors.Is(err, types.ErrDependencyUnavailable) {
		t.Fatalf("expected dependency unavailable, got %v", err)
	}
}

func TestPolicyClientRejectsInvalidDecisionFields(t *testing.T) {
	client := NewPolicyClient(&fakePolicyServiceClient{
		response: &policyv1.CheckMessageActionResponse{
			TenantId:       "tenant-1",
			UserId:         "user-1",
			ConversationId: "conv-1",
			Action:         policyv1.MessageAction_MESSAGE_ACTION_SEND,
			Allowed:        true,
		},
	}, 0)
	_, err := client.CheckSendPermission(context.Background(), testPolicyClientSendCommand(), testPolicyClientConversation())
	if !errors.Is(err, types.ErrDependencyUnavailable) {
		t.Fatalf("expected dependency unavailable, got %v", err)
	}
}

func TestPolicyClientRejectsInvalidOwnershipOverride(t *testing.T) {
	client := NewPolicyClient(&fakePolicyServiceClient{
		response: &policyv1.CheckMessageActionResponse{
			TenantId:          "tenant-1",
			UserId:            "user-1",
			ConversationId:    "conv-1",
			Action:            policyv1.MessageAction_MESSAGE_ACTION_SEND,
			Allowed:           true,
			PermissionVersion: 7,
			Classification:    "CONTACT",
			OwnershipOverride: true,
		},
	}, 0)
	_, err := client.CheckSendPermission(context.Background(), testPolicyClientSendCommand(), testPolicyClientConversation())
	if !errors.Is(err, types.ErrDependencyUnavailable) {
		t.Fatalf("expected dependency unavailable, got %v", err)
	}
}

func TestPolicyClientMapsUnavailable(t *testing.T) {
	client := NewPolicyClient(&fakePolicyServiceClient{
		err: status.Error(codes.Unavailable, "down"),
	}, 0)
	_, err := client.CheckSendPermission(context.Background(), testPolicyClientSendCommand(), testPolicyClientConversation())
	if !errors.Is(err, types.ErrDependencyUnavailable) {
		t.Fatalf("expected dependency unavailable, got %v", err)
	}
}

func TestPolicyClientMapsTransportPermissionDeniedAsDependencyUnavailable(t *testing.T) {
	client := NewPolicyClient(&fakePolicyServiceClient{
		err: status.Error(codes.PermissionDenied, "mtls peer is not allowed"),
	}, 0)
	_, err := client.CheckSendPermission(context.Background(), testPolicyClientSendCommand(), testPolicyClientConversation())
	if !errors.Is(err, types.ErrDependencyUnavailable) {
		t.Fatalf("expected dependency unavailable, got %v", err)
	}
}

func TestMessageTextFromPayloadJSON(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  []byte
		want string
	}{
		{name: "text", raw: []byte(`{"text":" hello "}`), want: "hello"},
		{name: "non string text", raw: []byte(`{"text":123}`), want: ""},
		{name: "malformed", raw: []byte(`{"text"`), want: ""},
		{name: "empty", raw: nil, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := messageTextFromPayloadJSON(tc.raw); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

type fakePolicyServiceClient struct {
	request          *policyv1.CheckMessageActionRequest
	outgoingMetadata metadata.MD
	response         *policyv1.CheckMessageActionResponse
	err              error
}

func (f *fakePolicyServiceClient) CheckMessageAction(
	ctx context.Context,
	request *policyv1.CheckMessageActionRequest,
	_ ...grpc.CallOption,
) (*policyv1.CheckMessageActionResponse, error) {
	f.request = request
	f.outgoingMetadata, _ = metadata.FromOutgoingContext(ctx)
	if f.err != nil {
		return nil, f.err
	}
	return f.response, nil
}

func testPolicyClientSendCommand() types.SendMessageCommand {
	return types.SendMessageCommand{
		AuthContext:    testPolicyClientAuth(),
		ConversationID: "conv-1",
		PayloadJSON:    []byte(`{"text":" hello policy "}`),
	}
}

func testPolicyClientConversation() types.ConversationSendContext {
	return types.ConversationSendContext{DirectPeerUserID: "user-2", PermissionVersion: 7}
}

func testPolicyClientAuth() types.AuthContext {
	return types.AuthContext{
		TenantID:  "tenant-1",
		UserID:    "user-1",
		DeviceID:  "device-1",
		SessionID: "session-1",
		TraceID:   "trace-1",
		RequestID: "request-1",
	}
}
