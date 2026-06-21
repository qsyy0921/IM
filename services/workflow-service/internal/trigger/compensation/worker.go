package compensation

import (
	"context"
	"errors"
	"time"

	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

const (
	defaultBatchSize    = 50
	defaultPollInterval = time.Second
	defaultErrorBackoff = time.Second
)

type Store interface {
	RequestApprovedCompensations(ctx context.Context, limit int) ([]types.WorkflowCompensation, error)
}

type Config struct {
	BatchSize    int
	PollInterval time.Duration
	ErrorBackoff time.Duration
	Logf         func(string, ...any)
}

type Worker struct {
	store  Store
	config Config
}

func NewWorker(store Store, config Config) *Worker {
	if config.BatchSize <= 0 {
		config.BatchSize = defaultBatchSize
	}
	if config.PollInterval <= 0 {
		config.PollInterval = defaultPollInterval
	}
	if config.ErrorBackoff <= 0 {
		config.ErrorBackoff = defaultErrorBackoff
	}
	return &Worker{store: store, config: config}
}

func (worker *Worker) RunOnce(ctx context.Context) (int, error) {
	if worker == nil || worker.store == nil {
		return 0, errors.New("workflow compensation worker store is not configured")
	}
	compensations, err := worker.store.RequestApprovedCompensations(ctx, worker.config.BatchSize)
	if err != nil {
		return 0, err
	}
	if len(compensations) > 0 && worker.config.Logf != nil {
		worker.config.Logf("workflow compensation worker requested %d compensations", len(compensations))
	}
	return len(compensations), nil
}

func (worker *Worker) Run(ctx context.Context) error {
	for {
		_, err := worker.RunOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if worker.config.Logf != nil {
				worker.config.Logf("workflow compensation worker retrying after error: %v", err)
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

func sleep(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
