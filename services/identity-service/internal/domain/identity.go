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
	DefaultChallengeTTL    = 15 * time.Minute
	MaxChallengeTTL        = 24 * time.Hour
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
	if strings.TrimSpace(command.MFACode) != "" && !isSixDigitCode(command.MFACode) {
		return types.NewInvalidArgument("mfa code is invalid")
	}
	if strings.TrimSpace(command.MFACode) != "" && strings.TrimSpace(command.MFARecoveryCode) != "" {
		return types.NewInvalidArgument("only one mfa credential is allowed")
	}
	if strings.TrimSpace(command.MFARecoveryCode) != "" && strings.TrimSpace(string(command.MFAFactorID)) != "" {
		return types.NewInvalidArgument("mfa_factor_id is not allowed with recovery code")
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
	if strings.TrimSpace(command.MFACode) != "" && !isSixDigitCode(command.MFACode) {
		return types.NewInvalidArgument("mfa code is invalid")
	}
	if strings.TrimSpace(command.MFACode) != "" && strings.TrimSpace(command.MFARecoveryCode) != "" {
		return types.NewInvalidArgument("only one mfa credential is allowed")
	}
	if strings.TrimSpace(command.MFARecoveryCode) != "" && strings.TrimSpace(string(command.MFAFactorID)) != "" {
		return types.NewInvalidArgument("mfa_factor_id is not allowed with recovery code")
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

func ValidateRequestVerificationChallenge(command types.RequestVerificationChallengeCommand) error {
	if err := validateChallengeTarget(command.TenantID, command.UserID, command.Channel, command.Destination); err != nil {
		return err
	}
	if strings.TrimSpace(command.Password) == "" {
		return types.NewInvalidArgument("password is required")
	}
	if command.Channel == types.VerificationChannelEmail && !strings.Contains(command.Destination, "@") {
		return types.NewInvalidArgument("email destination is invalid")
	}
	return nil
}

func ValidateConfirmVerificationChallenge(command types.ConfirmVerificationChallengeCommand) error {
	return validateChallengeConfirmation(command.TenantID, command.UserID, command.ChallengeID, command.ChallengeToken)
}

func ValidateRequestPasswordReset(command types.RequestPasswordResetCommand) error {
	return validateChallengeTarget(command.TenantID, command.UserID, command.Channel, command.Destination)
}

func ValidateConfirmPasswordReset(command types.ConfirmPasswordResetCommand) error {
	if err := validateChallengeConfirmation(command.TenantID, command.UserID, command.ChallengeID, command.ChallengeToken); err != nil {
		return err
	}
	if strings.TrimSpace(command.NewPassword) == "" {
		return types.NewInvalidArgument("new_password is required")
	}
	if len(command.NewPassword) < MinPasswordLength {
		return types.NewInvalidArgument("new_password is too short")
	}
	return nil
}

func ValidateBeginMFAEnrollment(command types.BeginMFAEnrollmentCommand) error {
	if command.TenantID == "" {
		return types.NewInvalidArgument("tenant_id is required")
	}
	if command.UserID == "" {
		return types.NewInvalidArgument("user_id is required")
	}
	if command.FactorType != types.MFAFactorTypeTOTP {
		return types.NewInvalidArgument("mfa factor type is invalid")
	}
	if strings.TrimSpace(command.Password) == "" {
		return types.NewInvalidArgument("password is required")
	}
	return nil
}

func ValidateConfirmMFAEnrollment(command types.ConfirmMFAEnrollmentCommand) error {
	if command.TenantID == "" {
		return types.NewInvalidArgument("tenant_id is required")
	}
	if command.UserID == "" {
		return types.NewInvalidArgument("user_id is required")
	}
	if command.FactorID == "" {
		return types.NewInvalidArgument("factor_id is required")
	}
	if !isSixDigitCode(command.Code) {
		return types.NewInvalidArgument("mfa code is invalid")
	}
	return nil
}

func ValidateDisableMFAFactor(command types.DisableMFAFactorCommand) error {
	if command.TenantID == "" {
		return types.NewInvalidArgument("tenant_id is required")
	}
	if command.UserID == "" {
		return types.NewInvalidArgument("user_id is required")
	}
	if command.FactorID == "" {
		return types.NewInvalidArgument("factor_id is required")
	}
	if strings.TrimSpace(command.Password) == "" {
		return types.NewInvalidArgument("password is required")
	}
	return nil
}

func ValidateRegenerateMFARecoveryCodes(command types.RegenerateMFARecoveryCodesCommand) error {
	if command.TenantID == "" {
		return types.NewInvalidArgument("tenant_id is required")
	}
	if command.UserID == "" {
		return types.NewInvalidArgument("user_id is required")
	}
	if command.FactorID == "" {
		return types.NewInvalidArgument("factor_id is required")
	}
	if strings.TrimSpace(command.Password) == "" {
		return types.NewInvalidArgument("password is required")
	}
	if !isSixDigitCode(command.Code) {
		return types.NewInvalidArgument("mfa code is invalid")
	}
	return nil
}

func ValidateRevokeMFARecoveryCodes(command types.RevokeMFARecoveryCodesCommand) error {
	if command.TenantID == "" {
		return types.NewInvalidArgument("tenant_id is required")
	}
	if command.UserID == "" {
		return types.NewInvalidArgument("user_id is required")
	}
	if strings.TrimSpace(command.Password) == "" {
		return types.NewInvalidArgument("password is required")
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

func NormalizeChallengeTTL(seconds int64) time.Duration {
	if seconds <= 0 {
		return DefaultChallengeTTL
	}
	ttl := time.Duration(seconds) * time.Second
	if ttl > MaxChallengeTTL {
		return MaxChallengeTTL
	}
	return ttl
}

func ChallengeTypeForVerificationChannel(channel types.VerificationChannel) types.ChallengeType {
	switch channel {
	case types.VerificationChannelEmail:
		return types.ChallengeTypeEmailVerification
	case types.VerificationChannelPhone:
		return types.ChallengeTypePhoneVerification
	default:
		return ""
	}
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

func validateChallengeTarget(tenantID types.TenantID, userID types.UserID, channel types.VerificationChannel, destination string) error {
	if tenantID == "" {
		return types.NewInvalidArgument("tenant_id is required")
	}
	if userID == "" {
		return types.NewInvalidArgument("user_id is required")
	}
	if channel != types.VerificationChannelEmail && channel != types.VerificationChannelPhone {
		return types.NewInvalidArgument("verification channel is invalid")
	}
	if strings.TrimSpace(destination) == "" {
		return types.NewInvalidArgument("destination is required")
	}
	return nil
}

func validateChallengeConfirmation(tenantID types.TenantID, userID types.UserID, challengeID types.ChallengeID, token string) error {
	if tenantID == "" {
		return types.NewInvalidArgument("tenant_id is required")
	}
	if userID == "" {
		return types.NewInvalidArgument("user_id is required")
	}
	if challengeID == "" {
		return types.NewInvalidArgument("challenge_id is required")
	}
	if strings.TrimSpace(token) == "" {
		return types.NewInvalidArgument("challenge_token is required")
	}
	return nil
}

func isSixDigitCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
