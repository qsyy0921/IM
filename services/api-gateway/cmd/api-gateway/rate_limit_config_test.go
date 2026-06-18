package main

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	ratelimitinfra "github.com/qsyy0921/IM/services/api-gateway/internal/infrastructure/ratelimit"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestNewRateLimiterFromEnvDisabledByDefault(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	limiter, closeFn, err := newRateLimiterFromEnv(context.Background(), nil)
	if err != nil {
		t.Fatalf("new rate limiter: %v", err)
	}
	defer closeFn()
	if snapshot := limiter.Snapshot(); snapshot.Enabled || snapshot.TotalLimited != 0 {
		t.Fatalf("expected disabled limiter, got %+v", snapshot)
	}
}

func TestNewRateLimiterFromEnvEnabled(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_ENABLED", "true")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_RPS", "12.5")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_BURST", "20")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_MAX_KEYS", "50")

	limiter, closeFn, err := newRateLimiterFromEnv(context.Background(), nil)
	if err != nil {
		t.Fatalf("new rate limiter: %v", err)
	}
	defer closeFn()
	snapshot := limiter.Snapshot()
	if !snapshot.Enabled || snapshot.RatePerSecond != 12.5 || snapshot.Burst != 20 || snapshot.MaxKeys != 50 {
		t.Fatalf("unexpected limiter snapshot: %+v", snapshot)
	}
}

func TestNewRateLimiterFromEnvEnabledRequiresRate(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_ENABLED", "true")
	if _, _, err := newRateLimiterFromEnv(context.Background(), nil); err == nil {
		t.Fatalf("expected missing rate to fail")
	}
}

func TestNewRateLimiterFromEnvRedisBackend(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	redisServer := miniredis.RunT(t)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_ENABLED", "true")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_BACKEND", "redis")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_RPS", "5")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_BURST", "8")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_ADDR", redisServer.Addr())
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_KEY_PREFIX", "nexusim:test:api-gateway")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_WINDOW", "2s")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_FAIL_OPEN", "false")

	limiter, closeFn, err := newRateLimiterFromEnv(context.Background(), nil)
	if err != nil {
		t.Fatalf("new redis rate limiter: %v", err)
	}
	defer closeFn()
	snapshot := limiter.Snapshot()
	if !snapshot.Enabled || snapshot.Backend != "redis" || snapshot.RedisMode != "single" || snapshot.Burst != 8 || snapshot.RedisWindowMS != 2000 || snapshot.RedisFailOpen {
		t.Fatalf("unexpected redis limiter snapshot: %+v", snapshot)
	}
}

func TestNewRateLimiterFromEnvRedisFailOpenAllowsStartupWhenRedisUnavailable(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_ENABLED", "true")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_BACKEND", "redis")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_RPS", "5")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_BURST", "8")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_ADDR", "127.0.0.1:1")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_FAIL_OPEN", "true")

	limiter, closeFn, err := newRateLimiterFromEnv(context.Background(), nil)
	if err != nil {
		t.Fatalf("fail-open redis limiter should start when redis is unavailable: %v", err)
	}
	defer closeFn()
	interceptor := limiter.UnaryServerInterceptor()
	called := false
	_, err = interceptor(context.Background(), nil, &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.gateway.v1.GatewayService/SendMessage"}, func(context.Context, any) (any, error) {
		called = true
		return nil, nil
	})
	if err != nil || !called {
		t.Fatalf("fail-open redis limiter should allow request on redis error, called=%v err=%v", called, err)
	}
	snapshot := limiter.Snapshot()
	if !snapshot.RedisFailOpen || snapshot.RedisErrors == 0 || snapshot.TotalAccepted != 1 {
		t.Fatalf("expected fail-open redis error accounting, got %+v", snapshot)
	}
}

func TestNewRateLimiterFromEnvRedisFailClosedRejectsStartupWhenRedisUnavailable(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_ENABLED", "true")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_BACKEND", "redis")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_RPS", "5")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_ADDR", "127.0.0.1:1")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_FAIL_OPEN", "false")

	if _, _, err := newRateLimiterFromEnv(context.Background(), nil); err == nil {
		t.Fatalf("fail-closed redis limiter should fail startup when redis is unavailable")
	}
}

