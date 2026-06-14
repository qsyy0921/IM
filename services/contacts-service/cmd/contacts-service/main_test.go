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
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	contactsv1 "github.com/qsyy0921/IM/api/proto/nexusim/contacts/v1"
	contactsgrpc "github.com/qsyy0921/IM/services/contacts-service/internal/api/grpc"
	monitoringinfra "github.com/qsyy0921/IM/services/contacts-service/internal/infrastructure/monitoring"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestNewGRPCServerRecordsAuthFailures(t *testing.T) {
	t.Setenv("NEXUSIM_CONTACTS_AUTH_MODE", "metadata")
	metrics := monitoringinfra.NewGRPCMetrics()
	server, err := newGRPCServer(metrics)
	if err != nil {
		t.Fatalf("new grpc server: %v", err)
	}
	contactsgrpc.Register(server, contactsgrpc.NewServer(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	listener := bufconn.Listen(1024 * 1024)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	conn, err := grpc.DialContext(
		context.Background(),
		"bufnet",
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithInsecure(),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	client := contactsv1.NewContactsServiceClient(conn)
	_, err = client.ListContacts(context.Background(), &contactsv1.ListContactsRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected unauthenticated, got %v", err)
	}

	snapshot := metrics.Snapshot()
	if snapshot.TotalRequests != 1 || snapshot.TotalErrors != 1 || len(snapshot.Methods) != 1 {
		t.Fatalf("expected recorded auth failure, got %+v", snapshot)
	}
	if got := snapshot.Methods[0].Codes[codes.Unauthenticated.String()]; got != 1 {
		t.Fatalf("expected unauthenticated code count, got %+v", snapshot.Methods[0].Codes)
	}
}

func TestLoadContactsGRPCCredentialsFromEnvDisabledByDefault(t *testing.T) {
	clearContactsGRPCTLSConfig(t)
	creds, ok, err := loadContactsGRPCCredentialsFromEnv()
	if err != nil {
		t.Fatalf("load contacts grpc credentials: %v", err)
	}
	if ok || creds != nil {
		t.Fatalf("expected contacts grpc tls to be disabled by default, ok=%t creds=%T", ok, creds)
	}
}

func TestValidateTrustedMetadataListenerConfigAllowsPrivateAddressWithoutMTLS(t *testing.T) {
	err := validateTrustedMetadataListenerConfig(
		"172.31.50.10:10500",
		"metadata",
		nil,
	)
	if err != nil {
		t.Fatalf("expected private address to be allowed without mTLS, got %v", err)
	}
}

func TestValidateTrustedMetadataListenerConfigRequiresMTLSForPublicAddress(t *testing.T) {
	err := validateTrustedMetadataListenerConfig(
		"8.8.8.8:10500",
		"verified-metadata",
		nil,
	)
	if err == nil {
		t.Fatalf("expected public address without mTLS client cert to fail")
	}
}

func TestValidateTrustedMetadataListenerConfigAllowsMTLSForPublicAddress(t *testing.T) {
	err := validateTrustedMetadataListenerConfig(
		"8.8.8.8:10500",
		"verified-metadata",
		&tls.Config{ClientAuth: tls.RequireAndVerifyClientCert},
	)
	if err != nil {
		t.Fatalf("expected public address with mTLS client cert to be allowed, got %v", err)
	}
}

func TestValidateTrustedMetadataListenerConfigIgnoresBodyAuth(t *testing.T) {
	err := validateTrustedMetadataListenerConfig(
		"8.8.8.8:10500",
		"body",
		nil,
	)
	if err != nil {
		t.Fatalf("expected body auth to skip trusted metadata guard, got %v", err)
	}
}

func TestLoadContactsGRPCCredentialsFromEnvRequiresCertKeyPair(t *testing.T) {
	clearContactsGRPCTLSConfig(t)
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_CERT_FILE", "server.crt")
	if _, ok, err := loadContactsGRPCCredentialsFromEnv(); err == nil || !ok {
		t.Fatalf("expected partial contacts grpc tls config to fail, ok=%t err=%v", ok, err)
	}
}

func TestLoadContactsGRPCCredentialsFromEnvLoadsServerTLS(t *testing.T) {
	clearContactsGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeContactsTLSTestCert(t, dir, "server")
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_KEY_FILE", keyFile)

	tlsConfig, ok, err := contactsGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load contacts grpc tls config: %v", err)
	}
	if !ok || tlsConfig == nil {
		t.Fatalf("expected contacts grpc tls config, ok=%t", ok)
	}
	if tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected TLS 1.2 minimum, got %d", tlsConfig.MinVersion)
	}

	creds, ok, err := loadContactsGRPCCredentialsFromEnv()
	if err != nil {
		t.Fatalf("load contacts grpc tls credentials: %v", err)
	}
	if !ok || creds == nil {
		t.Fatalf("expected contacts grpc tls credentials, ok=%t creds=%T", ok, creds)
	}
}

func TestLoadContactsGRPCCredentialsFromEnvRejectsInvalidRequireClientCert(t *testing.T) {
	clearContactsGRPCTLSConfig(t)
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_REQUIRE_CLIENT_CERT", "sometimes")
	if _, ok, err := loadContactsGRPCCredentialsFromEnv(); err == nil || !ok {
		t.Fatalf("expected invalid contacts client-cert bool to fail, ok=%t err=%v", ok, err)
	}
}

func TestLoadContactsGRPCCredentialsFromEnvRequiresClientCAForMTLS(t *testing.T) {
	clearContactsGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeContactsTLSTestCert(t, dir, "server")
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_REQUIRE_CLIENT_CERT", "true")
	if _, ok, err := loadContactsGRPCCredentialsFromEnv(); err == nil || !ok {
		t.Fatalf("expected contacts mtls without ca to fail, ok=%t err=%v", ok, err)
	}
}

func TestContactsGRPCTLSConfigLoadsMTLS(t *testing.T) {
	clearContactsGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeContactsTLSTestCert(t, dir, "server")
	caFile, _ := writeContactsTLSTestCert(t, dir, "ca")
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_CA_FILE", caFile)

	tlsConfig, ok, err := contactsGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load contacts grpc mtls config: %v", err)
	}
	if !ok || tlsConfig == nil {
		t.Fatalf("expected contacts grpc mtls config, ok=%t", ok)
	}
	if tlsConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("expected client cert verification, got %v", tlsConfig.ClientAuth)
	}
	if tlsConfig.ClientCAs == nil {
		t.Fatalf("expected client CA pool")
	}
}

