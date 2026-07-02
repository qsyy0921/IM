package main

import (
	"context"
	"errors"
	"strings"
	"time"

	redisdecision "github.com/qsyy0921/IM/services/policy-service/internal/infrastructure/redisdecision"
	"github.com/redis/go-redis/v9"
)

type policyDecisionCacheRuntime struct {
	cache  *redisdecision.Cache
	client redis.UniversalClient
	ttl    time.Duration
}

type policyDecisionCacheConfig struct {
	Enabled   bool
	Mode      string
	Addr      string
	Username  string
	Password  string
	DB        int
	KeyPrefix string
	TTL       time.Duration
}

func policyDecisionCacheRuntimeFromEnv(ctx context.Context) (policyDecisionCacheRuntime, error) {
	config, err := policyDecisionCacheConfigFromEnv()
	if err != nil {
		return policyDecisionCacheRuntime{}, err
	}
	if !config.Enabled {
		return policyDecisionCacheRuntime{}, nil
	}
	client, err := newPolicyDecisionCacheRedisClient(config)
	if err != nil {
		return policyDecisionCacheRuntime{}, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return policyDecisionCacheRuntime{}, err
	}
	return policyDecisionCacheRuntime{
		cache:  redisdecision.NewCache(client, config.KeyPrefix),
		client: client,
		ttl:    config.TTL,
	}, nil
}

func policyDecisionCacheConfigFromEnv() (policyDecisionCacheConfig, error) {
	backend := strings.ToLower(strings.TrimSpace(envString("NEXUSIM_POLICY_DECISION_CACHE_BACKEND", "")))
	enabled := envBool("NEXUSIM_POLICY_DECISION_CACHE_ENABLED", false)
	if backend == "disabled" || backend == "none" {
		return policyDecisionCacheConfig{}, nil
	}
	if backend == "" {
		if !enabled {
			return policyDecisionCacheConfig{}, nil
		}
		backend = "redis"
	}
	if backend != "redis" {
		return policyDecisionCacheConfig{}, errors.New("NEXUSIM_POLICY_DECISION_CACHE_BACKEND must be redis, disabled, or empty")
	}
	mode := strings.ToLower(strings.TrimSpace(envString("NEXUSIM_POLICY_DECISION_CACHE_REDIS_MODE", "single")))
	if mode != "single" && mode != "redis" {
		return policyDecisionCacheConfig{}, errors.New("NEXUSIM_POLICY_DECISION_CACHE_REDIS_MODE must be single")
	}
	addr := envString("NEXUSIM_POLICY_DECISION_CACHE_REDIS_ADDR", "")
	if addr == "" {
		return policyDecisionCacheConfig{}, errors.New("NEXUSIM_POLICY_DECISION_CACHE_REDIS_ADDR is required when policy decision cache is enabled")
	}
	ttl, err := envPositiveDuration("NEXUSIM_POLICY_DECISION_CACHE_TTL", 30*time.Second)
	if err != nil {
		return policyDecisionCacheConfig{}, err
	}
	return policyDecisionCacheConfig{
		Enabled:   true,
		Mode:      "single",
		Addr:      addr,
		Username:  envString("NEXUSIM_POLICY_DECISION_CACHE_REDIS_USERNAME", ""),
		Password:  envString("NEXUSIM_POLICY_DECISION_CACHE_REDIS_PASSWORD", ""),
		DB:        envIntAllowZero("NEXUSIM_POLICY_DECISION_CACHE_REDIS_DB", 0),
		KeyPrefix: envString("NEXUSIM_POLICY_DECISION_CACHE_KEY_PREFIX", "nexusim:policy"),
		TTL:       ttl,
	}, nil
}

func newPolicyDecisionCacheRedisClient(config policyDecisionCacheConfig) (redis.UniversalClient, error) {
	if !config.Enabled {
		return nil, nil
	}
	return redis.NewClient(&redis.Options{
		Addr:     config.Addr,
		Username: config.Username,
		Password: config.Password,
		DB:       config.DB,
	}), nil
}
