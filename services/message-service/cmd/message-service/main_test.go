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

func TestPolicyClientTLSConfigFromEnvDisabledByDefault(t *testing.T) {
	clearPolicyClientTLSConfig(t)
	config, err := policyClientTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load policy client tls config: %v", err)
	}
	if config.Enabled() {
		t.Fatalf("expected policy client tls to be disabled by default: %+v", config)
	}
}

func TestPolicyClientTLSConfigFromEnvRequiresCAFile(t *testing.T) {
	clearPolicyClientTLSConfig(t)
	t.Setenv("NEXUSIM_POLICY_SERVICE_TLS_SERVER_NAME", "policy-service.nexusim.local")
	if _, err := policyClientTLSConfigFromEnv(); err == nil {
		t.Fatalf("expected policy client tls without CA file to fail")
	}
}

func TestPolicyClientTLSConfigFromEnvRequiresClientKeyPair(t *testing.T) {
	clearPolicyClientTLSConfig(t)
	t.Setenv("NEXUSIM_POLICY_SERVICE_TLS_CA_FILE", "ca.pem")
	t.Setenv("NEXUSIM_POLICY_SERVICE_TLS_CLIENT_CERT_FILE", "client.crt")
	if _, err := policyClientTLSConfigFromEnv(); err == nil {
		t.Fatalf("expected partial policy client certificate config to fail")
	}
}

func TestPolicyClientTLSConfigFromEnvLoadsTLS(t *testing.T) {
	clearPolicyClientTLSConfig(t)
	t.Setenv("NEXUSIM_POLICY_SERVICE_TLS_CA_FILE", "ca.pem")
	t.Setenv("NEXUSIM_POLICY_SERVICE_TLS_SERVER_NAME", "policy-service.nexusim.local")
	t.Setenv("NEXUSIM_POLICY_SERVICE_TLS_CLIENT_CERT_FILE", "client.crt")
	t.Setenv("NEXUSIM_POLICY_SERVICE_TLS_CLIENT_KEY_FILE", "client.key")
	config, err := policyClientTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load policy client tls config: %v", err)
	}
	if config.CAFile != "ca.pem" ||
		config.ServerName != "policy-service.nexusim.local" ||
		config.ClientCertFile != "client.crt" ||
		config.ClientKeyFile != "client.key" {
		t.Fatalf("unexpected policy client tls config: %+v", config)
	}
}

func TestConversationClientTLSConfigFromEnvDisabledByDefault(t *testing.T) {
	clearConversationClientTLSConfig(t)
	config, err := conversationClientTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load conversation client tls config: %v", err)
	}
	if config.Enabled() {
		t.Fatalf("expected conversation client tls to be disabled by default: %+v", config)
	}
}

func TestConversationClientTLSConfigFromEnvRequiresCAFile(t *testing.T) {
	clearConversationClientTLSConfig(t)
	t.Setenv("NEXUSIM_CONVERSATION_SERVICE_TLS_SERVER_NAME", "conversation-service.nexusim.local")
	if _, err := conversationClientTLSConfigFromEnv(); err == nil {
		t.Fatalf("expected conversation client tls without CA file to fail")
	}
}

func TestConversationClientTLSConfigFromEnvRequiresClientKeyPair(t *testing.T) {
	clearConversationClientTLSConfig(t)
	t.Setenv("NEXUSIM_CONVERSATION_SERVICE_TLS_CA_FILE", "ca.pem")
	t.Setenv("NEXUSIM_CONVERSATION_SERVICE_TLS_CLIENT_CERT_FILE", "client.crt")
	if _, err := conversationClientTLSConfigFromEnv(); err == nil {
		t.Fatalf("expected partial conversation client certificate config to fail")
	}
}

