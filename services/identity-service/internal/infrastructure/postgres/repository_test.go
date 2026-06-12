package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
}

func TestRepositoryVerificationAndPasswordResetChallengesIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(
		pool,
		WithSessionIDGenerator(func() (string, error) { return "session-reset-1", nil }),
		WithEventIDGenerator(func() (string, error) { return "event-password-reset-session-revoked-1", nil }),
	)
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	expiresAt := issuedAt.Add(15 * time.Minute)
	if _, err := repository.RegisterUser(ctx, types.RegisterUserCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
	}, "old-password-hash", issuedAt); err != nil {
		t.Fatalf("register user: %v", err)
	}
	if _, err := repository.LoginGatewaySession(ctx, types.LoginCommand{
		TenantID:  "tenant-identity",
		UserID:    "user-1",
		DeviceID:  "device-1",
		Audience:  "push-gateway",
		TraceID:   "trace-login",
		RequestID: "request-login",
	}, types.RefreshTokenRecord{
		TokenID:   "rft_reset",
		TokenHash: "hash-reset",
	}, issuedAt.Add(10*time.Second), issuedAt.Add(15*time.Minute), issuedAt.Add(30*24*time.Hour)); err != nil {
		t.Fatalf("login before reset: %v", err)
	}
	assertSessionStatus(t, ctx, pool, "session-reset-1", "ACTIVE")
	assertRefreshTokenStatus(t, ctx, pool, "rft_reset", "ACTIVE")

	verification, err := repository.CreateVerificationChallenge(ctx, types.RequestVerificationChallengeCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
		TraceID:     "trace-verify",
		RequestID:   "request-verify",
	}, types.ChallengeTypeEmailVerification, types.ChallengeRecord{
		ChallengeID: "challenge-email-1",
		TokenHash:   "verify-hash",
	}, issuedAt, expiresAt)
	if err != nil {
		t.Fatalf("create verification challenge: %v", err)
	}
	if verification.ChallengeID != "challenge-email-1" || verification.ExpiresAtUnixMS != expiresAt.UnixMilli() {
		t.Fatalf("unexpected verification challenge: %+v", verification)
	}
	_, err = repository.ConfirmVerificationChallenge(ctx, types.ConfirmVerificationChallengeCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		ChallengeID: "challenge-email-1",
	}, "wrong-hash", issuedAt.Add(time.Minute))
	if !errors.Is(err, types.ErrInvalidChallenge) {
		t.Fatalf("expected invalid challenge for wrong token, got %v", err)
	}
	confirmed, err := repository.ConfirmVerificationChallenge(ctx, types.ConfirmVerificationChallengeCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		ChallengeID: "challenge-email-1",
	}, "verify-hash", issuedAt.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("confirm verification challenge: %v", err)
	}
	if confirmed.Channel != types.VerificationChannelEmail || confirmed.Destination != "user1@example.com" {
		t.Fatalf("unexpected verification confirmation: %+v", confirmed)
	}
	assertEmailVerified(t, ctx, pool, "user1@example.com", true)

	reset, err := repository.CreatePasswordResetChallenge(ctx, types.RequestPasswordResetCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
	}, types.ChallengeRecord{
		ChallengeID: "challenge-reset-1",
		TokenHash:   "reset-hash",
	}, issuedAt.Add(3*time.Minute), issuedAt.Add(18*time.Minute))
	if err != nil {
		t.Fatalf("create password reset challenge: %v", err)
	}
	if reset.ChallengeID != "challenge-reset-1" {
		t.Fatalf("unexpected reset challenge: %+v", reset)
	}
	resetResult, err := repository.ConfirmPasswordReset(ctx, types.ConfirmPasswordResetCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		ChallengeID: "challenge-reset-1",
	}, "reset-hash", "new-password-hash", issuedAt.Add(4*time.Minute))
	if err != nil {
		t.Fatalf("confirm password reset: %v", err)
	}
	if resetResult.ResetAtUnixMS != issuedAt.Add(4*time.Minute).UnixMilli() {
		t.Fatalf("unexpected reset result: %+v", resetResult)
	}
	credential, err := repository.GetUserCredential(ctx, "tenant-identity", "user-1")
	if err != nil {
		t.Fatalf("get credential after reset: %v", err)
	}
	if credential.PasswordHash != "new-password-hash" {
		t.Fatalf("expected reset password hash, got %+v", credential)
	}
	assertSessionStatus(t, ctx, pool, "session-reset-1", "REVOKED")
	assertRefreshTokenStatus(t, ctx, pool, "rft_reset", "REVOKED")
	assertOutboxEvent(t, ctx, pool, "identity.session.revoked.v1", "identity_session", "event-password-reset-session-revoked-1")
	_, err = repository.ConfirmPasswordReset(ctx, types.ConfirmPasswordResetCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		ChallengeID: "challenge-reset-1",
	}, "reset-hash", "another-password-hash", issuedAt.Add(5*time.Minute))
	if !errors.Is(err, types.ErrInvalidChallenge) {
		t.Fatalf("expected consumed reset challenge to reject replay, got %v", err)
	}
}