func TestNewRateLimiterFromEnvTenantScopeUsesAuthenticatedTenant(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	clearAPIGatewayAuthConfig(t)
	t.Setenv("NEXUSIM_API_GATEWAY_AUTH_MODE", "hmac")
	t.Setenv("NEXUSIM_API_GATEWAY_AUTH_HMAC_SECRET", "gateway-secret")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_ENABLED", "true")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_SCOPE", "tenant")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_RPS", "1")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_BURST", "1")

	authenticator, err := newAuthenticatorFromEnv()
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	t.Cleanup(authenticator.Close)
	limiter, closeFn, err := newRateLimiterFromEnv(context.Background(), authenticator)
	if err != nil {
		t.Fatalf("new tenant rate limiter: %v", err)
	}
	defer closeFn()

	tokenA1 := signAPIGatewayTestToken(t, "tenant-a", "user-1")
	tokenA2 := signAPIGatewayTestToken(t, "tenant-a", "user-2")
	tokenB := signAPIGatewayTestToken(t, "tenant-b", "user-3")
	ctxA1 := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+tokenA1))
	ctxA2 := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+tokenA2))
	ctxB := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+tokenB))
	interceptor := limiter.UnaryServerInterceptor()
	info := &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.gateway.v1.GatewayService/SendMessage"}
	handler := func(context.Context, any) (any, error) { return "ok", nil }

	if _, err := interceptor(ctxA1, nil, info, handler); err != nil {
		t.Fatalf("first tenant-a request should pass: %v", err)
	}
	if _, err := interceptor(ctxA2, nil, info, handler); err == nil {
		t.Fatalf("second tenant-a request should be limited")
	}
	if _, err := interceptor(ctxB, nil, info, handler); err != nil {
		t.Fatalf("tenant-b request should pass: %v", err)
	}
	snapshot := limiter.Snapshot()
	if snapshot.KeyScope != "tenant" || snapshot.TotalAccepted != 2 || snapshot.TotalLimited != 1 {
		t.Fatalf("unexpected tenant limiter snapshot: %+v", snapshot)
	}
}

func TestRateLimitAuthRequestFromMetadataKeepsTokenOutOfURL(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"x-nexusim-gateway-token", "gateway-token-secret",
		"x-nexusim-tenant-id", "tenant-a",
		"x-nexusim-user-id", "user-a",
		"x-nexusim-device-id", "device-a",
		"x-nexusim-trace-id", "trace user=user1@example.com",
	))

	request := rateLimitAuthRequestFromMetadata(ctx)
	if got := request.Header.Get("Authorization"); got != "Bearer gateway-token-secret" {
		t.Fatalf("expected gateway token to be forwarded as Authorization header, got %q", got)
	}
	rawURL := request.URL.String()
	for _, leaked := range []string{"gateway-token-secret", "trace+user", "user1%40example.com", "trace_id"} {
		if strings.Contains(rawURL, leaked) {
			t.Fatalf("rate-limit auth request URL leaked %q: %s", leaked, rawURL)
		}
	}
	if request.URL.Query().Get("tenant_id") != "tenant-a" ||
		request.URL.Query().Get("user_id") != "user-a" ||
		request.URL.Query().Get("device_id") != "device-a" {
		t.Fatalf("expected low-sensitive mock auth query fields, got %s", rawURL)
	}
}

func TestRateLimitAuthRequestFromMetadataPrefersAuthorizationHeader(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"authorization", "Bearer authorization-token",
		"x-nexusim-gateway-token", "gateway-token-secret",
	))

	request := rateLimitAuthRequestFromMetadata(ctx)
	if got := request.Header.Get("Authorization"); got != "Bearer authorization-token" {
		t.Fatalf("expected existing Authorization metadata to be preserved, got %q", got)
	}
	if rawURL := request.URL.String(); strings.Contains(rawURL, "gateway-token-secret") {
		t.Fatalf("rate-limit auth request URL leaked gateway token: %s", rawURL)
	}
}

func TestNewRateLimiterFromEnvLoadsTenantPlansJSON(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	clearAPIGatewayAuthConfig(t)
	t.Setenv("NEXUSIM_API_GATEWAY_AUTH_MODE", "hmac")
	t.Setenv("NEXUSIM_API_GATEWAY_AUTH_HMAC_SECRET", "gateway-secret")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_ENABLED", "true")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_SCOPE", "tenant")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_RPS", "1")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_BURST", "1")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_JSON", `{"tenant-vip":{"requests_per_second":10,"burst":2}}`)

	authenticator, err := newAuthenticatorFromEnv()
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	t.Cleanup(authenticator.Close)
	limiter, closeFn, err := newRateLimiterFromEnv(context.Background(), authenticator)
	if err != nil {
		t.Fatalf("new tenant plan limiter: %v", err)
	}
	defer closeFn()
	snapshot := limiter.Snapshot()
	if snapshot.TenantPlans != 1 || snapshot.TenantPlanSource != "inline" || snapshot.KeyScope != "tenant" {
		t.Fatalf("unexpected tenant plan snapshot: %+v", snapshot)
	}
}

