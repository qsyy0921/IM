package grpc

import (
	"context"
	"strings"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	metadataTenantID  = "x-nexusim-tenant-id"
	metadataUserID    = "x-nexusim-user-id"
	metadataTraceID   = "x-nexusim-trace-id"
	metadataRequestID = "x-nexusim-request-id"
)

type verifiedAdminContextKey struct{}

func VerifiedAdminUnaryInterceptor(required bool) grpcgo.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpcgo.UnaryServerInfo, handler grpcgo.UnaryHandler) (any, error) {
		admin := adminFromMetadata(ctx)
		if required && isAdminMethod(info.FullMethod) && (admin.TenantID == "" || admin.OperatorUserID == "") {
			return nil, status.Error(codes.Unauthenticated, "verified admin metadata is required")
		}
		if admin.TenantID != "" || admin.OperatorUserID != "" {
			ctx = context.WithValue(ctx, verifiedAdminContextKey{}, admin)
		}
		return handler(ctx, request)
	}
}

func adminFromMetadata(ctx context.Context) types.AdminContext {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return types.AdminContext{}
	}
	return types.AdminContext{
		TenantID:       types.TenantID(firstMetadataValue(md, metadataTenantID)),
		OperatorUserID: types.UserID(firstMetadataValue(md, metadataUserID)),
		TraceID:        firstMetadataValue(md, metadataTraceID),
		RequestID:      firstMetadataValue(md, metadataRequestID),
	}
}

func firstMetadataValue(md metadata.MD, key string) string {
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func verifiedAdminFromContext(ctx context.Context) (types.AdminContext, bool) {
	admin, ok := ctx.Value(verifiedAdminContextKey{}).(types.AdminContext)
	return admin, ok
}

func isAdminMethod(method string) bool {
	switch method {
	case "/nexusim.identity.v1.IdentityService/RevokeDevice",
		"/nexusim.identity.v1.IdentityService/RevokeSession",
		"/nexusim.identity.v1.IdentityService/GetDeviceState":
		return true
	default:
		return false
	}
}
