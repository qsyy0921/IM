package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadRedisClientConfigSingleDefaults(t *testing.T) {
	t.Setenv("NEXUSIM_PUSH_REDIS_MODE", "")
	t.Setenv("NEXUSIM_PUSH_REDIS_ADDR", "")
	t.Setenv("NEXUSIM_PUSH_REDIS_DB", "")

	config := loadRedisClientConfigFromEnv()
	if config.Mode != "single" {
		t.Fatalf("expected default mode single, got %q", config.Mode)
	}
	if config.Addr != "127.0.0.1:6379" {
		t.Fatalf("expected default addr, got %q", config.Addr)
	}
	if config.DB != 0 {
		t.Fatalf("expected default db 0, got %d", config.DB)
	}
}

func TestLoadRedisClientConfigSentinel(t *testing.T) {
	t.Setenv("NEXUSIM_PUSH_REDIS_MODE", "sentinel")
	t.Setenv("NEXUSIM_PUSH_REDIS_SENTINEL_ADDRS", "127.0.0.1:26379, 127.0.0.1:26380")
	t.Setenv("NEXUSIM_PUSH_REDIS_SENTINEL_MASTER_NAME", "mymaster")
	t.Setenv("NEXUSIM_PUSH_REDIS_USERNAME", "redis-user")
	t.Setenv("NEXUSIM_PUSH_REDIS_PASSWORD", "redis-pass")
	t.Setenv("NEXUSIM_PUSH_REDIS_SENTINEL_USERNAME", "sentinel-user")
	t.Setenv("NEXUSIM_PUSH_REDIS_SENTINEL_PASSWORD", "sentinel-pass")
	t.Setenv("NEXUSIM_PUSH_REDIS_DB", "2")

	config := loadRedisClientConfigFromEnv()
	if config.Mode != "sentinel" {
		t.Fatalf("expected sentinel mode, got %q", config.Mode)
	}
	if len(config.SentinelAddrs) != 2 ||
		config.SentinelAddrs[0] != "127.0.0.1:26379" ||
		config.SentinelAddrs[1] != "127.0.0.1:26380" {
		t.Fatalf("unexpected sentinel addrs: %#v", config.SentinelAddrs)
	}
	if config.SentinelMasterName != "mymaster" {
		t.Fatalf("unexpected sentinel master name: %q", config.SentinelMasterName)
	}
	if config.Username != "redis-user" || config.Password != "redis-pass" {
		t.Fatalf("unexpected redis auth: %q/%q", config.Username, config.Password)
	}
	if config.SentinelUsername != "sentinel-user" || config.SentinelPassword != "sentinel-pass" {
		t.Fatalf("unexpected sentinel auth: %q/%q", config.SentinelUsername, config.SentinelPassword)
	}
	if config.DB != 2 {
		t.Fatalf("expected db 2, got %d", config.DB)
	}
}

func TestNewRedisUniversalClientValidatesSentinelConfig(t *testing.T) {
	if _, err := newRedisUniversalClient(redisClientConfig{
		Mode:               "sentinel",
		SentinelMasterName: "",
		SentinelAddrs:      []string{"127.0.0.1:26379"},
	}); err == nil {
		t.Fatalf("expected missing sentinel master name error")
	}

	if _, err := newRedisUniversalClient(redisClientConfig{
		Mode:               "sentinel",
		SentinelMasterName: "mymaster",
	}); err == nil {
		t.Fatalf("expected missing sentinel addrs error")
	}

	client, err := newRedisUniversalClient(redisClientConfig{
		Mode:               "sentinel",
		SentinelMasterName: "mymaster",
		SentinelAddrs:      []string{"127.0.0.1:26379"},
	})
	if err != nil {
		t.Fatalf("expected sentinel client, got error: %v", err)
	}
	_ = client.Close()
}

func TestNewRedisUniversalClientRejectsUnsupportedMode(t *testing.T) {
	if _, err := newRedisUniversalClient(redisClientConfig{Mode: "cluster"}); err == nil {
		t.Fatalf("expected unsupported mode error")
	} else if !strings.Contains(err.Error(), "cluster") {
		t.Fatalf("expected unsupported mode error to include mode value, got %v", err)
	}
}

