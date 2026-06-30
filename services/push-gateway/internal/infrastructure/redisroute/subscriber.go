package redisroute

import (
	"context"
	"encoding/json"
	"errors"
	"hash/fnv"
	"sync/atomic"
	"time"

	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
	"github.com/redis/go-redis/v9"
)

type Subscriber struct {
	local  LocalRegistry
	client redis.UniversalClient
	config SubscriberConfig

	metrics      subscriberMetrics
	signalFanout *signalFanoutDispatcher
}

type subscriberMetrics struct {
	messageCount                  atomic.Uint64
	malformedCount                atomic.Uint64
	enqueuedCount                 atomic.Uint64
	evictedCount                  atomic.Uint64
	errorCount                    atomic.Uint64
	consecutiveErrors             atomic.Uint64
	lastErrorAtMS                 atomic.Int64
	lastSuccessAtMS               atomic.Int64
	lastErrorBackoffMS            atomic.Int64
	notifyFanoutDuration          durationMetrics
	signalFanoutDuration          durationMetrics
	signalFanoutQueuedCount       atomic.Uint64
	signalFanoutQueueFullCount    atomic.Uint64
	signalFanoutWorkerErrorCount  atomic.Uint64
	signalFanoutQueueDepth        atomic.Int64
	signalFanoutQueueWaitDuration durationMetrics
}

type SubscriberConfig struct {
	GatewayID             string
	KeyPrefix             string
	ErrorBackoff          time.Duration
	SignalFanoutWorkers   int
	SignalFanoutQueueSize int
	Logf                  func(format string, args ...any)
}

func NewSubscriber(local LocalRegistry, client redis.UniversalClient, config SubscriberConfig) *Subscriber {
	if config.KeyPrefix == "" {
		config.KeyPrefix = defaultKeyPrefix
	}
	if config.ErrorBackoff <= 0 {
		config.ErrorBackoff = 200 * time.Millisecond
	}
	if config.SignalFanoutWorkers <= 0 {
		config.SignalFanoutWorkers = 4
	}
	if config.SignalFanoutQueueSize <= 0 {
		config.SignalFanoutQueueSize = 4096
	}
	subscriber := &Subscriber{local: local, client: client, config: config}
	subscriber.signalFanout = newSignalFanoutDispatcher(
		local,
		&subscriber.metrics,
		config.SignalFanoutWorkers,
		config.SignalFanoutQueueSize,
	)
	return subscriber
}

func (subscriber *Subscriber) Metrics() Metrics {
	return Metrics{
		RedisRouteSubscriberMessageCount:                  subscriber.metrics.messageCount.Load(),
		RedisRouteSubscriberMalformedCount:                subscriber.metrics.malformedCount.Load(),
		RedisRouteSubscriberEnqueuedCount:                 subscriber.metrics.enqueuedCount.Load(),
		RedisRouteSubscriberEvictedCount:                  subscriber.metrics.evictedCount.Load(),
		RedisRouteSubscriberErrorCount:                    subscriber.metrics.errorCount.Load(),
		RedisRouteSubscriberNotifyFanoutDuration:          snapshotDuration(&subscriber.metrics.notifyFanoutDuration),
		RedisRouteSubscriberSignalFanoutDuration:          snapshotDuration(&subscriber.metrics.signalFanoutDuration),
		RedisRouteSubscriberSignalFanoutQueuedCount:       subscriber.metrics.signalFanoutQueuedCount.Load(),
		RedisRouteSubscriberSignalFanoutQueueFullCount:    subscriber.metrics.signalFanoutQueueFullCount.Load(),
		RedisRouteSubscriberSignalFanoutWorkerErrorCount:  subscriber.metrics.signalFanoutWorkerErrorCount.Load(),
		RedisRouteSubscriberSignalFanoutQueueDepth:        subscriber.metrics.signalFanoutQueueDepth.Load(),
		RedisRouteSubscriberSignalFanoutQueueWaitDuration: snapshotDuration(&subscriber.metrics.signalFanoutQueueWaitDuration),
	}
}

