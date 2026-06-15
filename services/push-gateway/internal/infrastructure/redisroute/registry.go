package redisroute

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qsyy0921/IM/services/push-gateway/internal/domain"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
	"github.com/redis/go-redis/v9"
)

const defaultKeyPrefix = "nexusim:push"

type LocalRegistry interface {
	Register(context.Context, types.SessionRegistration) (types.SessionRegistrationResult, error)
	Unregister(sessionID string)
	EnqueueNotification(context.Context, types.DeliveryNotification) (types.NotifyDeliveryResult, error)
	EvictDevice(ctx context.Context, tenantID string, userID string, deviceID string, reason string) (types.SessionEvictionResult, error)
	EvictSession(ctx context.Context, tenantID string, userID string, deviceID string, sessionID string, reason string) (types.SessionEvictionResult, error)
}

type Config struct {
	GatewayID             string
	KeyPrefix             string
	RouteTTL              time.Duration
	ResumeTTL             time.Duration
	RenewFailureThreshold int
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
	RedisRouteRenewSessionEvictedCount uint64 `json:"redis_route_renew_session_evicted_count"`
	RedisRouteLookupErrorCount         uint64 `json:"redis_route_lookup_error_count"`
	RedisRouteRemoteMatchedSessions    uint64 `json:"redis_route_remote_matched_sessions"`
	RedisRouteRemotePublishCallCount   uint64 `json:"redis_route_remote_publish_call_count"`
	RedisRouteRemotePublishErrorCount  uint64 `json:"redis_route_remote_publish_error_count"`
	RedisRouteRemoteNoSubscriberCount  uint64 `json:"redis_route_remote_no_subscriber_count"`
	RedisRouteRemoteEnqueuedSessions   uint64 `json:"redis_route_remote_enqueued_sessions"`
	RedisRouteStaleRemovedCount        uint64 `json:"redis_route_stale_removed_count"`
	RedisRouteCleanupErrorCount        uint64 `json:"redis_route_cleanup_error_count"`
	RedisRouteSubscriberMessageCount   uint64 `json:"redis_route_subscriber_message_count,omitempty"`
	RedisRouteSubscriberMalformedCount uint64 `json:"redis_route_subscriber_malformed_count,omitempty"`
	RedisRouteSubscriberEnqueuedCount  uint64 `json:"redis_route_subscriber_enqueued_count,omitempty"`
	RedisRouteSubscriberEvictedCount   uint64 `json:"redis_route_subscriber_evicted_count,omitempty"`
	RedisRouteSubscriberErrorCount     uint64 `json:"redis_route_subscriber_error_count,omitempty"`
	RedisResumeReplayCount             uint64 `json:"redis_resume_replay_count"`
	RedisResumeMissCount               uint64 `json:"redis_resume_miss_count"`
	RedisResumeAppendCount             uint64 `json:"redis_resume_append_count"`
	RedisResumeAppendErrorCount        uint64 `json:"redis_resume_append_error_count"`
	RedisResumePermissionDeniedCount   uint64 `json:"redis_resume_permission_denied_count"`
}

type registryMetrics struct {
	registerErrorCount      atomic.Uint64
	renewErrorCount         atomic.Uint64
	renewSessionEvicted     atomic.Uint64
	lookupErrorCount        atomic.Uint64
	remoteMatchedSessions   atomic.Uint64
	remotePublishCallCount  atomic.Uint64
	remotePublishErrorCount atomic.Uint64
	remoteNoSubscriberCount atomic.Uint64
	remoteEnqueuedSessions  atomic.Uint64
	staleRemovedCount       atomic.Uint64
	cleanupErrorCount       atomic.Uint64
	resumeReplayCount       atomic.Uint64
	resumeMissCount         atomic.Uint64
	resumeAppendCount       atomic.Uint64
	resumeAppendErrorCount  atomic.Uint64
	resumePermissionDenied  atomic.Uint64
}

type routeState struct {
	entry  routeEntry
	cancel context.CancelFunc
}