func TestPushTraceConfigDefaultsToDisabled(t *testing.T) {
	clearPushTraceConfig(t)
	config, err := pushTraceConfigFromEnv()
	if err != nil {
		t.Fatalf("load push trace config: %v", err)
	}
	if config.Enabled ||
		config.ServiceName != "push-gateway" ||
		config.Exporter != "stdout" ||
		config.OTLPEndpoint != "" ||
		config.OTLPInsecure ||
		config.SamplingRatio != 1 {
		t.Fatalf("unexpected default trace config: %+v", config)
	}
}

func TestPushTraceConfigLoadsOTLPGRPC(t *testing.T) {
	clearPushTraceConfig(t)
	t.Setenv("NEXUSIM_PUSH_OTEL_TRACES_ENABLED", "true")
	t.Setenv("NEXUSIM_PUSH_OTEL_SERVICE_NAME", "push-gateway-test")
	t.Setenv("NEXUSIM_PUSH_OTEL_TRACES_EXPORTER", "otlp-grpc")
	t.Setenv("NEXUSIM_PUSH_OTEL_TRACES_OTLP_ENDPOINT", "127.0.0.1:4317")
	t.Setenv("NEXUSIM_PUSH_OTEL_TRACES_OTLP_INSECURE", "true")
	t.Setenv("NEXUSIM_PUSH_OTEL_TRACES_SAMPLING_RATIO", "0.5")

	config, err := pushTraceConfigFromEnv()
	if err != nil {
		t.Fatalf("load push trace config: %v", err)
	}
	if !config.Enabled ||
		config.ServiceName != "push-gateway-test" ||
		config.Exporter != "otlp-grpc" ||
		config.OTLPEndpoint != "127.0.0.1:4317" ||
		!config.OTLPInsecure ||
		config.SamplingRatio != 0.5 {
		t.Fatalf("unexpected otlp trace config: %+v", config)
	}
}

func TestPushTraceConfigRejectsInvalidValues(t *testing.T) {
	clearPushTraceConfig(t)
	t.Setenv("NEXUSIM_PUSH_OTEL_TRACES_ENABLED", "sometimes")
	if _, err := pushTraceConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid enabled value to fail")
	}

	clearPushTraceConfig(t)
	t.Setenv("NEXUSIM_PUSH_OTEL_TRACES_SAMPLING_RATIO", "2")
	if _, err := pushTraceConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid sampling ratio to fail")
	}

	clearPushTraceConfig(t)
	t.Setenv("NEXUSIM_PUSH_OTEL_TRACES_OTLP_INSECURE", "sometimes")
	if _, err := pushTraceConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid otlp insecure value to fail")
	}
}

func TestAuthenticatorJWKStatsHandlesNilAuthenticator(t *testing.T) {
	if stats := authenticatorJWKStats(nil); stats != nil {
		t.Fatalf("expected nil stats for nil authenticator, got %+v", stats)
	}
}

func TestDeliveryClientTLSConfigFromEnvDisabledByDefault(t *testing.T) {
	clearDeliveryClientTLSEnv(t)

	config, err := deliveryClientTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load delivery tls config: %v", err)
	}
	if config.Enabled() {
		t.Fatalf("expected delivery tls to be disabled by default")
	}
}

func TestDeliveryClientTLSConfigFromEnvRequiresCAFile(t *testing.T) {
	clearDeliveryClientTLSEnv(t)
	t.Setenv("NEXUSIM_DELIVERY_SERVICE_TLS_SERVER_NAME", "delivery-service.nexusim.local")

	if _, err := deliveryClientTLSConfigFromEnv(); err == nil {
		t.Fatalf("expected missing CA file error")
	}
}

func TestDeliveryClientTLSConfigFromEnvRequiresClientKeyPair(t *testing.T) {
	clearDeliveryClientTLSEnv(t)
	t.Setenv("NEXUSIM_DELIVERY_SERVICE_TLS_CA_FILE", "ca.pem")
	t.Setenv("NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_CERT_FILE", "client.crt")

	if _, err := deliveryClientTLSConfigFromEnv(); err == nil {
		t.Fatalf("expected partial client key pair error")
	}
}

