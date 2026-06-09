package auth

import (
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
		"tenant_id": "tenant-1",
		"user_id":   "user-1",
		"device_id": "device-1",
		"trace_id":  "trace-1",
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	request := httptest.NewRequest("GET", "/ws?token="+token+"&device_id=device-1", nil)
	auth, err := authenticator.Authenticate(request)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if auth.TenantID != "tenant-1" || auth.UserID != "user-1" || auth.DeviceID != "device-1" || auth.TraceID != "trace-1" {
		t.Fatalf("unexpected auth: %+v", auth)
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
