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
	"testing"
	"time"
)

func TestLoadPolicyGRPCCredentialsFromEnvDisabledByDefault(t *testing.T) {
	clearPolicyGRPCTLSConfig(t)
	creds, ok, err := loadPolicyGRPCCredentialsFromEnv()
	if err != nil {
		t.Fatalf("load policy grpc credentials: %v", err)
	}
	if ok || creds != nil {
		t.Fatalf("expected policy grpc tls to be disabled by default, ok=%t creds=%T", ok, creds)
	}
}

func TestPolicyTraceConfigDefaultsToDisabled(t *testing.T) {
	clearPolicyTraceConfig(t)
	config, err := policyTraceConfigFromEnv()
	if err != nil {
		t.Fatalf("load policy trace config: %v", err)
	}
	if config.Enabled ||
		config.ServiceName != "policy-service" ||
		config.Exporter != "stdout" ||
		config.SamplingRatio != 1 {
		t.Fatalf("unexpected default trace config: %+v", config)
	}
}

func TestPolicyTraceConfigLoadsOTLPGRPC(t *testing.T) {
	clearPolicyTraceConfig(t)
	t.Setenv("NEXUSIM_POLICY_OTEL_TRACES_ENABLED", "true")
	t.Setenv("NEXUSIM_POLICY_OTEL_SERVICE_NAME", "policy-service-test")
	t.Setenv("NEXUSIM_POLICY_OTEL_TRACES_EXPORTER", "otlp-grpc")
	t.Setenv("NEXUSIM_POLICY_OTEL_TRACES_OTLP_ENDPOINT", "127.0.0.1:4317")
	t.Setenv("NEXUSIM_POLICY_OTEL_TRACES_OTLP_INSECURE", "true")
	t.Setenv("NEXUSIM_POLICY_OTEL_TRACES_SAMPLING_RATIO", "0.5")

	config, err := policyTraceConfigFromEnv()
	if err != nil {
		t.Fatalf("load policy trace config: %v", err)
	}
	if !config.Enabled ||
		config.ServiceName != "policy-service-test" ||
		config.Exporter != "otlp-grpc" ||
		config.OTLPEndpoint != "127.0.0.1:4317" ||
		!config.OTLPInsecure ||
		config.SamplingRatio != 0.5 {
		t.Fatalf("unexpected otlp trace config: %+v", config)
	}
}

func TestPolicyTraceConfigRejectsInvalidValues(t *testing.T) {
	clearPolicyTraceConfig(t)
	t.Setenv("NEXUSIM_POLICY_OTEL_TRACES_ENABLED", "sometimes")
	if _, err := policyTraceConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid enabled bool to fail")
	}

	clearPolicyTraceConfig(t)
	t.Setenv("NEXUSIM_POLICY_OTEL_TRACES_SAMPLING_RATIO", "2")
	if _, err := policyTraceConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid sampling ratio to fail")
	}

	clearPolicyTraceConfig(t)
	t.Setenv("NEXUSIM_POLICY_OTEL_TRACES_OTLP_INSECURE", "sometimes")
	if _, err := policyTraceConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid otlp insecure bool to fail")
	}
}

func TestPolicyContentModeratorFromEnvDisabledByDefault(t *testing.T) {
	clearPolicyModerationConfig(t)
	moderator, enabled, err := policyContentModeratorFromEnv()
	if err != nil {
		t.Fatalf("load policy moderation config: %v", err)
	}
	if enabled || moderator != nil {
		t.Fatalf("expected moderation disabled by default, enabled=%t moderator=%T", enabled, moderator)
	}
}

