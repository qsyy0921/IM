package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestLimiterAllowsBurstThenRejects(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	limiter, err := New(Config{
		Enabled:           true,
		RequestsPerSecond: 1,
		Burst:             2,
		Now:               func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new limiter: %v", err)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer token-1"))
	interceptor := limiter.UnaryServerInterceptor()
	info := &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.message.v1.MessageService/SendMessage"}
	handler := func(context.Context, any) (any, error) { return "ok", nil }

	for i := 0; i < 2; i++ {
		if _, err := interceptor(ctx, nil, info, handler); err != nil {
			t.Fatalf("request %d should pass: %v", i, err)
		}
	}
	if _, err := interceptor(ctx, nil, info, handler); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("expected third request to be rate limited, got %v", err)
	}
	snapshot := limiter.Snapshot()
	if snapshot.TotalAccepted != 2 || snapshot.TotalLimited != 1 || snapshot.TrackedKeys != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestLimiterRefillsOverTime(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	limiter, err := New(Config{
		Enabled:           true,
		RequestsPerSecond: 2,
		Burst:             1,
		Now:               func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new limiter: %v", err)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-nexusim-gateway-token", "token-1"))
	allowed, err := limiter.allow(ctx, "/method")
	if err != nil || !allowed {
		t.Fatalf("first request should pass, allowed=%v err=%v", allowed, err)
	}
	allowed, err = limiter.allow(ctx, "/method")
	if err != nil {
		t.Fatalf("second request returned error: %v", err)
	}
	if allowed {
		t.Fatalf("second immediate request should be limited")
	}
	now = now.Add(500 * time.Millisecond)
	allowed, err = limiter.allow(ctx, "/method")
	if err != nil || !allowed {
		t.Fatalf("request after refill should pass, allowed=%v err=%v", allowed, err)
	}
}

func TestLimiterKeysByMethodAndToken(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	limiter, err := New(Config{
		Enabled:           true,
		RequestsPerSecond: 1,
		Burst:             1,
		Now:               func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new limiter: %v", err)
	}
	ctx1 := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer token-1"))
	ctx2 := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer token-2"))
	allowed1, err1 := limiter.allow(ctx1, "/method")
	allowed2, err2 := limiter.allow(ctx2, "/method")
	allowed3, err3 := limiter.allow(ctx1, "/other")
	if err1 != nil || err2 != nil || err3 != nil || !allowed1 || !allowed2 || !allowed3 {
		t.Fatalf("different token or method should use separate buckets")
	}
	if limiter.Snapshot().TrackedKeys != 3 {
		t.Fatalf("expected 3 tracked keys, got %+v", limiter.Snapshot())
	}
}

func TestLimiterDisabledIsNoop(t *testing.T) {
	limiter, err := New(Config{})
	if err != nil {
		t.Fatalf("new limiter: %v", err)
	}
	interceptor := limiter.UnaryServerInterceptor()
	called := false
	_, err = interceptor(context.Background(), nil, &grpcgo.UnaryServerInfo{FullMethod: "/method"}, func(context.Context, any) (any, error) {
		called = true
		return nil, nil
	})
	if err != nil || !called {
		t.Fatalf("expected disabled limiter to call handler, called=%v err=%v", called, err)
	}
}

func TestLimiterRequiresPositiveRateWhenEnabled(t *testing.T) {
	if _, err := New(Config{Enabled: true}); err == nil {
		t.Fatalf("expected enabled limiter without rate to fail")
	}
}

func TestRedisLimiterSharesLimitAcrossInstances(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	now := time.Unix(1_800_000_000, 0)
	config := Config{
		Enabled:           true,
		Backend:           "redis",
		RequestsPerSecond: 1,
		Burst:             2,
		RedisClient:       client,
		RedisKeyPrefix:    "nexusim:test:api-gateway",
		RedisWindow:       time.Second,
		Now:               func() time.Time { return now },
	}
	limiterA, err := New(config)
	if err != nil {
		t.Fatalf("new limiter a: %v", err)
	}
	limiterB, err := New(config)
	if err != nil {
		t.Fatalf("new limiter b: %v", err)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer token-1"))
	for i, limiter := range []*Limiter{limiterA, limiterB} {
		allowed, err := limiter.allow(ctx, "/method")
		if err != nil || !allowed {
			t.Fatalf("request %d should pass, allowed=%v err=%v", i, allowed, err)
		}
	}
	allowed, err := limiterA.allow(ctx, "/method")
	if err != nil {
		t.Fatalf("third request returned error: %v", err)
	}
	if allowed {
		t.Fatalf("third request should be limited across instances")
	}
	if limiterA.Snapshot().TotalLimited != 1 {
		t.Fatalf("expected limiter a to record limited request, got %+v", limiterA.Snapshot())
	}
}

func TestRedisLimiterAllowsNewWindow(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	now := time.Unix(1_800_000_000, 0)
	limiter, err := New(Config{
		Enabled:           true,
		Backend:           "redis",
		RequestsPerSecond: 1,
		Burst:             1,
		RedisClient:       client,
		RedisWindow:       time.Second,
		Now:               func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new limiter: %v", err)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer token-1"))
	allowed, err := limiter.allow(ctx, "/method")
	if err != nil || !allowed {
		t.Fatalf("first request should pass")
	}
	allowed, err = limiter.allow(ctx, "/method")
	if err != nil || allowed {
		t.Fatalf("second same-window request should be limited, allowed=%v err=%v", allowed, err)
	}
	now = now.Add(time.Second)
	allowed, err = limiter.allow(ctx, "/method")
	if err != nil || !allowed {
		t.Fatalf("new window request should pass, allowed=%v err=%v", allowed, err)
	}
}

func TestRedisLimiterFailOpenOnRedisError(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	limiter, err := New(Config{
		Enabled:           true,
		Backend:           "redis",
		RequestsPerSecond: 1,
		Burst:             1,
		RedisClient:       client,
		RedisFailOpen:     true,
	})
	if err != nil {
		t.Fatalf("new limiter: %v", err)
	}
	server.Close()
	allowed, err := limiter.allow(context.Background(), "/method")
	if err != nil || !allowed {
		t.Fatalf("fail-open limiter should allow on redis error, allowed=%v err=%v", allowed, err)
	}
	snapshot := limiter.Snapshot()
	if snapshot.RedisErrors == 0 || snapshot.TotalAccepted != 1 {
		t.Fatalf("expected redis error and accepted count, got %+v", snapshot)
	}
}

func TestRedisLimiterFailClosedOnRedisError(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	limiter, err := New(Config{
		Enabled:           true,
		Backend:           "redis",
		RequestsPerSecond: 1,
		Burst:             1,
		RedisClient:       client,
	})
	if err != nil {
		t.Fatalf("new limiter: %v", err)
	}
	server.Close()
	allowed, err := limiter.allow(context.Background(), "/method")
	if err == nil || allowed {
		t.Fatalf("fail-closed limiter should reject redis error, allowed=%v err=%v", allowed, err)
	}
}
