package compensation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

func TestExecutionWorkerRunOnceExecutesAndCompletesCompensations(t *testing.T) {
	store := &fakeExecutionStore{
		claimed: []types.WorkflowCompensation{
			{TenantID: "tenant-1", WorkflowID: "wf_1", CompensationID: "wfc_1"},
		},
	}
	executor := &fakeExecutor{result: types.WorkflowCompensationExecutionResult{
		Status:               types.WorkflowCompensationStatusSucceeded,
		DownstreamService:    "control-plane-service",
		DownstreamRequestRef: "config-rollback:prod:kind:key:v1",
	}}
	worker := NewExecutionWorker(store, executor, Config{BatchSize: 7, ErrorBackoff: 2 * time.Second})

	count, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if count != 1 || store.limit != 7 || store.staleAfter != 2*time.Second {
		t.Fatalf("unexpected worker counters count=%d limit=%d stale=%s", count, store.limit, store.staleAfter)
	}
	if executor.executed.CompensationID != "wfc_1" || store.completed.CompensationID != "wfc_1" {
		t.Fatalf("unexpected execution state executor=%+v completed=%+v", executor.executed, store.completed)
	}
	if store.result.Status != types.WorkflowCompensationStatusSucceeded {
		t.Fatalf("unexpected completion result: %+v", store.result)
	}
}

func TestExecutionWorkerRunOnceReturnsExecutorError(t *testing.T) {
	expected := errors.New("executor failed")
	worker := NewExecutionWorker(
		&fakeExecutionStore{claimed: []types.WorkflowCompensation{{CompensationID: "wfc_1"}}},
		&fakeExecutor{err: expected},
		Config{},
	)

	_, err := worker.RunOnce(context.Background())
	if !errors.Is(err, expected) {
		t.Fatalf("expected executor error, got %v", err)
	}
}

type fakeExecutionStore struct {
	limit      int
	staleAfter time.Duration
	claimed    []types.WorkflowCompensation
	completed  types.WorkflowCompensation
	result     types.WorkflowCompensationExecutionResult
	err        error
}

func (store *fakeExecutionStore) ClaimRequestedCompensations(_ context.Context, limit int, staleAfter time.Duration) ([]types.WorkflowCompensation, error) {
	store.limit = limit
	store.staleAfter = staleAfter
	if store.err != nil {
		return nil, store.err
	}
	return store.claimed, nil
}

func (store *fakeExecutionStore) CompleteWorkflowCompensation(
	_ context.Context,
	compensation types.WorkflowCompensation,
	result types.WorkflowCompensationExecutionResult,
) (types.WorkflowCompensation, error) {
	store.completed = compensation
	store.result = result
	return compensation, nil
}

type fakeExecutor struct {
	executed types.WorkflowCompensation
	result   types.WorkflowCompensationExecutionResult
	err      error
}

func (executor *fakeExecutor) ExecuteCompensation(_ context.Context, compensation types.WorkflowCompensation) (types.WorkflowCompensationExecutionResult, error) {
	executor.executed = compensation
	if executor.err != nil {
		return types.WorkflowCompensationExecutionResult{}, executor.err
	}
	return executor.result, nil
}