func TestNewRateLimiterFromEnvExposesTenantPlanURLGuards(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	plans := map[string]ratelimitinfra.Plan{"tenant-url": {RequestsPerSecond: 9, Burst: 10}}
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer quota-config-token" {
			t.Fatalf("expected bearer token auth header, got %q", request.Header.Get("Authorization"))
		}
		_, _ = writer.Write([]byte(versionedTenantPlanSnapshotJSON(t, plans, "quota-v1.url-guards", time.Now().UnixMilli())))
	}))
	defer server.Close()
	caPath := filepath.Join(t.TempDir(), "quota-url-ca.pem")
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(caPath, caPEM, 0o600); err != nil {
		t.Fatalf("write quota URL CA file: %v", err)
	}

	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_ENABLED", "true")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_RPS", "1")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE", "url")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL", server.URL)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_BEARER_TOKEN", "quota-config-token")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_REQUIRE_HTTPS", "true")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_CA_FILE", caPath)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_REQUIRE_CHECKSUM", "true")

	limiter, closeFn, err := newRateLimiterFromEnv(context.Background(), nil)
	if err != nil {
		t.Fatalf("new URL tenant plan limiter: %v", err)
	}
	defer closeFn()
	snapshot := limiter.Snapshot()
	if snapshot.TenantPlanSource != "url" ||
		snapshot.TenantPlanVersion != "quota-v1.url-guards" ||
		!snapshot.TenantPlanChecksumPresent ||
		!snapshot.TenantPlanRequireChecksum ||
		!snapshot.TenantPlanURLBearerSet ||
		!snapshot.TenantPlanURLRequireHTTPS ||
		!snapshot.TenantPlanURLTLSConfigured ||
		snapshot.TenantPlanURLClientCertSet {
		t.Fatalf("unexpected URL guard snapshot: %+v", snapshot)
	}
}

func TestParseTenantRateLimitPlansAllowsRPSAlias(t *testing.T) {
	plans, err := parseTenantRateLimitPlans(`{"tenant-a":{"rps":3,"burst":4}}`)
	if err != nil {
		t.Fatalf("parse tenant plans: %v", err)
	}
	if plans["tenant-a"].RequestsPerSecond != 3 || plans["tenant-a"].Burst != 4 {
		t.Fatalf("unexpected tenant plan: %+v", plans["tenant-a"])
	}
}

func TestTenantRateLimitPlansFromEnvLoadsFile(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	path := filepath.Join(t.TempDir(), "tenant-plans.json")
	if err := os.WriteFile(path, []byte(`{"tenant-a":{"requests_per_second":5,"burst":6}}`), 0o600); err != nil {
		t.Fatalf("write tenant plans file: %v", err)
	}
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_FILE", path)
	snapshot, err := tenantRateLimitPlansFromEnv(context.Background())
	if err != nil {
		t.Fatalf("load tenant plans file: %v", err)
	}
	if snapshot.Source != "file" {
		t.Fatalf("expected file tenant plan source, got %q", snapshot.Source)
	}
	if snapshot.Plans["tenant-a"].RequestsPerSecond != 5 || snapshot.Plans["tenant-a"].Burst != 6 {
		t.Fatalf("unexpected tenant plan from file: %+v", snapshot.Plans["tenant-a"])
	}
}

func TestTenantRateLimitPlansFromEnvLoadsVersionedFileSnapshot(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	path := filepath.Join(t.TempDir(), "tenant-plans.json")
	plans := map[string]ratelimitinfra.Plan{"tenant-a": {RequestsPerSecond: 5, Burst: 6}}
	generatedAt := time.Now().UnixMilli()
	if err := os.WriteFile(path, []byte(versionedTenantPlanSnapshotJSON(t, plans, "quota-v1.20260614", generatedAt)), 0o600); err != nil {
		t.Fatalf("write tenant plans file: %v", err)
	}
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_FILE", path)
	snapshot, err := tenantRateLimitPlansFromEnv(context.Background())
	if err != nil {
		t.Fatalf("load versioned tenant plans file: %v", err)
	}
	if snapshot.Source != "file" || snapshot.Version != "quota-v1.20260614" || snapshot.GeneratedAtUnixMS != generatedAt || !snapshot.ChecksumPresent {
		t.Fatalf("unexpected versioned tenant plan snapshot: %+v", snapshot)
	}
	if snapshot.Plans["tenant-a"].RequestsPerSecond != 5 || snapshot.Plans["tenant-a"].Burst != 6 {
		t.Fatalf("unexpected tenant plan from versioned file: %+v", snapshot.Plans["tenant-a"])
	}
}

