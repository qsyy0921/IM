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
			message.EncryptedToken.KeyVersion != "v2" ||
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
	if state.Status != "EXPIRED" ||
		state.DeliveryStatus != "FAILED" ||
		state.DeliveryAttemptCount != 1 ||
		state.DeliveryFailedAt == nil ||
		state.DeliveryFailureClass != types.ChallengeDeliveryFailureClassDeliveryFailed {
		t.Fatalf("unexpected dlq challenge state: %+v", state)
	}
	assertChallengeDeliveryOutboxFailureClass(t, ctx, pool, "challenge-dlq", types.ChallengeDeliveryFailureClassDeliveryFailed)
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
	if state.Status != "EXPIRED" ||
		state.DeliveryStatus != "FAILED" ||
		state.DeliveryFailureClass != types.ChallengeDeliveryFailureClassInactive {
		t.Fatalf("unexpected canceled challenge state: %+v", state)
	}
	assertChallengeDeliveryOutboxFailureClass(t, ctx, pool, "challenge-expired", types.ChallengeDeliveryFailureClassInactive)
}

func TestChallengeDeliveryStoreRepairAuditsDLQWithoutReactivationIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(pool)
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	seedChallengeDeliveryOutbox(t, ctx, repository, issuedAt, "challenge-repair-dlq", issuedAt.Add(15*time.Minute))

	store := NewChallengeDeliveryStore(pool, WithChallengeDeliveryClock(func() time.Time { return issuedAt.Add(time.Minute) }))
	stats, err := store.ProcessReadyBatch(ctx, 10, 1, time.Second, func(_ context.Context, messages []types.ChallengeDeliveryMessage) []error {
		return []error{types.NewChallengeDeliveryFailed("provider unavailable")}
	})
	if err != nil {
		t.Fatalf("process failed delivery: %v", err)
	}
	if stats.DeadLettered != 1 {
		t.Fatalf("expected one dead-lettered delivery, got %+v", stats)
	}
	deliveryID := readChallengeDeliveryOutboxID(t, ctx, pool, "challenge-repair-dlq")

	repairStats, err := store.RepairDeliveries(ctx, types.ChallengeDeliveryRepairOptions{
		DeliveryIDs: []int64{deliveryID, deliveryID, 0},
		Mode:        types.ChallengeDeliveryRepairModeAudit,
		Operator:    "operator-1",
		Reason:      "provider recovered",
	})
	if err != nil {
		t.Fatalf("audit dlq delivery: %v", err)
	}
	if repairStats.Requested != 1 || repairStats.Audited != 1 || repairStats.Mutated != 0 || repairStats.Skipped != 0 {
		t.Fatalf("unexpected audit stats: %+v", repairStats)
	}
	assertChallengeDeliveryOutboxStatus(t, ctx, pool, "challenge-repair-dlq", "DLQ", 1)
	state := readChallengeDeliveryState(t, ctx, pool, "challenge-repair-dlq")
	if state.Status != "EXPIRED" || state.DeliveryStatus != "FAILED" {
		t.Fatalf("audit must not change dlq challenge state: %+v", state)
	}
	assertChallengeDeliveryRepairAudit(t, ctx, pool, deliveryID, "audit", "AUDITED", "", "DLQ", "EXPIRED", "FAILED", 1, "challenge delivery failed: provider unavailable", types.ChallengeDeliveryFailureClassDeliveryFailed, "DLQ", "EXPIRED", "FAILED", types.ChallengeDeliveryFailureClassDeliveryFailed, "provider recovered")

	repairStats, err = store.RepairDeliveries(ctx, types.ChallengeDeliveryRepairOptions{
		DeliveryIDs: []int64{deliveryID},
		Mode:        types.ChallengeDeliveryRepairModeRedriveActivePending,
		Operator:    "operator-1",
		Reason:      "provider recovered",
	})
	if err != nil {
		t.Fatalf("redrive dlq delivery: %v", err)
	}
	if repairStats.Requested != 1 || repairStats.Audited != 0 || repairStats.Mutated != 0 || repairStats.Skipped != 1 {
		t.Fatalf("unexpected dlq redrive stats: %+v", repairStats)
	}
	assertChallengeDeliveryOutboxStatus(t, ctx, pool, "challenge-repair-dlq", "DLQ", 1)
	state = readChallengeDeliveryState(t, ctx, pool, "challenge-repair-dlq")
	if state.Status != "EXPIRED" || state.DeliveryStatus != "FAILED" {
		t.Fatalf("dlq redrive must not reactivate challenge: %+v", state)
	}
	assertChallengeDeliveryRepairAudit(t, ctx, pool, deliveryID, "redrive-active-pending", "SKIPPED", "dlq_requires_new_challenge", "DLQ", "EXPIRED", "FAILED", 1, "challenge delivery failed: provider unavailable", types.ChallengeDeliveryFailureClassDeliveryFailed, "DLQ", "EXPIRED", "FAILED", types.ChallengeDeliveryFailureClassDeliveryFailed, "provider recovered")
	_, err = repository.ConfirmVerificationChallenge(ctx, types.ConfirmVerificationChallengeCommand{
		TenantID:    "tenant-identity",
		UserID:      "user-1",
		ChallengeID: "challenge-repair-dlq",
	}, "delivery-token-hash", issuedAt.Add(2*time.Minute))
	if !errors.Is(err, types.ErrInvalidChallenge) {
		t.Fatalf("expected dlq challenge to remain invalid, got %v", err)
	}
}

