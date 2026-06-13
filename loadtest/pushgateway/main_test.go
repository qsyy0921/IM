package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc/metadata"
)

func TestParseDeviceIDs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		fallback string
		want     []string
	}{
		{
			name:     "multiple",
			input:    "d1,d2",
			fallback: "fallback",
			want:     []string{"d1", "d2"},
		},
		{
			name:     "trim and dedupe",
			input:    " d1, d1, , d2 ",
			fallback: "fallback",
			want:     []string{"d1", "d2"},
		},
		{
			name:     "empty fallback",
			input:    "",
			fallback: "fallback",
			want:     []string{"fallback"},
		},
		{
			name:     "commas fallback",
			input:    ",,",
			fallback: "fallback",
			want:     []string{"fallback"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := parseDeviceIDs(test.input, test.fallback)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("parseDeviceIDs(%q, %q) = %#v, want %#v", test.input, test.fallback, got, test.want)
			}
		})
	}
}

func TestDerivePushMetricsURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "ws", in: "ws://127.0.0.1:11598", want: "http://127.0.0.1:11598/debug/metrics"},
		{name: "wss", in: "wss://push.example/ws?token=x", want: "https://push.example/debug/metrics"},
		{name: "invalid scheme", in: "http://127.0.0.1:11598", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := derivePushMetricsURL(test.in); got != test.want {
				t.Fatalf("derivePushMetricsURL(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestWebSocketDialOptionsDefaultNil(t *testing.T) {
	options, err := webSocketDialOptions(config{}, nil)
	if err != nil {
		t.Fatalf("websocket dial options: %v", err)
	}
	if options != nil {
		t.Fatalf("expected nil options by default")
	}
}

func TestWebSocketTLSConfigRequiresCAFile(t *testing.T) {
	_, err := webSocketTLSConfig(grpctls.Config{ServerName: "push-gateway.nexusim.local"}, "push-tls")
	if err == nil {
		t.Fatalf("expected missing CA file error")
	}
}

func TestWebSocketDialOptionsLoadsTLSAndHeader(t *testing.T) {
	dir := t.TempDir()
	caFile, _ := writePushGatewayLoadtestCert(t, dir, "ca")
	clientCertFile, clientKeyFile := writePushGatewayLoadtestCert(t, dir, "client")
	options, err := webSocketDialOptions(config{
		pushTLS: grpctls.Config{
			CAFile:         caFile,
			ServerName:     "push-gateway.nexusim.local",
			ClientCertFile: clientCertFile,
			ClientKeyFile:  clientKeyFile,
		},
	}, map[string][]string{"Authorization": []string{"Bearer token"}})
	if err != nil {
		t.Fatalf("websocket dial options: %v", err)
	}
	if options == nil || options.HTTPClient == nil {
		t.Fatalf("expected websocket HTTP client with TLS")
	}
	if got := options.HTTPHeader.Get("Authorization"); got != "Bearer token" {
		t.Fatalf("expected authorization header, got %q", got)
	}
}

func TestPushAuthTokenSignedWithNonCurrentSecret(t *testing.T) {
	if pushAuthTokenSignedWithNonCurrentSecret(config{
		pushAuthMode:               "hmac",
		pushAuthHMACSecret:         "current",
		pushAuthTokenSigningSecret: "current",
	}) {
		t.Fatalf("expected current signing secret not to be marked as previous")
	}
	if !pushAuthTokenSignedWithNonCurrentSecret(config{
		pushAuthMode:               "hmac",
		pushAuthHMACSecret:         "current",
		pushAuthTokenSigningSecret: "old",
	}) {
		t.Fatalf("expected distinct signing secret to be marked as previous")
	}
}

func TestNormalizePushAuthConfigDefaultsSigningSecretToCurrent(t *testing.T) {
	cfg := config{
		pushAuthMode:       "hmac",
		pushAuthHMACSecret: " current-secret ",
	}
	normalizePushAuthConfig(&cfg)
	if cfg.pushAuthTokenSigningSecret != "current-secret" {
		t.Fatalf("expected signing secret to default to current secret, got %q", cfg.pushAuthTokenSigningSecret)
	}
	if cfg.pushAuthTokenSigningSecretExplicit {
		t.Fatalf("expected defaulted signing secret not to be explicit")
	}
}

func TestNormalizePushAuthConfigPreservesExplicitSigningSecret(t *testing.T) {
	cfg := config{
		pushAuthMode:               "hmac",
		pushAuthHMACSecret:         "current-secret",
		pushAuthTokenSigningSecret: " old-secret ",
	}
	normalizePushAuthConfig(&cfg)
	if cfg.pushAuthTokenSigningSecret != "old-secret" {
		t.Fatalf("expected explicit signing secret, got %q", cfg.pushAuthTokenSigningSecret)
	}
	if !cfg.pushAuthTokenSigningSecretExplicit {
		t.Fatalf("expected explicit signing secret flag")
	}
}

func TestNormalizeIdentityTokenMethod(t *testing.T) {
	tests := map[string]string{
		"":                    "issue_gateway_token",
		"issue":               "issue_gateway_token",
		"issue_gateway":       "issue_gateway_token",
		"issue_gateway_token": "issue_gateway_token",
		"login":               "login",
		"register":            "register_login",
		"register_login":      "register_login",
		"register_then_login": "register_login",
	}
	for input, want := range tests {
		if got := normalizeIdentityTokenMethod(input); got != want {
			t.Fatalf("normalizeIdentityTokenMethod(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPushAuthTokenSourceMarksIdentityLogin(t *testing.T) {
	loginSource := pushAuthTokenSource(config{
		pushAuthMode:        "hmac",
		identityTarget:      "127.0.0.1:11610",
		identityTokenMethod: "login",
	})
	if loginSource != "identity_service_login" {
		t.Fatalf("expected identity_service_login, got %q", loginSource)
	}
	registerSource := pushAuthTokenSource(config{
		pushAuthMode:        "hmac",
		identityTarget:      "127.0.0.1:11610",
		identityTokenMethod: "register_login",
	})
	if registerSource != "identity_service_register_login" {
		t.Fatalf("expected identity_service_register_login, got %q", registerSource)
	}
	rs256Source := pushAuthTokenSource(config{
		pushAuthMode:               "jwt",
		identityTarget:             "127.0.0.1:11610",
		identityGatewayTokenFormat: "jwt-rs256",
		identityTokenMethod:        "issue_gateway_token",
	})
	if rs256Source != "identity_service" {
		t.Fatalf("expected identity_service for RS256 issue token path, got %q", rs256Source)
	}
}

func TestPushAuthTokenTransportUsesAuthorizationForJWT(t *testing.T) {
	if got := pushAuthTokenTransport(config{pushAuthMode: "jwt"}); got != "authorization_header" {
		t.Fatalf("expected jwt auth to use authorization header, got %q", got)
	}
	if got := pushAuthTokenTransport(config{pushAuthMode: "mock"}); got != "query" {
		t.Fatalf("expected mock auth to use query identity, got %q", got)
	}
}

func TestPushAuthQueryIdentitySentIsFalseForSignedModes(t *testing.T) {
	if pushAuthQueryIdentitySent(config{pushAuthMode: "hmac"}) {
		t.Fatalf("hmac auth should not send query identity")
	}
	if pushAuthQueryIdentitySent(config{pushAuthMode: "jwt"}) {
		t.Fatalf("jwt auth should not send query identity")
	}
	if !pushAuthQueryIdentitySent(config{pushAuthMode: "mock"}) {
		t.Fatalf("mock auth should send query identity")
	}
}

func TestWithVerifiedAuthMetadataDisabled(t *testing.T) {
	ctx := withVerifiedAuthMetadata(context.Background(), config{}, verifiedAuthIdentity{
		tenantID: "tenant-1",
		userID:   "user-1",
		deviceID: "device-1",
	})
	if _, ok := metadata.FromOutgoingContext(ctx); ok {
		t.Fatal("did not expect outgoing metadata when disabled")
	}
}

func TestWithVerifiedAuthMetadataAddsOutgoingMetadata(t *testing.T) {
	ctx := withVerifiedAuthMetadata(context.Background(), config{verifiedAuthMetadata: true}, verifiedAuthIdentity{
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

func TestAuthHelpersUseExpectedIdentities(t *testing.T) {
	cfg := config{
		tenantID:         "tenant-1",
		ownerUserID:      "owner-1",
		receiverUserID:   "receiver-1",
		receiverDeviceID: "receiver-device-1",
	}
	owner := ownerAuth(cfg, "trace-owner", "request-owner")
	if got := conversationAuth(owner); got.GetTenantId() != "tenant-1" || got.GetUserId() != "owner-1" || got.GetDeviceId() != "push-smoke-owner-device" {
		t.Fatalf("unexpected conversation owner auth: %+v", got)
	}
	if got := messageAuth(owner); got.GetTenantId() != "tenant-1" || got.GetUserId() != "owner-1" || got.GetDeviceId() != "push-smoke-owner-device" {
		t.Fatalf("unexpected message owner auth: %+v", got)
	}
	receiver := receiverAuth(cfg, "", "trace-receiver", "request-receiver")
	if got := deliveryAuth(receiver); got.GetTenantId() != "tenant-1" || got.GetUserId() != "receiver-1" || got.GetDeviceId() != "receiver-device-1" {
		t.Fatalf("unexpected delivery receiver auth: %+v", got)
	}
}

func TestEnvBoolUsesFirstConfiguredValue(t *testing.T) {
	t.Setenv("NEXUSIM_PUSH_TEST_BOOL_A", "")
	t.Setenv("NEXUSIM_PUSH_TEST_BOOL_B", "true")
	if !envBool(false, "NEXUSIM_PUSH_TEST_BOOL_A", "NEXUSIM_PUSH_TEST_BOOL_B") {
		t.Fatal("expected true from second configured env")
	}
}

func TestSmokePasswordHashUsesPBKDF2Format(t *testing.T) {
	encoded, err := smokePasswordHash("push-smoke-password")
	if err != nil {
		t.Fatalf("smoke password hash: %v", err)
	}
	if !strings.HasPrefix(encoded, "pbkdf2-sha256$10000$") {
		t.Fatalf("unexpected password hash format: %s", encoded)
	}
}

func TestSignPushGatewayTokenUsesTokenSigningSecret(t *testing.T) {
	cfg := config{
		tenantID:                   "tenant-1",
		receiverUserID:             "user-1",
		pushAuthHMACSecret:         "current-secret",
		pushAuthTokenSigningSecret: "old-secret",
		pushAuthTokenTTL:           time.Minute,
	}
	token, err := signPushGatewayToken(cfg, "device-1")
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		t.Fatalf("unexpected token shape: %q", token)
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	if !validHMAC(parts[0], signature, "old-secret") {
		t.Fatalf("expected token signed by old secret")
	}
	if validHMAC(parts[0], signature, "current-secret") {
		t.Fatalf("did not expect token signed by current secret")
	}
}

func validHMAC(payload string, signature []byte, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return hmac.Equal(signature, mac.Sum(nil))
}

func assertMetadataValue(t *testing.T, md metadata.MD, key string, want string) {
	t.Helper()
	values := md.Get(key)
	if len(values) != 1 || values[0] != want {
		t.Fatalf("metadata %s = %v, want [%s]", key, values, want)
	}
}

func writePushGatewayLoadtestCert(t *testing.T, dir string, name string) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate tls key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("generate tls serial: %v", err)
	}
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{name + ".nexusim.local"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create tls cert: %v", err)
	}
	certFile := filepath.Join(dir, name+".crt")
	keyFile := filepath.Join(dir, name+".key")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write tls cert: %v", err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0o600); err != nil {
		t.Fatalf("write tls key: %v", err)
	}
	return certFile, keyFile
}
