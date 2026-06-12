package app

import (
	"context"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/domain"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type RefreshGatewayTokenUseCase struct {
	repository    Repository
	signer        TokenSigner
	refreshTokens RefreshTokenCodec
	now           func() time.Time
}

func NewRefreshGatewayTokenUseCase(repository Repository, signer TokenSigner, refreshTokens RefreshTokenCodec) *RefreshGatewayTokenUseCase {
	return &RefreshGatewayTokenUseCase{
		repository:    repository,
		signer:        signer,
		refreshTokens: refreshTokens,
		now:           func() time.Time { return time.Now().UTC() },
	}
}

func (uc *RefreshGatewayTokenUseCase) Execute(ctx context.Context, command types.RefreshGatewayTokenCommand) (types.RefreshGatewayTokenResult, error) {
	if err := domain.ValidateRefreshGatewayToken(command); err != nil {
		return types.RefreshGatewayTokenResult{}, err
	}
	if uc.repository == nil {
		return types.RefreshGatewayTokenResult{}, types.NewDBWriteFailed("identity repository is not configured")
	}
	if uc.signer == nil {
		return types.RefreshGatewayTokenResult{}, types.NewTokenSigningFailed("token signer is not configured")
	}
	if uc.refreshTokens == nil {
		return types.RefreshGatewayTokenResult{}, types.NewInvalidRefreshToken("refresh token codec is not configured")
	}

	parsed, err := uc.refreshTokens.ParseRefreshToken(command.RefreshToken)
	if err != nil {
		return types.RefreshGatewayTokenResult{}, err
	}
	presentedHash := uc.refreshTokens.HashRefreshTokenSecret(parsed.Secret)
	nextRefreshToken, nextRecord, err := uc.refreshTokens.NewRefreshToken()
	if err != nil {
		return types.RefreshGatewayTokenResult{}, types.NewTokenSigningFailed(err.Error())
	}

	command.Audience = domain.NormalizeAudience(command.Audience)
	issuedAt := uc.now()
	gatewayExpiresAt := issuedAt.Add(domain.NormalizeTTL(command.GatewayTTLSeconds))
	refreshExpiresAt := issuedAt.Add(domain.NormalizeRefreshTTL(command.RefreshTTLSeconds))
	result, err := uc.repository.RefreshGatewaySession(ctx, command, parsed.TokenID, presentedHash, nextRecord, issuedAt, gatewayExpiresAt, refreshExpiresAt)
	if err != nil {
		return types.RefreshGatewayTokenResult{}, err
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
		return types.RefreshGatewayTokenResult{}, err
	}
	result.TokenType = "Bearer"
	result.GatewayToken = gatewayToken
	result.RefreshToken = nextRefreshToken
	return result, nil
}
