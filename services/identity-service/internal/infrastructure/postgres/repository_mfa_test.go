package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestRepositoryMFAFactorLifecycleIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(pool, WithMFAFactorIDGenerator(func() (string, error) { return "mfa-factor-1", nil }))
	now := time.Unix(1_800_000_000, 0).UTC()
	if _, err := repository.RegisterUser(ctx, types.RegisterUserCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
	}, "password-hash", now); err != nil {
		t.Fatalf("register user: %v", err)
	}

	secret := types.EncryptedMFASecret{
		Ciphertext: "encrypted-secret",
		Nonce:      "nonce-value",
		KeyVersion: "v2",
	}
	beginResult, err := repository.CreateMFAFactor(ctx, types.BeginMFAEnrollmentCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		FactorType:  types.MFAFactorTypeTOTP,
		DisplayName: "Authenticator",
		TraceID:     "trace-mfa",
		RequestID:   "request-mfa",
	}, secret, now)
	if err != nil {
		t.Fatalf("create mfa factor: %v", err)
	}
	if beginResult.FactorID != "mfa-factor-1" || beginResult.Status != types.MFAFactorStatusPending {
		t.Fatalf("unexpected begin mfa result: %+v", beginResult)
	}
	stored := readMFASecret(t, ctx, pool, "mfa-factor-1")
	if stored.Secret != secret || stored.Secret.Ciphertext == "PLAINSECRET" || stored.Status != types.MFAFactorStatusPending {
		t.Fatalf("unexpected stored mfa secret: %+v", stored)
	}

	loaded, err := repository.GetMFAFactorSecret(ctx, "tenant-identity", "user-1", "mfa-factor-1")
	if err != nil {
		t.Fatalf("get mfa factor secret: %v", err)
	}
	if loaded.Secret != secret || loaded.Status != types.MFAFactorStatusPending {
		t.Fatalf("unexpected loaded mfa secret: %+v", loaded)
	}

	confirmedAt := now.Add(time.Minute)
	confirmResult, err := repository.ConfirmMFAFactor(ctx, types.ConfirmMFAEnrollmentCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		FactorID: "mfa-factor-1",
	}, []types.MFARecoveryCodeRecord{
		{CodeID: "recovery-1", CodeHash: "recovery-hash-1"},
		{CodeID: "recovery-2", CodeHash: "recovery-hash-2"},
	}, confirmedAt)
	if err != nil {
		t.Fatalf("confirm mfa factor: %v", err)
	}
	if confirmResult.Status != types.MFAFactorStatusActive || confirmResult.VerifiedAtUnixMS != confirmedAt.UnixMilli() {
		t.Fatalf("unexpected confirm result: %+v", confirmResult)
	}
	activeFactors, err := repository.ListActiveMFAFactorSecrets(ctx, "tenant-identity", "user-1")
	if err != nil {
		t.Fatalf("list active mfa factors: %v", err)
	}
	if len(activeFactors) != 1 || activeFactors[0].FactorID != "mfa-factor-1" || activeFactors[0].Secret != secret {
		t.Fatalf("unexpected active factors after confirm: %+v", activeFactors)
	}
	recovery, err := repository.FindActiveMFARecoveryCode(ctx, "tenant-identity", "user-1", "recovery-hash-1")
	if err != nil {
		t.Fatalf("find recovery code: %v", err)
	}
	if recovery.CodeID != "recovery-1" || recovery.CodeHash != "recovery-hash-1" {
		t.Fatalf("unexpected recovery code record: %+v", recovery)
	}
	_, err = repository.ConfirmMFAFactor(ctx, types.ConfirmMFAEnrollmentCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		FactorID: "mfa-factor-1",
	}, []types.MFARecoveryCodeRecord{{CodeID: "recovery-replay", CodeHash: "recovery-hash-replay"}}, confirmedAt.Add(time.Minute))
	if !errors.Is(err, types.ErrMFAFactorNotFound) {
		t.Fatalf("expected replay confirm to fail as not pending, got %v", err)
	}

	disabledAt := now.Add(2 * time.Minute)
	disableResult, err := repository.DisableMFAFactor(ctx, types.DisableMFAFactorCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		FactorID: "mfa-factor-1",
	}, disabledAt)
	if err != nil {
		t.Fatalf("disable mfa factor: %v", err)
	}
	if disableResult.Status != types.MFAFactorStatusDisabled || disableResult.DisabledAtUnixMS != disabledAt.UnixMilli() {
		t.Fatalf("unexpected disable result: %+v", disableResult)
	}
	activeFactors, err = repository.ListActiveMFAFactorSecrets(ctx, "tenant-identity", "user-1")
	if err != nil {
		t.Fatalf("list active mfa factors after disable: %v", err)
	}
	if len(activeFactors) != 0 {
		t.Fatalf("expected no active factors after disable, got %+v", activeFactors)
	}
	_, err = repository.DisableMFAFactor(ctx, types.DisableMFAFactorCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		FactorID: "mfa-factor-1",
	}, disabledAt.Add(time.Minute))
	if !errors.Is(err, types.ErrMFAFactorNotFound) {
		t.Fatalf("expected disabled factor not found for repeat disable, got %v", err)
	}
}

