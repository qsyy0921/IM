package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestLoginUseCaseSkipsMFAWhenNoActiveFactorExists(t *testing.T) {
	repository := &fakeIdentityRepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "ACTIVE",
			PasswordHash: "expected-hash",
		},
	}
	useCase := NewLoginUseCase(
		repository,
		fakeTokenSigner{},
		&fakePasswordVerifier{ok: true},
		fakeRefreshTokenCodec{},
	)
	result, err := useCase.Execute(context.Background(), types.LoginCommand{
		TenantID: "tenant-1",
		UserID:   "user-1",
		Password: "correct horse battery staple",
		DeviceID: "device-1",
	})
	if err != nil {
		t.Fatalf("login without active mfa: %v", err)
	}
	if !repository.loginCalled || result.GatewayToken != "gateway-token" || result.RefreshToken != "rft_new.secret-new" {
		t.Fatalf("expected legacy login path to succeed, result=%+v loginCalled=%v", result, repository.loginCalled)
	}
}

func TestLoginUseCaseRequiresMFAWhenActiveFactorExists(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	repository := &fakeIdentityRepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "ACTIVE",
			PasswordHash: "expected-hash",
		},
		activeMFAFactors: []types.MFAFactorSecret{{
			TenantID: "tenant-1",
			UserID:   "user-1",
			FactorID: "mfa-1",
			Type:     types.MFAFactorTypeTOTP,
			Status:   types.MFAFactorStatusActive,
			Secret:   types.EncryptedMFASecret{Ciphertext: "ciphertext", Nonce: "nonce", KeyVersion: "local-v1"},
		}},
	}
	useCase := NewLoginUseCase(
		repository,
		fakeTokenSigner{},
		&fakePasswordVerifier{ok: true},
		fakeRefreshTokenCodec{},
		WithLoginClock(func() time.Time { return now }),
		WithLoginMFASecretManager(&fakeMFASecretManager{verifyOK: true}),
	)
	_, err := useCase.Execute(context.Background(), types.LoginCommand{
		TenantID: "tenant-1",
		UserID:   "user-1",
		Password: "correct horse battery staple",
		DeviceID: "device-1",
	})
	if !errors.Is(err, types.ErrMFARequired) {
		t.Fatalf("expected mfa required, got %v", err)
	}
	if repository.loginCalled || repository.refreshCalled {
		t.Fatal("login session and refresh token must not be written before MFA succeeds")
	}
}

func TestLoginUseCaseReturnsMFAUnavailableWhenSecretManagerMissing(t *testing.T) {
	repository := &fakeIdentityRepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "ACTIVE",
			PasswordHash: "expected-hash",
		},
		activeMFAFactors: []types.MFAFactorSecret{{
			TenantID: "tenant-1",
			UserID:   "user-1",
			FactorID: "mfa-1",
			Type:     types.MFAFactorTypeTOTP,
			Status:   types.MFAFactorStatusActive,
			Secret:   types.EncryptedMFASecret{Ciphertext: "ciphertext", Nonce: "nonce", KeyVersion: "local-v1"},
		}},
	}
	useCase := NewLoginUseCase(
		repository,
		fakeTokenSigner{},
		&fakePasswordVerifier{ok: true},
		fakeRefreshTokenCodec{},
	)
	_, err := useCase.Execute(context.Background(), types.LoginCommand{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		Password:    "correct horse battery staple",
		DeviceID:    "device-1",
		MFAFactorID: "mfa-1",
		MFACode:     "123456",
	})
	if !errors.Is(err, types.ErrMFAUnavailable) {
		t.Fatalf("expected mfa unavailable, got %v", err)
	}
	if repository.loginCalled || repository.mfaFailureRecorded {
		t.Fatal("unavailable mfa manager must not write login session or record invalid-code failure")
	}
}