func TestPolicyDecisionCacheConfigFromEnv(t *testing.T) {
	clearPolicyDecisionCacheConfig(t)
	config, err := policyDecisionCacheConfigFromEnv()
	if err != nil {
		t.Fatalf("load disabled decision cache config: %v", err)
	}
	if config.Enabled {
		t.Fatalf("expected decision cache disabled by default, got %+v", config)
	}

	clearPolicyDecisionCacheConfig(t)
	t.Setenv("NEXUSIM_POLICY_DECISION_CACHE_BACKEND", "redis")
	t.Setenv("NEXUSIM_POLICY_DECISION_CACHE_REDIS_ADDR", "127.0.0.1:6379")
	t.Setenv("NEXUSIM_POLICY_DECISION_CACHE_TTL", "15s")
	config, err = policyDecisionCacheConfigFromEnv()
	if err != nil {
		t.Fatalf("load redis decision cache config: %v", err)
	}
	if !config.Enabled || config.Addr != "127.0.0.1:6379" || config.TTL != 15*time.Second {
		t.Fatalf("unexpected redis decision cache config: %+v", config)
	}

	clearPolicyDecisionCacheConfig(t)
	t.Setenv("NEXUSIM_POLICY_DECISION_CACHE_BACKEND", "disabled")
	t.Setenv("NEXUSIM_POLICY_DECISION_CACHE_ENABLED", "true")
	config, err = policyDecisionCacheConfigFromEnv()
	if err != nil {
		t.Fatalf("load explicitly disabled decision cache config: %v", err)
	}
	if config.Enabled {
		t.Fatalf("explicit disabled backend should win, got %+v", config)
	}
}

func TestPolicyContentModeratorFromEnvLoadsKeywordMode(t *testing.T) {
	clearPolicyModerationConfig(t)
	t.Setenv("NEXUSIM_POLICY_MODERATION_MODE", "keyword")
	t.Setenv("NEXUSIM_POLICY_MODERATION_DENY_TERMS", "spam, abuse")
	t.Setenv("NEXUSIM_POLICY_MODERATION_PERMISSION_VERSION", "17")
	t.Setenv("NEXUSIM_POLICY_MODERATION_CLASSIFICATION", "CONTENT_REVIEW")
	t.Setenv("NEXUSIM_POLICY_MODERATION_DENY_REASON", "content rejected")

	moderator, enabled, err := policyContentModeratorFromEnv()
	if err != nil {
		t.Fatalf("load policy moderation config: %v", err)
	}
	if !enabled || moderator == nil {
		t.Fatalf("expected moderation enabled, enabled=%t moderator=%T", enabled, moderator)
	}
}

func TestPolicyContentModeratorFromEnvLoadsHTTPMode(t *testing.T) {
	clearPolicyModerationConfig(t)
	t.Setenv("NEXUSIM_POLICY_MODERATION_MODE", "http")
	t.Setenv("NEXUSIM_POLICY_MODERATION_HTTP_ENDPOINT", "https://moderation.example.test/check")
	t.Setenv("NEXUSIM_POLICY_MODERATION_HTTP_BEARER_TOKEN", "token")
	t.Setenv("NEXUSIM_POLICY_MODERATION_HTTP_TIMEOUT", "2s")
	t.Setenv("NEXUSIM_POLICY_MODERATION_PERMISSION_VERSION", "29")
	t.Setenv("NEXUSIM_POLICY_MODERATION_CLASSIFICATION", "PROVIDER_DENIED")
	t.Setenv("NEXUSIM_POLICY_MODERATION_DENY_REASON", "provider denied")

	moderator, enabled, err := policyContentModeratorFromEnv()
	if err != nil {
		t.Fatalf("load policy http moderation config: %v", err)
	}
	if !enabled || moderator == nil {
		t.Fatalf("expected http moderation enabled, enabled=%t moderator=%T", enabled, moderator)
	}
}

