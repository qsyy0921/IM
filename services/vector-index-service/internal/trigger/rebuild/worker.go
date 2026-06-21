package rebuild

import (
	"context"
	"errors"
	"time"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

type Store interface {
	ClaimRebuildTasks(ctx context.Context, limit int) ([]types.VectorRebuildTask, error)
	ContinueRebuildTask(ctx context.Context, task types.VectorRebuildTask, cursorValue string) error
	CompleteRebuildTask(ctx context.Context, task types.VectorRebuildTask) error
}

type Backfiller interface {
	BackfillRebuildTask(ctx context.Context, task types.VectorRebuildTask) (types.RebuildBackfillStats, error)
}

type Worker struct {
	store  Store
	config Config
}

type Config struct {
	BatchSize    int
	PollInterval time.Duration
	ErrorBackoff time.Duration
	Backfiller   Backfiller
	Logf         func(format string, args ...any)
}

func NewWorker(store Store, config Config) *Worker {
	return &Worker{store: store, config: normalizeConfig(config)}
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
				worker.config.Logf("vector-index-service rebuild worker retrying after runtime error: %v", err)
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

func (worker *Worker) RunOnce(ctx context.Context) (types.RebuildWorkerStats, error) {
	if worker == nil || worker.store == nil {
		return types.RebuildWorkerStats{}, errors.New("vector rebuild worker store is not configured")
	}
	tasks, err := worker.store.ClaimRebuildTasks(ctx, worker.config.BatchSize)
	if err != nil {
		return types.RebuildWorkerStats{}, err
	}
	stats := types.RebuildWorkerStats{Claimed: len(tasks)}
	for _, task := range tasks {
		if worker.config.Backfiller != nil {
			backfillStats, err := worker.config.Backfiller.BackfillRebuildTask(ctx, task)
			if err != nil {
				return stats, err
			}
			stats.Backfilled += backfillStats.Backfilled
			stats.Embedded += backfillStats.Embedded
			stats.Upserted += backfillStats.Upserted
			if backfillStats.HasMore {
				if err := worker.store.ContinueRebuildTask(ctx, task, backfillStats.NextCursor); err != nil {
					return stats, err
				}
				stats.Continued++
				continue
			}
		}
		if err := worker.store.CompleteRebuildTask(ctx, task); err != nil {
			return stats, err
		}
		stats.Completed++
	}
	return stats, nil
}

func normalizeConfig(config Config) Config {
	if config.BatchSize <= 0 {
		config.BatchSize = 50
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.ErrorBackoff <= 0 {
		config.ErrorBackoff = time.Second
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
