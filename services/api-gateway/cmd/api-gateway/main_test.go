package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	gatewayauth "github.com/qsyy0921/IM/internal/gatewayauth"
	ratelimitinfra "github.com/qsyy0921/IM/services/api-gateway/internal/infrastructure/ratelimit"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestGRPCClientTLSConfigFromEnvDisabledByDefault(t *testing.T) {
	clearAPIGatewayTestTLSConfig(t, "NEXUSIM_API_GATEWAY_MESSAGE_TLS")
	config := grpcClientTLSConfigFromEnv("NEXUSIM_API_GATEWAY_MESSAGE_TLS")
	if config.Enabled() {
		t.Fatalf("expected empty api-gateway downstream tls config to be disabled: %+v", config)
	}
}

func TestGRPCClientTLSCredentialsRequireCAFile(t *testing.T) {
	_, err := grpcClientTLSCredentials(grpcClientTLSConfig{
		EnvPrefix:  "NEXUSIM_API_GATEWAY_MESSAGE_TLS",
		ServerName: "message-service.nexusim.local",
	})
	if err == nil {
		t.Fatalf("expected missing CA file error")
	}
}

func TestGRPCClientTLSCredentialsRequireClientKeyPair(t *testing.T) {
	dir := t.TempDir()
	caFile, _ := writeAPIGatewayTLSTestCert(t, dir, "ca")
	_, err := grpcClientTLSCredentials(grpcClientTLSConfig{
		EnvPrefix:      "NEXUSIM_API_GATEWAY_MESSAGE_TLS",
		CAFile:         caFile,
		ClientCertFile: "client.crt",
	})
	if err == nil {
		t.Fatalf("expected partial client certificate config to fail")
	}
}

func TestGRPCClientTLSCredentialsLoadCAAndClientCert(t *testing.T) {
	dir := t.TempDir()
	caFile, _ := writeAPIGatewayTLSTestCert(t, dir, "ca")
	clientCertFile, clientKeyFile := writeAPIGatewayTLSTestCert(t, dir, "api-gateway")
	creds, err := grpcClientTLSCredentials(grpcClientTLSConfig{
		EnvPrefix:      "NEXUSIM_API_GATEWAY_MESSAGE_TLS",
		CAFile:         caFile,
		ServerName:     "message-service.nexusim.local",
		ClientCertFile: clientCertFile,
		ClientKeyFile:  clientKeyFile,
	})
	if err != nil {
		t.Fatalf("load api-gateway downstream tls credentials: %v", err)
	}
	if creds == nil {
		t.Fatalf("expected downstream tls credentials")
	}
}

func TestGRPCClientTLSConfigFromEnvLoadsValues(t *testing.T) {
	clearAPIGatewayTestTLSConfig(t, "NEXUSIM_API_GATEWAY_MESSAGE_TLS")
	t.Setenv("NEXUSIM_API_GATEWAY_MESSAGE_TLS_CA_FILE", "ca.crt")
	t.Setenv("NEXUSIM_API_GATEWAY_MESSAGE_TLS_SERVER_NAME", "message-service.nexusim.local")
	t.Setenv("NEXUSIM_API_GATEWAY_MESSAGE_TLS_CLIENT_CERT_FILE", "client.crt")
	t.Setenv("NEXUSIM_API_GATEWAY_MESSAGE_TLS_CLIENT_KEY_FILE", "client.key")
	config := grpcClientTLSConfigFromEnv("NEXUSIM_API_GATEWAY_MESSAGE_TLS")
	if config.CAFile != "ca.crt" ||
		config.ServerName != "message-service.nexusim.local" ||
		config.ClientCertFile != "client.crt" ||
		config.ClientKeyFile != "client.key" {
		t.Fatalf("unexpected downstream tls config: %+v", config)
	}
}

func TestAPIGatewayGRPCTLSConfigFromEnvDisabledByDefault(t *testing.T) {
	clearAPIGatewayServerTLSConfig(t)
	_, ok, err := apiGatewayGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load api-gateway grpc tls config: %v", err)
	}
	if ok {
		t.Fatalf("expected api-gateway grpc tls config to be disabled")
	}
}