func TestChallengeDeliveryStoreRepairRedrivesActivePendingIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(pool)
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	seedChallengeDeliveryOutbox(t, ctx, repository, issuedAt, "challenge-redrive-pending", issuedAt.Add(15*time.Minute))

	store := NewChallengeDeliveryStore(pool, WithChallengeDeliveryClock(func() time.Time { return issuedAt.Add(time.Minute) }))
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Second, func(_ context.Context, messages []types.ChallengeDeliveryMessage) []error {
		return []error{types.NewChallengeDeliveryFailed("provider unavailable")}
	})
	if err != nil {
		t.Fatalf("process failed delivery to retry: %v", err)
	}
	if stats.Retried != 1 {
		t.Fatalf("expected one retried delivery, got %+v", stats)
	}
	deliveryID := readChallengeDeliveryOutboxID(t, ctx, pool, "challenge-redrive-pending")
	assertChallengeDeliveryOutboxStatus(t, ctx, pool, "challenge-redrive-pending", "PENDING", 1)
	assertChallengeDeliveryOutboxFailureClass(t, ctx, pool, "challenge-redrive-pending", types.ChallengeDeliveryFailureClassDeliveryFailed)

	repairStats, err := store.RepairDeliveries(ctx, types.ChallengeDeliveryRepairOptions{
		DeliveryIDs: []int64{deliveryID},
		Mode:        types.ChallengeDeliveryRepairModeRedriveActivePending,
		Operator:    "operator-1",
		Reason:      "provider recovered",
	})
	if err != nil {
		t.Fatalf("redrive active pending delivery: %v", err)
	}
	if repairStats.Requested != 1 || repairStats.Mutated != 1 || repairStats.Skipped != 0 {
		t.Fatalf("unexpected redrive stats: %+v", repairStats)
	}
	assertChallengeDeliveryOutboxStatus(t, ctx, pool, "challenge-redrive-pending", "PENDING", 1)
	state := readChallengeDeliveryState(t, ctx, pool, "challenge-redrive-pending")
	if state.Status != "ACTIVE" ||
		state.DeliveryStatus != "PENDING" ||
		state.DeliveryFailedAt != nil ||
		state.DeliveryLastError != "" ||
		state.DeliveryFailureClass != "" {
		t.Fatalf("unexpected redriven challenge state: %+v", state)
	}
	assertChallengeDeliveryOutboxFailureClass(t, ctx, pool, "challenge-redrive-pending", "")
	assertChallengeDeliveryRepairAudit(t, ctx, pool, deliveryID, "redrive-active-pending", "MUTATED", "", "PENDING", "ACTIVE", "PENDING", 1, "challenge delivery failed: provider unavailable", types.ChallengeDeliveryFailureClassDeliveryFailed, "PENDING", "ACTIVE", "PENDING", "", "provider recovered")

	stats, err = store.ProcessReadyBatch(ctx, 10, 3, time.Second, func(_ context.Context, messages []types.ChallengeDeliveryMessage) []error {
		if len(messages) != 1 || messages[0].ChallengeID != "challenge-redrive-pending" {
			t.Fatalf("unexpected redriven delivery messages: %+v", messages)
		}
		return []error{nil}
	})
	if err != nil {
		t.Fatalf("process redriven delivery: %v", err)
	}
	if stats.Fetched != 1 || stats.Delivered != 1 {
		t.Fatalf("unexpected stats after redrive: %+v", stats)
	}
}

