package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
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
	} else if retryDelay := retryDelayFromError(t, err); retryDelay != time.Second {
		t.Fatalf("unexpected retry delay: %s", retryDelay)
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
	allowed, retryDelay, err := limiter.allow(ctx, "/method")
	if err != nil || !allowed || retryDelay != 0 {
		t.Fatalf("first request should pass, allowed=%v retryDelay=%s err=%v", allowed, retryDelay, err)
	}
	allowed, retryDelay, err = limiter.allow(ctx, "/method")
	if err != nil {
		t.Fatalf("second request returned error: %v", err)
	}
	if allowed {
		t.Fatalf("second immediate request should be limited")
	}
	if retryDelay != 500*time.Millisecond {
		t.Fatalf("unexpected retry delay: %s", retryDelay)
	}
	now = now.Add(500 * time.Millisecond)
	allowed, retryDelay, err = limiter.allow(ctx, "/method")
	if err != nil || !allowed || retryDelay != 0 {
		t.Fatalf("request after refill should pass, allowed=%v retryDelay=%s err=%v", allowed, retryDelay, err)
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
	allowed1, _, err1 := limiter.allow(ctx1, "/method")
	allowed2, _, err2 := limiter.allow(ctx2, "/method")
	allowed3, _, err3 := limiter.allow(ctx1, "/other")
	if err1 != nil || err2 != nil || err3 != nil || !allowed1 || !allowed2 || !allowed3 {
		t.Fatalf("different token or method should use separate buckets")
	}
	if limiter.Snapshot().TrackedKeys != 3 {
		t.Fatalf("expected 3 tracked keys, got %+v", limiter.Snapshot())
	}
}

func TestLimiterTenantScopeSharesQuotaAcrossTokensInTenant(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tenantByToken := map[string]string{
		"token-1": "tenant-a",
		"token-2": "tenant-a",
		"token-3": "tenant-b",
	}
	limiter, err := New(Config{
		Enabled:           true,
		KeyScope:          "tenant",
		RequestsPerSecond: 1,
		Burst:             1,
		Now:               func() time.Time { return now },
		IdentityFunc: func(ctx context.Context) (Identity, error) {
			md, _ := metadata.FromIncomingContext(ctx)
			values := md.Get("authorization")
			if len(values) == 0 {
				return Identity{}, context.Canceled
			}
			token := bearerToken(values[0])
			return Identity{TenantID: tenantByToken[token]}, nil
		},
	})
	if err != nil {
		t.Fatalf("new limiter: %v", err)
	}
	ctx1 := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer token-1"))
	ctx2 := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer token-2"))
	ctx3 := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer token-3"))

	allowed, _, err := limiter.allow(ctx1, "/method")
	if err != nil || !allowed {
		t.Fatalf("first tenant-a request should pass, allowed=%v err=%v", allowed, err)
	}
	allowed, _, err = limiter.allow(ctx2, "/method")
	if err != nil {
		t.Fatalf("second tenant-a request returned error: %v", err)
	}
	if allowed {
		t.Fatalf("second tenant-a request with different token should share tenant quota")
	}
	allowed, _, err = limiter.allow(ctx3, "/method")
	if err != nil || !allowed {
		t.Fatalf("tenant-b request should use separate quota, allowed=%v err=%v", allowed, err)
	}
	if snapshot := limiter.Snapshot(); snapshot.KeyScope != "tenant" || snapshot.TrackedKeys != 2 || snapshot.TotalLimited != 1 {
		t.Fatalf("unexpected tenant scope snapshot: %+v", snapshot)
	}
}

func TestLimiterTenantPlanOverridesDefaultQuota(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	limiter, err := New(Config{
		Enabled:           true,
		KeyScope:          "tenant",
		RequestsPerSecond: 1,
		Burst:             1,
		TenantPlans: map[string]Plan{
			"tenant-vip": {RequestsPerSecond: 10, Burst: 2},
		},
		Now: func() time.Time { return now },
		IdentityFunc: func(ctx context.Context) (Identity, error) {
			md, _ := metadata.FromIncomingContext(ctx)
			return Identity{TenantID: md.Get("tenant")[0]}, nil
		},
	})
	if err != nil {
		t.Fatalf("new limiter: %v", err)
	}
	vip := metadata.NewIncomingContext(context.Background(), metadata.Pairs("tenant", "tenant-vip"))
	regular := metadata.NewIncomingContext(context.Background(), metadata.Pairs("tenant", "tenant-regular"))

	for i := 0; i < 2; i++ {
		allowed, _, err := limiter.allow(vip, "/method")
		if err != nil || !allowed {
			t.Fatalf("vip request %d should pass, allowed=%v err=%v", i, allowed, err)
		}
	}
	if allowed, _, err := limiter.allow(vip, "/method"); err != nil || allowed {
		t.Fatalf("third vip request should be limited, allowed=%v err=%v", allowed, err)
	}
	if allowed, _, err := limiter.allow(regular, "/method"); err != nil || !allowed {
		t.Fatalf("regular first request should pass, allowed=%v err=%v", allowed, err)
	}
	if allowed, _, err := limiter.allow(regular, "/method"); err != nil || allowed {
		t.Fatalf("regular second request should use default burst and be limited, allowed=%v err=%v", allowed, err)
	}
	if snapshot := limiter.Snapshot(); snapshot.TenantPlans != 1 || snapshot.TotalAccepted != 3 || snapshot.TotalLimited != 2 {
		t.Fatalf("unexpected tenant plan snapshot: %+v", snapshot)
	}
}

func TestLimiterUpdateTenantPlansChangesQuotaAtRuntime(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	limiter, err := New(Config{
		Enabled:           true,
		KeyScope:          "tenant",
		RequestsPerSecond: 1,
		Burst:             1,
		TenantPlans: map[string]Plan{
			"tenant-vip": {RequestsPerSecond: 1, Burst: 1},
		},
		Now: func() time.Time { return now },
		IdentityFunc: func(ctx context.Context) (Identity, error) {
			md, _ := metadata.FromIncomingContext(ctx)
			return Identity{TenantID: md.Get("tenant")[0]}, nil
		},
	})
	if err != nil {
		t.Fatalf("new limiter: %v", err)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("tenant", "tenant-vip"))
	if allowed, _, err := limiter.allow(ctx, "/method"); err != nil || !allowed {
		t.Fatalf("first request should pass before update, allowed=%v err=%v", allowed, err)
	}
	if allowed, _, err := limiter.allow(ctx, "/method"); err != nil || allowed {
		t.Fatalf("second request should be limited before update, allowed=%v err=%v", allowed, err)
	}

	now = now.Add(2 * time.Second)
	if err := limiter.UpdateTenantPlans(map[string]Plan{
		"tenant-vip": {RequestsPerSecond: 10, Burst: 2},
	}); err != nil {
		t.Fatalf("update tenant plans: %v", err)
	}
	for i := 0; i < 2; i++ {
		allowed, _, err := limiter.allow(ctx, "/method")
		if err != nil || !allowed {
			t.Fatalf("request %d should pass after update, allowed=%v err=%v", i, allowed, err)
		}
	}
	if allowed, _, err := limiter.allow(ctx, "/method"); err != nil || allowed {
		t.Fatalf("third request should use updated burst and be limited, allowed=%v err=%v", allowed, err)
	}
	snapshot := limiter.Snapshot()
	if snapshot.TenantPlans != 1 || snapshot.TenantReloads != 1 || snapshot.TenantReloadAt == 0 || snapshot.TenantErrors != 0 {
		t.Fatalf("unexpected updated tenant plan snapshot: %+v", snapshot)
	}
}

func TestLimiterUpdateTenantPlanSnapshotTracksMetadata(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	limiter, err := New(Config{
		Enabled:                     true,
		KeyScope:                    "tenant",
		RequestsPerSecond:           1,
		Burst:                       1,
		TenantPlanRequireChecksum:   true,
		TenantPlanURLBearerTokenSet: true,
		TenantPlanURLRequireHTTPS:   true,
		TenantPlanURLTLSConfigured:  true,
		TenantPlanURLClientCertSet:  true,
		Now:                         func() time.Time { return now },
		IdentityFunc: func(ctx context.Context) (Identity, error) {
			md, _ := metadata.FromIncomingContext(ctx)
			return Identity{TenantID: md.Get("tenant")[0]}, nil
		},
	})
	if err != nil {
		t.Fatalf("new limiter: %v", err)
	}
	if err := limiter.UpdateTenantPlanSnapshot(map[string]Plan{
		"tenant-vip": {RequestsPerSecond: 10, Burst: 2},
	}, "quota-v1.test", 1_800_000_000_123, true); err != nil {
		t.Fatalf("update tenant plan snapshot: %v", err)
	}
	snapshot := limiter.Snapshot()
	if snapshot.TenantPlans != 1 ||
		snapshot.TenantPlanVersion != "quota-v1.test" ||
		snapshot.TenantPlanGeneratedAt != 1_800_000_000_123 ||
		!snapshot.TenantPlanChecksumPresent ||
		!snapshot.TenantPlanRequireChecksum ||
		!snapshot.TenantPlanURLBearerSet ||
		!snapshot.TenantPlanURLRequireHTTPS ||
		!snapshot.TenantPlanURLTLSConfigured ||
		!snapshot.TenantPlanURLClientCertSet {
		t.Fatalf("unexpected tenant plan metadata snapshot: %+v", snapshot)
	}
}

func TestLimiterSnapshotTracksStaleTenantPlan(t *testing.T) {
	now := time.UnixMilli(1_800_000_100_000)
	limiter, err := New(Config{
		Enabled:           true,
		KeyScope:          "tenant",
		RequestsPerSecond: 1,
		Burst:             1,
		TenantPlanMaxAge:  time.Minute,
		Now:               func() time.Time { return now },
		IdentityFunc: func(ctx context.Context) (Identity, error) {
			md, _ := metadata.FromIncomingContext(ctx)
			return Identity{TenantID: md.Get("tenant")[0]}, nil
		},
	})
	if err != nil {
		t.Fatalf("new limiter: %v", err)
	}
	if err := limiter.UpdateTenantPlanSnapshot(map[string]Plan{
		"tenant-vip": {RequestsPerSecond: 10, Burst: 2},
	}, "quota-v1.test", 1_800_000_000_000, true); err != nil {
		t.Fatalf("update tenant plan snapshot: %v", err)
	}
	snapshot := limiter.Snapshot()
	if snapshot.TenantPlanMaxAgeMS != int64(time.Minute/time.Millisecond) ||
		snapshot.TenantPlanAgeMS != 100_000 ||
		!snapshot.TenantPlanStale {
		t.Fatalf("expected stale tenant plan snapshot: %+v", snapshot)
	}
}

func TestLimiterUpdateTenantPlansRejectsInvalidPlanWithoutReplacingOldPlan(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	limiter, err := New(Config{
		Enabled:           true,
		KeyScope:          "tenant",
		RequestsPerSecond: 1,
		Burst:             1,
		TenantPlans: map[string]Plan{
			"tenant-vip": {RequestsPerSecond: 10, Burst: 2},
		},
		Now: func() time.Time { return now },
		IdentityFunc: func(ctx context.Context) (Identity, error) {
			md, _ := metadata.FromIncomingContext(ctx)
			return Identity{TenantID: md.Get("tenant")[0]}, nil
		},
	})
	if err != nil {
		t.Fatalf("new limiter: %v", err)
	}
	if err := limiter.UpdateTenantPlans(map[string]Plan{"tenant-vip": {RequestsPerSecond: 0}}); err == nil {
		t.Fatalf("expected invalid tenant plan update to fail")
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("tenant", "tenant-vip"))
	for i := 0; i < 2; i++ {
		allowed, _, err := limiter.allow(ctx, "/method")
		if err != nil || !allowed {
			t.Fatalf("request %d should still use old valid plan, allowed=%v err=%v", i, allowed, err)
		}
	}
	if allowed, _, err := limiter.allow(ctx, "/method"); err != nil || allowed {
		t.Fatalf("third request should still be limited by old plan, allowed=%v err=%v", allowed, err)
	}
	if snapshot := limiter.Snapshot(); snapshot.TenantPlans != 1 || snapshot.TenantReloads != 0 || snapshot.TenantErrors != 1 {
		t.Fatalf("unexpected invalid update snapshot: %+v", snapshot)
	}
}

func TestLimiterTenantScopeFallbackDoesNotChargeTenantOnIdentityError(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	limiter, err := New(Config{
		Enabled:           true,
		KeyScope:          "tenant",
		RequestsPerSecond: 1,
		Burst:             1,
		Now:               func() time.Time { return now },
		IdentityFunc: func(context.Context) (Identity, error) {
			return Identity{}, context.Canceled
		},
	})
	if err != nil {
		t.Fatalf("new limiter: %v", err)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer token-1"))
	allowed, _, err := limiter.allow(ctx, "/method")
	if err != nil || !allowed {
		t.Fatalf("fallback request should pass, allowed=%v err=%v", allowed, err)
	}
	snapshot := limiter.Snapshot()
	if snapshot.IdentityErrors != 1 || snapshot.TrackedKeys != 1 || snapshot.TotalAccepted != 1 {
		t.Fatalf("unexpected fallback snapshot: %+v", snapshot)
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

func TestLimiterTenantScopeRequiresIdentityResolver(t *testing.T) {
	if _, err := New(Config{Enabled: true, KeyScope: "tenant", RequestsPerSecond: 1}); err == nil {
		t.Fatalf("expected tenant scope without identity resolver to fail")
	}
}

func TestLimiterRejectsUnsupportedScope(t *testing.T) {
	if _, err := New(Config{Enabled: true, KeyScope: "global", RequestsPerSecond: 1}); err == nil {
		t.Fatalf("expected unsupported scope to fail")
	}
}

func TestLimiterRejectsInvalidTenantPlans(t *testing.T) {
	if _, err := New(Config{
		Enabled:           true,
		KeyScope:          "tenant",
		RequestsPerSecond: 1,
		Burst:             1,
		TenantPlans:       map[string]Plan{"tenant-a": {RequestsPerSecond: 0}},
		IdentityFunc: func(context.Context) (Identity, error) {
			return Identity{TenantID: "tenant-a"}, nil
		},
	}); err == nil {
		t.Fatalf("expected invalid tenant plan to fail")
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
		allowed, retryDelay, err := limiter.allow(ctx, "/method")
		if err != nil || !allowed || retryDelay != 0 {
			t.Fatalf("request %d should pass, allowed=%v retryDelay=%s err=%v", i, allowed, retryDelay, err)
		}
	}
	allowed, retryDelay, err := limiterA.allow(ctx, "/method")
	if err != nil {
		t.Fatalf("third request returned error: %v", err)
	}
	if allowed {
		t.Fatalf("third request should be limited across instances")
	}
	if retryDelay != time.Second {
		t.Fatalf("unexpected retry delay: %s", retryDelay)
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
	allowed, retryDelay, err := limiter.allow(ctx, "/method")
	if err != nil || !allowed || retryDelay != 0 {
		t.Fatalf("first request should pass")
	}
	allowed, retryDelay, err = limiter.allow(ctx, "/method")
	if err != nil || allowed {
		t.Fatalf("second same-window request should be limited, allowed=%v err=%v", allowed, err)
	}
	if retryDelay != time.Second {
		t.Fatalf("unexpected retry delay: %s", retryDelay)
	}
	now = now.Add(time.Second)
	allowed, retryDelay, err = limiter.allow(ctx, "/method")
	if err != nil || !allowed || retryDelay != 0 {
		t.Fatalf("new window request should pass, allowed=%v retryDelay=%s err=%v", allowed, retryDelay, err)
	}
}

func TestRedisLimiterTenantPlanOverridesDefaultQuota(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	now := time.Unix(1_800_000_000, 0)
	limiter, err := New(Config{
		Enabled:           true,
		Backend:           "redis",
		KeyScope:          "tenant",
		RequestsPerSecond: 1,
		Burst:             1,
		TenantPlans: map[string]Plan{
			"tenant-vip": {RequestsPerSecond: 10, Burst: 2},
		},
		RedisClient: client,
		RedisWindow: time.Second,
		Now:         func() time.Time { return now },
		IdentityFunc: func(ctx context.Context) (Identity, error) {
			md, _ := metadata.FromIncomingContext(ctx)
			return Identity{TenantID: md.Get("tenant")[0]}, nil
		},
	})
	if err != nil {
		t.Fatalf("new limiter: %v", err)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("tenant", "tenant-vip"))
	for i := 0; i < 2; i++ {
		allowed, _, err := limiter.allow(ctx, "/method")
		if err != nil || !allowed {
			t.Fatalf("vip redis request %d should pass, allowed=%v err=%v", i, allowed, err)
		}
	}
	allowed, retryDelay, err := limiter.allow(ctx, "/method")
	if err != nil || allowed {
		t.Fatalf("third vip redis request should be limited, allowed=%v err=%v", allowed, err)
	}
	if retryDelay != time.Second {
		t.Fatalf("unexpected retry delay: %s", retryDelay)
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
	allowed, retryDelay, err := limiter.allow(context.Background(), "/method")
	if err != nil || !allowed || retryDelay != 0 {
		t.Fatalf("fail-open limiter should allow on redis error, allowed=%v retryDelay=%s err=%v", allowed, retryDelay, err)
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
	allowed, _, err := limiter.allow(context.Background(), "/method")
	if err == nil || allowed {
		t.Fatalf("fail-closed limiter should reject redis error, allowed=%v err=%v", allowed, err)
	}
}

func retryDelayFromError(t *testing.T, err error) time.Duration {
	t.Helper()
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %v", err)
	}
	for _, detail := range st.Details() {
		if retryInfo, ok := detail.(*errdetails.RetryInfo); ok {
			return retryInfo.GetRetryDelay().AsDuration()
		}
	}
	t.Fatalf("expected RetryInfo detail in %v", st.Details())
	return 0
}