func TestAPIGatewayGRPCTLSConfigRequiresCertKeyPair(t *testing.T) {
	clearAPIGatewayServerTLSConfig(t)
	t.Setenv("NEXUSIM_API_GATEWAY_GRPC_TLS_CERT_FILE", "server.crt")
	_, ok, err := apiGatewayGRPCTLSConfigFromEnv()
	if err == nil || !ok {
		t.Fatalf("expected partial server cert config to fail, ok=%t err=%v", ok, err)
	}
}

func TestAPIGatewayGRPCTLSConfigRequiresClientCAWhenClientCertsRequired(t *testing.T) {
	dir := t.TempDir()
	serverCertFile, serverKeyFile := writeAPIGatewayTLSTestCert(t, dir, "api-gateway")
	clearAPIGatewayServerTLSConfig(t)
	t.Setenv("NEXUSIM_API_GATEWAY_GRPC_TLS_CERT_FILE", serverCertFile)
	t.Setenv("NEXUSIM_API_GATEWAY_GRPC_TLS_KEY_FILE", serverKeyFile)
	t.Setenv("NEXUSIM_API_GATEWAY_GRPC_TLS_REQUIRE_CLIENT_CERT", "true")
	_, ok, err := apiGatewayGRPCTLSConfigFromEnv()
	if err == nil || !ok {
		t.Fatalf("expected missing client CA to fail, ok=%t err=%v", ok, err)
	}
}

func TestAPIGatewayGRPCTLSConfigLoadsMutualTLSAllowlist(t *testing.T) {
	dir := t.TempDir()
	serverCertFile, serverKeyFile := writeAPIGatewayTLSTestCert(t, dir, "api-gateway")
	caFile, _ := writeAPIGatewayTLSTestCert(t, dir, "ca")
	clearAPIGatewayServerTLSConfig(t)
	t.Setenv("NEXUSIM_API_GATEWAY_GRPC_TLS_CERT_FILE", serverCertFile)
	t.Setenv("NEXUSIM_API_GATEWAY_GRPC_TLS_KEY_FILE", serverKeyFile)
	t.Setenv("NEXUSIM_API_GATEWAY_GRPC_TLS_CLIENT_CA_FILE", caFile)
	t.Setenv("NEXUSIM_API_GATEWAY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", "desktop-client.nexusim.local")
	t.Setenv("NEXUSIM_API_GATEWAY_GRPC_TLS_CLIENT_ALLOWED_URIS", "spiffe://nexusim/desktop-client")
	tlsConfig, ok, err := apiGatewayGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load api-gateway grpc tls config: %v", err)
	}
	if !ok || tlsConfig == nil {
		t.Fatalf("expected api-gateway grpc tls config")
	}
	if tlsConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("expected RequireAndVerifyClientCert, got %v", tlsConfig.ClientAuth)
	}
	if tlsConfig.VerifyConnection == nil {
		t.Fatalf("expected client certificate allowlist verifier")
	}
}

func TestNewAuthenticatorFromEnvDefaultsToAPIGatewayAudience(t *testing.T) {
	clearAPIGatewayAuthConfig(t)
	expiresAt := time.Now().Add(time.Minute)
	t.Setenv("NEXUSIM_API_GATEWAY_AUTH_MODE", "hmac")
	t.Setenv("NEXUSIM_API_GATEWAY_AUTH_HMAC_SECRET", "gateway-secret")

	authenticator, err := newAuthenticatorFromEnv()
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	t.Cleanup(authenticator.Close)

	apiGatewayToken, err := gatewayauth.SignGatewayToken("gateway-secret", map[string]string{
		"tenant_id": "tenant-1",
		"user_id":   "user-1",
		"device_id": "device-1",
		"aud":       "api-gateway",
	}, expiresAt)
	if err != nil {
		t.Fatalf("sign api-gateway token: %v", err)
	}
	if _, err := authenticator.Authenticate(httptest.NewRequest("GET", "/?token="+apiGatewayToken, nil)); err != nil {
		t.Fatalf("authenticate api-gateway token: %v", err)
	}

	pushToken, err := gatewayauth.SignGatewayToken("gateway-secret", map[string]string{
		"tenant_id": "tenant-1",
		"user_id":   "user-1",
		"device_id": "device-1",
		"aud":       "push-gateway",
	}, expiresAt)
	if err != nil {
		t.Fatalf("sign push token: %v", err)
	}
	if _, err := authenticator.Authenticate(httptest.NewRequest("GET", "/?token="+pushToken, nil)); err == nil {
		t.Fatalf("expected push-gateway token to be rejected by default api-gateway audience")
	}
}

