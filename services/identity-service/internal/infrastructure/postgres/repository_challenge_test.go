package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

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
	}, types.ChallengeDeliveryRecord{}, issuedAt, expiresAt)
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
	}, types.ChallengeDeliveryRecord{}, issuedAt.Add(3*time.Minute), issuedAt.Add(18*time.Minute))
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

func TestRepositoryChallengeDeliveryStatusIntegration(t *testing.T) {
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
	verification, err := repository.CreateVerificationChallenge(ctx, types.RequestVerificationChallengeCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
	}, types.ChallengeTypeEmailVerification, types.ChallengeRecord{
		ChallengeID: "challenge-delivery-success",
		TokenHash:   "delivery-success-hash",
	}, types.ChallengeDeliveryRecord{}, issuedAt, issuedAt.Add(15*time.Minute))
	if err != nil {
		t.Fatalf("create verification challenge: %v", err)
	}
	if err := repository.RecordChallengeDeliverySuccess(ctx, verification.TenantID, verification.UserID, verification.ChallengeID, issuedAt.Add(time.Second)); err != nil {
		t.Fatalf("record delivery success: %v", err)
	}
	successState := readChallengeDeliveryState(t, ctx, pool, "challenge-delivery-success")
	if successState.Status != "ACTIVE" ||
		successState.DeliveryStatus != "DELIVERED" ||
		successState.DeliveryAttemptCount != 1 ||
		successState.DeliveredAt == nil ||
		successState.DeliveryFailedAt != nil ||
		successState.DeliveryLastError != "" ||
		successState.DeliveryFailureClass != "" {
		t.Fatalf("unexpected delivered challenge state: %+v", successState)
	}

	seedVerifiedEmail(t, ctx, pool, "user1@example.com", issuedAt)
	reset, err := repository.CreatePasswordResetChallenge(ctx, types.RequestPasswordResetCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
	}, types.ChallengeRecord{
		ChallengeID: "challenge-delivery-failed",
		TokenHash:   "delivery-failed-hash",
	}, types.ChallengeDeliveryRecord{}, issuedAt.Add(time.Minute), issuedAt.Add(16*time.Minute))
	if err != nil {
		t.Fatalf("create reset challenge: %v", err)
	}
	rawProviderError := "provider returned non-success status 500 body=user1@example.com token=secret-token"
	if err := repository.RecordChallengeDeliveryFailure(ctx, reset.TenantID, reset.UserID, reset.ChallengeID, rawProviderError, issuedAt.Add(2*time.Minute)); err != nil {
		t.Fatalf("record delivery failure: %v", err)
	}
	failedState := readChallengeDeliveryState(t, ctx, pool, "challenge-delivery-failed")
	if failedState.Status != "EXPIRED" ||
		failedState.DeliveryStatus != "FAILED" ||
		failedState.DeliveryAttemptCount != 1 ||
		failedState.DeliveryFailedAt == nil ||
		failedState.DeliveryLastError != "challenge delivery provider returned non-success status" ||
		failedState.DeliveryFailureClass != types.ChallengeDeliveryFailureClassProviderNonSuccess {
		t.Fatalf("unexpected failed challenge state: %+v", failedState)
	}
	if strings.Contains(failedState.DeliveryLastError, "user1@example.com") ||
		strings.Contains(failedState.DeliveryLastError, "secret-token") ||
		strings.Contains(failedState.DeliveryLastError, "smtp body") {
		t.Fatalf("delivery last error leaked provider text: %q", failedState.DeliveryLastError)
	}
	_, err = repository.ConfirmPasswordReset(ctx, types.ConfirmPasswordResetCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		ChallengeID: "challenge-delivery-failed",
	}, "delivery-failed-hash", "new-password-hash", issuedAt.Add(3*time.Minute))
	if !errors.Is(err, types.ErrInvalidChallenge) {
		t.Fatalf("expected failed delivery challenge to reject confirmation, got %v", err)
	}
}

