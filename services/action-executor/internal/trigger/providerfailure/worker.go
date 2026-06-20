package providerfailure

import (
	"context"
	"errors"
	"time"

	"github.com/qsyy0921/IM/services/action-executor/internal/types"
)

type Store interface {
	ProcessDueProviderFailures(
		ctx context.Context,
		limit int,
		maxAttempts int,
		retryBaseDelay time.Duration,
		now time.Time,
	) (types.ProviderFailureRetryStats, error)
}

type Worker struct {
	store  Store
	config Config
}

type Config struct {
	BatchSize      int
	PollInterval   time.Duration
	ErrorBackoff   time.Duration
	MaxAttempts    int
	RetryBaseDelay time.Duration
	Logf           func(format string, args ...any)
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
				worker.config.Logf("action-executor provider failure worker retrying after error: %v", err)
			}
			if err := waitForInterval(ctx, worker.config.ErrorBackoff); err != nil {
				return err
			}
			continue
		}
		if stats.Fetched > 0 {
			continue
		}
		if err := waitForInterval(ctx, worker.config.PollInterval); err != nil {
			return err
		}
	}
}

func (worker *Worker) RunOnce(ctx context.Context) (types.ProviderFailureRetryStats, error) {
	if worker == nil || worker.store == nil {
		return types.ProviderFailureRetryStats{}, errors.New("action provider failure store is not configured")
	}
	return worker.store.ProcessDueProviderFailures(
		ctx,
		worker.config.BatchSize,
		worker.config.MaxAttempts,
		worker.config.RetryBaseDelay,
		time.Now().UTC(),
	)
}

func normalizeConfig(config Config) Config {
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.ErrorBackoff <= 0 {
		config.ErrorBackoff = time.Second
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 3
	}
	if config.RetryBaseDelay <= 0 {
		config.RetryBaseDelay = 30 * time.Second
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
