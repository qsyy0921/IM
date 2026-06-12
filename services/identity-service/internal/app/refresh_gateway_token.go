package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/domain"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type RefreshGatewayTokenUseCase struct {
	repository    Repository
	signer        TokenSigner
	refreshTokens RefreshTokenCodec
	mfaSecrets    MFASecretManager
	recoveryCodes MFARecoveryCodeManager
	now           func() time.Time
	mfaRisk       LoginRiskPolicy
}

type RefreshGatewayTokenUseCaseOption func(*RefreshGatewayTokenUseCase)

func NewRefreshGatewayTokenUseCase(repository Repository, signer TokenSigner, refreshTokens RefreshTokenCodec, opts ...RefreshGatewayTokenUseCaseOption) *RefreshGatewayTokenUseCase {
	uc := &RefreshGatewayTokenUseCase{
		repository:    repository,
		signer:        signer,
		refreshTokens: refreshTokens,
		now:           func() time.Time { return time.Now().UTC() },
		mfaRisk: LoginRiskPolicy{
			MaxFailedAttempts: DefaultMFAMaxFailedAttempts,
			FailureWindow:     DefaultMFAFailureWindow,
			LockDuration:      DefaultMFALockDuration,
		},
	}
	for _, opt := range opts {
		opt(uc)
	}
	uc.mfaRisk = normalizeMFARiskPolicy(uc.mfaRisk)
	return uc
}

func WithRefreshMFASecretManager(manager MFASecretManager) RefreshGatewayTokenUseCaseOption {
	return func(uc *RefreshGatewayTokenUseCase) {
		uc.mfaSecrets = manager
	}
}

func WithRefreshMFARecoveryCodeManager(manager MFARecoveryCodeManager) RefreshGatewayTokenUseCaseOption {
	return func(uc *RefreshGatewayTokenUseCase) {
		uc.recoveryCodes = manager
	}
}

func WithRefreshMFARiskPolicy(policy LoginRiskPolicy) RefreshGatewayTokenUseCaseOption {
	return func(uc *RefreshGatewayTokenUseCase) {
		uc.mfaRisk = policy
	}
}

func WithRefreshClock(clock func() time.Time) RefreshGatewayTokenUseCaseOption {
	return func(uc *RefreshGatewayTokenUseCase) {
		if clock != nil {
			uc.now = clock
		}
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

	issuedAt := uc.now()
	if hasRefreshMFAProof(command) {
		if err := uc.repository.ValidateRefreshGatewaySession(ctx, command, parsed.TokenID, presentedHash, issuedAt); err != nil {
			return types.RefreshGatewayTokenResult{}, err
		}
	}
	verifiedMFAFactorID, usedRecoveryCode, err := uc.verifyMFAIfSubmitted(ctx, command, issuedAt)
	if err != nil {
		return types.RefreshGatewayTokenResult{}, err
	}
	command.VerifiedMFAFactorID = verifiedMFAFactorID
	command.UsedMFARecoveryCode = usedRecoveryCode

	command.Audience = domain.NormalizeAudience(command.Audience)
	gatewayExpiresAt := issuedAt.Add(domain.NormalizeTTL(command.GatewayTTLSeconds))
	refreshExpiresAt := issuedAt.Add(domain.NormalizeRefreshTTL(command.RefreshTTLSeconds))
	nextRefreshToken, nextRecord, err := uc.refreshTokens.NewRefreshToken()
	if err != nil {
		return types.RefreshGatewayTokenResult{}, types.NewTokenSigningFailed(err.Error())
	}
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

func hasRefreshMFAProof(command types.RefreshGatewayTokenCommand) bool {
	return strings.TrimSpace(command.MFACode) != "" || strings.TrimSpace(command.MFARecoveryCode) != ""
}

func (uc *RefreshGatewayTokenUseCase) verifyMFAIfSubmitted(ctx context.Context, command types.RefreshGatewayTokenCommand, now time.Time) (types.MFAFactorID, types.MFARecoveryCodeRecord, error) {
	if strings.TrimSpace(command.MFACode) == "" && strings.TrimSpace(command.MFARecoveryCode) == "" {
		return "", types.MFARecoveryCodeRecord{}, nil
	}
	factors, err := uc.repository.ListActiveMFAFactorSecrets(ctx, command.TenantID, command.UserID)
	if err != nil {
		return "", types.MFARecoveryCodeRecord{}, err
	}
	if len(factors) == 0 {
		return "", types.MFARecoveryCodeRecord{}, types.NewInvalidMFA("invalid mfa")
	}
	if recoveryCode := strings.TrimSpace(command.MFARecoveryCode); recoveryCode != "" {
		if uc.recoveryCodes == nil {
			return "", types.MFARecoveryCodeRecord{}, types.NewMFAUnavailable("mfa recovery code manager is not configured")
		}
		credential, err := uc.repository.GetUserCredential(ctx, command.TenantID, command.UserID)
		if err != nil {
			return "", types.MFARecoveryCodeRecord{}, err
		}
		if credential.MFARecoveryLockedUntil.After(now) {
			return "", types.MFARecoveryCodeRecord{}, types.NewMFALocked("mfa temporarily locked")
		}
		codeHash, err := uc.recoveryCodes.HashRecoveryCode(recoveryCode)
		if err != nil {
			return "", types.MFARecoveryCodeRecord{}, err
		}
		record, err := uc.repository.FindActiveMFARecoveryCode(ctx, command.TenantID, command.UserID, codeHash)
		if err != nil {
			if errors.Is(err, types.ErrInvalidMFA) {
				lockUntil := now.Add(uc.mfaRisk.LockDuration)
				if recordErr := uc.repository.RecordMFARecoveryLoginFailure(ctx, command.TenantID, command.UserID, now, lockUntil, uc.mfaRisk.MaxFailedAttempts, now.Add(-uc.mfaRisk.FailureWindow)); recordErr != nil {
					return "", types.MFARecoveryCodeRecord{}, recordErr
				}
			}
			return "", types.MFARecoveryCodeRecord{}, err
		}
		return "", record, nil
	}
	if uc.mfaSecrets == nil {
		return "", types.MFARecoveryCodeRecord{}, types.NewMFAUnavailable("mfa secret manager is not configured")
	}
	factor, ok := selectMFAFactor(factors, command.MFAFactorID)
	if !ok {
		return "", types.MFARecoveryCodeRecord{}, types.NewInvalidMFA("invalid mfa factor")
	}
	if factor.LoginLockedUntil.After(now) {
		return "", types.MFARecoveryCodeRecord{}, types.NewMFALocked("mfa temporarily locked")
	}
	verified, err := uc.mfaSecrets.VerifyTOTP(factor.Secret, strings.TrimSpace(command.MFACode), now)
	if err != nil {
		return "", types.MFARecoveryCodeRecord{}, err
	}
	if !verified {
		lockUntil := now.Add(uc.mfaRisk.LockDuration)
		if err := uc.repository.RecordMFALoginFailure(ctx, command.TenantID, command.UserID, factor.FactorID, now, lockUntil, uc.mfaRisk.MaxFailedAttempts, now.Add(-uc.mfaRisk.FailureWindow)); err != nil {
			return "", types.MFARecoveryCodeRecord{}, err
		}
		return "", types.MFARecoveryCodeRecord{}, types.NewInvalidMFA("invalid mfa code")
	}
	return factor.FactorID, types.MFARecoveryCodeRecord{}, nil
}