func TestSanitizeChallengeDeliveryErrorUsesStablePublicMessages(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		want      string
		wantClass string
	}{
		{
			name:      "provider body",
			raw:       "provider returned non-success status 500 body=user1@example.com token=secret-token",
			want:      "challenge delivery provider returned non-success status",
			wantClass: types.ChallengeDeliveryFailureClassProviderNonSuccess,
		},
		{
			name:      "network details",
			raw:       "dial tcp 10.0.0.8:25: connection refused for user1@example.com",
			want:      "challenge delivery network failed",
			wantClass: types.ChallengeDeliveryFailureClassNetwork,
		},
		{
			name:      "serialization details",
			raw:       "json marshal failed for template user=user1@example.com",
			want:      "challenge delivery json serialization failed",
			wantClass: types.ChallengeDeliveryFailureClassSerialization,
		},
		{
			name:      "crypto details",
			raw:       "decrypt token ciphertext failed nonce=secret-nonce",
			want:      "challenge delivery token decrypt failed",
			wantClass: types.ChallengeDeliveryFailureClassTokenCrypto,
		},
		{
			name:      "unknown raw provider text",
			raw:       "smtp provider said raw body user=user1@example.com token=secret-token",
			want:      "challenge delivery unavailable",
			wantClass: types.ChallengeDeliveryFailureClassDeliveryFailed,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeChallengeDeliveryError(tc.raw)
			if got != tc.want {
				t.Fatalf("sanitizeChallengeDeliveryError(%q) = %q, want %q", tc.raw, got, tc.want)
			}
			if gotClass := types.ClassifyChallengeDeliveryFailureMessage(got, true); gotClass != tc.wantClass {
				t.Fatalf("classify sanitized challenge delivery error %q = %q, want %q", got, gotClass, tc.wantClass)
			}
			for _, leaked := range []string{"user1@example.com", "secret-token", "secret-nonce", "10.0.0.8"} {
				if strings.Contains(got, leaked) {
					t.Fatalf("sanitized challenge delivery error leaked %q in %q", leaked, got)
				}
			}
		})
	}
}

func TestRepositoryChallengeDeliveryOutboxIntegration(t *testing.T) {
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

	_, err := repository.CreateVerificationChallenge(ctx, types.RequestVerificationChallengeCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
		TraceID:     "trace-outbox",
		RequestID:   "request-outbox",
	}, types.ChallengeTypeEmailVerification, types.ChallengeRecord{
		ChallengeID: "challenge-delivery-outbox",
		TokenHash:   "hash-raw-token",
	}, types.ChallengeDeliveryRecord{
		EncryptedToken: types.EncryptedChallengeToken{
			Ciphertext: "encrypted-token",
			Nonce:      "nonce-value",
			KeyVersion: "v2",
		},
	}, issuedAt, issuedAt.Add(15*time.Minute))
	if err != nil {
		t.Fatalf("create verification challenge with delivery outbox: %v", err)
	}

	var status, challengeType, channel, destination, ciphertext, nonce, keyVersion, traceID, requestID string
	var expiresAt time.Time
	err = pool.QueryRow(ctx, `
SELECT
    status,
    challenge_type,
    channel,
    destination,
    token_ciphertext,
    token_nonce,
    token_key_version,
    expires_at,
    trace_id,
    request_id
FROM identity_challenge_delivery_outbox
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
  AND challenge_id = 'challenge-delivery-outbox'
`).Scan(&status, &challengeType, &channel, &destination, &ciphertext, &nonce, &keyVersion, &expiresAt, &traceID, &requestID)
	if err != nil {
		t.Fatalf("read delivery outbox: %v", err)
	}
	if status != "PENDING" ||
		challengeType != string(types.ChallengeTypeEmailVerification) ||
		channel != string(types.VerificationChannelEmail) ||
		destination != "user1@example.com" ||
		ciphertext != "encrypted-token" ||
		nonce != "nonce-value" ||
		keyVersion != "v2" ||
		traceID != "trace-outbox" ||
		requestID != "request-outbox" ||
		!expiresAt.Equal(issuedAt.Add(15*time.Minute)) {
		t.Fatalf("unexpected delivery outbox row: status=%s type=%s channel=%s destination=%s ciphertext=%s nonce=%s key=%s expires=%s trace=%s request=%s",
			status, challengeType, channel, destination, ciphertext, nonce, keyVersion, expiresAt, traceID, requestID)
	}

	var tokenHash string
	if err := pool.QueryRow(ctx, `
SELECT token_hash
FROM identity_challenges
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
  AND challenge_id = 'challenge-delivery-outbox'
`).Scan(&tokenHash); err != nil {
		t.Fatalf("read challenge token hash: %v", err)
	}
	if tokenHash != "hash-raw-token" || ciphertext == "raw-token" {
		t.Fatalf("unexpected raw token boundary: hash=%q ciphertext=%q", tokenHash, ciphertext)
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
		}, types.ChallengeDeliveryRecord{}, issuedAt.Add(time.Duration(i)*time.Minute), issuedAt.Add(30*time.Minute)); err != nil {
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
	}, types.ChallengeDeliveryRecord{}, issuedAt.Add(4*time.Minute), issuedAt.Add(30*time.Minute))
	if !errors.Is(err, types.ErrChallengeRateLimited) {
		t.Fatalf("expected challenge rate limit, got %v", err)
	}
	if err := repository.ExpireChallenge(ctx, "tenant-identity", "user-1", "challenge-reset-limit-1", issuedAt.Add(5*time.Minute)); err != nil {
		t.Fatalf("expire reset challenge after delivery failure: %v", err)
	}
	_, err = repository.ConfirmPasswordReset(ctx, types.ConfirmPasswordResetCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		ChallengeID: "challenge-reset-limit-1",
	}, "reset-limit-hash-1", "new-password-hash", issuedAt.Add(5*time.Minute))
	if !errors.Is(err, types.ErrInvalidChallenge) {
		t.Fatalf("expected expired reset challenge to reject confirmation, got %v", err)
	}
	if _, err := repository.CreatePasswordResetChallenge(ctx, types.RequestPasswordResetCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
	}, types.ChallengeRecord{
		ChallengeID: "challenge-reset-limit-4",
		TokenHash:   "reset-limit-hash-4",
	}, types.ChallengeDeliveryRecord{}, issuedAt.Add(6*time.Minute), issuedAt.Add(30*time.Minute)); err != nil {
		t.Fatalf("create reset challenge after consuming one active challenge: %v", err)
	}
}

