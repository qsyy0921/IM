package redisroute

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
	"github.com/redis/go-redis/v9"
)

func TestSubscriberQueuesConversationSignalFanout(t *testing.T) {
	local := &blockingConversationSignalLocal{
		calls:   make(chan types.DeliveryNotification, 2),
		release: make(chan struct{}),
		result:  types.NotifyDeliveryResult{MatchedSessions: 3, Enqueued: 3},
	}
	subscriber := NewSubscriber(local, noopRedisClient{}, SubscriberConfig{
		GatewayID:             "gateway-a",
		SignalFanoutWorkers:   1,
		SignalFanoutQueueSize: 2,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	subscriber.signalFanout.Start(ctx)

	subscriber.handleNotification(ctx, mustMarshalNotification(t, testConversationSignal()))
	select {
	case notification := <-local.calls:
		if notification.ConversationID != "conversation-1" {
			t.Fatalf("unexpected notification: %+v", notification)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for signal fanout worker")
	}
	close(local.release)
	waitForSubscriberMetrics(t, subscriber, func(metrics Metrics) bool {
		return metrics.RedisRouteSubscriberMessageCount == 1 &&
			metrics.RedisRouteSubscriberSignalFanoutQueuedCount == 1 &&
			metrics.RedisRouteSubscriberSignalFanoutQueueFullCount == 0 &&
			metrics.RedisRouteSubscriberSignalFanoutQueueDepth == 0 &&
			metrics.RedisRouteSubscriberSignalFanoutQueueWaitDuration.Count == 1 &&
			metrics.RedisRouteSubscriberSignalFanoutDuration.Count == 1 &&
			metrics.RedisRouteSubscriberEnqueuedCount == 3
	})
}

func TestSubscriberConversationSignalQueueFullIsExplicit(t *testing.T) {
	local := &blockingConversationSignalLocal{
		calls:   make(chan types.DeliveryNotification, 4),
		release: make(chan struct{}),
		result:  types.NotifyDeliveryResult{MatchedSessions: 1, Enqueued: 1},
	}
	subscriber := NewSubscriber(local, noopRedisClient{}, SubscriberConfig{
		GatewayID:             "gateway-a",
		SignalFanoutWorkers:   1,
		SignalFanoutQueueSize: 1,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	subscriber.signalFanout.Start(ctx)

	subscriber.handleNotification(ctx, mustMarshalNotification(t, testConversationSignal()))
	select {
	case <-local.calls:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for first signal fanout worker call")
	}
	second := testConversationSignal()
	second.EventID = "event-2"
	second.ConversationSeq = 2
	subscriber.handleNotification(ctx, mustMarshalNotification(t, second))
	third := testConversationSignal()
	third.EventID = "event-3"
	third.ConversationSeq = 3
	subscriber.handleNotification(ctx, mustMarshalNotification(t, third))

	metrics := subscriber.Metrics()
	if metrics.RedisRouteSubscriberSignalFanoutQueuedCount != 2 ||
		metrics.RedisRouteSubscriberSignalFanoutQueueFullCount != 1 ||
		metrics.RedisRouteSubscriberSignalFanoutQueueDepth != 1 ||
		metrics.RedisRouteSubscriberErrorCount != 1 {
		t.Fatalf("unexpected queue-full metrics before release: %+v", metrics)
	}
	close(local.release)
	waitForSubscriberMetrics(t, subscriber, func(metrics Metrics) bool {
		return metrics.RedisRouteSubscriberEnqueuedCount == 2 &&
			metrics.RedisRouteSubscriberSignalFanoutDuration.Count == 2 &&
			metrics.RedisRouteSubscriberSignalFanoutQueueDepth == 0
	})
}

func mustMarshalNotification(t *testing.T, notification types.DeliveryNotification) []byte {
	t.Helper()
	payload, err := json.Marshal(notification)
	if err != nil {
		t.Fatalf("marshal notification: %v", err)
	}
	return payload
}

func testConversationSignal() types.DeliveryNotification {
	return types.DeliveryNotification{
		Kind:            types.DeliveryNotificationKindConversationSignal,
		EventID:         "event-1",
		TenantID:        "tenant-1",
		ConversationID:  "conversation-1",
		ConversationSeq: 1,
		SourceEventID:   "timeline-event-1",
		SourceEventType: "conversation.message.created.v1",
		FanoutMode:      types.FanoutModeReadFanout,
	}
}

type blockingConversationSignalLocal struct {
	calls   chan types.DeliveryNotification
	release chan struct{}
	result  types.NotifyDeliveryResult
	err     error
}

func (local *blockingConversationSignalLocal) Register(context.Context, types.SessionRegistration) (types.SessionRegistrationResult, error) {
	return types.SessionRegistrationResult{}, nil
}

func (local *blockingConversationSignalLocal) Unregister(string) {}

func (local *blockingConversationSignalLocal) EnqueueNotification(context.Context, types.DeliveryNotification) (types.NotifyDeliveryResult, error) {
	return types.NotifyDeliveryResult{}, nil
}

func (local *blockingConversationSignalLocal) SubscribeConversation(context.Context, types.ConversationSubscriptionCommand) (types.ConversationSubscriptionResult, error) {
	return types.ConversationSubscriptionResult{}, nil
}

func (local *blockingConversationSignalLocal) UnsubscribeConversation(context.Context, types.ConversationSubscriptionCommand) (types.ConversationSubscriptionResult, error) {
	return types.ConversationSubscriptionResult{}, nil
}

func (local *blockingConversationSignalLocal) EnqueueConversationSignal(ctx context.Context, notification types.DeliveryNotification) (types.NotifyDeliveryResult, error) {
	select {
	case local.calls <- notification:
	case <-ctx.Done():
		return types.NotifyDeliveryResult{}, ctx.Err()
	}
	select {
	case <-local.release:
		return local.result, local.err
	case <-ctx.Done():
		return types.NotifyDeliveryResult{}, ctx.Err()
	}
}

func (local *blockingConversationSignalLocal) EvictDevice(context.Context, string, string, string, string) (types.SessionEvictionResult, error) {
	return types.SessionEvictionResult{}, nil
}

func (local *blockingConversationSignalLocal) EvictSession(context.Context, string, string, string, string, string) (types.SessionEvictionResult, error) {
	return types.SessionEvictionResult{}, nil
}

type noopRedisClient struct {
	redis.UniversalClient
}
