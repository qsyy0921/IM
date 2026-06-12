package revocation

import (
	"context"
	"encoding/base64"
	"strings"
	"sync"

	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
	"github.com/redis/go-redis/v9"
)

type Store interface {
	IsRevoked(context.Context, types.AuthContext) (bool, error)
	RevokeDevice(ctx context.Context, tenantID string, userID string, deviceID string) error
	RevokeSession(ctx context.Context, tenantID string, userID string, deviceID string, sessionID string) error
}

type SessionEvicter interface {
	EvictDevice(ctx context.Context, tenantID string, userID string, deviceID string, reason string) (types.SessionEvictionResult, error)
	EvictSession(ctx context.Context, tenantID string, userID string, deviceID string, sessionID string, reason string) (types.SessionEvictionResult, error)
}

type Recorder struct {
	store   Store
	evicter SessionEvicter
}

func NewRecorder(store Store, evicter SessionEvicter) *Recorder {
	return &Recorder{store: store, evicter: evicter}
}

func (recorder *Recorder) RevokeDevice(ctx context.Context, tenantID string, userID string, deviceID string) error {
	if recorder == nil {
		return nil
	}
	if recorder.store != nil {
		if err := recorder.store.RevokeDevice(ctx, tenantID, userID, deviceID); err != nil {
			return err
		}
	}
	if recorder.evicter != nil {
		_, err := recorder.evicter.EvictDevice(ctx, tenantID, userID, deviceID, "identity_revoked")
		return err
	}
	return nil
}

func (recorder *Recorder) RevokeSession(ctx context.Context, tenantID string, userID string, deviceID string, sessionID string) error {
	if recorder == nil {
		return nil
	}
	if recorder.store != nil {
		if err := recorder.store.RevokeSession(ctx, tenantID, userID, deviceID, sessionID); err != nil {
			return err
		}
	}
	if recorder.evicter != nil {
		_, err := recorder.evicter.EvictSession(ctx, tenantID, userID, deviceID, sessionID, "identity_revoked")
		return err
	}
	return nil
}

type MemoryStore struct {
	mu       sync.RWMutex
	devices  map[string]struct{}
	sessions map[string]struct{}
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		devices:  make(map[string]struct{}),
		sessions: make(map[string]struct{}),
	}
}

func (store *MemoryStore) IsRevoked(ctx context.Context, auth types.AuthContext) (bool, error) {
	if store == nil {
		return false, nil
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if _, ok := store.devices[deviceKey(auth.TenantID, auth.UserID, auth.DeviceID)]; ok {
		return true, nil
	}
	if auth.SessionID != "" {
		if _, ok := store.sessions[sessionKey(auth.TenantID, auth.UserID, auth.DeviceID, auth.SessionID)]; ok {
			return true, nil
		}
	}
	return false, nil
}

func (store *MemoryStore) RevokeDevice(ctx context.Context, tenantID string, userID string, deviceID string) error {
	if store == nil {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.devices[deviceKey(tenantID, userID, deviceID)] = struct{}{}
	return nil
}

func (store *MemoryStore) RevokeSession(ctx context.Context, tenantID string, userID string, deviceID string, sessionID string) error {
	if store == nil {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.sessions[sessionKey(tenantID, userID, deviceID, sessionID)] = struct{}{}
	return nil
}

type RedisStore struct {
	client    redis.UniversalClient
	keyPrefix string
}

func NewRedisStore(client redis.UniversalClient, keyPrefix string) *RedisStore {
	keyPrefix = strings.TrimSpace(keyPrefix)
	if keyPrefix == "" {
		keyPrefix = "nexusim:push"
	}
	return &RedisStore{client: client, keyPrefix: keyPrefix}
}

func (store *RedisStore) IsRevoked(ctx context.Context, auth types.AuthContext) (bool, error) {
	if store == nil || store.client == nil {
		return false, nil
	}
	keys := []string{store.deviceRedisKey(auth.TenantID, auth.UserID, auth.DeviceID)}
	if auth.SessionID != "" {
		keys = append(keys, store.sessionRedisKey(auth.TenantID, auth.UserID, auth.DeviceID, auth.SessionID))
	}
	count, err := store.client.Exists(ctx, keys...).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (store *RedisStore) RevokeDevice(ctx context.Context, tenantID string, userID string, deviceID string) error {
	if store == nil || store.client == nil {
		return nil
	}
	return store.client.Set(ctx, store.deviceRedisKey(tenantID, userID, deviceID), "1", 0).Err()
}

func (store *RedisStore) RevokeSession(ctx context.Context, tenantID string, userID string, deviceID string, sessionID string) error {
	if store == nil || store.client == nil {
		return nil
	}
	return store.client.Set(ctx, store.sessionRedisKey(tenantID, userID, deviceID, sessionID), "1", 0).Err()
}

func (store *RedisStore) deviceRedisKey(tenantID string, userID string, deviceID string) string {
	return store.keyPrefix + ":identity:revoked:device:" + keyParts(tenantID, userID, deviceID)
}

func (store *RedisStore) sessionRedisKey(tenantID string, userID string, deviceID string, sessionID string) string {
	return store.keyPrefix + ":identity:revoked:session:" + keyParts(tenantID, userID, deviceID, sessionID)
}

func deviceKey(tenantID string, userID string, deviceID string) string {
	return tenantID + "\x1f" + userID + "\x1f" + deviceID
}

func sessionKey(tenantID string, userID string, deviceID string, sessionID string) string {
	return tenantID + "\x1f" + userID + "\x1f" + deviceID + "\x1f" + sessionID
}

func keyParts(values ...string) string {
	encoded := make([]string, 0, len(values))
	for _, value := range values {
		encoded = append(encoded, base64.RawURLEncoding.EncodeToString([]byte(value)))
	}
	return strings.Join(encoded, ":")
}