func TestRepositoryChallengeRequestWindowRateLimitIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(pool, WithChallengeRequestLimit(5, 15*time.Minute))
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	if _, err := repository.RegisterUser(ctx, types.RegisterUserCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
	}, "password-hash", issuedAt); err != nil {
		t.Fatalf("register user: %v", err)
	}
	seedVerifiedEmail(t, ctx, pool, "user1@example.com", issuedAt)

	for i := 0; i < 5; i++ {
		challengeID := types.ChallengeID(fmt.Sprintf("challenge-reset-window-%d", i))
		if _, err := repository.CreatePasswordResetChallenge(ctx, types.RequestPasswordResetCommand{
			TenantID:    "tenant-identity",
			UserID:      "user-1",
			Channel:     types.VerificationChannelEmail,
			Destination: "user1@example.com",
		}, types.ChallengeRecord{
			ChallengeID: challengeID,
			TokenHash:   fmt.Sprintf("reset-window-hash-%d", i),
		}, types.ChallengeDeliveryRecord{}, issuedAt.Add(time.Duration(i)*time.Minute), issuedAt.Add(30*time.Minute)); err != nil {
			t.Fatalf("create reset challenge %d: %v", i, err)
		}
		if err := repository.ExpireChallenge(ctx, "tenant-identity", "user-1", challengeID, issuedAt.Add(time.Duration(i)*time.Minute+30*time.Second)); err != nil {
			t.Fatalf("expire reset challenge %d: %v", i, err)
		}
	}

	_, err := repository.CreatePasswordResetChallenge(ctx, types.RequestPasswordResetCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
	}, types.ChallengeRecord{
		ChallengeID: "challenge-reset-window-limited",
		TokenHash:   "reset-window-hash-limited",
	}, types.ChallengeDeliveryRecord{}, issuedAt.Add(6*time.Minute), issuedAt.Add(30*time.Minute))
	if !errors.Is(err, types.ErrChallengeRateLimited) {
		t.Fatalf("expected recent challenge rate limit, got %v", err)
	}

	if _, err := repository.CreatePasswordResetChallenge(ctx, types.RequestPasswordResetCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
	}, types.ChallengeRecord{
		ChallengeID: "challenge-reset-window-after",
		TokenHash:   "reset-window-hash-after",
	}, types.ChallengeDeliveryRecord{}, issuedAt.Add(20*time.Minute), issuedAt.Add(50*time.Minute)); err != nil {
		t.Fatalf("create reset challenge after request window: %v", err)
	}
}

