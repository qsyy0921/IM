package compensation

import (
	"context"
	"errors"
	"time"

	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

type ExecutionStore interface {
	ClaimRequestedCompensations(ctx context.Context, limit int, staleAfter time.Duration) ([]types.WorkflowCompensation, error)
	CompleteWorkflowCompensation(ctx context.Context, compensation types.WorkflowCompensation, result types.WorkflowCompensationExecutionResult) (types.WorkflowCompensation, error)
}

type Executor interface {
	ExecuteCompensation(ctx context.Context, compensation types.WorkflowCompensation) (types.WorkflowCompensationExecutionResult, error)
}

type ExecutionWorker struct {
	store    ExecutionStore
	executor Executor
	config   Config
}

func NewExecutionWorker(store ExecutionStore, executor Executor, config Config) *ExecutionWorker {
	if config.BatchSize <= 0 {
		config.BatchSize = defaultBatchSize
	}
	if config.PollInterval <= 0 {
		config.PollInterval = defaultPollInterval
	}
	if config.ErrorBackoff <= 0 {
		config.ErrorBackoff = defaultErrorBackoff
	}
	return &ExecutionWorker{store: store, executor: executor, config: config}
}

func (worker *ExecutionWorker) RunOnce(ctx context.Context) (int, error) {
	if worker == nil || worker.store == nil {
		return 0, errors.New("workflow compensation execution store is not configured")
	}
	if worker.executor == nil {
		return 0, errors.New("workflow compensation executor is not configured")
	}
	compensations, err := worker.store.ClaimRequestedCompensations(ctx, worker.config.BatchSize, worker.config.ErrorBackoff)
	if err != nil {
		return 0, err
	}
	completed := 0
	for _, compensation := range compensations {
		result, err := worker.executor.ExecuteCompensation(ctx, compensation)
		if err != nil {
			return completed, err
		}
		if _, err := worker.store.CompleteWorkflowCompensation(ctx, compensation, result); err != nil {
			return completed, err
		}
		completed++
	}
	if completed > 0 && worker.config.Logf != nil {
		worker.config.Logf("workflow compensation executor completed %d compensations", completed)
	}
	return completed, nil
}

func (worker *ExecutionWorker) Run(ctx context.Context) error {
	for {
		_, err := worker.RunOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if worker.config.Logf != nil {
				worker.config.Logf("workflow compensation executor retrying after error: %v", err)
			}
			if !sleep(ctx, worker.config.ErrorBackoff) {
				return ctx.Err()
			}
			continue
		}
		if !sleep(ctx, worker.config.PollInterval) {
			return ctx.Err()
		}
	}
}
