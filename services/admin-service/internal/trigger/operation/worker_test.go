package operation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/admin-service/internal/types"
)

func TestWorkerRunOnceCompletesSucceededOperation(t *testing.T) {
	store := &fakeStore{
		operations: []types.AdminOperation{{
			TenantID:    "tenant-admin-worker-test",
			OperationID: "admop_worker_success",
			Status:      types.OperationStatusExecuting,
		}},
	}
	worker := NewWorker(store, fakeExecutor{
		result: types.OperationExecutionResult{
			DownstreamService:    "local-noop",
			DownstreamRequestRef: "operation:admop_worker_success",
			Status:               types.OperationStatusSucceeded,
		},
	}, Config{BatchSize: 1, ResultIDPrefix: "result_"})

	stats, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats.Claimed != 1 || stats.Succeeded != 1 || stats.Failed != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if store.completedResultID != "result_admop_worker_success" {
		t.Fatalf("unexpected result id: %s", store.completedResultID)
	}
	if store.completedResult.Status != types.OperationStatusSucceeded {
		t.Fatalf("unexpected result: %+v", store.completedResult)
	}
}

func TestWorkerRunOnceMarksExecutorErrorFailed(t *testing.T) {
	store := &fakeStore{
		operations: []types.AdminOperation{{
			TenantID:    "tenant-admin-worker-test",
			OperationID: "admop_worker_failed",
			Status:      types.OperationStatusExecuting,
		}},
	}
	worker := NewWorker(store, fakeExecutor{err: errors.New("provider raw timeout")}, Config{BatchSize: 1})

	stats, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats.Claimed != 1 || stats.Succeeded != 0 || stats.Failed != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if store.completedResult.Status != types.OperationStatusFailed ||
		store.completedResult.FailureClass != "EXECUTOR_UNAVAILABLE" ||
		store.completedResult.PublicError != "admin operation execution failed" {
		t.Fatalf("unexpected failed result: %+v", store.completedResult)
	}
}

type fakeStore struct {
	operations        []types.AdminOperation
	completedResultID string
	completedResult   types.OperationExecutionResult
}

func (store *fakeStore) ClaimApprovedOperations(_ context.Context, _ int, _ time.Duration) ([]types.AdminOperation, error) {
	return store.operations, nil
}

func (store *fakeStore) CompleteAdminOperation(_ context.Context, operation types.AdminOperation, result types.OperationExecutionResult, resultID string) (types.AdminOperation, error) {
	store.completedResult = result
	store.completedResultID = resultID
	operation.Status = result.Status
	return operation, nil
}

type fakeExecutor struct {
	result types.OperationExecutionResult
	err    error
}

func (executor fakeExecutor) Execute(context.Context, types.AdminOperation) (types.OperationExecutionResult, error) {
	return executor.result, executor.err
}
