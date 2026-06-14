package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	metadataToken = "x-nexusim-gateway-token"
	maxKeyLength  = 96
	backendLocal  = "local"
	backendRedis  = "redis"
	scopeToken    = "token"
	scopeTenant   = "tenant"
)

type Config struct {
	Enabled                     bool
	Backend                     string
	KeyScope                    string
	RequestsPerSecond           float64
	Burst                       int
	TenantPlans                 map[string]Plan
	TenantPlanSource            string
	TenantPlanVersion           string
	TenantPlanGeneratedAtUnixMS int64
	TenantPlanChecksumPresent   bool
	MaxKeys                     int
	RedisClient                 redis.UniversalClient
	RedisKeyPrefix              string
	RedisWindow                 time.Duration
	RedisFailOpen               bool
	IdentityFunc                IdentityFunc
	Now                         func() time.Time
}

type Identity struct {
	TenantID string
	UserID   string
}

type IdentityFunc func(context.Context) (Identity, error)

type Plan struct {
	RequestsPerSecond float64
	Burst             int
}

type Limiter struct {
	enabled           bool
	backend           string
	scope             string
	rate              float64
	burst             float64
	source            string
	planVersion       string
	planGeneratedAtMS int64
	planChecksum      bool
	plansMu           sync.RWMutex
	plans             map[string]quotaPlan
	maxKeys           int
	redis             redis.UniversalClient
	prefix            string
	window            time.Duration
	failOpen          bool
	identity          IdentityFunc
	now               func() time.Time

	mu             sync.Mutex
	buckets        map[string]*bucket
	totalLimited   int64
	totalAccepted  int64
	redisErrors    atomic.Int64
	identityErrors atomic.Int64
	planReloads    atomic.Int64
	planReloadAtMS atomic.Int64
	planReloadErrs atomic.Int64
}

type bucket struct {
	tokens   float64
	lastSeen time.Time
	limited  int64
	accepted int64
}

type quotaPlan struct {
	rate  float64
	burst float64
}

type requestQuota struct {
	key   string
	rate  float64
	burst float64
}

type Snapshot struct {
	Enabled                   bool    `json:"enabled"`
	Backend                   string  `json:"backend,omitempty"`
	KeyScope                  string  `json:"key_scope,omitempty"`
	RatePerSecond             float64 `json:"rate_per_second,omitempty"`
	Burst                     int     `json:"burst,omitempty"`
	TenantPlans               int     `json:"tenant_plan_count,omitempty"`
	TenantPlanSource          string  `json:"tenant_plan_source,omitempty"`
	TenantPlanVersion         string  `json:"tenant_plan_version,omitempty"`
	TenantPlanGeneratedAt     int64   `json:"tenant_plan_generated_at_unix_ms,omitempty"`
	TenantPlanChecksumPresent bool    `json:"tenant_plan_checksum_present,omitempty"`
	TenantReloads             int64   `json:"tenant_plan_reload_count,omitempty"`
	TenantReloadAt            int64   `json:"tenant_plan_reloaded_at_unix_ms,omitempty"`
	TenantErrors              int64   `json:"tenant_plan_reload_error_count,omitempty"`
	TrackedKeys               int     `json:"tracked_keys,omitempty"`
	MaxKeys                   int     `json:"max_keys,omitempty"`
	RedisWindowMS             int64   `json:"redis_window_ms,omitempty"`
	RedisFailOpen             bool    `json:"redis_fail_open,omitempty"`
	RedisErrors               int64   `json:"redis_error_count,omitempty"`
	IdentityErrors            int64   `json:"identity_error_count,omitempty"`
	TotalAccepted             int64   `json:"total_accepted"`
	TotalLimited              int64   `json:"total_limited"`
}

