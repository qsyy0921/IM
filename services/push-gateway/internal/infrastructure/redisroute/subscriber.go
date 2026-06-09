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
	pubsub := subscriber.client.Subscribe(ctx, GatewayChannel(subscriber.config.KeyPrefix, subscriber.config.GatewayID))
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
			var notification types.DeliveryNotification
			if err := json.Unmarshal([]byte(message.Payload), &notification); err != nil {
				subscriber.metrics.malformedCount.Add(1)
				continue
			}
			subscriber.metrics.messageCount.Add(1)
			result, err := subscriber.local.EnqueueNotification(ctx, notification)
			if err != nil {
				subscriber.metrics.errorCount.Add(1)
				continue
			}
			if result.Enqueued > 0 {
				subscriber.metrics.enqueuedCount.Add(uint64(result.Enqueued))
			}
		}
	}
}