func (subscriber *Subscriber) Snapshot() types.RedisSubscriberWorkerSnapshot {
	return types.RedisSubscriberWorkerSnapshot{
		TotalErrors:        subscriber.metrics.errorCount.Load(),
		ConsecutiveErrors:  subscriber.metrics.consecutiveErrors.Load(),
		LastErrorAtMS:      subscriber.metrics.lastErrorAtMS.Load(),
		LastSuccessAtMS:    subscriber.metrics.lastSuccessAtMS.Load(),
		LastErrorBackoffMS: subscriber.metrics.lastErrorBackoffMS.Load(),
	}
}

func (subscriber *Subscriber) Run(ctx context.Context) error {
	if subscriber.signalFanout != nil {
		subscriber.signalFanout.Start(ctx)
	}
	for {
		if err := subscriber.runOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			subscriber.metrics.errorCount.Add(1)
			subscriber.metrics.consecutiveErrors.Add(1)
			subscriber.metrics.lastErrorAtMS.Store(time.Now().UnixMilli())
			subscriber.metrics.lastErrorBackoffMS.Store(subscriber.config.ErrorBackoff.Milliseconds())
			if subscriber.config.Logf != nil {
				subscriber.config.Logf("push-gateway redis route subscriber retrying after runtime error: %v", err)
			}
			if err := waitForInterval(ctx, subscriber.config.ErrorBackoff); err != nil {
				return err
			}
			continue
		}
		subscriber.metrics.consecutiveErrors.Store(0)
		subscriber.metrics.lastSuccessAtMS.Store(time.Now().UnixMilli())
		if err := waitForInterval(ctx, subscriber.config.ErrorBackoff); err != nil {
			return err
		}
	}
}

func (subscriber *Subscriber) runOnce(ctx context.Context) error {
	notificationChannel := GatewayChannel(subscriber.config.KeyPrefix, subscriber.config.GatewayID)
	evictionChannel := GatewayEvictionChannel(subscriber.config.KeyPrefix, subscriber.config.GatewayID)
	pubsub := subscriber.client.Subscribe(ctx, notificationChannel, evictionChannel)
	defer pubsub.Close()
	if _, err := pubsub.Receive(ctx); err != nil {
		return err
	}
	channel := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case message, ok := <-channel:
			if !ok {
				return nil
			}
			switch message.Channel {
			case notificationChannel:
				subscriber.handleNotification(ctx, []byte(message.Payload))
			case evictionChannel:
				subscriber.handleEviction(ctx, []byte(message.Payload))
			default:
				subscriber.metrics.malformedCount.Add(1)
			}
		}
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

func (subscriber *Subscriber) handleNotification(ctx context.Context, payload []byte) {
	var notification types.DeliveryNotification
	if err := json.Unmarshal(payload, &notification); err != nil {
		subscriber.metrics.malformedCount.Add(1)
		return
	}
	if err := notification.Validate(); err != nil {
		subscriber.metrics.malformedCount.Add(1)
		return
	}
	subscriber.metrics.messageCount.Add(1)
	var (
		result types.NotifyDeliveryResult
		err    error
	)
	if notification.Kind == types.DeliveryNotificationKindConversationSignal {
		err = subscriber.signalFanout.Submit(ctx, notification)
		if err != nil {
			subscriber.metrics.errorCount.Add(1)
		}
		return
	} else {
		startedAt := time.Now()
		result, err = subscriber.local.EnqueueNotification(ctx, notification)
		recordDuration(&subscriber.metrics.notifyFanoutDuration, time.Since(startedAt))
	}
	if err != nil {
		subscriber.metrics.errorCount.Add(1)
		return
	}
	if result.Enqueued > 0 {
		subscriber.metrics.enqueuedCount.Add(uint64(result.Enqueued))
	}
	if result.Evicted > 0 {
		subscriber.metrics.evictedCount.Add(uint64(result.Evicted))
	}
}

