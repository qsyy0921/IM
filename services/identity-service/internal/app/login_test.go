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

func TestLoginUseCaseRunsDummyPasswordVerifyForMissingCredential(t *testing.T) {
	repository := &fakeIdentityRepository{
		getCredentialErr: types.NewInvalidCredentials("invalid credentials"),
	}
	verifier := &fakePasswordVerifier{ok: false}
	useCase := NewLoginUseCase(
		repository,
		fakeTokenSigner{},
		verifier,
		fakeRefreshTokenCodec{},
		WithLoginDummyPasswordHash("dummy-password-hash"),
	)
	_, err := useCase.Execute(context.Background(), types.LoginCommand{
		TenantID: "tenant-1",
		UserID:   "missing-user",
		Password: "wrong",
		DeviceID: "device-1",
	})
	if !errors.Is(err, types.ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
	if verifier.calls != 1 || verifier.lastPassword != "wrong" || verifier.lastHash != "dummy-password-hash" {
		t.Fatalf("expected dummy password verification, calls=%d password=%q hash=%q", verifier.calls, verifier.lastPassword, verifier.lastHash)
	}
	if repository.failureRecorded || repository.loginCalled {
		t.Fatal("missing credential must not write failure counters or session state")
	}
}

func TestLoginUseCaseVerifiesPasswordBeforeRejectingInactiveUser(t *testing.T) {
	repository := &fakeIdentityRepository{
		credential: types.UserCredential{
			TenantID:     "tenant-1",
			UserID:       "user-1",
			Status:       "DISABLED",
			PasswordHash: "expected-hash",
		},
	}
	verifier := &fakePasswordVerifier{ok: true}
	useCase := NewLoginUseCase(
		repository,
		fakeTokenSigner{},
		verifier,
		fakeRefreshTokenCodec{},
	)
	_, err := useCase.Execute(context.Background(), types.LoginCommand{
		TenantID: "tenant-1",
		UserID:   "user-1",
		Password: "correct horse battery staple",
		DeviceID: "device-1",
	})
	if !errors.Is(err, types.ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
	if verifier.calls != 1 || verifier.lastHash != "expected-hash" {
		t.Fatalf("expected inactive user path to still verify password, calls=%d hash=%q", verifier.calls, verifier.lastHash)
	}
	if !repository.failureRecorded || repository.loginCalled {
		t.Fatalf("inactive user should record the failed login attempt and not write session, failureRecorded=%v loginCalled=%v", repository.failureRecorded, repository.loginCalled)
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

type fakeIdentityRepository struct {
	credential                          types.UserCredential
	getCredentialErr                    error
	registerCalled                      bool
	registerPasswordHash                string
	loginCalled                         bool
	loginCommand                        types.LoginCommand
	failureRecorded                     bool
	failureAt                           time.Time
	lockUntil                           time.Time
	maxFailedAttempts                   int
	failureWindowStart                  time.Time
	recordFailureErr                    error
	mfaFailureRecorded                  bool
	mfaFailureAt                        time.Time
	mfaLockUntil                        time.Time
	mfaMaxFailedAttempts                int
	mfaFailureWindowStart               time.Time
	mfaFailureFactorID                  types.MFAFactorID
	recordMFAFailureErr                 error
	mfaRecoveryFailureRecorded          bool
	mfaRecoveryFailureAt                time.Time
	mfaRecoveryLockUntil                time.Time
	mfaRecoveryMaxFailedAttempts        int
	mfaRecoveryFailureWindowStart       time.Time
	recordMFARecoveryFailureErr         error
	recoveryCodeRecord                  types.MFARecoveryCodeRecord
	recoveryCodeHash                    string
	findRecoveryCodeErr                 error
	validateRefreshCalled               bool
	validateRefreshErr                  error
	validatePresentedTokenID            types.RefreshTokenID
	validatePresentedTokenHash          string
	refreshCalled                       bool
	refreshCommand                      types.RefreshGatewayTokenCommand
	presentedTokenID                    types.RefreshTokenID
	presentedTokenHash                  string
	recordPasswordResetRequestCalled    bool
	recordPasswordResetRequestTargetKey string
	recordPasswordResetRequestErr       error
	createVerificationCalled            bool
	createVerificationDelivery          types.ChallengeDeliveryRecord
	createPasswordResetCalled           bool
	createPasswordResetDelivery         types.ChallengeDeliveryRecord
	createPasswordResetErr              error
	expireChallengeCalled               bool
	expiredChallengeID                  types.ChallengeID
	expireChallengeErr                  error
	deliverySuccessCalled               bool
	deliverySuccessChallengeID          types.ChallengeID
	deliverySuccessErr                  error
	deliveryFailureCalled               bool
	deliveryFailureChallengeID          types.ChallengeID
	deliveryFailureLastError            string
	deliveryFailureErr                  error
	confirmPasswordResetCalled          bool
	resetPasswordHash                   string
	resetTokenHash                      string
	activeMFAFactors                    []types.MFAFactorSecret
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
	if repo.getCredentialErr != nil {
		return types.UserCredential{}, repo.getCredentialErr
	}
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

func (repo *fakeIdentityRepository) LoginGatewaySession(_ context.Context, command types.LoginCommand, _ types.RefreshTokenRecord, _ time.Time, _ time.Time, _ time.Time) (types.LoginResult, error) {
	repo.loginCalled = true
	repo.loginCommand = command
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

func (repo *fakeIdentityRepository) ListActiveMFAFactorSecrets(context.Context, types.TenantID, types.UserID) ([]types.MFAFactorSecret, error) {
	return repo.activeMFAFactors, nil
}

func (repo *fakeIdentityRepository) RecordMFALoginFailure(_ context.Context, _ types.TenantID, _ types.UserID, factorID types.MFAFactorID, failedAt time.Time, lockUntil time.Time, maxFailedAttempts int, failureWindowStart time.Time) error {
	repo.mfaFailureRecorded = true
	repo.mfaFailureFactorID = factorID
	repo.mfaFailureAt = failedAt
	repo.mfaLockUntil = lockUntil
	repo.mfaMaxFailedAttempts = maxFailedAttempts
	repo.mfaFailureWindowStart = failureWindowStart
	return repo.recordMFAFailureErr
}

func (repo *fakeIdentityRepository) RecordMFARecoveryLoginFailure(_ context.Context, _ types.TenantID, _ types.UserID, failedAt time.Time, lockUntil time.Time, maxFailedAttempts int, failureWindowStart time.Time) error {
	repo.mfaRecoveryFailureRecorded = true
	repo.mfaRecoveryFailureAt = failedAt
	repo.mfaRecoveryLockUntil = lockUntil
	repo.mfaRecoveryMaxFailedAttempts = maxFailedAttempts
	repo.mfaRecoveryFailureWindowStart = failureWindowStart
	return repo.recordMFARecoveryFailureErr
}

func (repo *fakeIdentityRepository) FindActiveMFARecoveryCode(_ context.Context, _ types.TenantID, _ types.UserID, codeHash string) (types.MFARecoveryCodeRecord, error) {
	repo.recoveryCodeHash = codeHash
	if repo.findRecoveryCodeErr != nil {
		return types.MFARecoveryCodeRecord{}, repo.findRecoveryCodeErr
	}
	return repo.recoveryCodeRecord, nil
}

func (repo *fakeIdentityRepository) ValidateRefreshGatewaySession(_ context.Context, _ types.RefreshGatewayTokenCommand, tokenID types.RefreshTokenID, tokenHash string, _ time.Time) error {
	repo.validateRefreshCalled = true
	repo.validatePresentedTokenID = tokenID
	repo.validatePresentedTokenHash = tokenHash
	return repo.validateRefreshErr
}

func (repo *fakeIdentityRepository) RefreshGatewaySession(_ context.Context, command types.RefreshGatewayTokenCommand, tokenID types.RefreshTokenID, tokenHash string, _ types.RefreshTokenRecord, _ time.Time, _ time.Time, _ time.Time) (types.RefreshGatewayTokenResult, error) {
	repo.refreshCalled = true
	repo.refreshCommand = command
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

func (repo *fakeIdentityRepository) CreateVerificationChallenge(_ context.Context, _ types.RequestVerificationChallengeCommand, _ types.ChallengeType, _ types.ChallengeRecord, delivery types.ChallengeDeliveryRecord, _ time.Time, _ time.Time) (types.RequestVerificationChallengeResult, error) {
	repo.createVerificationCalled = true
	repo.createVerificationDelivery = delivery
	return types.RequestVerificationChallengeResult{TenantID: "tenant-1", UserID: "user-1", ChallengeID: "challenge-1", Channel: types.VerificationChannelEmail, Destination: "user1@example.com"}, nil
}

func (repo *fakeIdentityRepository) ConfirmVerificationChallenge(context.Context, types.ConfirmVerificationChallengeCommand, string, time.Time) (types.ConfirmVerificationChallengeResult, error) {
	return types.ConfirmVerificationChallengeResult{}, nil
}

func (repo *fakeIdentityRepository) ExpireChallenge(_ context.Context, _ types.TenantID, _ types.UserID, challengeID types.ChallengeID, _ time.Time) error {
	repo.expireChallengeCalled = true
	repo.expiredChallengeID = challengeID
	return repo.expireChallengeErr
}

func (repo *fakeIdentityRepository) RecordChallengeDeliverySuccess(_ context.Context, _ types.TenantID, _ types.UserID, challengeID types.ChallengeID, _ time.Time) error {
	repo.deliverySuccessCalled = true
	repo.deliverySuccessChallengeID = challengeID
	return repo.deliverySuccessErr
}

func (repo *fakeIdentityRepository) RecordChallengeDeliveryFailure(_ context.Context, _ types.TenantID, _ types.UserID, challengeID types.ChallengeID, lastError string, _ time.Time) error {
	repo.deliveryFailureCalled = true
	repo.deliveryFailureChallengeID = challengeID
	repo.deliveryFailureLastError = lastError
	return repo.deliveryFailureErr
}

func (repo *fakeIdentityRepository) RecordPasswordResetRequest(_ context.Context, _ types.TenantID, _ types.UserID, _ types.VerificationChannel, targetKey string, _ time.Time) error {
	repo.recordPasswordResetRequestCalled = true
	repo.recordPasswordResetRequestTargetKey = targetKey
	return repo.recordPasswordResetRequestErr
}

func (repo *fakeIdentityRepository) CreatePasswordResetChallenge(_ context.Context, command types.RequestPasswordResetCommand, record types.ChallengeRecord, delivery types.ChallengeDeliveryRecord, _ time.Time, expiresAt time.Time) (types.RequestPasswordResetResult, error) {
	repo.createPasswordResetCalled = true
	repo.createPasswordResetDelivery = delivery
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
	ok           bool
	calls        int
	lastPassword string
	lastHash     string
}

func (verifier *fakePasswordVerifier) VerifyPassword(password string, passwordHash string) bool {
	verifier.calls++
	verifier.lastPassword = password
	verifier.lastHash = passwordHash
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

type fakeChallengeDeliveryTokenCodec struct {
	token     string
	encrypted types.EncryptedChallengeToken
	err       error
}

func (codec *fakeChallengeDeliveryTokenCodec) SealChallengeToken(token string) (types.EncryptedChallengeToken, error) {
	codec.token = token
	if codec.err != nil {
		return types.EncryptedChallengeToken{}, codec.err
	}
	return codec.encrypted, nil
}

type fakeChallengeNotifier struct {
	called       bool
	notification types.ChallengeNotification
	err          error
}

func (notifier *fakeChallengeNotifier) SendChallenge(_ context.Context, notification types.ChallengeNotification) error {
	notifier.called = true
	notifier.notification = notification
	return notifier.err
}

type fakeRecoveryCodeManager struct {
	hash       string
	err        error
	hashCalled bool
}

func (manager *fakeRecoveryCodeManager) NewRecoveryCodes(int) ([]types.MFARecoveryCode, error) {
	return nil, nil
}

func (manager *fakeRecoveryCodeManager) HashRecoveryCode(string) (string, error) {
	manager.hashCalled = true
	if manager.err != nil {
		return "", manager.err
	}
	return manager.hash, nil
}
