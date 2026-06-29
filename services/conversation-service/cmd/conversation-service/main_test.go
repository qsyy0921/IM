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

func TestLoadConversationGRPCCredentialsFromEnvDisabledByDefault(t *testing.T) {
	clearConversationGRPCTLSConfig(t)
	creds, ok, err := loadConversationGRPCCredentialsFromEnv()
	if err != nil {
		t.Fatalf("load conversation grpc credentials: %v", err)
	}
	if ok || creds != nil {
		t.Fatalf("expected conversation grpc tls to be disabled by default, ok=%t creds=%T", ok, creds)
	}
}

func TestNewGRPCServerAcceptsMetadataAuthMode(t *testing.T) {
	clearConversationGRPCTLSConfig(t)
	t.Setenv("NEXUSIM_CONVERSATION_AUTH_MODE", "metadata")

	server, err := newGRPCServer()
	if err != nil {
		t.Fatalf("new grpc server: %v", err)
	}
	server.Stop()
}

func TestNewGRPCServerRejectsUnsupportedAuthMode(t *testing.T) {
	clearConversationGRPCTLSConfig(t)
	t.Setenv("NEXUSIM_CONVERSATION_AUTH_MODE", "unknown")

	server, err := newGRPCServer()
	if err == nil {
		if server != nil {
			server.Stop()
		}
		t.Fatalf("expected unsupported conversation auth mode to fail")
	}
}

func TestConversationTraceConfigDefaultsToDisabled(t *testing.T) {
	clearConversationTraceConfig(t)
	config, err := conversationTraceConfigFromEnv()
	if err != nil {
		t.Fatalf("load conversation trace config: %v", err)
	}
	if config.Enabled ||
		config.ServiceName != "conversation-service" ||
		config.Exporter != "stdout" ||
		config.SamplingRatio != 1 {
		t.Fatalf("unexpected default trace config: %+v", config)
	}
}

func TestConversationTraceConfigLoadsOTLPGRPC(t *testing.T) {
	clearConversationTraceConfig(t)
	t.Setenv("NEXUSIM_CONVERSATION_OTEL_TRACES_ENABLED", "true")
	t.Setenv("NEXUSIM_CONVERSATION_OTEL_SERVICE_NAME", "conversation-service-test")
	t.Setenv("NEXUSIM_CONVERSATION_OTEL_TRACES_EXPORTER", "otlp-grpc")
	t.Setenv("NEXUSIM_CONVERSATION_OTEL_TRACES_OTLP_ENDPOINT", "127.0.0.1:4317")
	t.Setenv("NEXUSIM_CONVERSATION_OTEL_TRACES_OTLP_INSECURE", "true")
	t.Setenv("NEXUSIM_CONVERSATION_OTEL_TRACES_SAMPLING_RATIO", "0.5")

	config, err := conversationTraceConfigFromEnv()
	if err != nil {
		t.Fatalf("load conversation trace config: %v", err)
	}
	if !config.Enabled ||
		config.ServiceName != "conversation-service-test" ||
		config.Exporter != "otlp-grpc" ||
		config.OTLPEndpoint != "127.0.0.1:4317" ||
		!config.OTLPInsecure ||
		config.SamplingRatio != 0.5 {
		t.Fatalf("unexpected otlp trace config: %+v", config)
	}
}

func TestConversationTraceConfigRejectsInvalidValues(t *testing.T) {
	clearConversationTraceConfig(t)
	t.Setenv("NEXUSIM_CONVERSATION_OTEL_TRACES_ENABLED", "sometimes")
	if _, err := conversationTraceConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid enabled bool to fail")
	}

	clearConversationTraceConfig(t)
	t.Setenv("NEXUSIM_CONVERSATION_OTEL_TRACES_SAMPLING_RATIO", "2")
	if _, err := conversationTraceConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid sampling ratio to fail")
	}

	clearConversationTraceConfig(t)
	t.Setenv("NEXUSIM_CONVERSATION_OTEL_TRACES_OTLP_INSECURE", "sometimes")
	if _, err := conversationTraceConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid otlp insecure bool to fail")
	}
}