func New(config Config) (*Limiter, error) {
	backend := strings.ToLower(strings.TrimSpace(config.Backend))
	if backend == "" {
		backend = backendLocal
	}
	scope := strings.ToLower(strings.TrimSpace(config.KeyScope))
	if scope == "" {
		scope = scopeToken
	}
	limiter := &Limiter{
		enabled:           config.Enabled,
		backend:           backend,
		scope:             scope,
		rate:              config.RequestsPerSecond,
		burst:             float64(config.Burst),
		source:            strings.TrimSpace(config.TenantPlanSource),
		planVersion:       strings.TrimSpace(config.TenantPlanVersion),
		planGeneratedAtMS: config.TenantPlanGeneratedAtUnixMS,
		planChecksum:      config.TenantPlanChecksumPresent,
		maxKeys:           config.MaxKeys,
		redis:             config.RedisClient,
		prefix:            strings.Trim(strings.TrimSpace(config.RedisKeyPrefix), ":"),
		window:            config.RedisWindow,
		failOpen:          config.RedisFailOpen,
		identity:          config.IdentityFunc,
		now:               config.Now,
		buckets:           make(map[string]*bucket),
	}
	if limiter.now == nil {
		limiter.now = time.Now
	}
	if !limiter.enabled {
		return limiter, nil
	}
	if limiter.source == "" {
		limiter.source = "none"
	}
	if limiter.rate <= 0 {
		return nil, errors.New("api-gateway rate limit rps must be greater than 0 when enabled")
	}
	switch limiter.scope {
	case scopeToken, scopeTenant:
	default:
		return nil, errors.New("unsupported api-gateway rate limit scope")
	}
	if limiter.scope == scopeTenant && limiter.identity == nil {
		return nil, errors.New("api-gateway tenant rate limit scope requires an identity resolver")
	}
	switch limiter.backend {
	case backendLocal:
	case backendRedis:
		if limiter.redis == nil {
			return nil, errors.New("api-gateway redis rate limit requires a Redis client")
		}
		if limiter.prefix == "" {
			limiter.prefix = "nexusim:api-gateway"
		}
		if limiter.window <= 0 {
			limiter.window = time.Second
		}
		if limiter.window < time.Millisecond {
			limiter.window = time.Millisecond
		}
	default:
		return nil, errors.New("unsupported api-gateway rate limit backend")
	}
	if limiter.burst <= 0 {
		limiter.burst = math.Ceil(limiter.rate)
	}
	plans, err := normalizeTenantPlans(config.TenantPlans)
	if err != nil {
		return nil, err
	}
	limiter.plans = plans
	if limiter.maxKeys <= 0 {
		limiter.maxKeys = 10000
	}
	return limiter, nil
}

func (limiter *Limiter) UnaryServerInterceptor() grpcgo.UnaryServerInterceptor {
	if limiter == nil || !limiter.enabled {
		return func(ctx context.Context, request any, info *grpcgo.UnaryServerInfo, handler grpcgo.UnaryHandler) (any, error) {
			return handler(ctx, request)
		}
	}
	return func(ctx context.Context, request any, info *grpcgo.UnaryServerInfo, handler grpcgo.UnaryHandler) (any, error) {
		allowed, retryDelay, err := limiter.allow(ctx, info.FullMethod)
		if err != nil && !limiter.failOpen {
			return nil, status.Error(codes.Unavailable, "rate limiter unavailable")
		}
		if !allowed {
			return nil, rateLimitExceededError(retryDelay)
		}
		return handler(ctx, request)
	}
}

func (limiter *Limiter) Snapshot() Snapshot {
	if limiter == nil {
		return Snapshot{}
	}
	limiter.mu.Lock()
	trackedKeys := len(limiter.buckets)
	totalAccepted := limiter.totalAccepted
	totalLimited := limiter.totalLimited
	limiter.mu.Unlock()

	limiter.plansMu.RLock()
	tenantPlanCount := len(limiter.plans)
	tenantPlanVersion := limiter.planVersion
	tenantPlanGeneratedAtMS := limiter.planGeneratedAtMS
	tenantPlanChecksumPresent := limiter.planChecksum
	limiter.plansMu.RUnlock()

	return Snapshot{
		Enabled:                   limiter.enabled,
		Backend:                   limiter.backend,
		KeyScope:                  limiter.scope,
		RatePerSecond:             limiter.rate,
		Burst:                     int(limiter.burst),
		TenantPlans:               tenantPlanCount,
		TenantPlanSource:          limiter.source,
		TenantPlanVersion:         tenantPlanVersion,
		TenantPlanGeneratedAt:     tenantPlanGeneratedAtMS,
		TenantPlanChecksumPresent: tenantPlanChecksumPresent,
		TenantReloads:             limiter.planReloads.Load(),
		TenantReloadAt:            limiter.planReloadAtMS.Load(),
		TenantErrors:              limiter.planReloadErrs.Load(),
		TrackedKeys:               trackedKeys,
		MaxKeys:                   limiter.maxKeys,
		RedisWindowMS:             limiter.window.Milliseconds(),
		RedisFailOpen:             limiter.failOpen,
		RedisErrors:               limiter.redisErrors.Load(),
		IdentityErrors:            limiter.identityErrors.Load(),
		TotalAccepted:             totalAccepted,
		TotalLimited:              totalLimited,
	}
}

