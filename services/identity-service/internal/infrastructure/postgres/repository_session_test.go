package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestRepositoryIssueGatewaySessionIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(
		pool,
		WithSessionIDGenerator(func() (string, error) { return "session-1", nil }),
		WithEventIDGenerator(func() (string, error) { return "event-device-revoked-1", nil }),
	)

	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	expiresAt := issuedAt.Add(15 * time.Minute)
	result, err := repository.IssueGatewaySession(ctx, types.IssueGatewayTokenCommand{
		TenantID:  "tenant-identity",
		UserID:    "user-1",
		DeviceID:  "device-1",
		Audience:  "push-gateway",
		TraceID:   "trace-1",
		RequestID: "request-1",
	}, issuedAt, expiresAt)
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}
	if result.SessionID != "session-1" || result.ExpiresAtUnixMS != expiresAt.UnixMilli() {
		t.Fatalf("unexpected result: %+v", result)
	}
	assertDeviceStatus(t, ctx, pool, "ACTIVE")
	assertSessionStatus(t, ctx, pool, "session-1", "ACTIVE")
}

func TestRepositoryRegisterUserIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(pool)
	createdAt := time.Unix(1_800_000_000, 0).UTC()

	result, err := repository.RegisterUser(ctx, types.RegisterUserCommand{
		TenantID:  "tenant-identity",
		UserID:    "user-1",
		TraceID:   "trace-register",
		RequestID: "request-register",
	}, "password-hash", createdAt)
	if err != nil {
		t.Fatalf("register user: %v", err)
	}
	if result.Status != types.UserStatusActive || result.CreatedAtUnixMS != createdAt.UnixMilli() {
		t.Fatalf("unexpected result: %+v", result)
	}
	credential, err := repository.GetUserCredential(ctx, "tenant-identity", "user-1")
	if err != nil {
		t.Fatalf("get credential: %v", err)
	}
	if credential.Status != "ACTIVE" || credential.PasswordHash != "password-hash" {
		t.Fatalf("unexpected credential: %+v", credential)
	}

	_, err = repository.RegisterUser(ctx, types.RegisterUserCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
	}, "different-password-hash", createdAt.Add(time.Minute))
	if !errors.Is(err, types.ErrUserAlreadyExists) {
		t.Fatalf("expected user already exists, got %v", err)
	}
}

func TestRepositoryRevokeDeviceRejectsFutureIssueIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(
		pool,
		WithSessionIDGenerator(func() (string, error) { return "session-1", nil }),
		WithEventIDGenerator(func() (string, error) { return "event-device-revoked-1", nil }),
	)
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	expiresAt := issuedAt.Add(15 * time.Minute)
	if _, err := repository.IssueGatewaySession(ctx, issueCommand(""), issuedAt, expiresAt); err != nil {
		t.Fatalf("issue before revoke: %v", err)
	}
	if _, err := repository.RevokeDevice(ctx, types.RevokeDeviceCommand{
		AdminContext: types.AdminContext{TenantID: "tenant-identity", OperatorUserID: "admin-1"},
		UserID:       "user-1",
		DeviceID:     "device-1",
		Reason:       "lost device",
	}, issuedAt.Add(time.Minute)); err != nil {
		t.Fatalf("revoke device: %v", err)
	}
	assertDeviceStatus(t, ctx, pool, "REVOKED")
	assertSessionStatus(t, ctx, pool, "session-1", "REVOKED")
	assertOutboxEvent(t, ctx, pool, "identity.device.revoked.v1", "identity_device", "event-device-revoked-1")
	_, err := repository.IssueGatewaySession(ctx, issueCommand("session-2"), issuedAt.Add(2*time.Minute), expiresAt.Add(2*time.Minute))
	if !errors.Is(err, types.ErrDeviceRevoked) {
		t.Fatalf("expected device revoked, got %v", err)
	}
}

func TestRepositoryRevokeSessionRejectsSameSessionIDIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(pool, WithEventIDGenerator(func() (string, error) { return "event-session-revoked-1", nil }))
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	expiresAt := issuedAt.Add(15 * time.Minute)
	if _, err := repository.IssueGatewaySession(ctx, issueCommand("session-explicit"), issuedAt, expiresAt); err != nil {
		t.Fatalf("issue before revoke: %v", err)
	}
	if _, err := repository.RevokeSession(ctx, types.RevokeSessionCommand{
		AdminContext: types.AdminContext{TenantID: "tenant-identity", OperatorUserID: "admin-1"},
		UserID:       "user-1",
		DeviceID:     "device-1",
		SessionID:    "session-explicit",
		Reason:       "manual revoke",
	}, issuedAt.Add(time.Minute)); err != nil {
		t.Fatalf("revoke session: %v", err)
	}
	_, err := repository.IssueGatewaySession(ctx, issueCommand("session-explicit"), issuedAt.Add(2*time.Minute), expiresAt.Add(2*time.Minute))
	if !errors.Is(err, types.ErrSessionRevoked) {
		t.Fatalf("expected session revoked, got %v", err)
	}
	assertOutboxEvent(t, ctx, pool, "identity.session.revoked.v1", "identity_session", "event-session-revoked-1")
}

func TestRepositoryLoginAndRefreshRotationIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	seedUserCredential(t, ctx, pool, "password-hash")
	repository := NewRepository(pool, WithSessionIDGenerator(func() (string, error) { return "session-login-1", nil }))
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	gatewayExpiresAt := issuedAt.Add(15 * time.Minute)
	refreshExpiresAt := issuedAt.Add(30 * 24 * time.Hour)

	loginResult, err := repository.LoginGatewaySession(ctx, types.LoginCommand{
		TenantID:  "tenant-identity",
		UserID:    "user-1",
		DeviceID:  "device-1",
		Audience:  "push-gateway",
		TraceID:   "trace-login",
		RequestID: "request-login",
	}, types.RefreshTokenRecord{
		TokenID:   "rft_login",
		TokenHash: "hash-login",
	}, issuedAt, gatewayExpiresAt, refreshExpiresAt)
	if err != nil {
		t.Fatalf("login session: %v", err)
	}
	if loginResult.SessionID != "session-login-1" || loginResult.RefreshExpiresAtUnixMS != refreshExpiresAt.UnixMilli() {
		t.Fatalf("unexpected login result: %+v", loginResult)
	}
	assertSessionStatus(t, ctx, pool, "session-login-1", "ACTIVE")
	assertRefreshTokenStatus(t, ctx, pool, "rft_login", "ACTIVE")

	refreshedAt := issuedAt.Add(time.Minute)
	nextRefreshExpiresAt := refreshedAt.Add(30 * 24 * time.Hour)
	refreshResult, err := repository.RefreshGatewaySession(ctx, types.RefreshGatewayTokenCommand{
		TenantID:  "tenant-identity",
		UserID:    "user-1",
		DeviceID:  "device-1",
		Audience:  "push-gateway",
		TraceID:   "trace-refresh",
		RequestID: "request-refresh",
	}, "rft_login", "hash-login", types.RefreshTokenRecord{
		TokenID:   "rft_next",
		TokenHash: "hash-next",
	}, refreshedAt, refreshedAt.Add(15*time.Minute), nextRefreshExpiresAt)
	if err != nil {
		t.Fatalf("refresh gateway session: %v", err)
	}
	if refreshResult.SessionID != "session-login-1" || refreshResult.RefreshExpiresAtUnixMS != nextRefreshExpiresAt.UnixMilli() {
		t.Fatalf("unexpected refresh result: %+v", refreshResult)
	}
	assertRefreshTokenStatus(t, ctx, pool, "rft_login", "USED")
	assertRefreshTokenStatus(t, ctx, pool, "rft_next", "ACTIVE")

	_, err = repository.RefreshGatewaySession(ctx, types.RefreshGatewayTokenCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		DeviceID: "device-1",
		Audience: "push-gateway",
	}, "rft_login", "hash-login", types.RefreshTokenRecord{
		TokenID:   "rft_reuse",
		TokenHash: "hash-reuse",
	}, refreshedAt.Add(time.Minute), refreshedAt.Add(16*time.Minute), nextRefreshExpiresAt.Add(time.Minute))
	if !errors.Is(err, types.ErrRefreshTokenReuseDetected) {
		t.Fatalf("expected refresh token reuse error, got %v", err)
	}
}

func TestRepositoryValidateRefreshGatewaySessionReuseRevokesSessionIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	seedUserCredential(t, ctx, pool, "password-hash")
	repository := NewRepository(
		pool,
		WithSessionIDGenerator(func() (string, error) { return "session-refresh-reuse-proof", nil }),
		WithEventIDGenerator(func() (string, error) { return "event-refresh-reuse-proof-1", nil }),
	)
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	if _, err := repository.LoginGatewaySession(ctx, types.LoginCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		DeviceID: "device-1",
		Audience: "push-gateway",
	}, types.RefreshTokenRecord{
		TokenID:   "rft_reuse_proof_login",
		TokenHash: "hash-reuse-proof-login",
	}, issuedAt, issuedAt.Add(15*time.Minute), issuedAt.Add(30*24*time.Hour)); err != nil {
		t.Fatalf("login before reuse validation: %v", err)
	}
	refreshedAt := issuedAt.Add(time.Minute)
	if _, err := repository.RefreshGatewaySession(ctx, types.RefreshGatewayTokenCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		DeviceID: "device-1",
		Audience: "push-gateway",
	}, "rft_reuse_proof_login", "hash-reuse-proof-login", types.RefreshTokenRecord{
		TokenID:   "rft_reuse_proof_next",
		TokenHash: "hash-reuse-proof-next",
	}, refreshedAt, refreshedAt.Add(15*time.Minute), refreshedAt.Add(30*24*time.Hour)); err != nil {
		t.Fatalf("first refresh: %v", err)
	}

	err := repository.ValidateRefreshGatewaySession(ctx, types.RefreshGatewayTokenCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		DeviceID:    "device-1",
		Audience:    "push-gateway",
		MFAFactorID: "mfa-factor-reuse-proof",
		MFACode:     "123456",
		TraceID:     "trace-reuse-proof",
		RequestID:   "request-reuse-proof",
	}, "rft_reuse_proof_login", "hash-reuse-proof-login", refreshedAt.Add(time.Minute))
	if !errors.Is(err, types.ErrRefreshTokenReuseDetected) {
		t.Fatalf("expected refresh reuse, got %v", err)
	}
	assertSessionStatus(t, ctx, pool, "session-refresh-reuse-proof", "REVOKED")
	assertOutboxEvent(t, ctx, pool, "identity.session.revoked.v1", "identity_session", "event-refresh-reuse-proof-1")
}

func TestRepositoryValidateRefreshGatewaySessionExpiresTokenIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	seedUserCredential(t, ctx, pool, "password-hash")
	repository := NewRepository(pool, WithSessionIDGenerator(func() (string, error) { return "session-refresh-expired-proof", nil }))
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	if _, err := repository.LoginGatewaySession(ctx, types.LoginCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		DeviceID: "device-1",
		Audience: "push-gateway",
	}, types.RefreshTokenRecord{
		TokenID:   "rft_expired_proof_login",
		TokenHash: "hash-expired-proof-login",
	}, issuedAt, issuedAt.Add(15*time.Minute), issuedAt.Add(time.Minute)); err != nil {
		t.Fatalf("login before expired validation: %v", err)
	}
	err := repository.ValidateRefreshGatewaySession(ctx, types.RefreshGatewayTokenCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		DeviceID:    "device-1",
		Audience:    "push-gateway",
		MFAFactorID: "mfa-factor-expired-proof",
		MFACode:     "123456",
	}, "rft_expired_proof_login", "hash-expired-proof-login", issuedAt.Add(2*time.Minute))
	if !errors.Is(err, types.ErrInvalidRefreshToken) {
		t.Fatalf("expected invalid refresh token, got %v", err)
	}
	assertRefreshTokenStatus(t, ctx, pool, "rft_expired_proof_login", "REVOKED")
	assertSessionStatus(t, ctx, pool, "session-refresh-expired-proof", "ACTIVE")
}