func TestLoginUseCaseRejectsAmbiguousActiveMFAFactors(t *testing.T) {
	repository := &fakeIdentityRepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "ACTIVE",
			PasswordHash: "expected-hash",
		},
		activeMFAFactors: []types.MFAFactorSecret{
			{
				TenantID: "tenant-1",
				UserID:   "user-1",
				FactorID: "mfa-1",
				Type:     types.MFAFactorTypeTOTP,
				Status:   types.MFAFactorStatusActive,
				Secret:   types.EncryptedMFASecret{Ciphertext: "ciphertext-1", Nonce: "nonce-1", KeyVersion: "local-v1"},
			},
			{
				TenantID: "tenant-1",
				UserID:   "user-1",
				FactorID: "mfa-2",
				Type:     types.MFAFactorTypeTOTP,
				Status:   types.MFAFactorStatusActive,
				Secret:   types.EncryptedMFASecret{Ciphertext: "ciphertext-2", Nonce: "nonce-2", KeyVersion: "local-v1"},
			},
		},
	}
	secrets := &fakeMFASecretManager{verifyOK: true}
	useCase := NewLoginUseCase(
		repository,
		fakeTokenSigner{},
		&fakePasswordVerifier{ok: true},
		fakeRefreshTokenCodec{},
		WithLoginMFASecretManager(secrets),
	)
	_, err := useCase.Execute(context.Background(), types.LoginCommand{
		TenantID: "tenant-1",
		UserID:   "user-1",
		Password: "correct horse battery staple",
		DeviceID: "device-1",
		MFACode:  "123456",
	})
	if !errors.Is(err, types.ErrInvalidMFA) {
		t.Fatalf("expected ambiguous mfa factors to fail with invalid mfa, got %v", err)
	}
	if repository.loginCalled || secrets.verifyCode != "" {
		t.Fatal("ambiguous factor selection must not verify code or write login session")
	}
}

func TestLoginUseCaseVerifiesMFABeforeIssuingToken(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	secret := types.EncryptedMFASecret{Ciphertext: "ciphertext", Nonce: "nonce", KeyVersion: "local-v1"}
	repository := &fakeIdentityRepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "ACTIVE",
			PasswordHash: "expected-hash",
		},
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
	useCase := NewLoginUseCase(
		repository,
		fakeTokenSigner{},
		&fakePasswordVerifier{ok: true},
		fakeRefreshTokenCodec{},
		WithLoginClock(func() time.Time { return now }),
		WithLoginMFASecretManager(secrets),
	)
	result, err := useCase.Execute(context.Background(), types.LoginCommand{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		Password:    "correct horse battery staple",
		DeviceID:    "device-1",
		MFAFactorID: "mfa-1",
		MFACode:     "123456",
	})
	if err != nil {
		t.Fatalf("login with mfa: %v", err)
	}
	if !repository.loginCalled || result.GatewayToken != "gateway-token" || result.RefreshToken != "rft_new.secret-new" {
		t.Fatalf("expected login token after mfa, result=%+v loginCalled=%v", result, repository.loginCalled)
	}
	if repository.loginCommand.VerifiedMFAFactorID != "mfa-1" {
		t.Fatalf("expected verified mfa factor to be passed to login transaction, got %+v", repository.loginCommand)
	}
	if secrets.verifySecret != secret || secrets.verifyCode != "123456" || !secrets.verifyNow.Equal(now) {
		t.Fatalf("unexpected mfa verify input: secret=%+v code=%s now=%s", secrets.verifySecret, secrets.verifyCode, secrets.verifyNow)
	}
}

