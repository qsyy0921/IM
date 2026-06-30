package outbox

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

const TopicConversationTimelineEvents = "conversation.timeline.events"

type Store interface {
	ProcessReady(
		ctx context.Context,
		limit int,
		maxAttempts int,
		retryBaseDelay time.Duration,
		publish func(context.Context, types.OutboxMessage) error,
	) (types.OutboxRelayStats, error)
}

type BatchStore interface {
	ProcessReadyBatch(
		ctx context.Context,
		limit int,
		maxAttempts int,
		retryBaseDelay time.Duration,
		publish func(context.Context, []types.OutboxMessage) []error,
	) (types.OutboxRelayStats, error)
}

type ShardedBatchStore interface {
	ProcessReadyShardBatch(
		ctx context.Context,
		limit int,
		maxAttempts int,
		retryBaseDelay time.Duration,
		shardCount int,
		shardID int,
		publish func(context.Context, []types.OutboxMessage) []error,
	) (types.OutboxRelayStats, error)
}

type Publisher interface {
	Publish(ctx context.Context, topic string, key []byte, value []byte) error
}

type BatchPublisher interface {
	PublishBatch(ctx context.Context, topic string, records []types.KafkaPublishRecord) error
}

type Relay struct {
	store     Store
	publisher Publisher
	config    Config
	metrics   relayMetrics
}

type Config struct {
	Topic               string
	BatchSize           int
	WorkerCount         int
	DisablePublishBatch bool
	PollInterval        time.Duration
	FailureBackoff      time.Duration
	ErrorBackoff        time.Duration
	MaxAttempts         int
	RetryBaseDelay      time.Duration
	Metrics             types.LatencyRecorder
	Logf                func(format string, args ...any)
}

type relayMetrics struct {
	totalErrors        atomic.Uint64
	consecutiveErrors  atomic.Uint64
	lastErrorAtMS      atomic.Int64
	lastSuccessAtMS    atomic.Int64
	lastPublishedAtMS  atomic.Int64
	lastErrorBackoffMS atomic.Int64
}

func NewRelay(store Store, publisher Publisher, config Config) *Relay {
	return &Relay{
		store:     store,
		publisher: publisher,
		config:    normalizeConfig(config),
	}
}

func (r *Relay) Run(ctx context.Context) error {
	if r.config.WorkerCount <= 1 {
		return r.runWorker(ctx, 0)
	}

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for worker := 0; worker < r.config.WorkerCount; worker++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			if err := r.runWorker(workerCtx, workerID); err != nil && !errors.Is(err, context.Canceled) {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
					cancel()
				}
				mu.Unlock()
			}
		}(worker)
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

func (r *Relay) runWorker(ctx context.Context, workerID int) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		stats, err := r.runOnceForWorker(ctx, workerID)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			if r.config.Logf != nil {
				r.config.Logf("message-service outbox relay retrying after runtime error: %v", err)
			}
			r.recordError()
			r.metrics.lastErrorBackoffMS.Store(r.config.ErrorBackoff.Milliseconds())
			if err := waitForInterval(ctx, r.config.ErrorBackoff); err != nil {
				return err
			}
			continue
		}
		r.recordSuccess(stats)
		if stats.Published > 0 {
			continue
		}
		delay := r.config.PollInterval
		if stats.Fetched > 0 {
			delay = r.config.FailureBackoff
		}
		if err := waitForInterval(ctx, delay); err != nil {
			return err
		}
	}
}

func (r *Relay) Snapshot() types.OutboxRelayWorkerSnapshot {
	return types.OutboxRelayWorkerSnapshot{
		TotalErrors:        r.metrics.totalErrors.Load(),
		ConsecutiveErrors:  r.metrics.consecutiveErrors.Load(),
		LastErrorAtMS:      r.metrics.lastErrorAtMS.Load(),
		LastSuccessAtMS:    r.metrics.lastSuccessAtMS.Load(),
		LastPublishedAtMS:  r.metrics.lastPublishedAtMS.Load(),
		LastErrorBackoffMS: r.metrics.lastErrorBackoffMS.Load(),
	}
}

func (r *Relay) RunOnce(ctx context.Context) (types.OutboxRelayStats, error) {
	return r.runOnceForWorker(ctx, 0)
}