func TestRepositoryMFALoginFailureLocksAndSuccessClearsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(
		pool,
		WithMFAFactorIDGenerator(func() (string, error) { return "mfa-factor-login", nil }),
		WithSessionIDGenerator(func() (string, error) { return "session-mfa-login", nil }),
	)
	now := time.Unix(1_800_000_000, 0).UTC()
	if _, err := repository.RegisterUser(ctx, types.RegisterUserCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
	}, "password-hash", now); err != nil {
		t.Fatalf("register user: %v", err)
	}
	secret := types.EncryptedMFASecret{Ciphertext: "encrypted-secret", Nonce: "nonce-value", KeyVersion: "local-v1"}
	if _, err := repository.CreateMFAFactor(ctx, types.BeginMFAEnrollmentCommand{
		TenantID:   "tenant-identity",
		UserID:     "user-1",
		FactorType: types.MFAFactorTypeTOTP,
	}, secret, now); err != nil {
		t.Fatalf("create mfa factor: %v", err)
	}
	if _, err := repository.ConfirmMFAFactor(ctx, types.ConfirmMFAEnrollmentCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		FactorID: "mfa-factor-login",
	}, []types.MFARecoveryCodeRecord{{CodeID: "recovery-login", CodeHash: "recovery-login-hash"}}, now.Add(time.Minute)); err != nil {
		t.Fatalf("confirm mfa factor: %v", err)
	}

	firstFailureAt := now.Add(2 * time.Minute)
	lockUntil := firstFailureAt.Add(15 * time.Minute)
	if err := repository.RecordMFALoginFailure(ctx, "tenant-identity", "user-1", "mfa-factor-login", firstFailureAt, lockUntil, 2, firstFailureAt.Add(-15*time.Minute)); err != nil {
		t.Fatalf("record first mfa failure: %v", err)
	}
	assertMFALoginRisk(t, ctx, pool, "mfa-factor-login", 1, false, false)

	err := repository.RecordMFALoginFailure(ctx, "tenant-identity", "user-1", "mfa-factor-login", firstFailureAt.Add(time.Minute), lockUntil.Add(time.Minute), 2, firstFailureAt.Add(-14*time.Minute))
	if !errors.Is(err, types.ErrMFALocked) {
		t.Fatalf("expected mfa locked on threshold, got %v", err)
	}
	assertMFALoginRisk(t, ctx, pool, "mfa-factor-login", 2, true, false)

	activeFactors, err := repository.ListActiveMFAFactorSecrets(ctx, "tenant-identity", "user-1")
	if err != nil {
		t.Fatalf("list active mfa factors after lock: %v", err)
	}
	if len(activeFactors) != 1 || activeFactors[0].LoginFailedCount != 2 || !activeFactors[0].LoginLockedUntil.After(firstFailureAt) {
		t.Fatalf("expected active factor to carry login risk state, got %+v", activeFactors)
	}

	loginAt := lockUntil.Add(2 * time.Minute)
	_, err = repository.LoginGatewaySession(ctx, types.LoginCommand{
		TenantID:            "tenant-identity",
		UserID:              "user-1",
		DeviceID:            "device-1",
		Audience:            "push-gateway",
		VerifiedMFAFactorID: "mfa-factor-login",
	}, types.RefreshTokenRecord{
		TokenID:   "rft_mfa_login",
		TokenHash: "hash-mfa-login",
	}, loginAt, loginAt.Add(15*time.Minute), loginAt.Add(30*24*time.Hour))
	if err != nil {
		t.Fatalf("login after mfa lock window: %v", err)
	}
	assertMFALoginRisk(t, ctx, pool, "mfa-factor-login", 0, false, true)
}

