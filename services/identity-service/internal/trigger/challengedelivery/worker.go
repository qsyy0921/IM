package challengedelivery

import (
	"context"
	"errors"
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
}

type Config struct {
	BatchSize      int
	PollInterval   time.Duration
	MaxAttempts    int
	RetryBaseDelay time.Duration
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
		stats, err := worker.RunOnce(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		if stats.Fetched > 0 || stats.Canceled > 0 {
			continue
		}
		timer := time.NewTimer(worker.config.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
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
	return config
}
