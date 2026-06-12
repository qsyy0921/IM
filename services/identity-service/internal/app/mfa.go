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
	ConfirmMFAFactor(context.Context, types.ConfirmMFAEnrollmentCommand, []types.MFARecoveryCodeRecord, time.Time) (types.ConfirmMFAEnrollmentResult, error)
	DisableMFAFactor(context.Context, types.DisableMFAFactorCommand, time.Time) (types.DisableMFAFactorResult, error)
	ReplaceMFARecoveryCodes(context.Context, types.RegenerateMFARecoveryCodesCommand, []types.MFARecoveryCodeRecord, time.Time) (types.RegenerateMFARecoveryCodesResult, error)
	RevokeMFARecoveryCodes(context.Context, types.RevokeMFARecoveryCodesCommand, time.Time) (types.RevokeMFARecoveryCodesResult, error)
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
	recovery   MFARecoveryCodeManager
	now        func() time.Time
}

const defaultMFARecoveryCodeCount = 10

func NewConfirmMFAEnrollmentUseCase(repository MFARepository, secrets MFASecretManager, recovery ...MFARecoveryCodeManager) *ConfirmMFAEnrollmentUseCase {
	uc := &ConfirmMFAEnrollmentUseCase{repository: repository, secrets: secrets, now: func() time.Time { return time.Now().UTC() }}
	if len(recovery) > 0 {
		uc.recovery = recovery[0]
	}
	return uc
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
	if uc.recovery == nil {
		return types.ConfirmMFAEnrollmentResult{}, types.NewMFAUnavailable("mfa recovery code manager is not configured")
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
	recoveryCodes, err := uc.recovery.NewRecoveryCodes(defaultMFARecoveryCodeCount)
	if err != nil {
		return types.ConfirmMFAEnrollmentResult{}, err
	}
	records, plainCodes := recoveryCodeRecords(recoveryCodes)
	result, err := uc.repository.ConfirmMFAFactor(ctx, command, records, now)
	if err != nil {
		return types.ConfirmMFAEnrollmentResult{}, err
	}
	result.RecoveryCodes = plainCodes
	return result, nil
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

type RegenerateMFARecoveryCodesUseCase struct {
	repository MFARepository
	passwords  PasswordVerifier
	secrets    MFASecretManager
	recovery   MFARecoveryCodeManager
	now        func() time.Time
}

func NewRegenerateMFARecoveryCodesUseCase(repository MFARepository, passwords PasswordVerifier, secrets MFASecretManager, recovery MFARecoveryCodeManager) *RegenerateMFARecoveryCodesUseCase {
	return &RegenerateMFARecoveryCodesUseCase{repository: repository, passwords: passwords, secrets: secrets, recovery: recovery, now: func() time.Time { return time.Now().UTC() }}
}

func (uc *RegenerateMFARecoveryCodesUseCase) Execute(ctx context.Context, command types.RegenerateMFARecoveryCodesCommand) (types.RegenerateMFARecoveryCodesResult, error) {
	if err := domain.ValidateRegenerateMFARecoveryCodes(command); err != nil {
		return types.RegenerateMFARecoveryCodesResult{}, err
	}
	if uc.repository == nil || uc.passwords == nil {
		return types.RegenerateMFARecoveryCodesResult{}, types.NewDBWriteFailed("identity mfa dependencies are not configured")
	}
	if uc.secrets == nil || uc.recovery == nil {
		return types.RegenerateMFARecoveryCodesResult{}, types.NewMFAUnavailable("mfa dependencies are not configured")
	}
	credential, err := uc.repository.GetUserCredential(ctx, command.TenantID, command.UserID)
	if err != nil {
		return types.RegenerateMFARecoveryCodesResult{}, err
	}
	if !uc.passwords.VerifyPassword(command.Password, credential.PasswordHash) {
		return types.RegenerateMFARecoveryCodesResult{}, types.NewInvalidCredentials("invalid credentials")
	}
	factor, err := uc.repository.GetMFAFactorSecret(ctx, command.TenantID, command.UserID, command.FactorID)
	if err != nil {
		return types.RegenerateMFARecoveryCodesResult{}, err
	}
	if factor.Type != types.MFAFactorTypeTOTP || factor.Status != types.MFAFactorStatusActive {
		return types.RegenerateMFARecoveryCodesResult{}, types.NewMFAFactorNotFound("mfa factor not found")
	}
	now := uc.now()
	ok, err := uc.secrets.VerifyTOTP(factor.Secret, command.Code, now)
	if err != nil {
		return types.RegenerateMFARecoveryCodesResult{}, err
	}
	if !ok {
		return types.RegenerateMFARecoveryCodesResult{}, types.NewInvalidMFA("invalid mfa code")
	}
	recoveryCodes, err := uc.recovery.NewRecoveryCodes(defaultMFARecoveryCodeCount)
	if err != nil {
		return types.RegenerateMFARecoveryCodesResult{}, err
	}
	records, plainCodes := recoveryCodeRecords(recoveryCodes)
	result, err := uc.repository.ReplaceMFARecoveryCodes(ctx, command, records, now)
	if err != nil {
		return types.RegenerateMFARecoveryCodesResult{}, err
	}
	result.RecoveryCodes = plainCodes
	return result, nil
}

type RevokeMFARecoveryCodesUseCase struct {
	repository MFARepository
	passwords  PasswordVerifier
	now        func() time.Time
}

func NewRevokeMFARecoveryCodesUseCase(repository MFARepository, passwords PasswordVerifier) *RevokeMFARecoveryCodesUseCase {
	return &RevokeMFARecoveryCodesUseCase{repository: repository, passwords: passwords, now: func() time.Time { return time.Now().UTC() }}
}

func (uc *RevokeMFARecoveryCodesUseCase) Execute(ctx context.Context, command types.RevokeMFARecoveryCodesCommand) (types.RevokeMFARecoveryCodesResult, error) {
	if err := domain.ValidateRevokeMFARecoveryCodes(command); err != nil {
		return types.RevokeMFARecoveryCodesResult{}, err
	}
	if uc.repository == nil || uc.passwords == nil {
		return types.RevokeMFARecoveryCodesResult{}, types.NewDBWriteFailed("identity mfa dependencies are not configured")
	}
	credential, err := uc.repository.GetUserCredential(ctx, command.TenantID, command.UserID)
	if err != nil {
		return types.RevokeMFARecoveryCodesResult{}, err
	}
	if !uc.passwords.VerifyPassword(command.Password, credential.PasswordHash) {
		return types.RevokeMFARecoveryCodesResult{}, types.NewInvalidCredentials("invalid credentials")
	}
	return uc.repository.RevokeMFARecoveryCodes(ctx, command, uc.now())
}

func recoveryCodeRecords(codes []types.MFARecoveryCode) ([]types.MFARecoveryCodeRecord, []string) {
	records := make([]types.MFARecoveryCodeRecord, 0, len(codes))
	plainCodes := make([]string, 0, len(codes))
	for _, code := range codes {
		records = append(records, types.MFARecoveryCodeRecord{CodeID: code.CodeID, CodeHash: code.CodeHash})
		plainCodes = append(plainCodes, code.Code)
	}
	return records, plainCodes
}
