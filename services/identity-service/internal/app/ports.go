package app

import (
	"context"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type Repository interface {
	RegisterUser(context.Context, types.RegisterUserCommand, string, time.Time) (types.RegisterUserResult, error)
	GetUserCredential(context.Context, types.TenantID, types.UserID) (types.UserCredential, error)
	RecordLoginFailure(context.Context, types.TenantID, types.UserID, time.Time, time.Time, int, time.Time) error
	LoginGatewaySession(context.Context, types.LoginCommand, types.RefreshTokenRecord, time.Time, time.Time, time.Time) (types.LoginResult, error)
	RefreshGatewaySession(context.Context, types.RefreshGatewayTokenCommand, types.RefreshTokenID, string, types.RefreshTokenRecord, time.Time, time.Time, time.Time) (types.RefreshGatewayTokenResult, error)
	CreateVerificationChallenge(context.Context, types.RequestVerificationChallengeCommand, types.ChallengeType, types.ChallengeRecord, time.Time, time.Time) (types.RequestVerificationChallengeResult, error)
	ConfirmVerificationChallenge(context.Context, types.ConfirmVerificationChallengeCommand, string, time.Time) (types.ConfirmVerificationChallengeResult, error)
	CreatePasswordResetChallenge(context.Context, types.RequestPasswordResetCommand, types.ChallengeRecord, time.Time, time.Time) (types.RequestPasswordResetResult, error)
	ConfirmPasswordReset(context.Context, types.ConfirmPasswordResetCommand, string, string, time.Time) (types.ConfirmPasswordResetResult, error)
	IssueGatewaySession(context.Context, types.IssueGatewayTokenCommand, time.Time, time.Time) (types.IssueGatewayTokenResult, error)
	RevokeDevice(context.Context, types.RevokeDeviceCommand, time.Time) (types.RevokeDeviceResult, error)
	RevokeSession(context.Context, types.RevokeSessionCommand, time.Time) (types.RevokeSessionResult, error)
	GetDeviceState(context.Context, types.GetDeviceStateCommand) (types.GetDeviceStateResult, error)
}

type TokenSigner interface {
	SignGatewayToken(types.TokenClaims) (string, error)
}

type PasswordVerifier interface {
	VerifyPassword(password string, passwordHash string) bool
}

type PasswordHasher interface {
	HashPassword(password string) (string, error)
}

type RefreshTokenCodec interface {
	NewRefreshToken() (plain string, record types.RefreshTokenRecord, err error)
	ParseRefreshToken(token string) (types.ParsedRefreshToken, error)
	HashRefreshTokenSecret(secret string) string
}

type ChallengeTokenCodec interface {
	NewChallengeToken() (plain string, record types.ChallengeRecord, err error)
	HashChallengeToken(token string) string
}

type MFASecretManager interface {
	NewTOTPSecret() (plain string, encrypted types.EncryptedMFASecret, err error)
	VerifyTOTP(encrypted types.EncryptedMFASecret, code string, now time.Time) (bool, error)
	OTPAuthURI(issuer string, accountName string, secret string) string
}
