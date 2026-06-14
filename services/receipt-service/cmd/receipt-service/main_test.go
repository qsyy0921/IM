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

func TestLoadReceiptGRPCCredentialsFromEnvDisabledByDefault(t *testing.T) {
	clearReceiptGRPCTLSConfig(t)
	creds, ok, err := loadReceiptGRPCCredentialsFromEnv()
	if err != nil {
		t.Fatalf("load receipt grpc credentials: %v", err)
	}
	if ok || creds != nil {
		t.Fatalf("expected receipt grpc tls to be disabled by default, ok=%t creds=%T", ok, creds)
	}
}

func TestNewGRPCServerAcceptsMetadataAuthMode(t *testing.T) {
	clearReceiptGRPCTLSConfig(t)
	t.Setenv("NEXUSIM_RECEIPT_AUTH_MODE", "metadata")

	server, err := newGRPCServer()
	if err != nil {
		t.Fatalf("new grpc server: %v", err)
	}
	server.Stop()
}

func TestNewGRPCServerRejectsUnsupportedAuthMode(t *testing.T) {
	clearReceiptGRPCTLSConfig(t)
	t.Setenv("NEXUSIM_RECEIPT_AUTH_MODE", "unknown")

	server, err := newGRPCServer()
	if err == nil {
		if server != nil {
			server.Stop()
		}
		t.Fatalf("expected unsupported receipt auth mode to fail")
	}
}

func TestReceiptTraceConfigDefaultsToDisabled(t *testing.T) {
	clearReceiptTraceConfig(t)
	config, err := receiptTraceConfigFromEnv()
	if err != nil {
		t.Fatalf("load receipt trace config: %v", err)
	}
	if config.Enabled ||
		config.ServiceName != "receipt-service" ||
		config.Exporter != "stdout" ||
		config.SamplingRatio != 1 {
		t.Fatalf("unexpected default trace config: %+v", config)
	}
}

func TestReceiptTraceConfigLoadsOTLPGRPC(t *testing.T) {
	clearReceiptTraceConfig(t)
	t.Setenv("NEXUSIM_RECEIPT_OTEL_TRACES_ENABLED", "true")
	t.Setenv("NEXUSIM_RECEIPT_OTEL_SERVICE_NAME", "receipt-service-test")
	t.Setenv("NEXUSIM_RECEIPT_OTEL_TRACES_EXPORTER", "otlp-grpc")
	t.Setenv("NEXUSIM_RECEIPT_OTEL_TRACES_OTLP_ENDPOINT", "127.0.0.1:4317")
	t.Setenv("NEXUSIM_RECEIPT_OTEL_TRACES_OTLP_INSECURE", "true")
	t.Setenv("NEXUSIM_RECEIPT_OTEL_TRACES_SAMPLING_RATIO", "0.5")

	config, err := receiptTraceConfigFromEnv()
	if err != nil {
		t.Fatalf("load receipt trace config: %v", err)
	}
	if !config.Enabled ||
		config.ServiceName != "receipt-service-test" ||
		config.Exporter != "otlp-grpc" ||
		config.OTLPEndpoint != "127.0.0.1:4317" ||
		!config.OTLPInsecure ||
		config.SamplingRatio != 0.5 {
		t.Fatalf("unexpected otlp trace config: %+v", config)
	}
}

func TestReceiptTraceConfigRejectsInvalidValues(t *testing.T) {
	clearReceiptTraceConfig(t)
	t.Setenv("NEXUSIM_RECEIPT_OTEL_TRACES_ENABLED", "sometimes")
	if _, err := receiptTraceConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid enabled bool to fail")
	}

	clearReceiptTraceConfig(t)
	t.Setenv("NEXUSIM_RECEIPT_OTEL_TRACES_SAMPLING_RATIO", "2")
	if _, err := receiptTraceConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid sampling ratio to fail")
	}

	clearReceiptTraceConfig(t)
	t.Setenv("NEXUSIM_RECEIPT_OTEL_TRACES_OTLP_INSECURE", "sometimes")
	if _, err := receiptTraceConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid otlp insecure bool to fail")
	}
}

func TestValidateTrustedMetadataListenerConfigAllowsPrivateAddressWithoutMTLS(t *testing.T) {
	err := validateTrustedMetadataListenerConfig(
		"172.31.50.10:10499",
		"metadata",
		nil,
	)
	if err != nil {
		t.Fatalf("expected private address to be allowed without mTLS, got %v", err)
	}
}

