package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestChallengeDeliveryStoreDeliversIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(pool)
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	seedChallengeDeliveryOutbox(t, ctx, repository, issuedAt, "challenge-deliver", issuedAt.Add(15*time.Minute))

	store := NewChallengeDeliveryStore(pool, WithChallengeDeliveryClock(func() time.Time { return issuedAt.Add(time.Minute) }))
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Second, func(_ context.Context, messages []types.ChallengeDeliveryMessage) []error {
		if len(messages) != 1 {
			t.Fatalf("expected one delivery message, got %d", len(messages))
		}
		message := messages[0]
		if message.ChallengeID != "challenge-deliver" ||
			message.EncryptedToken.Ciphertext != "encrypted-challenge-deliver" ||
			message.TraceID != "trace-challenge-deliver" ||
			message.RequestID != "request-challenge-deliver" {
			t.Fatalf("unexpected delivery message: %+v", message)
		}
		return []error{nil}
	})
	if err != nil {
		t.Fatalf("process ready delivery: %v", err)
	}
	if stats.Fetched != 1 || stats.Delivered != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	assertChallengeDeliveryOutboxStatus(t, ctx, pool, "challenge-deliver", "DELIVERED", 0)
	state := readChallengeDeliveryState(t, ctx, pool, "challenge-deliver")
	if state.Status != "ACTIVE" || state.DeliveryStatus != "DELIVERED" || state.DeliveryAttemptCount != 1 {
		t.Fatalf("unexpected delivered challenge state: %+v", state)
	}
}

func TestChallengeDeliveryStoreDeadLettersIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(pool)
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	seedChallengeDeliveryOutbox(t, ctx, repository, issuedAt, "challenge-dlq", issuedAt.Add(15*time.Minute))

	store := NewChallengeDeliveryStore(pool, WithChallengeDeliveryClock(func() time.Time { return issuedAt.Add(time.Minute) }))
	stats, err := store.ProcessReadyBatch(ctx, 10, 1, time.Second, func(_ context.Context, messages []types.ChallengeDeliveryMessage) []error {
		if len(messages) != 1 {
			t.Fatalf("expected one delivery message, got %d", len(messages))
		}
		return []error{types.NewChallengeDeliveryFailed("provider unavailable")}
	})
	if err != nil {
		t.Fatalf("process failed delivery: %v", err)
	}
	if stats.Fetched != 1 || stats.DeadLettered != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	assertChallengeDeliveryOutboxStatus(t, ctx, pool, "challenge-dlq", "DLQ", 1)
	state := readChallengeDeliveryState(t, ctx, pool, "challenge-dlq")
	if state.Status != "EXPIRED" || state.DeliveryStatus != "FAILED" || state.DeliveryAttemptCount != 1 || state.DeliveryFailedAt == nil {
		t.Fatalf("unexpected dlq challenge state: %+v", state)
	}
	_, err = repository.ConfirmVerificationChallenge(ctx, types.ConfirmVerificationChallengeCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		ChallengeID: "challenge-dlq",
	}, "delivery-token-hash", issuedAt.Add(2*time.Minute))
	if !errors.Is(err, types.ErrInvalidChallenge) {
		t.Fatalf("expected dlq challenge to reject confirmation, got %v", err)
	}
}

func TestChallengeDeliveryStoreCancelsExpiredIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(pool)
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	seedChallengeDeliveryOutbox(t, ctx, repository, issuedAt, "challenge-expired", issuedAt.Add(time.Minute))

	callbackCalled := false
	store := NewChallengeDeliveryStore(pool, WithChallengeDeliveryClock(func() time.Time { return issuedAt.Add(2 * time.Minute) }))
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Second, func(_ context.Context, messages []types.ChallengeDeliveryMessage) []error {
		callbackCalled = true
		return make([]error, len(messages))
	})
	if err != nil {
		t.Fatalf("process expired delivery: %v", err)
	}
	if callbackCalled {
		t.Fatal("expired challenge must not be delivered")
	}
	if stats.Canceled != 1 || stats.Fetched != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	assertChallengeDeliveryOutboxStatus(t, ctx, pool, "challenge-expired", "CANCELED", 0)
	state := readChallengeDeliveryState(t, ctx, pool, "challenge-expired")
	if state.Status != "EXPIRED" || state.DeliveryStatus != "FAILED" {
		t.Fatalf("unexpected canceled challenge state: %+v", state)
	}
}

func seedChallengeDeliveryOutbox(t *testing.T, ctx context.Context, repository *Repository, issuedAt time.Time, challengeID types.ChallengeID, expiresAt time.Time) {
	t.Helper()
	if _, err := repository.RegisterUser(ctx, types.RegisterUserCommand{
		TenantID: "tenant-identity",
		UserID:   "user-1",
	}, "password-hash", issuedAt); err != nil {
		t.Fatalf("register user: %v", err)
	}
	if _, err := repository.CreateVerificationChallenge(ctx, types.RequestVerificationChallengeCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
		TraceID:     "trace-" + string(challengeID),
		RequestID:   "request-" + string(challengeID),
	}, types.ChallengeTypeEmailVerification, types.ChallengeRecord{
		ChallengeID: challengeID,
		TokenHash:   "delivery-token-hash",
	}, types.ChallengeDeliveryRecord{
		EncryptedToken: types.EncryptedChallengeToken{
			Ciphertext: "encrypted-" + string(challengeID),
			Nonce:      "nonce-" + string(challengeID),
			KeyVersion: "local-v1",
		},
	}, issuedAt, expiresAt); err != nil {
		t.Fatalf("create challenge delivery outbox: %v", err)
	}
}

func assertChallengeDeliveryOutboxStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, challengeID string, wantStatus string, wantRetryCount int) {
	t.Helper()
	var gotStatus string
	var gotRetryCount int
	err := pool.QueryRow(ctx, `
SELECT status, retry_count
FROM identity_challenge_delivery_outbox
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
  AND challenge_id = $1
`, challengeID).Scan(&gotStatus, &gotRetryCount)
	if err != nil {
		t.Fatalf("read challenge delivery outbox status: %v", err)
	}
	if gotStatus != wantStatus || gotRetryCount != wantRetryCount {
		t.Fatalf("expected delivery outbox status=%s retry=%d, got status=%s retry=%d", wantStatus, wantRetryCount, gotStatus, gotRetryCount)
	}
}
