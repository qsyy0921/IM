package providerfailure

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/action-executor/internal/types"
)

func TestWorkerRunOnceProcessesDueProviderFailures(t *testing.T) {
	store := &fakeStore{stats: types.ProviderFailureRetryStats{Fetched: 2, Rescheduled: 1, DeadLettered: 1}}
	worker := NewWorker(store, Config{
		BatchSize:      7,
		MaxAttempts:    4,
		RetryBaseDelay: 15 * time.Second,
	})
	stats, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats != store.stats {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if store.limit != 7 || store.maxAttempts != 4 || store.retryBaseDelay != 15*time.Second || store.now.IsZero() {
		t.Fatalf("unexpected store call: %+v", store)
	}
}

func TestWorkerRunOnceRequiresStore(t *testing.T) {
	_, err := NewWorker(nil, Config{}).RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected nil store error")
	}
}

func TestWorkerRunOnceReturnsStoreError(t *testing.T) {
	wantErr := errors.New("store unavailable")
	_, err := NewWorker(&fakeStore{err: wantErr}, Config{}).RunOnce(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected store error, got %v", err)
	}
}

type fakeStore struct {
	limit          int
	maxAttempts    int
	retryBaseDelay time.Duration
	now            time.Time
	stats          types.ProviderFailureRetryStats
	err            error
}

func (store *fakeStore) ProcessDueProviderFailures(
	_ context.Context,
	limit int,
	maxAttempts int,
	retryBaseDelay time.Duration,
	now time.Time,
) (types.ProviderFailureRetryStats, error) {
	store.limit = limit
	store.maxAttempts = maxAttempts
	store.retryBaseDelay = retryBaseDelay
	store.now = now
	if store.err != nil {
		return types.ProviderFailureRetryStats{}, store.err
	}
	return store.stats, nil
}