func TestTenantRateLimitPlansFromEnvRejectsOversizedFileSnapshot(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	path := filepath.Join(t.TempDir(), "tenant-plans-too-large.json")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", tenantPlanSnapshotMaxBytes+1)), 0o600); err != nil {
		t.Fatalf("write oversized tenant plans file: %v", err)
	}
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_FILE", path)
	if _, err := tenantRateLimitPlansFromEnv(context.Background()); err == nil {
		t.Fatalf("expected oversized tenant plan file to fail")
	}
}

func TestTenantRateLimitPlansFromEnvRejectsVersionedFileChecksumMismatch(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	path := filepath.Join(t.TempDir(), "tenant-plans.json")
	payload := `{"version":"quota-v1.20260614","generated_at_unix_ms":1800000000000,"checksum":"sha256:0000000000000000000000000000000000000000000000000000000000000000","plans":{"tenant-a":{"requests_per_second":5,"burst":6}}}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write tenant plans file: %v", err)
	}
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_FILE", path)
	if _, err := tenantRateLimitPlansFromEnv(context.Background()); err == nil {
		t.Fatalf("expected checksum mismatch to fail")
	}
}

func TestTenantRateLimitPlansFromEnvRejectsMissingChecksumWhenRequired(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	path := filepath.Join(t.TempDir(), "tenant-plans.json")
	payload := `{"version":"quota-v1.no-checksum","generated_at_unix_ms":1800000000000,"plans":{"tenant-a":{"requests_per_second":5,"burst":6}}}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write tenant plans file: %v", err)
	}
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_FILE", path)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_REQUIRE_CHECKSUM", "true")
	if _, err := tenantRateLimitPlansFromEnv(context.Background()); err == nil {
		t.Fatalf("expected missing checksum to fail when checksum is required")
	}
}

func TestTenantRateLimitPlansFromEnvRejectsUnversionedSnapshotWhenRequired(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE", "inline")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_JSON", `{"tenant-a":{"requests_per_second":5,"burst":6}}`)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_REQUIRE_VERSIONED", "true")
	if _, err := tenantRateLimitPlansFromEnv(context.Background()); err == nil {
		t.Fatalf("expected unversioned tenant plan snapshot to fail when versioned snapshots are required")
	}
}

func TestNewRateLimiterFromEnvExposesTenantPlanVersionedPolicy(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	plans := map[string]ratelimitinfra.Plan{"tenant-a": {RequestsPerSecond: 5, Burst: 6}}
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_ENABLED", "true")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_RPS", "1")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE", "inline")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_JSON", versionedTenantPlanSnapshotJSON(t, plans, "quota-v1.require-versioned", time.Now().UnixMilli()))
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_REQUIRE_VERSIONED", "true")

	limiter, closeFn, err := newRateLimiterFromEnv(context.Background(), nil)
	if err != nil {
		t.Fatalf("new rate limiter with versioned tenant plan policy: %v", err)
	}
	defer closeFn()
	snapshot := limiter.Snapshot()
	if !snapshot.TenantPlanRequireVersioned || snapshot.TenantPlanVersion != "quota-v1.require-versioned" {
		t.Fatalf("expected versioned tenant plan policy in snapshot, got %+v", snapshot)
	}
}

func TestTenantRateLimitPlansFromEnvLoadsURLSnapshot(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	plans := map[string]ratelimitinfra.Plan{"tenant-url": {RequestsPerSecond: 9, Burst: 10}}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Accept") != "application/json" {
			t.Fatalf("expected application/json accept header, got %q", request.Header.Get("Accept"))
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(versionedTenantPlanSnapshotJSON(t, plans, "quota-v1.url", time.Now().UnixMilli())))
	}))
	defer server.Close()

	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE", "url")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL", server.URL)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_MAX_AGE", "1h")
	snapshot, err := tenantRateLimitPlansFromEnv(context.Background())
	if err != nil {
		t.Fatalf("load url tenant plans: %v", err)
	}
	if snapshot.Source != "url" || snapshot.Version != "quota-v1.url" || !snapshot.ChecksumPresent {
		t.Fatalf("unexpected url tenant plan snapshot: %+v", snapshot)
	}
	if snapshot.Plans["tenant-url"].RequestsPerSecond != 9 || snapshot.Plans["tenant-url"].Burst != 10 {
		t.Fatalf("unexpected tenant plan from url source: %+v", snapshot.Plans["tenant-url"])
	}
}

func TestTenantRateLimitPlansFromURLSanitizesTransportErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Fatalf("closed tenant plan URL server should not receive requests")
	}))
	endpoint := server.URL + "/quota?token=secret-token&user=user1@example.com"
	server.Close()

	_, err := tenantRateLimitPlansFromURL(context.Background(), endpoint, 0, false, false)
	if err == nil {
		t.Fatalf("expected URL transport error")
	}
	if got, want := err.Error(), "api-gateway tenant plan URL source request failed"; got != want {
		t.Fatalf("unexpected sanitized URL transport error: %q", got)
	}
	for _, leaked := range []string{"secret-token", "user1@example.com", endpoint, "127.0.0.1"} {
		if strings.Contains(err.Error(), leaked) {
			t.Fatalf("tenant plan URL transport error leaked %q: %q", leaked, err.Error())
		}
	}
}

