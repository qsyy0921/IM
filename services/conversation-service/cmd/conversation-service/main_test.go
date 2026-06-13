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