func TestNewAuthenticatorFromEnvAllowsExplicitLegacyAudience(t *testing.T) {
	clearAPIGatewayAuthConfig(t)
	expiresAt := time.Now().Add(time.Minute)
	t.Setenv("NEXUSIM_API_GATEWAY_AUTH_MODE", "hmac")
	t.Setenv("NEXUSIM_API_GATEWAY_AUTH_HMAC_SECRET", "gateway-secret")
	t.Setenv("NEXUSIM_API_GATEWAY_AUTH_AUDIENCE", "push-gateway")

	authenticator, err := newAuthenticatorFromEnv()
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	t.Cleanup(authenticator.Close)

	token, err := gatewayauth.SignGatewayToken("gateway-secret", map[string]string{
		"tenant_id": "tenant-1",
		"user_id":   "user-1",
		"device_id": "device-1",
		"aud":       "push-gateway",
	}, expiresAt)
	if err != nil {
		t.Fatalf("sign push token: %v", err)
	}
	if _, err := authenticator.Authenticate(httptest.NewRequest("GET", "/?token="+token, nil)); err != nil {
		t.Fatalf("authenticate explicit legacy audience: %v", err)
	}
}

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
	if !snapshot.Enabled || snapshot.Backend != "redis" || snapshot.Burst != 8 || snapshot.RedisWindowMS != 2000 || snapshot.RedisFailOpen {
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
	if err := os.WriteFile(path, []byte(versionedTenantPlanSnapshotJSON(t, plans, "quota-v1.20260614", 1_800_000_000_000)), 0o600); err != nil {
		t.Fatalf("write tenant plans file: %v", err)
	}
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_FILE", path)
	snapshot, err := tenantRateLimitPlansFromEnv(context.Background())
	if err != nil {
		t.Fatalf("load versioned tenant plans file: %v", err)
	}
	if snapshot.Source != "file" || snapshot.Version != "quota-v1.20260614" || snapshot.GeneratedAtUnixMS != 1_800_000_000_000 || !snapshot.ChecksumPresent {
		t.Fatalf("unexpected versioned tenant plan snapshot: %+v", snapshot)
	}
	if snapshot.Plans["tenant-a"].RequestsPerSecond != 5 || snapshot.Plans["tenant-a"].Burst != 6 {
		t.Fatalf("unexpected tenant plan from versioned file: %+v", snapshot.Plans["tenant-a"])
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

	_, err := tenantRateLimitPlansFromURL(context.Background(), endpoint, 0, false)
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
	if _, err := startTenantPlanReloader(context.Background(), limiter, "file", "", 0, false, time.Millisecond); err == nil {
		t.Fatalf("expected missing tenant plan reload file to fail")
	}
}

func TestTenantRateLimitPlansFromSourceRejectsMissingChecksumWhenRequired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tenant-plans.json")
	payload := `{"version":"quota-v1.reload-no-checksum","generated_at_unix_ms":1800000000000,"plans":{"tenant-a":{"requests_per_second":5,"burst":6}}}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write tenant plans file: %v", err)
	}
	if _, err := tenantRateLimitPlansFromSource(context.Background(), "file", path, 0, true); err == nil {
		t.Fatalf("expected reload source without checksum to fail when checksum is required")
	}
}

func TestStartTenantPlanReloaderUpdatesLimiter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tenant-plans.json")
	plans := map[string]ratelimitinfra.Plan{"tenant-vip": {RequestsPerSecond: 10, Burst: 2}}
	if err := os.WriteFile(path, []byte(versionedTenantPlanSnapshotJSON(t, plans, "quota-v1.reload", 1_800_000_000_100)), 0o600); err != nil {
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
	stop, err := startTenantPlanReloader(ctx, limiter, "file", path, 0, false, 5*time.Millisecond)
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
	stop, err := startTenantPlanReloader(ctx, limiter, "url", server.URL, time.Hour, false, 5*time.Millisecond)
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
	stop, err := startTenantPlanReloader(ctx, limiter, "file", filepath.Join(t.TempDir(), "missing.json"), 0, false, 5*time.Millisecond)
	if err != nil {
		t.Fatalf("start tenant plan reloader: %v", err)
	}
	defer stop()

	waitForAPIGatewayTestCondition(t, time.Second, func() bool {
		return limiter.Snapshot().TenantErrors > 0
	})
}

func TestValidateTrustedMetadataBackendConfigAllowsPrivateAddressWithoutMTLS(t *testing.T) {
	err := validateTrustedMetadataBackendConfig(
		"message-service",
		"172.31.50.10:10495",
		"metadata",
		grpcClientTLSConfig{},
	)
	if err != nil {
		t.Fatalf("expected private address to be allowed without mTLS, got %v", err)
	}
}

func TestValidateTrustedMetadataBackendConfigRequiresMTLSForPublicAddress(t *testing.T) {
	err := validateTrustedMetadataBackendConfig(
		"message-service",
		"8.8.8.8:10495",
		"verified-metadata",
		grpcClientTLSConfig{},
	)
	if err == nil {
		t.Fatalf("expected public address without mTLS client cert to fail")
	}
}

func TestValidateTrustedMetadataBackendConfigAllowsMTLSForPublicAddress(t *testing.T) {
	err := validateTrustedMetadataBackendConfig(
		"message-service",
		"8.8.8.8:10495",
		"verified-metadata",
		grpcClientTLSConfig{
			ClientCertFile: "client.crt",
			ClientKeyFile:  "client.key",
		},
	)
	if err != nil {
		t.Fatalf("expected public address with mTLS client cert to be allowed, got %v", err)
	}
}

func TestValidateTrustedMetadataBackendConfigIgnoresBodyAuth(t *testing.T) {
	err := validateTrustedMetadataBackendConfig(
		"message-service",
		"8.8.8.8:10495",
		"body",
		grpcClientTLSConfig{},
	)
	if err != nil {
		t.Fatalf("expected body auth to skip trusted metadata guard, got %v", err)
	}
}

func TestValidateAPIGatewayAuthListenerConfigAllowsPrivateAddressForMock(t *testing.T) {
	if err := validateAPIGatewayAuthListenerConfig("172.31.50.10:12000", "mock", false); err != nil {
		t.Fatalf("expected private listener mock auth to be allowed: %v", err)
	}
}

func TestValidateAPIGatewayAuthListenerConfigRejectsPublicAddressForMock(t *testing.T) {
	err := validateAPIGatewayAuthListenerConfig("8.8.8.8:12000", "mock", false)
	if err == nil {
		t.Fatalf("expected public listener mock auth to be rejected")
	}
}

func TestValidateAPIGatewayAuthListenerConfigRejectsPublicAddressForSignedAuthWithoutTLS(t *testing.T) {
	err := validateAPIGatewayAuthListenerConfig("8.8.8.8:12000", "hmac", false)
	if err == nil {
		t.Fatalf("expected signed auth without TLS on public listener to be rejected")
	}
}

func TestValidateAPIGatewayAuthListenerConfigAllowsPublicAddressForSignedAuthWithTLS(t *testing.T) {
	if err := validateAPIGatewayAuthListenerConfig("8.8.8.8:12000", "jwt", true); err != nil {
		t.Fatalf("expected signed auth with TLS to be allowed on public listener: %v", err)
	}
}

func TestValidateTrustedMetadataBackendConfigCoversIdentityService(t *testing.T) {
	err := validateTrustedMetadataBackendConfig(
		"identity-service",
		"8.8.8.8:10501",
		"verified-metadata",
		grpcClientTLSConfig{},
	)
	if err == nil {
		t.Fatalf("expected identity-service public address without mTLS client cert to fail")
	}
}

func TestAPIGatewayRegisterLegacyDescriptorsDefaultsToFalse(t *testing.T) {
	t.Setenv("NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS", "")
	t.Setenv("NEXUSIM_API_GATEWAY_LEGACY_DESCRIPTORS_ALLOWED_UNTIL", "")
	enabled, err := apiGatewayRegisterLegacyDescriptors()
	if err != nil {
		t.Fatalf("load register legacy descriptors config: %v", err)
	}
	if enabled {
		t.Fatalf("expected legacy descriptor registration to default to false")
	}
}

func TestAPIGatewayRegisterLegacyDescriptorsCanBeEnabled(t *testing.T) {
	t.Setenv("NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS", "true")
	t.Setenv("NEXUSIM_API_GATEWAY_LEGACY_DESCRIPTORS_ALLOWED_UNTIL", "")
	enabled, err := apiGatewayRegisterLegacyDescriptors()
	if err != nil {
		t.Fatalf("load register legacy descriptors config: %v", err)
	}
	if !enabled {
		t.Fatalf("expected legacy descriptor registration to be enabled")
	}
}

func TestAPIGatewayRegisterLegacyDescriptorsRejectsInvalidValue(t *testing.T) {
	t.Setenv("NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS", "sometimes")
	t.Setenv("NEXUSIM_API_GATEWAY_LEGACY_DESCRIPTORS_ALLOWED_UNTIL", "")
	if _, err := apiGatewayRegisterLegacyDescriptors(); err == nil {
		t.Fatalf("expected invalid legacy descriptor registration config to fail")
	}
}

func TestAPIGatewayLegacyDescriptorConfigAllowsFutureDeadline(t *testing.T) {
	t.Setenv("NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS", "true")
	t.Setenv("NEXUSIM_API_GATEWAY_LEGACY_DESCRIPTORS_ALLOWED_UNTIL", "2026-06-15T00:00:00Z")
	config, err := apiGatewayLegacyDescriptorConfigFromEnv(func() time.Time {
		return time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatalf("load legacy descriptor config: %v", err)
	}
	if !config.Register || config.AllowedUntilUnixMS != time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC).UnixMilli() {
		t.Fatalf("unexpected legacy descriptor config: %+v", config)
	}
}

func TestAPIGatewayLegacyDescriptorConfigRejectsExpiredDeadline(t *testing.T) {
	t.Setenv("NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS", "true")
	t.Setenv("NEXUSIM_API_GATEWAY_LEGACY_DESCRIPTORS_ALLOWED_UNTIL", "2026-06-13T00:00:00Z")
	if _, err := apiGatewayLegacyDescriptorConfigFromEnv(func() time.Time {
		return time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	}); err == nil {
		t.Fatalf("expected expired legacy descriptor opt-in to fail")
	}
}

func TestAPIGatewayLegacyDescriptorConfigRejectsInvalidDeadline(t *testing.T) {
	t.Setenv("NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS", "false")
	t.Setenv("NEXUSIM_API_GATEWAY_LEGACY_DESCRIPTORS_ALLOWED_UNTIL", "tomorrow")
	if _, err := apiGatewayLegacyDescriptorConfigFromEnv(func() time.Time { return time.Now() }); err == nil {
		t.Fatalf("expected invalid legacy descriptor deadline to fail")
	}
}

func TestAPIGatewayTraceConfigDefaultsToDisabled(t *testing.T) {
	clearAPIGatewayTraceConfig(t)
	config, err := apiGatewayTraceConfigFromEnv()
	if err != nil {
		t.Fatalf("load trace config: %v", err)
	}
	if config.Enabled || config.ServiceName != "api-gateway" || config.Exporter != "stdout" || config.SamplingRatio != 1 {
		t.Fatalf("unexpected default trace config: %+v", config)
	}
}

func TestAPIGatewayTraceConfigLoadsOTLPGRPC(t *testing.T) {
	clearAPIGatewayTraceConfig(t)
	t.Setenv("NEXUSIM_API_GATEWAY_OTEL_TRACES_ENABLED", "true")
	t.Setenv("NEXUSIM_API_GATEWAY_OTEL_SERVICE_NAME", "api-gateway-test")
	t.Setenv("NEXUSIM_API_GATEWAY_OTEL_TRACES_EXPORTER", "otlp-grpc")
	t.Setenv("NEXUSIM_API_GATEWAY_OTEL_TRACES_OTLP_ENDPOINT", "127.0.0.1:4317")
	t.Setenv("NEXUSIM_API_GATEWAY_OTEL_TRACES_OTLP_INSECURE", "true")
	t.Setenv("NEXUSIM_API_GATEWAY_OTEL_TRACES_SAMPLING_RATIO", "0.25")

	config, err := apiGatewayTraceConfigFromEnv()
	if err != nil {
		t.Fatalf("load trace config: %v", err)
	}
	if !config.Enabled ||
		config.ServiceName != "api-gateway-test" ||
		config.Exporter != "otlp-grpc" ||
		config.OTLPEndpoint != "127.0.0.1:4317" ||
		!config.OTLPInsecure ||
		config.SamplingRatio != 0.25 {
		t.Fatalf("unexpected otlp trace config: %+v", config)
	}
}

func TestAPIGatewayTraceConfigRejectsInvalidValues(t *testing.T) {
	clearAPIGatewayTraceConfig(t)
	t.Setenv("NEXUSIM_API_GATEWAY_OTEL_TRACES_ENABLED", "sometimes")
	if _, err := apiGatewayTraceConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid trace enabled bool to fail")
	}

	clearAPIGatewayTraceConfig(t)
	t.Setenv("NEXUSIM_API_GATEWAY_OTEL_TRACES_SAMPLING_RATIO", "2")
	if _, err := apiGatewayTraceConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid trace sampling ratio to fail")
	}
}

func TestNewRedisUniversalClientRequiresSentinelConfig(t *testing.T) {
	if _, err := newRedisUniversalClient(redisClientConfig{Mode: "sentinel"}); err == nil {
		t.Fatalf("expected sentinel mode without master name to fail")
	}
	if _, err := newRedisUniversalClient(redisClientConfig{Mode: "cluster"}); err == nil {
		t.Fatalf("expected unsupported redis mode to fail")
	}
}

func clearAPIGatewayTestTLSConfig(t *testing.T, prefix string) {
	t.Helper()
	t.Setenv(prefix+"_CA_FILE", "")
	t.Setenv(prefix+"_SERVER_NAME", "")
	t.Setenv(prefix+"_CLIENT_CERT_FILE", "")
	t.Setenv(prefix+"_CLIENT_KEY_FILE", "")
}

func clearAPIGatewayServerTLSConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_API_GATEWAY_GRPC_TLS_CERT_FILE", "")
	t.Setenv("NEXUSIM_API_GATEWAY_GRPC_TLS_KEY_FILE", "")
	t.Setenv("NEXUSIM_API_GATEWAY_GRPC_TLS_CLIENT_CA_FILE", "")
	t.Setenv("NEXUSIM_API_GATEWAY_GRPC_TLS_REQUIRE_CLIENT_CERT", "")
	t.Setenv("NEXUSIM_API_GATEWAY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", "")
	t.Setenv("NEXUSIM_API_GATEWAY_GRPC_TLS_CLIENT_ALLOWED_URIS", "")
}

func clearAPIGatewayAuthConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_API_GATEWAY_AUTH_MODE", "")
	t.Setenv("NEXUSIM_API_GATEWAY_AUTH_HMAC_SECRET", "")
	t.Setenv("NEXUSIM_API_GATEWAY_AUTH_HMAC_PREVIOUS_SECRETS", "")
	t.Setenv("NEXUSIM_API_GATEWAY_AUTH_JWKS_JSON", "")
	t.Setenv("NEXUSIM_API_GATEWAY_AUTH_JWKS_FILE", "")
	t.Setenv("NEXUSIM_API_GATEWAY_AUTH_JWKS_URL", "")
	t.Setenv("NEXUSIM_API_GATEWAY_AUTH_TRUSTED_ISSUERS", "")
	t.Setenv("NEXUSIM_API_GATEWAY_AUTH_AUDIENCE", "")
}

func clearAPIGatewayRateLimitConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_ENABLED", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_SCOPE", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_RPS", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_BURST", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_JSON", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_FILE", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_BEARER_TOKEN", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_REQUIRE_HTTPS", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_CA_FILE", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_SERVER_NAME", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_CLIENT_CERT_FILE", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_CLIENT_KEY_FILE", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_REQUIRE_CHECKSUM", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_MAX_AGE", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_RELOAD_INTERVAL", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_MAX_KEYS", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_BACKEND", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_MODE", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_ADDR", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_SENTINEL_ADDRS", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_SENTINEL_MASTER_NAME", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_USERNAME", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_PASSWORD", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_DB", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_SENTINEL_USERNAME", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_SENTINEL_PASSWORD", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_KEY_PREFIX", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_WINDOW", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_FAIL_OPEN", "")
}

func clearAPIGatewayTraceConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_API_GATEWAY_OTEL_TRACES_ENABLED", "")
	t.Setenv("NEXUSIM_API_GATEWAY_OTEL_SERVICE_NAME", "")
	t.Setenv("NEXUSIM_API_GATEWAY_OTEL_TRACES_EXPORTER", "")
	t.Setenv("NEXUSIM_API_GATEWAY_OTEL_TRACES_OTLP_ENDPOINT", "")
	t.Setenv("NEXUSIM_API_GATEWAY_OTEL_TRACES_OTLP_INSECURE", "")
	t.Setenv("NEXUSIM_API_GATEWAY_OTEL_TRACES_SAMPLING_RATIO", "")
}

func signAPIGatewayTestToken(t *testing.T, tenantID string, userID string) string {
	t.Helper()
	token, err := gatewayauth.SignGatewayToken("gateway-secret", map[string]string{
		"tenant_id": tenantID,
		"user_id":   userID,
		"device_id": "device-1",
		"aud":       "api-gateway",
	}, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("sign api-gateway token: %v", err)
	}
	return token
}

func versionedTenantPlanSnapshotJSON(t *testing.T, plans map[string]ratelimitinfra.Plan, version string, generatedAtUnixMS int64) string {
	t.Helper()
	checksum, err := tenantPlanSnapshotChecksum(plans)
	if err != nil {
		t.Fatalf("calculate tenant plan checksum: %v", err)
	}
	type planPayload struct {
		RequestsPerSecond float64 `json:"requests_per_second"`
		Burst             int     `json:"burst"`
	}
	payloadPlans := make(map[string]planPayload, len(plans))
	for tenantID, plan := range plans {
		payloadPlans[tenantID] = planPayload{RequestsPerSecond: plan.RequestsPerSecond, Burst: plan.Burst}
	}
	payload := map[string]any{
		"version":              version,
		"generated_at_unix_ms": generatedAtUnixMS,
		"checksum":             checksum,
		"plans":                payloadPlans,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal tenant plan snapshot: %v", err)
	}
	return string(data)
}

func writeAPIGatewayTLSTestCert(t *testing.T, dir string, name string) (string, string) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate tls key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber:          newSerialNumber(t),
		Subject:               pkix.Name{CommonName: "api-gateway-test-" + name},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"message-service.nexusim.local", "api-gateway.nexusim.local"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("create tls cert: %v", err)
	}
	certFile := filepath.Join(dir, name+".crt")
	keyFile := filepath.Join(dir, name+".key")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write tls cert: %v", err)
	}
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privateKeyBytes}), 0o600); err != nil {
		t.Fatalf("write tls key: %v", err)
	}
	return certFile, keyFile
}

func newSerialNumber(t *testing.T) *big.Int {
	t.Helper()
	serial, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		t.Fatalf("generate tls serial: %v", err)
	}
	return serial
}

func waitForAPIGatewayTestCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !condition() {
		t.Fatalf("condition was not met within %s", timeout)
	}
}