func TestRepositoryPasswordResetChallengeRateLimitIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(pool)
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	if _, err := repository.RegisterUser(ctx, types.RegisterUserCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
	}, "password-hash", issuedAt); err != nil {
		t.Fatalf("register user: %v", err)
	}
	seedVerifiedEmail(t, ctx, pool, "user1@example.com", issuedAt)

	for i := 1; i <= 3; i++ {
		if _, err := repository.CreatePasswordResetChallenge(ctx, types.RequestPasswordResetCommand{
			TenantID:    "tenant-identity",
			UserID:      "user-1",
			Channel:     types.VerificationChannelEmail,
			Destination: "user1@example.com",
		}, types.ChallengeRecord{
			ChallengeID: types.ChallengeID(fmt.Sprintf("challenge-reset-limit-%d", i)),
			TokenHash:   fmt.Sprintf("reset-limit-hash-%d", i),
		}, issuedAt.Add(time.Duration(i)*time.Minute), issuedAt.Add(30*time.Minute)); err != nil {
			t.Fatalf("create reset challenge %d: %v", i, err)
		}
	}
	_, err := repository.CreatePasswordResetChallenge(ctx, types.RequestPasswordResetCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
	}, types.ChallengeRecord{
		ChallengeID: "challenge-reset-limit-4",
		TokenHash:   "reset-limit-hash-4",
	}, issuedAt.Add(4*time.Minute), issuedAt.Add(30*time.Minute))
	if !errors.Is(err, types.ErrChallengeRateLimited) {
		t.Fatalf("expected challenge rate limit, got %v", err)
	}
	if _, err := repository.ConfirmPasswordReset(ctx, types.ConfirmPasswordResetCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		ChallengeID: "challenge-reset-limit-1",
	}, "reset-limit-hash-1", "new-password-hash", issuedAt.Add(5*time.Minute)); err != nil {
		t.Fatalf("consume reset challenge: %v", err)
	}
	if _, err := repository.CreatePasswordResetChallenge(ctx, types.RequestPasswordResetCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
	}, types.ChallengeRecord{
		ChallengeID: "challenge-reset-limit-4",
		TokenHash:   "reset-limit-hash-4",
	}, issuedAt.Add(6*time.Minute), issuedAt.Add(30*time.Minute)); err != nil {
		t.Fatalf("create reset challenge after consuming one active challenge: %v", err)
	}
}

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
		KeyVersion: "local-v1",
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

func issueCommand(sessionID types.SessionID) types.IssueGatewayTokenCommand {
	return types.IssueGatewayTokenCommand{
		TenantID:  "tenant-identity",
		UserID:    "user-1",
		DeviceID:  "device-1",
		SessionID: sessionID,
		Audience:  "push-gateway",
		TraceID:   "trace-1",
		RequestID: "request-1",
	}
}

