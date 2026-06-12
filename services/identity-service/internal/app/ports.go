package app

import (
	"context"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type Repository interface {
	GetUserCredential(context.Context, types.TenantID, types.UserID) (types.UserCredential, error)
	LoginGatewaySession(context.Context, types.LoginCommand, types.RefreshTokenRecord, time.Time, time.Time, time.Time) (types.LoginResult, error)
	RefreshGatewaySession(context.Context, types.RefreshGatewayTokenCommand, types.RefreshTokenID, string, types.RefreshTokenRecord, time.Time, time.Time, time.Time) (types.RefreshGatewayTokenResult, error)
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

type RefreshTokenCodec interface {
	NewRefreshToken() (plain string, record types.RefreshTokenRecord, err error)
	ParseRefreshToken(token string) (types.ParsedRefreshToken, error)
	HashRefreshTokenSecret(secret string) string
}
