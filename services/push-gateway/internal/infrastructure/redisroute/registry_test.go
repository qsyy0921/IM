package redisroute

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/qsyy0921/IM/services/push-gateway/internal/domain"
	"github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/memory"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
	"github.com/redis/go-redis/v9"
)

func TestRegistryWritesAndDeletesRoute(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	local := memory.NewRegistry()
	registry := NewRegistry(local, client, Config{GatewayID: "gateway-a", RouteTTL: time.Minute})

	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"},
		SessionID:   "session-1",
		Outbound:    make(chan types.ServerFrame, 1),
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	sessionKey := registry.sessionKey("tenant-1", "user-1", "session-1")
	userKey := registry.userKey("tenant-1", "user-1")
	if !server.Exists(sessionKey) {
		t.Fatalf("expected session route key")
	}
	if !redisSetHasMember(t, server, userKey, "session-1") {
		t.Fatalf("expected user route membership")
	}

	registry.Unregister("session-1")
	if server.Exists(sessionKey) {
		t.Fatalf("session route key should be removed")
	}
	if redisSetHasMember(t, server, userKey, "session-1") {
		t.Fatalf("user route membership should be removed")
	}
}

func TestRegistryRouteKeysUseClusterHashTag(t *testing.T) {
	registry := NewRegistry(memory.NewRegistry(), nil, Config{})
	userKey := registry.userKey("tenant-1", "user-1")
	sessionKey := registry.sessionKey("tenant-1", "user-1", "session-1")
	if redisHashTagForTest(userKey) == "" || redisHashTagForTest(userKey) != redisHashTagForTest(sessionKey) {
		t.Fatalf("expected user and session route keys to share hash tag: user=%s session=%s", userKey, sessionKey)
	}
	if sessionKey != registry.sessionKeyForUserKey(userKey, "session-1") {
		t.Fatalf("session key reconstructed from user key does not match")
	}
	metaKey := registry.resumeMetaKey("resume-token-1")
	framesKey := registry.resumeFramesKey("resume-token-1")
	if redisHashTagForTest(metaKey) == "" || redisHashTagForTest(metaKey) != redisHashTagForTest(framesKey) {
		t.Fatalf("expected resume keys to share hash tag: meta=%s frames=%s", metaKey, framesKey)
	}
}