func TestRepositoryRefreshRequiresStepUpWhenMFAEnabledAfterLoginIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	seedUserCredential(t, ctx, pool, "password-hash")
	repository := NewRepository(
		pool,
		WithSessionIDGenerator(func() (string, error) { return "session-step-up", nil }),
		WithMFAFactorIDGenerator(func() (string, error) { return "mfa-factor-step-up", nil }),
	)
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	if _, err := repository.LoginGatewaySession(ctx, types.LoginCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		DeviceID: "device-1",
		Audience: "push-gateway",
	}, types.RefreshTokenRecord{
		TokenID:   "rft_pre_mfa_login",
		TokenHash: "hash-pre-mfa-login",
	}, issuedAt, issuedAt.Add(15*time.Minute), issuedAt.Add(30*24*time.Hour)); err != nil {
		t.Fatalf("login before mfa enrollment: %v", err)
	}
	if _, err := repository.CreateMFAFactor(ctx, types.BeginMFAEnrollmentCommand{
		TenantID:   "tenant-identity",
		UserID:     "user-1",
		FactorType: types.MFAFactorTypeTOTP,
	}, types.EncryptedMFASecret{Ciphertext: "encrypted-secret", Nonce: "nonce-value", KeyVersion: "local-v1"}, issuedAt.Add(time.Minute)); err != nil {
		t.Fatalf("create mfa factor: %v", err)
	}
	if _, err := repository.ConfirmMFAFactor(ctx, types.ConfirmMFAEnrollmentCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		FactorID: "mfa-factor-step-up",
	}, []types.MFARecoveryCodeRecord{{CodeID: "recovery-step-up", CodeHash: "recovery-step-up-hash"}}, issuedAt.Add(2*time.Minute)); err != nil {
		t.Fatalf("confirm mfa factor: %v", err)
	}
	_, err := repository.RefreshGatewaySession(ctx, types.RefreshGatewayTokenCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		DeviceID: "device-1",
		Audience: "push-gateway",
	}, "rft_pre_mfa_login", "hash-pre-mfa-login", types.RefreshTokenRecord{
		TokenID:   "rft_should_not_rotate",
		TokenHash: "hash-should-not-rotate",
	}, issuedAt.Add(3*time.Minute), issuedAt.Add(18*time.Minute), issuedAt.Add(30*24*time.Hour))
	if !errors.Is(err, types.ErrMFARequired) {
		t.Fatalf("expected refresh step-up to require mfa, got %v", err)
	}
	assertRefreshTokenStatus(t, ctx, pool, "rft_pre_mfa_login", "ACTIVE")
	assertRefreshTokenMissing(t, ctx, pool, "rft_should_not_rotate")
}

func TestRepositoryRefreshAcceptsSubmittedTOTPProofIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	seedUserCredential(t, ctx, pool, "password-hash")
	repository := NewRepository(
		pool,
		WithSessionIDGenerator(func() (string, error) { return "session-refresh-step-up-totp", nil }),
		WithMFAFactorIDGenerator(func() (string, error) { return "mfa-factor-refresh-step-up", nil }),
	)
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	if _, err := repository.LoginGatewaySession(ctx, types.LoginCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		DeviceID: "device-1",
		Audience: "push-gateway",
	}, types.RefreshTokenRecord{
		TokenID:   "rft_refresh_totp_login",
		TokenHash: "hash-refresh-totp-login",
	}, issuedAt, issuedAt.Add(15*time.Minute), issuedAt.Add(30*24*time.Hour)); err != nil {
		t.Fatalf("login before mfa enrollment: %v", err)
	}
	if _, err := repository.CreateMFAFactor(ctx, types.BeginMFAEnrollmentCommand{
		TenantID:   "tenant-identity",
		UserID:     "user-1",
		FactorType: types.MFAFactorTypeTOTP,
	}, types.EncryptedMFASecret{Ciphertext: "encrypted-secret", Nonce: "nonce-value", KeyVersion: "local-v1"}, issuedAt.Add(time.Minute)); err != nil {
		t.Fatalf("create mfa factor: %v", err)
	}
	if _, err := repository.ConfirmMFAFactor(ctx, types.ConfirmMFAEnrollmentCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		FactorID: "mfa-factor-refresh-step-up",
	}, []types.MFARecoveryCodeRecord{{CodeID: "recovery-refresh-step-up", CodeHash: "recovery-refresh-step-up-hash"}}, issuedAt.Add(2*time.Minute)); err != nil {
		t.Fatalf("confirm mfa factor: %v", err)
	}
	if err := repository.RecordMFALoginFailure(ctx, "tenant-identity", "user-1", "mfa-factor-refresh-step-up", issuedAt.Add(3*time.Minute), issuedAt.Add(13*time.Minute), 3, issuedAt); err != nil {
		t.Fatalf("record mfa failure before refresh proof: %v", err)
	}
	assertMFALoginRisk(t, ctx, pool, "mfa-factor-refresh-step-up", 1, false, false)
	refreshAt := issuedAt.Add(4 * time.Minute)
	refreshResult, err := repository.RefreshGatewaySession(ctx, types.RefreshGatewayTokenCommand{
		TenantID:            "tenant-identity",
		UserID:              "user-1",
		DeviceID:            "device-1",
		Audience:            "push-gateway",
		VerifiedMFAFactorID: "mfa-factor-refresh-step-up",
	}, "rft_refresh_totp_login", "hash-refresh-totp-login", types.RefreshTokenRecord{
		TokenID:   "rft_refresh_totp_next",
		TokenHash: "hash-refresh-totp-next",
	}, refreshAt, refreshAt.Add(15*time.Minute), refreshAt.Add(30*24*time.Hour))
	if err != nil {
		t.Fatalf("refresh with submitted totp proof: %v", err)
	}
	if refreshResult.SessionID != "session-refresh-step-up-totp" {
		t.Fatalf("unexpected refresh result: %+v", refreshResult)
	}
	assertRefreshTokenStatus(t, ctx, pool, "rft_refresh_totp_login", "USED")
	assertRefreshTokenStatus(t, ctx, pool, "rft_refresh_totp_next", "ACTIVE")
	assertSessionMFAProof(t, ctx, pool, "session-refresh-step-up-totp", "TOTP", "mfa-factor-refresh-step-up")
	assertMFALoginRisk(t, ctx, pool, "mfa-factor-refresh-step-up", 0, false, true)
}

