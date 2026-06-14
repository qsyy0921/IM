package challengedelivery

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type Store interface {
	ProcessReadyBatch(
		ctx context.Context,
		limit int,
		maxAttempts int,
		retryBaseDelay time.Duration,
		deliver func(context.Context, []types.ChallengeDeliveryMessage) []error,
	) (types.ChallengeDeliveryStats, error)
}

type Notifier interface {
	SendChallenge(context.Context, types.ChallengeNotification) error
}

type TokenOpener interface {
	OpenChallengeToken(types.EncryptedChallengeToken) (string, error)
}

type Worker struct {
	store    Store
	notifier Notifier
	tokens   TokenOpener
	config   Config
	metrics  workerMetrics
}

type Config struct {
	BatchSize      int
	PollInterval   time.Duration
	MaxAttempts    int
	RetryBaseDelay time.Duration
	ErrorBackoff   time.Duration
	Logf           func(format string, args ...any)
}

type workerMetrics struct {
	totalErrors        atomic.Uint64
	consecutiveErrors  atomic.Uint64
	lastErrorAtMS      atomic.Int64
	lastSuccessAtMS    atomic.Int64
	lastErrorBackoffMS atomic.Int64
}

func NewWorker(store Store, notifier Notifier, tokens TokenOpener, config Config) *Worker {
	return &Worker{
		store:    store,
		notifier: notifier,
		tokens:   tokens,
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
				worker.config.Logf("identity-service challenge delivery worker retrying after runtime error: %v", err)
			}
			worker.recordError()
			worker.metrics.lastErrorBackoffMS.Store(worker.config.ErrorBackoff.Milliseconds())
			if err := waitForInterval(ctx, worker.config.ErrorBackoff); err != nil {
				return err
			}
			continue
		}
		worker.recordSuccess()
		if stats.Fetched > 0 || stats.Canceled > 0 {
			continue
		}
		if err := waitForInterval(ctx, worker.config.PollInterval); err != nil {
			return err
		}
	}
}

func (worker *Worker) Snapshot() types.ChallengeDeliveryWorkerSnapshot {
	return types.ChallengeDeliveryWorkerSnapshot{
		TotalErrors:        worker.metrics.totalErrors.Load(),
		ConsecutiveErrors:  worker.metrics.consecutiveErrors.Load(),
		LastErrorAtMS:      worker.metrics.lastErrorAtMS.Load(),
		LastSuccessAtMS:    worker.metrics.lastSuccessAtMS.Load(),
		LastErrorBackoffMS: worker.metrics.lastErrorBackoffMS.Load(),
	}
}

func (worker *Worker) RunOnce(ctx context.Context) (types.ChallengeDeliveryStats, error) {
	if worker == nil || worker.store == nil {
		return types.ChallengeDeliveryStats{}, errors.New("identity challenge delivery store is not configured")
	}
	if worker.notifier == nil {
		return types.ChallengeDeliveryStats{}, errors.New("identity challenge delivery notifier is not configured")
	}
	if worker.tokens == nil {
		return types.ChallengeDeliveryStats{}, errors.New("identity challenge delivery token opener is not configured")
	}
	return worker.store.ProcessReadyBatch(
		ctx,
		worker.config.BatchSize,
		worker.config.MaxAttempts,
		worker.config.RetryBaseDelay,
		worker.deliverMessages,
	)
}

func (worker *Worker) deliverMessages(ctx context.Context, messages []types.ChallengeDeliveryMessage) []error {
	errs := make([]error, len(messages))
	for index, message := range messages {
		token, err := worker.tokens.OpenChallengeToken(message.EncryptedToken)
		if err != nil {
			errs[index] = err
			continue
		}
		if token == "" ||
			message.TenantID == "" ||
			message.UserID == "" ||
			message.ChallengeID == "" ||
			message.Type == "" ||
			message.Channel == "" ||
			message.Destination == "" ||
			message.ExpiresAt.IsZero() {
			errs[index] = types.NewChallengeDeliveryFailed("identity challenge delivery message is incomplete")
			continue
		}
		errs[index] = worker.notifier.SendChallenge(ctx, types.ChallengeNotification{
			TenantID:        message.TenantID,
			UserID:          message.UserID,
			ChallengeID:     message.ChallengeID,
			Type:            message.Type,
			Channel:         message.Channel,
			Destination:     message.Destination,
			Token:           token,
			ExpiresAtUnixMS: message.ExpiresAt.UnixMilli(),
			TraceID:         message.TraceID,
			RequestID:       message.RequestID,
		})
	}
	return errs
}

func normalizeConfig(config Config) Config {
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 5
	}
	if config.RetryBaseDelay <= 0 {
		config.RetryBaseDelay = time.Second
	}
	if config.ErrorBackoff <= 0 {
		config.ErrorBackoff = time.Second
	}
	return config
}

func (worker *Worker) recordError() {
	worker.metrics.totalErrors.Add(1)
	worker.metrics.consecutiveErrors.Add(1)
	worker.metrics.lastErrorAtMS.Store(time.Now().UnixMilli())
}

func (worker *Worker) recordSuccess() {
	worker.metrics.consecutiveErrors.Store(0)
	worker.metrics.lastSuccessAtMS.Store(time.Now().UnixMilli())
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