func TestConversationScaleThresholdsFromEnvDefaults(t *testing.T) {
	clearConversationScaleThresholds(t)
	thresholds, err := conversationScaleThresholdsFromEnv()
	if err != nil {
		t.Fatalf("load conversation scale thresholds: %v", err)
	}
	if thresholds.SmallGroupMaxActiveMembers != 500 ||
		thresholds.MediumGroupMaxActiveMembers != 5000 ||
		thresholds.LargeGroupMaxActiveMembers != 50000 {
		t.Fatalf("unexpected default thresholds: %+v", thresholds)
	}
}

func TestConversationScaleThresholdsFromEnvLoadsStrictValues(t *testing.T) {
	clearConversationScaleThresholds(t)
	t.Setenv("NEXUSIM_CONVERSATION_SCALE_SMALL_MAX", "20")
	t.Setenv("NEXUSIM_CONVERSATION_SCALE_MEDIUM_MAX", "40")
	t.Setenv("NEXUSIM_CONVERSATION_SCALE_LARGE_MAX", "60")

	thresholds, err := conversationScaleThresholdsFromEnv()
	if err != nil {
		t.Fatalf("load conversation scale thresholds: %v", err)
	}
	if thresholds.SmallGroupMaxActiveMembers != 20 ||
		thresholds.MediumGroupMaxActiveMembers != 40 ||
		thresholds.LargeGroupMaxActiveMembers != 60 {
		t.Fatalf("unexpected thresholds: %+v", thresholds)
	}
}

func TestConversationScaleThresholdsFromEnvRejectsInvalidValues(t *testing.T) {
	clearConversationScaleThresholds(t)
	t.Setenv("NEXUSIM_CONVERSATION_SCALE_SMALL_MAX", "zero")
	if _, err := conversationScaleThresholdsFromEnv(); err == nil {
		t.Fatalf("expected non-integer threshold to fail")
	}

	clearConversationScaleThresholds(t)
	t.Setenv("NEXUSIM_CONVERSATION_SCALE_SMALL_MAX", "20")
	t.Setenv("NEXUSIM_CONVERSATION_SCALE_MEDIUM_MAX", "10")
	if _, err := conversationScaleThresholdsFromEnv(); err == nil {
		t.Fatalf("expected unordered thresholds to fail")
	}
}

func TestValidateTrustedMetadataListenerConfigAllowsPrivateAddressWithoutMTLS(t *testing.T) {
	err := validateTrustedMetadataListenerConfig(
		"172.31.50.10:10496",
		"metadata",
		nil,
	)
	if err != nil {
		t.Fatalf("expected private address to be allowed without mTLS, got %v", err)
	}
}

func TestValidateTrustedMetadataListenerConfigRequiresMTLSForPublicAddress(t *testing.T) {
	err := validateTrustedMetadataListenerConfig(
		"8.8.8.8:10496",
		"verified-metadata",
		nil,
	)
	if err == nil {
		t.Fatalf("expected public address without mTLS client cert to fail")
	}
}

func TestValidateTrustedMetadataListenerConfigAllowsMTLSForPublicAddress(t *testing.T) {
	err := validateTrustedMetadataListenerConfig(
		"8.8.8.8:10496",
		"verified-metadata",
		&tls.Config{ClientAuth: tls.RequireAndVerifyClientCert},
	)
	if err != nil {
		t.Fatalf("expected public address with mTLS client cert to be allowed, got %v", err)
	}
}

func TestValidateTrustedMetadataListenerConfigIgnoresBodyAuth(t *testing.T) {
	err := validateTrustedMetadataListenerConfig(
		"8.8.8.8:10496",
		"body",
		nil,
	)
	if err != nil {
		t.Fatalf("expected body auth to skip trusted metadata guard, got %v", err)
	}
}

func TestConversationDebugAddrPrefersServiceSpecificEnv(t *testing.T) {
	t.Setenv("NEXUSIM_DEBUG_ADDR", "127.0.0.1:19100")
	t.Setenv("NEXUSIM_CONVERSATION_DEBUG_ADDR", "127.0.0.1:19101")

	if addr := conversationDebugAddr(); addr != "127.0.0.1:19101" {
		t.Fatalf("expected service-specific debug addr to win, got %q", addr)
	}
}