func TestRepositoryRefreshRejectsSubmittedTOTPProofWhenFactorLockedIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	seedUserCredential(t, ctx, pool, "password-hash")
	repository := NewRepository(
		pool,
		WithSessionIDGenerator(func() (string, error) { return "session-refresh-totp-locked", nil }),
		WithMFAFactorIDGenerator(func() (string, error) { return "mfa-factor-refresh-totp-locked", nil }),
	)
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	if _, err := repository.LoginGatewaySession(ctx, types.LoginCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		DeviceID: "device-1",
		Audience: "push-gateway",
	}, types.RefreshTokenRecord{
		TokenID:   "rft_refresh_totp_locked_login",
		TokenHash: "hash-refresh-totp-locked-login",
	}, issuedAt, issuedAt.Add(15*time.Minute), issuedAt.Add(30*24*time.Hour)); err != nil {
		t.Fatalf("login before mfa enrollment: %v", err)
	}
	if _, err := repository.CreateMFAFactor(ctx, types.BeginMFAEnrollmentCommand{
		TenantID:   "tenant-identity",
		UserID:     "user-1",
		FactorType: types.MFAFactorTypeTOTP,
	}, types.EncryptedMFASecret{Ciphertext: "encrypted-secret", Nonce: "nonce-value", KeyVersion: "local-v1"}, issuedAt.Add(time.Minute)); err != nil {
		t.Fatalf("create mfa factor: %v", err)
	}
	if _, err := repository.ConfirmMFAFactor(ctx, types.ConfirmMFAEnrollmentCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		FactorID: "mfa-factor-refresh-totp-locked",
	}, []types.MFARecoveryCodeRecord{{CodeID: "recovery-refresh-totp-locked", CodeHash: "recovery-refresh-totp-locked-hash"}}, issuedAt.Add(2*time.Minute)); err != nil {
		t.Fatalf("confirm mfa factor: %v", err)
	}
	lockAt := issuedAt.Add(3 * time.Minute)
	err := repository.RecordMFALoginFailure(ctx, "tenant-identity", "user-1", "mfa-factor-refresh-totp-locked", lockAt, lockAt.Add(15*time.Minute), 1, lockAt.Add(-15*time.Minute))
	if !errors.Is(err, types.ErrMFALocked) {
		t.Fatalf("expected totp factor lock, got %v", err)
	}
	assertMFALoginRisk(t, ctx, pool, "mfa-factor-refresh-totp-locked", 1, true, false)

	refreshAt := issuedAt.Add(4 * time.Minute)
	_, err = repository.RefreshGatewaySession(ctx, types.RefreshGatewayTokenCommand{
		TenantID:            "tenant-identity",
		UserID:              "user-1",
		DeviceID:            "device-1",
		Audience:            "push-gateway",
		VerifiedMFAFactorID: "mfa-factor-refresh-totp-locked",
	}, "rft_refresh_totp_locked_login", "hash-refresh-totp-locked-login", types.RefreshTokenRecord{
		TokenID:   "rft_refresh_totp_locked_next",
		TokenHash: "hash-refresh-totp-locked-next",
	}, refreshAt, refreshAt.Add(15*time.Minute), refreshAt.Add(30*24*time.Hour))
	if !errors.Is(err, types.ErrMFALocked) {
		t.Fatalf("expected locked totp refresh proof to fail, got %v", err)
	}
	assertRefreshTokenStatus(t, ctx, pool, "rft_refresh_totp_locked_login", "ACTIVE")
	assertRefreshTokenMissing(t, ctx, pool, "rft_refresh_totp_locked_next")
	assertSessionMFAProof(t, ctx, pool, "session-refresh-totp-locked", "", "")
	assertMFALoginRisk(t, ctx, pool, "mfa-factor-refresh-totp-locked", 1, true, false)
}