func TestRepositoryMFALoginRejectsLockedFactorBeforeSessionWriteIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(
		pool,
		WithMFAFactorIDGenerator(func() (string, error) { return "mfa-factor-login-locked", nil }),
		WithSessionIDGenerator(func() (string, error) { return "session-mfa-login-locked", nil }),
	)
	now := time.Unix(1_800_000_000, 0).UTC()
	if _, err := repository.RegisterUser(ctx, types.RegisterUserCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
	}, "password-hash", now); err != nil {
		t.Fatalf("register user: %v", err)
	}
	if _, err := repository.CreateMFAFactor(ctx, types.BeginMFAEnrollmentCommand{
		TenantID:   "tenant-identity",
		UserID:     "user-1",
		FactorType: types.MFAFactorTypeTOTP,
	}, types.EncryptedMFASecret{Ciphertext: "encrypted-secret", Nonce: "nonce-value", KeyVersion: "local-v1"}, now); err != nil {
		t.Fatalf("create mfa factor: %v", err)
	}
	if _, err := repository.ConfirmMFAFactor(ctx, types.ConfirmMFAEnrollmentCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		FactorID: "mfa-factor-login-locked",
	}, []types.MFARecoveryCodeRecord{{CodeID: "recovery-login-totp-locked", CodeHash: "recovery-login-totp-locked-hash"}}, now.Add(time.Minute)); err != nil {
		t.Fatalf("confirm mfa factor: %v", err)
	}
	lockAt := now.Add(2 * time.Minute)
	err := repository.RecordMFALoginFailure(ctx, "tenant-identity", "user-1", "mfa-factor-login-locked", lockAt, lockAt.Add(15*time.Minute), 1, lockAt.Add(-15*time.Minute))
	if !errors.Is(err, types.ErrMFALocked) {
		t.Fatalf("expected totp factor lock, got %v", err)
	}
	assertMFALoginRisk(t, ctx, pool, "mfa-factor-login-locked", 1, true, false)

	_, err = repository.LoginGatewaySession(ctx, types.LoginCommand{
		TenantID:            "tenant-identity",
		UserID:              "user-1",
		DeviceID:            "device-1",
		Audience:            "push-gateway",
		VerifiedMFAFactorID: "mfa-factor-login-locked",
	}, types.RefreshTokenRecord{
		TokenID:   "rft_mfa_login_locked",
		TokenHash: "hash-mfa-login-locked",
	}, now.Add(3*time.Minute), now.Add(18*time.Minute), now.Add(30*24*time.Hour))
	if !errors.Is(err, types.ErrMFALocked) {
		t.Fatalf("expected locked totp login proof to fail, got %v", err)
	}
	assertRefreshTokenMissing(t, ctx, pool, "rft_mfa_login_locked")
	assertSessionMissing(t, ctx, pool, "session-mfa-login-locked")
	assertMFALoginRisk(t, ctx, pool, "mfa-factor-login-locked", 1, true, false)
}