func TestChallengeDeliveryStoreRepairCancelsInactiveSelectedDeliveryIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(pool)
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	seedChallengeDeliveryOutbox(t, ctx, repository, issuedAt, "challenge-repair-expired", issuedAt.Add(time.Minute))
	deliveryID := readChallengeDeliveryOutboxID(t, ctx, pool, "challenge-repair-expired")
	store := NewChallengeDeliveryStore(pool, WithChallengeDeliveryClock(func() time.Time { return issuedAt.Add(2 * time.Minute) }))

	repairStats, err := store.RepairDeliveries(ctx, types.ChallengeDeliveryRepairOptions{
		DeliveryIDs: []int64{deliveryID},
		Mode:        types.ChallengeDeliveryRepairModeCancelInactive,
		Operator:    "operator-1",
		Reason:      "manual cleanup",
	})
	if err != nil {
		t.Fatalf("cancel inactive selected delivery: %v", err)
	}
	if repairStats.Requested != 1 || repairStats.Mutated != 1 || repairStats.Skipped != 0 {
		t.Fatalf("unexpected cancel inactive stats: %+v", repairStats)
	}
	assertChallengeDeliveryOutboxStatus(t, ctx, pool, "challenge-repair-expired", "CANCELED", 0)
	state := readChallengeDeliveryState(t, ctx, pool, "challenge-repair-expired")
	if state.Status != "EXPIRED" ||
		state.DeliveryStatus != "FAILED" ||
		state.DeliveryFailureClass != types.ChallengeDeliveryFailureClassInactive {
		t.Fatalf("unexpected canceled challenge state: %+v", state)
	}
	assertChallengeDeliveryOutboxFailureClass(t, ctx, pool, "challenge-repair-expired", types.ChallengeDeliveryFailureClassInactive)
	assertChallengeDeliveryRepairAudit(t, ctx, pool, deliveryID, "cancel-inactive", "MUTATED", "", "PENDING", "ACTIVE", "PENDING", 0, "", "", "CANCELED", "EXPIRED", "FAILED", types.ChallengeDeliveryFailureClassInactive, "manual cleanup")
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
			KeyVersion: "v2",
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

func assertChallengeDeliveryOutboxFailureClass(t *testing.T, ctx context.Context, pool *pgxpool.Pool, challengeID string, wantFailureClass string) {
	t.Helper()
	var gotFailureClass string
	err := pool.QueryRow(ctx, `
SELECT failure_class
FROM identity_challenge_delivery_outbox
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
  AND challenge_id = $1
`, challengeID).Scan(&gotFailureClass)
	if err != nil {
		t.Fatalf("read challenge delivery outbox failure class: %v", err)
	}
	if gotFailureClass != wantFailureClass {
		t.Fatalf("expected delivery outbox failure_class=%q, got %q", wantFailureClass, gotFailureClass)
	}
}