func TestEnvOptionalRFC3339Time(t *testing.T) {
	const name = "NEXUSIM_CONVERSATION_TEST_TIME"

	t.Setenv(name, "")
	parsed, err := envOptionalRFC3339Time(name)
	if err != nil {
		t.Fatalf("empty optional time returned error: %v", err)
	}
	if parsed != nil {
		t.Fatalf("empty optional time = %v, want nil", parsed)
	}

	t.Setenv(name, "2026-06-17T09:20:00+08:00")
	parsed, err = envOptionalRFC3339Time(name)
	if err != nil {
		t.Fatalf("valid optional time returned error: %v", err)
	}
	if got := formatAuditFilterTime(parsed); got != "2026-06-17T01:20:00Z" {
		t.Fatalf("optional time = %q, want UTC RFC3339", got)
	}

	t.Setenv(name, "2026-06-17")
	if _, err := envOptionalRFC3339Time(name); err == nil {
		t.Fatalf("expected date-only optional time to fail")
	}
}

func TestValidateConversationDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	for _, addr := range []string{"", "127.0.0.1:11906", "localhost:11906", "172.31.50.10:11906"} {
		if err := validateConversationDebugListenerConfig(addr, false); err != nil {
			t.Fatalf("expected conversation debug listener %q to be allowed: %v", addr, err)
		}
	}
}

func TestValidateConversationDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:11906", ":11906", "8.8.8.8:11906"} {
		if err := validateConversationDebugListenerConfig(addr, false); err == nil {
			t.Fatalf("expected conversation debug listener %q to be rejected by default", addr)
		}
	}
}

func TestValidateConversationDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateConversationDebugListenerConfig("0.0.0.0:11906", true); err != nil {
		t.Fatalf("expected explicit public conversation debug listener opt-in to be allowed: %v", err)
	}
}

func TestLoadConversationGRPCCredentialsFromEnvRequiresCertKeyPair(t *testing.T) {
	clearConversationGRPCTLSConfig(t)
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_CERT_FILE", "server.crt")
	if _, ok, err := loadConversationGRPCCredentialsFromEnv(); err == nil || !ok {
		t.Fatalf("expected partial conversation grpc tls config to fail, ok=%t err=%v", ok, err)
	}
}

func TestLoadConversationGRPCCredentialsFromEnvLoadsServerTLS(t *testing.T) {
	clearConversationGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeConversationTLSTestCert(t, dir, "server")
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_KEY_FILE", keyFile)

	tlsConfig, ok, err := conversationGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load conversation grpc tls config: %v", err)
	}
	if !ok || tlsConfig == nil {
		t.Fatalf("expected conversation grpc tls config, ok=%t", ok)
	}
	if tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected TLS 1.2 minimum, got %d", tlsConfig.MinVersion)
	}
}

func TestConversationGRPCTLSConfigRejectsInvalidRequireClientCert(t *testing.T) {
	clearConversationGRPCTLSConfig(t)
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_REQUIRE_CLIENT_CERT", "sometimes")
	if _, ok, err := conversationGRPCTLSConfigFromEnv(); err == nil || !ok {
		t.Fatalf("expected invalid require client cert error, ok=%v err=%v", ok, err)
	}
}

func TestConversationGRPCTLSConfigRequiresClientCAForMTLS(t *testing.T) {
	clearConversationGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeConversationTLSTestCert(t, dir, "server")
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_REQUIRE_CLIENT_CERT", "true")

	if _, ok, err := conversationGRPCTLSConfigFromEnv(); err == nil || !ok {
		t.Fatalf("expected missing client CA to fail, ok=%v err=%v", ok, err)
	}
}