func TestRepositoryRefreshAcceptsSubmittedRecoveryCodeProofIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	seedUserCredential(t, ctx, pool, "password-hash")
	repository := NewRepository(
		pool,
		WithSessionIDGenerator(func() (string, error) { return "session-refresh-step-up-recovery", nil }),
		WithMFAFactorIDGenerator(func() (string, error) { return "mfa-factor-refresh-recovery", nil }),
	)
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	if _, err := repository.LoginGatewaySession(ctx, types.LoginCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		DeviceID: "device-1",
		Audience: "push-gateway",
	}, types.RefreshTokenRecord{
		TokenID:   "rft_refresh_recovery_login",
		TokenHash: "hash-refresh-recovery-login",
	}, issuedAt, issuedAt.Add(15*time.Minute), issuedAt.Add(30*24*time.Hour)); err != nil {
		t.Fatalf("login before mfa enrollment: %v", err)
	}
	if _, err := repository.CreateMFAFactor(ctx, types.BeginMFAEnrollmentCommand{
		TenantID:   "tenant-identity",
		UserID:     "user-1",
		FactorType: types.MFAFactorTypeTOTP,
	}, types.EncryptedMFASecret{Ciphertext: "encrypted-secret", Nonce: "nonce-value", KeyVersion: "local-v1"}, issuedAt.Add(time.Minute)); err != nil {
		t.Fatalf("create mfa factor: %v", err)
	}
	if _, err := repository.ConfirmMFAFactor(ctx, types.ConfirmMFAEnrollmentCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		FactorID: "mfa-factor-refresh-recovery",
	}, []types.MFARecoveryCodeRecord{{CodeID: "recovery-refresh-direct", CodeHash: "recovery-refresh-direct-hash"}}, issuedAt.Add(2*time.Minute)); err != nil {
		t.Fatalf("confirm mfa factor: %v", err)
	}
	recovery, err := repository.FindActiveMFARecoveryCode(ctx, "tenant-identity", "user-1", "recovery-refresh-direct-hash")
	if err != nil {
		t.Fatalf("find active recovery code: %v", err)
	}
	if err := repository.RecordMFARecoveryLoginFailure(ctx, "tenant-identity", "user-1", issuedAt.Add(3*time.Minute), issuedAt.Add(13*time.Minute), 3, issuedAt); err != nil {
		t.Fatalf("record recovery-code failure before refresh proof: %v", err)
	}
	assertMFARecoveryLoginRisk(t, ctx, pool, 1, false)
	refreshAt := issuedAt.Add(4 * time.Minute)
	_, err = repository.RefreshGatewaySession(ctx, types.RefreshGatewayTokenCommand{
		TenantID:            "tenant-identity",
		UserID:              "user-1",
		DeviceID:            "device-1",
		Audience:            "push-gateway",
		UsedMFARecoveryCode: recovery,
	}, "rft_refresh_recovery_login", "hash-refresh-recovery-login", types.RefreshTokenRecord{
		TokenID:   "rft_refresh_recovery_next",
		TokenHash: "hash-refresh-recovery-next",
	}, refreshAt, refreshAt.Add(15*time.Minute), refreshAt.Add(30*24*time.Hour))
	if err != nil {
		t.Fatalf("refresh with submitted recovery proof: %v", err)
	}
	assertRefreshTokenStatus(t, ctx, pool, "rft_refresh_recovery_login", "USED")
	assertRefreshTokenStatus(t, ctx, pool, "rft_refresh_recovery_next", "ACTIVE")
	assertSessionMFAProof(t, ctx, pool, "session-refresh-step-up-recovery", "RECOVERY_CODE", "")
	if _, err := repository.FindActiveMFARecoveryCode(ctx, "tenant-identity", "user-1", "recovery-refresh-direct-hash"); !errors.Is(err, types.ErrInvalidMFA) {
		t.Fatalf("expected recovery code to be consumed, got %v", err)
	}
	assertMFARecoveryLoginRisk(t, ctx, pool, 0, false)
}

func TestRepositoryRefreshRejectsSubmittedRecoveryCodeProofWhenRecoveryPathLockedIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	seedUserCredential(t, ctx, pool, "password-hash")
	repository := NewRepository(
		pool,
		WithSessionIDGenerator(func() (string, error) { return "session-refresh-recovery-locked", nil }),
		WithMFAFactorIDGenerator(func() (string, error) { return "mfa-factor-refresh-recovery-locked", nil }),
	)
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	if _, err := repository.LoginGatewaySession(ctx, types.LoginCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		DeviceID: "device-1",
		Audience: "push-gateway",
	}, types.RefreshTokenRecord{
		TokenID:   "rft_refresh_recovery_locked_login",
		TokenHash: "hash-refresh-recovery-locked-login",
	}, issuedAt, issuedAt.Add(15*time.Minute), issuedAt.Add(30*24*time.Hour)); err != nil {
		t.Fatalf("login before mfa enrollment: %v", err)
	}
	if _, err := repository.CreateMFAFactor(ctx, types.BeginMFAEnrollmentCommand{
		TenantID:   "tenant-identity",
		UserID:     "user-1",
		FactorType: types.MFAFactorTypeTOTP,
	}, types.EncryptedMFASecret{Ciphertext: "encrypted-secret", Nonce: "nonce-value", KeyVersion: "local-v1"}, issuedAt.Add(time.Minute)); err != nil {
		t.Fatalf("create mfa factor: %v", err)
	}
	if _, err := repository.ConfirmMFAFactor(ctx, types.ConfirmMFAEnrollmentCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		FactorID: "mfa-factor-refresh-recovery-locked",
	}, []types.MFARecoveryCodeRecord{{CodeID: "recovery-refresh-locked", CodeHash: "recovery-refresh-locked-hash"}}, issuedAt.Add(2*time.Minute)); err != nil {
		t.Fatalf("confirm mfa factor: %v", err)
	}
	recovery, err := repository.FindActiveMFARecoveryCode(ctx, "tenant-identity", "user-1", "recovery-refresh-locked-hash")
	if err != nil {
		t.Fatalf("find active recovery code: %v", err)
	}
	lockAt := issuedAt.Add(3 * time.Minute)
	err = repository.RecordMFARecoveryLoginFailure(ctx, "tenant-identity", "user-1", lockAt, lockAt.Add(15*time.Minute), 1, lockAt.Add(-15*time.Minute))
	if !errors.Is(err, types.ErrMFALocked) {
		t.Fatalf("expected recovery-code lock, got %v", err)
	}
	assertMFARecoveryLoginRisk(t, ctx, pool, 1, true)

	refreshAt := issuedAt.Add(4 * time.Minute)
	_, err = repository.RefreshGatewaySession(ctx, types.RefreshGatewayTokenCommand{
		TenantID:            "tenant-identity",
		UserID:              "user-1",
		DeviceID:            "device-1",
		Audience:            "push-gateway",
		UsedMFARecoveryCode: recovery,
	}, "rft_refresh_recovery_locked_login", "hash-refresh-recovery-locked-login", types.RefreshTokenRecord{
		TokenID:   "rft_refresh_recovery_locked_next",
		TokenHash: "hash-refresh-recovery-locked-next",
	}, refreshAt, refreshAt.Add(15*time.Minute), refreshAt.Add(30*24*time.Hour))
	if !errors.Is(err, types.ErrMFALocked) {
		t.Fatalf("expected locked recovery-code refresh proof to fail, got %v", err)
	}
	assertRefreshTokenStatus(t, ctx, pool, "rft_refresh_recovery_locked_login", "ACTIVE")
	assertRefreshTokenMissing(t, ctx, pool, "rft_refresh_recovery_locked_next")
	assertSessionMFAProof(t, ctx, pool, "session-refresh-recovery-locked", "", "")
	if _, err := repository.FindActiveMFARecoveryCode(ctx, "tenant-identity", "user-1", "recovery-refresh-locked-hash"); err != nil {
		t.Fatalf("expected locked refresh to leave recovery code active, got %v", err)
	}
	assertMFARecoveryLoginRisk(t, ctx, pool, 1, true)
}

