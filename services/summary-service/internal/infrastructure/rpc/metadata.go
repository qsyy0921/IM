package rpc

import (
	"context"

	"github.com/qsyy0921/IM/services/summary-service/internal/types"
	"google.golang.org/grpc/metadata"
)

func outgoingMetadataContext(ctx context.Context, auth types.AuthContext) context.Context {
	md := metadata.Pairs(
		"x-nexusim-tenant-id", string(auth.TenantID),
		"x-nexusim-user-id", string(auth.UserID),
		"x-nexusim-device-id", auth.DeviceID,
	)
	if auth.SessionID != "" {
		md.Append("x-nexusim-session-id", auth.SessionID)
	}
	if auth.TraceID != "" {
		md.Append("x-nexusim-trace-id", auth.TraceID)
	}
	if auth.RequestID != "" {
		md.Append("x-nexusim-request-id", auth.RequestID)
	}
	return metadata.NewOutgoingContext(ctx, md)
}
