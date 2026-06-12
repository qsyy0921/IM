package redisroute

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"

	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
	"github.com/redis/go-redis/v9"
)

type Subscriber struct {
	local  LocalRegistry
	client redis.UniversalClient
	config Config

	metrics subscriberMetrics
}

type subscriberMetrics struct {
	messageCount   atomic.Uint64
	malformedCount atomic.Uint64
	enqueuedCount  atomic.Uint64
	evictedCount   atomic.Uint64
	errorCount     atomic.Uint64
}

func NewSubscriber(local LocalRegistry, client redis.UniversalClient, config Config) *Subscriber {
	if config.KeyPrefix == "" {
		config.KeyPrefix = defaultKeyPrefix
	}
	return &Subscriber{local: local, client: client, config: config}
}

func (subscriber *Subscriber) Metrics() Metrics {
	return Metrics{
		RedisRouteSubscriberMessageCount:   subscriber.metrics.messageCount.Load(),
		RedisRouteSubscriberMalformedCount: subscriber.metrics.malformedCount.Load(),
		RedisRouteSubscriberEnqueuedCount:  subscriber.metrics.enqueuedCount.Load(),
		RedisRouteSubscriberEvictedCount:   subscriber.metrics.evictedCount.Load(),
		RedisRouteSubscriberErrorCount:     subscriber.metrics.errorCount.Load(),
	}
}

func (subscriber *Subscriber) Run(ctx context.Context) error {
	for {
		if err := subscriber.runOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			subscriber.metrics.errorCount.Add(1)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
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

func (subscriber *Subscriber) handleNotification(ctx context.Context, payload []byte) {
	var notification types.DeliveryNotification
	if err := json.Unmarshal(payload, &notification); err != nil {
		subscriber.metrics.malformedCount.Add(1)
		return
	}
	subscriber.metrics.messageCount.Add(1)
	result, err := subscriber.local.EnqueueNotification(ctx, notification)
	if err != nil {
		subscriber.metrics.errorCount.Add(1)
		return
	}
	if result.Enqueued > 0 {
		subscriber.metrics.enqueuedCount.Add(uint64(result.Enqueued))
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
