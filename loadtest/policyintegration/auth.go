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

type verifiedAuthIdentity struct {
	tenantID  string
	userID    string
	deviceID  string
	sessionID string
	traceID   string
	requestID string
}

func withVerifiedAuthMetadata(ctx context.Context, cfg config, auth verifiedAuthIdentity) context.Context {
	if !cfg.verifiedMetadata {
		return ctx
	}
	pairs := []string{
		metadataTenantID, auth.tenantID,
		metadataUserID, auth.userID,
		metadataDeviceID, auth.deviceID,
	}
	if auth.sessionID != "" {
		pairs = append(pairs, metadataSessionID, auth.sessionID)
	}
	if auth.traceID != "" {
		pairs = append(pairs, metadataTraceID, auth.traceID)
	}
	if auth.requestID != "" {
		pairs = append(pairs, metadataRequestID, auth.requestID)
	}
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(pairs...))
}

func sendAuth(cfg config, requestID string) verifiedAuthIdentity {
	return verifiedAuthIdentity{
		tenantID:  cfg.tenantID,
		userID:    cfg.userID,
		deviceID:  cfg.deviceID,
		sessionID: cfg.sessionID,
		traceID:   "trace-policy-message-smoke",
		requestID: requestID,
	}
}

func changeAuth(cfg config, requestID string) verifiedAuthIdentity {
	return verifiedAuthIdentity{
		tenantID:  cfg.tenantID,
		userID:    cfg.changeUserID,
		deviceID:  cfg.changeDeviceID,
		sessionID: cfg.changeSessionID,
		traceID:   "trace-policy-message-smoke",
		requestID: requestID,
	}
}

func messageAuth(auth verifiedAuthIdentity) *messagev1.AuthContext {
	return &messagev1.AuthContext{
		TenantId:  auth.tenantID,
		UserId:    auth.userID,
		DeviceId:  auth.deviceID,
		SessionId: auth.sessionID,
		TraceId:   auth.traceID,
		RequestId: auth.requestID,
	}
}
