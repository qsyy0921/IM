package app

import (
	"context"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/domain"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type LoginUseCase struct {
	repository    Repository
	signer        TokenSigner
	passwords     PasswordVerifier
	refreshTokens RefreshTokenCodec
	now           func() time.Time
}

func NewLoginUseCase(repository Repository, signer TokenSigner, passwords PasswordVerifier, refreshTokens RefreshTokenCodec) *LoginUseCase {
	return &LoginUseCase{
		repository:    repository,
		signer:        signer,
		passwords:     passwords,
		refreshTokens: refreshTokens,
		now:           func() time.Time { return time.Now().UTC() },
	}
}

func (uc *LoginUseCase) Execute(ctx context.Context, command types.LoginCommand) (types.LoginResult, error) {
	if err := domain.ValidateLogin(command); err != nil {
		return types.LoginResult{}, err
	}
	if uc.repository == nil {
		return types.LoginResult{}, types.NewDBWriteFailed("identity repository is not configured")
	}
	if uc.signer == nil {
		return types.LoginResult{}, types.NewTokenSigningFailed("token signer is not configured")
	}
	if uc.passwords == nil || uc.refreshTokens == nil {
		return types.LoginResult{}, types.NewInvalidCredentials("credential verifier is not configured")
	}

	credential, err := uc.repository.GetUserCredential(ctx, command.TenantID, command.UserID)
	if err != nil {
		return types.LoginResult{}, err
	}
	if credential.Status != "ACTIVE" || !uc.passwords.VerifyPassword(command.Password, credential.PasswordHash) {
		return types.LoginResult{}, types.NewInvalidCredentials("invalid credentials")
	}

	command.Audience = domain.NormalizeAudience(command.Audience)
	issuedAt := uc.now()
	gatewayExpiresAt := issuedAt.Add(domain.NormalizeTTL(command.GatewayTTLSeconds))
	refreshExpiresAt := issuedAt.Add(domain.NormalizeRefreshTTL(command.RefreshTTLSeconds))
	refreshToken, refreshRecord, err := uc.refreshTokens.NewRefreshToken()
	if err != nil {
		return types.LoginResult{}, types.NewTokenSigningFailed(err.Error())
	}

	result, err := uc.repository.LoginGatewaySession(ctx, command, refreshRecord, issuedAt, gatewayExpiresAt, refreshExpiresAt)
	if err != nil {
		return types.LoginResult{}, err
	}
	gatewayToken, err := uc.signer.SignGatewayToken(types.TokenClaims{
		TenantID:  result.TenantID,
		UserID:    result.UserID,
		DeviceID:  result.DeviceID,
		SessionID: result.SessionID,
		Audience:  result.Audience,
		TraceID:   command.TraceID,
		IssuedAt:  issuedAt.Unix(),
		ExpiresAt: gatewayExpiresAt.Unix(),
	})
	if err != nil {
		return types.LoginResult{}, err
	}
	result.TokenType = "Bearer"
	result.GatewayToken = gatewayToken
	result.RefreshToken = refreshToken
	return result, nil
}
