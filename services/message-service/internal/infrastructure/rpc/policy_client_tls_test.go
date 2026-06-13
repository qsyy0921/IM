package rpc

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPolicyClientTLSConfigEnabled(t *testing.T) {
	if (PolicyClientTLSConfig{}).Enabled() {
		t.Fatalf("expected empty policy tls config to be disabled")
	}
	if !(PolicyClientTLSConfig{CAFile: "ca.pem"}).Enabled() {
		t.Fatalf("expected CA file to enable policy tls")
	}
	if !(PolicyClientTLSConfig{ServerName: "policy-service.nexusim.local"}).Enabled() {
		t.Fatalf("expected server name to enable policy tls")
	}
}

func TestPolicyClientTLSCredentialsRequireCAFile(t *testing.T) {
	_, err := policyClientTLSCredentials(PolicyClientTLSConfig{ServerName: "policy-service.nexusim.local"})
	if err == nil {
		t.Fatalf("expected policy tls without CA file to fail")
	}
}

func TestPolicyClientTLSCredentialsRequireClientKeyPair(t *testing.T) {
	dir := t.TempDir()
	caFile, _ := writePolicyClientTLSTestCert(t, dir, "ca")
	_, err := policyClientTLSCredentials(PolicyClientTLSConfig{
		CAFile:         caFile,
		ClientCertFile: "client.crt",
	})
	if err == nil {
		t.Fatalf("expected partial client key pair to fail")
	}
}

func TestPolicyClientTLSCredentialsLoadCAAndClientCert(t *testing.T) {
	dir := t.TempDir()
	caFile, _ := writePolicyClientTLSTestCert(t, dir, "ca")
	clientCertFile, clientKeyFile := writePolicyClientTLSTestCert(t, dir, "client")
	creds, err := policyClientTLSCredentials(PolicyClientTLSConfig{
		CAFile:         caFile,
		ServerName:     "policy-service.nexusim.local",
		ClientCertFile: clientCertFile,
		ClientKeyFile:  clientKeyFile,
	})
	if err != nil {
		t.Fatalf("load policy tls credentials: %v", err)
	}
	if creds == nil {
		t.Fatalf("expected policy tls credentials")
	}
}

func TestConversationClientTLSCredentialsRequireCAFile(t *testing.T) {
	_, err := conversationClientTLSCredentials(ConversationClientTLSConfig{ServerName: "conversation-service.nexusim.local"})
	if err == nil {
		t.Fatalf("expected conversation tls without CA file to fail")
	}
}

func TestConversationClientTLSCredentialsRequireClientKeyPair(t *testing.T) {
	dir := t.TempDir()
	caFile, _ := writePolicyClientTLSTestCert(t, dir, "ca")
	_, err := conversationClientTLSCredentials(ConversationClientTLSConfig{
		CAFile:         caFile,
		ClientCertFile: "client.crt",
	})
	if err == nil {
		t.Fatalf("expected partial conversation client key pair to fail")
	}
}

func TestConversationClientTLSCredentialsLoadCAAndClientCert(t *testing.T) {
	dir := t.TempDir()
	caFile, _ := writePolicyClientTLSTestCert(t, dir, "ca")
	clientCertFile, clientKeyFile := writePolicyClientTLSTestCert(t, dir, "client")
	creds, err := conversationClientTLSCredentials(ConversationClientTLSConfig{
		CAFile:         caFile,
		ServerName:     "conversation-service.nexusim.local",
		ClientCertFile: clientCertFile,
		ClientKeyFile:  clientKeyFile,
	})
	if err != nil {
		t.Fatalf("load conversation tls credentials: %v", err)
	}
	if creds == nil {
		t.Fatalf("expected conversation tls credentials")
	}
}

func writePolicyClientTLSTestCert(t *testing.T, dir string, name string) (string, string) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate tls key: %v", err)
	}
	serialNumber, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		t.Fatalf("generate tls serial: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "message-policy-client-" + name,
		},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"policy-service.nexusim.local", "message-service.nexusim.local"},
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
