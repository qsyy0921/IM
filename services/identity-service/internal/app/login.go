package app

import (
	"context"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/domain"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

const (
	DefaultLoginMaxFailedAttempts = 5
	DefaultLoginFailureWindow     = 15 * time.Minute
	DefaultLoginLockDuration      = 15 * time.Minute
)

type LoginRiskPolicy struct {
	MaxFailedAttempts int
	FailureWindow     time.Duration
	LockDuration      time.Duration
}

type LoginUseCaseOption func(*LoginUseCase)

type LoginUseCase struct {
	repository    Repository
	signer        TokenSigner
	passwords     PasswordVerifier
	refreshTokens RefreshTokenCodec
	now           func() time.Time
	risk          LoginRiskPolicy
}

func NewLoginUseCase(repository Repository, signer TokenSigner, passwords PasswordVerifier, refreshTokens RefreshTokenCodec, opts ...LoginUseCaseOption) *LoginUseCase {
	uc := &LoginUseCase{
		repository:    repository,
		signer:        signer,
		passwords:     passwords,
		refreshTokens: refreshTokens,
		now:           func() time.Time { return time.Now().UTC() },
		risk: LoginRiskPolicy{
			MaxFailedAttempts: DefaultLoginMaxFailedAttempts,
			FailureWindow:     DefaultLoginFailureWindow,
			LockDuration:      DefaultLoginLockDuration,
		},
	}
	for _, opt := range opts {
		opt(uc)
	}
	uc.risk = normalizeLoginRiskPolicy(uc.risk)
	return uc
}

func WithLoginRiskPolicy(policy LoginRiskPolicy) LoginUseCaseOption {
	return func(uc *LoginUseCase) {
		uc.risk = policy
	}
}

func WithLoginClock(clock func() time.Time) LoginUseCaseOption {
	return func(uc *LoginUseCase) {
		if clock != nil {
			uc.now = clock
		}
	}
}

func normalizeLoginRiskPolicy(policy LoginRiskPolicy) LoginRiskPolicy {
	if policy.MaxFailedAttempts <= 0 {
		policy.MaxFailedAttempts = DefaultLoginMaxFailedAttempts
	}
	if policy.FailureWindow <= 0 {
		policy.FailureWindow = DefaultLoginFailureWindow
	}
	if policy.LockDuration <= 0 {
		policy.LockDuration = DefaultLoginLockDuration
	}
	return policy
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

	now := uc.now()
	credential, err := uc.repository.GetUserCredential(ctx, command.TenantID, command.UserID)
	if err != nil {
		return types.LoginResult{}, err
	}
	if credential.LockedUntil.After(now) {
		return types.LoginResult{}, types.NewAccountLocked("account temporarily locked")
	}
	if credential.Status != "ACTIVE" || !uc.passwords.VerifyPassword(command.Password, credential.PasswordHash) {
		lockUntil := now.Add(uc.risk.LockDuration)
		if err := uc.repository.RecordLoginFailure(ctx, command.TenantID, command.UserID, now, lockUntil, uc.risk.MaxFailedAttempts, now.Add(-uc.risk.FailureWindow)); err != nil {
			return types.LoginResult{}, err
		}
		return types.LoginResult{}, types.NewInvalidCredentials("invalid credentials")
	}

	command.Audience = domain.NormalizeAudience(command.Audience)
	issuedAt := now
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
