package redisroute

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/push-gateway/internal/domain"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
	"github.com/redis/go-redis/v9"
)

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
	pattern := registry.config.KeyPrefix + ":route:*:user"
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

func (registry *Registry) SubscribeConversation(
	ctx context.Context,
	command types.ConversationSubscriptionCommand,
) (types.ConversationSubscriptionResult, error) {
	localResult, err := registry.local.SubscribeConversation(ctx, command)
	if err != nil {
		return localResult, err
	}
	registry.mu.Lock()
	state, ok := registry.routes[command.AuthContext.SessionID]
	if ok {
		if registry.subscriptions[command.AuthContext.SessionID] == nil {
			registry.subscriptions[command.AuthContext.SessionID] = make(map[string]struct{})
		}
		registry.subscriptions[command.AuthContext.SessionID][command.ConversationID] = struct{}{}
	}
	registry.mu.Unlock()
	if !ok {
		_, _ = registry.local.UnsubscribeConversation(ctx, command)
		return types.ConversationSubscriptionResult{}, types.ErrPermissionDenied
	}
	if err := registry.writeConversationSubscription(ctx, state.entry, command.ConversationID); err != nil {
		_, _ = registry.local.UnsubscribeConversation(ctx, command)
		registry.mu.Lock()
		delete(registry.subscriptions[command.AuthContext.SessionID], command.ConversationID)
		registry.mu.Unlock()
		registry.metrics.registerErrorCount.Add(1)
		return types.ConversationSubscriptionResult{}, err
	}
	return localResult, nil
}

func (registry *Registry) UnsubscribeConversation(
	ctx context.Context,
	command types.ConversationSubscriptionCommand,
) (types.ConversationSubscriptionResult, error) {
	localResult, err := registry.local.UnsubscribeConversation(ctx, command)
	if err != nil {
		return localResult, err
	}
	registry.mu.Lock()
	state, ok := registry.routes[command.AuthContext.SessionID]
	if ok {
		delete(registry.subscriptions[command.AuthContext.SessionID], command.ConversationID)
	}
	registry.mu.Unlock()
	if !ok {
		return localResult, nil
	}
	if err := registry.deleteConversationSubscription(ctx, state.entry, command.ConversationID); err != nil {
		registry.metrics.cleanupErrorCount.Add(1)
		return localResult, err
	}
	return localResult, nil
}

func (registry *Registry) EnqueueConversationSignal(
	ctx context.Context,
	notification types.DeliveryNotification,
) (types.NotifyDeliveryResult, error) {
	notification.Kind = types.DeliveryNotificationKindConversationSignal
	if err := notification.Validate(); err != nil {
		return types.NotifyDeliveryResult{}, err
	}
	if !registry.shouldEmitConversationSignal(notification) {
		registry.metrics.conversationSignalSuppressedEventCount.Add(1)
		return types.NotifyDeliveryResult{}, nil
	}
	localResult, err := registry.local.EnqueueConversationSignal(ctx, notification)
	if err != nil {
		return localResult, err
	}
	routes, err := registry.lookupConversationRoutes(ctx, notification.TenantID, notification.ConversationID)
	if err != nil {
		localResult.Dropped++
		registry.metrics.lookupErrorCount.Add(1)
		return localResult, nil
	}
	result := localResult
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
		result.Enqueued += sessionCount
		registry.metrics.remoteEnqueuedSessions.Add(uint64(sessionCount))
	}
	return result, nil
}