func TestLoginUseCaseAcceptsRecoveryCodeBeforeIssuingToken(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	repository := &fakeIdentityRepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "ACTIVE",
			PasswordHash: "expected-hash",
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
	secrets := &fakeMFASecretManager{verifyOK: true}
	useCase := NewLoginUseCase(
		repository,
		fakeTokenSigner{},
		&fakePasswordVerifier{ok: true},
		fakeRefreshTokenCodec{},
		WithLoginClock(func() time.Time { return now }),
		WithLoginMFASecretManager(secrets),
		WithLoginMFARecoveryCodeManager(&fakeRecoveryCodeManager{hash: "hash-1"}),
	)
	result, err := useCase.Execute(context.Background(), types.LoginCommand{
		TenantID:        "tenant-1",
		UserID:          "user-1",
		Password:        "correct horse battery staple",
		DeviceID:        "device-1",
		MFARecoveryCode: "aaaa-bbbb-cccc-dddd",
	})
	if err != nil {
		t.Fatalf("login with recovery code: %v", err)
	}
	if !repository.loginCalled || result.GatewayToken != "gateway-token" || result.RefreshToken != "rft_new.secret-new" {
		t.Fatalf("expected login token after recovery code, result=%+v loginCalled=%v", result, repository.loginCalled)
	}
	if repository.recoveryCodeHash != "hash-1" || repository.loginCommand.UsedMFARecoveryCode.CodeID != "recovery-1" {
		t.Fatalf("expected recovery code to be looked up and passed to login transaction, hash=%q command=%+v", repository.recoveryCodeHash, repository.loginCommand)
	}
	if secrets.verifyCode != "" || repository.mfaFailureRecorded || repository.loginCommand.VerifiedMFAFactorID != "" {
		t.Fatal("recovery code login must not verify TOTP, record TOTP failure, or mark a TOTP factor as verified")
	}
}

func TestLoginUseCaseRejectsInvalidRecoveryCodeBeforeSessionWrite(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	repository := &fakeIdentityRepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "ACTIVE",
			PasswordHash: "expected-hash",
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
	useCase := NewLoginUseCase(
		repository,
		fakeTokenSigner{},
		&fakePasswordVerifier{ok: true},
		fakeRefreshTokenCodec{},
		WithLoginClock(func() time.Time { return now }),
		WithLoginMFARecoveryRiskPolicy(LoginRiskPolicy{MaxFailedAttempts: 3, FailureWindow: 20 * time.Minute, LockDuration: 10 * time.Minute}),
		WithLoginMFARecoveryCodeManager(&fakeRecoveryCodeManager{hash: "hash-missing"}),
	)
	_, err := useCase.Execute(context.Background(), types.LoginCommand{
		TenantID:        "tenant-1",
		UserID:          "user-1",
		Password:        "correct horse battery staple",
		DeviceID:        "device-1",
		MFARecoveryCode: "wrong-code",
	})
	if !errors.Is(err, types.ErrInvalidMFA) {
		t.Fatalf("expected invalid mfa, got %v", err)
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
	if repository.loginCalled {
		t.Fatal("invalid recovery code must not write login session")
	}
}

func TestLoginUseCaseReturnsMFALockedWhenRecoveryCodeFailuresReachThreshold(t *testing.T) {
	repository := &fakeIdentityRepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "ACTIVE",
			PasswordHash: "expected-hash",
		},
		activeMFAFactors: []types.MFAFactorSecret{{
			TenantID: "tenant-1",
			UserID:   "user-1",
			FactorID: "mfa-1",
			Type:     types.MFAFactorTypeTOTP,
			Status:   types.MFAFactorStatusActive,
			Secret:   types.EncryptedMFASecret{Ciphertext: "ciphertext", Nonce: "nonce", KeyVersion: "local-v1"},
		}},
		findRecoveryCodeErr:         types.NewInvalidMFA("invalid recovery code"),
		recordMFARecoveryFailureErr: types.NewMFALocked("mfa temporarily locked"),
	}
	useCase := NewLoginUseCase(
		repository,
		fakeTokenSigner{},
		&fakePasswordVerifier{ok: true},
		fakeRefreshTokenCodec{},
		WithLoginMFARecoveryCodeManager(&fakeRecoveryCodeManager{hash: "hash-missing"}),
	)
	_, err := useCase.Execute(context.Background(), types.LoginCommand{
		TenantID:        "tenant-1",
		UserID:          "user-1",
		Password:        "correct horse battery staple",
		DeviceID:        "device-1",
		MFARecoveryCode: "wrong-code",
	})
	if !errors.Is(err, types.ErrMFALocked) {
		t.Fatalf("expected mfa locked, got %v", err)
	}
	if repository.loginCalled {
		t.Fatal("locked recovery-code risk must not write login session")
	}
}

