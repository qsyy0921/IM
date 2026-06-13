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
	limiter, closeFn, err := newRateLimiterFromEnv(context.Background())
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

	limiter, closeFn, err := newRateLimiterFromEnv(context.Background())
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
	if _, _, err := newRateLimiterFromEnv(context.Background()); err == nil {
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

	limiter, closeFn, err := newRateLimiterFromEnv(context.Background())
	if err != nil {
		t.Fatalf("new redis rate limiter: %v", err)
	}
	defer closeFn()
	snapshot := limiter.Snapshot()
	if !snapshot.Enabled || snapshot.Backend != "redis" || snapshot.Burst != 8 || snapshot.RedisWindowMS != 2000 || snapshot.RedisFailOpen {
		t.Fatalf("unexpected redis limiter snapshot: %+v", snapshot)
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
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_RPS", "")
	t.Setenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_BURST", "")
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