func TestDeliveryClientTLSConfigFromEnvLoadsCompleteConfig(t *testing.T) {
	clearDeliveryClientTLSEnv(t)
	t.Setenv("NEXUSIM_DELIVERY_SERVICE_TLS_CA_FILE", "ca.pem")
	t.Setenv("NEXUSIM_DELIVERY_SERVICE_TLS_SERVER_NAME", "delivery-service.nexusim.local")
	t.Setenv("NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_CERT_FILE", "client.crt")
	t.Setenv("NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_KEY_FILE", "client.key")

	config, err := deliveryClientTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load delivery tls config: %v", err)
	}
	if config.CAFile != "ca.pem" ||
		config.ServerName != "delivery-service.nexusim.local" ||
		config.ClientCertFile != "client.crt" ||
		config.ClientKeyFile != "client.key" {
		t.Fatalf("unexpected delivery tls config: %#v", config)
	}
}

func TestPushWSTLSConfigFromEnvDisabledByDefault(t *testing.T) {
	clearPushWSTLSEnv(t)

	config, ok, err := pushWSTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load push ws tls config: %v", err)
	}
	if ok || config != nil {
		t.Fatalf("expected push websocket tls to be disabled by default")
	}
}

func TestPushWSTLSConfigFromEnvRequiresCertKeyPair(t *testing.T) {
	clearPushWSTLSEnv(t)
	t.Setenv("NEXUSIM_PUSH_WS_TLS_CERT_FILE", "server.crt")

	if _, ok, err := pushWSTLSConfigFromEnv(); err == nil || !ok {
		t.Fatalf("expected missing websocket TLS key error, ok=%v err=%v", ok, err)
	}
}

func TestPushWSTLSConfigFromEnvRequiresClientCAFile(t *testing.T) {
	clearPushWSTLSEnv(t)
	dir := t.TempDir()
	certFile, keyFile := writePushWSTLSTestCert(t, dir, "server")
	t.Setenv("NEXUSIM_PUSH_WS_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_PUSH_WS_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_PUSH_WS_TLS_CLIENT_ALLOWED_DNS_NAMES", "desktop-client.nexusim.local")

	if _, ok, err := pushWSTLSConfigFromEnv(); err == nil || !ok {
		t.Fatalf("expected missing websocket client CA error, ok=%v err=%v", ok, err)
	}
}

func TestPushWSTLSConfigFromEnvRejectsInvalidRequireClientCert(t *testing.T) {
	clearPushWSTLSEnv(t)
	t.Setenv("NEXUSIM_PUSH_WS_TLS_REQUIRE_CLIENT_CERT", "sometimes")

	if _, ok, err := pushWSTLSConfigFromEnv(); err == nil || !ok {
		t.Fatalf("expected invalid require client cert error, ok=%v err=%v", ok, err)
	}
}

func TestPushWSTLSConfigFromEnvLoadsMTLS(t *testing.T) {
	clearPushWSTLSEnv(t)
	dir := t.TempDir()
	certFile, keyFile := writePushWSTLSTestCert(t, dir, "server")
	caFile, _ := writePushWSTLSTestCert(t, dir, "ca")
	t.Setenv("NEXUSIM_PUSH_WS_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_PUSH_WS_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_PUSH_WS_TLS_CLIENT_CA_FILE", caFile)
	t.Setenv("NEXUSIM_PUSH_WS_TLS_REQUIRE_CLIENT_CERT", "true")

	config, ok, err := pushWSTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load push ws tls config: %v", err)
	}
	if !ok || config == nil {
		t.Fatalf("expected push websocket tls config")
	}
	if config.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected TLS 1.2 minimum, got %x", config.MinVersion)
	}
	if config.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("expected mTLS client auth, got %v", config.ClientAuth)
	}
	if len(config.Certificates) != 1 || config.ClientCAs == nil {
		t.Fatalf("expected server certificate and client CA")
	}
}

func TestVerifyAllowedPushWSClient(t *testing.T) {
	uri, err := url.Parse("spiffe://nexusim/desktop-client")
	if err != nil {
		t.Fatalf("parse uri: %v", err)
	}
	verify := verifyAllowedPushWSClient(
		map[string]struct{}{"desktop-client.nexusim.local": {}},
		map[string]struct{}{"spiffe://nexusim/desktop-client": {}},
	)
	if err := verify(tls.ConnectionState{PeerCertificates: []*x509.Certificate{{
		DNSNames: []string{"DESKTOP-CLIENT.NEXUSIM.LOCAL"},
	}}}); err != nil {
		t.Fatalf("expected DNS identity to be allowed: %v", err)
	}
	if err := verify(tls.ConnectionState{PeerCertificates: []*x509.Certificate{{
		URIs: []*url.URL{uri},
	}}}); err != nil {
		t.Fatalf("expected URI identity to be allowed: %v", err)
	}
	if err := verify(tls.ConnectionState{PeerCertificates: []*x509.Certificate{{
		DNSNames: []string{"other-client.nexusim.local"},
	}}}); err == nil {
		t.Fatalf("expected unlisted client identity to be rejected")
	}
}

