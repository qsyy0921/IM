package auth

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
)

func TestAuthenticatorMapsExpiredToken(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	authenticator, err := NewAuthenticator(Config{
		Mode:     ModeHMAC,
		Secret:   "secret",
		Audience: "push-gateway",
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	token, err := SignGatewayToken("secret", map[string]string{
		"tenant_id": "tenant-1",
		"user_id":   "user-1",
		"device_id": "device-1",
	}, now.Add(-time.Second))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	_, err = authenticator.Authenticate(httptest.NewRequest("GET", "/ws?token="+token, nil))
	if !errors.Is(err, types.ErrAuthExpired) {
		t.Fatalf("expected push auth expired error, got %v", err)
	}
}

func TestAuthenticatorMapsRevocationToPermissionDenied(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	revocation := &fakeRevocationChecker{revoked: true}
	authenticator, err := NewAuthenticator(Config{
		Mode:       ModeHMAC,
		Secret:     "secret",
		Audience:   "push-gateway",
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

	_, err = authenticator.Authenticate(httptest.NewRequest("GET", "/ws?token="+token, nil))
	if !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected permission denied, got %v", err)
	}
	if revocation.auth.SessionID != "session-1" || revocation.auth.DeviceID != "device-1" {
		t.Fatalf("expected revocation adapter to receive push auth, got %+v", revocation.auth)
	}
}

func TestAuthenticatorMapsInvalidMockRequest(t *testing.T) {
	authenticator, err := NewAuthenticator(Config{Mode: ModeMock})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	_, err = authenticator.Authenticate(httptest.NewRequest("GET", "/ws", nil))
	if !errors.Is(err, types.ErrInvalidFrame) {
		t.Fatalf("expected invalid frame, got %v", err)
	}
}

func TestNilAuthenticatorKeepsMockRecovery(t *testing.T) {
	var authenticator *Authenticator
	auth, err := authenticator.Authenticate(httptest.NewRequest("GET", "/ws?token=tenant-1:user-1:device-1", nil))
	if err != nil {
		t.Fatalf("authenticate nil recovery: %v", err)
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

func (checker *fakeRevocationChecker) IsRevoked(_ context.Context, auth types.AuthContext) (bool, error) {
	checker.auth = auth
	return checker.revoked, checker.err
}
