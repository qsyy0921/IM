package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestRegisterUserUseCaseHashesPasswordBeforeWrite(t *testing.T) {
	repository := &fakeIdentityRepository{}
	useCase := NewRegisterUserUseCase(repository, fakePasswordHasher{hash: "hashed-password"})
	result, err := useCase.Execute(context.Background(), types.RegisterUserCommand{
		TenantID: "tenant-1",
		UserID:   "user-1",
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("register user: %v", err)
	}
	if !repository.registerCalled {
		t.Fatal("expected repository register to be called")
	}
	if repository.registerPasswordHash != "hashed-password" {
		t.Fatalf("expected hashed password to be stored, got %q", repository.registerPasswordHash)
	}
	if result.Status != types.UserStatusActive {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRegisterUserUseCaseRejectsShortPasswordBeforeHash(t *testing.T) {
	repository := &fakeIdentityRepository{}
	hasher := fakePasswordHasher{hash: "hashed-password"}
	useCase := NewRegisterUserUseCase(repository, hasher)
	_, err := useCase.Execute(context.Background(), types.RegisterUserCommand{
		TenantID: "tenant-1",
		UserID:   "user-1",
		Password: "short",
	})
	if err == nil {
		t.Fatal("expected short password to fail")
	}
	if repository.registerCalled {
		t.Fatal("register should not be written after validation failure")
	}
}

func TestLoginUseCaseRecordsInvalidPasswordBeforeSessionWrite(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	repository := &fakeIdentityRepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "ACTIVE",
			PasswordHash: "expected-hash",
		},
	}
	verifier := &fakePasswordVerifier{ok: false}
	useCase := NewLoginUseCase(
		repository,
		fakeTokenSigner{},
		verifier,
		fakeRefreshTokenCodec{},
		WithLoginClock(func() time.Time { return now }),
		WithLoginRiskPolicy(LoginRiskPolicy{MaxFailedAttempts: 3, FailureWindow: 20 * time.Minute, LockDuration: 10 * time.Minute}),
	)
	_, err := useCase.Execute(context.Background(), types.LoginCommand{
		TenantID: "tenant-1",
		UserID:   "user-1",
		Password: "wrong",
		DeviceID: "device-1",
	})
	if err == nil {
		t.Fatal("expected invalid password to fail")
	}
	if !repository.failureRecorded {
		t.Fatal("expected login failure to be recorded")
	}
	if repository.failureAt != now || repository.lockUntil != now.Add(10*time.Minute) || repository.maxFailedAttempts != 3 || repository.failureWindowStart != now.Add(-20*time.Minute) {
		t.Fatalf("unexpected failure record: at=%s lock=%s max=%d window=%s", repository.failureAt, repository.lockUntil, repository.maxFailedAttempts, repository.failureWindowStart)
	}
	if repository.loginCalled {
		t.Fatal("login session should not be written after invalid password")
	}
	if verifier.calls != 1 {
		t.Fatalf("expected password verifier to be called once, got %d", verifier.calls)
	}
}

func TestLoginUseCaseRejectsLockedAccountBeforePasswordVerify(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	repository := &fakeIdentityRepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "ACTIVE",
			PasswordHash: "expected-hash",
			LockedUntil:  now.Add(time.Minute),
		},
	}
	verifier := &fakePasswordVerifier{ok: true}
	useCase := NewLoginUseCase(
		repository,
		fakeTokenSigner{},
		verifier,
		fakeRefreshTokenCodec{},
		WithLoginClock(func() time.Time { return now }),
	)
	_, err := useCase.Execute(context.Background(), types.LoginCommand{
		TenantID: "tenant-1",
		UserID:   "user-1",
		Password: "correct horse battery staple",
		DeviceID: "device-1",
	})
	if !errors.Is(err, types.ErrAccountLocked) {
		t.Fatalf("expected account locked, got %v", err)
	}
	if verifier.calls != 0 {
		t.Fatalf("expected password verifier not to be called, got %d", verifier.calls)
	}
	if repository.failureRecorded || repository.loginCalled {
		t.Fatalf("locked account should not record another failure or write session")
	}
}

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

