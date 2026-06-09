package redisroute

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
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
	routes map[string]routeState

	metrics registryMetrics
}

type Metrics struct {
	RedisRouteRegisterErrorCount       uint64 `json:"redis_route_register_error_count"`
	RedisRouteRenewErrorCount          uint64 `json:"redis_route_renew_error_count"`
	RedisRouteLookupErrorCount         uint64 `json:"redis_route_lookup_error_count"`
	RedisRouteRemoteMatchedSessions    uint64 `json:"redis_route_remote_matched_sessions"`
	RedisRouteRemotePublishCallCount   uint64 `json:"redis_route_remote_publish_call_count"`
	RedisRouteRemotePublishErrorCount  uint64 `json:"redis_route_remote_publish_error_count"`
	RedisRouteRemoteEnqueuedSessions   uint64 `json:"redis_route_remote_enqueued_sessions"`
	RedisRouteStaleRemovedCount        uint64 `json:"redis_route_stale_removed_count"`
	RedisRouteCleanupErrorCount        uint64 `json:"redis_route_cleanup_error_count"`
	RedisRouteSubscriberMessageCount   uint64 `json:"redis_route_subscriber_message_count,omitempty"`
	RedisRouteSubscriberMalformedCount uint64 `json:"redis_route_subscriber_malformed_count,omitempty"`
	RedisRouteSubscriberEnqueuedCount  uint64 `json:"redis_route_subscriber_enqueued_count,omitempty"`
	RedisRouteSubscriberErrorCount     uint64 `json:"redis_route_subscriber_error_count,omitempty"`
}

type registryMetrics struct {
	registerErrorCount      atomic.Uint64
	renewErrorCount         atomic.Uint64
	lookupErrorCount        atomic.Uint64
	remoteMatchedSessions   atomic.Uint64
	remotePublishCallCount  atomic.Uint64
	remotePublishErrorCount atomic.Uint64
	remoteEnqueuedSessions  atomic.Uint64
	staleRemovedCount       atomic.Uint64
	cleanupErrorCount       atomic.Uint64
}

type routeState struct {
	entry  routeEntry
	cancel context.CancelFunc
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
		routes: make(map[string]routeState),
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
		registry.metrics.registerErrorCount.Add(1)
		return types.SessionRegistrationResult{}, err
	}
	renewCtx, cancel := context.WithCancel(context.Background())
	registry.mu.Lock()
	registry.routes[registration.SessionID] = routeState{entry: entry, cancel: cancel}
	registry.mu.Unlock()
	go registry.renewRouteLoop(renewCtx, entry)
	return result, nil
}

func (registry *Registry) Unregister(sessionID string) {
	registry.local.Unregister(sessionID)

	registry.mu.Lock()
	state, ok := registry.routes[sessionID]
	if ok {
		delete(registry.routes, sessionID)
	}
	registry.mu.Unlock()
	if !ok {
		return
	}
	state.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = registry.deleteRoute(ctx, state.entry)
}

func (registry *Registry) StartCleanupLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cleanupCtx, cancel := context.WithTimeout(ctx, interval)
				_, err := registry.CleanupStaleRoutes(cleanupCtx)
				if err != nil {
					registry.metrics.cleanupErrorCount.Add(1)
				}
				cancel()
			}
		}
	}()
}

