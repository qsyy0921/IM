package main

import (
	"context"
	"strings"
	"time"

	contactsv1 "github.com/qsyy0921/IM/api/proto/nexusim/contacts/v1"
	"github.com/qsyy0921/IM/internal/gatewayauth"
	"google.golang.org/grpc/metadata"
)

const (
	metadataTenantID  = "x-nexusim-tenant-id"
	metadataUserID    = "x-nexusim-user-id"
	metadataDeviceID  = "x-nexusim-device-id"
	metadataSessionID = "x-nexusim-session-id"
	metadataTraceID   = "x-nexusim-trace-id"
	metadataRequestID = "x-nexusim-request-id"
	metadataToken     = "x-nexusim-gateway-token"
)

func requestContext(cfg config, userID string, deviceID string, requestID string, traceID string) (context.Context, context.CancelFunc, *contactsv1.AuthContext) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.requestTimeout)
	auth := &contactsv1.AuthContext{
		TenantId:  cfg.tenantID,
		UserId:    userID,
		DeviceId:  deviceID,
		SessionId: sessionIDForDevice(deviceID),
		RequestId: requestID,
		TraceId:   traceID,
	}
	if cfg.verifiedMetadata {
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(
			metadataTenantID, auth.GetTenantId(),
			metadataUserID, auth.GetUserId(),
			metadataDeviceID, auth.GetDeviceId(),
			metadataSessionID, auth.GetSessionId(),
			metadataTraceID, auth.GetTraceId(),
			metadataRequestID, auth.GetRequestId(),
		))
	}
	if cfg.gatewayAuthMode != "" {
		ctx = withGatewayAuthMetadata(ctx, cfg, auth)
	}
	return ctx, cancel, auth
}

func withGatewayAuthMetadata(ctx context.Context, cfg config, auth *contactsv1.AuthContext) context.Context {
	switch cfg.gatewayAuthMode {
	case "mock":
		pairs := []string{
			metadataToken, auth.GetTenantId() + ":" + auth.GetUserId() + ":" + auth.GetDeviceId(),
		}
		if auth.GetTraceId() != "" {
			pairs = append(pairs, metadataTraceID, auth.GetTraceId())
		}
		if auth.GetRequestId() != "" {
			pairs = append(pairs, metadataRequestID, auth.GetRequestId())
		}
		return metadata.NewOutgoingContext(ctx, metadata.Pairs(pairs...))
	case "hmac":
		token, err := signGatewayAuthToken(cfg.gatewayAuthHMACSecret, cfg.gatewayAuthTokenTTL, auth, cfg.gatewayAuthAudience)
		if err != nil {
			return metadata.NewOutgoingContext(ctx, metadata.Pairs("x-nexusim-loadtest-auth-error", err.Error()))
		}
		pairs := []string{"authorization", "Bearer " + token}
		if auth.GetRequestId() != "" {
			pairs = append(pairs, metadataRequestID, auth.GetRequestId())
		}
		return metadata.NewOutgoingContext(ctx, metadata.Pairs(pairs...))
	default:
		return ctx
	}
}

func signGatewayAuthToken(secret string, ttl time.Duration, auth *contactsv1.AuthContext, audience string) (string, error) {
	return gatewayauth.SignGatewayToken(secret, map[string]string{
		"tenant_id":  auth.GetTenantId(),
		"user_id":    auth.GetUserId(),
		"device_id":  auth.GetDeviceId(),
		"session_id": auth.GetSessionId(),
		"trace_id":   auth.GetTraceId(),
		"aud":        strings.TrimSpace(audience),
	}, time.Now().Add(ttl))
}

func gatewayAuthAudienceSummary(mode string, audience string) string {
	if strings.TrimSpace(mode) == "" {
		return ""
	}
	return normalizedGatewayAuthAudience(audience)
}

func normalizedGatewayAuthAudience(audience string) string {
	audience = strings.TrimSpace(audience)
	if audience == "" {
		return "api-gateway"
	}
	return audience
}

func deviceIDForUser(cfg config, userID string) string {
	if userID == cfg.receiverUserID {
		return cfg.receiverDeviceID
	}
	return cfg.senderDeviceID
}

func sessionIDForDevice(deviceID string) string {
	if strings.TrimSpace(deviceID) == "" {
		return ""
	}
	return "contacts-smoke-" + deviceID
}