func TestLoginUseCaseRejectsLockedRecoveryCodeBeforeLookup(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	repository := &fakeIdentityRepository{
		credential: types.UserCredential{
			TenantID:               "tenant-1",
			UserID:                 "user-1",
			Status:                 "ACTIVE",
			PasswordHash:           "expected-hash",
			MFARecoveryLockedUntil: now.Add(time.Minute),
			MFARecoveryFailedCount: 5,
		},
		activeMFAFactors: []types.MFAFactorSecret{{
			TenantID: "tenant-1",
			UserID:   "user-1",
			FactorID: "mfa-1",
			Type:     types.MFAFactorTypeTOTP,
			Status:   types.MFAFactorStatusActive,
			Secret:   types.EncryptedMFASecret{Ciphertext: "ciphertext", Nonce: "nonce", KeyVersion: "local-v1"},
		}},
	}
	recovery := &fakeRecoveryCodeManager{hash: "hash-1"}
	useCase := NewLoginUseCase(
		repository,
		fakeTokenSigner{},
		&fakePasswordVerifier{ok: true},
		fakeRefreshTokenCodec{},
		WithLoginClock(func() time.Time { return now }),
		WithLoginMFARecoveryCodeManager(recovery),
	)
	_, err := useCase.Execute(context.Background(), types.LoginCommand{
		TenantID:        "tenant-1",
		UserID:          "user-1",
		Password:        "correct horse battery staple",
		DeviceID:        "device-1",
		MFARecoveryCode: "aaaa-bbbb-cccc-dddd",
	})
	if !errors.Is(err, types.ErrMFALocked) {
		t.Fatalf("expected mfa locked, got %v", err)
	}
	if recovery.hashCalled || repository.recoveryCodeHash != "" || repository.mfaRecoveryFailureRecorded || repository.loginCalled {
		t.Fatal("locked recovery-code risk must not hash, lookup, record another failure, or write login session")
	}
}

func TestLoginUseCaseRejectsInvalidMFA(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	repository := &fakeIdentityRepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "ACTIVE",
			PasswordHash: "expected-hash",
		},
		activeMFAFactors: []types.MFAFactorSecret{{
			TenantID: "tenant-1",
			UserID:   "user-1",
			FactorID: "mfa-1",
			Type:     types.MFAFactorTypeTOTP,
			Status:   types.MFAFactorStatusActive,
			Secret:   types.EncryptedMFASecret{Ciphertext: "ciphertext", Nonce: "nonce", KeyVersion: "local-v1"},
		}},
	}
	useCase := NewLoginUseCase(
		repository,
		fakeTokenSigner{},
		&fakePasswordVerifier{ok: true},
		fakeRefreshTokenCodec{},
		WithLoginClock(func() time.Time { return now }),
		WithLoginMFARiskPolicy(LoginRiskPolicy{MaxFailedAttempts: 3, FailureWindow: 20 * time.Minute, LockDuration: 10 * time.Minute}),
		WithLoginMFASecretManager(&fakeMFASecretManager{verifyOK: false}),
	)
	_, err := useCase.Execute(context.Background(), types.LoginCommand{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		Password:    "correct horse battery staple",
		DeviceID:    "device-1",
		MFAFactorID: "mfa-1",
		MFACode:     "123456",
	})
	if !errors.Is(err, types.ErrInvalidMFA) {
		t.Fatalf("expected invalid mfa, got %v", err)
	}
	if !repository.mfaFailureRecorded || repository.mfaFailureAt != now || repository.mfaLockUntil != now.Add(10*time.Minute) || repository.mfaMaxFailedAttempts != 3 || repository.mfaFailureWindowStart != now.Add(-20*time.Minute) {
		t.Fatalf("unexpected mfa failure record: recorded=%v factor=%s at=%s lock=%s max=%d window=%s",
			repository.mfaFailureRecorded,
			repository.mfaFailureFactorID,
			repository.mfaFailureAt,
			repository.mfaLockUntil,
			repository.mfaMaxFailedAttempts,
			repository.mfaFailureWindowStart,
		)
	}
	if repository.mfaFailureFactorID != "mfa-1" {
		t.Fatalf("expected mfa failure to be recorded against selected factor, got %s", repository.mfaFailureFactorID)
	}
	if repository.loginCalled {
		t.Fatal("invalid mfa must not write login session")
	}
}

