package memberchange

import (
	"context"
	"errors"
	"time"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type ProgressExecutor interface {
	Execute(context.Context) (types.MemberChangePublishProgressStats, error)
}

type ProgressWorker struct {
	executor ProgressExecutor
	config   ProgressConfig
}

type ProgressConfig struct {
	PollInterval time.Duration
	ErrorBackoff time.Duration
	Logf         func(format string, args ...any)
}

func NewProgressWorker(executor ProgressExecutor, config ProgressConfig) *ProgressWorker {
	return &ProgressWorker{
		executor: executor,
		config:   normalizeProgressConfig(config),
	}
}

func (w *ProgressWorker) Run(ctx context.Context) error {
	if w.executor == nil {
		return errors.New("member change progress executor is not configured")
	}
	for {
		stats, err := w.RunOnce(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			if w.config.Logf != nil {
				w.config.Logf("conversation-service member change progress worker retrying after error: %v", err)
			}
			if err := waitForInterval(ctx, w.config.ErrorBackoff); err != nil {
				return err
			}
			continue
		}
		if stats.Advanced > 0 {
			continue
		}
		if err := waitForInterval(ctx, w.config.PollInterval); err != nil {
			return err
		}
	}
}

func (w *ProgressWorker) RunOnce(ctx context.Context) (types.MemberChangePublishProgressStats, error) {
	if w.executor == nil {
		return types.MemberChangePublishProgressStats{}, errors.New("member change progress executor is not configured")
	}
	return w.executor.Execute(ctx)
}

func normalizeProgressConfig(config ProgressConfig) ProgressConfig {
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.ErrorBackoff <= 0 {
		config.ErrorBackoff = config.PollInterval
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