func (r *Relay) runOnceForWorker(ctx context.Context, workerID int) (types.OutboxRelayStats, error) {
	if r.store == nil {
		return types.OutboxRelayStats{}, errors.New("outbox relay store is not configured")
	}
	if r.publisher == nil {
		return types.OutboxRelayStats{}, errors.New("outbox relay publisher is not configured")
	}
	started := time.Now()
	var stats types.OutboxRelayStats
	var err error
	useSingle := r.config.DisablePublishBatch
	if !useSingle {
		if r.config.WorkerCount > 1 {
			store, ok := r.store.(ShardedBatchStore)
			if !ok {
				return types.OutboxRelayStats{}, errors.New("outbox relay store does not support sharded workers")
			}
			stats, err = store.ProcessReadyShardBatch(
				ctx,
				r.config.BatchSize,
				r.config.MaxAttempts,
				r.config.RetryBaseDelay,
				r.config.WorkerCount,
				workerID,
				r.publishMessages,
			)
		} else if store, ok := r.store.(BatchStore); ok {
			stats, err = store.ProcessReadyBatch(
				ctx,
				r.config.BatchSize,
				r.config.MaxAttempts,
				r.config.RetryBaseDelay,
				r.publishMessages,
			)
		} else {
			useSingle = true
		}
	}
	if useSingle {
		stats, err = r.store.ProcessReady(
			ctx,
			r.config.BatchSize,
			r.config.MaxAttempts,
			r.config.RetryBaseDelay,
			r.publishMessage,
		)
	}
	r.config.Metrics.ObserveOutboxProcessReadyResult(time.Since(started), stats.Fetched)
	return stats, err
}

func (r *Relay) publishMessage(ctx context.Context, message types.OutboxMessage) error {
	value, err := BuildKafkaValue(message)
	if err != nil {
		return err
	}
	started := time.Now()
	err = r.publisher.Publish(ctx, r.config.Topic, []byte(message.PartitionKey), value)
	r.config.Metrics.ObserveKafkaPublishCall(time.Since(started), 1)
	return err
}

func (r *Relay) publishMessages(ctx context.Context, messages []types.OutboxMessage) []error {
	errs := make([]error, len(messages))
	if len(messages) == 0 {
		return errs
	}
	records := make([]types.KafkaPublishRecord, 0, len(messages))
	indexes := make([]int, 0, len(messages))
	blockedConversations := make(map[string]struct{})
	for index, message := range messages {
		conversationKey := string(message.TenantID) + ":" + string(message.ConversationID)
		if _, blocked := blockedConversations[conversationKey]; blocked {
			errs[index] = types.ErrOutboxPublishSkipped
			continue
		}
		value, err := BuildKafkaValue(message)
		if err != nil {
			errs[index] = err
			blockedConversations[conversationKey] = struct{}{}
			continue
		}
		records = append(records, types.KafkaPublishRecord{
			Key:   []byte(message.PartitionKey),
			Value: value,
		})
		indexes = append(indexes, index)
	}
	if len(records) == 0 {
		return errs
	}

	started := time.Now()
	if publisher, ok := r.publisher.(BatchPublisher); ok {
		err := publisher.PublishBatch(ctx, r.config.Topic, records)
		r.config.Metrics.ObserveKafkaPublishCall(time.Since(started), len(records))
		if err != nil {
			for _, index := range indexes {
				errs[index] = err
			}
		}
		return errs
	}

	for recordIndex, record := range records {
		err := r.publisher.Publish(ctx, r.config.Topic, record.Key, record.Value)
		if err != nil {
			errs[indexes[recordIndex]] = err
		}
	}
	r.config.Metrics.ObserveKafkaPublishCall(time.Since(started), len(records))
	return errs
}

func normalizeConfig(config Config) Config {
	if config.Topic == "" {
		config.Topic = TopicConversationTimelineEvents
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 500
	}
	if config.WorkerCount <= 0 {
		config.WorkerCount = 1
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.FailureBackoff <= 0 {
		config.FailureBackoff = config.PollInterval
	}
	if config.ErrorBackoff <= 0 {
		config.ErrorBackoff = time.Second
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 5
	}
	if config.RetryBaseDelay <= 0 {
		config.RetryBaseDelay = time.Second
	}
	if config.Metrics == nil {
		config.Metrics = types.NoopLatencyRecorder{}
	}
	return config
}

func (r *Relay) recordError() {
	r.metrics.totalErrors.Add(1)
	r.metrics.consecutiveErrors.Add(1)
	r.metrics.lastErrorAtMS.Store(time.Now().UnixMilli())
}

func (r *Relay) recordSuccess(stats types.OutboxRelayStats) {
	r.metrics.consecutiveErrors.Store(0)
	now := time.Now().UnixMilli()
	r.metrics.lastSuccessAtMS.Store(now)
	if stats.Published > 0 {
		r.metrics.lastPublishedAtMS.Store(now)
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
