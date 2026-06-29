package redisroute

import (
	"context"
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
	SubscribeConversation(context.Context, types.ConversationSubscriptionCommand) (types.ConversationSubscriptionResult, error)
	UnsubscribeConversation(context.Context, types.ConversationSubscriptionCommand) (types.ConversationSubscriptionResult, error)
	EnqueueConversationSignal(context.Context, types.DeliveryNotification) (types.NotifyDeliveryResult, error)
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

	mu            sync.Mutex
	routes        map[string]routeState
	subscriptions map[string]map[string]struct{}

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
		local:         local,
		client:        client,
		config:        config,
		routes:        make(map[string]routeState),
		subscriptions: make(map[string]map[string]struct{}),
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
	if registry.subscriptions[registration.SessionID] == nil {
		registry.subscriptions[registration.SessionID] = make(map[string]struct{})
	}
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
	conversations := registry.subscriptions[sessionID]
	delete(registry.subscriptions, sessionID)
	registry.mu.Unlock()
	if !ok {
		return
	}
	state.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = registry.deleteRoute(ctx, state.entry)
	for conversationID := range conversations {
		_ = registry.deleteConversationSubscription(ctx, state.entry, conversationID)
	}
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
