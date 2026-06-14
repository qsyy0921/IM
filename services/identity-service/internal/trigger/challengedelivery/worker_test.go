package challengedelivery

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestWorkerRunOnceDeliversChallenge(t *testing.T) {
	store := &fakeStore{messages: []types.ChallengeDeliveryMessage{{
		ID:             1,
		TenantID:       "tenant-1",
		UserID:         "user-1",
		ChallengeID:    "challenge-1",
		Type:           types.ChallengeTypeEmailVerification,
		Channel:        types.VerificationChannelEmail,
		Destination:    "user1@example.com",
		EncryptedToken: types.EncryptedChallengeToken{Ciphertext: "ciphertext", Nonce: "nonce", KeyVersion: "local-v1"},
		ExpiresAt:      time.Unix(1_800_000_600, 0).UTC(),
		TraceID:        "trace-1",
		RequestID:      "request-1",
	}}}
	notifier := &fakeNotifier{}
	worker := NewWorker(store, notifier, fakeTokenOpener{token: "plain-token"}, Config{BatchSize: 1})
	stats, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats.Fetched != 1 || !store.called {
		t.Fatalf("expected store to fetch one message, stats=%+v called=%v", stats, store.called)
	}
	if !notifier.called || notifier.notification.Token != "plain-token" || notifier.notification.Type != types.ChallengeTypeEmailVerification {
		t.Fatalf("expected notification with decrypted token, got called=%v notification=%+v", notifier.called, notifier.notification)
	}
}

func TestWorkerRunOnceReturnsPerMessageDecryptError(t *testing.T) {
	store := &fakeStore{messages: []types.ChallengeDeliveryMessage{{
		ID:             1,
		TenantID:       "tenant-1",
		UserID:         "user-1",
		ChallengeID:    "challenge-1",
		Type:           types.ChallengeTypeEmailVerification,
		Channel:        types.VerificationChannelEmail,
		Destination:    "user1@example.com",
		EncryptedToken: types.EncryptedChallengeToken{Ciphertext: "bad", Nonce: "nonce", KeyVersion: "local-v1"},
		ExpiresAt:      time.Unix(1_800_000_600, 0).UTC(),
	}}}
	notifier := &fakeNotifier{}
	decryptErr := types.NewChallengeDeliveryFailed("decrypt failed")
	worker := NewWorker(store, notifier, fakeTokenOpener{err: decryptErr}, Config{BatchSize: 1})
	if _, err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if notifier.called {
		t.Fatalf("decrypt error must not send notification: %+v", notifier.notification)
	}
	if len(store.deliveryErrors) != 1 || !errors.Is(store.deliveryErrors[0], types.ErrChallengeDeliveryFailed) {
		t.Fatalf("expected per-message delivery error, got %+v", store.deliveryErrors)
	}
}

func TestWorkerRetriesTransientRunOnceErrorAndExposesSnapshot(t *testing.T) {
	store := &transientErrorStore{messages: []types.ChallengeDeliveryMessage{{
		ID:             1,
		TenantID:       "tenant-1",
		UserID:         "user-1",
		ChallengeID:    "challenge-1",
		Type:           types.ChallengeTypeEmailVerification,
		Channel:        types.VerificationChannelEmail,
		Destination:    "user1@example.com",
		EncryptedToken: types.EncryptedChallengeToken{Ciphertext: "ciphertext", Nonce: "nonce", KeyVersion: "local-v1"},
		ExpiresAt:      time.Unix(1_800_000_600, 0).UTC(),
		TraceID:        "trace-1",
		RequestID:      "request-1",
	}}}
	notifier := &fakeNotifier{}
	worker := NewWorker(store, notifier, fakeTokenOpener{token: "plain-token"}, Config{
		BatchSize:    1,
		PollInterval: time.Hour,
		ErrorBackoff: time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	notifier.onSend = cancel
	done := make(chan error, 1)
	go func() {
		done <- worker.Run(ctx)
	}()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled run, got %v", err)
	}
	cancel()
	if !notifier.called {
		t.Fatalf("expected notifier call after transient error")
	}
	if store.calls.Load() < 2 {
		t.Fatalf("expected worker retry after transient error")
	}
	snapshot := worker.Snapshot()
	if snapshot.TotalErrors == 0 || snapshot.ConsecutiveErrors != 0 {
		t.Fatalf("unexpected snapshot after recovery: %+v", snapshot)
	}
	if snapshot.LastSuccessAtMS == 0 || snapshot.LastErrorBackoffMS != time.Millisecond.Milliseconds() {
		t.Fatalf("unexpected success/backoff snapshot: %+v", snapshot)
	}
}

type fakeStore struct {
	called         bool
	messages       []types.ChallengeDeliveryMessage
	deliveryErrors []error
}

func (store *fakeStore) ProcessReadyBatch(
	ctx context.Context,
	limit int,
	maxAttempts int,
	retryBaseDelay time.Duration,
	deliver func(context.Context, []types.ChallengeDeliveryMessage) []error,
) (types.ChallengeDeliveryStats, error) {
	store.called = true
	store.deliveryErrors = deliver(ctx, store.messages)
	return types.ChallengeDeliveryStats{Fetched: len(store.messages), Delivered: len(store.messages)}, nil
}

type fakeNotifier struct {
	called       bool
	notification types.ChallengeNotification
	err          error
	onSend       func()
}

func (notifier *fakeNotifier) SendChallenge(_ context.Context, notification types.ChallengeNotification) error {
	notifier.called = true
	notifier.notification = notification
	if notifier.onSend != nil {
		notifier.onSend()
	}
	return notifier.err
}

type fakeTokenOpener struct {
	token string
	err   error
}

func (opener fakeTokenOpener) OpenChallengeToken(types.EncryptedChallengeToken) (string, error) {
	if opener.err != nil {
		return "", opener.err
	}
	return opener.token, nil
}

type transientErrorStore struct {
	calls    atomic.Int32
	messages []types.ChallengeDeliveryMessage
}

func (store *transientErrorStore) ProcessReadyBatch(
	ctx context.Context,
	limit int,
	maxAttempts int,
	retryBaseDelay time.Duration,
	deliver func(context.Context, []types.ChallengeDeliveryMessage) []error,
) (types.ChallengeDeliveryStats, error) {
	if store.calls.Add(1) == 1 {
		return types.ChallengeDeliveryStats{}, errors.New("temporary store failure")
	}
	errs := deliver(ctx, store.messages)
	stats := types.ChallengeDeliveryStats{Fetched: len(store.messages)}
	for _, err := range errs {
		if err != nil {
			stats.Retried++
		} else {
			stats.Delivered++
		}
	}
	return stats, nil
}