func TestPolicyContentModeratorFromEnvRejectsInvalidConfig(t *testing.T) {
	clearPolicyModerationConfig(t)
	t.Setenv("NEXUSIM_POLICY_MODERATION_MODE", "keyword")
	if _, enabled, err := policyContentModeratorFromEnv(); err == nil || !enabled {
		t.Fatalf("expected keyword mode without terms to fail, enabled=%t err=%v", enabled, err)
	}

	clearPolicyModerationConfig(t)
	t.Setenv("NEXUSIM_POLICY_MODERATION_MODE", "provider")
	if _, enabled, err := policyContentModeratorFromEnv(); err == nil || !enabled {
		t.Fatalf("expected unsupported moderation mode to fail, enabled=%t err=%v", enabled, err)
	}

	clearPolicyModerationConfig(t)
	t.Setenv("NEXUSIM_POLICY_MODERATION_MODE", "http")
	if _, enabled, err := policyContentModeratorFromEnv(); err == nil || !enabled {
		t.Fatalf("expected http mode without endpoint to fail, enabled=%t err=%v", enabled, err)
	}

	clearPolicyModerationConfig(t)
	t.Setenv("NEXUSIM_POLICY_MODERATION_MODE", "http")
	t.Setenv("NEXUSIM_POLICY_MODERATION_HTTP_ENDPOINT", "http://moderation.example.test/check")
	if _, enabled, err := policyContentModeratorFromEnv(); err == nil || !enabled {
		t.Fatalf("expected insecure http endpoint to fail without override, enabled=%t err=%v", enabled, err)
	}

	clearPolicyModerationConfig(t)
	t.Setenv("NEXUSIM_POLICY_MODERATION_MODE", "http")
	t.Setenv("NEXUSIM_POLICY_MODERATION_HTTP_ENDPOINT", "http://moderation.example.test/check")
	t.Setenv("NEXUSIM_POLICY_MODERATION_HTTP_ALLOW_INSECURE", "true")
	if _, enabled, err := policyContentModeratorFromEnv(); err != nil || !enabled {
		t.Fatalf("expected insecure http endpoint to load with override, enabled=%t err=%v", enabled, err)
	}
}

func TestStaticMessagePolicyFromEnvLeavesPermissionVersionUnsetByDefault(t *testing.T) {
	t.Setenv("NEXUSIM_POLICY_MESSAGE_ALLOWED", "")
	t.Setenv("NEXUSIM_POLICY_PERMISSION_VERSION", "")
	t.Setenv("NEXUSIM_POLICY_CLASSIFICATION", "")
	t.Setenv("NEXUSIM_POLICY_DENY_REASON", "")

	policy := staticMessagePolicyFromEnv()
	if !policy.Allowed ||
		policy.PermissionVersion != 0 ||
		policy.Classification != "INTERNAL" ||
		policy.Reason != "" {
		t.Fatalf("unexpected default static policy: %+v", policy)
	}
}

func TestStaticMessagePolicyFromEnvUsesConfiguredPermissionVersion(t *testing.T) {
	t.Setenv("NEXUSIM_POLICY_MESSAGE_ALLOWED", "false")
	t.Setenv("NEXUSIM_POLICY_PERMISSION_VERSION", "17")
	t.Setenv("NEXUSIM_POLICY_CLASSIFICATION", "TENANT_RULE")
	t.Setenv("NEXUSIM_POLICY_DENY_REASON", "blocked")

	policy := staticMessagePolicyFromEnv()
	if policy.Allowed ||
		policy.PermissionVersion != 17 ||
		policy.Classification != "TENANT_RULE" ||
		policy.Reason != "blocked" {
		t.Fatalf("unexpected configured static policy: %+v", policy)
	}
}

func TestLoadPolicyGRPCCredentialsFromEnvRequiresCertKeyPair(t *testing.T) {
	clearPolicyGRPCTLSConfig(t)
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_CERT_FILE", "server.crt")
	if _, ok, err := loadPolicyGRPCCredentialsFromEnv(); err == nil || !ok {
		t.Fatalf("expected partial policy grpc tls config to fail, ok=%t err=%v", ok, err)
	}
}