func TestContactsGRPCTLSConfigAllowsClientIdentity(t *testing.T) {
	clearContactsGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeContactsTLSTestCert(t, dir, "server")
	caFile, _ := writeContactsTLSTestCert(t, dir, "ca")
	clientCertFile, _ := writeContactsTLSTestCertWithSANs(t, dir, "client", []string{"api-gateway.nexusim.local"}, []string{"spiffe://nexusim/api-gateway"})
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_CA_FILE", caFile)
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", " API-GATEWAY.NEXUSIM.LOCAL ")
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_ALLOWED_URIS", "spiffe://nexusim/other-client")

	tlsConfig, ok, err := contactsGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load contacts grpc mtls config: %v", err)
	}
	if !ok || tlsConfig.VerifyConnection == nil {
		t.Fatalf("expected client identity verifier, ok=%t has_verifier=%t", ok, tlsConfig.VerifyConnection != nil)
	}
	if err := tlsConfig.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{readContactsTLSTestCert(t, clientCertFile)}}); err != nil {
		t.Fatalf("expected client identity to be allowed: %v", err)
	}
}

func TestContactsGRPCTLSConfigRejectsUnlistedClientIdentity(t *testing.T) {
	clearContactsGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeContactsTLSTestCert(t, dir, "server")
	caFile, _ := writeContactsTLSTestCert(t, dir, "ca")
	clientCertFile, _ := writeContactsTLSTestCertWithSANs(t, dir, "client", []string{"unknown.nexusim.local"}, nil)
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_CA_FILE", caFile)
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", "api-gateway.nexusim.local")

	tlsConfig, ok, err := contactsGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load contacts grpc mtls config: %v", err)
	}
	if !ok || tlsConfig.VerifyConnection == nil {
		t.Fatalf("expected client identity verifier, ok=%t has_verifier=%t", ok, tlsConfig.VerifyConnection != nil)
	}
	if err := tlsConfig.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{readContactsTLSTestCert(t, clientCertFile)}}); err == nil {
		t.Fatalf("expected unlisted client identity to be rejected")
	}
}

