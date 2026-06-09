package redisroute

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
	"github.com/redis/go-redis/v9"
)

const defaultKeyPrefix = "nexusim:push"

type LocalRegistry interface {
	Register(context.Context, types.SessionRegistration) (types.SessionRegistrationResult, error)
	Unregister(sessionID string)
	EnqueueNotification(context.Context, types.DeliveryNotification) (types.NotifyDeliveryResult, error)
}

type Config struct {
	GatewayID string
	KeyPrefix string
	RouteTTL  time.Duration
}

type Registry struct {
	local  LocalRegistry
	client redis.UniversalClient
	config Config

	mu     sync.Mutex
	routes map[string]routeEntry
}

type routeEntry struct {
	TenantID  string `json:"tenant_id"`
	UserID    string `json:"user_id"`
	DeviceID  string `json:"device_id"`
	SessionID string `json:"session_id"`
	GatewayID string `json:"gateway_id"`
}

func NewRegistry(local LocalRegistry, client redis.UniversalClient, config Config) *Registry {
	if config.KeyPrefix == "" {
		config.KeyPrefix = defaultKeyPrefix
	}
	if config.RouteTTL <= 0 {
		config.RouteTTL = 90 * time.Second
	}
	return &Registry{
		local:  local,
		client: client,
		config: config,
		routes: make(map[string]routeEntry),
	}
}

func (registry *Registry) Register(
	ctx context.Context,
	registration types.SessionRegistration,
) (types.SessionRegistrationResult, error) {
	if registry.config.GatewayID == "" {
		return types.SessionRegistrationResult{}, types.NewInvalidFrame("gateway id is required")
	}
	result, err := registry.local.Register(ctx, registration)
	if err != nil {
		return types.SessionRegistrationResult{}, err
	}
	entry := routeEntry{
		TenantID:  registration.AuthContext.TenantID,
		UserID:    registration.AuthContext.UserID,
		DeviceID:  registration.AuthContext.DeviceID,
		SessionID: registration.SessionID,
		GatewayID: registry.config.GatewayID,
	}
	if err := registry.writeRoute(ctx, entry); err != nil {
		registry.local.Unregister(registration.SessionID)
		return types.SessionRegistrationResult{}, err
	}
	registry.mu.Lock()
	registry.routes[registration.SessionID] = entry
	registry.mu.Unlock()
	return result, nil
}

func (registry *Registry) Unregister(sessionID string) {
	registry.local.Unregister(sessionID)

	registry.mu.Lock()
	entry, ok := registry.routes[sessionID]
	if ok {
		delete(registry.routes, sessionID)
	}
	registry.mu.Unlock()
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = registry.deleteRoute(ctx, entry)
}

func (registry *Registry) EnqueueNotification(
	ctx context.Context,
	notification types.DeliveryNotification,
) (types.NotifyDeliveryResult, error) {
	localResult, err := registry.local.EnqueueNotification(ctx, notification)
	if err != nil {
		return localResult, err
	}
	routes, err := registry.lookupRoutes(ctx, notification.TenantID, notification.UserID)
	if err != nil {
		return localResult, err
	}
	result := localResult
	publishedGateways := make(map[string]struct{})
	for _, route := range routes {
		if route.GatewayID == "" || route.SessionID == "" {
			continue
		}
		if route.GatewayID == registry.config.GatewayID {
			continue
		}
		if _, ok := publishedGateways[route.GatewayID]; !ok {
			if err := registry.publishRemote(ctx, route.GatewayID, notification); err != nil {
				return result, err
			}
			publishedGateways[route.GatewayID] = struct{}{}
		}
		result.MatchedSessions++
		result.Enqueued++
	}
	return result, nil
}

func (registry *Registry) writeRoute(ctx context.Context, entry routeEntry) error {
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	sessionKey := registry.sessionKey(entry.SessionID)
	userKey := registry.userKey(entry.TenantID, entry.UserID)
	pipe := registry.client.TxPipeline()
	pipe.Set(ctx, sessionKey, payload, registry.config.RouteTTL)
	pipe.SAdd(ctx, userKey, entry.SessionID)
	pipe.Expire(ctx, userKey, registry.config.RouteTTL)
	_, err = pipe.Exec(ctx)
	return err
}

func (registry *Registry) deleteRoute(ctx context.Context, entry routeEntry) error {
	pipe := registry.client.TxPipeline()
	pipe.Del(ctx, registry.sessionKey(entry.SessionID))
	pipe.SRem(ctx, registry.userKey(entry.TenantID, entry.UserID), entry.SessionID)
	_, err := pipe.Exec(ctx)
	return err
}

func (registry *Registry) lookupRoutes(ctx context.Context, tenantID string, userID string) ([]routeEntry, error) {
	sessionIDs, err := registry.client.SMembers(ctx, registry.userKey(tenantID, userID)).Result()
	if err != nil {
		return nil, err
	}
	routes := make([]routeEntry, 0, len(sessionIDs))
	stale := make([]interface{}, 0)
	for _, sessionID := range sessionIDs {
		raw, err := registry.client.Get(ctx, registry.sessionKey(sessionID)).Result()
		if errors.Is(err, redis.Nil) {
			stale = append(stale, sessionID)
			continue
		}
		if err != nil {
			return nil, err
		}
		var entry routeEntry
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			stale = append(stale, sessionID)
			continue
		}
		if entry.TenantID != tenantID || entry.UserID != userID {
			stale = append(stale, sessionID)
			continue
		}
		routes = append(routes, entry)
	}
	if len(stale) > 0 {
		_ = registry.client.SRem(ctx, registry.userKey(tenantID, userID), stale...).Err()
	}
	return routes, nil
}

func (registry *Registry) publishRemote(
	ctx context.Context,
	gatewayID string,
	notification types.DeliveryNotification,
) error {
	payload, err := json.Marshal(notification)
	if err != nil {
		return err
	}
	return registry.client.Publish(ctx, registry.gatewayChannel(gatewayID), payload).Err()
}

func (registry *Registry) sessionKey(sessionID string) string {
	return strings.Join([]string{registry.config.KeyPrefix, "route", "session", sessionID}, ":")
}

func (registry *Registry) userKey(tenantID string, userID string) string {
	return strings.Join([]string{registry.config.KeyPrefix, "route", "user", tenantID, userID}, ":")
}

func (registry *Registry) gatewayChannel(gatewayID string) string {
	return GatewayChannel(registry.config.KeyPrefix, gatewayID)
}

func GatewayChannel(keyPrefix string, gatewayID string) string {
	if keyPrefix == "" {
		keyPrefix = defaultKeyPrefix
	}
	return strings.Join([]string{keyPrefix, "route", "gateway", gatewayID, "notify"}, ":")
}