func (limiter *Limiter) UpdateTenantPlans(plans map[string]Plan) error {
	return limiter.UpdateTenantPlanSnapshot(plans, "", 0, false)
}

func (limiter *Limiter) UpdateTenantPlanSnapshot(plans map[string]Plan, version string, generatedAtUnixMS int64, checksumPresent bool) error {
	if limiter == nil {
		return nil
	}
	normalized, err := normalizeTenantPlans(plans)
	if err != nil {
		limiter.planReloadErrs.Add(1)
		return err
	}
	limiter.plansMu.Lock()
	limiter.plans = normalized
	limiter.planVersion = strings.TrimSpace(version)
	limiter.planGeneratedAtMS = generatedAtUnixMS
	limiter.planChecksum = checksumPresent
	limiter.plansMu.Unlock()
	limiter.planReloads.Add(1)
	now := time.Now
	if limiter.now != nil {
		now = limiter.now
	}
	limiter.planReloadAtMS.Store(now().UnixMilli())
	return nil
}

func (limiter *Limiter) RecordTenantPlanReloadError() {
	if limiter == nil {
		return
	}
	limiter.planReloadErrs.Add(1)
}

func (limiter *Limiter) allow(ctx context.Context, method string) (bool, time.Duration, error) {
	if limiter.backend == backendRedis {
		return limiter.allowRedis(ctx, method)
	}
	allowed, retryDelay := limiter.allowLocal(ctx, method)
	return allowed, retryDelay, nil
}

func (limiter *Limiter) allowLocal(ctx context.Context, method string) (bool, time.Duration) {
	now := limiter.now()
	quota := limiter.requestQuota(ctx, method)

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	entry := limiter.buckets[quota.key]
	if entry == nil {
		if len(limiter.buckets) >= limiter.maxKeys {
			limiter.evictOldestLocked()
		}
		entry = &bucket{tokens: quota.burst, lastSeen: now}
		limiter.buckets[quota.key] = entry
	}
	elapsed := now.Sub(entry.lastSeen).Seconds()
	if elapsed > 0 {
		entry.tokens = math.Min(quota.burst, entry.tokens+elapsed*quota.rate)
	}
	if entry.tokens > quota.burst {
		entry.tokens = quota.burst
	}
	entry.lastSeen = now
	if entry.tokens < 1 {
		retryDelay := localRetryDelay(1-entry.tokens, quota.rate)
		entry.limited++
		limiter.totalLimited++
		return false, retryDelay
	}
	entry.tokens--
	entry.accepted++
	limiter.totalAccepted++
	return true, 0
}

func (limiter *Limiter) allowRedis(ctx context.Context, method string) (bool, time.Duration, error) {
	now := limiter.now()
	window := limiter.window
	if window <= 0 {
		window = time.Second
	}
	if window < time.Millisecond {
		window = time.Millisecond
	}
	windowID := now.UnixMilli() / window.Milliseconds()
	quota := limiter.requestQuota(ctx, method)
	key := limiter.redisKey(quota.key, windowID)
	count, err := limiter.redis.Incr(ctx, key).Result()
	if err != nil {
		limiter.redisErrors.Add(1)
		if limiter.failOpen {
			limiter.recordAccepted()
			return true, 0, nil
		}
		return false, 0, err
	}
	if count == 1 {
		if err := limiter.redis.PExpire(ctx, key, window*2).Err(); err != nil {
			limiter.redisErrors.Add(1)
			if limiter.failOpen {
				limiter.recordAccepted()
				return true, 0, nil
			}
			return false, 0, err
		}
	}
	if count > int64(quota.burst) {
		limiter.recordLimited()
		return false, redisRetryDelay(now, window), nil
	}
	limiter.recordAccepted()
	return true, 0, nil
}

func (limiter *Limiter) redisKey(rawKey string, windowID int64) string {
	digest := sha256.Sum256([]byte(rawKey))
	return limiter.prefix + ":rate:" + hex.EncodeToString(digest[:12]) + ":" + strconv.FormatInt(windowID, 10)
}

func localRetryDelay(missingTokens float64, rate float64) time.Duration {
	if missingTokens <= 0 || rate <= 0 {
		return time.Second
	}
	delay := time.Duration(math.Ceil((missingTokens / rate) * float64(time.Second)))
	if delay < time.Millisecond {
		return time.Millisecond
	}
	return delay
}

