package grpc

import (
	"context"
	"strings"

	"github.com/qsyy0921/IM/services/agent-service/internal/types"
	"google.golang.org/grpc/metadata"
)

const (
	metadataTenantID  = "x-nexusim-tenant-id"
	metadataUserID    = "x-nexusim-user-id"
	metadataDeviceID  = "x-nexusim-device-id"
	metadataSessionID = "x-nexusim-session-id"
	metadataTraceID   = "x-nexusim-trace-id"
	metadataRequestID = "x-nexusim-request-id"
)

func verifiedAuthFromContext(ctx context.Context) (types.AuthContext, bool) {
	values, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return types.AuthContext{}, false
	}
	tenantID := firstMetadata(values, metadataTenantID)
	userID := firstMetadata(values, metadataUserID)
	deviceID := firstMetadata(values, metadataDeviceID)
	if tenantID == "" || userID == "" || deviceID == "" {
		return types.AuthContext{}, false
	}
	return types.AuthContext{
		TenantID:  types.TenantID(tenantID),
		UserID:    types.UserID(userID),
		DeviceID:  deviceID,
		SessionID: firstMetadata(values, metadataSessionID),
		TraceID:   firstMetadata(values, metadataTraceID),
		RequestID: firstMetadata(values, metadataRequestID),
	}, true
}

func firstMetadata(values metadata.MD, key string) string {
	entries := values.Get(key)
	if len(entries) == 0 {
		return ""
	}
	return strings.TrimSpace(entries[0])
}
