package ratelimit

import (
	"context"
	"testing"
	"time"

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
	if !limiter.allow(ctx, "/method") {
		t.Fatalf("first request should pass")
	}
	if limiter.allow(ctx, "/method") {
		t.Fatalf("second immediate request should be limited")
	}
	now = now.Add(500 * time.Millisecond)
	if !limiter.allow(ctx, "/method") {
		t.Fatalf("request after refill should pass")
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
	if !limiter.allow(ctx1, "/method") || !limiter.allow(ctx2, "/method") || !limiter.allow(ctx1, "/other") {
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