func TestRepositoryVerificationChallengeRequestWindowRateLimitIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(pool, WithChallengeRequestLimit(5, 15*time.Minute))
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	if _, err := repository.RegisterUser(ctx, types.RegisterUserCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
	}, "password-hash", issuedAt); err != nil {
		t.Fatalf("register user: %v", err)
	}

	for i := 0; i < 5; i++ {
		challengeID := types.ChallengeID(fmt.Sprintf("challenge-email-window-%d", i))
		if _, err := repository.CreateVerificationChallenge(ctx, types.RequestVerificationChallengeCommand{
			TenantID:    "tenant-identity",
			UserID:      "user-1",
			Channel:     types.VerificationChannelEmail,
			Destination: "user1@example.com",
		}, types.ChallengeTypeEmailVerification, types.ChallengeRecord{
			ChallengeID: challengeID,
			TokenHash:   fmt.Sprintf("email-window-hash-%d", i),
		}, types.ChallengeDeliveryRecord{}, issuedAt.Add(time.Duration(i)*time.Minute), issuedAt.Add(30*time.Minute)); err != nil {
			t.Fatalf("create verification challenge %d: %v", i, err)
		}
		if err := repository.ExpireChallenge(ctx, "tenant-identity", "user-1", challengeID, issuedAt.Add(time.Duration(i)*time.Minute+30*time.Second)); err != nil {
			t.Fatalf("expire verification challenge %d: %v", i, err)
		}
	}

	_, err := repository.CreateVerificationChallenge(ctx, types.RequestVerificationChallengeCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
	}, types.ChallengeTypeEmailVerification, types.ChallengeRecord{
		ChallengeID: "challenge-email-window-limited",
		TokenHash:   "email-window-hash-limited",
	}, types.ChallengeDeliveryRecord{}, issuedAt.Add(6*time.Minute), issuedAt.Add(30*time.Minute))
	if !errors.Is(err, types.ErrChallengeRateLimited) {
		t.Fatalf("expected recent verification challenge rate limit, got %v", err)
	}

	if _, err := repository.CreateVerificationChallenge(ctx, types.RequestVerificationChallengeCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
	}, types.ChallengeTypeEmailVerification, types.ChallengeRecord{
		ChallengeID: "challenge-email-window-after",
		TokenHash:   "email-window-hash-after",
	}, types.ChallengeDeliveryRecord{}, issuedAt.Add(20*time.Minute), issuedAt.Add(50*time.Minute)); err != nil {
		t.Fatalf("create verification challenge after request window: %v", err)
	}
}