func TestTenantRateLimitPlansFromEnvRejectsURLUserInfo(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Fatalf("tenant plan URL source should reject user info before request")
	}))
	defer server.Close()

	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test URL: %v", err)
	}
	endpoint.User = url.UserPassword("quota-user", "quota-secret")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE", "url")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL", endpoint.String())
	if _, err := tenantRateLimitPlansFromEnv(context.Background()); err == nil {
		t.Fatalf("expected tenant plan URL user info to fail")
	}
}

func TestTenantRateLimitPlansFromEnvRejectsURLRedirect(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	plans := map[string]ratelimitinfra.Plan{"tenant-url": {RequestsPerSecond: 9, Burst: 10}}
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Fatalf("tenant plan URL source should not follow redirects")
		_, _ = writer.Write([]byte(versionedTenantPlanSnapshotJSON(t, plans, "quota-v1.redirect-target", time.Now().UnixMilli())))
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL, http.StatusFound)
	}))
	defer redirector.Close()

	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE", "url")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL", redirector.URL)
	if _, err := tenantRateLimitPlansFromEnv(context.Background()); err == nil {
		t.Fatalf("expected tenant plan URL redirect to fail")
	}
}

func TestTenantRateLimitPlansFromEnvLoadsURLSnapshotWithBearerToken(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	plans := map[string]ratelimitinfra.Plan{"tenant-url": {RequestsPerSecond: 9, Burst: 10}}
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer quota-config-token" {
			t.Fatalf("expected bearer token auth header, got %q", request.Header.Get("Authorization"))
		}
		_, _ = writer.Write([]byte(versionedTenantPlanSnapshotJSON(t, plans, "quota-v1.url-auth", time.Now().UnixMilli())))
	}))
	defer server.Close()
	previousTransport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE", "url")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL", server.URL)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_BEARER_TOKEN", "quota-config-token")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_MAX_AGE", "1h")
	snapshot, err := tenantRateLimitPlansFromEnv(context.Background())
	if err != nil {
		t.Fatalf("load authenticated url tenant plans: %v", err)
	}
	if snapshot.Source != "url" || snapshot.Version != "quota-v1.url-auth" || !snapshot.ChecksumPresent {
		t.Fatalf("unexpected authenticated url tenant plan snapshot: %+v", snapshot)
	}
}

func TestTenantRateLimitPlansFromEnvLoadsURLSnapshotWithCAFile(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	plans := map[string]ratelimitinfra.Plan{"tenant-url": {RequestsPerSecond: 9, Burst: 10}}
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(versionedTenantPlanSnapshotJSON(t, plans, "quota-v1.url-ca", time.Now().UnixMilli())))
	}))
	defer server.Close()

	caPath := filepath.Join(t.TempDir(), "quota-url-ca.pem")
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(caPath, caPEM, 0o600); err != nil {
		t.Fatalf("write quota URL CA file: %v", err)
	}

	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE", "url")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL", server.URL)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_CA_FILE", caPath)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_MAX_AGE", "1h")
	snapshot, err := tenantRateLimitPlansFromEnv(context.Background())
	if err != nil {
		t.Fatalf("load URL tenant plans with CA file: %v", err)
	}
	if snapshot.Source != "url" || snapshot.Version != "quota-v1.url-ca" || !snapshot.ChecksumPresent {
		t.Fatalf("unexpected CA-backed URL tenant plan snapshot: %+v", snapshot)
	}
}

