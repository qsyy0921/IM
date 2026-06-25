package timer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

func TestWorkerRunOnceFiresDueTimers(t *testing.T) {
	now := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	store := &fakeStore{
		workflows: []types.Workflow{
			{WorkflowID: "wf_1"},
			{WorkflowID: "wf_2"},
		},
	}
	worker := NewWorker(store, Config{BatchSize: 2, Now: func() time.Time { return now }})

	count, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if count != 2 || store.limit != 2 || !store.now.Equal(now) {
		t.Fatalf("unexpected run result count=%d limit=%d now=%s", count, store.limit, store.now)
	}
}

func TestWorkerRunOnceReturnsStoreError(t *testing.T) {
	expected := errors.New("store failed")
	worker := NewWorker(&fakeStore{err: expected}, Config{})

	_, err := worker.RunOnce(context.Background())
	if !errors.Is(err, expected) {
		t.Fatalf("expected store error, got %v", err)
	}
}

type fakeStore struct {
	now       time.Time
	limit     int
	workflows []types.Workflow
	err       error
}

func (store *fakeStore) FireDueWorkflowTimers(_ context.Context, now time.Time, limit int) ([]types.Workflow, error) {
	store.now = now
	store.limit = limit
	if store.err != nil {
		return nil, store.err
	}
	return store.workflows, nil
}