func TestContactsTraceConfigDefaultsToDisabled(t *testing.T) {
	clearContactsTraceConfig(t)
	config, err := contactsTraceConfigFromEnv()
	if err != nil {
		t.Fatalf("load contacts trace config: %v", err)
	}
	if config.Enabled || config.ServiceName != "contacts-service" || config.Exporter != "stdout" || config.SamplingRatio != 1 {
		t.Fatalf("unexpected default trace config: %+v", config)
	}
}

func TestContactsTraceConfigLoadsOTLPGRPC(t *testing.T) {
	clearContactsTraceConfig(t)
	t.Setenv("NEXUSIM_CONTACTS_OTEL_TRACES_ENABLED", "true")
	t.Setenv("NEXUSIM_CONTACTS_OTEL_SERVICE_NAME", "contacts-service-test")
	t.Setenv("NEXUSIM_CONTACTS_OTEL_TRACES_EXPORTER", "otlp-grpc")
	t.Setenv("NEXUSIM_CONTACTS_OTEL_TRACES_OTLP_ENDPOINT", "127.0.0.1:4317")
	t.Setenv("NEXUSIM_CONTACTS_OTEL_TRACES_OTLP_INSECURE", "true")
	t.Setenv("NEXUSIM_CONTACTS_OTEL_TRACES_SAMPLING_RATIO", "0.5")

	config, err := contactsTraceConfigFromEnv()
	if err != nil {
		t.Fatalf("load contacts trace config: %v", err)
	}
	if !config.Enabled ||
		config.ServiceName != "contacts-service-test" ||
		config.Exporter != "otlp-grpc" ||
		config.OTLPEndpoint != "127.0.0.1:4317" ||
		!config.OTLPInsecure ||
		config.SamplingRatio != 0.5 {
		t.Fatalf("unexpected trace config: %+v", config)
	}
}

func TestContactsTraceConfigRejectsInvalidValues(t *testing.T) {
	clearContactsTraceConfig(t)
	t.Setenv("NEXUSIM_CONTACTS_OTEL_TRACES_ENABLED", "sometimes")
	if _, err := contactsTraceConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid trace enabled bool to fail")
	}

	clearContactsTraceConfig(t)
	t.Setenv("NEXUSIM_CONTACTS_OTEL_TRACES_SAMPLING_RATIO", "2")
	if _, err := contactsTraceConfigFromEnv(); err == nil {
		t.Fatalf("expected invalid trace sampling ratio to fail")
	}
}

func clearContactsGRPCTLSConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_CERT_FILE", "")
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_KEY_FILE", "")
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_CA_FILE", "")
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_REQUIRE_CLIENT_CERT", "")
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", "")
	t.Setenv("NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_ALLOWED_URIS", "")
}

func clearContactsTraceConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_CONTACTS_OTEL_TRACES_ENABLED", "")
	t.Setenv("NEXUSIM_CONTACTS_OTEL_SERVICE_NAME", "")
	t.Setenv("NEXUSIM_CONTACTS_OTEL_TRACES_EXPORTER", "")
	t.Setenv("NEXUSIM_CONTACTS_OTEL_TRACES_OTLP_ENDPOINT", "")
	t.Setenv("NEXUSIM_CONTACTS_OTEL_TRACES_OTLP_INSECURE", "")
	t.Setenv("NEXUSIM_CONTACTS_OTEL_TRACES_SAMPLING_RATIO", "")
}

func writeContactsTLSTestCert(t *testing.T, dir string, name string) (string, string) {
	return writeContactsTLSTestCertWithSANs(t, dir, name, []string{"localhost"}, nil)
}

func writeContactsTLSTestCertWithSANs(t *testing.T, dir string, name string, dnsNames []string, uriNames []string) (string, string) {
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
			CommonName: "contacts-" + name,
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

func readContactsTLSTestCert(t *testing.T, certFile string) *x509.Certificate {
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
