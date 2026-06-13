package main

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

func TestGRPCClientTLSConfigFromEnvDisabledByDefault(t *testing.T) {
	clearAPIGatewayTestTLSConfig(t, "NEXUSIM_API_GATEWAY_MESSAGE_TLS")
	config := grpcClientTLSConfigFromEnv("NEXUSIM_API_GATEWAY_MESSAGE_TLS")
	if config.Enabled() {
		t.Fatalf("expected empty api-gateway downstream tls config to be disabled: %+v", config)
	}
}

func TestGRPCClientTLSCredentialsRequireCAFile(t *testing.T) {
	_, err := grpcClientTLSCredentials(grpcClientTLSConfig{
		EnvPrefix:  "NEXUSIM_API_GATEWAY_MESSAGE_TLS",
		ServerName: "message-service.nexusim.local",
	})
	if err == nil {
		t.Fatalf("expected missing CA file error")
	}
}

func TestGRPCClientTLSCredentialsRequireClientKeyPair(t *testing.T) {
	dir := t.TempDir()
	caFile, _ := writeAPIGatewayTLSTestCert(t, dir, "ca")
	_, err := grpcClientTLSCredentials(grpcClientTLSConfig{
		EnvPrefix:      "NEXUSIM_API_GATEWAY_MESSAGE_TLS",
		CAFile:         caFile,
		ClientCertFile: "client.crt",
	})
	if err == nil {
		t.Fatalf("expected partial client certificate config to fail")
	}
}

func TestGRPCClientTLSCredentialsLoadCAAndClientCert(t *testing.T) {
	dir := t.TempDir()
	caFile, _ := writeAPIGatewayTLSTestCert(t, dir, "ca")
	clientCertFile, clientKeyFile := writeAPIGatewayTLSTestCert(t, dir, "api-gateway")
	creds, err := grpcClientTLSCredentials(grpcClientTLSConfig{
		EnvPrefix:      "NEXUSIM_API_GATEWAY_MESSAGE_TLS",
		CAFile:         caFile,
		ServerName:     "message-service.nexusim.local",
		ClientCertFile: clientCertFile,
		ClientKeyFile:  clientKeyFile,
	})
	if err != nil {
		t.Fatalf("load api-gateway downstream tls credentials: %v", err)
	}
	if creds == nil {
		t.Fatalf("expected downstream tls credentials")
	}
}

func TestGRPCClientTLSConfigFromEnvLoadsValues(t *testing.T) {
	clearAPIGatewayTestTLSConfig(t, "NEXUSIM_API_GATEWAY_MESSAGE_TLS")
	t.Setenv("NEXUSIM_API_GATEWAY_MESSAGE_TLS_CA_FILE", "ca.crt")
	t.Setenv("NEXUSIM_API_GATEWAY_MESSAGE_TLS_SERVER_NAME", "message-service.nexusim.local")
	t.Setenv("NEXUSIM_API_GATEWAY_MESSAGE_TLS_CLIENT_CERT_FILE", "client.crt")
	t.Setenv("NEXUSIM_API_GATEWAY_MESSAGE_TLS_CLIENT_KEY_FILE", "client.key")
	config := grpcClientTLSConfigFromEnv("NEXUSIM_API_GATEWAY_MESSAGE_TLS")
	if config.CAFile != "ca.crt" ||
		config.ServerName != "message-service.nexusim.local" ||
		config.ClientCertFile != "client.crt" ||
		config.ClientKeyFile != "client.key" {
		t.Fatalf("unexpected downstream tls config: %+v", config)
	}
}

func clearAPIGatewayTestTLSConfig(t *testing.T, prefix string) {
	t.Helper()
	t.Setenv(prefix+"_CA_FILE", "")
	t.Setenv(prefix+"_SERVER_NAME", "")
	t.Setenv(prefix+"_CLIENT_CERT_FILE", "")
	t.Setenv(prefix+"_CLIENT_KEY_FILE", "")
}

func writeAPIGatewayTLSTestCert(t *testing.T, dir string, name string) (string, string) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate tls key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber:          newSerialNumber(t),
		Subject:               pkix.Name{CommonName: "api-gateway-test-" + name},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"message-service.nexusim.local", "api-gateway.nexusim.local"},
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

func newSerialNumber(t *testing.T) *big.Int {
	t.Helper()
	serial, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		t.Fatalf("generate tls serial: %v", err)
	}
	return serial
}
