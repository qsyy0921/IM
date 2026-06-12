package app

import (
	"context"
	"errors"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/domain"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type ChallengeOptions struct {
	ReturnDevToken bool
}

type RequestVerificationChallengeUseCase struct {
	repository Repository
	tokens     ChallengeTokenCodec
	passwords  PasswordVerifier
	now        func() time.Time
	options    ChallengeOptions
}

func NewRequestVerificationChallengeUseCase(repository Repository, tokens ChallengeTokenCodec, passwords PasswordVerifier, options ChallengeOptions) *RequestVerificationChallengeUseCase {
	return &RequestVerificationChallengeUseCase{repository: repository, tokens: tokens, passwords: passwords, now: func() time.Time { return time.Now().UTC() }, options: options}
}

func (uc *RequestVerificationChallengeUseCase) Execute(ctx context.Context, command types.RequestVerificationChallengeCommand) (types.RequestVerificationChallengeResult, error) {
	if err := domain.ValidateRequestVerificationChallenge(command); err != nil {
		return types.RequestVerificationChallengeResult{}, err
	}
	if uc.repository == nil || uc.tokens == nil || uc.passwords == nil {
		return types.RequestVerificationChallengeResult{}, types.NewDBWriteFailed("identity challenge dependencies are not configured")
	}
	credential, err := uc.repository.GetUserCredential(ctx, command.TenantID, command.UserID)
	if err != nil {
		return types.RequestVerificationChallengeResult{}, err
	}
	if !uc.passwords.VerifyPassword(command.Password, credential.PasswordHash) {
		return types.RequestVerificationChallengeResult{}, types.NewInvalidCredentials("invalid credentials")
	}
	plain, record, err := uc.tokens.NewChallengeToken()
	if err != nil {
		return types.RequestVerificationChallengeResult{}, err
	}
	issuedAt := uc.now()
	result, err := uc.repository.CreateVerificationChallenge(ctx, command, domain.ChallengeTypeForVerificationChannel(command.Channel), record, issuedAt, issuedAt.Add(domain.NormalizeChallengeTTL(command.TTLSeconds)))
	if err != nil {
		return types.RequestVerificationChallengeResult{}, err
	}
	if uc.options.ReturnDevToken {
		result.DevChallengeToken = plain
	}
	return result, nil
}

type ConfirmVerificationChallengeUseCase struct {
	repository Repository
	tokens     ChallengeTokenCodec
	now        func() time.Time
}

func NewConfirmVerificationChallengeUseCase(repository Repository, tokens ChallengeTokenCodec) *ConfirmVerificationChallengeUseCase {
	return &ConfirmVerificationChallengeUseCase{repository: repository, tokens: tokens, now: func() time.Time { return time.Now().UTC() }}
}

func (uc *ConfirmVerificationChallengeUseCase) Execute(ctx context.Context, command types.ConfirmVerificationChallengeCommand) (types.ConfirmVerificationChallengeResult, error) {
	if err := domain.ValidateConfirmVerificationChallenge(command); err != nil {
		return types.ConfirmVerificationChallengeResult{}, err
	}
	if uc.repository == nil || uc.tokens == nil {
		return types.ConfirmVerificationChallengeResult{}, types.NewDBWriteFailed("identity challenge dependencies are not configured")
	}
	return uc.repository.ConfirmVerificationChallenge(ctx, command, uc.tokens.HashChallengeToken(command.ChallengeToken), uc.now())
}

type RequestPasswordResetUseCase struct {
	repository Repository
	tokens     ChallengeTokenCodec
	now        func() time.Time
	options    ChallengeOptions
}

func NewRequestPasswordResetUseCase(repository Repository, tokens ChallengeTokenCodec, options ChallengeOptions) *RequestPasswordResetUseCase {
	return &RequestPasswordResetUseCase{repository: repository, tokens: tokens, now: func() time.Time { return time.Now().UTC() }, options: options}
}

func (uc *RequestPasswordResetUseCase) Execute(ctx context.Context, command types.RequestPasswordResetCommand) (types.RequestPasswordResetResult, error) {
	if err := domain.ValidateRequestPasswordReset(command); err != nil {
		return types.RequestPasswordResetResult{}, err
	}
	if uc.repository == nil || uc.tokens == nil {
		return types.RequestPasswordResetResult{}, types.NewDBWriteFailed("identity challenge dependencies are not configured")
	}
	_, record, err := uc.tokens.NewChallengeToken()
	if err != nil {
		return types.RequestPasswordResetResult{}, err
	}
	issuedAt := uc.now()
	result, err := uc.repository.CreatePasswordResetChallenge(ctx, command, record, issuedAt, issuedAt.Add(domain.NormalizeChallengeTTL(command.TTLSeconds)))
	if err != nil {
		if errors.Is(err, types.ErrInvalidCredentials) || errors.Is(err, types.ErrChallengeRateLimited) {
			return neutralPasswordResetResult(command, record.ChallengeID, issuedAt.Add(domain.NormalizeChallengeTTL(command.TTLSeconds))), nil
		}
		return types.RequestPasswordResetResult{}, err
	}
	return result, nil
}

func neutralPasswordResetResult(command types.RequestPasswordResetCommand, challengeID types.ChallengeID, expiresAt time.Time) types.RequestPasswordResetResult {
	return types.RequestPasswordResetResult{
		TenantID:        command.TenantID,
		UserID:          command.UserID,
		ChallengeID:     challengeID,
		Channel:         command.Channel,
		Destination:     command.Destination,
		ExpiresAtUnixMS: expiresAt.UnixMilli(),
	}
}

type ConfirmPasswordResetUseCase struct {
	repository Repository
	tokens     ChallengeTokenCodec
	passwords  PasswordHasher
	now        func() time.Time
}

func NewConfirmPasswordResetUseCase(repository Repository, tokens ChallengeTokenCodec, passwords PasswordHasher) *ConfirmPasswordResetUseCase {
	return &ConfirmPasswordResetUseCase{repository: repository, tokens: tokens, passwords: passwords, now: func() time.Time { return time.Now().UTC() }}
}

func (uc *ConfirmPasswordResetUseCase) Execute(ctx context.Context, command types.ConfirmPasswordResetCommand) (types.ConfirmPasswordResetResult, error) {
	if err := domain.ValidateConfirmPasswordReset(command); err != nil {
		return types.ConfirmPasswordResetResult{}, err
	}
	if uc.repository == nil || uc.tokens == nil || uc.passwords == nil {
		return types.ConfirmPasswordResetResult{}, types.NewDBWriteFailed("identity challenge dependencies are not configured")
	}
	passwordHash, err := uc.passwords.HashPassword(command.NewPassword)
	if err != nil {
		return types.ConfirmPasswordResetResult{}, err
	}
	return uc.repository.ConfirmPasswordReset(ctx, command, uc.tokens.HashChallengeToken(command.ChallengeToken), passwordHash, uc.now())
}
