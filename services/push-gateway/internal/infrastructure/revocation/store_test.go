package revocation

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
	"github.com/redis/go-redis/v9"
)

func TestMemoryStoreRevokesDeviceAndSession(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	auth := types.AuthContext{
		TenantID:  "tenant-1",
		UserID:    "user-1",
		DeviceID:  "device-1",
		SessionID: "session-1",
	}

	revoked, err := store.IsRevoked(ctx, auth)
	if err != nil {
		t.Fatalf("check revoked: %v", err)
	}
	if revoked {
		t.Fatalf("new store must not mark auth revoked")
	}
	if err := store.RevokeSession(ctx, auth.TenantID, auth.UserID, auth.DeviceID, auth.SessionID); err != nil {
		t.Fatalf("revoke session: %v", err)
	}
	revoked, err = store.IsRevoked(ctx, auth)
	if err != nil {
		t.Fatalf("check session revoked: %v", err)
	}
	if !revoked {
		t.Fatalf("expected session to be revoked")
	}

	otherSession := auth
	otherSession.SessionID = "session-2"
	revoked, err = store.IsRevoked(ctx, otherSession)
	if err != nil {
		t.Fatalf("check other session: %v", err)
	}
	if revoked {
		t.Fatalf("session revoke must not revoke other sessions")
	}
	if err := store.RevokeDevice(ctx, auth.TenantID, auth.UserID, auth.DeviceID); err != nil {
		t.Fatalf("revoke device: %v", err)
	}
	revoked, err = store.IsRevoked(ctx, otherSession)
	if err != nil {
		t.Fatalf("check device revoked: %v", err)
	}
	if !revoked {
		t.Fatalf("expected device to revoke all sessions")
	}
}

func TestRedisStoreSharesRevocation(t *testing.T) {
	ctx := context.Background()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()

	writer := NewRedisStore(client, "test:push")
	reader := NewRedisStore(client, "test:push")
	auth := types.AuthContext{
		TenantID:  "tenant-1",
		UserID:    "user-1",
		DeviceID:  "device-1",
		SessionID: "session-1",
	}

	if err := writer.RevokeDevice(ctx, auth.TenantID, auth.UserID, auth.DeviceID); err != nil {
		t.Fatalf("revoke device: %v", err)
	}
	revoked, err := reader.IsRevoked(ctx, auth)
	if err != nil {
		t.Fatalf("read revoked: %v", err)
	}
	if !revoked {
		t.Fatalf("expected redis revocation to be shared")
	}
}

func TestRecorderWritesStoreAndEvictsDevice(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	evicter := &fakeEvicter{}
	recorder := NewRecorder(store, evicter)

	if err := recorder.RevokeDevice(ctx, "tenant-1", "user-1", "device-1"); err != nil {
		t.Fatalf("revoke device: %v", err)
	}
	revoked, err := store.IsRevoked(ctx, types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"})
	if err != nil {
		t.Fatalf("check revoked: %v", err)
	}
	if !revoked {
		t.Fatalf("expected store to record device revocation")
	}
	if evicter.deviceCalls != 1 || evicter.reason != "identity_revoked" {
		t.Fatalf("unexpected evicter: %+v", evicter)
	}
}

func TestRecorderWritesStoreAndEvictsSession(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	evicter := &fakeEvicter{}
	recorder := NewRecorder(store, evicter)

	if err := recorder.RevokeSession(ctx, "tenant-1", "user-1", "device-1", "session-1"); err != nil {
		t.Fatalf("revoke session: %v", err)
	}
	revoked, err := store.IsRevoked(ctx, types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1", SessionID: "session-1"})
	if err != nil {
		t.Fatalf("check revoked: %v", err)
	}
	if !revoked {
		t.Fatalf("expected store to record session revocation")
	}
	if evicter.sessionCalls != 1 || evicter.sessionID != "session-1" || evicter.reason != "identity_revoked" {
		t.Fatalf("unexpected evicter: %+v", evicter)
	}
}

type fakeEvicter struct {
	deviceCalls  int
	sessionCalls int
	sessionID    string
	reason       string
}

func (evicter *fakeEvicter) EvictDevice(ctx context.Context, tenantID string, userID string, deviceID string, reason string) (types.SessionEvictionResult, error) {
	evicter.deviceCalls++
	evicter.reason = reason
	return types.SessionEvictionResult{MatchedSessions: 1, Evicted: 1}, nil
}

func (evicter *fakeEvicter) EvictSession(ctx context.Context, tenantID string, userID string, deviceID string, sessionID string, reason string) (types.SessionEvictionResult, error) {
	evicter.sessionCalls++
	evicter.sessionID = sessionID
	evicter.reason = reason
	return types.SessionEvictionResult{MatchedSessions: 1, Evicted: 1}, nil
}