func TestLoadPolicyGRPCCredentialsFromEnvLoadsServerTLS(t *testing.T) {
	clearPolicyGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writePolicyTLSTestCert(t, dir, "server")
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_KEY_FILE", keyFile)

	tlsConfig, ok, err := policyGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load policy grpc tls config: %v", err)
	}
	if !ok || tlsConfig == nil {
		t.Fatalf("expected policy grpc tls config, ok=%t", ok)
	}
	if tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected TLS 1.2 minimum, got %d", tlsConfig.MinVersion)
	}

	creds, ok, err := loadPolicyGRPCCredentialsFromEnv()
	if err != nil {
		t.Fatalf("load policy grpc tls credentials: %v", err)
	}
	if !ok || creds == nil {
		t.Fatalf("expected policy grpc tls credentials, ok=%t creds=%T", ok, creds)
	}
}

func TestLoadPolicyGRPCCredentialsFromEnvRejectsInvalidRequireClientCert(t *testing.T) {
	clearPolicyGRPCTLSConfig(t)
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_REQUIRE_CLIENT_CERT", "sometimes")
	if _, ok, err := loadPolicyGRPCCredentialsFromEnv(); err == nil || !ok {
		t.Fatalf("expected invalid policy client-cert bool to fail, ok=%t err=%v", ok, err)
	}
}

func TestLoadPolicyGRPCCredentialsFromEnvRequiresClientCAForMTLS(t *testing.T) {
	clearPolicyGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writePolicyTLSTestCert(t, dir, "server")
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_REQUIRE_CLIENT_CERT", "true")
	if _, ok, err := loadPolicyGRPCCredentialsFromEnv(); err == nil || !ok {
		t.Fatalf("expected policy mtls without ca to fail, ok=%t err=%v", ok, err)
	}
}

func TestPolicyGRPCTLSConfigLoadsMTLS(t *testing.T) {
	clearPolicyGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writePolicyTLSTestCert(t, dir, "server")
	caFile, _ := writePolicyTLSTestCert(t, dir, "ca")
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_CLIENT_CA_FILE", caFile)

	tlsConfig, ok, err := policyGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load policy grpc mtls config: %v", err)
	}
	if !ok || tlsConfig == nil {
		t.Fatalf("expected policy grpc mtls config, ok=%t", ok)
	}
	if tlsConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("expected client cert verification, got %v", tlsConfig.ClientAuth)
	}
	if tlsConfig.ClientCAs == nil {
		t.Fatalf("expected client CA pool")
	}
}

func TestPolicyGRPCTLSConfigAllowsClientIdentity(t *testing.T) {
	clearPolicyGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writePolicyTLSTestCert(t, dir, "server")
	caFile, _ := writePolicyTLSTestCert(t, dir, "ca")
	clientCertFile, _ := writePolicyTLSTestCertWithSANs(t, dir, "client", []string{"message-service.nexusim.local"}, []string{"spiffe://nexusim/message-service"})
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_CLIENT_CA_FILE", caFile)
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", " MESSAGE-SERVICE.NEXUSIM.LOCAL ")
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_CLIENT_ALLOWED_URIS", "spiffe://nexusim/other-client")

	tlsConfig, ok, err := policyGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load policy grpc mtls config: %v", err)
	}
	if !ok || tlsConfig.VerifyConnection == nil {
		t.Fatalf("expected client identity verifier, ok=%t has_verifier=%t", ok, tlsConfig.VerifyConnection != nil)
	}
	if err := tlsConfig.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{readPolicyTLSTestCert(t, clientCertFile)}}); err != nil {
		t.Fatalf("expected client identity to be allowed: %v", err)
	}
}

