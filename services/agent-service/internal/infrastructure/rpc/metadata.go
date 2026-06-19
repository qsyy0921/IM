package rpc

import (
	"context"

	"github.com/qsyy0921/IM/services/agent-service/internal/types"
	"google.golang.org/grpc/metadata"
)

func outgoingMetadataContext(ctx context.Context, auth types.AuthContext) context.Context {
	pairs := []string{
		"x-nexusim-tenant-id", string(auth.TenantID),
		"x-nexusim-user-id", string(auth.UserID),
		"x-nexusim-device-id", auth.DeviceID,
	}
	if auth.SessionID != "" {
		pairs = append(pairs, "x-nexusim-session-id", auth.SessionID)
	}
	if auth.TraceID != "" {
		pairs = append(pairs, "x-nexusim-trace-id", auth.TraceID)
	}
	if auth.RequestID != "" {
		pairs = append(pairs, "x-nexusim-request-id", auth.RequestID)
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...)
}
