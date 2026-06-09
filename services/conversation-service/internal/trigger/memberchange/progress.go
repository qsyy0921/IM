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
}

func NewProgressWorker(executor ProgressExecutor, config ProgressConfig) *ProgressWorker {
	return &ProgressWorker{
		executor: executor,
		config:   normalizeProgressConfig(config),
	}
}

func (w *ProgressWorker) Run(ctx context.Context) error {
	for {
		stats, err := w.RunOnce(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		if stats.Advanced > 0 {
			continue
		}
		timer := time.NewTimer(w.config.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
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
	return config
}
