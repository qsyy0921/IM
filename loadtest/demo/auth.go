package main

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc/metadata"
)

const (
	opClientHello    = "client.hello"
	opDeliveryAck    = "delivery.ack"
	opServerHello    = "server.hello"
	opDeliveryNotify = "delivery.notify"
	opDeliveryAckOK  = "delivery.ack.ok"
	opResumeHint     = "server.resume_hint"

	metadataTenantID  = "x-nexusim-tenant-id"
	metadataUserID    = "x-nexusim-user-id"
	metadataDeviceID  = "x-nexusim-device-id"
	metadataSessionID = "x-nexusim-session-id"
	metadataTraceID   = "x-nexusim-trace-id"
	metadataRequestID = "x-nexusim-request-id"
	metadataToken     = "x-nexusim-gateway-token"
)

type demoAuth struct {
	tenantID  string
	userID    string
	deviceID  string
	sessionID string
	traceID   string
	requestID string
}

func withVerifiedAuthMetadata(ctx context.Context, cfg config, auth demoAuth) context.Context {
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

func withUserFacingAuthMetadata(ctx context.Context, cfg config, auth demoAuth) (context.Context, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.gatewayAuthMode)) {
	case "":
		return withVerifiedAuthMetadata(ctx, cfg, auth), nil
	case "mock":
		pairs := []string{
			metadataToken, auth.tenantID + ":" + auth.userID + ":" + auth.deviceID,
		}
		if auth.traceID != "" {
			pairs = append(pairs, metadataTraceID, auth.traceID)
		}
		if auth.requestID != "" {
			pairs = append(pairs, metadataRequestID, auth.requestID)
		}
		return metadata.NewOutgoingContext(ctx, metadata.Pairs(pairs...)), nil
	case "hmac":
		token, err := signGatewayAuthToken(cfg.gatewayAuthHMACSecret, cfg.gatewayAuthTokenTTL, auth, normalizedGatewayAuthAudience(cfg.gatewayAuthAudience))
		if err != nil {
			return nil, err
		}
		pairs := []string{"authorization", "Bearer " + token}
		if auth.requestID != "" {
			pairs = append(pairs, metadataRequestID, auth.requestID)
		}
		return metadata.NewOutgoingContext(ctx, metadata.Pairs(pairs...)), nil
	default:
		return nil, fmt.Errorf("unsupported gateway auth mode: %s", cfg.gatewayAuthMode)
	}
}