func TestSplitCSVTrimsAndDropsEmptyValues(t *testing.T) {
	values := splitCSV(" old-1, ,old-2 , old-1 ")
	if len(values) != 3 || values[0] != "old-1" || values[1] != "old-2" || values[2] != "old-1" {
		t.Fatalf("unexpected values: %#v", values)
	}
	if values := splitCSV(" , , "); len(values) != 0 {
		t.Fatalf("expected empty values, got %#v", values)
	}
}

func TestValidatePushAuthListenerConfigAllowsPrivateAddressForMock(t *testing.T) {
	if err := validatePushAuthListenerConfig("172.31.50.10:10496", "mock", false); err != nil {
		t.Fatalf("expected private address mock auth to be allowed: %v", err)
	}
}

func TestValidatePushAuthListenerConfigRejectsPublicAddressForMock(t *testing.T) {
	err := validatePushAuthListenerConfig("8.8.8.8:10496", "mock", false)
	if err == nil {
		t.Fatalf("expected public address mock auth to be rejected")
	}
	if !strings.Contains(err.Error(), "mock auth") {
		t.Fatalf("expected mock auth error, got %v", err)
	}
}

func TestValidatePushAuthListenerConfigRejectsPublicAddressForSignedAuthWithoutTLS(t *testing.T) {
	err := validatePushAuthListenerConfig("8.8.8.8:10496", "jwt", false)
	if err == nil {
		t.Fatalf("expected signed auth without tls to be rejected on public address")
	}
}

func TestValidatePushAuthListenerConfigAllowsPublicAddressForSignedAuthWithTLS(t *testing.T) {
	if err := validatePushAuthListenerConfig("8.8.8.8:10496", "jwt", true); err != nil {
		t.Fatalf("expected jwt auth with tls to be allowed on public address: %v", err)
	}
}

func clearDeliveryClientTLSEnv(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_DELIVERY_SERVICE_TLS_CA_FILE", "")
	t.Setenv("NEXUSIM_DELIVERY_SERVICE_TLS_SERVER_NAME", "")
	t.Setenv("NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_CERT_FILE", "")
	t.Setenv("NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_KEY_FILE", "")
}

func clearPushTraceConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_PUSH_OTEL_TRACES_ENABLED", "")
	t.Setenv("NEXUSIM_PUSH_OTEL_SERVICE_NAME", "")
	t.Setenv("NEXUSIM_PUSH_OTEL_TRACES_EXPORTER", "")
	t.Setenv("NEXUSIM_PUSH_OTEL_TRACES_OTLP_ENDPOINT", "")
	t.Setenv("NEXUSIM_PUSH_OTEL_TRACES_OTLP_INSECURE", "")
	t.Setenv("NEXUSIM_PUSH_OTEL_TRACES_SAMPLING_RATIO", "")
}

func clearPushWSTLSEnv(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_PUSH_WS_TLS_CERT_FILE", "")
	t.Setenv("NEXUSIM_PUSH_WS_TLS_KEY_FILE", "")
	t.Setenv("NEXUSIM_PUSH_WS_TLS_CLIENT_CA_FILE", "")
	t.Setenv("NEXUSIM_PUSH_WS_TLS_REQUIRE_CLIENT_CERT", "")
	t.Setenv("NEXUSIM_PUSH_WS_TLS_CLIENT_ALLOWED_DNS_NAMES", "")
	t.Setenv("NEXUSIM_PUSH_WS_TLS_CLIENT_ALLOWED_URIS", "")
}

func writePushWSTLSTestCert(t *testing.T, dir string, name string) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate tls key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("generate tls serial: %v", err)
	}
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{name + ".nexusim.local"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create tls cert: %v", err)
	}
	certFile := filepath.Join(dir, name+".crt")
	keyFile := filepath.Join(dir, name+".key")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write tls cert: %v", err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0o600); err != nil {
		t.Fatalf("write tls key: %v", err)
	}
	return certFile, keyFile
}
