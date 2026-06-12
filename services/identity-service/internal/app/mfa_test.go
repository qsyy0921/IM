package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestBeginMFAEnrollmentUseCaseStoresEncryptedSecretAndReturnsPlainSecretOnce(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	repository := &fakeMFARepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "ACTIVE",
			PasswordHash: "expected-hash",
		},
	}
	secrets := &fakeMFASecretManager{
		plain: "PLAINSECRET",
		encrypted: types.EncryptedMFASecret{
			Ciphertext: "ciphertext",
			Nonce:      "nonce",
			KeyVersion: "local-v1",
		},
	}
	useCase := NewBeginMFAEnrollmentUseCase(repository, &fakePasswordVerifier{ok: true}, secrets)
	useCase.now = func() time.Time { return now }

	result, err := useCase.Execute(context.Background(), types.BeginMFAEnrollmentCommand{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		FactorType:  types.MFAFactorTypeTOTP,
		Password:    "correct horse battery staple",
		DisplayName: "Authenticator",
		Issuer:      "NexusIM Test",
	})
	if err != nil {
		t.Fatalf("begin mfa enrollment: %v", err)
	}
	if !repository.createCalled {
		t.Fatal("expected repository CreateMFAFactor to be called")
	}
	if repository.createdSecret.Ciphertext != "ciphertext" || repository.createdSecret.Ciphertext == result.Secret {
		t.Fatalf("repository must receive encrypted secret only, stored=%+v result=%+v", repository.createdSecret, result)
	}
	if result.Secret != "PLAINSECRET" || result.OTPAuthURI == "" || result.Status != types.MFAFactorStatusPending {
		t.Fatalf("unexpected mfa enrollment result: %+v", result)
	}
	if repository.createdAt != now {
		t.Fatalf("expected fixed clock to be used, got %s", repository.createdAt)
	}
}

func TestBeginMFAEnrollmentUseCaseRejectsWrongPasswordBeforeSecretCreation(t *testing.T) {
	repository := &fakeMFARepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "ACTIVE",
			PasswordHash: "expected-hash",
		},
	}
	secrets := &fakeMFASecretManager{}
	useCase := NewBeginMFAEnrollmentUseCase(repository, &fakePasswordVerifier{ok: false}, secrets)

	_, err := useCase.Execute(context.Background(), types.BeginMFAEnrollmentCommand{
		TenantID:   "tenant-1",
		UserID:     "user-1",
		FactorType: types.MFAFactorTypeTOTP,
		Password:   "wrong",
	})
	if !errors.Is(err, types.ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
	if secrets.newCalled || repository.createCalled {
		t.Fatal("wrong password must not create a TOTP secret or repository row")
	}
}

func TestBeginMFAEnrollmentUseCaseReturnsMFAUnavailableWhenSecretManagerFails(t *testing.T) {
	repository := &fakeMFARepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "ACTIVE",
			PasswordHash: "expected-hash",
		},
	}
	secrets := &fakeMFASecretManager{newErr: types.NewMFAUnavailable("mfa secret encryption key is required")}
	useCase := NewBeginMFAEnrollmentUseCase(repository, &fakePasswordVerifier{ok: true}, secrets)

	_, err := useCase.Execute(context.Background(), types.BeginMFAEnrollmentCommand{
		TenantID:   "tenant-1",
		UserID:     "user-1",
		FactorType: types.MFAFactorTypeTOTP,
		Password:   "correct horse battery staple",
	})
	if !errors.Is(err, types.ErrMFAUnavailable) {
		t.Fatalf("expected mfa unavailable, got %v", err)
	}
	if repository.createCalled {
		t.Fatal("mfa factor row must not be created when secret manager is unavailable")
	}
}

func TestBeginMFAEnrollmentUseCaseReturnsMFAUnavailableWhenSecretManagerMissing(t *testing.T) {
	repository := &fakeMFARepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "ACTIVE",
			PasswordHash: "expected-hash",
		},
	}
	useCase := NewBeginMFAEnrollmentUseCase(repository, &fakePasswordVerifier{ok: true}, nil)

	_, err := useCase.Execute(context.Background(), types.BeginMFAEnrollmentCommand{
		TenantID:   "tenant-1",
		UserID:     "user-1",
		FactorType: types.MFAFactorTypeTOTP,
		Password:   "correct horse battery staple",
	})
	if !errors.Is(err, types.ErrMFAUnavailable) {
		t.Fatalf("expected mfa unavailable, got %v", err)
	}
	if repository.createCalled {
		t.Fatal("mfa factor row must not be created when secret manager is missing")
	}
}

