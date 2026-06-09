package redisroute

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
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
	if !server.Exists("nexusim:push:route:session:session-1") {
		t.Fatalf("expected session route key")
	}
	if !redisSetHasMember(t, server, "nexusim:push:route:user:tenant-1:user-1", "session-1") {
		t.Fatalf("expected user route membership")
	}

	registry.Unregister("session-1")
	if server.Exists("nexusim:push:route:session:session-1") {
		t.Fatalf("session route key should be removed")
	}
	if redisSetHasMember(t, server, "nexusim:push:route:user:tenant-1:user-1", "session-1") {
		t.Fatalf("user route membership should be removed")
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
	if !server.Exists("nexusim:push:route:session:session-1") {
		t.Fatalf("expected session route key to be renewed before TTL expiry")
	}

	registry.Unregister("session-1")
	if server.Exists("nexusim:push:route:session:session-1") {
		t.Fatalf("session route key should be removed after unregister")
	}
}

func TestRegistryCleansStaleRoutesDuringLookup(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	local := memory.NewRegistry()
	registry := NewRegistry(local, client, Config{GatewayID: "gateway-a", RouteTTL: time.Minute})
	ctx := context.Background()

	if err := client.SAdd(ctx, "nexusim:push:route:user:tenant-1:user-1",
		"missing-session",
		"malformed-session",
		"wrong-user-session",
	).Err(); err != nil {
		t.Fatalf("seed user set: %v", err)
	}
	if err := client.Set(ctx, "nexusim:push:route:session:malformed-session", "{", time.Minute).Err(); err != nil {
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
	if err := client.SAdd(ctx, "nexusim:push:route:user:tenant-1:user-1", "wrong-user-session").Err(); err != nil {
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
		if redisSetHasMember(t, server, "nexusim:push:route:user:tenant-1:user-1", sessionID) {
			t.Fatalf("expected stale session %s to be removed from user set", sessionID)
		}
	}
}

func TestRegistryCleanupStaleRoutesScansUserSets(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	local := memory.NewRegistry()
	registry := NewRegistry(local, client, Config{GatewayID: "gateway-a", RouteTTL: time.Minute})
	ctx := context.Background()
	userKey := "nexusim:push:route:user:tenant-1:user-1"

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
	if err := client.Set(ctx, "nexusim:push:route:session:malformed-session", "{", time.Minute).Err(); err != nil {
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
	go func() {
		done <- NewSubscriber(local, client, Config{GatewayID: "gateway-a"}).Run(ctx)
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
	go func() {
		done <- NewSubscriber(local, client, Config{GatewayID: "gateway-a"}).Run(ctx)
	}()
	time.Sleep(50 * time.Millisecond)

	channel := GatewayChannel(defaultKeyPrefix, "gateway-a")
	if err := client.Publish(ctx, channel, "{").Err(); err != nil {
		t.Fatalf("publish malformed payload: %v", err)
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
	case err := <-done:
		t.Fatalf("subscriber exited after malformed payload: %v", err)
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for local enqueue")
	}
	cancel()
	<-done
}

func testNotification() types.DeliveryNotification {
	return types.DeliveryNotification{
		EventID:         "delivery-event-1",
		TenantID:        "tenant-1",
		UserID:          "user-1",
		ConversationID:  "conversation-1",
		ConversationSeq: 7,
		SourceEventID:   "timeline-event-1",
		MessageID:       "message-1",
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
