package operation

import (
	"context"
	"errors"
	"time"

	"github.com/qsyy0921/IM/services/admin-service/internal/types"
)

type Store interface {
	ClaimApprovedOperations(ctx context.Context, limit int, staleAfter time.Duration) ([]types.AdminOperation, error)
	CompleteAdminOperation(ctx context.Context, operation types.AdminOperation, result types.OperationExecutionResult, resultID string) (types.AdminOperation, error)
}

type Executor interface {
	Execute(ctx context.Context, operation types.AdminOperation) (types.OperationExecutionResult, error)
}

type Worker struct {
	store    Store
	executor Executor
	config   Config
}

type Config struct {
	BatchSize      int
	PollInterval   time.Duration
	StaleAfter     time.Duration
	ErrorBackoff   time.Duration
	ResultIDPrefix string
	Logf           func(format string, args ...any)
}

func NewWorker(store Store, executor Executor, config Config) *Worker {
	return &Worker{
		store:    store,
		executor: executor,
		config:   normalizeConfig(config),
	}
}

func (worker *Worker) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		stats, err := worker.RunOnce(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			if worker.config.Logf != nil {
				worker.config.Logf("admin-service operation worker retrying after runtime error: %v", err)
			}
			if err := waitForInterval(ctx, worker.config.ErrorBackoff); err != nil {
				return err
			}
			continue
		}
		if stats.Claimed > 0 {
			continue
		}
		if err := waitForInterval(ctx, worker.config.PollInterval); err != nil {
			return err
		}
	}
}

func (worker *Worker) RunOnce(ctx context.Context) (types.OperationWorkerStats, error) {
	if worker == nil || worker.store == nil {
		return types.OperationWorkerStats{}, errors.New("admin operation worker store is not configured")
	}
	if worker.executor == nil {
		return types.OperationWorkerStats{}, errors.New("admin operation worker executor is not configured")
	}
	operations, err := worker.store.ClaimApprovedOperations(ctx, worker.config.BatchSize, worker.config.StaleAfter)
	if err != nil {
		return types.OperationWorkerStats{}, err
	}
	stats := types.OperationWorkerStats{Claimed: len(operations)}
	for _, operation := range operations {
		result, err := worker.executor.Execute(ctx, operation)
		if err != nil {
			result = types.OperationExecutionResult{
				DownstreamService:    "local-executor",
				DownstreamRequestRef: operation.OperationID,
				Status:               types.OperationStatusFailed,
				FailureClass:         "EXECUTOR_UNAVAILABLE",
				PublicError:          "admin operation execution failed",
			}
		}
		completed, markErr := worker.store.CompleteAdminOperation(ctx, operation, result, worker.resultID(operation))
		if markErr != nil {
			return stats, markErr
		}
		if completed.Status == types.OperationStatusSucceeded {
			stats.Succeeded++
		}
		if completed.Status == types.OperationStatusFailed {
			stats.Failed++
		}
	}
	return stats, nil
}

func (worker *Worker) resultID(operation types.AdminOperation) string {
	return worker.config.ResultIDPrefix + operation.OperationID
}

func normalizeConfig(config Config) Config {
	if config.BatchSize <= 0 {
		config.BatchSize = 50
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.StaleAfter <= 0 {
		config.StaleAfter = 5 * time.Minute
	}
	if config.ErrorBackoff <= 0 {
		config.ErrorBackoff = time.Second
	}
	if config.ResultIDPrefix == "" {
		config.ResultIDPrefix = "admres_"
	}
	return config
}

func waitForInterval(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
