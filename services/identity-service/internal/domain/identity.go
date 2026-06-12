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
	DefaultRefreshTTL      = 30 * 24 * time.Hour
	MaxRefreshTTL          = 90 * 24 * time.Hour
	MinPasswordLength      = 8
)

func ValidateRegisterUser(command types.RegisterUserCommand) error {
	if command.TenantID == "" {
		return types.NewInvalidArgument("tenant_id is required")
	}
	if command.UserID == "" {
		return types.NewInvalidArgument("user_id is required")
	}
	if strings.TrimSpace(command.Password) == "" {
		return types.NewInvalidArgument("password is required")
	}
	if len(command.Password) < MinPasswordLength {
		return types.NewInvalidArgument("password is too short")
	}
	return nil
}

func ValidateLogin(command types.LoginCommand) error {
	if command.TenantID == "" {
		return types.NewInvalidArgument("tenant_id is required")
	}
	if command.UserID == "" {
		return types.NewInvalidArgument("user_id is required")
	}
	if command.DeviceID == "" {
		return types.NewInvalidArgument("device_id is required")
	}
	if strings.TrimSpace(command.Password) == "" {
		return types.NewInvalidArgument("password is required")
	}
	return nil
}

func ValidateRefreshGatewayToken(command types.RefreshGatewayTokenCommand) error {
	if command.TenantID == "" {
		return types.NewInvalidArgument("tenant_id is required")
	}
	if command.UserID == "" {
		return types.NewInvalidArgument("user_id is required")
	}
	if command.DeviceID == "" {
		return types.NewInvalidArgument("device_id is required")
	}
	if strings.TrimSpace(command.RefreshToken) == "" {
		return types.NewInvalidArgument("refresh_token is required")
	}
	return nil
}

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

func NormalizeRefreshTTL(seconds int64) time.Duration {
	if seconds <= 0 {
		return DefaultRefreshTTL
	}
	ttl := time.Duration(seconds) * time.Second
	if ttl > MaxRefreshTTL {
		return MaxRefreshTTL
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
