package rpc

import (
	"context"
	"strings"

	"github.com/qsyy0921/IM/services/retrieval-gateway/internal/types"
	"google.golang.org/grpc/metadata"
)

const (
	metadataTenantID  = "x-nexusim-tenant-id"
	metadataUserID    = "x-nexusim-user-id"
	metadataDeviceID  = "x-nexusim-device-id"
	metadataSessionID = "x-nexusim-session-id"
	metadataTraceID   = "x-nexusim-trace-id"
	metadataRequestID = "x-nexusim-request-id"

	maxCorrelationMetadataLength = 128
)

func outgoingMetadataContext(ctx context.Context, auth types.AuthContext) context.Context {
	auth = sanitizeCorrelation(auth)
	pairs := []string{
		metadataTenantID, string(auth.TenantID),
		metadataUserID, string(auth.UserID),
		metadataDeviceID, auth.DeviceID,
	}
	if auth.SessionID != "" {
		pairs = append(pairs, metadataSessionID, auth.SessionID)
	}
	if auth.TraceID != "" {
		pairs = append(pairs, metadataTraceID, auth.TraceID)
	}
	if auth.RequestID != "" {
		pairs = append(pairs, metadataRequestID, auth.RequestID)
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...)
}

func sanitizeCorrelation(auth types.AuthContext) types.AuthContext {
	auth.TraceID = sanitizeCorrelationValue(auth.TraceID)
	auth.RequestID = sanitizeCorrelationValue(auth.RequestID)
	return auth
}

func sanitizeCorrelationValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) > maxCorrelationMetadataLength {
		runes = runes[:maxCorrelationMetadataLength]
	}
	for _, r := range runes {
		if isCorrelationRune(r) {
			continue
		}
		return ""
	}
	return string(runes)
}

func isCorrelationRune(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '-' ||
		r == '_' ||
		r == '.' ||
		r == ':'
}