func TestConfirmMFAEnrollmentUseCaseVerifiesPendingTOTP(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	secret := types.EncryptedMFASecret{Ciphertext: "ciphertext", Nonce: "nonce", KeyVersion: "local-v1"}
	repository := &fakeMFARepository{
		factorSecret: types.MFAFactorSecret{
			TenantID: "tenant-1",
			UserID:   "user-1",
			FactorID: "mfa-1",
			Type:     types.MFAFactorTypeTOTP,
			Status:   types.MFAFactorStatusPending,
			Secret:   secret,
		},
	}
	secrets := &fakeMFASecretManager{verifyOK: true}
	useCase := NewConfirmMFAEnrollmentUseCase(repository, secrets)
	useCase.now = func() time.Time { return now }

	result, err := useCase.Execute(context.Background(), types.ConfirmMFAEnrollmentCommand{
		TenantID: "tenant-1",
		UserID:   "user-1",
		FactorID: "mfa-1",
		Code:     "123456",
	})
	if err != nil {
		t.Fatalf("confirm mfa enrollment: %v", err)
	}
	if !repository.confirmCalled || result.Status != types.MFAFactorStatusActive {
		t.Fatalf("expected active mfa factor, result=%+v called=%v", result, repository.confirmCalled)
	}
	if secrets.verifySecret != secret || secrets.verifyCode != "123456" || !secrets.verifyNow.Equal(now) {
		t.Fatalf("unexpected verify input: secret=%+v code=%s now=%s", secrets.verifySecret, secrets.verifyCode, secrets.verifyNow)
	}
}

func TestConfirmMFAEnrollmentUseCaseRejectsInvalidCodeBeforeActivation(t *testing.T) {
	repository := &fakeMFARepository{
		factorSecret: types.MFAFactorSecret{
			TenantID: "tenant-1",
			UserID:   "user-1",
			FactorID: "mfa-1",
			Type:     types.MFAFactorTypeTOTP,
			Status:   types.MFAFactorStatusPending,
			Secret:   types.EncryptedMFASecret{Ciphertext: "ciphertext", Nonce: "nonce", KeyVersion: "local-v1"},
		},
	}
	useCase := NewConfirmMFAEnrollmentUseCase(repository, &fakeMFASecretManager{verifyOK: false})

	_, err := useCase.Execute(context.Background(), types.ConfirmMFAEnrollmentCommand{
		TenantID: "tenant-1",
		UserID:   "user-1",
		FactorID: "mfa-1",
		Code:     "123456",
	})
	if !errors.Is(err, types.ErrInvalidMFA) {
		t.Fatalf("expected invalid mfa, got %v", err)
	}
	if repository.confirmCalled {
		t.Fatal("invalid mfa code must not activate the factor")
	}
}

func TestConfirmMFAEnrollmentUseCaseReturnsMFAUnavailableWhenSecretManagerFails(t *testing.T) {
	repository := &fakeMFARepository{
		factorSecret: types.MFAFactorSecret{
			TenantID: "tenant-1",
			UserID:   "user-1",
			FactorID: "mfa-1",
			Type:     types.MFAFactorTypeTOTP,
			Status:   types.MFAFactorStatusPending,
			Secret:   types.EncryptedMFASecret{Ciphertext: "ciphertext", Nonce: "nonce", KeyVersion: "local-v1"},
		},
	}
	useCase := NewConfirmMFAEnrollmentUseCase(repository, &fakeMFASecretManager{verifyErr: types.NewMFAUnavailable("mfa secret encryption key is required")})

	_, err := useCase.Execute(context.Background(), types.ConfirmMFAEnrollmentCommand{
		TenantID: "tenant-1",
		UserID:   "user-1",
		FactorID: "mfa-1",
		Code:     "123456",
	})
	if !errors.Is(err, types.ErrMFAUnavailable) {
		t.Fatalf("expected mfa unavailable, got %v", err)
	}
	if repository.confirmCalled {
		t.Fatal("mfa factor must not be activated when secret manager is unavailable")
	}
}

func TestConfirmMFAEnrollmentUseCaseReturnsMFAUnavailableWhenSecretManagerMissing(t *testing.T) {
	useCase := NewConfirmMFAEnrollmentUseCase(&fakeMFARepository{}, nil)

	_, err := useCase.Execute(context.Background(), types.ConfirmMFAEnrollmentCommand{
		TenantID: "tenant-1",
		UserID:   "user-1",
		FactorID: "mfa-1",
		Code:     "123456",
	})
	if !errors.Is(err, types.ErrMFAUnavailable) {
		t.Fatalf("expected mfa unavailable, got %v", err)
	}
}

