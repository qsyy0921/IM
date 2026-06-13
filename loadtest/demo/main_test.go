package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc/metadata"
)

func TestEnvBool(t *testing.T) {
	t.Setenv("NEXUSIM_TEST_BOOL", "true")
	if !envBool("NEXUSIM_TEST_BOOL", false) {
		t.Fatal("expected true env bool")
	}
	t.Setenv("NEXUSIM_TEST_BOOL", "off")
	if envBool("NEXUSIM_TEST_BOOL", true) {
		t.Fatal("expected false env bool")
	}
	t.Setenv("NEXUSIM_TEST_BOOL", "invalid")
	if !envBool("NEXUSIM_TEST_BOOL", true) {
		t.Fatal("expected invalid env bool to keep fallback")
	}
}

func TestWithVerifiedAuthMetadataDisabled(t *testing.T) {
	ctx := withVerifiedAuthMetadata(context.Background(), config{}, demoAuth{
		tenantID: "tenant-1",
		userID:   "user-1",
		deviceID: "device-1",
	})
	if _, ok := metadata.FromOutgoingContext(ctx); ok {
		t.Fatal("did not expect outgoing metadata when disabled")
	}
}

func TestWithVerifiedAuthMetadataAddsOutgoingMetadata(t *testing.T) {
	ctx := withVerifiedAuthMetadata(context.Background(), config{verifiedAuthMetadata: true}, demoAuth{
		tenantID:  "tenant-1",
		userID:    "user-1",
		deviceID:  "device-1",
		sessionID: "session-1",
		traceID:   "trace-1",
		requestID: "request-1",
	})
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("expected outgoing metadata")
	}
	assertMetadataValue(t, md, metadataTenantID, "tenant-1")
	assertMetadataValue(t, md, metadataUserID, "user-1")
	assertMetadataValue(t, md, metadataDeviceID, "device-1")
	assertMetadataValue(t, md, metadataSessionID, "session-1")
	assertMetadataValue(t, md, metadataTraceID, "trace-1")
	assertMetadataValue(t, md, metadataRequestID, "request-1")
}

func TestWithUserFacingAuthMetadataUsesGatewayHMACToken(t *testing.T) {
	ctx, err := withUserFacingAuthMetadata(context.Background(), config{
		gatewayAuthMode:       "hmac",
		gatewayAuthHMACSecret: "gateway-secret",
		gatewayAuthTokenTTL:   time.Minute,
	}, demoAuth{
		tenantID:  "tenant-1",
		userID:    "user-1",
		deviceID:  "device-1",
		sessionID: "session-1",
		traceID:   "trace-1",
		requestID: "request-1",
	})
	if err != nil {
		t.Fatalf("withUserFacingAuthMetadata returned error: %v", err)
	}
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("expected outgoing metadata")
	}
	if got := md.Get("authorization"); len(got) != 1 || got[0] == "" {
		t.Fatalf("expected authorization bearer metadata, got %v", got)
	}
	assertMetadataValue(t, md, metadataRequestID, "request-1")
	if values := md.Get(metadataTenantID); len(values) != 0 {
		t.Fatalf("did not expect trusted metadata when using api-gateway auth, got %v", values)
	}
}

func TestWithUserFacingAuthMetadataUsesGatewayMockToken(t *testing.T) {
	ctx, err := withUserFacingAuthMetadata(context.Background(), config{gatewayAuthMode: "mock"}, demoAuth{
		tenantID:  "tenant-1",
		userID:    "user-1",
		deviceID:  "device-1",
		traceID:   "trace-1",
		requestID: "request-1",
	})
	if err != nil {
		t.Fatalf("withUserFacingAuthMetadata returned error: %v", err)
	}
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("expected outgoing metadata")
	}
	assertMetadataValue(t, md, metadataToken, "tenant-1:user-1:device-1")
	assertMetadataValue(t, md, metadataTraceID, "trace-1")
	assertMetadataValue(t, md, metadataRequestID, "request-1")
}

func TestWebSocketDialOptionsCombinesHeaderAndTLS(t *testing.T) {
	caFile := writeTestCACert(t)
	options, err := webSocketDialOptions(config{
		pushTLS: grpctls.Config{
			CAFile:     caFile,
			ServerName: "push-gateway.nexusim.local",
		},
	}, http.Header{"Authorization": []string{"Bearer token-1"}})
	if err != nil {
		t.Fatalf("webSocketDialOptions returned error: %v", err)
	}
	if options == nil {
		t.Fatal("expected dial options")
	}
	if got := options.HTTPHeader.Get("Authorization"); got != "Bearer token-1" {
		t.Fatalf("Authorization header = %q", got)
	}
	if options.HTTPClient == nil {
		t.Fatal("expected HTTP client for WSS TLS")
	}
}

func TestWebSocketTLSConfigRequiresCAFile(t *testing.T) {
	_, err := webSocketTLSConfig(grpctls.Config{ServerName: "push-gateway.nexusim.local"}, "push-tls")
	if err == nil {
		t.Fatal("expected missing CA file error")
	}
}

func TestWebSocketTLSConfigRequiresClientCertPair(t *testing.T) {
	caFile := writeTestCACert(t)
	_, err := webSocketTLSConfig(grpctls.Config{
		CAFile:         caFile,
		ClientCertFile: filepath.Join(t.TempDir(), "client.crt"),
	}, "push-tls")
	if err == nil {
		t.Fatal("expected client cert/key pair error")
	}
}

func assertMetadataValue(t *testing.T, md metadata.MD, key string, want string) {
	t.Helper()
	values := md.Get(key)
	if len(values) != 1 || values[0] != want {
		t.Fatalf("metadata %s = %v, want [%s]", key, values, want)
	}
}

func writeTestCACert(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("generate serial: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	path := filepath.Join(t.TempDir(), "ca.crt")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create CA file: %v", err)
	}
	defer file.Close()
	if err := pem.Encode(file, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatalf("write CA file: %v", err)
	}
	return path
}