func (registry *Registry) CleanupStaleRoutes(ctx context.Context) (int, error) {
	pattern := registry.config.KeyPrefix + ":route:user:*"
	var cursor uint64
	totalRemoved := 0
	for {
		keys, nextCursor, err := registry.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return totalRemoved, err
		}
		for _, key := range keys {
			removed, err := registry.cleanupUserRouteKey(ctx, key)
			if err != nil {
				return totalRemoved, err
			}
			totalRemoved += removed
		}
		cursor = nextCursor
		if cursor == 0 {
			return totalRemoved, nil
		}
	}
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
		localResult.Dropped++
		registry.metrics.lookupErrorCount.Add(1)
		return localResult, nil
	}
	result := localResult
	publishedGateways := make(map[string]struct{})
	remoteSessionsByGateway := make(map[string]int)
	for _, route := range routes {
		if route.GatewayID == "" || route.SessionID == "" {
			continue
		}
		if route.GatewayID == registry.config.GatewayID {
			continue
		}
		remoteSessionsByGateway[route.GatewayID]++
		result.MatchedSessions++
	}
	var remoteMatched int
	for _, sessionCount := range remoteSessionsByGateway {
		remoteMatched += sessionCount
	}
	if remoteMatched > 0 {
		registry.metrics.remoteMatchedSessions.Add(uint64(remoteMatched))
	}
	for gatewayID, sessionCount := range remoteSessionsByGateway {
		if _, ok := publishedGateways[gatewayID]; ok {
			continue
		}
		registry.metrics.remotePublishCallCount.Add(1)
		if err := registry.publishRemote(ctx, gatewayID, notification); err != nil {
			result.Dropped += sessionCount
			registry.metrics.remotePublishErrorCount.Add(1)
			continue
		}
		publishedGateways[gatewayID] = struct{}{}
		result.Enqueued += sessionCount
		registry.metrics.remoteEnqueuedSessions.Add(uint64(sessionCount))
	}
	return result, nil
}

func (registry *Registry) Metrics() Metrics {
	return Metrics{
		RedisRouteRegisterErrorCount:      registry.metrics.registerErrorCount.Load(),
		RedisRouteRenewErrorCount:         registry.metrics.renewErrorCount.Load(),
		RedisRouteLookupErrorCount:        registry.metrics.lookupErrorCount.Load(),
		RedisRouteRemoteMatchedSessions:   registry.metrics.remoteMatchedSessions.Load(),
		RedisRouteRemotePublishCallCount:  registry.metrics.remotePublishCallCount.Load(),
		RedisRouteRemotePublishErrorCount: registry.metrics.remotePublishErrorCount.Load(),
		RedisRouteRemoteEnqueuedSessions:  registry.metrics.remoteEnqueuedSessions.Load(),
		RedisRouteStaleRemovedCount:       registry.metrics.staleRemovedCount.Load(),
		RedisRouteCleanupErrorCount:       registry.metrics.cleanupErrorCount.Load(),
	}
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

func (registry *Registry) renewRouteLoop(ctx context.Context, entry routeEntry) {
	interval := registry.config.RouteTTL / 3
	if interval <= 0 {
		interval = time.Second
	}
	if interval < 50*time.Millisecond {
		interval = 50 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshCtx, cancel := context.WithTimeout(ctx, time.Second)
			if err := registry.writeRoute(refreshCtx, entry); err != nil {
				registry.metrics.renewErrorCount.Add(1)
			}
			cancel()
		}
	}
}

func (registry *Registry) deleteRoute(ctx context.Context, entry routeEntry) error {
	pipe := registry.client.TxPipeline()
	pipe.Del(ctx, registry.sessionKey(entry.SessionID))
	pipe.SRem(ctx, registry.userKey(entry.TenantID, entry.UserID), entry.SessionID)
	_, err := pipe.Exec(ctx)
	return err
}

func (registry *Registry) cleanupUserRouteKey(ctx context.Context, userKey string) (int, error) {
	sessionIDs, err := registry.client.SMembers(ctx, userKey).Result()
	if err != nil {
		return 0, err
	}
	stale := make([]interface{}, 0)
	for _, sessionID := range sessionIDs {
		raw, err := registry.client.Get(ctx, registry.sessionKey(sessionID)).Result()
		if errors.Is(err, redis.Nil) {
			stale = append(stale, sessionID)
			continue
		}
		if err != nil {
			return 0, err
		}
		var entry routeEntry
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			stale = append(stale, sessionID)
			continue
		}
		if registry.userKey(entry.TenantID, entry.UserID) != userKey {
			stale = append(stale, sessionID)
		}
	}
	if len(stale) == 0 {
		return 0, nil
	}
	removed, err := registry.client.SRem(ctx, userKey, stale...).Result()
	if removed > 0 {
		registry.metrics.staleRemovedCount.Add(uint64(removed))
	}
	return int(removed), err
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
		removed, _ := registry.client.SRem(ctx, registry.userKey(tenantID, userID), stale...).Result()
		if removed > 0 {
			registry.metrics.staleRemovedCount.Add(uint64(removed))
		}
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
