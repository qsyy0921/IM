package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http/httptest"
	"strings"
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