func TestTenantRateLimitPlansFromEnvRejectsURLSnapshotWithoutChecksumWhenRequired(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"version":"quota-v1.url-no-checksum","generated_at_unix_ms":1800000000000,"plans":{"tenant-url":{"requests_per_second":9,"burst":10}}}`))
	}))
	defer server.Close()

	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE", "url")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL", server.URL)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_REQUIRE_CHECKSUM", "true")
	if _, err := tenantRateLimitPlansFromEnv(context.Background()); err == nil {
		t.Fatalf("expected URL snapshot without checksum to fail when checksum is required")
	}
}

func TestTenantRateLimitPlansFromEnvRejectsURLBearerTokenWithoutHTTPS(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Fatalf("tenant plan URL source should reject before sending bearer token over HTTP")
	}))
	defer server.Close()

	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE", "url")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL", server.URL)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_BEARER_TOKEN", "quota-config-token")
	if _, err := tenantRateLimitPlansFromEnv(context.Background()); err == nil {
		t.Fatalf("expected bearer token over HTTP to fail")
	}
}

func TestTenantRateLimitPlansFromEnvRejectsHTTPWhenURLRequiresHTTPS(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Fatalf("tenant plan URL source should reject HTTP before request")
	}))
	defer server.Close()

	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE", "url")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL", server.URL)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_REQUIRE_HTTPS", "true")
	if _, err := tenantRateLimitPlansFromEnv(context.Background()); err == nil {
		t.Fatalf("expected HTTP url source to fail when HTTPS is required")
	}
}

func TestTenantRateLimitPlansFromEnvRejectsURLTLSConfigWithoutHTTPS(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Fatalf("tenant plan URL source should reject TLS config before HTTP request")
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "quota-url-ca.pem")
	if err := os.WriteFile(path, []byte("not used for http"), 0o600); err != nil {
		t.Fatalf("write placeholder CA file: %v", err)
	}

	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE", "url")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL", server.URL)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_CA_FILE", path)
	if _, err := tenantRateLimitPlansFromEnv(context.Background()); err == nil {
		t.Fatalf("expected URL TLS config over HTTP to fail")
	}
}

func TestTenantRateLimitPlansFromEnvRejectsURLClientCertWithoutKey(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Fatalf("tenant plan URL source should reject incomplete client certificate before request")
	}))
	defer server.Close()
	certPath := filepath.Join(t.TempDir(), "quota-url-client.pem")
	if err := os.WriteFile(certPath, []byte("not used without key"), 0o600); err != nil {
		t.Fatalf("write placeholder client cert file: %v", err)
	}

	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE", "url")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL", server.URL)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_CLIENT_CERT_FILE", certPath)
	if _, err := tenantRateLimitPlansFromEnv(context.Background()); err == nil {
		t.Fatalf("expected incomplete URL client certificate config to fail")
	}
}

func TestTenantRateLimitPlansFromEnvRejectsUnsupportedSnapshotVersion(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	path := filepath.Join(t.TempDir(), "tenant-plans.json")
	plans := map[string]ratelimitinfra.Plan{"tenant-a": {RequestsPerSecond: 5, Burst: 6}}
	if err := os.WriteFile(path, []byte(versionedTenantPlanSnapshotJSON(t, plans, "quota-v2.bad", time.Now().UnixMilli())), 0o600); err != nil {
		t.Fatalf("write tenant plans file: %v", err)
	}
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_FILE", path)
	if _, err := tenantRateLimitPlansFromEnv(context.Background()); err == nil {
		t.Fatalf("expected unsupported snapshot version to fail")
	}
}

func TestTenantRateLimitPlansFromEnvRejectsStaleURLSnapshot(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	plans := map[string]ratelimitinfra.Plan{"tenant-url": {RequestsPerSecond: 9, Burst: 10}}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(versionedTenantPlanSnapshotJSON(t, plans, "quota-v1.stale", time.Now().Add(-2*time.Hour).UnixMilli())))
	}))
	defer server.Close()

	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE", "url")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL", server.URL)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_MAX_AGE", "1h")
	if _, err := tenantRateLimitPlansFromEnv(context.Background()); err == nil {
		t.Fatalf("expected stale url snapshot to fail")
	}
}

func TestTenantRateLimitPlansFromEnvRejectsFutureSnapshot(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	path := filepath.Join(t.TempDir(), "tenant-plans-future.json")
	plans := map[string]ratelimitinfra.Plan{"tenant-a": {RequestsPerSecond: 5, Burst: 6}}
	if err := os.WriteFile(path, []byte(versionedTenantPlanSnapshotJSON(t, plans, "quota-v1.future", time.Now().Add(time.Hour).UnixMilli())), 0o600); err != nil {
		t.Fatalf("write future tenant plans file: %v", err)
	}
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_FILE", path)
	if _, err := tenantRateLimitPlansFromEnv(context.Background()); err == nil {
		t.Fatalf("expected future tenant plan snapshot to fail")
	}
}

func TestTenantRateLimitPlansFromEnvLoadsExplicitInlineSource(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE", "inline")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_JSON", `{"tenant-a":{"requests_per_second":7,"burst":8}}`)
	snapshot, err := tenantRateLimitPlansFromEnv(context.Background())
	if err != nil {
		t.Fatalf("load inline tenant plans: %v", err)
	}
	if snapshot.Source != "inline" {
		t.Fatalf("expected inline tenant plan source, got %q", snapshot.Source)
	}
	if snapshot.Plans["tenant-a"].RequestsPerSecond != 7 || snapshot.Plans["tenant-a"].Burst != 8 {
		t.Fatalf("unexpected tenant plan from inline source: %+v", snapshot.Plans["tenant-a"])
	}
}

func TestTenantRateLimitPlansFromEnvRejectsInvalidJSON(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_JSON", "{")
	if _, err := tenantRateLimitPlansFromEnv(context.Background()); err == nil {
		t.Fatalf("expected invalid tenant plans JSON to fail")
	}
}

func TestTenantRateLimitPlansFromEnvRejectsUnsupportedSource(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE", "db")
	if _, err := tenantRateLimitPlansFromEnv(context.Background()); err == nil {
		t.Fatalf("expected unsupported tenant plan source to fail closed")
	}
}

func TestNewRateLimiterFromEnvTenantPlanReloadRequiresFileSource(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_ENABLED", "true")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_RPS", "1")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE", "inline")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_JSON", `{"tenant-a":{"requests_per_second":7,"burst":8}}`)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_RELOAD_INTERVAL", "1s")
	if _, _, err := newRateLimiterFromEnv(context.Background(), nil); err == nil {
		t.Fatalf("expected tenant plan reload with inline source to fail")
	}
}

func TestTenantPlanReloadIntervalFromEnv(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	if interval, err := tenantPlanReloadIntervalFromEnv(); err != nil || interval != 0 {
		t.Fatalf("expected empty reload interval to be disabled, interval=%s err=%v", interval, err)
	}
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_RELOAD_INTERVAL", "250ms")
	if interval, err := tenantPlanReloadIntervalFromEnv(); err != nil || interval != 250*time.Millisecond {
		t.Fatalf("expected reload interval to parse, interval=%s err=%v", interval, err)
	}
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_RELOAD_INTERVAL", "-1s")
	if _, err := tenantPlanReloadIntervalFromEnv(); err == nil {
		t.Fatalf("expected negative reload interval to fail")
	}
}

func TestStartTenantPlanReloaderRequiresFile(t *testing.T) {
	limiter, err := ratelimitinfra.New(ratelimitinfra.Config{
		Enabled:           true,
		RequestsPerSecond: 1,
		Burst:             1,
	})
	if err != nil {
		t.Fatalf("new limiter: %v", err)
	}
	if _, err := startTenantPlanReloader(context.Background(), limiter, "file", "", 0, false, false, time.Millisecond); err == nil {
		t.Fatalf("expected missing tenant plan reload file to fail")
	}
}

func TestTenantRateLimitPlansFromSourceRejectsMissingChecksumWhenRequired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tenant-plans.json")
	payload := `{"version":"quota-v1.reload-no-checksum","generated_at_unix_ms":1800000000000,"plans":{"tenant-a":{"requests_per_second":5,"burst":6}}}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write tenant plans file: %v", err)
	}
	if _, err := tenantRateLimitPlansFromSource(context.Background(), "file", path, 0, true, false); err == nil {
		t.Fatalf("expected reload source without checksum to fail when checksum is required")
	}
}

