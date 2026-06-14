package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	gatewayauth "github.com/qsyy0921/IM/internal/gatewayauth"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
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
	if snapshot.TenantPlans != 1 || snapshot.KeyScope != "tenant" {
		t.Fatalf("unexpected tenant plan snapshot: %+v", snapshot)
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
	plans, err := tenantRateLimitPlansFromEnv()
	if err != nil {
		t.Fatalf("load tenant plans file: %v", err)
	}
	if plans["tenant-a"].RequestsPerSecond != 5 || plans["tenant-a"].Burst != 6 {
		t.Fatalf("unexpected tenant plan from file: %+v", plans["tenant-a"])
	}
}

func TestTenantRateLimitPlansFromEnvRejectsInvalidJSON(t *testing.T) {
	clearAPIGatewayRateLimitConfig(t)
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_JSON", "{")
	if _, err := tenantRateLimitPlansFromEnv(); err == nil {
		t.Fatalf("expected invalid tenant plans JSON to fail")
	}
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

func TestAPIGatewayRegisterLegacyDescriptorsDefaultsToTrue(t *testing.T) {
	t.Setenv("NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS", "")
	enabled, err := apiGatewayRegisterLegacyDescriptors()
	if err != nil {
		t.Fatalf("load register legacy descriptors config: %v", err)
	}
	if !enabled {
		t.Fatalf("expected legacy descriptor registration to default to true")
	}
}

func TestAPIGatewayRegisterLegacyDescriptorsCanBeDisabled(t *testing.T) {
	t.Setenv("NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS", "false")
	enabled, err := apiGatewayRegisterLegacyDescriptors()
	if err != nil {
		t.Fatalf("load register legacy descriptors config: %v", err)
	}
	if enabled {
		t.Fatalf("expected legacy descriptor registration to be disabled")
	}
}

func TestAPIGatewayRegisterLegacyDescriptorsRejectsInvalidValue(t *testing.T) {
	t.Setenv("NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS", "sometimes")
	if _, err := apiGatewayRegisterLegacyDescriptors(); err == nil {
		t.Fatalf("expected invalid legacy descriptor registration config to fail")
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
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_JSON", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_FILE", "")
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