type routeEntry struct {
	TenantID    string `json:"tenant_id"`
	UserID      string `json:"user_id"`
	DeviceID    string `json:"device_id"`
	SessionID   string `json:"session_id"`
	GatewayID   string `json:"gateway_id"`
	ResumeToken string `json:"resume_token,omitempty"`
}

type evictionMessage struct {
	TenantID  string `json:"tenant_id"`
	UserID    string `json:"user_id"`
	DeviceID  string `json:"device_id"`
	SessionID string `json:"session_id,omitempty"`
	Reason    string `json:"reason"`
}

type resumeMeta struct {
	TenantID string `json:"tenant_id"`
	UserID   string `json:"user_id"`
	DeviceID string `json:"device_id"`
}

type redisResumeState struct {
	meta   resumeMeta
	frames []types.ServerFrame
}

func NewRegistry(local LocalRegistry, client redis.UniversalClient, config Config) *Registry {
	if config.KeyPrefix == "" {
		config.KeyPrefix = defaultKeyPrefix
	}
	if config.RouteTTL <= 0 {
		config.RouteTTL = 90 * time.Second
	}
	if config.ResumeTTL <= 0 {
		config.ResumeTTL = types.DefaultResumeBufferTTL
	}
	if config.RenewFailureThreshold < 0 {
		config.RenewFailureThreshold = 0
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
	redisResume, knownRedisToken, err := registry.loadRedisResume(ctx, registration.ResumeToken)
	if err != nil {
		registry.metrics.registerErrorCount.Add(1)
		return types.SessionRegistrationResult{}, err
	}
	replayFromRedis := knownRedisToken
	if knownRedisToken {
		if !sameDevice(redisResume.meta, registration.AuthContext) {
			registry.metrics.resumePermissionDenied.Add(1)
			return types.SessionRegistrationResult{}, types.ErrPermissionDenied
		}
		registration.ResumeRequested = false
	} else if registration.ResumeRequested {
		registry.metrics.resumeMissCount.Add(1)
	}

	result, err := registry.local.Register(ctx, registration)
	if err != nil {
		return types.SessionRegistrationResult{}, err
	}
	if result.ResumeToken == "" {
		result.ResumeToken = registration.ResumeToken
	}
	if err := registry.writeResumeMeta(ctx, result.ResumeToken, registration.AuthContext); err != nil {
		registry.local.Unregister(registration.SessionID)
		registry.metrics.registerErrorCount.Add(1)
		return types.SessionRegistrationResult{}, err
	}
	entry := routeEntry{
		TenantID:    registration.AuthContext.TenantID,
		UserID:      registration.AuthContext.UserID,
		DeviceID:    registration.AuthContext.DeviceID,
		SessionID:   registration.SessionID,
		GatewayID:   registry.config.GatewayID,
		ResumeToken: result.ResumeToken,
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
	if replayFromRedis && registry.replayRedisResume(registration, redisResume.frames) {
		registry.enqueueResumeHint(registration.Outbound)
		registry.metrics.resumeMissCount.Add(1)
	}
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
		if route.ResumeToken != "" {
			if err := registry.appendRedisResume(ctx, route.ResumeToken, domain.DeliveryNotify(notification)); err != nil {
				registry.metrics.resumeAppendErrorCount.Add(1)
			}
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
		subscriberCount, err := registry.publishRemote(ctx, gatewayID, notification)
		if err != nil {
			result.Dropped += sessionCount
			registry.metrics.remotePublishErrorCount.Add(1)
			continue
		}
		if subscriberCount == 0 {
			result.Dropped += sessionCount
			registry.metrics.remoteNoSubscriberCount.Add(uint64(sessionCount))
			continue
		}
		publishedGateways[gatewayID] = struct{}{}
		result.Enqueued += sessionCount
		registry.metrics.remoteEnqueuedSessions.Add(uint64(sessionCount))
	}
	return result, nil
}

func (registry *Registry) EvictDevice(ctx context.Context, tenantID string, userID string, deviceID string, reason string) (types.SessionEvictionResult, error) {
	localResult, err := registry.local.EvictDevice(ctx, tenantID, userID, deviceID, reason)
	if err != nil {
		return localResult, err
	}
	routes, err := registry.lookupRoutes(ctx, tenantID, userID)
	if err != nil {
		registry.metrics.lookupErrorCount.Add(1)
		return localResult, nil
	}
	result := localResult
	remoteSessionsByGateway := make(map[string]int)
	for _, route := range routes {
		if route.GatewayID == "" || route.GatewayID == registry.config.GatewayID || route.DeviceID != deviceID {
			continue
		}
		remoteSessionsByGateway[route.GatewayID]++
		result.MatchedSessions++
	}
	for gatewayID, sessionCount := range remoteSessionsByGateway {
		subscriberCount, err := registry.publishRemoteEviction(ctx, gatewayID, evictionMessage{
			TenantID: tenantID,
			UserID:   userID,
			DeviceID: deviceID,
			Reason:   firstNonEmpty(reason, "identity_revoked"),
		})
		if err != nil {
			registry.metrics.remotePublishErrorCount.Add(1)
			continue
		}
		if subscriberCount == 0 {
			registry.metrics.remoteNoSubscriberCount.Add(uint64(sessionCount))
			continue
		}
		result.Evicted += sessionCount
	}
	return result, nil
}

func (registry *Registry) EvictSession(ctx context.Context, tenantID string, userID string, deviceID string, sessionID string, reason string) (types.SessionEvictionResult, error) {
	localResult, err := registry.local.EvictSession(ctx, tenantID, userID, deviceID, sessionID, reason)
	if err != nil {
		return localResult, err
	}
	routes, err := registry.lookupRoutes(ctx, tenantID, userID)
	if err != nil {
		registry.metrics.lookupErrorCount.Add(1)
		return localResult, nil
	}
	result := localResult
	publishedGateways := make(map[string]struct{})
	for _, route := range routes {
		if route.GatewayID == "" ||
			route.GatewayID == registry.config.GatewayID ||
			route.DeviceID != deviceID ||
			route.SessionID != sessionID {
			continue
		}
		result.MatchedSessions++
		if _, ok := publishedGateways[route.GatewayID]; ok {
			continue
		}
		subscriberCount, err := registry.publishRemoteEviction(ctx, route.GatewayID, evictionMessage{
			TenantID:  tenantID,
			UserID:    userID,
			DeviceID:  deviceID,
			SessionID: sessionID,
			Reason:    firstNonEmpty(reason, "identity_revoked"),
		})
		if err != nil {
			registry.metrics.remotePublishErrorCount.Add(1)
			continue
		}
		if subscriberCount == 0 {
			registry.metrics.remoteNoSubscriberCount.Add(1)
			continue
		}
		publishedGateways[route.GatewayID] = struct{}{}
		result.Evicted++
	}
	return result, nil
}

func (registry *Registry) Metrics() Metrics {
	return Metrics{
		RedisRouteRegisterErrorCount:       registry.metrics.registerErrorCount.Load(),
		RedisRouteRenewErrorCount:          registry.metrics.renewErrorCount.Load(),
		RedisRouteRenewSessionEvictedCount: registry.metrics.renewSessionEvicted.Load(),
		RedisRouteLookupErrorCount:         registry.metrics.lookupErrorCount.Load(),
		RedisRouteRemoteMatchedSessions:    registry.metrics.remoteMatchedSessions.Load(),
		RedisRouteRemotePublishCallCount:   registry.metrics.remotePublishCallCount.Load(),
		RedisRouteRemotePublishErrorCount:  registry.metrics.remotePublishErrorCount.Load(),
		RedisRouteRemoteNoSubscriberCount:  registry.metrics.remoteNoSubscriberCount.Load(),
		RedisRouteRemoteEnqueuedSessions:   registry.metrics.remoteEnqueuedSessions.Load(),
		RedisRouteStaleRemovedCount:        registry.metrics.staleRemovedCount.Load(),
		RedisRouteCleanupErrorCount:        registry.metrics.cleanupErrorCount.Load(),
		RedisResumeReplayCount:             registry.metrics.resumeReplayCount.Load(),
		RedisResumeMissCount:               registry.metrics.resumeMissCount.Load(),
		RedisResumeAppendCount:             registry.metrics.resumeAppendCount.Load(),
		RedisResumeAppendErrorCount:        registry.metrics.resumeAppendErrorCount.Load(),
		RedisResumePermissionDeniedCount:   registry.metrics.resumePermissionDenied.Load(),
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
	consecutiveFailures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshCtx, cancel := context.WithTimeout(ctx, time.Second)
			tickFailed := false
			if err := registry.writeRoute(refreshCtx, entry); err != nil {
				registry.metrics.renewErrorCount.Add(1)
				tickFailed = true
			}
			if err := registry.writeResumeMeta(refreshCtx, entry.ResumeToken, types.AuthContext{
				TenantID: entry.TenantID,
				UserID:   entry.UserID,
				DeviceID: entry.DeviceID,
			}); err != nil {
				registry.metrics.renewErrorCount.Add(1)
				tickFailed = true
			}
			cancel()
			if tickFailed {
				consecutiveFailures++
				if registry.config.RenewFailureThreshold > 0 &&
					consecutiveFailures >= registry.config.RenewFailureThreshold {
					evictCtx, evictCancel := context.WithTimeout(context.Background(), time.Second)
					result, err := registry.local.EvictSession(
						evictCtx,
						entry.TenantID,
						entry.UserID,
						entry.DeviceID,
						entry.SessionID,
						"redis_route_unavailable",
					)
					evictCancel()
					if err == nil && result.Evicted > 0 {
						registry.metrics.renewSessionEvicted.Add(uint64(result.Evicted))
					}
					return
				}
				continue
			}
			consecutiveFailures = 0
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
) (int64, error) {
	payload, err := json.Marshal(notification)
	if err != nil {
		return 0, err
	}
	return registry.client.Publish(ctx, registry.gatewayChannel(gatewayID), payload).Result()
}

func (registry *Registry) publishRemoteEviction(
	ctx context.Context,
	gatewayID string,
	eviction evictionMessage,
) (int64, error) {
	payload, err := json.Marshal(eviction)
	if err != nil {
		return 0, err
	}
	return registry.client.Publish(ctx, registry.gatewayEvictionChannel(gatewayID), payload).Result()
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

func (registry *Registry) gatewayEvictionChannel(gatewayID string) string {
	return GatewayEvictionChannel(registry.config.KeyPrefix, gatewayID)
}

func (registry *Registry) writeResumeMeta(ctx context.Context, token string, auth types.AuthContext) error {
	if token == "" {
		return nil
	}
	payload, err := json.Marshal(resumeMeta{
		TenantID: auth.TenantID,
		UserID:   auth.UserID,
		DeviceID: auth.DeviceID,
	})
	if err != nil {
		return err
	}
	pipe := registry.client.TxPipeline()
	pipe.Set(ctx, registry.resumeMetaKey(token), payload, registry.config.ResumeTTL)
	pipe.Expire(ctx, registry.resumeFramesKey(token), registry.config.ResumeTTL)
	_, err = pipe.Exec(ctx)
	return err
}

func (registry *Registry) loadRedisResume(ctx context.Context, token string) (redisResumeState, bool, error) {
	if token == "" {
		return redisResumeState{}, false, nil
	}
	rawMeta, err := registry.client.Get(ctx, registry.resumeMetaKey(token)).Result()
	if errors.Is(err, redis.Nil) {
		return redisResumeState{}, false, nil
	}
	if err != nil {
		return redisResumeState{}, false, err
	}
	var meta resumeMeta
	if err := json.Unmarshal([]byte(rawMeta), &meta); err != nil {
		return redisResumeState{}, false, nil
	}
	rawFrames, err := registry.client.LRange(ctx, registry.resumeFramesKey(token), 0, -1).Result()
	if err != nil {
		return redisResumeState{}, false, err
	}
	frames := make([]types.ServerFrame, 0, len(rawFrames))
	for _, rawFrame := range rawFrames {
		var frame types.ServerFrame
		if err := json.Unmarshal([]byte(rawFrame), &frame); err != nil {
			return redisResumeState{meta: meta}, true, nil
		}
		if isResumeFrame(frame) {
			frames = append(frames, frame)
		}
	}
	return redisResumeState{meta: meta, frames: frames}, true, nil
}

func (registry *Registry) appendRedisResume(ctx context.Context, token string, frame types.ServerFrame) error {
	if token == "" || !isResumeFrame(frame) {
		return nil
	}
	payload, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	pipe := registry.client.TxPipeline()
	pipe.RPush(ctx, registry.resumeFramesKey(token), payload)
	pipe.LTrim(ctx, registry.resumeFramesKey(token), int64(-types.DefaultResumeBufferSize), -1)
	pipe.Expire(ctx, registry.resumeMetaKey(token), registry.config.ResumeTTL)
	pipe.Expire(ctx, registry.resumeFramesKey(token), registry.config.ResumeTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	registry.metrics.resumeAppendCount.Add(1)
	return nil
}

func (registry *Registry) replayRedisResume(
	registration types.SessionRegistration,
	frames []types.ServerFrame,
) bool {
	if len(frames) == 0 {
		return false
	}
	lastReceived := make(map[string]int64, len(registration.LastReceived))
	for _, cursor := range registration.LastReceived {
		if cursor.ConversationID == "" {
			continue
		}
		if cursor.Seq > lastReceived[cursor.ConversationID] {
			lastReceived[cursor.ConversationID] = cursor.Seq
		}
	}
	oldestByConversation := make(map[string]int64)
	for _, frame := range frames {
		if frame.Op != types.OpDeliveryNotify || frame.ConversationID == "" {
			continue
		}
		if oldestByConversation[frame.ConversationID] == 0 ||
			frame.ConversationSeq < oldestByConversation[frame.ConversationID] {
			oldestByConversation[frame.ConversationID] = frame.ConversationSeq
		}
	}
	for conversationID, seq := range lastReceived {
		oldest := oldestByConversation[conversationID]
		if oldest > 0 && seq+1 < oldest {
			return true
		}
	}
	replayFrames := make([]types.ServerFrame, 0, len(frames))
	for _, frame := range frames {
		if !isResumeFrame(frame) {
			continue
		}
		if frame.Op == types.OpDeliveryNotify && frame.ConversationID != "" && frame.ConversationSeq <= lastReceived[frame.ConversationID] {
			continue
		}
		replayFrames = append(replayFrames, frame)
	}
	if len(replayFrames) == 0 {
		return false
	}
	if cap(registration.Outbound)-len(registration.Outbound) < len(replayFrames) {
		return true
	}
	for _, frame := range replayFrames {
		select {
		case registration.Outbound <- frame:
			registry.metrics.resumeReplayCount.Add(1)
		default:
			return true
		}
	}
	return false
}

func isResumeFrame(frame types.ServerFrame) bool {
	return frame.Op == types.OpDeliveryNotify || frame.Op == types.OpDeliveryHide
}

func (registry *Registry) enqueueResumeHint(outbound chan<- types.ServerFrame) {
	select {
	case outbound <- domain.ResumeHint("buffer_miss", nil):
	default:
	}
}

func sameDevice(left resumeMeta, right types.AuthContext) bool {
	return left.TenantID == right.TenantID &&
		left.UserID == right.UserID &&
		left.DeviceID == right.DeviceID
}

func (registry *Registry) resumeMetaKey(token string) string {
	return strings.Join([]string{registry.config.KeyPrefix, "resume", "token", token, "meta"}, ":")
}

func (registry *Registry) resumeFramesKey(token string) string {
	return strings.Join([]string{registry.config.KeyPrefix, "resume", "token", token, "frames"}, ":")
}

func GatewayChannel(keyPrefix string, gatewayID string) string {
	if keyPrefix == "" {
		keyPrefix = defaultKeyPrefix
	}
	return strings.Join([]string{keyPrefix, "route", "gateway", gatewayID, "notify"}, ":")
}

func GatewayEvictionChannel(keyPrefix string, gatewayID string) string {
	if keyPrefix == "" {
		keyPrefix = defaultKeyPrefix
	}
	return strings.Join([]string{keyPrefix, "route", "gateway", gatewayID, "evict"}, ":")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