func TestTenantRateLimitPlansFromSourceRejectsOversizedFileSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tenant-plans-too-large.json")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", tenantPlanSnapshotMaxBytes+1)), 0o600); err != nil {
		t.Fatalf("write oversized tenant plans file: %v", err)
	}
	if _, err := tenantRateLimitPlansFromSource(context.Background(), "file", path, 0, false, false); err == nil {
		t.Fatalf("expected oversized reload file snapshot to fail")
	}
}

func TestTenantRateLimitPlansFromSourceRejectsFutureSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tenant-plans-future.json")
	plans := map[string]ratelimitinfra.Plan{"tenant-a": {RequestsPerSecond: 5, Burst: 6}}
	if err := os.WriteFile(path, []byte(versionedTenantPlanSnapshotJSON(t, plans, "quota-v1.reload-future", time.Now().Add(time.Hour).UnixMilli())), 0o600); err != nil {
		t.Fatalf("write future tenant plans file: %v", err)
	}
	if _, err := tenantRateLimitPlansFromSource(context.Background(), "file", path, 0, false, false); err == nil {
		t.Fatalf("expected future reload file snapshot to fail")
	}
}

func TestStartTenantPlanReloaderUpdatesLimiter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tenant-plans.json")
	plans := map[string]ratelimitinfra.Plan{"tenant-vip": {RequestsPerSecond: 10, Burst: 2}}
	if err := os.WriteFile(path, []byte(versionedTenantPlanSnapshotJSON(t, plans, "quota-v1.reload", time.Now().UnixMilli())), 0o600); err != nil {
		t.Fatalf("write tenant plans: %v", err)
	}
	limiter, err := ratelimitinfra.New(ratelimitinfra.Config{
		Enabled:           true,
		KeyScope:          "tenant",
		RequestsPerSecond: 1,
		Burst:             1,
		IdentityFunc: func(ctx context.Context) (ratelimitinfra.Identity, error) {
			md, _ := metadata.FromIncomingContext(ctx)
			return ratelimitinfra.Identity{TenantID: md.Get("tenant")[0]}, nil
		},
	})
	if err != nil {
		t.Fatalf("new tenant limiter: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop, err := startTenantPlanReloader(ctx, limiter, "file", path, 0, false, false, 5*time.Millisecond)
	if err != nil {
		t.Fatalf("start tenant plan reloader: %v", err)
	}
	defer stop()

	waitForAPIGatewayTestCondition(t, time.Second, func() bool {
		snapshot := limiter.Snapshot()
		return snapshot.TenantPlans == 1 && snapshot.TenantReloads > 0 && snapshot.TenantReloadAt > 0 && snapshot.TenantPlanVersion == "quota-v1.reload"
	})

	interceptor := limiter.UnaryServerInterceptor()
	requestCtx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("tenant", "tenant-vip"))
	info := &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.gateway.v1.GatewayService/SendMessage"}
	handler := func(context.Context, any) (any, error) { return "ok", nil }
	for i := 0; i < 2; i++ {
		if _, err := interceptor(requestCtx, nil, info, handler); err != nil {
			t.Fatalf("request %d should pass with reloaded tenant plan: %v", i, err)
		}
	}
	if _, err := interceptor(requestCtx, nil, info, handler); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("expected third request to use reloaded burst and be limited, got %v", err)
	}
}

