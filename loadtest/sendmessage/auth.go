package main

import (
	"context"

	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
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

func withVerifiedAuthMetadata(ctx context.Context, cfg config, auth *messagev1.AuthContext) context.Context {
	if !cfg.VerifiedAuthMetadata || auth == nil {
		return ctx
	}
	pairs := []string{
		metadataTenantID, auth.GetTenantId(),
		metadataUserID, auth.GetUserId(),
		metadataDeviceID, auth.GetDeviceId(),
	}
	if auth.GetSessionId() != "" {
		pairs = append(pairs, metadataSessionID, auth.GetSessionId())
	}
	if auth.GetTraceId() != "" {
		pairs = append(pairs, metadataTraceID, auth.GetTraceId())
	}
	if auth.GetRequestId() != "" {
		pairs = append(pairs, metadataRequestID, auth.GetRequestId())
	}
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(pairs...))
}