func TestRepositoryRefreshRecoveryCodeProofDoesNotDowngradeExistingTOTPProofIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	seedUserCredential(t, ctx, pool, "password-hash")
	repository := NewRepository(
		pool,
		WithSessionIDGenerator(func() (string, error) { return "session-refresh-totp-preserved", nil }),
		WithMFAFactorIDGenerator(func() (string, error) { return "mfa-factor-refresh-totp-preserved", nil }),
	)
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	if _, err := repository.CreateMFAFactor(ctx, types.BeginMFAEnrollmentCommand{
		TenantID:   "tenant-identity",
		UserID:     "user-1",
		FactorType: types.MFAFactorTypeTOTP,
	}, types.EncryptedMFASecret{Ciphertext: "encrypted-secret", Nonce: "nonce-value", KeyVersion: "local-v1"}, issuedAt); err != nil {
		t.Fatalf("create mfa factor: %v", err)
	}
	if _, err := repository.ConfirmMFAFactor(ctx, types.ConfirmMFAEnrollmentCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		FactorID: "mfa-factor-refresh-totp-preserved",
	}, []types.MFARecoveryCodeRecord{{CodeID: "recovery-refresh-preserve", CodeHash: "recovery-refresh-preserve-hash"}}, issuedAt.Add(time.Minute)); err != nil {
		t.Fatalf("confirm mfa factor: %v", err)
	}
	recovery, err := repository.FindActiveMFARecoveryCode(ctx, "tenant-identity", "user-1", "recovery-refresh-preserve-hash")
	if err != nil {
		t.Fatalf("find active recovery code: %v", err)
	}
	if _, err := repository.LoginGatewaySession(ctx, types.LoginCommand{
		TenantID:            "tenant-identity",
		UserID:              "user-1",
		DeviceID:            "device-1",
		Audience:            "push-gateway",
		VerifiedMFAFactorID: "mfa-factor-refresh-totp-preserved",
	}, types.RefreshTokenRecord{
		TokenID:   "rft_refresh_totp_preserved_login",
		TokenHash: "hash-refresh-totp-preserved-login",
	}, issuedAt.Add(2*time.Minute), issuedAt.Add(17*time.Minute), issuedAt.Add(30*24*time.Hour)); err != nil {
		t.Fatalf("login with totp proof: %v", err)
	}
	assertSessionMFAProof(t, ctx, pool, "session-refresh-totp-preserved", "TOTP", "mfa-factor-refresh-totp-preserved")

	refreshAt := issuedAt.Add(3 * time.Minute)
	_, err = repository.RefreshGatewaySession(ctx, types.RefreshGatewayTokenCommand{
		TenantID:            "tenant-identity",
		UserID:              "user-1",
		DeviceID:            "device-1",
		Audience:            "push-gateway",
		UsedMFARecoveryCode: recovery,
	}, "rft_refresh_totp_preserved_login", "hash-refresh-totp-preserved-login", types.RefreshTokenRecord{
		TokenID:   "rft_refresh_totp_preserved_next",
		TokenHash: "hash-refresh-totp-preserved-next",
	}, refreshAt, refreshAt.Add(15*time.Minute), refreshAt.Add(30*24*time.Hour))
	if err != nil {
		t.Fatalf("refresh with submitted recovery proof on totp session: %v", err)
	}
	assertRefreshTokenStatus(t, ctx, pool, "rft_refresh_totp_preserved_login", "USED")
	assertRefreshTokenStatus(t, ctx, pool, "rft_refresh_totp_preserved_next", "ACTIVE")
	assertSessionMFAProof(t, ctx, pool, "session-refresh-totp-preserved", "TOTP", "mfa-factor-refresh-totp-preserved")
	if _, err := repository.FindActiveMFARecoveryCode(ctx, "tenant-identity", "user-1", "recovery-refresh-preserve-hash"); !errors.Is(err, types.ErrInvalidMFA) {
		t.Fatalf("expected submitted recovery code to be consumed, got %v", err)
	}
}