func (subscriber *Subscriber) handleEviction(ctx context.Context, payload []byte) {
	var eviction evictionMessage
	if err := json.Unmarshal(payload, &eviction); err != nil {
		subscriber.metrics.malformedCount.Add(1)
		return
	}
	if eviction.TenantID == "" || eviction.UserID == "" || eviction.DeviceID == "" {
		subscriber.metrics.malformedCount.Add(1)
		return
	}
	if eviction.Reason == "" {
		eviction.Reason = "identity_revoked"
	}
	subscriber.metrics.messageCount.Add(1)
	var (
		result types.SessionEvictionResult
		err    error
	)
	if eviction.SessionID != "" {
		result, err = subscriber.local.EvictSession(ctx, eviction.TenantID, eviction.UserID, eviction.DeviceID, eviction.SessionID, eviction.Reason)
	} else {
		result, err = subscriber.local.EvictDevice(ctx, eviction.TenantID, eviction.UserID, eviction.DeviceID, eviction.Reason)
	}
	if err != nil {
		subscriber.metrics.errorCount.Add(1)
		return
	}
	if result.Evicted > 0 {
		subscriber.metrics.evictedCount.Add(uint64(result.Evicted))
	}
}

var errSignalFanoutQueueFull = errors.New("redis subscriber conversation signal fanout queue full")

type signalFanoutDispatcher struct {
	local   LocalRegistry
	metrics *subscriberMetrics
	queues  []chan signalFanoutJob

	started atomic.Bool
}

type signalFanoutJob struct {
	notification types.DeliveryNotification
	enqueuedAt   time.Time
}

func newSignalFanoutDispatcher(local LocalRegistry, metrics *subscriberMetrics, workers int, queueSize int) *signalFanoutDispatcher {
	if workers <= 0 {
		workers = 1
	}
	if queueSize <= 0 {
		queueSize = 1
	}
	queues := make([]chan signalFanoutJob, workers)
	for index := range queues {
		queues[index] = make(chan signalFanoutJob, queueSize)
	}
	return &signalFanoutDispatcher{
		local:   local,
		metrics: metrics,
		queues:  queues,
	}
}

func (dispatcher *signalFanoutDispatcher) Start(ctx context.Context) {
	if dispatcher == nil || !dispatcher.started.CompareAndSwap(false, true) {
		return
	}
	for index := range dispatcher.queues {
		go dispatcher.runWorker(ctx, dispatcher.queues[index])
	}
}

func (dispatcher *signalFanoutDispatcher) Submit(ctx context.Context, notification types.DeliveryNotification) error {
	if dispatcher == nil {
		return errSignalFanoutQueueFull
	}
	queue := dispatcher.queues[dispatcher.shard(notification)]
	job := signalFanoutJob{
		notification: notification,
		enqueuedAt:   time.Now(),
	}
	dispatcher.metrics.signalFanoutQueueDepth.Add(1)
	select {
	case <-ctx.Done():
		dispatcher.metrics.signalFanoutQueueDepth.Add(-1)
		return ctx.Err()
	case queue <- job:
		dispatcher.metrics.signalFanoutQueuedCount.Add(1)
		return nil
	default:
		dispatcher.metrics.signalFanoutQueueDepth.Add(-1)
		dispatcher.metrics.signalFanoutQueueFullCount.Add(1)
		return errSignalFanoutQueueFull
	}
}

func (dispatcher *signalFanoutDispatcher) runWorker(ctx context.Context, queue <-chan signalFanoutJob) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-queue:
			dispatcher.metrics.signalFanoutQueueDepth.Add(-1)
			dispatcher.process(ctx, job)
		}
	}
}

func (dispatcher *signalFanoutDispatcher) process(ctx context.Context, job signalFanoutJob) {
	recordDuration(&dispatcher.metrics.signalFanoutQueueWaitDuration, time.Since(job.enqueuedAt))
	startedAt := time.Now()
	result, err := dispatcher.local.EnqueueConversationSignal(ctx, job.notification)
	recordDuration(&dispatcher.metrics.signalFanoutDuration, time.Since(startedAt))
	if err != nil {
		if ctx.Err() == nil {
			dispatcher.metrics.signalFanoutWorkerErrorCount.Add(1)
			dispatcher.metrics.errorCount.Add(1)
		}
		return
	}
	if result.Enqueued > 0 {
		dispatcher.metrics.enqueuedCount.Add(uint64(result.Enqueued))
	}
	if result.Evicted > 0 {
		dispatcher.metrics.evictedCount.Add(uint64(result.Evicted))
	}
}

func (dispatcher *signalFanoutDispatcher) shard(notification types.DeliveryNotification) int {
	if len(dispatcher.queues) == 1 {
		return 0
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(notification.TenantID))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(notification.ConversationID))
	return int(hash.Sum32() % uint32(len(dispatcher.queues)))
}