func TestRequestVerificationChallengeUseCaseRequiresCurrentPassword(t *testing.T) {
	repository := &fakeIdentityRepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "ACTIVE",
			PasswordHash: "expected-hash",
		},
	}
	verifier := &fakePasswordVerifier{ok: false}
	useCase := NewRequestVerificationChallengeUseCase(repository, fakeChallengeTokenCodec{}, verifier, ChallengeOptions{ReturnDevToken: true})
	_, err := useCase.Execute(context.Background(), types.RequestVerificationChallengeCommand{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
		Password:    "wrong",
	})
	if !errors.Is(err, types.ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
	if repository.createVerificationCalled {
		t.Fatal("verification challenge should not be created after invalid password")
	}

	verifier.ok = true
	result, err := useCase.Execute(context.Background(), types.RequestVerificationChallengeCommand{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
		Password:    "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("request verification: %v", err)
	}
	if !repository.createVerificationCalled || result.DevChallengeToken != "challenge-token" {
		t.Fatalf("expected challenge creation with dev token, result=%+v called=%v", result, repository.createVerificationCalled)
	}
}

func TestConfirmPasswordResetUseCaseHashesNewPassword(t *testing.T) {
	repository := &fakeIdentityRepository{}
	useCase := NewConfirmPasswordResetUseCase(repository, fakeChallengeTokenCodec{}, fakePasswordHasher{hash: "new-password-hash"})
	_, err := useCase.Execute(context.Background(), types.ConfirmPasswordResetCommand{
		TenantID:       "tenant-1",
		UserID:         "user-1",
		ChallengeID:    "challenge-1",
		ChallengeToken: "challenge-token",
		NewPassword:    "new correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("confirm password reset: %v", err)
	}
	if !repository.confirmPasswordResetCalled || repository.resetPasswordHash != "new-password-hash" || repository.resetTokenHash == "" {
		t.Fatalf("expected hashed password reset, called=%v password=%q token=%q", repository.confirmPasswordResetCalled, repository.resetPasswordHash, repository.resetTokenHash)
	}
}

func TestRequestPasswordResetUseCaseNeverReturnsDevToken(t *testing.T) {
	repository := &fakeIdentityRepository{}
	useCase := NewRequestPasswordResetUseCase(repository, fakeChallengeTokenCodec{}, ChallengeOptions{ReturnDevToken: true})
	result, err := useCase.Execute(context.Background(), types.RequestPasswordResetCommand{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
	})
	if err != nil {
		t.Fatalf("request password reset: %v", err)
	}
	if !repository.createPasswordResetCalled {
		t.Fatal("expected repository to create reset challenge")
	}
	if result.ChallengeID != "challenge-1" || result.DevChallengeToken != "" {
		t.Fatalf("password reset response must not expose dev token: %+v", result)
	}
}

func TestRequestPasswordResetUseCaseHidesInvalidOrRateLimitedTarget(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "invalid credentials", err: types.NewInvalidCredentials("invalid credentials")},
		{name: "rate limited", err: types.NewChallengeRateLimited("too many active challenges")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repository := &fakeIdentityRepository{createPasswordResetErr: tc.err}
			useCase := NewRequestPasswordResetUseCase(repository, fakeChallengeTokenCodec{}, ChallengeOptions{ReturnDevToken: true})
			useCase.now = func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }
			result, err := useCase.Execute(context.Background(), types.RequestPasswordResetCommand{
				TenantID:    "tenant-1",
				UserID:      "user-1",
				Channel:     types.VerificationChannelEmail,
				Destination: "user1@example.com",
				TTLSeconds:  600,
			})
			if err != nil {
				t.Fatalf("request password reset should return neutral success: %v", err)
			}
			if !repository.createPasswordResetCalled {
				t.Fatal("expected repository to be asked to create reset challenge")
			}
			if result.ChallengeID != "challenge-1" || result.DevChallengeToken != "" || result.ExpiresAtUnixMS != time.Unix(1_800_000_600, 0).UnixMilli() {
				t.Fatalf("unexpected neutral result: %+v", result)
			}
		})
	}
}