func TestRepositoryRefreshAllowsMFAAuthenticatedSessionIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(
		pool,
		WithSessionIDGenerator(func() (string, error) { return "session-mfa-login", nil }),
		WithMFAFactorIDGenerator(func() (string, error) { return "mfa-factor-refresh", nil }),
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
	}, types.EncryptedMFASecret{Ciphertext: "encrypted-secret", Nonce: "nonce-value", KeyVersion: "local-v1"}, now.Add(time.Minute)); err != nil {
		t.Fatalf("create mfa factor: %v", err)
	}
	if _, err := repository.ConfirmMFAFactor(ctx, types.ConfirmMFAEnrollmentCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		FactorID: "mfa-factor-refresh",
	}, []types.MFARecoveryCodeRecord{{CodeID: "recovery-refresh", CodeHash: "recovery-refresh-hash"}}, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("confirm mfa factor: %v", err)
	}
	loginAt := now.Add(3 * time.Minute)
	if _, err := repository.LoginGatewaySession(ctx, types.LoginCommand{
		TenantID:            "tenant-identity",
		UserID:              "user-1",
		DeviceID:            "device-1",
		Audience:            "push-gateway",
		VerifiedMFAFactorID: "mfa-factor-refresh",
	}, types.RefreshTokenRecord{
		TokenID:   "rft_mfa_login",
		TokenHash: "hash-mfa-login",
	}, loginAt, loginAt.Add(15*time.Minute), loginAt.Add(30*24*time.Hour)); err != nil {
		t.Fatalf("login with mfa proof: %v", err)
	}
	assertSessionMFAProof(t, ctx, pool, "session-mfa-login", "TOTP", "mfa-factor-refresh")
	refreshAt := loginAt.Add(time.Minute)
	refreshResult, err := repository.RefreshGatewaySession(ctx, types.RefreshGatewayTokenCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		DeviceID: "device-1",
		Audience: "push-gateway",
	}, "rft_mfa_login", "hash-mfa-login", types.RefreshTokenRecord{
		TokenID:   "rft_mfa_next",
		TokenHash: "hash-mfa-next",
	}, refreshAt, refreshAt.Add(15*time.Minute), refreshAt.Add(30*24*time.Hour))
	if err != nil {
		t.Fatalf("refresh mfa authenticated session: %v", err)
	}
	if refreshResult.SessionID != "session-mfa-login" {
		t.Fatalf("unexpected refresh result: %+v", refreshResult)
	}
	assertRefreshTokenStatus(t, ctx, pool, "rft_mfa_login", "USED")
	assertRefreshTokenStatus(t, ctx, pool, "rft_mfa_next", "ACTIVE")
	assertSessionMFAProof(t, ctx, pool, "session-mfa-login", "TOTP", "mfa-factor-refresh")
}

func TestRepositorySessionMFAProofConstraintsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	now := time.Unix(1_800_000_000, 0).UTC()

	validCases := []struct {
		name       string
		sessionID  string
		method     string
		factorID   string
		verifiedAt any
	}{
		{name: "empty proof", sessionID: "session-proof-empty", method: "", factorID: "", verifiedAt: nil},
		{name: "totp proof", sessionID: "session-proof-totp", method: "TOTP", factorID: "mfa-factor-1", verifiedAt: now},
		{name: "recovery proof", sessionID: "session-proof-recovery", method: "RECOVERY_CODE", factorID: "", verifiedAt: now},
	}
	for _, tc := range validCases {
		t.Run("valid "+tc.name, func(t *testing.T) {
			if err := insertSessionProof(ctx, pool, tc.sessionID, tc.method, tc.factorID, tc.verifiedAt, now); err != nil {
				t.Fatalf("insert valid proof %s: %v", tc.name, err)
			}
		})
	}

	invalidCases := []struct {
		name       string
		sessionID  string
		method     string
		factorID   string
		verifiedAt any
	}{
		{name: "empty method with verified time", sessionID: "session-proof-invalid-empty-time", method: "", factorID: "", verifiedAt: now},
		{name: "empty method with factor", sessionID: "session-proof-invalid-empty-factor", method: "", factorID: "mfa-factor-1", verifiedAt: nil},
		{name: "totp without verified time", sessionID: "session-proof-invalid-totp-time", method: "TOTP", factorID: "mfa-factor-1", verifiedAt: nil},
		{name: "totp without factor", sessionID: "session-proof-invalid-totp-factor", method: "TOTP", factorID: "", verifiedAt: now},
		{name: "recovery without verified time", sessionID: "session-proof-invalid-recovery-time", method: "RECOVERY_CODE", factorID: "", verifiedAt: nil},
		{name: "recovery with factor", sessionID: "session-proof-invalid-recovery-factor", method: "RECOVERY_CODE", factorID: "mfa-factor-1", verifiedAt: now},
	}
	for _, tc := range invalidCases {
		t.Run("invalid "+tc.name, func(t *testing.T) {
			if err := insertSessionProof(ctx, pool, tc.sessionID, tc.method, tc.factorID, tc.verifiedAt, now); err == nil {
				t.Fatalf("expected invalid proof %s to fail", tc.name)
			}
		})
	}
	stats, err := NewRepository(pool).AuditSessionMFAProofConstraints(ctx)
	if err != nil {
		t.Fatalf("audit session mfa proof constraints: %v", err)
	}
	if stats.InvalidTotal != 0 || stats.UnknownMethod != 0 || stats.EmptyMethodWithProof != 0 || stats.TOTPMissingProof != 0 || stats.RecoveryInvalidProof != 0 {
		t.Fatalf("expected valid session proof rows to pass audit, got %+v", stats)
	}
}

func TestRepositoryLoginFailureLocksAndSuccessClearsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	seedUserCredential(t, ctx, pool, "password-hash")
	repository := NewRepository(pool, WithSessionIDGenerator(func() (string, error) { return "session-login-locked", nil }))
	firstFailureAt := time.Unix(1_800_000_000, 0).UTC()
	lockUntil := firstFailureAt.Add(15 * time.Minute)

	if err := repository.RecordLoginFailure(ctx, "tenant-identity", "user-1", firstFailureAt, lockUntil, 2, firstFailureAt.Add(-15*time.Minute)); err != nil {
		t.Fatalf("record first login failure: %v", err)
	}
	assertLoginRisk(t, ctx, pool, 1, false)

	err := repository.RecordLoginFailure(ctx, "tenant-identity", "user-1", firstFailureAt.Add(time.Minute), lockUntil.Add(time.Minute), 2, firstFailureAt.Add(-14*time.Minute))
	if !errors.Is(err, types.ErrAccountLocked) {
		t.Fatalf("expected account locked on threshold, got %v", err)
	}
	assertLoginRisk(t, ctx, pool, 2, true)

	_, err = repository.LoginGatewaySession(ctx, types.LoginCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		DeviceID: "device-1",
		Audience: "push-gateway",
	}, types.RefreshTokenRecord{
		TokenID:   "rft_locked",
		TokenHash: "hash-locked",
	}, firstFailureAt.Add(2*time.Minute), firstFailureAt.Add(17*time.Minute), firstFailureAt.Add(30*24*time.Hour))
	if !errors.Is(err, types.ErrAccountLocked) {
		t.Fatalf("expected locked login to fail, got %v", err)
	}

	if _, err := pool.Exec(ctx, `
UPDATE identity_users
SET locked_until = $1
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
`, firstFailureAt.Add(-time.Minute)); err != nil {
		t.Fatalf("expire lock: %v", err)
	}
	issuedAt := firstFailureAt.Add(20 * time.Minute)
	if _, err := repository.LoginGatewaySession(ctx, types.LoginCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		DeviceID: "device-1",
		Audience: "push-gateway",
	}, types.RefreshTokenRecord{
		TokenID:   "rft_after_lock",
		TokenHash: "hash-after-lock",
	}, issuedAt, issuedAt.Add(15*time.Minute), issuedAt.Add(30*24*time.Hour)); err != nil {
		t.Fatalf("login after lock expiry: %v", err)
	}
	assertLoginRisk(t, ctx, pool, 0, false)
	assertSessionStatus(t, ctx, pool, "session-login-locked", "ACTIVE")

	if err := repository.RecordLoginFailure(ctx, "tenant-identity", "user-1", issuedAt.Add(time.Hour), issuedAt.Add(time.Hour+15*time.Minute), 2, issuedAt.Add(45*time.Minute)); err != nil {
		t.Fatalf("record fresh window login failure: %v", err)
	}
	assertLoginRisk(t, ctx, pool, 1, false)
}