func TestConversationClientTLSConfigFromEnvLoadsTLS(t *testing.T) {
	clearConversationClientTLSConfig(t)
	t.Setenv("NEXUSIM_CONVERSATION_SERVICE_TLS_CA_FILE", "ca.pem")
	t.Setenv("NEXUSIM_CONVERSATION_SERVICE_TLS_SERVER_NAME", "conversation-service.nexusim.local")
	t.Setenv("NEXUSIM_CONVERSATION_SERVICE_TLS_CLIENT_CERT_FILE", "client.crt")
	t.Setenv("NEXUSIM_CONVERSATION_SERVICE_TLS_CLIENT_KEY_FILE", "client.key")
	config, err := conversationClientTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load conversation client tls config: %v", err)
	}
	if config.CAFile != "ca.pem" ||
		config.ServerName != "conversation-service.nexusim.local" ||
		config.ClientCertFile != "client.crt" ||
		config.ClientKeyFile != "client.key" {
		t.Fatalf("unexpected conversation client tls config: %+v", config)
	}
}

func TestLoadMessageGRPCCredentialsFromEnvDisabledByDefault(t *testing.T) {
	clearMessageGRPCTLSConfig(t)
	creds, ok, err := loadMessageGRPCCredentialsFromEnv()
	if err != nil {
		t.Fatalf("load message grpc credentials: %v", err)
	}
	if ok || creds != nil {
		t.Fatalf("expected message grpc tls to be disabled by default, ok=%t creds=%T", ok, creds)
	}
}

func TestLoadMessageGRPCCredentialsFromEnvRequiresCertKeyPair(t *testing.T) {
	clearMessageGRPCTLSConfig(t)
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_CERT_FILE", "server.crt")
	if _, ok, err := loadMessageGRPCCredentialsFromEnv(); err == nil || !ok {
		t.Fatalf("expected partial message grpc tls config to fail, ok=%t err=%v", ok, err)
	}
}

func TestLoadMessageGRPCCredentialsFromEnvLoadsServerTLS(t *testing.T) {
	clearMessageGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeMessageTLSTestCert(t, dir, "server")
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_KEY_FILE", keyFile)

	tlsConfig, ok, err := messageGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load message grpc tls config: %v", err)
	}
	if !ok || tlsConfig == nil {
		t.Fatalf("expected message grpc tls config, ok=%t", ok)
	}
	if tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected TLS 1.2 minimum, got %d", tlsConfig.MinVersion)
	}

	creds, ok, err := loadMessageGRPCCredentialsFromEnv()
	if err != nil {
		t.Fatalf("load message grpc tls credentials: %v", err)
	}
	if !ok || creds == nil {
		t.Fatalf("expected message grpc tls credentials, ok=%t creds=%T", ok, creds)
	}
}

func TestLoadMessageGRPCCredentialsFromEnvRejectsInvalidRequireClientCert(t *testing.T) {
	clearMessageGRPCTLSConfig(t)
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_REQUIRE_CLIENT_CERT", "sometimes")
	if _, ok, err := loadMessageGRPCCredentialsFromEnv(); err == nil || !ok {
		t.Fatalf("expected invalid message client-cert bool to fail, ok=%t err=%v", ok, err)
	}
}

func TestLoadMessageGRPCCredentialsFromEnvRequiresClientCAForMTLS(t *testing.T) {
	clearMessageGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeMessageTLSTestCert(t, dir, "server")
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_REQUIRE_CLIENT_CERT", "true")
	if _, ok, err := loadMessageGRPCCredentialsFromEnv(); err == nil || !ok {
		t.Fatalf("expected message mtls without ca to fail, ok=%t err=%v", ok, err)
	}
}

func TestMessageGRPCTLSConfigLoadsMTLS(t *testing.T) {
	clearMessageGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeMessageTLSTestCert(t, dir, "server")
	caFile, _ := writeMessageTLSTestCert(t, dir, "ca")
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_CLIENT_CA_FILE", caFile)

	tlsConfig, ok, err := messageGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load message grpc mtls config: %v", err)
	}
	if !ok || tlsConfig == nil {
		t.Fatalf("expected message grpc mtls config, ok=%t", ok)
	}
	if tlsConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("expected client cert verification, got %v", tlsConfig.ClientAuth)
	}
	if tlsConfig.ClientCAs == nil {
		t.Fatalf("expected client CA pool")
	}
}

