package delivery

import (
	"context"
	"errors"
	"time"

	"github.com/qsyy0921/IM/services/notification-service/internal/types"
)

type Store interface {
	ClaimReadyDeliveryRequests(ctx context.Context, limit int, providerID string) ([]types.DeliveryRequest, error)
	MarkDeliverySucceeded(ctx context.Context, request types.DeliveryRequest, result types.DeliveryResult) error
	MarkDeliveryFailed(ctx context.Context, request types.DeliveryRequest, failure types.DeliveryFailure, maxAttempts int, retryBaseDelay time.Duration) (bool, error)
}

type Provider interface {
	Send(ctx context.Context, request types.DeliveryRequest) (types.DeliveryResult, error)
}

type FailureClassifier interface {
	ClassifyProviderError(error) types.DeliveryFailure
}

type Worker struct {
	store      Store
	provider   Provider
	classifier FailureClassifier
	config     Config
	now        func() time.Time
}

type Config struct {
	BatchSize      int
	PollInterval   time.Duration
	MaxAttempts    int
	RetryBaseDelay time.Duration
	ErrorBackoff   time.Duration
	ProviderID     string
	Logf           func(format string, args ...any)
}

func NewWorker(store Store, provider Provider, classifier FailureClassifier, config Config) *Worker {
	return &Worker{
		store:      store,
		provider:   provider,
		classifier: classifier,
		config:     normalizeConfig(config),
		now:        func() time.Time { return time.Now().UTC() },
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
				worker.config.Logf("notification-service delivery worker retrying after runtime error: %v", err)
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

func (worker *Worker) RunOnce(ctx context.Context) (types.DeliveryWorkerStats, error) {
	if worker == nil || worker.store == nil {
		return types.DeliveryWorkerStats{}, errors.New("notification delivery worker store is not configured")
	}
	if worker.provider == nil {
		return types.DeliveryWorkerStats{}, errors.New("notification delivery worker provider is not configured")
	}
	requests, err := worker.store.ClaimReadyDeliveryRequests(ctx, worker.config.BatchSize, worker.config.ProviderID)
	if err != nil {
		return types.DeliveryWorkerStats{}, err
	}
	stats := types.DeliveryWorkerStats{Claimed: len(requests)}
	for _, request := range requests {
		if worker.requestExpired(request) {
			deadLettered, markErr := worker.store.MarkDeliveryFailed(
				ctx,
				request,
				types.NewExpiredDeliveryFailure(),
				worker.config.MaxAttempts,
				worker.config.RetryBaseDelay,
			)
			if markErr != nil {
				return stats, markErr
			}
			if deadLettered {
				stats.DeadLettered++
			}
			continue
		}
		result, err := worker.provider.Send(ctx, request)
		if err != nil {
			deadLettered, markErr := worker.store.MarkDeliveryFailed(
				ctx,
				request,
				worker.classify(err),
				worker.config.MaxAttempts,
				worker.config.RetryBaseDelay,
			)
			if markErr != nil {
				return stats, markErr
			}
			if deadLettered {
				stats.DeadLettered++
			} else {
				stats.Retried++
			}
			continue
		}
		if err := worker.store.MarkDeliverySucceeded(ctx, request, result); err != nil {
			return stats, err
		}
		stats.Succeeded++
	}
	return stats, nil
}

func (worker *Worker) requestExpired(request types.DeliveryRequest) bool {
	return !request.ExpiresAt.IsZero() && !request.ExpiresAt.After(worker.now())
}

func (worker *Worker) classify(err error) types.DeliveryFailure {
	if worker.classifier != nil {
		failure := worker.classifier.ClassifyProviderError(err)
		if failure.FailureClass != "" || failure.PublicError != "" {
			return failure
		}
	}
	return types.NewProviderUnavailableFailure()
}

func normalizeConfig(config Config) Config {
	if config.BatchSize <= 0 {
		config.BatchSize = 50
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 3
	}
	if config.RetryBaseDelay <= 0 {
		config.RetryBaseDelay = time.Second
	}
	if config.ErrorBackoff <= 0 {
		config.ErrorBackoff = time.Second
	}
	if config.ProviderID == "" {
		config.ProviderID = "local-noop"
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