func TestRepositoryRefreshRejectsWrongSecretIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	seedUserCredential(t, ctx, pool, "password-hash")
	repository := NewRepository(pool, WithSessionIDGenerator(func() (string, error) { return "session-login-1", nil }))
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	if _, err := repository.LoginGatewaySession(ctx, types.LoginCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		DeviceID: "device-1",
		Audience: "push-gateway",
	}, types.RefreshTokenRecord{
		TokenID:   "rft_login",
		TokenHash: "hash-login",
	}, issuedAt, issuedAt.Add(15*time.Minute), issuedAt.Add(30*24*time.Hour)); err != nil {
		t.Fatalf("login session: %v", err)
	}
	_, err := repository.RefreshGatewaySession(ctx, types.RefreshGatewayTokenCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		DeviceID: "device-1",
		Audience: "push-gateway",
	}, "rft_login", "wrong-hash", types.RefreshTokenRecord{
		TokenID:   "rft_next",
		TokenHash: "hash-next",
	}, issuedAt.Add(time.Minute), issuedAt.Add(16*time.Minute), issuedAt.Add(30*24*time.Hour))
	if !errors.Is(err, types.ErrInvalidRefreshToken) {
		t.Fatalf("expected invalid refresh token, got %v", err)
	}
	assertRefreshTokenStatus(t, ctx, pool, "rft_login", "ACTIVE")
}

func TestRepositoryRefreshAllowedWhilePasswordLoginLockedIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	seedUserCredential(t, ctx, pool, "password-hash")
	repository := NewRepository(pool, WithSessionIDGenerator(func() (string, error) { return "session-login-1", nil }))
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	if _, err := repository.LoginGatewaySession(ctx, types.LoginCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		DeviceID: "device-1",
		Audience: "push-gateway",
	}, types.RefreshTokenRecord{
		TokenID:   "rft_login",
		TokenHash: "hash-login",
	}, issuedAt, issuedAt.Add(15*time.Minute), issuedAt.Add(30*24*time.Hour)); err != nil {
		t.Fatalf("login session: %v", err)
	}
	if _, err := pool.Exec(ctx, `
UPDATE identity_users
SET failed_login_count = 5,
    failed_login_last_at = $1,
    locked_until = $2
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
`, issuedAt.Add(time.Minute), issuedAt.Add(15*time.Minute)); err != nil {
		t.Fatalf("lock password login: %v", err)
	}
	refreshResult, err := repository.RefreshGatewaySession(ctx, types.RefreshGatewayTokenCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		DeviceID: "device-1",
		Audience: "push-gateway",
	}, "rft_login", "hash-login", types.RefreshTokenRecord{
		TokenID:   "rft_next",
		TokenHash: "hash-next",
	}, issuedAt.Add(2*time.Minute), issuedAt.Add(17*time.Minute), issuedAt.Add(30*24*time.Hour))
	if err != nil {
		t.Fatalf("refresh while password login locked: %v", err)
	}
	if refreshResult.SessionID != "session-login-1" {
		t.Fatalf("unexpected refresh result: %+v", refreshResult)
	}
	assertRefreshTokenStatus(t, ctx, pool, "rft_login", "USED")
	assertRefreshTokenStatus(t, ctx, pool, "rft_next", "ACTIVE")
}

func TestRepositoryRefreshExpiresTokenIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	seedUserCredential(t, ctx, pool, "password-hash")
	repository := NewRepository(pool, WithSessionIDGenerator(func() (string, error) { return "session-login-1", nil }))
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	if _, err := repository.LoginGatewaySession(ctx, types.LoginCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		DeviceID: "device-1",
		Audience: "push-gateway",
	}, types.RefreshTokenRecord{
		TokenID:   "rft_login",
		TokenHash: "hash-login",
	}, issuedAt, issuedAt.Add(15*time.Minute), issuedAt.Add(time.Minute)); err != nil {
		t.Fatalf("login session: %v", err)
	}
	_, err := repository.RefreshGatewaySession(ctx, types.RefreshGatewayTokenCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
		DeviceID: "device-1",
		Audience: "push-gateway",
	}, "rft_login", "hash-login", types.RefreshTokenRecord{
		TokenID:   "rft_next",
		TokenHash: "hash-next",
	}, issuedAt.Add(2*time.Minute), issuedAt.Add(17*time.Minute), issuedAt.Add(30*24*time.Hour))
	if !errors.Is(err, types.ErrInvalidRefreshToken) {
		t.Fatalf("expected invalid refresh token, got %v", err)
	}
	assertRefreshTokenStatus(t, ctx, pool, "rft_login", "REVOKED")
}