func TestConversationGRPCTLSConfigLoadsMTLS(t *testing.T) {
	clearConversationGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeConversationTLSTestCert(t, dir, "server")
	caFile, _ := writeConversationTLSTestCert(t, dir, "ca")
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_CA_FILE", caFile)

	tlsConfig, ok, err := conversationGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load conversation grpc mtls config: %v", err)
	}
	if !ok || tlsConfig == nil {
		t.Fatalf("expected conversation grpc mtls config, ok=%t", ok)
	}
	if tlsConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("expected client cert verification, got %v", tlsConfig.ClientAuth)
	}
	if tlsConfig.ClientCAs == nil {
		t.Fatalf("expected client CA pool")
	}
}

func TestConversationGRPCTLSConfigAllowsClientIdentity(t *testing.T) {
	clearConversationGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeConversationTLSTestCert(t, dir, "server")
	caFile, _ := writeConversationTLSTestCert(t, dir, "ca")
	clientCertFile, _ := writeConversationTLSTestCertWithSANs(t, dir, "client", []string{"message-service.nexusim.local"}, []string{"spiffe://nexusim/message-service"})
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_CA_FILE", caFile)
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", " MESSAGE-SERVICE.NEXUSIM.LOCAL ")

	tlsConfig, ok, err := conversationGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load conversation grpc mtls config: %v", err)
	}
	if !ok || tlsConfig.VerifyConnection == nil {
		t.Fatalf("expected client identity verifier, ok=%t has_verifier=%t", ok, tlsConfig.VerifyConnection != nil)
	}
	if err := tlsConfig.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{readConversationTLSTestCert(t, clientCertFile)}}); err != nil {
		t.Fatalf("expected client identity to be allowed: %v", err)
	}
}

func TestConversationGRPCTLSConfigRejectsUnlistedClientIdentity(t *testing.T) {
	clearConversationGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeConversationTLSTestCert(t, dir, "server")
	caFile, _ := writeConversationTLSTestCert(t, dir, "ca")
	clientCertFile, _ := writeConversationTLSTestCertWithSANs(t, dir, "client", []string{"policy-service.nexusim.local"}, nil)
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_CA_FILE", caFile)
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", "message-service.nexusim.local")

	tlsConfig, ok, err := conversationGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load conversation grpc mtls config: %v", err)
	}
	if !ok || tlsConfig.VerifyConnection == nil {
		t.Fatalf("expected client identity verifier, ok=%t has_verifier=%t", ok, tlsConfig.VerifyConnection != nil)
	}
	if err := tlsConfig.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{readConversationTLSTestCert(t, clientCertFile)}}); err == nil {
		t.Fatalf("expected unlisted client identity to be rejected")
	}
}

func clearConversationGRPCTLSConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_CERT_FILE", "")
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_KEY_FILE", "")
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_CA_FILE", "")
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_REQUIRE_CLIENT_CERT", "")
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", "")
	t.Setenv("NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_ALLOWED_URIS", "")
}

func clearConversationTraceConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_CONVERSATION_OTEL_TRACES_ENABLED", "")
	t.Setenv("NEXUSIM_CONVERSATION_OTEL_SERVICE_NAME", "")
	t.Setenv("NEXUSIM_CONVERSATION_OTEL_TRACES_EXPORTER", "")
	t.Setenv("NEXUSIM_CONVERSATION_OTEL_TRACES_OTLP_ENDPOINT", "")
	t.Setenv("NEXUSIM_CONVERSATION_OTEL_TRACES_OTLP_INSECURE", "")
	t.Setenv("NEXUSIM_CONVERSATION_OTEL_TRACES_SAMPLING_RATIO", "")
}

func clearConversationScaleThresholds(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_CONVERSATION_SCALE_SMALL_MAX", "")
	t.Setenv("NEXUSIM_CONVERSATION_SCALE_MEDIUM_MAX", "")
	t.Setenv("NEXUSIM_CONVERSATION_SCALE_LARGE_MAX", "")
}

func writeConversationTLSTestCert(t *testing.T, dir string, name string) (string, string) {
	return writeConversationTLSTestCertWithSANs(t, dir, name, []string{"localhost"}, nil)
}

func writeConversationTLSTestCertWithSANs(t *testing.T, dir string, name string, dnsNames []string, uriNames []string) (string, string) {
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
			CommonName: "conversation-" + name,
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

func readConversationTLSTestCert(t *testing.T, certFile string) *x509.Certificate {
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