func TestRepositoryMFARecoveryCodeLoginConsumesCodeIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(
		pool,
		WithMFAFactorIDGenerator(func() (string, error) { return "mfa-factor-recovery-login", nil }),
		WithSessionIDGenerator(func() (string, error) { return "session-recovery-login", nil }),
	)
	now := time.Unix(1_800_000_000, 0).UTC()
	if _, err := repository.RegisterUser(ctx, types.RegisterUserCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
	}, "password-hash", now); err != nil {
		t.Fatalf("register user: %v", err)
	}
	if _, err := repository.CreateMFAFactor(ctx, types.BeginMFAEnrollmentCommand{
		TenantID:   "tenant-identity",
		UserID:     "user-1",
		FactorType: types.MFAFactorTypeTOTP,
	}, types.EncryptedMFASecret{Ciphertext: "encrypted-secret", Nonce: "nonce-value", KeyVersion: "local-v1"}, now); err != nil {
		t.Fatalf("create mfa factor: %v", err)
	}
	if _, err := repository.ConfirmMFAFactor(ctx, types.ConfirmMFAEnrollmentCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		FactorID: "mfa-factor-recovery-login",
	}, []types.MFARecoveryCodeRecord{{CodeID: "recovery-login-1", CodeHash: "recovery-login-hash-1"}}, now.Add(time.Minute)); err != nil {
		t.Fatalf("confirm mfa factor: %v", err)
	}
	recovery, err := repository.FindActiveMFARecoveryCode(ctx, "tenant-identity", "user-1", "recovery-login-hash-1")
	if err != nil {
		t.Fatalf("find recovery code before login: %v", err)
	}
	loginAt := now.Add(2 * time.Minute)
	failureAt := loginAt.Add(-time.Minute)
	if err := repository.RecordMFARecoveryLoginFailure(ctx, "tenant-identity", "user-1", failureAt, failureAt.Add(15*time.Minute), 2, failureAt.Add(-15*time.Minute)); err != nil {
		t.Fatalf("record first recovery-code failure: %v", err)
	}
	assertMFARecoveryLoginRisk(t, ctx, pool, 1, false)
	if _, err := repository.LoginGatewaySession(ctx, types.LoginCommand{
		TenantID:            "tenant-identity",
		UserID:              "user-1",
		DeviceID:            "device-1",
		Audience:            "push-gateway",
		UsedMFARecoveryCode: recovery,
	}, types.RefreshTokenRecord{
		TokenID:   "rft_recovery_login",
		TokenHash: "hash-recovery-login",
	}, loginAt, loginAt.Add(15*time.Minute), loginAt.Add(30*24*time.Hour)); err != nil {
		t.Fatalf("login with recovery code: %v", err)
	}
	if _, err := repository.FindActiveMFARecoveryCode(ctx, "tenant-identity", "user-1", "recovery-login-hash-1"); !errors.Is(err, types.ErrInvalidMFA) {
		t.Fatalf("expected consumed recovery code to be inactive, got %v", err)
	}
	assertMFARecoveryLoginRisk(t, ctx, pool, 0, false)
	assertSessionMFAProof(t, ctx, pool, "session-recovery-login", "RECOVERY_CODE", "")
	refreshAt := loginAt.Add(30 * time.Second)
	if _, err := repository.RefreshGatewaySession(ctx, types.RefreshGatewayTokenCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		DeviceID: "device-1",
		Audience: "push-gateway",
	}, "rft_recovery_login", "hash-recovery-login", types.RefreshTokenRecord{
		TokenID:   "rft_recovery_next",
		TokenHash: "hash-recovery-next",
	}, refreshAt, refreshAt.Add(15*time.Minute), refreshAt.Add(30*24*time.Hour)); err != nil {
		t.Fatalf("refresh recovery-code authenticated session: %v", err)
	}
	assertRefreshTokenStatus(t, ctx, pool, "rft_recovery_login", "USED")
	assertRefreshTokenStatus(t, ctx, pool, "rft_recovery_next", "ACTIVE")
	assertSessionMFAProof(t, ctx, pool, "session-recovery-login", "RECOVERY_CODE", "")
	if _, err := repository.LoginGatewaySession(ctx, types.LoginCommand{
		TenantID:            "tenant-identity",
		UserID:              "user-1",
		DeviceID:            "device-2",
		Audience:            "push-gateway",
		UsedMFARecoveryCode: recovery,
	}, types.RefreshTokenRecord{
		TokenID:   "rft_recovery_replay",
		TokenHash: "hash-recovery-replay",
	}, loginAt.Add(time.Minute), loginAt.Add(16*time.Minute), loginAt.Add(30*24*time.Hour)); !errors.Is(err, types.ErrInvalidMFA) {
		t.Fatalf("expected replayed recovery code to fail, got %v", err)
	}
}

