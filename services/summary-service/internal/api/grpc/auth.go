package grpc

import (
	"context"
	"strings"

	"github.com/qsyy0921/IM/services/summary-service/internal/types"
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
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return types.AuthContext{}, false
	}
	auth := types.AuthContext{
		TenantID:  types.TenantID(firstMetadataValue(md, metadataTenantID)),
		UserID:    types.UserID(firstMetadataValue(md, metadataUserID)),
		DeviceID:  firstMetadataValue(md, metadataDeviceID),
		SessionID: firstMetadataValue(md, metadataSessionID),
		TraceID:   firstMetadataValue(md, metadataTraceID),
		RequestID: firstMetadataValue(md, metadataRequestID),
	}
	if strings.TrimSpace(string(auth.TenantID)) == "" ||
		strings.TrimSpace(string(auth.UserID)) == "" ||
		strings.TrimSpace(auth.DeviceID) == "" {
		return types.AuthContext{}, false
	}
	return auth, true
}

func firstMetadataValue(md metadata.MD, key string) string {
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}
