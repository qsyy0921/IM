package redisdecision

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
	"github.com/redis/go-redis/v9"
)

func TestCacheStoresAndLoadsMessageDecision(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewCache(client, "test:policy")
	ctx := context.Background()
	decision := types.MessageActionDecision{
		TenantID:          "tenant-cache",
		UserID:            "user-cache",
		ConversationID:    "conv-cache",
		Action:            types.MessageActionSend,
		Allowed:           true,
		PermissionVersion: 7,
		Classification:    "TENANT_ALLOW",
		DecisionSource:    types.PolicyDecisionSourceTenantRule,
	}

	if err := cache.SetMessageDecision(ctx, "key-1", decision, time.Minute); err != nil {
		t.Fatalf("set decision cache: %v", err)
	}
	cached, ok, err := cache.GetMessageDecision(ctx, "key-1")
	if err != nil {
		t.Fatalf("get decision cache: %v", err)
	}
	if !ok || !cached.Allowed || cached.PermissionVersion != 7 || cached.DecisionSource != types.PolicyDecisionSourceTenantRule {
		t.Fatalf("unexpected cached decision ok=%t decision=%+v", ok, cached)
	}
}

func TestCacheMissReturnsFalse(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewCache(client, "test:policy")

	_, ok, err := cache.GetMessageDecision(context.Background(), "missing")
	if err != nil || ok {
		t.Fatalf("expected cache miss without error, ok=%t err=%v", ok, err)
	}
}

func TestCacheRejectsNonCacheableDecisionSource(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewCache(client, "test:policy")
	decision := types.MessageActionDecision{
		TenantID:          "tenant-cache",
		UserID:            "user-cache",
		ConversationID:    "conv-cache",
		Action:            types.MessageActionSend,
		Allowed:           true,
		PermissionVersion: 7,
		Classification:    "STATIC_ALLOW",
		DecisionSource:    types.PolicyDecisionSourceStaticDefault,
	}

	if err := cache.SetMessageDecision(context.Background(), "key-static", decision, time.Minute); err == nil {
		t.Fatal("expected static default decision to be rejected by cache")
	}
}