func redisRetryDelay(now time.Time, window time.Duration) time.Duration {
	if window <= 0 {
		return time.Second
	}
	windowMillis := window.Milliseconds()
	if windowMillis <= 0 {
		return time.Millisecond
	}
	nextWindowMillis := ((now.UnixMilli() / windowMillis) + 1) * windowMillis
	delay := time.Duration(nextWindowMillis-now.UnixMilli()) * time.Millisecond
	if delay < time.Millisecond {
		return time.Millisecond
	}
	return delay
}

func rateLimitExceededError(retryDelay time.Duration) error {
	if retryDelay <= 0 {
		retryDelay = time.Second
	}
	st := status.New(codes.ResourceExhausted, "rate limit exceeded")
	st, err := st.WithDetails(&errdetails.RetryInfo{RetryDelay: durationpb.New(retryDelay)})
	if err != nil {
		return status.Error(codes.ResourceExhausted, "rate limit exceeded")
	}
	return st.Err()
}

func (limiter *Limiter) recordAccepted() {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	limiter.totalAccepted++
}

func (limiter *Limiter) recordLimited() {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	limiter.totalLimited++
}

func (limiter *Limiter) evictOldestLocked() {
	var oldestKey string
	var oldestTime time.Time
	for key, entry := range limiter.buckets {
		if oldestKey == "" || entry.lastSeen.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.lastSeen
		}
	}
	if oldestKey != "" {
		delete(limiter.buckets, oldestKey)
	}
}

func (limiter *Limiter) requestQuota(ctx context.Context, method string) requestQuota {
	method = trimKeyPart(method)
	if limiter != nil && limiter.scope == scopeTenant {
		identity, err := limiter.identity(ctx)
		if err == nil && strings.TrimSpace(identity.TenantID) != "" {
			tenantID := strings.TrimSpace(identity.TenantID)
			plan := limiter.planForTenant(tenantID)
			return requestQuota{key: method + "|tenant:" + trimKeyPart(tenantID), rate: plan.rate, burst: plan.burst}
		}
		limiter.identityErrors.Add(1)
	}
	if token := bearerOrGatewayToken(ctx); token != "" {
		digest := sha256.Sum256([]byte(token))
		return limiter.defaultQuota(method + "|token:" + hex.EncodeToString(digest[:12]))
	}
	if peerInfo, ok := peer.FromContext(ctx); ok && peerInfo.Addr != nil {
		return limiter.defaultQuota(method + "|peer:" + trimKeyPart(peerInfo.Addr.String()))
	}
	return limiter.defaultQuota(method + "|unknown")
}

func (limiter *Limiter) defaultQuota(key string) requestQuota {
	return requestQuota{key: key, rate: limiter.rate, burst: limiter.burst}
}

func (limiter *Limiter) planForTenant(tenantID string) quotaPlan {
	if limiter != nil {
		limiter.plansMu.RLock()
		defer limiter.plansMu.RUnlock()
		if plan, ok := limiter.plans[tenantID]; ok {
			return plan
		}
	}
	return quotaPlan{rate: limiter.rate, burst: limiter.burst}
}

func normalizeTenantPlans(plans map[string]Plan) (map[string]quotaPlan, error) {
	result := make(map[string]quotaPlan, len(plans))
	for tenantID, plan := range plans {
		tenantID = strings.TrimSpace(tenantID)
		if tenantID == "" {
			return nil, errors.New("api-gateway tenant rate limit plan has empty tenant id")
		}
		if plan.RequestsPerSecond <= 0 {
			return nil, errors.New("api-gateway tenant rate limit plan rps must be greater than 0")
		}
		burst := float64(plan.Burst)
		if burst <= 0 {
			burst = math.Ceil(plan.RequestsPerSecond)
		}
		result[tenantID] = quotaPlan{rate: plan.RequestsPerSecond, burst: burst}
	}
	return result, nil
}

func bearerOrGatewayToken(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	if values := md.Get(metadataToken); len(values) > 0 {
		if value := strings.TrimSpace(values[0]); value != "" {
			return value
		}
	}
	if values := md.Get("authorization"); len(values) > 0 {
		return bearerToken(values[0])
	}
	return ""
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	prefix := "Bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

func trimKeyPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	runes := []rune(value)
	if len(runes) <= maxKeyLength {
		return value
	}
	return string(runes[:maxKeyLength])
}
