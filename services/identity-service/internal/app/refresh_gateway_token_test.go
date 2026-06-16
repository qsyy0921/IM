package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestRefreshGatewayTokenUseCaseRotatesRefreshToken(t *testing.T) {
	repository := &fakeIdentityRepository{}
	useCase := NewRefreshGatewayTokenUseCase(repository, fakeTokenSigner{}, fakeRefreshTokenCodec{})
	result, err := useCase.Execute(context.Background(), types.RefreshGatewayTokenCommand{
		TenantID:     "tenant-1",
		UserID:       "user-1",
		DeviceID:     "device-1",
		RefreshToken: "rft_old.secret-old",
	})
	if err != nil {
		t.Fatalf("refresh gateway token: %v", err)
	}
	if !repository.refreshCalled {
		t.Fatal("expected repository refresh to be called")
	}
	if repository.presentedTokenID != "rft_old" || repository.presentedTokenHash == "" {
		t.Fatalf("unexpected presented token: id=%s hash=%s", repository.presentedTokenID, repository.presentedTokenHash)
	}
	if result.RefreshToken != "rft_new.secret-new" || result.GatewayToken != "gateway-token" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRefreshGatewayTokenUseCaseRejectsMFAFactorIDWithoutCode(t *testing.T) {
	repository := &fakeIdentityRepository{}
	useCase := NewRefreshGatewayTokenUseCase(repository, fakeTokenSigner{}, fakeRefreshTokenCodec{})
	_, err := useCase.Execute(context.Background(), types.RefreshGatewayTokenCommand{
		TenantID:     "tenant-1",
		UserID:       "user-1",
		DeviceID:     "device-1",
		RefreshToken: "rft_old.secret-old",
		MFAFactorID:  "mfa-1",
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if repository.validateRefreshCalled || repository.refreshCalled {
		t.Fatal("invalid mfa field combination must not preflight or rotate refresh token")
	}
}

func TestRefreshGatewayTokenUseCaseAcceptsTOTPProofForStepUp(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	secret := types.EncryptedMFASecret{Ciphertext: "ciphertext", Nonce: "nonce", KeyVersion: "local-v1"}
	repository := &fakeIdentityRepository{
		activeMFAFactors: []types.MFAFactorSecret{{
			TenantID: "tenant-1",
			UserID:   "user-1",
			FactorID: "mfa-1",
			Type:     types.MFAFactorTypeTOTP,
			Status:   types.MFAFactorStatusActive,
			Secret:   secret,
		}},
	}
	secrets := &fakeMFASecretManager{verifyOK: true}
	useCase := NewRefreshGatewayTokenUseCase(
		repository,
		fakeTokenSigner{},
		fakeRefreshTokenCodec{},
		WithRefreshClock(func() time.Time { return now }),
		WithRefreshMFASecretManager(secrets),
	)
	result, err := useCase.Execute(context.Background(), types.RefreshGatewayTokenCommand{
		TenantID:     "tenant-1",
		UserID:       "user-1",
		DeviceID:     "device-1",
		RefreshToken: "rft_old.secret-old",
		MFAFactorID:  "mfa-1",
		MFACode:      "123456",
	})
	if err != nil {
		t.Fatalf("refresh with mfa proof: %v", err)
	}
	if !repository.refreshCalled || result.GatewayToken != "gateway-token" {
		t.Fatalf("expected refresh to proceed after mfa proof, result=%+v refreshCalled=%v", result, repository.refreshCalled)
	}
	if !repository.validateRefreshCalled || repository.validatePresentedTokenID != "rft_old" || repository.validatePresentedTokenHash == "" {
		t.Fatalf("expected refresh token preflight before mfa proof, called=%v tokenID=%s hash=%s", repository.validateRefreshCalled, repository.validatePresentedTokenID, repository.validatePresentedTokenHash)
	}
	if repository.refreshCommand.VerifiedMFAFactorID != "mfa-1" {
		t.Fatalf("expected verified mfa factor in refresh command, got %+v", repository.refreshCommand)
	}
	if secrets.verifySecret != secret || secrets.verifyCode != "123456" || !secrets.verifyNow.Equal(now) {
		t.Fatalf("unexpected mfa verify input: secret=%+v code=%s now=%s", secrets.verifySecret, secrets.verifyCode, secrets.verifyNow)
	}
}

func TestRefreshGatewayTokenUseCaseRecordsInvalidTOTPBeforeRotation(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	repository := &fakeIdentityRepository{
		activeMFAFactors: []types.MFAFactorSecret{{
			TenantID: "tenant-1",
			UserID:   "user-1",
			FactorID: "mfa-1",
			Type:     types.MFAFactorTypeTOTP,
			Status:   types.MFAFactorStatusActive,
			Secret:   types.EncryptedMFASecret{Ciphertext: "ciphertext", Nonce: "nonce", KeyVersion: "local-v1"},
		}},
	}
	useCase := NewRefreshGatewayTokenUseCase(
		repository,
		fakeTokenSigner{},
		fakeRefreshTokenCodec{},
		WithRefreshClock(func() time.Time { return now }),
		WithRefreshMFARiskPolicy(LoginRiskPolicy{MaxFailedAttempts: 3, FailureWindow: 20 * time.Minute, LockDuration: 10 * time.Minute}),
		WithRefreshMFASecretManager(&fakeMFASecretManager{verifyOK: false}),
	)
	_, err := useCase.Execute(context.Background(), types.RefreshGatewayTokenCommand{
		TenantID:     "tenant-1",
		UserID:       "user-1",
		DeviceID:     "device-1",
		RefreshToken: "rft_old.secret-old",
		MFAFactorID:  "mfa-1",
		MFACode:      "123456",
	})
	if !errors.Is(err, types.ErrInvalidMFA) {
		t.Fatalf("expected invalid mfa, got %v", err)
	}
	if !repository.validateRefreshCalled {
		t.Fatal("invalid mfa proof must only be evaluated after refresh token preflight")
	}
	if !repository.mfaFailureRecorded || repository.mfaFailureFactorID != "mfa-1" || repository.mfaFailureAt != now || repository.mfaLockUntil != now.Add(10*time.Minute) || repository.mfaMaxFailedAttempts != 3 || repository.mfaFailureWindowStart != now.Add(-20*time.Minute) {
		t.Fatalf("unexpected mfa failure record: recorded=%v factor=%s at=%s lock=%s max=%d window=%s",
			repository.mfaFailureRecorded,
			repository.mfaFailureFactorID,
			repository.mfaFailureAt,
			repository.mfaLockUntil,
			repository.mfaMaxFailedAttempts,
			repository.mfaFailureWindowStart,
		)
	}
	if repository.refreshCalled {
		t.Fatal("invalid mfa proof must not rotate refresh token")
	}
}

func TestRefreshGatewayTokenUseCaseRejectsInvalidRefreshBeforeTOTPFailure(t *testing.T) {
	repository := &fakeIdentityRepository{
		validateRefreshErr: types.NewInvalidRefreshToken("invalid refresh token"),
		activeMFAFactors: []types.MFAFactorSecret{{
			TenantID: "tenant-1",
			UserID:   "user-1",
			FactorID: "mfa-1",
			Type:     types.MFAFactorTypeTOTP,
			Status:   types.MFAFactorStatusActive,
			Secret:   types.EncryptedMFASecret{Ciphertext: "ciphertext", Nonce: "nonce", KeyVersion: "local-v1"},
		}},
	}
	secrets := &fakeMFASecretManager{verifyOK: false}
	useCase := NewRefreshGatewayTokenUseCase(
		repository,
		fakeTokenSigner{},
		fakeRefreshTokenCodec{},
		WithRefreshMFASecretManager(secrets),
	)
	_, err := useCase.Execute(context.Background(), types.RefreshGatewayTokenCommand{
		TenantID:     "tenant-1",
		UserID:       "user-1",
		DeviceID:     "device-1",
		RefreshToken: "rft_old.secret-old",
		MFAFactorID:  "mfa-1",
		MFACode:      "123456",
	})
	if !errors.Is(err, types.ErrInvalidRefreshToken) {
		t.Fatalf("expected invalid refresh token, got %v", err)
	}
	if !repository.validateRefreshCalled {
		t.Fatal("expected refresh token preflight")
	}
	if secrets.verifyCode != "" || repository.mfaFailureRecorded || repository.refreshCalled {
		t.Fatal("invalid refresh token must not verify mfa, record mfa failure, or rotate refresh token")
	}
}

func TestRefreshGatewayTokenUseCaseAcceptsRecoveryCodeProofForStepUp(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	repository := &fakeIdentityRepository{
		credential: types.UserCredential{
			TenantID: "tenant-1",
			UserID:   "user-1",
			Status:   "ACTIVE",
		},
		activeMFAFactors: []types.MFAFactorSecret{{
			TenantID: "tenant-1",
			UserID:   "user-1",
			FactorID: "mfa-1",
			Type:     types.MFAFactorTypeTOTP,
			Status:   types.MFAFactorStatusActive,
			Secret:   types.EncryptedMFASecret{Ciphertext: "ciphertext", Nonce: "nonce", KeyVersion: "local-v1"},
		}},
		recoveryCodeRecord: types.MFARecoveryCodeRecord{CodeID: "recovery-1", CodeHash: "hash-1"},
	}
	useCase := NewRefreshGatewayTokenUseCase(
		repository,
		fakeTokenSigner{},
		fakeRefreshTokenCodec{},
		WithRefreshClock(func() time.Time { return now }),
		WithRefreshMFARecoveryCodeManager(&fakeRecoveryCodeManager{hash: "hash-1"}),
	)
	_, err := useCase.Execute(context.Background(), types.RefreshGatewayTokenCommand{
		TenantID:        "tenant-1",
		UserID:          "user-1",
		DeviceID:        "device-1",
		RefreshToken:    "rft_old.secret-old",
		MFARecoveryCode: "aaaa-bbbb-cccc-dddd",
	})
	if err != nil {
		t.Fatalf("refresh with recovery code proof: %v", err)
	}
	if !repository.validateRefreshCalled {
		t.Fatal("expected refresh token preflight before recovery proof")
	}
	if !repository.refreshCalled || repository.recoveryCodeHash != "hash-1" || repository.refreshCommand.UsedMFARecoveryCode.CodeID != "recovery-1" {
		t.Fatalf("expected recovery proof to reach refresh transaction, hash=%q command=%+v refreshCalled=%v", repository.recoveryCodeHash, repository.refreshCommand, repository.refreshCalled)
	}
	if repository.refreshCommand.VerifiedMFAFactorID != "" || repository.mfaFailureRecorded {
		t.Fatal("recovery proof must not mark a TOTP factor verified or record TOTP failure")
	}
}

func TestRefreshGatewayTokenUseCaseRecordsInvalidRecoveryCodeBeforeRotation(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	repository := &fakeIdentityRepository{
		credential: types.UserCredential{
			TenantID: "tenant-1",
			UserID:   "user-1",
			Status:   "ACTIVE",
		},
		activeMFAFactors: []types.MFAFactorSecret{{
			TenantID: "tenant-1",
			UserID:   "user-1",
			FactorID: "mfa-1",
			Type:     types.MFAFactorTypeTOTP,
			Status:   types.MFAFactorStatusActive,
			Secret:   types.EncryptedMFASecret{Ciphertext: "ciphertext", Nonce: "nonce", KeyVersion: "local-v1"},
		}},
		findRecoveryCodeErr: types.NewInvalidMFA("invalid recovery code"),
	}
	useCase := NewRefreshGatewayTokenUseCase(
		repository,
		fakeTokenSigner{},
		fakeRefreshTokenCodec{},
		WithRefreshClock(func() time.Time { return now }),
		WithRefreshMFARecoveryRiskPolicy(LoginRiskPolicy{MaxFailedAttempts: 3, FailureWindow: 20 * time.Minute, LockDuration: 10 * time.Minute}),
		WithRefreshMFARecoveryCodeManager(&fakeRecoveryCodeManager{hash: "hash-missing"}),
	)
	_, err := useCase.Execute(context.Background(), types.RefreshGatewayTokenCommand{
		TenantID:        "tenant-1",
		UserID:          "user-1",
		DeviceID:        "device-1",
		RefreshToken:    "rft_old.secret-old",
		MFARecoveryCode: "wrong-code",
	})
	if !errors.Is(err, types.ErrInvalidMFA) {
		t.Fatalf("expected invalid mfa, got %v", err)
	}
	if !repository.validateRefreshCalled {
		t.Fatal("invalid recovery proof must only be evaluated after refresh token preflight")
	}
	if !repository.mfaRecoveryFailureRecorded || repository.mfaRecoveryFailureAt != now || repository.mfaRecoveryLockUntil != now.Add(10*time.Minute) || repository.mfaRecoveryMaxFailedAttempts != 3 || repository.mfaRecoveryFailureWindowStart != now.Add(-20*time.Minute) {
		t.Fatalf("unexpected recovery failure record: recorded=%v at=%s lock=%s max=%d window=%s",
			repository.mfaRecoveryFailureRecorded,
			repository.mfaRecoveryFailureAt,
			repository.mfaRecoveryLockUntil,
			repository.mfaRecoveryMaxFailedAttempts,
			repository.mfaRecoveryFailureWindowStart,
		)
	}
	if repository.refreshCalled {
		t.Fatal("invalid recovery proof must not rotate refresh token")
	}
}

func TestRefreshGatewayTokenUseCaseRejectsInvalidRefreshBeforeRecoveryFailure(t *testing.T) {
	repository := &fakeIdentityRepository{
		validateRefreshErr: types.NewInvalidRefreshToken("invalid refresh token"),
		credential: types.UserCredential{
			TenantID: "tenant-1",
			UserID:   "user-1",
			Status:   "ACTIVE",
		},
		activeMFAFactors: []types.MFAFactorSecret{{
			TenantID: "tenant-1",
			UserID:   "user-1",
			FactorID: "mfa-1",
			Type:     types.MFAFactorTypeTOTP,
			Status:   types.MFAFactorStatusActive,
			Secret:   types.EncryptedMFASecret{Ciphertext: "ciphertext", Nonce: "nonce", KeyVersion: "local-v1"},
		}},
		findRecoveryCodeErr: types.NewInvalidMFA("invalid recovery code"),
	}
	recovery := &fakeRecoveryCodeManager{hash: "hash-missing"}
	useCase := NewRefreshGatewayTokenUseCase(
		repository,
		fakeTokenSigner{},
		fakeRefreshTokenCodec{},
		WithRefreshMFARecoveryCodeManager(recovery),
	)
	_, err := useCase.Execute(context.Background(), types.RefreshGatewayTokenCommand{
		TenantID:        "tenant-1",
		UserID:          "user-1",
		DeviceID:        "device-1",
		RefreshToken:    "rft_old.secret-old",
		MFARecoveryCode: "wrong-code",
	})
	if !errors.Is(err, types.ErrInvalidRefreshToken) {
		t.Fatalf("expected invalid refresh token, got %v", err)
	}
	if !repository.validateRefreshCalled {
		t.Fatal("expected refresh token preflight")
	}
	if recovery.hashCalled || repository.recoveryCodeHash != "" || repository.mfaRecoveryFailureRecorded || repository.refreshCalled {
		t.Fatal("invalid refresh token must not hash/lookup recovery code, record recovery failure, or rotate refresh token")
	}
}
