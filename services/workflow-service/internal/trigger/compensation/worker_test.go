package compensation

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

func TestWorkerRunOnceRequestsApprovedCompensations(t *testing.T) {
	store := &fakeStore{
		compensations: []types.WorkflowCompensation{
			{CompensationID: "wfc_1"},
			{CompensationID: "wfc_2"},
		},
	}
	worker := NewWorker(store, Config{BatchSize: 2})

	count, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if count != 2 || store.limit != 2 {
		t.Fatalf("unexpected run result count=%d limit=%d", count, store.limit)
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
	limit         int
	compensations []types.WorkflowCompensation
	err           error
}

func (store *fakeStore) RequestApprovedCompensations(_ context.Context, limit int) ([]types.WorkflowCompensation, error) {
	store.limit = limit
	if store.err != nil {
		return nil, store.err
	}
	return store.compensations, nil
}
