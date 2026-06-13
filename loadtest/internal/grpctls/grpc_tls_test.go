package grpctls

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

func TestConfigEnabled(t *testing.T) {
	if (Config{}).Enabled() {
		t.Fatalf("expected empty config to be disabled")
	}
	if !(Config{CAFile: "ca.pem"}).Enabled() {
		t.Fatalf("expected CA file to enable config")
	}
	if !(Config{ServerName: "receipt-service.nexusim.local"}).Enabled() {
		t.Fatalf("expected server name to enable config")
	}
	if !(Config{ClientCertFile: "client.crt"}).Enabled() {
		t.Fatalf("expected client cert to enable config")
	}
}

func TestDialOptionDefaultsToInsecure(t *testing.T) {
	option, err := DialOption(Config{}, "receipt-tls")
	if err != nil {
		t.Fatalf("dial option: %v", err)
	}
	if option == nil {
		t.Fatalf("expected dial option")
	}
}

func TestTransportCredentialsRequireCAFile(t *testing.T) {
	_, err := TransportCredentials(Config{ServerName: "receipt-service.nexusim.local"}, "receipt-tls")
	if err == nil {
		t.Fatalf("expected missing CA file error")
	}
}

func TestTransportCredentialsRequireClientKeyPair(t *testing.T) {
	dir := t.TempDir()
	caFile, _ := writeTestCert(t, dir, "ca")
	_, err := TransportCredentials(Config{
		CAFile:         caFile,
		ClientCertFile: "client.crt",
	}, "receipt-tls")
	if err == nil {
		t.Fatalf("expected partial client key pair error")
	}
}

func TestTransportCredentialsLoadCAAndClientCert(t *testing.T) {
	dir := t.TempDir()
	caFile, _ := writeTestCert(t, dir, "ca")
	clientCertFile, clientKeyFile := writeTestCert(t, dir, "client")
	creds, err := TransportCredentials(Config{
		CAFile:         caFile,
		ServerName:     "receipt-service.nexusim.local",
		ClientCertFile: clientCertFile,
		ClientKeyFile:  clientKeyFile,
	}, "receipt-tls")
	if err != nil {
		t.Fatalf("load tls credentials: %v", err)
	}
	if creds == nil {
		t.Fatalf("expected tls credentials")
	}
}

func writeTestCert(t *testing.T, dir string, name string) (string, string) {
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
			CommonName: "loadtest-grpc-tls-" + name,
		},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"receipt-service.nexusim.local", "loadtest.nexusim.local"},
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