func readChallengeDeliveryOutboxID(t *testing.T, ctx context.Context, pool *pgxpool.Pool, challengeID string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(ctx, `
SELECT id
FROM identity_challenge_delivery_outbox
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
  AND challenge_id = $1
`, challengeID).Scan(&id)
	if err != nil {
		t.Fatalf("read challenge delivery outbox id: %v", err)
	}
	return id
}

func assertChallengeDeliveryRepairAudit(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	deliveryID int64,
	wantMode string,
	wantOutcome string,
	wantSkipReason string,
	wantPreviousDeliveryStatus string,
	wantPreviousChallengeStatus string,
	wantPreviousChallengeDeliveryStatus string,
	wantPreviousRetryCount int,
	wantPreviousLastError string,
	wantPreviousFailureClass string,
	wantNewDeliveryStatus string,
	wantNewChallengeStatus string,
	wantNewChallengeDeliveryStatus string,
	wantNewFailureClass string,
	wantReason string,
) {
	t.Helper()
	var previousDeliveryStatus string
	var previousChallengeStatus string
	var previousChallengeDeliveryStatus string
	var previousRetryCount int
	var previousLastError string
	var previousFailureClass string
	var newDeliveryStatus string
	var newChallengeStatus string
	var newChallengeDeliveryStatus string
	var newFailureClass string
	var mode string
	var outcome string
	var skipReason string
	var dryRun bool
	var operator string
	var reason string
	err := pool.QueryRow(ctx, `
SELECT
    previous_delivery_status,
    previous_challenge_status,
    previous_challenge_delivery_status,
    previous_retry_count,
    previous_last_error,
    previous_failure_class,
    new_delivery_status,
    new_challenge_status,
    new_challenge_delivery_status,
    new_failure_class,
    repair_mode,
    repair_outcome,
    skip_reason,
    dry_run,
    repair_operator,
    repair_reason
FROM identity_challenge_delivery_repair_audit
WHERE delivery_id = $1
ORDER BY id DESC
LIMIT 1
`, deliveryID).Scan(
		&previousDeliveryStatus,
		&previousChallengeStatus,
		&previousChallengeDeliveryStatus,
		&previousRetryCount,
		&previousLastError,
		&previousFailureClass,
		&newDeliveryStatus,
		&newChallengeStatus,
		&newChallengeDeliveryStatus,
		&newFailureClass,
		&mode,
		&outcome,
		&skipReason,
		&dryRun,
		&operator,
		&reason,
	)
	if err != nil {
		t.Fatalf("read challenge delivery repair audit: %v", err)
	}
	if previousDeliveryStatus != wantPreviousDeliveryStatus ||
		previousChallengeStatus != wantPreviousChallengeStatus ||
		previousChallengeDeliveryStatus != wantPreviousChallengeDeliveryStatus ||
		previousRetryCount != wantPreviousRetryCount ||
		previousLastError != wantPreviousLastError ||
		previousFailureClass != wantPreviousFailureClass ||
		newDeliveryStatus != wantNewDeliveryStatus ||
		newChallengeStatus != wantNewChallengeStatus ||
		newChallengeDeliveryStatus != wantNewChallengeDeliveryStatus ||
		newFailureClass != wantNewFailureClass ||
		mode != wantMode ||
		outcome != wantOutcome ||
		skipReason != wantSkipReason ||
		dryRun ||
		operator != "operator-1" ||
		reason != wantReason {
		t.Fatalf("unexpected repair audit: previous_delivery=%s previous_challenge=%s previous_challenge_delivery=%s previous_retry=%d previous_error=%q previous_class=%q new_delivery=%s new_challenge=%s new_challenge_delivery=%s new_class=%q mode=%s outcome=%s skip=%q dry_run=%t operator=%q reason=%q",
			previousDeliveryStatus,
			previousChallengeStatus,
			previousChallengeDeliveryStatus,
			previousRetryCount,
			previousLastError,
			previousFailureClass,
			newDeliveryStatus,
			newChallengeStatus,
			newChallengeDeliveryStatus,
			newFailureClass,
			mode,
			outcome,
			skipReason,
			dryRun,
			operator,
			reason,
		)
	}
}
