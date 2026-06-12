package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
)

func TestAuthenticatorHMACAcceptsSignedGatewayToken(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	authenticator, err := NewAuthenticator(Config{Mode: ModeHMAC, Secret: "secret", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	token, err := SignGatewayToken("secret", map[string]string{
		"tenant_id":  "tenant-1",
		"user_id":    "user-1",
		"device_id":  "device-1",
		"session_id": "session-1",
		"trace_id":   "trace-1",
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	request := httptest.NewRequest("GET", "/ws?token="+token+"&device_id=device-1", nil)
	auth, err := authenticator.Authenticate(request)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if auth.TenantID != "tenant-1" || auth.UserID != "user-1" || auth.DeviceID != "device-1" || auth.SessionID != "session-1" || auth.TraceID != "trace-1" {
		t.Fatalf("unexpected auth: %+v", auth)
	}
}

func TestAuthenticatorHMACAcceptsJWTGatewayToken(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	authenticator, err := NewAuthenticator(Config{Mode: ModeHMAC, Secret: "secret", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	token, err := SignGatewayJWT("secret", map[string]string{
		"tenant_id":  "tenant-1",
		"user_id":    "user-1",
		"device_id":  "device-1",
		"session_id": "session-1",
		"trace_id":   "trace-1",
		"kid":        "kid-1",
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	request := httptest.NewRequest("GET", "/ws?device_id=device-1", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	auth, err := authenticator.Authenticate(request)
	if err != nil {
		t.Fatalf("authenticate jwt: %v", err)
	}
	if auth.TenantID != "tenant-1" || auth.UserID != "user-1" || auth.DeviceID != "device-1" || auth.SessionID != "session-1" || auth.TraceID != "trace-1" {
		t.Fatalf("unexpected auth: %+v", auth)
	}
}

func TestAuthenticatorJWTAcceptsRS256GatewayToken(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	privateKey := generateTestRSAKey(t)
	authenticator, err := NewAuthenticator(Config{
		Mode:           ModeJWT,
		JWKSetJSON:     testRSAJWKSetJSON(t, privateKey, "rsa-kid-1"),
		TrustedIssuers: []string{"issuer-1"},
		Now:            func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	token := signTestRS256JWT(t, privateKey, "rsa-kid-1", map[string]string{
		"tenant_id":  "tenant-1",
		"user_id":    "user-1",
		"device_id":  "device-1",
		"session_id": "session-1",
		"trace_id":   "trace-1",
		"iss":        "issuer-1",
		"aud":        "push-gateway",
	}, now.Add(time.Minute))

	request := httptest.NewRequest("GET", "/ws?device_id=device-1", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	auth, err := authenticator.Authenticate(request)
	if err != nil {
		t.Fatalf("authenticate jwt: %v", err)
	}
	if auth.TenantID != "tenant-1" || auth.UserID != "user-1" || auth.DeviceID != "device-1" || auth.SessionID != "session-1" || auth.TraceID != "trace-1" {
		t.Fatalf("unexpected auth: %+v", auth)
	}
}

func TestAuthenticatorJWTRejectsHS256GatewayToken(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	privateKey := generateTestRSAKey(t)
	authenticator, err := NewAuthenticator(Config{
		Mode:       ModeJWT,
		JWKSetJSON: testRSAJWKSetJSON(t, privateKey, "rsa-kid-1"),
		Now:        func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	token, err := SignGatewayJWT("secret", map[string]string{
		"tenant_id": "tenant-1",
		"user_id":   "user-1",
		"device_id": "device-1",
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	_, err = authenticator.Authenticate(httptest.NewRequest("GET", "/ws?token="+token, nil))
	if err == nil || !strings.Contains(err.Error(), types.ErrPermissionDenied.Error()) {
		t.Fatalf("expected permission denied, got %v", err)
	}
}

func TestAuthenticatorJWTRejectsUntrustedIssuer(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	privateKey := generateTestRSAKey(t)
	authenticator, err := NewAuthenticator(Config{
		Mode:           ModeJWT,
		JWKSetJSON:     testRSAJWKSetJSON(t, privateKey, "rsa-kid-1"),
		TrustedIssuers: []string{"issuer-1"},
		Now:            func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	token := signTestRS256JWT(t, privateKey, "rsa-kid-1", map[string]string{
		"tenant_id": "tenant-1",
		"user_id":   "user-1",
		"device_id": "device-1",
		"iss":       "issuer-2",
		"aud":       "push-gateway",
	}, now.Add(time.Minute))

	_, err = authenticator.Authenticate(httptest.NewRequest("GET", "/ws?token="+token, nil))
	if err == nil || !strings.Contains(err.Error(), types.ErrPermissionDenied.Error()) {
		t.Fatalf("expected permission denied, got %v", err)
	}
}

func TestAuthenticatorJWTRefreshesRemoteJWKSet(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	privateKey1 := generateTestRSAKey(t)
	privateKey2 := generateTestRSAKey(t)
	var currentJWKSet atomic.Value
	currentJWKSet.Store(testRSAJWKSetJSON(t, privateKey1, "rsa-kid-1"))
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(currentJWKSet.Load().(string)))
	}))
	defer server.Close()
	authenticator, err := NewAuthenticator(Config{
		Mode:               ModeJWT,
		JWKSetURL:          server.URL,
		JWKRefreshInterval: 10 * time.Millisecond,
		TrustedIssuers:     []string{"issuer-1"},
		Now:                func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	defer authenticator.Close()
	token1 := signTestRS256JWT(t, privateKey1, "rsa-kid-1", map[string]string{
		"tenant_id": "tenant-1",
		"user_id":   "user-1",
		"device_id": "device-1",
		"iss":       "issuer-1",
		"aud":       "push-gateway",
	}, now.Add(time.Minute))
	if _, err := authenticator.Authenticate(requestWithBearer(token1)); err != nil {
		t.Fatalf("authenticate initial remote jwks token: %v", err)
	}

	token2 := signTestRS256JWT(t, privateKey2, "rsa-kid-2", map[string]string{
		"tenant_id": "tenant-1",
		"user_id":   "user-1",
		"device_id": "device-1",
		"iss":       "issuer-1",
		"aud":       "push-gateway",
	}, now.Add(time.Minute))
	if _, err := authenticator.Authenticate(requestWithBearer(token2)); err == nil {
		t.Fatalf("expected token signed by unknown kid to fail before refresh")
	}
	currentJWKSet.Store(testRSAJWKSetJSON(t, privateKey2, "rsa-kid-2"))
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := authenticator.Authenticate(requestWithBearer(token2)); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected refreshed remote jwks to accept rotated key")
}

func TestAuthenticatorJWTRejectsUnavailableRemoteJWKSetWithoutStaticFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "unavailable", http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := NewAuthenticator(Config{
		Mode:      ModeJWT,
		JWKSetURL: server.URL,
		Now:       func() time.Time { return time.Unix(1_800_000_000, 0) },
	})
	if err == nil {
		t.Fatalf("expected remote jwks startup failure without static fallback")
	}
}

func TestAuthenticatorJWTKeepsStaticJWKSetWhenInitialRemoteFetchFails(t *testing.T) {
	privateKey := generateTestRSAKey(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "unavailable", http.StatusInternalServerError)
	}))
	defer server.Close()
	authenticator, err := NewAuthenticator(Config{
		Mode:           ModeJWT,
		JWKSetJSON:     testRSAJWKSetJSON(t, privateKey, "rsa-kid-1"),
		JWKSetURL:      server.URL,
		TrustedIssuers: []string{"issuer-1"},
		Now:            func() time.Time { return time.Unix(1_800_000_000, 0) },
	})
	if err != nil {
		t.Fatalf("new authenticator with static fallback: %v", err)
	}
	defer authenticator.Close()
	stats := authenticator.JWKStats()
	if !stats.RemoteURLConfigured || stats.CachedKeyCount != 1 || stats.RefreshFailures == 0 || stats.LastRefreshFailure == 0 {
		t.Fatalf("expected remote failure stats with cached static key, got %+v", stats)
	}
	token := signTestRS256JWT(t, privateKey, "rsa-kid-1", map[string]string{
		"tenant_id": "tenant-1",
		"user_id":   "user-1",
		"device_id": "device-1",
		"iss":       "issuer-1",
		"aud":       "push-gateway",
	}, time.Unix(1_800_000_000, 0).Add(time.Minute))
	if _, err := authenticator.Authenticate(requestWithBearer(token)); err != nil {
		t.Fatalf("expected static key to remain usable: %v", err)
	}
}

func TestAuthenticatorJWTRefreshFailureKeepsPreviousRemoteJWKSet(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	privateKey := generateTestRSAKey(t)
	var failRemote atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if failRemote.Load() {
			http.Error(writer, "unavailable", http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(testRSAJWKSetJSON(t, privateKey, "rsa-kid-1")))
	}))
	defer server.Close()
	authenticator, err := NewAuthenticator(Config{
		Mode:               ModeJWT,
		JWKSetURL:          server.URL,
		JWKRefreshInterval: 10 * time.Millisecond,
		TrustedIssuers:     []string{"issuer-1"},
		Now:                func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	defer authenticator.Close()
	failRemote.Store(true)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if authenticator.JWKStats().RefreshFailures > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	stats := authenticator.JWKStats()
	if stats.RefreshFailures == 0 || stats.LastRefreshFailure == 0 || stats.CachedKeyCount != 1 {
		t.Fatalf("expected refresh failure to be recorded while keeping old key, got %+v", stats)
	}
	token := signTestRS256JWT(t, privateKey, "rsa-kid-1", map[string]string{
		"tenant_id": "tenant-1",
		"user_id":   "user-1",
		"device_id": "device-1",
		"iss":       "issuer-1",
		"aud":       "push-gateway",
	}, now.Add(time.Minute))
	if _, err := authenticator.Authenticate(requestWithBearer(token)); err != nil {
		t.Fatalf("expected previous remote key to remain usable: %v", err)
	}
}

func TestParseRS256JWKSetRejectsNonSigningAndWeakKeys(t *testing.T) {
	privateKey := generateTestRSAKey(t)
	nonSigning, err := json.Marshal(jwkSet{Keys: []jwk{{
		KeyType:   "RSA",
		KeyUse:    "enc",
		KeyID:     "enc-key",
		Algorithm: "RS256",
		Modulus:   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
		Exponent:  base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privateKey.PublicKey.E)).Bytes()),
	}}})
	if err != nil {
		t.Fatalf("marshal non-signing jwks: %v", err)
	}
	if _, err := parseRS256JWKSet(string(nonSigning)); err == nil {
		t.Fatalf("expected non-signing jwk to be rejected")
	}

	weakKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate weak rsa key: %v", err)
	}
	weak, err := json.Marshal(jwkSet{Keys: []jwk{{
		KeyType:   "RSA",
		KeyUse:    "sig",
		KeyID:     "weak-key",
		Algorithm: "RS256",
		Modulus:   base64.RawURLEncoding.EncodeToString(weakKey.PublicKey.N.Bytes()),
		Exponent:  base64.RawURLEncoding.EncodeToString(big.NewInt(int64(weakKey.PublicKey.E)).Bytes()),
	}}})
	if err != nil {
		t.Fatalf("marshal weak jwks: %v", err)
	}
	if _, err := parseRS256JWKSet(string(weak)); err == nil {
		t.Fatalf("expected weak jwk to be rejected")
	}
}

func TestAuthenticatorHMACRejectsJWTWrongAlgorithm(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	authenticator, err := NewAuthenticator(Config{Mode: ModeHMAC, Secret: "secret", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	token, err := SignGatewayJWT("secret", map[string]string{"tenant_id": "tenant-1", "user_id": "user-1"}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	parts := strings.Split(token, ".")
	header := `{"alg":"none","typ":"JWT"}`
	parts[0] = base64.RawURLEncoding.EncodeToString([]byte(header))
	_, err = authenticator.Authenticate(httptest.NewRequest("GET", "/ws?token="+strings.Join(parts, "."), nil))
	if err == nil || !strings.Contains(err.Error(), types.ErrPermissionDenied.Error()) {
		t.Fatalf("expected permission denied, got %v", err)
	}
}

func TestAuthenticatorHMACRejectsExpiredToken(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	authenticator, err := NewAuthenticator(Config{Mode: ModeHMAC, Secret: "secret", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	token, err := SignGatewayToken("secret", map[string]string{"tenant_id": "tenant-1", "user_id": "user-1"}, now.Add(-time.Second))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	_, err = authenticator.Authenticate(httptest.NewRequest("GET", "/ws?token="+token, nil))
	if err == nil || !strings.Contains(err.Error(), types.ErrAuthExpired.Error()) {
		t.Fatalf("expected auth expired, got %v", err)
	}
}

func TestAuthenticatorHMACRejectsDeviceMismatch(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	authenticator, err := NewAuthenticator(Config{Mode: ModeHMAC, Secret: "secret", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	token, err := SignGatewayToken("secret", map[string]string{
		"tenant_id": "tenant-1",
		"user_id":   "user-1",
		"device_id": "device-1",
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	_, err = authenticator.Authenticate(httptest.NewRequest("GET", "/ws?token="+token+"&device_id=device-2", nil))
	if err == nil || !strings.Contains(err.Error(), types.ErrPermissionDenied.Error()) {
		t.Fatalf("expected permission denied, got %v", err)
	}
}

func TestAuthenticatorHMACRejectsWrongAudience(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	authenticator, err := NewAuthenticator(Config{Mode: ModeHMAC, Secret: "secret", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	token, err := SignGatewayToken("secret", map[string]string{
		"tenant_id": "tenant-1",
		"user_id":   "user-1",
		"aud":       "other-service",
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	_, err = authenticator.Authenticate(httptest.NewRequest("GET", "/ws?token="+token, nil))
	if err == nil || !strings.Contains(err.Error(), types.ErrPermissionDenied.Error()) {
		t.Fatalf("expected permission denied, got %v", err)
	}
}

func TestAuthenticatorHMACAcceptsPreviousSecretDuringRotation(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	authenticator, err := NewAuthenticator(Config{
		Mode:            ModeHMAC,
		Secret:          "current-secret",
		PreviousSecrets: []string{"previous-secret"},
		Now:             func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	token, err := SignGatewayToken("previous-secret", map[string]string{
		"tenant_id": "tenant-1",
		"user_id":   "user-1",
		"device_id": "device-1",
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	auth, err := authenticator.Authenticate(httptest.NewRequest("GET", "/ws?token="+token+"&device_id=device-1", nil))
	if err != nil {
		t.Fatalf("authenticate previous secret: %v", err)
	}
	if auth.TenantID != "tenant-1" || auth.UserID != "user-1" || auth.DeviceID != "device-1" {
		t.Fatalf("unexpected auth: %+v", auth)
	}
}

func TestAuthenticatorHMACTrimsAndDeduplicatesPreviousSecrets(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	authenticator, err := NewAuthenticator(Config{
		Mode:            ModeHMAC,
		Secret:          "current-secret",
		PreviousSecrets: []string{"", " current-secret ", " previous-secret ", "previous-secret"},
		Now:             func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	if len(authenticator.secrets) != 2 {
		t.Fatalf("expected current plus one previous secret, got %d", len(authenticator.secrets))
	}
	token, err := SignGatewayToken("previous-secret", map[string]string{
		"tenant_id": "tenant-1",
		"user_id":   "user-1",
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	if _, err := authenticator.Authenticate(httptest.NewRequest("GET", "/ws?token="+token, nil)); err != nil {
		t.Fatalf("authenticate previous secret: %v", err)
	}
}

func TestAuthenticatorHMACRejectsUnknownSecret(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	authenticator, err := NewAuthenticator(Config{
		Mode:            ModeHMAC,
		Secret:          "current-secret",
		PreviousSecrets: []string{"previous-secret"},
		Now:             func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	token, err := SignGatewayToken("unknown-secret", map[string]string{
		"tenant_id": "tenant-1",
		"user_id":   "user-1",
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	_, err = authenticator.Authenticate(httptest.NewRequest("GET", "/ws?token="+token, nil))
	if err == nil || !strings.Contains(err.Error(), types.ErrPermissionDenied.Error()) {
		t.Fatalf("expected permission denied, got %v", err)
	}
}

func TestAuthenticatorHMACRejectsRevokedToken(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	revocation := &fakeRevocationChecker{revoked: true}
	authenticator, err := NewAuthenticator(Config{
		Mode:       ModeHMAC,
		Secret:     "secret",
		Revocation: revocation,
		Now:        func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	token, err := SignGatewayToken("secret", map[string]string{
		"tenant_id":  "tenant-1",
		"user_id":    "user-1",
		"device_id":  "device-1",
		"session_id": "session-1",
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	_, err = authenticator.Authenticate(httptest.NewRequest("GET", "/ws?token="+token+"&device_id=device-1", nil))
	if err == nil || !strings.Contains(err.Error(), types.ErrPermissionDenied.Error()) {
		t.Fatalf("expected permission denied, got %v", err)
	}
	if revocation.auth.SessionID != "session-1" {
		t.Fatalf("expected revocation check to receive session id, got %+v", revocation.auth)
	}
}

func TestAuthenticatorHMACRejectsRevocationCheckError(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	authenticator, err := NewAuthenticator(Config{
		Mode:       ModeHMAC,
		Secret:     "secret",
		Revocation: &fakeRevocationChecker{err: errors.New("redis unavailable")},
		Now:        func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	token, err := SignGatewayToken("secret", map[string]string{"tenant_id": "tenant-1", "user_id": "user-1"}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	_, err = authenticator.Authenticate(httptest.NewRequest("GET", "/ws?token="+token, nil))
	if err == nil || !strings.Contains(err.Error(), types.ErrPermissionDenied.Error()) {
		t.Fatalf("expected permission denied, got %v", err)
	}
}

func TestAuthenticatorMockKeepsLocalSmokeCompatibility(t *testing.T) {
	authenticator, err := NewAuthenticator(Config{Mode: ModeMock})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	auth, err := authenticator.Authenticate(httptest.NewRequest("GET", "/ws?token=tenant-1:user-1:device-1", nil))
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if auth.TenantID != "tenant-1" || auth.UserID != "user-1" || auth.DeviceID != "device-1" {
		t.Fatalf("unexpected auth: %+v", auth)
	}
}

type fakeRevocationChecker struct {
	auth    types.AuthContext
	revoked bool
	err     error
}

func (checker *fakeRevocationChecker) IsRevoked(ctx context.Context, auth types.AuthContext) (bool, error) {
	checker.auth = auth
	if checker.err != nil {
		return false, checker.err
	}
	return checker.revoked, nil
}

func generateTestRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	return key
}

func testRSAJWKSetJSON(t *testing.T, privateKey *rsa.PrivateKey, keyID string) string {
	t.Helper()
	raw, err := json.Marshal(jwkSet{Keys: []jwk{{
		KeyType:   "RSA",
		KeyUse:    "sig",
		KeyID:     keyID,
		Algorithm: "RS256",
		Modulus:   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
		Exponent:  base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privateKey.PublicKey.E)).Bytes()),
	}}})
	if err != nil {
		t.Fatalf("marshal jwks: %v", err)
	}
	return string(raw)
}

func signTestRS256JWT(t *testing.T, privateKey *rsa.PrivateKey, keyID string, claims map[string]string, expiresAt time.Time) string {
	t.Helper()
	headerRaw, err := json.Marshal(tokenHeader{Algorithm: "RS256", Type: "JWT", KeyID: keyID})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	payload := tokenClaims{
		TenantID:  claims["tenant_id"],
		UserID:    claims["user_id"],
		DeviceID:  claims["device_id"],
		SessionID: claims["session_id"],
		TraceID:   claims["trace_id"],
		Issuer:    firstNonEmpty(claims["iss"], "issuer-1"),
		Subject:   firstNonEmpty(claims["sub"], claims["user_id"]),
		Audience:  firstNonEmpty(claims["aud"], "push-gateway"),
		IssuedAt:  time.Now().Unix(),
		Expires:   expiresAt.Unix(),
	}
	payloadRaw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	signingInput := base64.RawURLEncoding.EncodeToString(headerRaw) + "." + base64.RawURLEncoding.EncodeToString(payloadRaw)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func requestWithBearer(token string) *http.Request {
	request := httptest.NewRequest("GET", "/ws?device_id=device-1", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	return request
}