func TestMessageGRPCTLSConfigAllowsClientIdentity(t *testing.T) {
	clearMessageGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeMessageTLSTestCert(t, dir, "server")
	caFile, _ := writeMessageTLSTestCert(t, dir, "ca")
	clientCertFile, _ := writeMessageTLSTestCertWithSANs(t, dir, "client", []string{"api-gateway.nexusim.local"}, []string{"spiffe://nexusim/api-gateway"})
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_CLIENT_CA_FILE", caFile)
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", " API-GATEWAY.NEXUSIM.LOCAL ")
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_CLIENT_ALLOWED_URIS", "spiffe://nexusim/other-client")

	tlsConfig, ok, err := messageGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load message grpc mtls config: %v", err)
	}
	if !ok || tlsConfig.VerifyConnection == nil {
		t.Fatalf("expected client identity verifier, ok=%t has_verifier=%t", ok, tlsConfig.VerifyConnection != nil)
	}
	if err := tlsConfig.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{readMessageTLSTestCert(t, clientCertFile)}}); err != nil {
		t.Fatalf("expected client identity to be allowed: %v", err)
	}
}

func TestMessageGRPCTLSConfigRejectsUnlistedClientIdentity(t *testing.T) {
	clearMessageGRPCTLSConfig(t)
	dir := t.TempDir()
	certFile, keyFile := writeMessageTLSTestCert(t, dir, "server")
	caFile, _ := writeMessageTLSTestCert(t, dir, "ca")
	clientCertFile, _ := writeMessageTLSTestCertWithSANs(t, dir, "client", []string{"unknown.nexusim.local"}, nil)
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_CERT_FILE", certFile)
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_KEY_FILE", keyFile)
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_CLIENT_CA_FILE", caFile)
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", "api-gateway.nexusim.local")

	tlsConfig, ok, err := messageGRPCTLSConfigFromEnv()
	if err != nil {
		t.Fatalf("load message grpc mtls config: %v", err)
	}
	if !ok || tlsConfig.VerifyConnection == nil {
		t.Fatalf("expected client identity verifier, ok=%t has_verifier=%t", ok, tlsConfig.VerifyConnection != nil)
	}
	if err := tlsConfig.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{readMessageTLSTestCert(t, clientCertFile)}}); err == nil {
		t.Fatalf("expected unlisted client identity to be rejected")
	}
}

func clearPolicyClientTLSConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_POLICY_SERVICE_TLS_CA_FILE", "")
	t.Setenv("NEXUSIM_POLICY_SERVICE_TLS_SERVER_NAME", "")
	t.Setenv("NEXUSIM_POLICY_SERVICE_TLS_CLIENT_CERT_FILE", "")
	t.Setenv("NEXUSIM_POLICY_SERVICE_TLS_CLIENT_KEY_FILE", "")
}

func clearConversationClientTLSConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_CONVERSATION_SERVICE_TLS_CA_FILE", "")
	t.Setenv("NEXUSIM_CONVERSATION_SERVICE_TLS_SERVER_NAME", "")
	t.Setenv("NEXUSIM_CONVERSATION_SERVICE_TLS_CLIENT_CERT_FILE", "")
	t.Setenv("NEXUSIM_CONVERSATION_SERVICE_TLS_CLIENT_KEY_FILE", "")
}

func clearMessageGRPCTLSConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_CERT_FILE", "")
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_KEY_FILE", "")
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_CLIENT_CA_FILE", "")
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_REQUIRE_CLIENT_CERT", "")
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", "")
	t.Setenv("NEXUSIM_MESSAGE_GRPC_TLS_CLIENT_ALLOWED_URIS", "")
}

func writeMessageTLSTestCert(t *testing.T, dir string, name string) (string, string) {
	return writeMessageTLSTestCertWithSANs(t, dir, name, []string{"localhost"}, nil)
}

func writeMessageTLSTestCertWithSANs(t *testing.T, dir string, name string, dnsNames []string, uriNames []string) (string, string) {
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
			CommonName: "message-" + name,
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

func readMessageTLSTestCert(t *testing.T, certFile string) *x509.Certificate {
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