func TestValidateTrustedMetadataListenerConfigRequiresMTLSForPublicAddress(t *testing.T) {
	err := validateTrustedMetadataListenerConfig(
		"8.8.8.8:10499",
		"verified-metadata",
		nil,
	)
	if err == nil {
		t.Fatalf("expected public address without mTLS client cert to fail")
	}
}

func TestValidateTrustedMetadataListenerConfigAllowsMTLSForPublicAddress(t *testing.T) {
	err := validateTrustedMetadataListenerConfig(
		"8.8.8.8:10499",
		"verified-metadata",
		&tls.Config{ClientAuth: tls.RequireAndVerifyClientCert},
	)
	if err != nil {
		t.Fatalf("expected public address with mTLS client cert to be allowed, got %v", err)
	}
}

func TestValidateTrustedMetadataListenerConfigIgnoresBodyAuth(t *testing.T) {
	err := validateTrustedMetadataListenerConfig(
		"8.8.8.8:10499",
		"body",
		nil,
	)
	if err != nil {
		t.Fatalf("expected body auth to skip trusted metadata guard, got %v", err)
	}
}

func TestLoadReceiptGRPCCredentialsFromEnvRequiresCertKeyPair(t *testing.T) {
	clearReceiptGRPCTLSConfig(t)
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_CERT_FILE", "server.crt")
	if _, ok, err := loadReceiptGRPCCredentialsFromEnv(); err == nil || !ok {
		t.Fatalf("expected partial receipt grpc tls config to fail, ok=%t err=%v", ok, err)
	}
}

func TestLoadReceiptGRPCCredentialsFromEnvLoadsServerTLS(t *testing.T) {
	clearReceiptGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeReceiptTLSTestCert(t, dir, "server")
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_KEY_FILE", keyFile)

	tlsConfig, ok, err := receiptGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load receipt grpc tls config: %v", err)
	}
	if !ok || tlsConfig == nil {
		t.Fatalf("expected receipt grpc tls config, ok=%t", ok)
	}
	if tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected TLS 1.2 minimum, got %d", tlsConfig.MinVersion)
	}

	creds, ok, err := loadReceiptGRPCCredentialsFromEnv()
	if err != nil {
		t.Fatalf("load receipt grpc tls credentials: %v", err)
	}
	if !ok || creds == nil {
		t.Fatalf("expected receipt grpc tls credentials, ok=%t creds=%T", ok, creds)
	}
}

func TestLoadReceiptGRPCCredentialsFromEnvRejectsInvalidRequireClientCert(t *testing.T) {
	clearReceiptGRPCTLSConfig(t)
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_REQUIRE_CLIENT_CERT", "sometimes")
	if _, ok, err := loadReceiptGRPCCredentialsFromEnv(); err == nil || !ok {
		t.Fatalf("expected invalid receipt client-cert bool to fail, ok=%t err=%v", ok, err)
	}
}

func TestLoadReceiptGRPCCredentialsFromEnvRequiresClientCAForMTLS(t *testing.T) {
	clearReceiptGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeReceiptTLSTestCert(t, dir, "server")
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_REQUIRE_CLIENT_CERT", "true")
	if _, ok, err := loadReceiptGRPCCredentialsFromEnv(); err == nil || !ok {
		t.Fatalf("expected receipt mtls without ca to fail, ok=%t err=%v", ok, err)
	}
}

func TestReceiptGRPCTLSConfigLoadsMTLS(t *testing.T) {
	clearReceiptGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeReceiptTLSTestCert(t, dir, "server")
	caFile, _ := writeReceiptTLSTestCert(t, dir, "ca")
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_CA_FILE", caFile)

	tlsConfig, ok, err := receiptGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load receipt grpc mtls config: %v", err)
	}
	if !ok || tlsConfig == nil {
		t.Fatalf("expected receipt grpc mtls config, ok=%t", ok)
	}
	if tlsConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("expected client cert verification, got %v", tlsConfig.ClientAuth)
	}
	if tlsConfig.ClientCAs == nil {
		t.Fatalf("expected client CA pool")
	}
}