func TestDisableMFAFactorUseCaseRequiresCurrentPassword(t *testing.T) {
	repository := &fakeMFARepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "ACTIVE",
			PasswordHash: "expected-hash",
		},
	}
	useCase := NewDisableMFAFactorUseCase(repository, &fakePasswordVerifier{ok: true})
	result, err := useCase.Execute(context.Background(), types.DisableMFAFactorCommand{
		TenantID: "tenant-1",
		UserID:   "user-1",
		FactorID: "mfa-1",
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("disable mfa factor: %v", err)
	}
	if !repository.disableCalled || result.Status != types.MFAFactorStatusDisabled {
		t.Fatalf("expected disabled factor, result=%+v called=%v", result, repository.disableCalled)
	}
}

type fakeMFARepository struct {
	credential    types.UserCredential
	factorSecret  types.MFAFactorSecret
	createCalled  bool
	confirmCalled bool
	disableCalled bool
	createdSecret types.EncryptedMFASecret
	createdAt     time.Time
}

func (repo *fakeMFARepository) GetUserCredential(context.Context, types.TenantID, types.UserID) (types.UserCredential, error) {
	return repo.credential, nil
}

func (repo *fakeMFARepository) CreateMFAFactor(_ context.Context, command types.BeginMFAEnrollmentCommand, secret types.EncryptedMFASecret, createdAt time.Time) (types.BeginMFAEnrollmentResult, error) {
	repo.createCalled = true
	repo.createdSecret = secret
	repo.createdAt = createdAt
	return types.BeginMFAEnrollmentResult{
		TenantID:        command.TenantID,
		UserID:          command.UserID,
		FactorID:        "mfa-1",
		FactorType:      types.MFAFactorTypeTOTP,
		Status:          types.MFAFactorStatusPending,
		CreatedAtUnixMS: createdAt.UnixMilli(),
	}, nil
}

func (repo *fakeMFARepository) GetMFAFactorSecret(context.Context, types.TenantID, types.UserID, types.MFAFactorID) (types.MFAFactorSecret, error) {
	return repo.factorSecret, nil
}

func (repo *fakeMFARepository) ConfirmMFAFactor(_ context.Context, command types.ConfirmMFAEnrollmentCommand, verifiedAt time.Time) (types.ConfirmMFAEnrollmentResult, error) {
	repo.confirmCalled = true
	return types.ConfirmMFAEnrollmentResult{
		TenantID:         command.TenantID,
		UserID:           command.UserID,
		FactorID:         command.FactorID,
		Status:           types.MFAFactorStatusActive,
		VerifiedAtUnixMS: verifiedAt.UnixMilli(),
	}, nil
}

func (repo *fakeMFARepository) DisableMFAFactor(_ context.Context, command types.DisableMFAFactorCommand, disabledAt time.Time) (types.DisableMFAFactorResult, error) {
	repo.disableCalled = true
	return types.DisableMFAFactorResult{
		TenantID:         command.TenantID,
		UserID:           command.UserID,
		FactorID:         command.FactorID,
		Status:           types.MFAFactorStatusDisabled,
		DisabledAtUnixMS: disabledAt.UnixMilli(),
	}, nil
}

type fakeMFASecretManager struct {
	plain        string
	encrypted    types.EncryptedMFASecret
	verifyOK     bool
	newErr       error
	verifyErr    error
	newCalled    bool
	verifySecret types.EncryptedMFASecret
	verifyCode   string
	verifyNow    time.Time
}

func (manager *fakeMFASecretManager) NewTOTPSecret() (string, types.EncryptedMFASecret, error) {
	manager.newCalled = true
	if manager.newErr != nil {
		return "", types.EncryptedMFASecret{}, manager.newErr
	}
	return manager.plain, manager.encrypted, nil
}

func (manager *fakeMFASecretManager) VerifyTOTP(encrypted types.EncryptedMFASecret, code string, now time.Time) (bool, error) {
	manager.verifySecret = encrypted
	manager.verifyCode = code
	manager.verifyNow = now
	if manager.verifyErr != nil {
		return false, manager.verifyErr
	}
	return manager.verifyOK, nil
}

func (manager *fakeMFASecretManager) OTPAuthURI(issuer string, accountName string, secret string) string {
	return "otpauth://" + issuer + "/" + accountName + "?secret=" + secret
}
