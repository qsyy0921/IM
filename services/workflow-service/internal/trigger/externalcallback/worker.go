package externalcallback

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

const (
	defaultBatchSize      = 50
	defaultPollInterval   = time.Second
	defaultErrorBackoff   = time.Second
	defaultLeaseDuration  = 30 * time.Second
	defaultRetryBaseDelay = time.Second
)

type Store interface {
	ClaimReadyExternalCallbackDeliveries(ctx context.Context, now time.Time, limit int, leaseDuration time.Duration) ([]types.WorkflowExternalCallbackDelivery, error)
	MarkExternalCallbackDeliveryDelivered(ctx context.Context, delivery types.WorkflowExternalCallbackDelivery, result types.WorkflowExternalCallbackDeliveryResult) (types.WorkflowExternalCallbackDelivery, error)
	MarkExternalCallbackDeliveryFailed(ctx context.Context, delivery types.WorkflowExternalCallbackDelivery, result types.WorkflowExternalCallbackDeliveryResult, nextRetryAt time.Time) (types.WorkflowExternalCallbackDelivery, error)
}

type Provider interface {
	DeliverExternalCallback(ctx context.Context, delivery types.WorkflowExternalCallbackDelivery) (types.WorkflowExternalCallbackDeliveryResult, error)
}

type Config struct {
	BatchSize      int
	PollInterval   time.Duration
	ErrorBackoff   time.Duration
	LeaseDuration  time.Duration
	RetryBaseDelay time.Duration
	Now            func() time.Time
	Logf           func(string, ...any)
}

type Worker struct {
	store    Store
	provider Provider
	config   Config
}

func NewWorker(store Store, provider Provider, config Config) *Worker {
	if config.BatchSize <= 0 {
		config.BatchSize = defaultBatchSize
	}
	if config.PollInterval <= 0 {
		config.PollInterval = defaultPollInterval
	}
	if config.ErrorBackoff <= 0 {
		config.ErrorBackoff = defaultErrorBackoff
	}
	if config.LeaseDuration <= 0 {
		config.LeaseDuration = defaultLeaseDuration
	}
	if config.RetryBaseDelay <= 0 {
		config.RetryBaseDelay = defaultRetryBaseDelay
	}
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Worker{store: store, provider: provider, config: config}
}

func (worker *Worker) RunOnce(ctx context.Context) (int, error) {
	if worker == nil || worker.store == nil {
		return 0, errors.New("workflow external callback delivery store is not configured")
	}
	if worker.provider == nil {
		return 0, errors.New("workflow external callback delivery provider is not configured")
	}
	now := worker.config.Now().UTC()
	deliveries, err := worker.store.ClaimReadyExternalCallbackDeliveries(ctx, now, worker.config.BatchSize, worker.config.LeaseDuration)
	if err != nil {
		return 0, err
	}
	completed := 0
	for _, delivery := range deliveries {
		result, err := worker.provider.DeliverExternalCallback(ctx, delivery)
		if err != nil {
			result.FailureClass = normalizeFailureClass(result.FailureClass)
			if result.FailureClass == "" {
				result.FailureClass = "provider_unavailable"
			}
			nextRetryAt := worker.nextRetryAt(now, delivery)
			if _, markErr := worker.store.MarkExternalCallbackDeliveryFailed(ctx, delivery, result, nextRetryAt); markErr != nil {
				return completed, markErr
			}
			completed++
			continue
		}
		result.DeliveryResultRef = strings.TrimSpace(result.DeliveryResultRef)
		if result.DeliveryResultRef == "" {
			result.FailureClass = "provider_result_missing"
			nextRetryAt := worker.nextRetryAt(now, delivery)
			if _, markErr := worker.store.MarkExternalCallbackDeliveryFailed(ctx, delivery, result, nextRetryAt); markErr != nil {
				return completed, markErr
			}
			completed++
			continue
		}
		if _, err := worker.store.MarkExternalCallbackDeliveryDelivered(ctx, delivery, result); err != nil {
			return completed, err
		}
		completed++
	}
	if completed > 0 && worker.config.Logf != nil {
		worker.config.Logf("workflow external callback delivery worker handled %d deliveries", completed)
	}
	return completed, nil
}

func (worker *Worker) Run(ctx context.Context) error {
	for {
		_, err := worker.RunOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if worker.config.Logf != nil {
				worker.config.Logf("workflow external callback delivery worker retrying after error: %v", err)
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

func (worker *Worker) nextRetryAt(now time.Time, delivery types.WorkflowExternalCallbackDelivery) time.Time {
	if delivery.AttemptCount >= delivery.MaxAttempts {
		return now
	}
	delay := worker.config.RetryBaseDelay * time.Duration(delivery.AttemptCount)
	if delay <= 0 {
		delay = worker.config.RetryBaseDelay
	}
	return now.Add(delay)
}

func normalizeFailureClass(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "-", "_")
	return value
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