type fakeIdentityRepository struct {
	credential                 types.UserCredential
	registerCalled             bool
	registerPasswordHash       string
	loginCalled                bool
	failureRecorded            bool
	failureAt                  time.Time
	lockUntil                  time.Time
	maxFailedAttempts          int
	failureWindowStart         time.Time
	recordFailureErr           error
	refreshCalled              bool
	presentedTokenID           types.RefreshTokenID
	presentedTokenHash         string
	createVerificationCalled   bool
	createPasswordResetCalled  bool
	createPasswordResetErr     error
	confirmPasswordResetCalled bool
	resetPasswordHash          string
	resetTokenHash             string
}

func (repo *fakeIdentityRepository) RegisterUser(_ context.Context, command types.RegisterUserCommand, passwordHash string, createdAt time.Time) (types.RegisterUserResult, error) {
	repo.registerCalled = true
	repo.registerPasswordHash = passwordHash
	return types.RegisterUserResult{
		TenantID:        command.TenantID,
		UserID:          command.UserID,
		Status:          types.UserStatusActive,
		CreatedAtUnixMS: createdAt.UnixMilli(),
	}, nil
}

func (repo *fakeIdentityRepository) GetUserCredential(context.Context, types.TenantID, types.UserID) (types.UserCredential, error) {
	return repo.credential, nil
}

func (repo *fakeIdentityRepository) RecordLoginFailure(_ context.Context, _ types.TenantID, _ types.UserID, failedAt time.Time, lockUntil time.Time, maxFailedAttempts int, failureWindowStart time.Time) error {
	repo.failureRecorded = true
	repo.failureAt = failedAt
	repo.lockUntil = lockUntil
	repo.maxFailedAttempts = maxFailedAttempts
	repo.failureWindowStart = failureWindowStart
	return repo.recordFailureErr
}

func (repo *fakeIdentityRepository) LoginGatewaySession(context.Context, types.LoginCommand, types.RefreshTokenRecord, time.Time, time.Time, time.Time) (types.LoginResult, error) {
	repo.loginCalled = true
	return types.LoginResult{
		TenantID:               "tenant-1",
		UserID:                 "user-1",
		DeviceID:               "device-1",
		SessionID:              "session-1",
		Audience:               "push-gateway",
		GatewayExpiresAtUnixMS: time.Unix(1_800_000_900, 0).UnixMilli(),
		RefreshExpiresAtUnixMS: time.Unix(1_802_592_000, 0).UnixMilli(),
		IssuedAtUnixMS:         time.Unix(1_800_000_000, 0).UnixMilli(),
	}, nil
}

func (repo *fakeIdentityRepository) RefreshGatewaySession(_ context.Context, _ types.RefreshGatewayTokenCommand, tokenID types.RefreshTokenID, tokenHash string, _ types.RefreshTokenRecord, _ time.Time, _ time.Time, _ time.Time) (types.RefreshGatewayTokenResult, error) {
	repo.refreshCalled = true
	repo.presentedTokenID = tokenID
	repo.presentedTokenHash = tokenHash
	return types.RefreshGatewayTokenResult{
		TenantID:               "tenant-1",
		UserID:                 "user-1",
		DeviceID:               "device-1",
		SessionID:              "session-1",
		Audience:               "push-gateway",
		GatewayExpiresAtUnixMS: time.Unix(1_800_000_900, 0).UnixMilli(),
		RefreshExpiresAtUnixMS: time.Unix(1_802_592_000, 0).UnixMilli(),
		IssuedAtUnixMS:         time.Unix(1_800_000_000, 0).UnixMilli(),
	}, nil
}

func (repo *fakeIdentityRepository) CreateVerificationChallenge(context.Context, types.RequestVerificationChallengeCommand, types.ChallengeType, types.ChallengeRecord, time.Time, time.Time) (types.RequestVerificationChallengeResult, error) {
	repo.createVerificationCalled = true
	return types.RequestVerificationChallengeResult{TenantID: "tenant-1", UserID: "user-1", ChallengeID: "challenge-1", Channel: types.VerificationChannelEmail, Destination: "user1@example.com"}, nil
}