func seedUserCredential(t *testing.T, ctx context.Context, pool *pgxpool.Pool, passwordHash string) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO identity_users (tenant_id, user_id, status, password_hash, password_updated_at, created_at, updated_at)
VALUES ('tenant-identity', 'user-1', 'ACTIVE', $1, now(), now(), now())
ON CONFLICT (tenant_id, user_id) DO UPDATE
SET status = 'ACTIVE',
    password_hash = EXCLUDED.password_hash,
    password_updated_at = EXCLUDED.password_updated_at,
    updated_at = EXCLUDED.updated_at
`, passwordHash)
	if err != nil {
		t.Fatalf("seed user credential: %v", err)
	}
}

func seedVerifiedEmail(t *testing.T, ctx context.Context, pool *pgxpool.Pool, email string, verifiedAt time.Time) {
	t.Helper()
	_, err := pool.Exec(ctx, `
UPDATE identity_users
SET email = $3,
    email_verified_at = $4,
    updated_at = $4
WHERE tenant_id = $1
  AND user_id = $2
`, "tenant-identity", "user-1", email, verifiedAt)
	if err != nil {
		t.Fatalf("seed verified email: %v", err)
	}
}

func assertDeviceStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want string) {
	t.Helper()
	var got string
	err := pool.QueryRow(ctx, `
SELECT status
FROM identity_devices
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
  AND device_id = 'device-1'
`).Scan(&got)
	if err != nil {
		t.Fatalf("read device status: %v", err)
	}
	if got != want {
		t.Fatalf("expected device status %s, got %s", want, got)
	}
}

func assertSessionStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sessionID string, want string) {
	t.Helper()
	var got string
	err := pool.QueryRow(ctx, `
SELECT status
FROM identity_sessions
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
  AND device_id = 'device-1'
  AND session_id = $1
`, sessionID).Scan(&got)
	if err != nil {
		t.Fatalf("read session status: %v", err)
	}
	if got != want {
		t.Fatalf("expected session status %s, got %s", want, got)
	}
}

func assertRefreshTokenStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tokenID string, want string) {
	t.Helper()
	var got string
	err := pool.QueryRow(ctx, `
SELECT status
FROM identity_refresh_tokens
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
  AND device_id = 'device-1'
  AND token_id = $1
`, tokenID).Scan(&got)
	if err != nil {
		t.Fatalf("read refresh token status: %v", err)
	}
	if got != want {
		t.Fatalf("expected refresh token status %s, got %s", want, got)
	}
}

func assertRefreshTokenMissing(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tokenID string) {
	t.Helper()
	var count int
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM identity_refresh_tokens
WHERE token_id = $1
`, tokenID).Scan(&count)
	if err != nil {
		t.Fatalf("read refresh token count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected refresh token %s to be missing, got count %d", tokenID, count)
	}
}

func assertSessionMFAProof(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sessionID string, wantMethod string, wantFactorID types.MFAFactorID) {
	t.Helper()
	var verifiedAt time.Time
	var method string
	var factorID types.MFAFactorID
	err := pool.QueryRow(ctx, `
SELECT COALESCE(mfa_verified_at, 'epoch'::timestamptz), mfa_method, mfa_factor_id
FROM identity_sessions
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
  AND device_id = 'device-1'
  AND session_id = $1
`, sessionID).Scan(&verifiedAt, &method, &factorID)
	if err != nil {
		t.Fatalf("read session mfa proof: %v", err)
	}
	if method != wantMethod || factorID != wantFactorID {
		t.Fatalf("expected session mfa proof method=%q factor=%q, got method=%q factor=%q", wantMethod, wantFactorID, method, factorID)
	}
	emptyTime := time.Unix(0, 0).UTC()
	if wantMethod == "" && !verifiedAt.Equal(emptyTime) {
		t.Fatalf("expected empty mfa verified time, got %s", verifiedAt)
	}
	if wantMethod != "" && verifiedAt.Equal(emptyTime) {
		t.Fatal("expected mfa verified time to be set")
	}
}