func TestReceiptGRPCTLSConfigAllowsClientIdentity(t *testing.T) {
	clearReceiptGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeReceiptTLSTestCert(t, dir, "server")
	caFile, _ := writeReceiptTLSTestCert(t, dir, "ca")
	clientCertFile, _ := writeReceiptTLSTestCertWithSANs(t, dir, "client", []string{"api-gateway.nexusim.local"}, []string{"spiffe://nexusim/api-gateway"})
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_CA_FILE", caFile)
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", " API-GATEWAY.NEXUSIM.LOCAL ")
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_ALLOWED_URIS", "spiffe://nexusim/other-client")

	tlsConfig, ok, err := receiptGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load receipt grpc mtls config: %v", err)
	}
	if !ok || tlsConfig.VerifyConnection == nil {
		t.Fatalf("expected client identity verifier, ok=%t has_verifier=%t", ok, tlsConfig.VerifyConnection != nil)
	}
	if err := tlsConfig.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{readReceiptTLSTestCert(t, clientCertFile)}}); err != nil {
		t.Fatalf("expected client identity to be allowed: %v", err)
	}
}

func TestReceiptGRPCTLSConfigAllowsClientURIIdentity(t *testing.T) {
	clearReceiptGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeReceiptTLSTestCert(t, dir, "server")
	caFile, _ := writeReceiptTLSTestCert(t, dir, "ca")
	clientCertFile, _ := writeReceiptTLSTestCertWithSANs(t, dir, "client", nil, []string{"spiffe://nexusim/api-gateway"})
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_CA_FILE", caFile)
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_ALLOWED_URIS", "spiffe://nexusim/api-gateway")

	tlsConfig, ok, err := receiptGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load receipt grpc mtls config: %v", err)
	}
	if !ok || tlsConfig.VerifyConnection == nil {
		t.Fatalf("expected client identity verifier, ok=%t has_verifier=%t", ok, tlsConfig.VerifyConnection != nil)
	}
	if err := tlsConfig.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{readReceiptTLSTestCert(t, clientCertFile)}}); err != nil {
		t.Fatalf("expected client uri identity to be allowed: %v", err)
	}
}

func TestReceiptGRPCTLSConfigRejectsUnlistedClientIdentity(t *testing.T) {
	clearReceiptGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeReceiptTLSTestCert(t, dir, "server")
	caFile, _ := writeReceiptTLSTestCert(t, dir, "ca")
	clientCertFile, _ := writeReceiptTLSTestCertWithSANs(t, dir, "client", []string{"unknown.nexusim.local"}, nil)
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_CA_FILE", caFile)
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", "api-gateway.nexusim.local")

	tlsConfig, ok, err := receiptGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load receipt grpc mtls config: %v", err)
	}
	if !ok || tlsConfig.VerifyConnection == nil {
		t.Fatalf("expected client identity verifier, ok=%t has_verifier=%t", ok, tlsConfig.VerifyConnection != nil)
	}
	if err := tlsConfig.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{readReceiptTLSTestCert(t, clientCertFile)}}); err == nil {
		t.Fatalf("expected unlisted client identity to be rejected")
	}
}

func clearReceiptGRPCTLSConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_CERT_FILE", "")
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_KEY_FILE", "")
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_CA_FILE", "")
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_REQUIRE_CLIENT_CERT", "")
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", "")
	t.Setenv("NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_ALLOWED_URIS", "")
}

func clearReceiptTraceConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_RECEIPT_OTEL_TRACES_ENABLED", "")
	t.Setenv("NEXUSIM_RECEIPT_OTEL_SERVICE_NAME", "")
	t.Setenv("NEXUSIM_RECEIPT_OTEL_TRACES_EXPORTER", "")
	t.Setenv("NEXUSIM_RECEIPT_OTEL_TRACES_OTLP_ENDPOINT", "")
	t.Setenv("NEXUSIM_RECEIPT_OTEL_TRACES_OTLP_INSECURE", "")
	t.Setenv("NEXUSIM_RECEIPT_OTEL_TRACES_SAMPLING_RATIO", "")
}

func writeReceiptTLSTestCert(t *testing.T, dir string, name string) (string, string) {
	return writeReceiptTLSTestCertWithSANs(t, dir, name, []string{"localhost"}, nil)
}

func writeReceiptTLSTestCertWithSANs(t *testing.T, dir string, name string, dnsNames []string, uriNames []string) (string, string) {
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
			CommonName: "receipt-" + name,
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

func readReceiptTLSTestCert(t *testing.T, certFile string) *x509.Certificate {
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
