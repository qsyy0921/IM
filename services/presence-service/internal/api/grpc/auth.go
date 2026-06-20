package grpc

import (
	"context"
	"strings"

	"github.com/qsyy0921/IM/services/presence-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	metadataTenantID    = "x-nexusim-tenant-id"
	metadataUserID      = "x-nexusim-user-id"
	metadataServiceName = "x-nexusim-service-name"
	metadataInstanceRef = "x-nexusim-instance-ref"
	metadataTraceID     = "x-nexusim-trace-id"
	metadataRequestID   = "x-nexusim-request-id"
)

type verifiedAuthContextKey struct{}

func VerifiedAuthUnaryInterceptor(required bool) grpcgo.UnaryServerInterceptor {
	return func(ctx context.Context, request any, _ *grpcgo.UnaryServerInfo, handler grpcgo.UnaryHandler) (any, error) {
		auth := authFromMetadata(ctx)
		if required && auth.TenantID == "" {
			return nil, status.Error(codes.Unauthenticated, "verified auth metadata is required")
		}
		if auth.TenantID != "" {
			ctx = context.WithValue(ctx, verifiedAuthContextKey{}, auth)
		}
		return handler(ctx, request)
	}
}

func ContextWithVerifiedAuth(ctx context.Context, auth types.AuthContext) context.Context {
	return context.WithValue(ctx, verifiedAuthContextKey{}, auth)
}

func verifiedAuthFromContext(ctx context.Context) (types.AuthContext, bool) {
	auth, ok := ctx.Value(verifiedAuthContextKey{}).(types.AuthContext)
	return auth, ok
}

func authFromMetadata(ctx context.Context) types.AuthContext {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return types.AuthContext{}
	}
	return types.AuthContext{
		TenantID:    types.TenantID(firstMetadataValue(md, metadataTenantID)),
		UserID:      firstMetadataValue(md, metadataUserID),
		ServiceName: firstMetadataValue(md, metadataServiceName),
		InstanceRef: firstMetadataValue(md, metadataInstanceRef),
		TraceID:     firstMetadataValue(md, metadataTraceID),
		RequestID:   firstMetadataValue(md, metadataRequestID),
	}
}

func firstMetadataValue(md metadata.MD, key string) string {
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}