func insertSessionProof(ctx context.Context, pool *pgxpool.Pool, sessionID string, method string, factorID string, verifiedAt any, now time.Time) error {
	_, err := pool.Exec(ctx, `
INSERT INTO identity_sessions (
    tenant_id,
    user_id,
    device_id,
    session_id,
    status,
    audience,
    issued_at,
    expires_at,
    mfa_verified_at,
    mfa_method,
    mfa_factor_id
) VALUES ('tenant-identity', 'user-1', 'device-1', $1, 'ACTIVE', 'push-gateway', $2, $3, $4, $5, $6)
`, sessionID, now, now.Add(15*time.Minute), verifiedAt, method, factorID)
	return err
}

func assertLoginRisk(t *testing.T, ctx context.Context, pool *pgxpool.Pool, wantFailedCount int, wantLocked bool) {
	t.Helper()
	var failedCount int
	var lockedUntil *time.Time
	err := pool.QueryRow(ctx, `
SELECT failed_login_count, locked_until
FROM identity_users
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
`).Scan(&failedCount, &lockedUntil)
	if err != nil {
		t.Fatalf("read login risk: %v", err)
	}
	if failedCount != wantFailedCount {
		t.Fatalf("expected failed login count %d, got %d", wantFailedCount, failedCount)
	}
	if wantLocked && lockedUntil == nil {
		t.Fatal("expected account to be locked")
	}
	if !wantLocked && lockedUntil != nil {
		t.Fatalf("expected account to be unlocked, got locked_until=%s", lockedUntil)
	}
}

func assertMFARecoveryLoginRisk(t *testing.T, ctx context.Context, pool *pgxpool.Pool, wantFailedCount int, wantLocked bool) {
	t.Helper()
	var failedCount int
	var lockedUntil *time.Time
	err := pool.QueryRow(ctx, `
SELECT mfa_recovery_failed_count, mfa_recovery_locked_until
FROM identity_users
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
`).Scan(&failedCount, &lockedUntil)
	if err != nil {
		t.Fatalf("read mfa recovery login risk: %v", err)
	}
	if failedCount != wantFailedCount {
		t.Fatalf("expected mfa recovery failed count %d, got %d", wantFailedCount, failedCount)
	}
	if wantLocked && lockedUntil == nil {
		t.Fatal("expected mfa recovery login to be locked")
	}
	if !wantLocked && lockedUntil != nil {
		t.Fatalf("expected mfa recovery login to be unlocked, got mfa_recovery_locked_until=%s", lockedUntil)
	}
}

func assertOutboxEvent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventType string, aggregateType string, eventID string) {
	t.Helper()
	var gotEventID, gotEventType, gotAggregateType, gotStatus string
	var payloadTenantID, payloadUserID, payloadDeviceID string
	err := pool.QueryRow(ctx, `
SELECT
    event_id,
    event_type,
    aggregate_type,
    status,
    payload_json->>'tenant_id',
    payload_json->>'user_id',
    payload_json->>'device_id'
FROM identity_outbox
WHERE event_id = $1
`, eventID).Scan(
		&gotEventID,
		&gotEventType,
		&gotAggregateType,
		&gotStatus,
		&payloadTenantID,
		&payloadUserID,
		&payloadDeviceID,
	)
	if err != nil {
		t.Fatalf("read identity outbox event: %v", err)
	}
	if gotEventID != eventID || gotEventType != eventType || gotAggregateType != aggregateType || gotStatus != "PENDING" {
		t.Fatalf("unexpected outbox event: id=%s type=%s aggregate=%s status=%s", gotEventID, gotEventType, gotAggregateType, gotStatus)
	}
	if payloadTenantID != "tenant-identity" || payloadUserID != "user-1" || payloadDeviceID != "device-1" {
		t.Fatalf("unexpected outbox payload tenant=%s user=%s device=%s", payloadTenantID, payloadUserID, payloadDeviceID)
	}
}

