package app

import (
	"context"
	"fmt"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/domain"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type BeginMFAEnrollmentUseCase struct {
	repository MFARepository
	passwords  PasswordVerifier
	secrets    MFASecretManager
	now        func() time.Time
}

type MFARepository interface {
	GetUserCredential(context.Context, types.TenantID, types.UserID) (types.UserCredential, error)
	CreateMFAFactor(context.Context, types.BeginMFAEnrollmentCommand, types.EncryptedMFASecret, time.Time) (types.BeginMFAEnrollmentResult, error)
	GetMFAFactorSecret(context.Context, types.TenantID, types.UserID, types.MFAFactorID) (types.MFAFactorSecret, error)
	ConfirmMFAFactor(context.Context, types.ConfirmMFAEnrollmentCommand, time.Time) (types.ConfirmMFAEnrollmentResult, error)
	DisableMFAFactor(context.Context, types.DisableMFAFactorCommand, time.Time) (types.DisableMFAFactorResult, error)
}

func NewBeginMFAEnrollmentUseCase(repository MFARepository, passwords PasswordVerifier, secrets MFASecretManager) *BeginMFAEnrollmentUseCase {
	return &BeginMFAEnrollmentUseCase{repository: repository, passwords: passwords, secrets: secrets, now: func() time.Time { return time.Now().UTC() }}
}

func (uc *BeginMFAEnrollmentUseCase) Execute(ctx context.Context, command types.BeginMFAEnrollmentCommand) (types.BeginMFAEnrollmentResult, error) {
	if err := domain.ValidateBeginMFAEnrollment(command); err != nil {
		return types.BeginMFAEnrollmentResult{}, err
	}
	if uc.repository == nil || uc.passwords == nil {
		return types.BeginMFAEnrollmentResult{}, types.NewDBWriteFailed("identity mfa dependencies are not configured")
	}
	if uc.secrets == nil {
		return types.BeginMFAEnrollmentResult{}, types.NewMFAUnavailable("mfa secret manager is not configured")
	}
	credential, err := uc.repository.GetUserCredential(ctx, command.TenantID, command.UserID)
	if err != nil {
		return types.BeginMFAEnrollmentResult{}, err
	}
	if !uc.passwords.VerifyPassword(command.Password, credential.PasswordHash) {
		return types.BeginMFAEnrollmentResult{}, types.NewInvalidCredentials("invalid credentials")
	}
	plain, encrypted, err := uc.secrets.NewTOTPSecret()
	if err != nil {
		return types.BeginMFAEnrollmentResult{}, err
	}
	result, err := uc.repository.CreateMFAFactor(ctx, command, encrypted, uc.now())
	if err != nil {
		return types.BeginMFAEnrollmentResult{}, err
	}
	result.Secret = plain
	result.OTPAuthURI = uc.secrets.OTPAuthURI(command.Issuer, fmt.Sprintf("%s:%s", command.TenantID, command.UserID), plain)
	return result, nil
}

type ConfirmMFAEnrollmentUseCase struct {
	repository MFARepository
	secrets    MFASecretManager
	now        func() time.Time
}

func NewConfirmMFAEnrollmentUseCase(repository MFARepository, secrets MFASecretManager) *ConfirmMFAEnrollmentUseCase {
	return &ConfirmMFAEnrollmentUseCase{repository: repository, secrets: secrets, now: func() time.Time { return time.Now().UTC() }}
}

func (uc *ConfirmMFAEnrollmentUseCase) Execute(ctx context.Context, command types.ConfirmMFAEnrollmentCommand) (types.ConfirmMFAEnrollmentResult, error) {
	if err := domain.ValidateConfirmMFAEnrollment(command); err != nil {
		return types.ConfirmMFAEnrollmentResult{}, err
	}
	if uc.repository == nil {
		return types.ConfirmMFAEnrollmentResult{}, types.NewDBWriteFailed("identity mfa dependencies are not configured")
	}
	if uc.secrets == nil {
		return types.ConfirmMFAEnrollmentResult{}, types.NewMFAUnavailable("mfa secret manager is not configured")
	}
	factor, err := uc.repository.GetMFAFactorSecret(ctx, command.TenantID, command.UserID, command.FactorID)
	if err != nil {
		return types.ConfirmMFAEnrollmentResult{}, err
	}
	if factor.Type != types.MFAFactorTypeTOTP || factor.Status != types.MFAFactorStatusPending {
		return types.ConfirmMFAEnrollmentResult{}, types.NewInvalidMFA("mfa factor is not pending")
	}
	now := uc.now()
	ok, err := uc.secrets.VerifyTOTP(factor.Secret, command.Code, now)
	if err != nil {
		return types.ConfirmMFAEnrollmentResult{}, err
	}
	if !ok {
		return types.ConfirmMFAEnrollmentResult{}, types.NewInvalidMFA("invalid mfa code")
	}
	return uc.repository.ConfirmMFAFactor(ctx, command, now)
}

type DisableMFAFactorUseCase struct {
	repository MFARepository
	passwords  PasswordVerifier
	now        func() time.Time
}

func NewDisableMFAFactorUseCase(repository MFARepository, passwords PasswordVerifier) *DisableMFAFactorUseCase {
	return &DisableMFAFactorUseCase{repository: repository, passwords: passwords, now: func() time.Time { return time.Now().UTC() }}
}

func (uc *DisableMFAFactorUseCase) Execute(ctx context.Context, command types.DisableMFAFactorCommand) (types.DisableMFAFactorResult, error) {
	if err := domain.ValidateDisableMFAFactor(command); err != nil {
		return types.DisableMFAFactorResult{}, err
	}
	if uc.repository == nil || uc.passwords == nil {
		return types.DisableMFAFactorResult{}, types.NewDBWriteFailed("identity mfa dependencies are not configured")
	}
	credential, err := uc.repository.GetUserCredential(ctx, command.TenantID, command.UserID)
	if err != nil {
		return types.DisableMFAFactorResult{}, err
	}
	if !uc.passwords.VerifyPassword(command.Password, credential.PasswordHash) {
		return types.DisableMFAFactorResult{}, types.NewInvalidCredentials("invalid credentials")
	}
	return uc.repository.DisableMFAFactor(ctx, command, uc.now())
}
