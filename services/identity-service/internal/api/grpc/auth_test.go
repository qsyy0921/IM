package grpc

import (
	"context"
	"testing"

	identityv1 "github.com/qsyy0921/IM/api/proto/nexusim/identity/v1"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestAdminFromProtoPrefersVerifiedMetadata(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		metadataTenantID, "trusted-tenant",
		metadataUserID, "trusted-admin",
	))
	interceptor := VerifiedAdminUnaryInterceptor(true)
	_, err := interceptor(ctx, nil, &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.identity.v1.IdentityService/RevokeDevice"}, func(ctx context.Context, req any) (any, error) {
		admin := adminFromProto(ctx, &identityv1.AdminContext{
			TenantId:       "spoofed-tenant",
			OperatorUserId: "spoofed-admin",
			TraceId:        "body-trace",
			RequestId:      "body-request",
		})
		if admin.TenantID != types.TenantID("trusted-tenant") || admin.OperatorUserID != types.UserID("trusted-admin") {
			t.Fatalf("verified metadata should override body admin context: %+v", admin)
		}
		if admin.TraceID != "body-trace" || admin.RequestID != "body-request" {
			t.Fatalf("expected trace/request fallback from body, got %+v", admin)
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
}

func TestVerifiedAdminUnaryInterceptorRequiresAdminMetadataForAdminMethods(t *testing.T) {
	interceptor := VerifiedAdminUnaryInterceptor(true)
	_, err := interceptor(context.Background(), nil, &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.identity.v1.IdentityService/RevokeSession"}, func(ctx context.Context, req any) (any, error) {
		t.Fatal("handler should not be called without verified admin metadata")
		return nil, nil
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected unauthenticated, got %v (%v)", status.Code(err), err)
	}
}

func TestVerifiedAdminUnaryInterceptorDoesNotRequireMetadataForIssueToken(t *testing.T) {
	interceptor := VerifiedAdminUnaryInterceptor(true)
	called := false
	_, err := interceptor(context.Background(), nil, &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.identity.v1.IdentityService/IssueGatewayToken"}, func(ctx context.Context, req any) (any, error) {
		called = true
		return nil, nil
	})
	if err != nil {
		t.Fatalf("issue token should not require admin metadata: %v", err)
	}
	if !called {
		t.Fatal("handler was not called")
	}
}