func assertEmailVerified(t *testing.T, ctx context.Context, pool *pgxpool.Pool, email string, wantVerified bool) {
	t.Helper()
	var gotEmail string
	var verified bool
	err := pool.QueryRow(ctx, `
SELECT email, email_verified_at IS NOT NULL
FROM identity_users
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
`).Scan(&gotEmail, &verified)
	if err != nil {
		t.Fatalf("read identity user email state: %v", err)
	}
	if gotEmail != email || verified != wantVerified {
		t.Fatalf("unexpected email state: email=%s verified=%v", gotEmail, verified)
	}
}

func assertMFALoginRisk(t *testing.T, ctx context.Context, pool *pgxpool.Pool, factorID string, wantFailedCount int, wantLocked bool, wantLastUsed bool) {
	t.Helper()
	var failedCount int
	var lockedUntil *time.Time
	var lastUsedAt *time.Time
	err := pool.QueryRow(ctx, `
SELECT login_failed_count, login_locked_until, last_used_at
FROM identity_mfa_factors
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
  AND factor_id = $1
`, factorID).Scan(&failedCount, &lockedUntil, &lastUsedAt)
	if err != nil {
		t.Fatalf("read mfa login risk: %v", err)
	}
	if failedCount != wantFailedCount {
		t.Fatalf("expected mfa failed count %d, got %d", wantFailedCount, failedCount)
	}
	if wantLocked && lockedUntil == nil {
		t.Fatal("expected mfa factor to be locked")
	}
	if !wantLocked && lockedUntil != nil {
		t.Fatalf("expected mfa factor to be unlocked, got login_locked_until=%s", lockedUntil)
	}
	if wantLastUsed && lastUsedAt == nil {
		t.Fatal("expected mfa factor last_used_at to be set")
	}
	if !wantLastUsed && lastUsedAt != nil {
		t.Fatalf("expected mfa factor last_used_at to be empty, got %s", lastUsedAt)
	}
}

func readMFASecret(t *testing.T, ctx context.Context, pool *pgxpool.Pool, factorID string) types.MFAFactorSecret {
	t.Helper()
	var row types.MFAFactorSecret
	err := pool.QueryRow(ctx, `
SELECT tenant_id, user_id, factor_id, factor_type, status, secret_ciphertext, secret_nonce, secret_key_version
FROM identity_mfa_factors
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
  AND factor_id = $1
`, factorID).Scan(
		&row.TenantID,
		&row.UserID,
		&row.FactorID,
		&row.Type,
		&row.Status,
		&row.Secret.Ciphertext,
		&row.Secret.Nonce,
		&row.Secret.KeyVersion,
	)
	if err != nil {
		t.Fatalf("read mfa secret: %v", err)
	}
	return row
}

func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	t.Cleanup(pool.Close)
	applyIdentityMigration(t, ctx, pool)
	return pool
}

func applyIdentityMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	root := findRepoRoot(t)
	migrationFiles, err := filepath.Glob(filepath.Join(root, "migrations", "postgres", "identity", "*.sql"))
	if err != nil {
		t.Fatalf("find migrations: %v", err)
	}
	for _, migrationPath := range migrationFiles {
		sqlBytes, err := os.ReadFile(migrationPath)
		if err != nil {
			t.Fatalf("read migration %s: %v", migrationPath, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("apply migration %s: %v", migrationPath, err)
		}
	}
}

func resetIdentityTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
TRUNCATE
    identity_mfa_recovery_codes,
    identity_mfa_factors,
    identity_challenges,
    identity_outbox,
    identity_refresh_tokens,
    identity_sessions,
    identity_devices,
    identity_users
RESTART IDENTITY
`)
	if err != nil {
		t.Fatalf("reset identity tables: %v", err)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("repo root not found")
		}
		wd = parent
	}
}