func TestRepositoryMFARecoveryCodeLoginRejectsLockedRecoveryPathIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(
		pool,
		WithMFAFactorIDGenerator(func() (string, error) { return "mfa-factor-recovery-login-locked", nil }),
		WithSessionIDGenerator(func() (string, error) { return "session-recovery-login-locked", nil }),
	)
	now := time.Unix(1_800_000_000, 0).UTC()
	if _, err := repository.RegisterUser(ctx, types.RegisterUserCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
	}, "password-hash", now); err != nil {
		t.Fatalf("register user: %v", err)
	}
	if _, err := repository.CreateMFAFactor(ctx, types.BeginMFAEnrollmentCommand{
		TenantID:   "tenant-identity",
		UserID:     "user-1",
		FactorType: types.MFAFactorTypeTOTP,
	}, types.EncryptedMFASecret{Ciphertext: "encrypted-secret", Nonce: "nonce-value", KeyVersion: "local-v1"}, now); err != nil {
		t.Fatalf("create mfa factor: %v", err)
	}
	if _, err := repository.ConfirmMFAFactor(ctx, types.ConfirmMFAEnrollmentCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		FactorID: "mfa-factor-recovery-login-locked",
	}, []types.MFARecoveryCodeRecord{{CodeID: "recovery-login-locked", CodeHash: "recovery-login-locked-hash"}}, now.Add(time.Minute)); err != nil {
		t.Fatalf("confirm mfa factor: %v", err)
	}
	recovery, err := repository.FindActiveMFARecoveryCode(ctx, "tenant-identity", "user-1", "recovery-login-locked-hash")
	if err != nil {
		t.Fatalf("find recovery code before login: %v", err)
	}
	lockAt := now.Add(2 * time.Minute)
	err = repository.RecordMFARecoveryLoginFailure(ctx, "tenant-identity", "user-1", lockAt, lockAt.Add(15*time.Minute), 1, lockAt.Add(-15*time.Minute))
	if !errors.Is(err, types.ErrMFALocked) {
		t.Fatalf("expected recovery-code lock, got %v", err)
	}
	assertMFARecoveryLoginRisk(t, ctx, pool, 1, true)

	_, err = repository.LoginGatewaySession(ctx, types.LoginCommand{
		TenantID:            "tenant-identity",
		UserID:              "user-1",
		DeviceID:            "device-1",
		Audience:            "push-gateway",
		UsedMFARecoveryCode: recovery,
	}, types.RefreshTokenRecord{
		TokenID:   "rft_recovery_login_locked",
		TokenHash: "hash-recovery-login-locked",
	}, now.Add(3*time.Minute), now.Add(18*time.Minute), now.Add(30*24*time.Hour))
	if !errors.Is(err, types.ErrMFALocked) {
		t.Fatalf("expected locked recovery-code login to fail, got %v", err)
	}
	if _, err := repository.FindActiveMFARecoveryCode(ctx, "tenant-identity", "user-1", "recovery-login-locked-hash"); err != nil {
		t.Fatalf("expected locked login to leave recovery code active, got %v", err)
	}
	assertRefreshTokenMissing(t, ctx, pool, "rft_recovery_login_locked")
	assertSessionMissing(t, ctx, pool, "session-recovery-login-locked")
	assertMFARecoveryLoginRisk(t, ctx, pool, 1, true)
}

func TestRepositoryMFARecoveryLoginFailureLocksIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(pool)
	now := time.Unix(1_800_000_000, 0).UTC()
	if _, err := repository.RegisterUser(ctx, types.RegisterUserCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
	}, "password-hash", now); err != nil {
		t.Fatalf("register user: %v", err)
	}
	firstFailureAt := now.Add(time.Minute)
	lockUntil := firstFailureAt.Add(15 * time.Minute)
	if err := repository.RecordMFARecoveryLoginFailure(ctx, "tenant-identity", "user-1", firstFailureAt, lockUntil, 2, firstFailureAt.Add(-15*time.Minute)); err != nil {
		t.Fatalf("record first recovery-code failure: %v", err)
	}
	assertMFARecoveryLoginRisk(t, ctx, pool, 1, false)

	err := repository.RecordMFARecoveryLoginFailure(ctx, "tenant-identity", "user-1", firstFailureAt.Add(time.Minute), lockUntil.Add(time.Minute), 2, firstFailureAt.Add(-14*time.Minute))
	if !errors.Is(err, types.ErrMFALocked) {
		t.Fatalf("expected recovery-code mfa lock on threshold, got %v", err)
	}
	assertMFARecoveryLoginRisk(t, ctx, pool, 2, true)

	credential, err := repository.GetUserCredential(ctx, "tenant-identity", "user-1")
	if err != nil {
		t.Fatalf("get credential after recovery-code lock: %v", err)
	}
	if credential.MFARecoveryFailedCount != 2 || !credential.MFARecoveryLockedUntil.After(firstFailureAt) {
		t.Fatalf("expected credential to carry recovery-code risk state, got %+v", credential)
	}
}