func TestPolicyGRPCTLSConfigRejectsUnlistedClientIdentity(t *testing.T) {
	clearPolicyGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writePolicyTLSTestCert(t, dir, "server")
	caFile, _ := writePolicyTLSTestCert(t, dir, "ca")
	clientCertFile, _ := writePolicyTLSTestCertWithSANs(t, dir, "client", []string{"contacts-service.nexusim.local"}, nil)
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_CLIENT_CA_FILE", caFile)
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", "message-service.nexusim.local")

	tlsConfig, ok, err := policyGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load policy grpc mtls config: %v", err)
	}
	if !ok || tlsConfig.VerifyConnection == nil {
		t.Fatalf("expected client identity verifier, ok=%t has_verifier=%t", ok, tlsConfig.VerifyConnection != nil)
	}
	if err := tlsConfig.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{readPolicyTLSTestCert(t, clientCertFile)}}); err == nil {
		t.Fatalf("expected unlisted client identity to be rejected")
	}
}

func TestValidatePolicyListenerConfigAllowsPrivateAddressWithoutTLS(t *testing.T) {
	if err := validatePolicyListenerConfig("172.31.50.10:10800", false); err != nil {
		t.Fatalf("expected private policy listener without tls to be allowed: %v", err)
	}
}

func TestValidatePolicyListenerConfigRejectsPublicAddressWithoutTLS(t *testing.T) {
	if err := validatePolicyListenerConfig("8.8.8.8:10800", false); err == nil {
		t.Fatalf("expected public policy listener without tls to be rejected")
	}
}

func TestValidatePolicyListenerConfigAllowsPublicAddressWithTLS(t *testing.T) {
	if err := validatePolicyListenerConfig("8.8.8.8:10800", true); err != nil {
		t.Fatalf("expected public policy listener with tls to be allowed: %v", err)
	}
}

func TestPolicyDebugAddrPrefersServiceSpecificEnv(t *testing.T) {
	t.Setenv("NEXUSIM_DEBUG_ADDR", "127.0.0.1:19200")
	t.Setenv("NEXUSIM_POLICY_DEBUG_ADDR", "127.0.0.1:19203")

	if addr := policyDebugAddr(); addr != "127.0.0.1:19203" {
		t.Fatalf("expected service-specific debug addr to win, got %q", addr)
	}
}

func TestEnvOptionalRFC3339Time(t *testing.T) {
	t.Setenv("NEXUSIM_POLICY_TEST_TIME", "")
	parsed, err := envOptionalRFC3339Time("NEXUSIM_POLICY_TEST_TIME")
	if err != nil || parsed != nil {
		t.Fatalf("expected empty optional time to be nil, parsed=%v err=%v", parsed, err)
	}

	t.Setenv("NEXUSIM_POLICY_TEST_TIME", "2026-06-17T09:20:00+08:00")
	parsed, err = envOptionalRFC3339Time("NEXUSIM_POLICY_TEST_TIME")
	if err != nil || parsed == nil || parsed.Format(time.RFC3339) != "2026-06-17T01:20:00Z" {
		t.Fatalf("expected parsed UTC RFC3339 time, parsed=%v err=%v", parsed, err)
	}

	t.Setenv("NEXUSIM_POLICY_TEST_TIME", "2026-06-17")
	if _, err := envOptionalRFC3339Time("NEXUSIM_POLICY_TEST_TIME"); err == nil {
		t.Fatalf("expected invalid optional time to fail")
	}
}

func TestValidatePolicyDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	for _, addr := range []string{"", "127.0.0.1:11911", "localhost:11911", "172.31.50.10:11911"} {
		if err := validatePolicyDebugListenerConfig(addr, false); err != nil {
			t.Fatalf("expected policy debug listener %q to be allowed: %v", addr, err)
		}
	}
}

func TestValidatePolicyDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:11911", ":11911", "8.8.8.8:11911"} {
		if err := validatePolicyDebugListenerConfig(addr, false); err == nil {
			t.Fatalf("expected policy debug listener %q to be rejected by default", addr)
		}
	}
}

func TestValidatePolicyDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validatePolicyDebugListenerConfig("0.0.0.0:11911", true); err != nil {
		t.Fatalf("expected explicit public policy debug listener opt-in to be allowed: %v", err)
	}
}

func clearPolicyGRPCTLSConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_CERT_FILE", "")
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_KEY_FILE", "")
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_CLIENT_CA_FILE", "")
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_REQUIRE_CLIENT_CERT", "")
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", "")
	t.Setenv("NEXUSIM_POLICY_GRPC_TLS_CLIENT_ALLOWED_URIS", "")
}

func clearPolicyTraceConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_POLICY_OTEL_TRACES_ENABLED", "")
	t.Setenv("NEXUSIM_POLICY_OTEL_SERVICE_NAME", "")
	t.Setenv("NEXUSIM_POLICY_OTEL_TRACES_EXPORTER", "")
	t.Setenv("NEXUSIM_POLICY_OTEL_TRACES_OTLP_ENDPOINT", "")
	t.Setenv("NEXUSIM_POLICY_OTEL_TRACES_OTLP_INSECURE", "")
	t.Setenv("NEXUSIM_POLICY_OTEL_TRACES_SAMPLING_RATIO", "")
}

func clearPolicyModerationConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_POLICY_MODERATION_MODE", "")
	t.Setenv("NEXUSIM_POLICY_MODERATION_DENY_TERMS", "")
	t.Setenv("NEXUSIM_POLICY_MODERATION_HTTP_ENDPOINT", "")
	t.Setenv("NEXUSIM_POLICY_MODERATION_HTTP_BEARER_TOKEN", "")
	t.Setenv("NEXUSIM_POLICY_MODERATION_HTTP_TIMEOUT", "")
	t.Setenv("NEXUSIM_POLICY_MODERATION_HTTP_ALLOW_INSECURE", "")
	t.Setenv("NEXUSIM_POLICY_MODERATION_PERMISSION_VERSION", "")
	t.Setenv("NEXUSIM_POLICY_MODERATION_CLASSIFICATION", "")
	t.Setenv("NEXUSIM_POLICY_MODERATION_DENY_REASON", "")
}

func clearPolicyDecisionCacheConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_POLICY_DECISION_CACHE_BACKEND", "")
	t.Setenv("NEXUSIM_POLICY_DECISION_CACHE_ENABLED", "")
	t.Setenv("NEXUSIM_POLICY_DECISION_CACHE_REDIS_MODE", "")
	t.Setenv("NEXUSIM_POLICY_DECISION_CACHE_REDIS_ADDR", "")
	t.Setenv("NEXUSIM_POLICY_DECISION_CACHE_REDIS_USERNAME", "")
	t.Setenv("NEXUSIM_POLICY_DECISION_CACHE_REDIS_PASSWORD", "")
	t.Setenv("NEXUSIM_POLICY_DECISION_CACHE_REDIS_DB", "")
	t.Setenv("NEXUSIM_POLICY_DECISION_CACHE_KEY_PREFIX", "")
	t.Setenv("NEXUSIM_POLICY_DECISION_CACHE_TTL", "")
}

func writePolicyTLSTestCert(t *testing.T, dir string, name string) (string, string) {
	return writePolicyTLSTestCertWithSANs(t, dir, name, []string{"localhost"}, nil)
}

func writePolicyTLSTestCertWithSANs(t *testing.T, dir string, name string, dnsNames []string, uriNames []string) (string, string) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate tls key: %v", err)
	}
	serialNumber, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		t.Fatalf("generate tls serial: %v", err)
	}
	uris := make([]*url.URL, 0, len(uriNames))
	for _, uriName := range uriNames {
		parsed, err := url.Parse(uriName)
		if err != nil {
			t.Fatalf("parse tls uri san: %v", err)
		}
		uris = append(uris, parsed)
	}
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "policy-" + name,
		},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              dnsNames,
		URIs:                  uris,
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

func readPolicyTLSTestCert(t *testing.T, certFile string) *x509.Certificate {
	t.Helper()
	raw, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatalf("read tls cert: %v", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		t.Fatalf("decode tls cert")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse tls cert: %v", err)
	}
	return cert
}
