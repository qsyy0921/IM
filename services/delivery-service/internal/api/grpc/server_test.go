package grpc

import (
	"context"
	"testing"

	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type fakePullInboxExecutor struct {
	result  types.PullInboxResult
	err     error
	command types.PullInboxCommand
}

func (executor *fakePullInboxExecutor) Execute(
	_ context.Context,
	command types.PullInboxCommand,
) (types.PullInboxResult, error) {
	executor.command = command
	return executor.result, executor.err
}

type fakeAckDeliveryExecutor struct {
	result  types.AckDeliveryResult
	err     error
	command types.AckDeliveryCommand
}

func (executor *fakeAckDeliveryExecutor) Execute(
	_ context.Context,
	command types.AckDeliveryCommand,
) (types.AckDeliveryResult, error) {
	executor.command = command
	return executor.result, executor.err
}

func TestAckDeliveryMapsOutOfVisibleRange(t *testing.T) {
	server := NewServer(
		&fakePullInboxExecutor{},
		&fakeAckDeliveryExecutor{err: types.NewAckOutOfVisibleRange("too high")},
	)
	_, err := server.AckDelivery(context.Background(), &deliveryv1.AckDeliveryRequest{
		AuthContext: &deliveryv1.AuthContext{
			TenantId: "tenant-1",
			UserId:   "user-1",
			DeviceId: "device-1",
		},
		ConversationId: "conv-1",
		ReceivedSeq:    100,
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected failed precondition, got %v", err)
	}
}

func TestDeliveryAuthMetadataOverridesBodyForCommands(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		metadataTenantID, "trusted-tenant",
		metadataUserID, "trusted-user",
		metadataDeviceID, "trusted-device",
		metadataSessionID, "trusted-session",
	))
	interceptor := VerifiedAuthUnaryInterceptor(true)
	_, err := interceptor(ctx, nil, &grpcgo.UnaryServerInfo{}, func(ctx context.Context, request any) (any, error) {
		pullExecutor := &fakePullInboxExecutor{result: types.PullInboxResult{}}
		ackExecutor := &fakeAckDeliveryExecutor{result: types.AckDeliveryResult{
			TenantID:        "trusted-tenant",
			UserID:          "trusted-user",
			DeviceID:        "trusted-device",
			ConversationID:  "conv-1",
			LastReceivedSeq: 8,
		}}
		server := NewServer(pullExecutor, ackExecutor)

		if _, err := server.PullInbox(ctx, &deliveryv1.PullInboxRequest{
			AuthContext:    testSpoofedAuthContext(),
			ConversationId: "conv-1",
			AfterSeq:       7,
			Limit:          10,
		}); err != nil {
			t.Fatalf("pull inbox: %v", err)
		}
		if _, err := server.AckDelivery(ctx, &deliveryv1.AckDeliveryRequest{
			AuthContext:    testSpoofedAuthContext(),
			ConversationId: "conv-1",
			ReceivedSeq:    8,
		}); err != nil {
			t.Fatalf("ack delivery: %v", err)
		}

		assertTrustedMetadataAuth(t, pullExecutor.command.AuthContext)
		assertTrustedMetadataAuth(t, ackExecutor.command.AuthContext)
		return nil, nil
	})
	if err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
}

func TestDeliveryAuthMetadataDoesNotRequireBodyAuthContext(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		metadataTenantID, "trusted-tenant",
		metadataUserID, "trusted-user",
		metadataDeviceID, "trusted-device",
		metadataTraceID, "trusted-trace",
		metadataRequestID, "trusted-request",
	))
	interceptor := VerifiedAuthUnaryInterceptor(true)
	_, err := interceptor(ctx, nil, &grpcgo.UnaryServerInfo{}, func(ctx context.Context, request any) (any, error) {
		pullExecutor := &fakePullInboxExecutor{result: types.PullInboxResult{}}
		server := NewServer(pullExecutor, &fakeAckDeliveryExecutor{})
		if _, err := server.PullInbox(ctx, &deliveryv1.PullInboxRequest{
			ConversationId: "conv-1",
			AfterSeq:       1,
			Limit:          10,
		}); err != nil {
			t.Fatalf("pull inbox: %v", err)
		}
		auth := pullExecutor.command.AuthContext
		if auth.TenantID != "trusted-tenant" ||
			auth.UserID != "trusted-user" ||
			auth.DeviceID != "trusted-device" ||
			auth.TraceID != "trusted-trace" ||
			auth.RequestID != "trusted-request" {
			t.Fatalf("unexpected verified auth without body auth: %+v", auth)
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
}

func TestVerifiedAuthUnaryInterceptorRequiresTrustedIdentity(t *testing.T) {
	interceptor := VerifiedAuthUnaryInterceptor(true)
	_, err := interceptor(context.Background(), nil, &grpcgo.UnaryServerInfo{}, func(ctx context.Context, request any) (any, error) {
		t.Fatal("handler should not be called without verified auth")
		return nil, nil
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected unauthenticated, got %v (%v)", status.Code(err), err)
	}
}

func testSpoofedAuthContext() *deliveryv1.AuthContext {
	return &deliveryv1.AuthContext{
		TenantId:  "spoofed-tenant",
		UserId:    "spoofed-user",
		DeviceId:  "spoofed-device",
		SessionId: "spoofed-session",
		TraceId:   "body-trace",
		RequestId: "body-request",
	}
}

func assertTrustedMetadataAuth(t *testing.T, auth types.AuthContext) {
	t.Helper()
	if auth.TenantID != "trusted-tenant" ||
		auth.UserID != "trusted-user" ||
		auth.DeviceID != "trusted-device" ||
		auth.SessionID != "trusted-session" ||
		auth.TraceID != "body-trace" ||
		auth.RequestID != "body-request" {
		t.Fatalf("unexpected verified auth: %+v", auth)
	}
}
