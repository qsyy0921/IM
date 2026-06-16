package main

import (
	"context"

	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	receiptv1 "github.com/qsyy0921/IM/api/proto/nexusim/receipt/v1"
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

func ownerAuth(cfg config, traceID string, requestID string) verifiedAuthIdentity {
	return verifiedAuthIdentity{
		tenantID:  cfg.tenantID,
		userID:    cfg.ownerUserID,
		deviceID:  "receipt-smoke-owner-device",
		sessionID: "receipt-smoke-owner-session",
		traceID:   traceID,
		requestID: requestID,
	}
}

func receiverAuth(cfg config, traceID string, requestID string) verifiedAuthIdentity {
	return verifiedAuthIdentity{
		tenantID:  cfg.tenantID,
		userID:    cfg.receiverUserID,
		deviceID:  cfg.receiverDeviceID,
		sessionID: "receipt-smoke",
		traceID:   traceID,
		requestID: requestID,
	}
}

func withVerifiedAuthMetadata(ctx context.Context, cfg config, auth verifiedAuthIdentity) context.Context {
	if !cfg.verifiedAuthMetadata {
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

func conversationAuth(auth verifiedAuthIdentity) *conversationv1.AuthContext {
	return &conversationv1.AuthContext{
		TenantId:  auth.tenantID,
		UserId:    auth.userID,
		DeviceId:  auth.deviceID,
		SessionId: auth.sessionID,
		TraceId:   auth.traceID,
		RequestId: auth.requestID,
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

func deliveryAuth(auth verifiedAuthIdentity) *deliveryv1.AuthContext {
	return &deliveryv1.AuthContext{
		TenantId:  auth.tenantID,
		UserId:    auth.userID,
		DeviceId:  auth.deviceID,
		SessionId: auth.sessionID,
		TraceId:   auth.traceID,
		RequestId: auth.requestID,
	}
}

func receiptAuth(auth verifiedAuthIdentity) *receiptv1.AuthContext {
	return &receiptv1.AuthContext{
		TenantId:  auth.tenantID,
		UserId:    auth.userID,
		DeviceId:  auth.deviceID,
		SessionId: auth.sessionID,
		TraceId:   auth.traceID,
		RequestId: auth.requestID,
	}
}