func TestRepositoryMFARecoveryCodeRegenerateAndRevokeIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(
		pool,
		WithMFAFactorIDGenerator(func() (string, error) { return "mfa-factor-recovery-manage", nil }),
	)
	now := time.Unix(1_800_000_000, 0).UTC()
	if _, err := repository.RegisterUser(ctx, types.RegisterUserCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
	}, "password-hash", now); err != nil {
		t.Fatalf("register user: %v", err)
	}
	if _, err := repository.CreateMFAFactor(ctx, types.BeginMFAEnrollmentCommand{
		TenantID:   "tenant-identity",
		UserID:     "user-1",
		FactorType: types.MFAFactorTypeTOTP,
	}, types.EncryptedMFASecret{Ciphertext: "encrypted-secret", Nonce: "nonce-value", KeyVersion: "local-v1"}, now); err != nil {
		t.Fatalf("create mfa factor: %v", err)
	}
	if _, err := repository.ConfirmMFAFactor(ctx, types.ConfirmMFAEnrollmentCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		FactorID: "mfa-factor-recovery-manage",
	}, []types.MFARecoveryCodeRecord{{CodeID: "recovery-old-1", CodeHash: "recovery-old-hash-1"}}, now.Add(time.Minute)); err != nil {
		t.Fatalf("confirm mfa factor: %v", err)
	}
	if _, err := repository.FindActiveMFARecoveryCode(ctx, "tenant-identity", "user-1", "recovery-old-hash-1"); err != nil {
		t.Fatalf("expected old recovery code to be active before regenerate: %v", err)
	}
	generatedAt := now.Add(2 * time.Minute)
	result, err := repository.ReplaceMFARecoveryCodes(ctx, types.RegenerateMFARecoveryCodesCommand{
		TenantID:  "tenant-identity",
		UserID:    "user-1",
		FactorID:  "mfa-factor-recovery-manage",
		TraceID:   "trace-regenerate",
		RequestID: "request-regenerate",
	}, []types.MFARecoveryCodeRecord{
		{CodeID: "recovery-new-1", CodeHash: "recovery-new-hash-1"},
		{CodeID: "recovery-new-2", CodeHash: "recovery-new-hash-2"},
	}, generatedAt)
	if err != nil {
		t.Fatalf("replace recovery codes: %v", err)
	}
	if result.FactorID != "mfa-factor-recovery-manage" || result.GeneratedAtUnixMS != generatedAt.UnixMilli() {
		t.Fatalf("unexpected regenerate result: %+v", result)
	}
	if _, err := repository.FindActiveMFARecoveryCode(ctx, "tenant-identity", "user-1", "recovery-old-hash-1"); !errors.Is(err, types.ErrInvalidMFA) {
		t.Fatalf("expected old recovery code to be disabled, got %v", err)
	}
	if _, err := repository.FindActiveMFARecoveryCode(ctx, "tenant-identity", "user-1", "recovery-new-hash-1"); err != nil {
		t.Fatalf("expected new recovery code to be active: %v", err)
	}
	revokedAt := now.Add(3 * time.Minute)
	revokeResult, err := repository.RevokeMFARecoveryCodes(ctx, types.RevokeMFARecoveryCodesCommand{
		TenantID:  "tenant-identity",
		UserID:    "user-1",
		TraceID:   "trace-revoke",
		RequestID: "request-revoke",
	}, revokedAt)
	if err != nil {
		t.Fatalf("revoke recovery codes: %v", err)
	}
	if revokeResult.RevokedCount != 2 || revokeResult.RevokedAtUnixMS != revokedAt.UnixMilli() {
		t.Fatalf("unexpected revoke result: %+v", revokeResult)
	}
	if _, err := repository.FindActiveMFARecoveryCode(ctx, "tenant-identity", "user-1", "recovery-new-hash-1"); !errors.Is(err, types.ErrInvalidMFA) {
		t.Fatalf("expected new recovery code to be revoked, got %v", err)
	}
	replay, err := repository.RevokeMFARecoveryCodes(ctx, types.RevokeMFARecoveryCodesCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
	}, revokedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("revoke recovery codes replay: %v", err)
	}
	if replay.RevokedCount != 0 {
		t.Fatalf("expected idempotent replay to revoke zero codes, got %+v", replay)
	}
	if _, err := repository.DisableMFAFactor(ctx, types.DisableMFAFactorCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		FactorID: "mfa-factor-recovery-manage",
	}, revokedAt.Add(2*time.Minute)); err != nil {
		t.Fatalf("disable mfa factor: %v", err)
	}
	_, err = repository.ReplaceMFARecoveryCodes(ctx, types.RegenerateMFARecoveryCodesCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		FactorID: "mfa-factor-recovery-manage",
	}, []types.MFARecoveryCodeRecord{{CodeID: "recovery-after-disabled", CodeHash: "recovery-after-disabled-hash"}}, revokedAt.Add(3*time.Minute))
	if !errors.Is(err, types.ErrMFAFactorNotFound) {
		t.Fatalf("expected disabled factor to reject recovery code replace, got %v", err)
	}
}