func (repo *fakeIdentityRepository) ConfirmVerificationChallenge(context.Context, types.ConfirmVerificationChallengeCommand, string, time.Time) (types.ConfirmVerificationChallengeResult, error) {
	return types.ConfirmVerificationChallengeResult{}, nil
}

func (repo *fakeIdentityRepository) CreatePasswordResetChallenge(_ context.Context, command types.RequestPasswordResetCommand, record types.ChallengeRecord, _ time.Time, expiresAt time.Time) (types.RequestPasswordResetResult, error) {
	repo.createPasswordResetCalled = true
	if repo.createPasswordResetErr != nil {
		return types.RequestPasswordResetResult{}, repo.createPasswordResetErr
	}
	return types.RequestPasswordResetResult{
		TenantID:        command.TenantID,
		UserID:          command.UserID,
		ChallengeID:     record.ChallengeID,
		Channel:         command.Channel,
		Destination:     command.Destination,
		ExpiresAtUnixMS: expiresAt.UnixMilli(),
	}, nil
}

func (repo *fakeIdentityRepository) ConfirmPasswordReset(_ context.Context, _ types.ConfirmPasswordResetCommand, tokenHash string, passwordHash string, _ time.Time) (types.ConfirmPasswordResetResult, error) {
	repo.confirmPasswordResetCalled = true
	repo.resetTokenHash = tokenHash
	repo.resetPasswordHash = passwordHash
	return types.ConfirmPasswordResetResult{TenantID: "tenant-1", UserID: "user-1"}, nil
}

func (repo *fakeIdentityRepository) IssueGatewaySession(context.Context, types.IssueGatewayTokenCommand, time.Time, time.Time) (types.IssueGatewayTokenResult, error) {
	return types.IssueGatewayTokenResult{}, nil
}

func (repo *fakeIdentityRepository) RevokeDevice(context.Context, types.RevokeDeviceCommand, time.Time) (types.RevokeDeviceResult, error) {
	return types.RevokeDeviceResult{}, nil
}

func (repo *fakeIdentityRepository) RevokeSession(context.Context, types.RevokeSessionCommand, time.Time) (types.RevokeSessionResult, error) {
	return types.RevokeSessionResult{}, nil
}

func (repo *fakeIdentityRepository) GetDeviceState(context.Context, types.GetDeviceStateCommand) (types.GetDeviceStateResult, error) {
	return types.GetDeviceStateResult{}, nil
}

type fakeTokenSigner struct{}

func (fakeTokenSigner) SignGatewayToken(types.TokenClaims) (string, error) {
	return "gateway-token", nil
}

type fakePasswordVerifier struct {
	ok    bool
	calls int
}

func (verifier *fakePasswordVerifier) VerifyPassword(string, string) bool {
	verifier.calls++
	return verifier.ok
}

type fakePasswordHasher struct{ hash string }

func (hasher fakePasswordHasher) HashPassword(string) (string, error) {
	return hasher.hash, nil
}

type fakeRefreshTokenCodec struct{}

func (fakeRefreshTokenCodec) NewRefreshToken() (string, types.RefreshTokenRecord, error) {
	return "rft_new.secret-new", types.RefreshTokenRecord{TokenID: "rft_new", TokenHash: "hash-new"}, nil
}

func (fakeRefreshTokenCodec) ParseRefreshToken(token string) (types.ParsedRefreshToken, error) {
	if token != "rft_old.secret-old" {
		return types.ParsedRefreshToken{}, types.NewInvalidRefreshToken("invalid refresh token")
	}
	return types.ParsedRefreshToken{TokenID: "rft_old", Secret: "secret-old"}, nil
}

func (fakeRefreshTokenCodec) HashRefreshTokenSecret(secret string) string {
	return "hash-" + secret
}

type fakeChallengeTokenCodec struct{}

func (fakeChallengeTokenCodec) NewChallengeToken() (string, types.ChallengeRecord, error) {
	return "challenge-token", types.ChallengeRecord{ChallengeID: "challenge-1", TokenHash: "challenge-hash"}, nil
}

func (fakeChallengeTokenCodec) HashChallengeToken(token string) string {
	return "hash-" + token
}
