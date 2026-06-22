package grpc

import (
	"context"
	"testing"

	contactsv1 "github.com/qsyy0921/IM/api/proto/nexusim/contacts/v1"
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestAuthFromProtoPrefersVerifiedMetadata(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		metadataTenantID, "trusted-tenant",
		metadataUserID, "trusted-user",
		metadataDeviceID, "trusted-device",
	))
	interceptor := VerifiedAuthUnaryInterceptor(true)
	_, err := interceptor(ctx, nil, &grpcgo.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		auth := authFromProto(ctx, &contactsv1.AuthContext{
			TenantId: "spoofed-tenant",
			UserId:   "spoofed-user",
			DeviceId: "spoofed-device",
			TraceId:  "body-trace",
		})
		if auth.TenantID != types.TenantID("trusted-tenant") || auth.UserID != types.UserID("trusted-user") {
			t.Fatalf("verified metadata should override body auth: %+v", auth)
		}
		if auth.DeviceID != "trusted-device" {
			t.Fatalf("expected trusted device id, got %q", auth.DeviceID)
		}
		if auth.TraceID != "body-trace" {
			t.Fatalf("expected trace propagation from body, got %q", auth.TraceID)
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
}

func TestVerifiedAuthUnaryInterceptorRequiresTenantAndUser(t *testing.T) {
	interceptor := VerifiedAuthUnaryInterceptor(true)
	_, err := interceptor(context.Background(), nil, &grpcgo.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		t.Fatal("handler should not be called without verified auth")
		return nil, nil
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected unauthenticated, got %v (%v)", status.Code(err), err)
	}
}