func TestRegistryPublishesRemoteRouteAndEnqueuesLocal(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	local := memory.NewRegistry()
	registry := NewRegistry(local, client, Config{GatewayID: "gateway-a", RouteTTL: time.Minute})

	outbound := make(chan types.ServerFrame, 1)
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-local"},
		SessionID:   "local-session",
		Outbound:    outbound,
	}); err != nil {
		t.Fatalf("register local route: %v", err)
	}
	if err := registry.writeRoute(context.Background(), routeEntry{
		TenantID:  "tenant-1",
		UserID:    "user-1",
		DeviceID:  "device-remote",
		SessionID: "remote-session",
		GatewayID: "gateway-b",
	}); err != nil {
		t.Fatalf("write remote route: %v", err)
	}
	if err := registry.writeRoute(context.Background(), routeEntry{
		TenantID:  "tenant-1",
		UserID:    "user-1",
		DeviceID:  "device-remote-2",
		SessionID: "remote-session-2",
		GatewayID: "gateway-b",
	}); err != nil {
		t.Fatalf("write second remote route: %v", err)
	}

	pubsub := client.Subscribe(context.Background(), GatewayChannel(defaultKeyPrefix, "gateway-b"))
	defer pubsub.Close()
	if _, err := pubsub.Receive(context.Background()); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	notification := testNotification()
	result, err := registry.EnqueueNotification(context.Background(), notification)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if result.Enqueued != 3 || result.MatchedSessions != 3 || len(outbound) != 1 {
		t.Fatalf("unexpected result=%+v local_queue=%d", result, len(outbound))
	}
	metrics := registry.Metrics()
	if metrics.RedisRouteRemoteMatchedSessions != 2 ||
		metrics.RedisRouteRemotePublishCallCount != 1 ||
		metrics.RedisRouteRemoteEnqueuedSessions != 2 ||
		metrics.RedisRouteRemoteNoSubscriberCount != 0 ||
		metrics.RedisRouteRemotePublishErrorCount != 0 {
		t.Fatalf("unexpected redis route metrics: %+v", metrics)
	}
	select {
	case message := <-pubsub.Channel():
		var forwarded types.DeliveryNotification
		if err := json.Unmarshal([]byte(message.Payload), &forwarded); err != nil {
			t.Fatalf("decode forwarded notification: %v", err)
		}
		if forwarded.EventID != notification.EventID {
			t.Fatalf("unexpected forwarded notification: %+v", forwarded)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for remote publish")
	}
	select {
	case message := <-pubsub.Channel():
		t.Fatalf("unexpected duplicate remote publish: %+v", message)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestRegistryPublishesRemoteConversationSignalToSubscribedGateway(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gatewayA := NewRegistry(memory.NewRegistry(), client, Config{GatewayID: "gateway-a", RouteTTL: time.Minute})
	localB := memory.NewRegistry()
	gatewayB := NewRegistry(localB, client, Config{GatewayID: "gateway-b", RouteTTL: time.Minute})
	outbound := make(chan types.ServerFrame, 2)
	auth := types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1", SessionID: "session-b"}
	if _, err := gatewayB.Register(ctx, types.SessionRegistration{
		AuthContext: auth,
		SessionID:   "session-b",
		Outbound:    outbound,
	}); err != nil {
		t.Fatalf("register remote session: %v", err)
	}
	if _, err := gatewayB.SubscribeConversation(ctx, types.ConversationSubscriptionCommand{
		AuthContext:    auth,
		ConversationID: "conversation-1",
	}); err != nil {
		t.Fatalf("subscribe remote conversation: %v", err)
	}
	done := make(chan error, 1)
	subscriber := NewSubscriber(localB, client, SubscriberConfig{GatewayID: "gateway-b"})
	go func() {
		done <- subscriber.Run(ctx)
	}()
	time.Sleep(50 * time.Millisecond)

	notification := testNotification()
	notification.Kind = types.DeliveryNotificationKindConversationSignal
	notification.UserID = ""
	result, err := gatewayA.EnqueueConversationSignal(ctx, notification)
	if err != nil {
		t.Fatalf("enqueue remote conversation signal: %v", err)
	}
	if result.MatchedSessions != 1 || result.Enqueued != 1 {
		t.Fatalf("unexpected remote signal result: %+v", result)
	}
	select {
	case frame := <-outbound:
		if frame.Op != types.OpDeliveryNotify || frame.EventID != notification.EventID || !frame.PullRequired {
			t.Fatalf("unexpected remote signal frame: %+v", frame)
		}
		waitForSubscriberMetrics(t, subscriber, func(metrics Metrics) bool {
			return metrics.RedisRouteSubscriberMessageCount == 1 &&
				metrics.RedisRouteSubscriberEnqueuedCount == 1 &&
				metrics.RedisRouteSubscriberSignalFanoutDuration.Count == 1 &&
				metrics.RedisRouteSubscriberNotifyFanoutDuration.Count == 0
		})
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for remote conversation signal")
	}
	cancel()
	<-done
}

func TestRegistryConversationSignalSamplingSkipsRemotePublish(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	ctx := context.Background()

	gatewayA := NewRegistry(memory.NewRegistry(), client, Config{
		GatewayID: "gateway-a",
		RouteTTL:  time.Minute,
		ConversationSignalPolicy: types.ConversationSignalPolicy{
			DefaultSampleEvery: 2,
		},
	})
	localB := memory.NewRegistry()
	gatewayB := NewRegistry(localB, client, Config{GatewayID: "gateway-b", RouteTTL: time.Minute})
	outbound := make(chan types.ServerFrame, 2)
	auth := types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1", SessionID: "session-b"}
	if _, err := gatewayB.Register(ctx, types.SessionRegistration{
		AuthContext: auth,
		SessionID:   "session-b",
		Outbound:    outbound,
	}); err != nil {
		t.Fatalf("register remote session: %v", err)
	}
	if _, err := gatewayB.SubscribeConversation(ctx, types.ConversationSubscriptionCommand{
		AuthContext:    auth,
		ConversationID: "conversation-1",
	}); err != nil {
		t.Fatalf("subscribe remote conversation: %v", err)
	}
	pubsub := client.Subscribe(ctx, GatewayChannel(defaultKeyPrefix, "gateway-b"))
	defer pubsub.Close()
	if _, err := pubsub.Receive(ctx); err != nil {
		t.Fatalf("subscribe gateway channel: %v", err)
	}

	notification := testNotification()
	notification.Kind = types.DeliveryNotificationKindConversationSignal
	notification.UserID = ""
	notification.ConversationSeq = 1
	result, err := gatewayA.EnqueueConversationSignal(ctx, notification)
	if err != nil {
		t.Fatalf("enqueue sampled-out signal: %v", err)
	}
	if result.MatchedSessions != 0 || result.Enqueued != 0 {
		t.Fatalf("sampled-out signal should stop before remote route lookup: %+v", result)
	}
	if metrics := gatewayA.Metrics(); metrics.RedisRouteConversationSignalSuppressedEventCount != 1 ||
		metrics.RedisRouteConversationSignalSuppressedSessionCount != 0 {
		t.Fatalf("unexpected sampling metrics: %+v", metrics)
	}
	select {
	case message := <-pubsub.Channel():
		t.Fatalf("sampled-out signal must not publish remotely: %+v", message)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestRegistryConversationSignalPolicyUsesFanoutModeOverride(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	ctx := context.Background()

	gatewayA := NewRegistry(memory.NewRegistry(), client, Config{
		GatewayID: "gateway-a",
		RouteTTL:  time.Minute,
		ConversationSignalPolicy: types.ConversationSignalPolicy{
			DefaultSampleEvery: 1,
			ByFanoutMode: map[string]int{
				types.FanoutModeReadFanout: 3,
			},
		},
	})
	localB := memory.NewRegistry()
	gatewayB := NewRegistry(localB, client, Config{GatewayID: "gateway-b", RouteTTL: time.Minute})
	outbound := make(chan types.ServerFrame, 2)
	auth := types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1", SessionID: "session-b"}
	if _, err := gatewayB.Register(ctx, types.SessionRegistration{
		AuthContext: auth,
		SessionID:   "session-b",
		Outbound:    outbound,
	}); err != nil {
		t.Fatalf("register remote session: %v", err)
	}
	if _, err := gatewayB.SubscribeConversation(ctx, types.ConversationSubscriptionCommand{
		AuthContext:    auth,
		ConversationID: "conversation-1",
	}); err != nil {
		t.Fatalf("subscribe remote conversation: %v", err)
	}
	pubsub := client.Subscribe(ctx, GatewayChannel(defaultKeyPrefix, "gateway-b"))
	defer pubsub.Close()
	if _, err := pubsub.Receive(ctx); err != nil {
		t.Fatalf("subscribe gateway channel: %v", err)
	}

	readFanout := testNotification()
	readFanout.Kind = types.DeliveryNotificationKindConversationSignal
	readFanout.UserID = ""
	readFanout.FanoutMode = types.FanoutModeReadFanout
	readFanout.ConversationSeq = 2
	result, err := gatewayA.EnqueueConversationSignal(ctx, readFanout)
	if err != nil {
		t.Fatalf("read fanout signal: %v", err)
	}
	if result.MatchedSessions != 0 || result.Enqueued != 0 {
		t.Fatalf("READ_FANOUT seq=2 should be suppressed: %+v", result)
	}

	writeFanout := readFanout
	writeFanout.EventID = "delivery-event-write-fanout"
	writeFanout.FanoutMode = types.FanoutModeWriteFanout
	result, err = gatewayA.EnqueueConversationSignal(ctx, writeFanout)
	if err != nil {
		t.Fatalf("write fanout signal: %v", err)
	}
	if result.MatchedSessions != 1 || result.Enqueued != 1 {
		t.Fatalf("WRITE_FANOUT should use default full signal policy: %+v", result)
	}
	select {
	case message := <-pubsub.Channel():
		var forwarded types.DeliveryNotification
		if err := json.Unmarshal([]byte(message.Payload), &forwarded); err != nil {
			t.Fatalf("decode forwarded notification: %v", err)
		}
		if forwarded.FanoutMode != types.FanoutModeWriteFanout {
			t.Fatalf("unexpected forwarded mode: %+v", forwarded)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for remote publish")
	}
}

func TestRegistryConversationSignalPolicyUsesRemoteSubscriberThreshold(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	ctx := context.Background()

	gatewayA := NewRegistry(memory.NewRegistry(), client, Config{
		GatewayID: "gateway-a",
		RouteTTL:  time.Minute,
		ConversationSignalPolicy: types.ConversationSignalPolicy{
			DefaultSampleEvery: 1,
			ByFanoutMode: map[string]int{
				types.FanoutModeReadFanout: 2,
			},
			SubscriberThresholdsByFanoutMode: map[string][]types.ConversationSignalSubscriberThreshold{
				types.FanoutModeReadFanout: {
					{MinSubscribers: 2, SampleEvery: 5},
				},
			},
		},
	})
	localB := memory.NewRegistry()
	gatewayB := NewRegistry(localB, client, Config{GatewayID: "gateway-b", RouteTTL: time.Minute})
	for index := 0; index < 2; index++ {
		sessionID := "remote-threshold-session-" + string(rune('a'+index))
		auth := types.AuthContext{
			TenantID:  "tenant-1",
			UserID:    "remote-threshold-user-" + string(rune('a'+index)),
			DeviceID:  "remote-threshold-device-" + string(rune('a'+index)),
			SessionID: sessionID,
		}
		if _, err := gatewayB.Register(ctx, types.SessionRegistration{
			AuthContext: auth,
			SessionID:   sessionID,
			Outbound:    make(chan types.ServerFrame, 2),
		}); err != nil {
			t.Fatalf("register remote session %d: %v", index, err)
		}
		if _, err := gatewayB.SubscribeConversation(ctx, types.ConversationSubscriptionCommand{
			AuthContext:    auth,
			ConversationID: "conversation-1",
		}); err != nil {
			t.Fatalf("subscribe remote session %d: %v", index, err)
		}
	}
	pubsub := client.Subscribe(ctx, GatewayChannel(defaultKeyPrefix, "gateway-b"))
	defer pubsub.Close()
	if _, err := pubsub.Receive(ctx); err != nil {
		t.Fatalf("subscribe gateway channel: %v", err)
	}

	suppressed := testNotification()
	suppressed.Kind = types.DeliveryNotificationKindConversationSignal
	suppressed.UserID = ""
	suppressed.FanoutMode = types.FanoutModeReadFanout
	suppressed.ConversationSeq = 4
	result, err := gatewayA.EnqueueConversationSignal(ctx, suppressed)
	if err != nil {
		t.Fatalf("enqueue threshold suppressed signal: %v", err)
	}
	if result.MatchedSessions != 2 || result.Enqueued != 0 {
		t.Fatalf("threshold suppressed signal should match but not publish: %+v", result)
	}
	select {
	case message := <-pubsub.Channel():
		t.Fatalf("threshold suppressed signal must not publish remotely: %+v", message)
	case <-time.After(100 * time.Millisecond):
	}

	emitted := suppressed
	emitted.EventID = "delivery-event-threshold-5"
	emitted.ConversationSeq = 5
	result, err = gatewayA.EnqueueConversationSignal(ctx, emitted)
	if err != nil {
		t.Fatalf("enqueue threshold emitted signal: %v", err)
	}
	if result.MatchedSessions != 2 || result.Enqueued != 2 {
		t.Fatalf("threshold emitted signal should publish once for two sessions: %+v", result)
	}
	select {
	case message := <-pubsub.Channel():
		var forwarded types.DeliveryNotification
		if err := json.Unmarshal([]byte(message.Payload), &forwarded); err != nil {
			t.Fatalf("decode forwarded notification: %v", err)
		}
		if forwarded.EventID != emitted.EventID {
			t.Fatalf("unexpected forwarded signal: %+v", forwarded)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for threshold emitted publish")
	}
	metrics := gatewayA.Metrics()
	if metrics.RedisRouteConversationSignalSuppressedEventCount != 1 ||
		metrics.RedisRouteConversationSignalSuppressedSessionCount != 2 ||
		metrics.RedisRouteRemotePublishCallCount != 1 ||
		metrics.RedisRouteRemoteEnqueuedSessions != 2 {
		t.Fatalf("unexpected threshold metrics: %+v", metrics)
	}
}

func TestRegistryPublishesRemoteDeviceEvictionAndSubscriberEvicts(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gatewayA := NewRegistry(memory.NewRegistry(), client, Config{GatewayID: "gateway-a", RouteTTL: time.Minute})
	localB := memory.NewRegistry()
	gatewayB := NewRegistry(localB, client, Config{GatewayID: "gateway-b", RouteTTL: time.Minute})
	evicted := make(chan types.SessionEviction, 1)
	if _, err := gatewayB.Register(ctx, types.SessionRegistration{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"},
		SessionID:   "session-b",
		Outbound:    make(chan types.ServerFrame, 1),
		Evicted:     evicted,
	}); err != nil {
		t.Fatalf("register remote session: %v", err)
	}
	done := make(chan error, 1)
	subscriber := NewSubscriber(localB, client, SubscriberConfig{GatewayID: "gateway-b"})
	go func() {
		done <- subscriber.Run(ctx)
	}()
	time.Sleep(50 * time.Millisecond)

	result, err := gatewayA.EvictDevice(ctx, "tenant-1", "user-1", "device-1", "identity_revoked")
	if err != nil {
		t.Fatalf("evict remote device: %v", err)
	}
	if result.MatchedSessions != 1 || result.Evicted != 1 {
		t.Fatalf("unexpected remote eviction result: %+v", result)
	}
	select {
	case eviction := <-evicted:
		if eviction.Reason != "identity_revoked" {
			t.Fatalf("unexpected eviction: %+v", eviction)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for remote eviction")
	}
	if metrics := subscriber.Metrics(); metrics.RedisRouteSubscriberEvictedCount != 1 {
		t.Fatalf("unexpected subscriber metrics: %+v", metrics)
	}
	cancel()
	<-done
}

func TestRegistryPublishesRemoteSessionEvictionOnlyForTarget(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gatewayA := NewRegistry(memory.NewRegistry(), client, Config{GatewayID: "gateway-a", RouteTTL: time.Minute})
	localB := memory.NewRegistry()
	gatewayB := NewRegistry(localB, client, Config{GatewayID: "gateway-b", RouteTTL: time.Minute})
	targetEvicted := make(chan types.SessionEviction, 1)
	otherEvicted := make(chan types.SessionEviction, 1)
	if _, err := gatewayB.Register(ctx, types.SessionRegistration{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"},
		SessionID:   "session-target",
		Outbound:    make(chan types.ServerFrame, 1),
		Evicted:     targetEvicted,
	}); err != nil {
		t.Fatalf("register target session: %v", err)
	}
	if _, err := gatewayB.Register(ctx, types.SessionRegistration{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"},
		SessionID:   "session-other",
		Outbound:    make(chan types.ServerFrame, 1),
		Evicted:     otherEvicted,
	}); err != nil {
		t.Fatalf("register other session: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- NewSubscriber(localB, client, SubscriberConfig{GatewayID: "gateway-b"}).Run(ctx)
	}()
	time.Sleep(50 * time.Millisecond)

	result, err := gatewayA.EvictSession(ctx, "tenant-1", "user-1", "device-1", "session-target", "identity_revoked")
	if err != nil {
		t.Fatalf("evict remote session: %v", err)
	}
	if result.MatchedSessions != 1 || result.Evicted != 1 {
		t.Fatalf("unexpected remote session eviction result: %+v", result)
	}
	select {
	case eviction := <-targetEvicted:
		if eviction.Reason != "identity_revoked" {
			t.Fatalf("unexpected target eviction: %+v", eviction)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for target eviction")
	}
	select {
	case eviction := <-otherEvicted:
		t.Fatalf("other session should not be evicted: %+v", eviction)
	case <-time.After(100 * time.Millisecond):
	}
	cancel()
	<-done
}

func TestRegistryRenewsRouteTTLUntilUnregister(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	local := memory.NewRegistry()
	registry := NewRegistry(local, client, Config{
		GatewayID: "gateway-a",
		RouteTTL:  3 * time.Second,
	})

	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"},
		SessionID:   "session-1",
		Outbound:    make(chan types.ServerFrame, 1),
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	time.Sleep(1200 * time.Millisecond)
	server.FastForward(2500 * time.Millisecond)
	if !server.Exists(registry.sessionKey("tenant-1", "user-1", "session-1")) {
		t.Fatalf("expected session route key to be renewed before TTL expiry")
	}

	registry.Unregister("session-1")
	if server.Exists(registry.sessionKey("tenant-1", "user-1", "session-1")) {
		t.Fatalf("session route key should be removed after unregister")
	}
}

func TestRegistryEvictsSessionAfterRepeatedRenewFailures(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	local := memory.NewRegistry()
	registry := NewRegistry(local, client, Config{
		GatewayID:             "gateway-a",
		RouteTTL:              150 * time.Millisecond,
		RenewFailureThreshold: 2,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	evicted := make(chan types.SessionEviction, 1)
	if _, err := registry.Register(ctx, types.SessionRegistration{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"},
		SessionID:   "session-1",
		Outbound:    make(chan types.ServerFrame, 1),
		Evicted:     evicted,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	server.Close()

	select {
	case eviction := <-evicted:
		if eviction.Reason != "redis_route_unavailable" {
			t.Fatalf("unexpected eviction: %+v", eviction)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for renew-failure eviction")
	}

	metrics := registry.Metrics()
	if metrics.RedisRouteRenewErrorCount == 0 {
		t.Fatalf("expected renew error count, got %+v", metrics)
	}
	if metrics.RedisRouteRenewSessionEvictedCount != 1 {
		t.Fatalf("expected one renew-failure eviction, got %+v", metrics)
	}
}

func TestRegistryCleansStaleRoutesDuringLookup(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	local := memory.NewRegistry()
	registry := NewRegistry(local, client, Config{GatewayID: "gateway-a", RouteTTL: time.Minute})
	ctx := context.Background()
	userKey := registry.userKey("tenant-1", "user-1")

	if err := client.SAdd(ctx, userKey,
		"missing-session",
		"malformed-session",
		"wrong-user-session",
	).Err(); err != nil {
		t.Fatalf("seed user set: %v", err)
	}
	if err := client.Set(ctx, registry.sessionKeyForUserKey(userKey, "malformed-session"), "{", time.Minute).Err(); err != nil {
		t.Fatalf("seed malformed route: %v", err)
	}
	if err := registry.writeRoute(ctx, routeEntry{
		TenantID:  "tenant-1",
		UserID:    "other-user",
		DeviceID:  "device-1",
		SessionID: "wrong-user-session",
		GatewayID: "gateway-b",
	}); err != nil {
		t.Fatalf("seed wrong user route: %v", err)
	}
	if err := client.SAdd(ctx, userKey, "wrong-user-session").Err(); err != nil {
		t.Fatalf("re-seed wrong user membership: %v", err)
	}

	result, err := registry.EnqueueNotification(ctx, testNotification())
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if result.MatchedSessions != 0 || result.Enqueued != 0 {
		t.Fatalf("unexpected result for stale-only routes: %+v", result)
	}
	for _, sessionID := range []string{"missing-session", "malformed-session", "wrong-user-session"} {
		if redisSetHasMember(t, server, userKey, sessionID) {
			t.Fatalf("expected stale session %s to be removed from user set", sessionID)
		}
	}
	if metrics := registry.Metrics(); metrics.RedisRouteStaleRemovedCount != 3 {
		t.Fatalf("expected stale removal metric, got %+v", metrics)
	}
}

func TestRegistryCleanupStaleRoutesScansUserSets(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	local := memory.NewRegistry()
	registry := NewRegistry(local, client, Config{GatewayID: "gateway-a", RouteTTL: time.Minute})
	ctx := context.Background()
	userKey := registry.userKey("tenant-1", "user-1")

	if err := client.SAdd(ctx, userKey,
		"valid-session",
		"missing-session",
		"malformed-session",
		"wrong-user-session",
	).Err(); err != nil {
		t.Fatalf("seed user set: %v", err)
	}
	if err := registry.writeRoute(ctx, routeEntry{
		TenantID:  "tenant-1",
		UserID:    "user-1",
		DeviceID:  "device-1",
		SessionID: "valid-session",
		GatewayID: "gateway-b",
	}); err != nil {
		t.Fatalf("seed valid route: %v", err)
	}
	if err := client.Set(ctx, registry.sessionKeyForUserKey(userKey, "malformed-session"), "{", time.Minute).Err(); err != nil {
		t.Fatalf("seed malformed route: %v", err)
	}
	if err := registry.writeRoute(ctx, routeEntry{
		TenantID:  "tenant-1",
		UserID:    "other-user",
		DeviceID:  "device-2",
		SessionID: "wrong-user-session",
		GatewayID: "gateway-b",
	}); err != nil {
		t.Fatalf("seed wrong user route: %v", err)
	}
	if err := client.SAdd(ctx, userKey, "wrong-user-session").Err(); err != nil {
		t.Fatalf("re-seed wrong user membership: %v", err)
	}

	removed, err := registry.CleanupStaleRoutes(ctx)
	if err != nil {
		t.Fatalf("cleanup stale routes: %v", err)
	}
	if removed != 3 {
		t.Fatalf("expected 3 stale routes removed, got %d", removed)
	}
	if !redisSetHasMember(t, server, userKey, "valid-session") {
		t.Fatalf("valid route should remain")
	}
	for _, sessionID := range []string{"missing-session", "malformed-session", "wrong-user-session"} {
		if redisSetHasMember(t, server, userKey, sessionID) {
			t.Fatalf("expected stale session %s to be removed", sessionID)
		}
	}
}

func TestRegistryFailsOpenWhenRedisLookupUnavailable(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	local := memory.NewRegistry()
	registry := NewRegistry(local, client, Config{GatewayID: "gateway-a", RouteTTL: time.Minute})

	server.Close()
	result, err := registry.EnqueueNotification(context.Background(), testNotification())
	if err != nil {
		t.Fatalf("enqueue should fail open when redis lookup is unavailable: %v", err)
	}
	if result.Enqueued != 0 || result.MatchedSessions != 0 || result.Dropped != 1 {
		t.Fatalf("unexpected local result after redis failure: %+v", result)
	}
	if metrics := registry.Metrics(); metrics.RedisRouteLookupErrorCount != 1 {
		t.Fatalf("expected lookup error metric, got %+v", metrics)
	}
}

func TestRegistryRemoteNotificationWithoutSubscriberCountsDropped(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	registry := NewRegistry(memory.NewRegistry(), client, Config{GatewayID: "gateway-a", RouteTTL: time.Minute})

	if err := registry.writeRoute(context.Background(), routeEntry{
		TenantID:  "tenant-1",
		UserID:    "user-1",
		DeviceID:  "device-remote",
		SessionID: "remote-session",
		GatewayID: "gateway-b",
	}); err != nil {
		t.Fatalf("write remote route: %v", err)
	}

	result, err := registry.EnqueueNotification(context.Background(), testNotification())
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if result.MatchedSessions != 1 || result.Enqueued != 0 || result.Dropped != 1 {
		t.Fatalf("unexpected no-subscriber notify result: %+v", result)
	}
	metrics := registry.Metrics()
	if metrics.RedisRouteRemotePublishCallCount != 1 ||
		metrics.RedisRouteRemotePublishErrorCount != 0 ||
		metrics.RedisRouteRemoteNoSubscriberCount != 1 ||
		metrics.RedisRouteRemoteEnqueuedSessions != 0 {
		t.Fatalf("unexpected no-subscriber metrics: %+v", metrics)
	}
}

func TestRegistryRegisterFailsClosedWhenRedisUnavailable(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	local := memory.NewRegistry()
	registry := NewRegistry(local, client, Config{GatewayID: "gateway-a", RouteTTL: time.Minute})

	server.Close()
	_, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"},
		SessionID:   "session-1",
		Outbound:    make(chan types.ServerFrame, 1),
	})
	if err == nil {
		t.Fatalf("expected register to fail when redis route cannot be written")
	}
	if metrics := local.Metrics(); metrics.ConnectedSessions != 0 {
		t.Fatalf("local session should be rolled back after redis write failure: %+v", metrics)
	}
}

func TestRegistryRemoteEvictionWithoutSubscriberDoesNotCountEvicted(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	ctx := context.Background()
	registry := NewRegistry(memory.NewRegistry(), client, Config{GatewayID: "gateway-a", RouteTTL: time.Minute})

	if err := registry.writeRoute(ctx, routeEntry{
		TenantID:  "tenant-1",
		UserID:    "user-1",
		DeviceID:  "device-1",
		SessionID: "session-b",
		GatewayID: "gateway-b",
	}); err != nil {
		t.Fatalf("write remote route: %v", err)
	}

	result, err := registry.EvictDevice(ctx, "tenant-1", "user-1", "device-1", "identity_revoked")
	if err != nil {
		t.Fatalf("evict remote device: %v", err)
	}
	if result.MatchedSessions != 1 || result.Evicted != 0 {
		t.Fatalf("unexpected no-subscriber eviction result: %+v", result)
	}
	metrics := registry.Metrics()
	if metrics.RedisRouteRemoteNoSubscriberCount != 1 || metrics.RedisRouteRemotePublishErrorCount != 0 {
		t.Fatalf("unexpected no-subscriber eviction metrics: %+v", metrics)
	}
}

func TestRegistryReplaysRedisResumeAcrossGateways(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	ctx := context.Background()
	auth := types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"}
	token := "resume-known"

	localA := memory.NewRegistry()
	gatewayA := NewRegistry(localA, client, Config{GatewayID: "gateway-a", RouteTTL: time.Minute})
	if _, err := gatewayA.Register(ctx, types.SessionRegistration{
		AuthContext: auth,
		SessionID:   "session-a",
		ResumeToken: token,
		Outbound:    make(chan types.ServerFrame, 1),
	}); err != nil {
		t.Fatalf("register gateway A: %v", err)
	}

	gatewayB := NewRegistry(memory.NewRegistry(), client, Config{GatewayID: "gateway-b", RouteTTL: time.Minute})
	notification := testNotification()
	result, err := gatewayB.EnqueueNotification(ctx, notification)
	if err != nil {
		t.Fatalf("enqueue from gateway B: %v", err)
	}
	if result.MatchedSessions != 1 || result.Enqueued != 0 || result.Dropped != 1 {
		t.Fatalf("unexpected enqueue result: %+v", result)
	}
	if metrics := gatewayB.Metrics(); metrics.RedisResumeAppendCount != 1 || metrics.RedisRouteRemoteNoSubscriberCount != 1 {
		t.Fatalf("expected redis resume append metric, got %+v", metrics)
	}

	gatewayA.Unregister("session-a")
	outbound := make(chan types.ServerFrame, 2)
	gatewayC := NewRegistry(memory.NewRegistry(), client, Config{GatewayID: "gateway-c", RouteTTL: time.Minute})
	resumed, err := gatewayC.Register(ctx, types.SessionRegistration{
		AuthContext:     auth,
		SessionID:       "session-c",
		ResumeToken:     token,
		ResumeRequested: true,
		LastReceived:    []types.ConversationCursor{{ConversationID: notification.ConversationID, Seq: notification.ConversationSeq - 1}},
		Outbound:        outbound,
	})
	if err != nil {
		t.Fatalf("resume on gateway C: %v", err)
	}
	if resumed.ResumeToken != token {
		t.Fatalf("expected known redis token to be reused, got %s", resumed.ResumeToken)
	}
	select {
	case frame := <-outbound:
		if frame.Op != types.OpDeliveryNotify || frame.EventID != notification.EventID {
			t.Fatalf("unexpected replay frame: %+v", frame)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for redis resume replay")
	}
	if metrics := gatewayC.Metrics(); metrics.RedisResumeReplayCount != 1 || metrics.RedisResumeMissCount != 0 {
		t.Fatalf("unexpected resume metrics: %+v", metrics)
	}
}

func TestRegistryUnknownRedisResumeTokenIssuesNewTokenAndBufferMiss(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	registry := NewRegistry(memory.NewRegistry(), client, Config{GatewayID: "gateway-a", RouteTTL: time.Minute})
	ctx := context.Background()
	outbound := make(chan types.ServerFrame, 1)

	result, err := registry.Register(ctx, types.SessionRegistration{
		AuthContext:     types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"},
		SessionID:       "session-1",
		ResumeToken:     "client-chosen-token",
		ResumeRequested: true,
		Outbound:        outbound,
	})
	if err != nil {
		t.Fatalf("register unknown resume token: %v", err)
	}
	if result.ResumeToken == "" || result.ResumeToken == "client-chosen-token" {
		t.Fatalf("expected server-generated replacement token, got %q", result.ResumeToken)
	}
	if server.Exists(registry.resumeMetaKey("client-chosen-token")) {
		t.Fatalf("unknown client token should not be registered")
	}
	if !server.Exists(registry.resumeMetaKey(result.ResumeToken)) {
		t.Fatalf("replacement token metadata should be stored in redis")
	}
	select {
	case frame := <-outbound:
		if frame.Op != types.OpResumeHint || frame.Reason != "buffer_miss" {
			t.Fatalf("unexpected frame: %+v", frame)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for buffer miss")
	}
	if metrics := registry.Metrics(); metrics.RedisResumeMissCount != 1 {
		t.Fatalf("expected redis resume miss metric, got %+v", metrics)
	}
}

func TestRegistryRejectsRedisResumeTokenForDifferentDevice(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	ctx := context.Background()
	registry := NewRegistry(memory.NewRegistry(), client, Config{GatewayID: "gateway-a", RouteTTL: time.Minute})
	token := "resume-device-1"

	if _, err := registry.Register(ctx, types.SessionRegistration{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"},
		SessionID:   "session-1",
		ResumeToken: token,
		Outbound:    make(chan types.ServerFrame, 1),
	}); err != nil {
		t.Fatalf("register first device: %v", err)
	}
	_, err := registry.Register(ctx, types.SessionRegistration{
		AuthContext:     types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-2"},
		SessionID:       "session-2",
		ResumeToken:     token,
		ResumeRequested: true,
		Outbound:        make(chan types.ServerFrame, 1),
	})
	if !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected permission denied, got %v", err)
	}
	if metrics := registry.Metrics(); metrics.RedisResumePermissionDeniedCount != 1 {
		t.Fatalf("expected permission denied metric, got %+v", metrics)
	}
}

func TestRegistryRedisResumeGapReturnsBufferMiss(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	ctx := context.Background()
	registry := NewRegistry(memory.NewRegistry(), client, Config{GatewayID: "gateway-a", RouteTTL: time.Minute})
	auth := types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"}
	token := "resume-gap"

	if err := registry.writeResumeMeta(ctx, token, auth); err != nil {
		t.Fatalf("write resume meta: %v", err)
	}
	notification := testNotification()
	notification.ConversationSeq = 10
	if err := registry.appendRedisResume(ctx, token, domain.DeliveryNotify(notification)); err != nil {
		t.Fatalf("append resume frame: %v", err)
	}

	outbound := make(chan types.ServerFrame, 2)
	result, err := registry.Register(ctx, types.SessionRegistration{
		AuthContext:     auth,
		SessionID:       "session-1",
		ResumeToken:     token,
		ResumeRequested: true,
		LastReceived:    []types.ConversationCursor{{ConversationID: notification.ConversationID, Seq: 7}},
		Outbound:        outbound,
	})
	if err != nil {
		t.Fatalf("register with gapped resume token: %v", err)
	}
	if result.ResumeToken != token {
		t.Fatalf("known token should be preserved on gap, got %q", result.ResumeToken)
	}
	select {
	case frame := <-outbound:
		if frame.Op != types.OpResumeHint || frame.Reason != "buffer_miss" {
			t.Fatalf("expected buffer miss, got %+v", frame)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for buffer miss")
	}
	select {
	case frame := <-outbound:
		t.Fatalf("should not replay after gap, got %+v", frame)
	default:
	}
	if metrics := registry.Metrics(); metrics.RedisResumeReplayCount != 0 || metrics.RedisResumeMissCount != 1 {
		t.Fatalf("unexpected gap metrics: %+v", metrics)
	}
}

func TestRegistryRedisResumeQueueTooSmallReturnsBufferMissWithoutPartialReplay(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	ctx := context.Background()
	registry := NewRegistry(memory.NewRegistry(), client, Config{GatewayID: "gateway-a", RouteTTL: time.Minute})
	auth := types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"}
	token := "resume-small-queue"

	if err := registry.writeResumeMeta(ctx, token, auth); err != nil {
		t.Fatalf("write resume meta: %v", err)
	}
	first := testNotification()
	first.ConversationSeq = 7
	if err := registry.appendRedisResume(ctx, token, domain.DeliveryNotify(first)); err != nil {
		t.Fatalf("append first resume frame: %v", err)
	}
	second := testNotification()
	second.EventID = "delivery-event-2"
	second.SourceEventID = "timeline-event-2"
	second.MessageID = "message-2"
	second.ConversationSeq = 8
	if err := registry.appendRedisResume(ctx, token, domain.DeliveryNotify(second)); err != nil {
		t.Fatalf("append second resume frame: %v", err)
	}

	outbound := make(chan types.ServerFrame, 1)
	if _, err := registry.Register(ctx, types.SessionRegistration{
		AuthContext:     auth,
		SessionID:       "session-1",
		ResumeToken:     token,
		ResumeRequested: true,
		LastReceived:    []types.ConversationCursor{{ConversationID: first.ConversationID, Seq: 6}},
		Outbound:        outbound,
	}); err != nil {
		t.Fatalf("register with small resume queue: %v", err)
	}
	select {
	case frame := <-outbound:
		if frame.Op != types.OpResumeHint || frame.Reason != "buffer_miss" {
			t.Fatalf("expected buffer miss, got %+v", frame)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for buffer miss")
	}
	select {
	case frame := <-outbound:
		t.Fatalf("should not partially replay redis resume frames, got %+v", frame)
	default:
	}
	if metrics := registry.Metrics(); metrics.RedisResumeReplayCount != 0 || metrics.RedisResumeMissCount != 1 {
		t.Fatalf("unexpected small queue metrics: %+v", metrics)
	}
}

func TestRegistryRedisResumeReplaysHiddenFrameAtAlreadyReceivedSeq(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	ctx := context.Background()
	registry := NewRegistry(memory.NewRegistry(), client, Config{GatewayID: "gateway-a", RouteTTL: time.Minute})
	auth := types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"}
	token := "resume-hide"

	if err := registry.writeResumeMeta(ctx, token, auth); err != nil {
		t.Fatalf("write resume meta: %v", err)
	}
	hidden := testNotification()
	hidden.Kind = types.DeliveryNotificationKindInboxItemHidden
	hidden.EventID = "delivery-event-hide-1"
	hidden.SourceEventType = "delivery.inbox_item.hidden.v1"
	if err := registry.appendRedisResume(ctx, token, domain.DeliveryNotify(hidden)); err != nil {
		t.Fatalf("append hidden resume frame: %v", err)
	}

	outbound := make(chan types.ServerFrame, 1)
	result, err := registry.Register(ctx, types.SessionRegistration{
		AuthContext:     auth,
		SessionID:       "session-1",
		ResumeToken:     token,
		ResumeRequested: true,
		LastReceived: []types.ConversationCursor{{
			ConversationID: hidden.ConversationID,
			Seq:            hidden.ConversationSeq,
		}},
		Outbound: outbound,
	})
	if err != nil {
		t.Fatalf("register with hidden resume token: %v", err)
	}
	if result.ResumeToken != token {
		t.Fatalf("known token should be preserved, got %q", result.ResumeToken)
	}
	select {
	case frame := <-outbound:
		if frame.Op != types.OpDeliveryHide || frame.EventID != hidden.EventID {
			t.Fatalf("unexpected hidden replay frame: %+v", frame)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for hidden replay")
	}
	if metrics := registry.Metrics(); metrics.RedisResumeReplayCount != 1 || metrics.RedisResumeMissCount != 0 {
		t.Fatalf("unexpected hidden replay metrics: %+v", metrics)
	}
}

func TestRegistryRedisLookupUnavailableKeepsLocalDeliveryAndResume(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	local := memory.NewRegistry()
	registry := NewRegistry(local, client, Config{GatewayID: "gateway-a", RouteTTL: time.Minute})
	ctx := context.Background()
	outbound := make(chan types.ServerFrame, 1)

	if _, err := registry.Register(ctx, types.SessionRegistration{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"},
		SessionID:   "session-1",
		ResumeToken: "resume-local",
		Outbound:    outbound,
	}); err != nil {
		t.Fatalf("register local session: %v", err)
	}
	server.Close()
	result, err := registry.EnqueueNotification(ctx, testNotification())
	if err != nil {
		t.Fatalf("enqueue should fail open after redis lookup error: %v", err)
	}
	if result.Enqueued != 1 || result.MatchedSessions != 1 || result.Dropped != 1 {
		t.Fatalf("expected local enqueue plus redis lookup drop marker, got %+v", result)
	}
	select {
	case frame := <-outbound:
		if frame.Op != types.OpDeliveryNotify || frame.EventID != "delivery-event-1" {
			t.Fatalf("unexpected local frame: %+v", frame)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for local delivery")
	}
	localMetrics := local.Metrics()
	if localMetrics.ResumeBufferStoredFrames != 1 {
		t.Fatalf("expected local resume buffer to keep frame, got %+v", localMetrics)
	}
	if metrics := registry.Metrics(); metrics.RedisRouteLookupErrorCount != 1 {
		t.Fatalf("expected lookup error metric, got %+v", metrics)
	}
}

func TestSubscriberEnqueuesRemoteNotificationLocally(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	local := memory.NewRegistry()
	outbound := make(chan types.ServerFrame, 1)
	if _, err := local.Register(context.Background(), types.SessionRegistration{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"},
		SessionID:   "session-1",
		Outbound:    outbound,
	}); err != nil {
		t.Fatalf("local register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	subscriber := NewSubscriber(local, client, SubscriberConfig{GatewayID: "gateway-a"})
	go func() {
		done <- subscriber.Run(ctx)
	}()
	time.Sleep(50 * time.Millisecond)

	payload, err := json.Marshal(testNotification())
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Publish(ctx, GatewayChannel(defaultKeyPrefix, "gateway-a"), payload).Err(); err != nil {
		t.Fatalf("publish: %v", err)
	}
	select {
	case frame := <-outbound:
		if frame.Op != types.OpDeliveryNotify || frame.EventID != "delivery-event-1" {
			t.Fatalf("unexpected frame: %+v", frame)
		}
		waitForSubscriberMetrics(t, subscriber, func(metrics Metrics) bool {
			return metrics.RedisRouteSubscriberMessageCount == 1 &&
				metrics.RedisRouteSubscriberEnqueuedCount == 1 &&
				metrics.RedisRouteSubscriberMalformedCount == 0 &&
				metrics.RedisRouteSubscriberNotifyFanoutDuration.Count == 1 &&
				metrics.RedisRouteSubscriberSignalFanoutDuration.Count == 0
		})
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for local enqueue")
	}
	cancel()
	<-done
}

func TestSubscriberSkipsMalformedPayloadAndContinues(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	local := memory.NewRegistry()
	outbound := make(chan types.ServerFrame, 1)
	if _, err := local.Register(context.Background(), types.SessionRegistration{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"},
		SessionID:   "session-1",
		Outbound:    outbound,
	}); err != nil {
		t.Fatalf("local register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	subscriber := NewSubscriber(local, client, SubscriberConfig{GatewayID: "gateway-a"})
	go func() {
		done <- subscriber.Run(ctx)
	}()
	time.Sleep(50 * time.Millisecond)

	channel := GatewayChannel(defaultKeyPrefix, "gateway-a")
	if err := client.Publish(ctx, channel, "{").Err(); err != nil {
		t.Fatalf("publish malformed payload: %v", err)
	}
	incompletePayload, err := json.Marshal(types.DeliveryNotification{
		EventID:  "delivery-event-incomplete",
		TenantID: "tenant-1",
		UserID:   "user-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Publish(ctx, channel, incompletePayload).Err(); err != nil {
		t.Fatalf("publish incomplete payload: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	payload, err := json.Marshal(testNotification())
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Publish(ctx, channel, payload).Err(); err != nil {
		t.Fatalf("publish valid payload: %v", err)
	}

	select {
	case frame := <-outbound:
		if frame.Op != types.OpDeliveryNotify || frame.EventID != "delivery-event-1" {
			t.Fatalf("unexpected frame: %+v", frame)
		}
		waitForSubscriberMetrics(t, subscriber, func(metrics Metrics) bool {
			return metrics.RedisRouteSubscriberMessageCount == 1 &&
				metrics.RedisRouteSubscriberEnqueuedCount == 1 &&
				metrics.RedisRouteSubscriberMalformedCount == 2
		})
	case err := <-done:
		t.Fatalf("subscriber exited after malformed payload: %v", err)
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for local enqueue")
	}
	cancel()
	<-done
}

func TestSubscriberRetriesRuntimeErrorAndExposesSnapshot(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	local := memory.NewRegistry()
	subscriber := NewSubscriber(local, client, SubscriberConfig{
		GatewayID:    "gateway-a",
		ErrorBackoff: time.Millisecond,
	})
	server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- subscriber.Run(ctx)
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snapshot := subscriber.Snapshot()
		if snapshot.TotalErrors > 0 && snapshot.LastErrorBackoffMS == time.Millisecond.Milliseconds() {
			cancel()
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Fatalf("expected canceled run, got %v", err)
			}
			if snapshot.ConsecutiveErrors == 0 || snapshot.LastErrorAtMS == 0 {
				t.Fatalf("unexpected retry snapshot: %+v", snapshot)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	t.Fatalf("timed out waiting for subscriber retry snapshot")
}

func testNotification() types.DeliveryNotification {
	return types.DeliveryNotification{
		EventID:         "delivery-event-1",
		TenantID:        "tenant-1",
		UserID:          "user-1",
		ConversationID:  "conversation-1",
		ConversationSeq: 7,
		SourceEventID:   "timeline-event-1",
		SourceEventType: "message.persisted.v1",
		MessageID:       "message-1",
		FanoutMode:      types.FanoutModeReadFanout,
	}
}

func redisSetHasMember(t *testing.T, server *miniredis.Miniredis, key string, member string) bool {
	t.Helper()
	if !server.Exists(key) {
		return false
	}
	ok, err := server.SIsMember(key, member)
	if err != nil {
		t.Fatalf("check set membership: %v", err)
	}
	return ok
}

func redisHashTagForTest(key string) string {
	start := -1
	for i, char := range key {
		if char == '{' {
			start = i
			break
		}
	}
	if start < 0 {
		return ""
	}
	for i := start + 1; i < len(key); i++ {
		if key[i] == '}' {
			return key[start+1 : i]
		}
	}
	return ""
}

func waitForSubscriberMetrics(t *testing.T, subscriber *Subscriber, ready func(Metrics) bool) Metrics {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		metrics := subscriber.Metrics()
		if ready(metrics) {
			return metrics
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for subscriber metrics, last=%+v", metrics)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
