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

func TestDeliveryClientTLSConfigEnabled(t *testing.T) {
	if (DeliveryClientTLSConfig{}).Enabled() {
		t.Fatalf("expected empty delivery tls config to be disabled")
	}
	if !(DeliveryClientTLSConfig{CAFile: "ca.pem"}).Enabled() {
		t.Fatalf("expected CA file to enable delivery tls")
	}
	if !(DeliveryClientTLSConfig{ServerName: "delivery-service.nexusim.local"}).Enabled() {
		t.Fatalf("expected server name to enable delivery tls")
	}
	if !(DeliveryClientTLSConfig{ClientCertFile: "client.crt"}).Enabled() {
		t.Fatalf("expected client cert file to enable delivery tls")
	}
}

func TestDeliveryClientTLSCredentialsRequireCAFile(t *testing.T) {
	_, err := deliveryClientTLSCredentials(DeliveryClientTLSConfig{ServerName: "delivery-service.nexusim.local"})
	if err == nil {
		t.Fatalf("expected delivery tls without CA file to fail")
	}
}

func TestDeliveryClientTLSCredentialsRequireClientKeyPair(t *testing.T) {
	dir := t.TempDir()
	caFile, _ := writeDeliveryClientTLSTestCert(t, dir, "ca")
	_, err := deliveryClientTLSCredentials(DeliveryClientTLSConfig{
		CAFile:         caFile,
		ClientCertFile: "client.crt",
	})
	if err == nil {
		t.Fatalf("expected partial delivery client key pair to fail")
	}
}

func TestDeliveryClientTLSCredentialsLoadCAAndClientCert(t *testing.T) {
	dir := t.TempDir()
	caFile, _ := writeDeliveryClientTLSTestCert(t, dir, "ca")
	clientCertFile, clientKeyFile := writeDeliveryClientTLSTestCert(t, dir, "client")
	creds, err := deliveryClientTLSCredentials(DeliveryClientTLSConfig{
		CAFile:         caFile,
		ServerName:     "delivery-service.nexusim.local",
		ClientCertFile: clientCertFile,
		ClientKeyFile:  clientKeyFile,
	})
	if err != nil {
		t.Fatalf("load delivery tls credentials: %v", err)
	}
	if creds == nil {
		t.Fatalf("expected delivery tls credentials")
	}
}

func writeDeliveryClientTLSTestCert(t *testing.T, dir string, name string) (string, string) {
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
			CommonName: "push-delivery-client-" + name,
		},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"delivery-service.nexusim.local", "push-gateway.nexusim.local"},
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
