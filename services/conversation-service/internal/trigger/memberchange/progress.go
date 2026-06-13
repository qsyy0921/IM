package memberchange

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type ProgressExecutor interface {
	Execute(context.Context) (types.MemberChangePublishProgressStats, error)
}

type ProgressWorker struct {
	executor ProgressExecutor
	config   ProgressConfig
	metrics  progressMetrics
}

type ProgressConfig struct {
	PollInterval time.Duration
	ErrorBackoff time.Duration
	Logf         func(format string, args ...any)
}

type progressMetrics struct {
	totalErrors        atomic.Uint64
	consecutiveErrors  atomic.Uint64
	lastErrorAtMS      atomic.Int64
	lastSuccessAtMS    atomic.Int64
	lastAdvancedAtMS   atomic.Int64
	lastAdvancedCount  atomic.Int64
	lastErrorBackoffMS atomic.Int64
	lastPollIntervalMS atomic.Int64
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
			w.recordError()
			w.metrics.lastErrorBackoffMS.Store(w.config.ErrorBackoff.Milliseconds())
			if err := waitForInterval(ctx, w.config.ErrorBackoff); err != nil {
				return err
			}
			continue
		}
		w.recordSuccess(stats)
		if stats.Advanced > 0 {
			continue
		}
		w.metrics.lastPollIntervalMS.Store(w.config.PollInterval.Milliseconds())
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

func (w *ProgressWorker) Snapshot() types.MemberChangeWorkerSnapshot {
	return types.MemberChangeWorkerSnapshot{
		TotalErrors:        w.metrics.totalErrors.Load(),
		ConsecutiveErrors:  w.metrics.consecutiveErrors.Load(),
		LastErrorAtMS:      w.metrics.lastErrorAtMS.Load(),
		LastSuccessAtMS:    w.metrics.lastSuccessAtMS.Load(),
		LastAdvancedAtMS:   w.metrics.lastAdvancedAtMS.Load(),
		LastAdvancedCount:  w.metrics.lastAdvancedCount.Load(),
		LastErrorBackoffMS: w.metrics.lastErrorBackoffMS.Load(),
		LastPollIntervalMS: w.metrics.lastPollIntervalMS.Load(),
	}
}

func (w *ProgressWorker) recordError() {
	w.metrics.totalErrors.Add(1)
	w.metrics.consecutiveErrors.Add(1)
	w.metrics.lastErrorAtMS.Store(time.Now().UnixMilli())
}

func (w *ProgressWorker) recordSuccess(stats types.MemberChangePublishProgressStats) {
	w.metrics.consecutiveErrors.Store(0)
	now := time.Now().UnixMilli()
	w.metrics.lastSuccessAtMS.Store(now)
	if stats.Advanced > 0 {
		w.metrics.lastAdvancedAtMS.Store(now)
		w.metrics.lastAdvancedCount.Store(int64(stats.Advanced))
	}
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
