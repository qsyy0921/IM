package domain

import (
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

const (
	DefaultGatewayAudience = "push-gateway"
	DefaultGatewayTTL      = 15 * time.Minute
	MaxGatewayTTL          = 24 * time.Hour
)

func ValidateIssueGatewayToken(command types.IssueGatewayTokenCommand) error {
	if command.TenantID == "" {
		return types.NewInvalidArgument("tenant_id is required")
	}
	if command.UserID == "" {
		return types.NewInvalidArgument("user_id is required")
	}
	if command.DeviceID == "" {
		return types.NewInvalidArgument("device_id is required")
	}
	return nil
}

func NormalizeAudience(audience string) string {
	audience = strings.TrimSpace(audience)
	if audience == "" {
		return DefaultGatewayAudience
	}
	return audience
}

func NormalizeTTL(seconds int64) time.Duration {
	if seconds <= 0 {
		return DefaultGatewayTTL
	}
	ttl := time.Duration(seconds) * time.Second
	if ttl > MaxGatewayTTL {
		return MaxGatewayTTL
	}
	return ttl
}

func ValidateRevokeTarget(userID types.UserID, deviceID types.DeviceID) error {
	if userID == "" {
		return types.NewInvalidArgument("user_id is required")
	}
	if deviceID == "" {
		return types.NewInvalidArgument("device_id is required")
	}
	return nil
}

func ValidateSessionID(sessionID types.SessionID) error {
	if sessionID == "" {
		return types.NewInvalidArgument("session_id is required")
	}
	return nil
}