func TestStartTenantPlanReloaderKeepsLastValidURLSnapshotOnError(t *testing.T) {
	var requests atomic.Int64
	validPlans := map[string]ratelimitinfra.Plan{"tenant-vip": {RequestsPerSecond: 10, Burst: 2}}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if requests.Add(1) == 1 {
			_, _ = writer.Write([]byte(versionedTenantPlanSnapshotJSON(t, validPlans, "quota-v1.url-reload", time.Now().UnixMilli())))
			return
		}
		_, _ = writer.Write([]byte(`{"version":"quota-v1.url-reload-bad","generated_at_unix_ms":1800000000000,"checksum":"sha256:0000000000000000000000000000000000000000000000000000000000000000","plans":{"tenant-vip":{"requests_per_second":1,"burst":1}}}`))
	}))
	defer server.Close()

	limiter, err := ratelimitinfra.New(ratelimitinfra.Config{
		Enabled:           true,
		KeyScope:          "tenant",
		RequestsPerSecond: 1,
		Burst:             1,
		IdentityFunc: func(ctx context.Context) (ratelimitinfra.Identity, error) {
			md, _ := metadata.FromIncomingContext(ctx)
			return ratelimitinfra.Identity{TenantID: md.Get("tenant")[0]}, nil
		},
	})
	if err != nil {
		t.Fatalf("new tenant limiter: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop, err := startTenantPlanReloader(ctx, limiter, "url", server.URL, time.Hour, false, false, 5*time.Millisecond)
	if err != nil {
		t.Fatalf("start tenant plan reloader: %v", err)
	}
	defer stop()

	waitForAPIGatewayTestCondition(t, time.Second, func() bool {
		snapshot := limiter.Snapshot()
		return snapshot.TenantPlans == 1 && snapshot.TenantPlanVersion == "quota-v1.url-reload"
	})
	waitForAPIGatewayTestCondition(t, time.Second, func() bool {
		return limiter.Snapshot().TenantErrors > 0
	})

	interceptor := limiter.UnaryServerInterceptor()
	requestCtx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("tenant", "tenant-vip"))
	info := &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.gateway.v1.GatewayService/SendMessage"}
	handler := func(context.Context, any) (any, error) { return "ok", nil }
	for i := 0; i < 2; i++ {
		if _, err := interceptor(requestCtx, nil, info, handler); err != nil {
			t.Fatalf("request %d should still use last valid tenant plan: %v", i, err)
		}
	}
	if _, err := interceptor(requestCtx, nil, info, handler); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("expected third request to use last valid tenant plan and be limited, got %v", err)
	}
}

func TestStartTenantPlanReloaderRecordsLoadErrors(t *testing.T) {
	limiter, err := ratelimitinfra.New(ratelimitinfra.Config{
		Enabled:           true,
		RequestsPerSecond: 1,
		Burst:             1,
	})
	if err != nil {
		t.Fatalf("new limiter: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop, err := startTenantPlanReloader(ctx, limiter, "file", filepath.Join(t.TempDir(), "missing.json"), 0, false, false, 5*time.Millisecond)
	if err != nil {
		t.Fatalf("start tenant plan reloader: %v", err)
	}
	defer stop()

	waitForAPIGatewayTestCondition(t, time.Second, func() bool {
		return limiter.Snapshot().TenantErrors > 0
	})
}