func TestRepositoryPasswordResetRequestLimiterHashesInvalidTargetIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(
		pool,
		WithChallengeRequestLimit(2, 10*time.Minute),
		WithChallengeRequestLockDuration(20*time.Minute),
	)
	requestedAt := time.Unix(1_800_000_000, 0).UTC()
	command := types.RequestPasswordResetCommand{
		TenantID:    "tenant-identity",
		UserID:      "missing-user",
		Channel:     types.VerificationChannelEmail,
		Destination: "User1@Example.COM",
	}
	targetKey := strings.Repeat("a", 64)

	for i := 0; i < 2; i++ {
		if err := repository.RecordPasswordResetRequest(ctx, command.TenantID, command.UserID, command.Channel, targetKey, requestedAt.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatalf("record password reset request %d: %v", i, err)
		}
	}
	err := repository.RecordPasswordResetRequest(ctx, command.TenantID, command.UserID, command.Channel, targetKey, requestedAt.Add(2*time.Minute))
	if !errors.Is(err, types.ErrChallengeRateLimited) {
		t.Fatalf("expected password reset request limiter, got %v", err)
	}

	var storedKey string
	var requestCount int
	var lockedUntil *time.Time
	if err := pool.QueryRow(ctx, `
SELECT target_key, request_count, locked_until
FROM identity_challenge_request_limits
WHERE tenant_id = $1
  AND user_id = $2
  AND challenge_type = $3
  AND channel = $4
`, command.TenantID, command.UserID, types.ChallengeTypePasswordReset, command.Channel).Scan(&storedKey, &requestCount, &lockedUntil); err != nil {
		t.Fatalf("read password reset request limiter row: %v", err)
	}
	if storedKey != targetKey ||
		requestCount != 3 ||
		lockedUntil == nil ||
		!lockedUntil.Equal(requestedAt.Add(22*time.Minute)) {
		t.Fatalf("unexpected limiter row key=%q count=%d locked=%v", storedKey, requestCount, lockedUntil)
	}
	if strings.Contains(storedKey, "User1") || strings.Contains(storedKey, "example.com") {
		t.Fatalf("limiter stored raw destination in target key: %q", storedKey)
	}

	err = repository.RecordPasswordResetRequest(ctx, command.TenantID, command.UserID, command.Channel, targetKey, requestedAt.Add(3*time.Minute))
	if !errors.Is(err, types.ErrChallengeRateLimited) {
		t.Fatalf("expected locked password reset request limiter, got %v", err)
	}
	if err := repository.RecordPasswordResetRequest(ctx, command.TenantID, command.UserID, command.Channel, targetKey, requestedAt.Add(25*time.Minute)); err != nil {
		t.Fatalf("expected limiter to reset after lock/window expiry: %v", err)
	}
}

func TestRepositoryPasswordResetRequestLimiterConcurrentFirstRequestIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(
		pool,
		WithChallengeRequestLimit(16, 10*time.Minute),
		WithChallengeRequestLockDuration(20*time.Minute),
	)
	requestedAt := time.Unix(1_800_000_000, 0).UTC()
	tenantID := types.TenantID("tenant-identity")
	userID := types.UserID("missing-user")
	channel := types.VerificationChannelEmail
	targetKey := strings.Repeat("b", 64)

	const workerCount = 8
	errCh := make(chan error, workerCount)
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			errCh <- repository.RecordPasswordResetRequest(ctx, tenantID, userID, channel, targetKey, requestedAt.Add(time.Duration(offset)*time.Millisecond))
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent first request should not fail: %v", err)
		}
	}

	var rowCount int
	var requestCount int
	if err := pool.QueryRow(ctx, `
SELECT count(*), COALESCE(MAX(request_count), 0)
FROM identity_challenge_request_limits
WHERE tenant_id = $1
  AND user_id = $2
  AND challenge_type = $3
  AND channel = $4
  AND target_key = $5
`, tenantID, userID, types.ChallengeTypePasswordReset, channel, targetKey).Scan(&rowCount, &requestCount); err != nil {
		t.Fatalf("read concurrent limiter row: %v", err)
	}
	if rowCount != 1 || requestCount != workerCount {
		t.Fatalf("unexpected concurrent limiter state rows=%d request_count=%d", rowCount, requestCount)
	}
}

func TestRepositoryCleanupChallengeRequestLimitsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(pool)
	now := time.Unix(1_800_000_000, 0).UTC()
	cutoff := now.Add(-24 * time.Hour)
	rows := []struct {
		userID        string
		targetKey     string
		lastRequestAt time.Time
		lockedUntil   any
	}{
		{userID: "stale-unlocked", targetKey: strings.Repeat("c", 64), lastRequestAt: cutoff.Add(-time.Hour), lockedUntil: nil},
		{userID: "stale-expired-lock", targetKey: strings.Repeat("d", 64), lastRequestAt: cutoff.Add(-2 * time.Hour), lockedUntil: cutoff.Add(-time.Minute)},
		{userID: "recent", targetKey: strings.Repeat("e", 64), lastRequestAt: cutoff.Add(time.Minute), lockedUntil: nil},
		{userID: "active-lock", targetKey: strings.Repeat("f", 64), lastRequestAt: cutoff.Add(-time.Hour), lockedUntil: now.Add(time.Hour)},
	}
	for _, row := range rows {
		if _, err := pool.Exec(ctx, `
INSERT INTO identity_challenge_request_limits (
    tenant_id,
    user_id,
    challenge_type,
    channel,
    target_key,
    request_count,
    window_start,
    last_request_at,
    locked_until,
    created_at,
    updated_at
) VALUES ($1, $2, $3, $4, $5, 3, $6, $7, $8, $7, $7)
`, "tenant-identity", row.userID, types.ChallengeTypePasswordReset, types.VerificationChannelEmail, row.targetKey, row.lastRequestAt.Add(-time.Minute), row.lastRequestAt, row.lockedUntil); err != nil {
			t.Fatalf("seed limiter row %s: %v", row.userID, err)
		}
	}

	deleted, err := repository.CleanupChallengeRequestLimits(ctx, cutoff, 100, false)
	if err != nil {
		t.Fatalf("cleanup challenge request limits: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected 2 deleted rows, got %d", deleted)
	}

	var remaining []string
	resultRows, err := pool.Query(ctx, `
SELECT user_id
FROM identity_challenge_request_limits
WHERE tenant_id = $1
ORDER BY user_id
`, "tenant-identity")
	if err != nil {
		t.Fatalf("read remaining limiter rows: %v", err)
	}
	defer resultRows.Close()
	for resultRows.Next() {
		var userID string
		if err := resultRows.Scan(&userID); err != nil {
			t.Fatalf("scan remaining limiter row: %v", err)
		}
		remaining = append(remaining, userID)
	}
	if err := resultRows.Err(); err != nil {
		t.Fatalf("iterate remaining limiter rows: %v", err)
	}
	if strings.Join(remaining, ",") != "active-lock,recent" {
		t.Fatalf("unexpected remaining limiter rows: %v", remaining)
	}
}

func TestRepositoryCleanupChallengeRequestLimitsDryRunDoesNotDeleteIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(pool)
	now := time.Unix(1_800_000_000, 0).UTC()
	cutoff := now.Add(-24 * time.Hour)
	rows := []struct {
		userID        string
		targetKey     string
		lastRequestAt time.Time
		lockedUntil   any
	}{
		{userID: "dry-run-stale-unlocked", targetKey: strings.Repeat("a", 64), lastRequestAt: cutoff.Add(-time.Hour), lockedUntil: nil},
		{userID: "dry-run-stale-expired-lock", targetKey: strings.Repeat("b", 64), lastRequestAt: cutoff.Add(-2 * time.Hour), lockedUntil: cutoff.Add(-time.Minute)},
		{userID: "dry-run-recent", targetKey: strings.Repeat("c", 64), lastRequestAt: cutoff.Add(time.Minute), lockedUntil: nil},
	}
	for _, row := range rows {
		if _, err := pool.Exec(ctx, `
INSERT INTO identity_challenge_request_limits (
    tenant_id,
    user_id,
    challenge_type,
    channel,
    target_key,
    request_count,
    window_start,
    last_request_at,
    locked_until,
    created_at,
    updated_at
) VALUES ($1, $2, $3, $4, $5, 3, $6, $7, $8, $7, $7)
`, "tenant-identity-dry-run", row.userID, types.ChallengeTypePasswordReset, types.VerificationChannelEmail, row.targetKey, row.lastRequestAt.Add(-time.Minute), row.lastRequestAt, row.lockedUntil); err != nil {
			t.Fatalf("seed dry-run limiter row %s: %v", row.userID, err)
		}
	}

	deleted, err := repository.CleanupChallengeRequestLimits(ctx, cutoff, 100, true)
	if err != nil {
		t.Fatalf("dry-run cleanup challenge request limits: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected 2 dry-run deleted rows, got %d", deleted)
	}

	var remaining int
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM identity_challenge_request_limits
WHERE tenant_id = $1
`, "tenant-identity-dry-run").Scan(&remaining); err != nil {
		t.Fatalf("count dry-run limiter rows: %v", err)
	}
	if remaining != 3 {
		t.Fatalf("expected dry-run to keep 3 limiter rows, got %d", remaining)
	}
}