func (registry *Registry) shouldEmitConversationSignal(notification types.DeliveryNotification) bool {
	return registry.config.ConversationSignalPolicy.ShouldEmit(notification.ConversationSeq, notification.FanoutMode)
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

func (registry *Registry) writeRoute(ctx context.Context, entry routeEntry) error {
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	userKey := registry.userKey(entry.TenantID, entry.UserID)
	sessionKey := registry.sessionKey(entry.TenantID, entry.UserID, entry.SessionID)
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
			if err := registry.renewConversationSubscriptions(refreshCtx, entry); err != nil {
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
	pipe.Del(ctx, registry.sessionKey(entry.TenantID, entry.UserID, entry.SessionID))
	pipe.SRem(ctx, registry.userKey(entry.TenantID, entry.UserID), entry.SessionID)
	_, err := pipe.Exec(ctx)
	return err
}

type conversationRouteRef struct {
	TenantID  string `json:"tenant_id"`
	UserID    string `json:"user_id"`
	SessionID string `json:"session_id"`
}

func (registry *Registry) writeConversationSubscription(ctx context.Context, entry routeEntry, conversationID string) error {
	payload, err := json.Marshal(conversationRouteRef{
		TenantID:  entry.TenantID,
		UserID:    entry.UserID,
		SessionID: entry.SessionID,
	})
	if err != nil {
		return err
	}
	pipe := registry.client.TxPipeline()
	key := registry.conversationKey(entry.TenantID, conversationID)
	pipe.SAdd(ctx, key, string(payload))
	pipe.Expire(ctx, key, registry.config.RouteTTL)
	_, err = pipe.Exec(ctx)
	return err
}

func (registry *Registry) deleteConversationSubscription(ctx context.Context, entry routeEntry, conversationID string) error {
	payload, err := json.Marshal(conversationRouteRef{
		TenantID:  entry.TenantID,
		UserID:    entry.UserID,
		SessionID: entry.SessionID,
	})
	if err != nil {
		return err
	}
	_, err = registry.client.SRem(ctx, registry.conversationKey(entry.TenantID, conversationID), string(payload)).Result()
	return err
}

func (registry *Registry) renewConversationSubscriptions(ctx context.Context, entry routeEntry) error {
	registry.mu.Lock()
	conversations := make([]string, 0, len(registry.subscriptions[entry.SessionID]))
	for conversationID := range registry.subscriptions[entry.SessionID] {
		conversations = append(conversations, conversationID)
	}
	registry.mu.Unlock()
	for _, conversationID := range conversations {
		if err := registry.writeConversationSubscription(ctx, entry, conversationID); err != nil {
			return err
		}
	}
	return nil
}

func (registry *Registry) cleanupUserRouteKey(ctx context.Context, userKey string) (int, error) {
	sessionIDs, err := registry.client.SMembers(ctx, userKey).Result()
	if err != nil {
		return 0, err
	}
	stale := make([]interface{}, 0)
	for _, sessionID := range sessionIDs {
		raw, err := registry.client.Get(ctx, registry.sessionKeyForUserKey(userKey, sessionID)).Result()
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

func (registry *Registry) lookupConversationRoutes(ctx context.Context, tenantID string, conversationID string) ([]routeEntry, error) {
	key := registry.conversationKey(tenantID, conversationID)
	rawRefs, err := registry.client.SMembers(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	routes := make([]routeEntry, 0, len(rawRefs))
	stale := make([]interface{}, 0)
	for _, rawRef := range rawRefs {
		var ref conversationRouteRef
		if err := json.Unmarshal([]byte(rawRef), &ref); err != nil {
			stale = append(stale, rawRef)
			continue
		}
		if ref.TenantID != tenantID || ref.UserID == "" || ref.SessionID == "" {
			stale = append(stale, rawRef)
			continue
		}
		raw, err := registry.client.Get(ctx, registry.sessionKey(ref.TenantID, ref.UserID, ref.SessionID)).Result()
		if errors.Is(err, redis.Nil) {
			stale = append(stale, rawRef)
			continue
		}
		if err != nil {
			return nil, err
		}
		var entry routeEntry
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			stale = append(stale, rawRef)
			continue
		}
		if entry.TenantID != ref.TenantID || entry.UserID != ref.UserID || entry.SessionID != ref.SessionID {
			stale = append(stale, rawRef)
			continue
		}
		routes = append(routes, entry)
	}
	if len(stale) > 0 {
		removed, _ := registry.client.SRem(ctx, key, stale...).Result()
		if removed > 0 {
			registry.metrics.staleRemovedCount.Add(uint64(removed))
		}
	}
	return routes, nil
}

func (registry *Registry) lookupRoutes(ctx context.Context, tenantID string, userID string) ([]routeEntry, error) {
	sessionIDs, err := registry.client.SMembers(ctx, registry.userKey(tenantID, userID)).Result()
	if err != nil {
		return nil, err
	}
	routes := make([]routeEntry, 0, len(sessionIDs))
	stale := make([]interface{}, 0)
	for _, sessionID := range sessionIDs {
		raw, err := registry.client.Get(ctx, registry.sessionKey(tenantID, userID, sessionID)).Result()
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

func (registry *Registry) sessionKey(tenantID string, userID string, sessionID string) string {
	return strings.Join([]string{registry.config.KeyPrefix, "route", registry.userRouteHashTag(tenantID, userID), "session", sessionID}, ":")
}

func (registry *Registry) sessionKeyForUserKey(userKey string, sessionID string) string {
	prefix := strings.TrimSuffix(userKey, ":user")
	if prefix == userKey {
		return strings.Join([]string{registry.config.KeyPrefix, "route", "{user:unknown}", "session", sessionID}, ":")
	}
	return strings.Join([]string{prefix, "session", sessionID}, ":")
}

func (registry *Registry) userKey(tenantID string, userID string) string {
	return strings.Join([]string{registry.config.KeyPrefix, "route", registry.userRouteHashTag(tenantID, userID), "user"}, ":")
}

func (registry *Registry) conversationKey(tenantID string, conversationID string) string {
	return strings.Join([]string{registry.config.KeyPrefix, "route", registry.conversationRouteHashTag(tenantID, conversationID), "conversation"}, ":")
}

func (registry *Registry) userRouteHashTag(tenantID string, userID string) string {
	return "{user:" + redisKeyPart(tenantID) + ":" + redisKeyPart(userID) + "}"
}

func (registry *Registry) conversationRouteHashTag(tenantID string, conversationID string) string {
	return "{conversation:" + redisKeyPart(tenantID) + ":" + redisKeyPart(conversationID) + "}"
}

func (registry *Registry) gatewayChannel(gatewayID string) string {
	return GatewayChannel(registry.config.KeyPrefix, gatewayID)
}

func (registry *Registry) gatewayEvictionChannel(gatewayID string) string {
	return GatewayEvictionChannel(registry.config.KeyPrefix, gatewayID)
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

func redisKeyPart(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}