func TestLoginUseCaseReturnsMFALockedWhenFailureThresholdIsReached(t *testing.T) {
	repository := &fakeIdentityRepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "ACTIVE",
			PasswordHash: "expected-hash",
		},
		activeMFAFactors: []types.MFAFactorSecret{{
			TenantID: "tenant-1",
			UserID:   "user-1",
			FactorID: "mfa-1",
			Type:     types.MFAFactorTypeTOTP,
			Status:   types.MFAFactorStatusActive,
			Secret:   types.EncryptedMFASecret{Ciphertext: "ciphertext", Nonce: "nonce", KeyVersion: "local-v1"},
		}},
		recordMFAFailureErr: types.NewMFALocked("mfa temporarily locked"),
	}
	useCase := NewLoginUseCase(
		repository,
		fakeTokenSigner{},
		&fakePasswordVerifier{ok: true},
		fakeRefreshTokenCodec{},
		WithLoginMFASecretManager(&fakeMFASecretManager{verifyOK: false}),
	)
	_, err := useCase.Execute(context.Background(), types.LoginCommand{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		Password:    "correct horse battery staple",
		DeviceID:    "device-1",
		MFAFactorID: "mfa-1",
		MFACode:     "123456",
	})
	if !errors.Is(err, types.ErrMFALocked) {
		t.Fatalf("expected mfa locked, got %v", err)
	}
	if repository.loginCalled {
		t.Fatal("locked mfa factor must not write login session")
	}
}

func TestLoginUseCaseRejectsLockedMFAFactorBeforeVerify(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	repository := &fakeIdentityRepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "ACTIVE",
			PasswordHash: "expected-hash",
		},
		activeMFAFactors: []types.MFAFactorSecret{{
			TenantID:         "tenant-1",
			UserID:           "user-1",
			FactorID:         "mfa-1",
			Type:             types.MFAFactorTypeTOTP,
			Status:           types.MFAFactorStatusActive,
			Secret:           types.EncryptedMFASecret{Ciphertext: "ciphertext", Nonce: "nonce", KeyVersion: "local-v1"},
			LoginLockedUntil: now.Add(time.Minute),
			LoginFailedCount: 5,
		}},
	}
	secrets := &fakeMFASecretManager{verifyOK: true}
	useCase := NewLoginUseCase(
		repository,
		fakeTokenSigner{},
		&fakePasswordVerifier{ok: true},
		fakeRefreshTokenCodec{},
		WithLoginClock(func() time.Time { return now }),
		WithLoginMFASecretManager(secrets),
	)
	_, err := useCase.Execute(context.Background(), types.LoginCommand{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		Password:    "correct horse battery staple",
		DeviceID:    "device-1",
		MFAFactorID: "mfa-1",
		MFACode:     "123456",
	})
	if !errors.Is(err, types.ErrMFALocked) {
		t.Fatalf("expected mfa locked, got %v", err)
	}
	if secrets.verifyCode != "" || repository.mfaFailureRecorded || repository.loginCalled {
		t.Fatal("locked mfa factor must not verify code, record another failure, or write login session")
	}
}
