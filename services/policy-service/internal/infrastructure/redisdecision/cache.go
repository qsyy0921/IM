package redisdecision

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
	"github.com/redis/go-redis/v9"
)

type Cache struct {
	client redis.UniversalClient
	prefix string
}

type cachedMessageDecision struct {
	TenantID          string `json:"tenant_id"`
	UserID            string `json:"user_id"`
	ConversationID    string `json:"conversation_id"`
	MessageID         string `json:"message_id,omitempty"`
	Action            string `json:"action"`
	Allowed           bool   `json:"allowed"`
	PermissionVersion int64  `json:"permission_version"`
	Classification    string `json:"classification"`
	Reason            string `json:"reason,omitempty"`
	OwnershipOverride bool   `json:"ownership_override,omitempty"`
	DecisionSource    string `json:"decision_source"`
}

func NewCache(client redis.UniversalClient, prefix string) *Cache {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "nexusim:policy"
	}
	return &Cache{client: client, prefix: prefix}
}

func (cache *Cache) GetMessageDecision(
	ctx context.Context,
	key string,
) (types.MessageActionDecision, bool, error) {
	if cache == nil || cache.client == nil {
		return types.MessageActionDecision{}, false, errors.New("policy decision cache is not configured")
	}
	data, err := cache.client.Get(ctx, cache.redisKey(key)).Bytes()
	if errors.Is(err, redis.Nil) {
		return types.MessageActionDecision{}, false, nil
	}
	if err != nil {
		return types.MessageActionDecision{}, false, err
	}
	var cached cachedMessageDecision
	if err := json.Unmarshal(data, &cached); err != nil {
		return types.MessageActionDecision{}, false, err
	}
	decision := types.MessageActionDecision{
		TenantID:          types.TenantID(cached.TenantID),
		UserID:            types.UserID(cached.UserID),
		ConversationID:    types.ConversationID(cached.ConversationID),
		MessageID:         types.MessageID(cached.MessageID),
		Action:            types.MessageAction(cached.Action),
		Allowed:           cached.Allowed,
		PermissionVersion: cached.PermissionVersion,
		Classification:    strings.TrimSpace(cached.Classification),
		Reason:            strings.TrimSpace(cached.Reason),
		OwnershipOverride: cached.OwnershipOverride,
		DecisionSource:    types.PolicyDecisionSource(cached.DecisionSource),
	}
	if decision.TenantID == "" ||
		decision.UserID == "" ||
		decision.ConversationID == "" ||
		decision.Action == "" ||
		decision.PermissionVersion <= 0 ||
		decision.Classification == "" ||
		!cacheableMessageDecisionSource(decision.DecisionSource) {
		return types.MessageActionDecision{}, false, errors.New("policy decision cache payload is invalid")
	}
	return decision, true, nil
}

func (cache *Cache) SetMessageDecision(
	ctx context.Context,
	key string,
	decision types.MessageActionDecision,
	ttl time.Duration,
) error {
	if cache == nil || cache.client == nil {
		return errors.New("policy decision cache is not configured")
	}
	if ttl <= 0 {
		return nil
	}
	if decision.PermissionVersion <= 0 ||
		strings.TrimSpace(decision.Classification) == "" ||
		!cacheableMessageDecisionSource(decision.DecisionSource) {
		return errors.New("policy decision cache payload is invalid")
	}
	cached := cachedMessageDecision{
		TenantID:          string(decision.TenantID),
		UserID:            string(decision.UserID),
		ConversationID:    string(decision.ConversationID),
		MessageID:         string(decision.MessageID),
		Action:            string(decision.Action),
		Allowed:           decision.Allowed,
		PermissionVersion: decision.PermissionVersion,
		Classification:    strings.TrimSpace(decision.Classification),
		Reason:            strings.TrimSpace(decision.Reason),
		OwnershipOverride: decision.OwnershipOverride,
		DecisionSource:    string(decision.DecisionSource),
	}
	data, err := json.Marshal(cached)
	if err != nil {
		return err
	}
	return cache.client.Set(ctx, cache.redisKey(key), data, ttl).Err()
}

func (cache *Cache) redisKey(key string) string {
	return cache.prefix + ":decision:v1:" + strings.TrimSpace(key)
}

func cacheableMessageDecisionSource(source types.PolicyDecisionSource) bool {
	switch source {
	case types.PolicyDecisionSourceContactProjection,
		types.PolicyDecisionSourceConversationRole,
		types.PolicyDecisionSourceReBACRelation,
		types.PolicyDecisionSourceExactRule,
		types.PolicyDecisionSourceTenantRule:
		return true
	default:
		return false
	}
}
