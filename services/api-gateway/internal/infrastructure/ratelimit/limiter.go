package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"strings"
	"sync"
	"time"

	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

const (
	metadataToken = "x-nexusim-gateway-token"
	maxKeyLength  = 96
)

type Config struct {
	Enabled           bool
	RequestsPerSecond float64
	Burst             int
	MaxKeys           int
	Now               func() time.Time
}

type Limiter struct {
	enabled bool
	rate    float64
	burst   float64
	maxKeys int
	now     func() time.Time

	mu            sync.Mutex
	buckets       map[string]*bucket
	totalLimited  int64
	totalAccepted int64
}

type bucket struct {
	tokens   float64
	lastSeen time.Time
	limited  int64
	accepted int64
}

type Snapshot struct {
	Enabled       bool    `json:"enabled"`
	RatePerSecond float64 `json:"rate_per_second,omitempty"`
	Burst         int     `json:"burst,omitempty"`
	TrackedKeys   int     `json:"tracked_keys,omitempty"`
	MaxKeys       int     `json:"max_keys,omitempty"`
	TotalAccepted int64   `json:"total_accepted"`
	TotalLimited  int64   `json:"total_limited"`
}

func New(config Config) (*Limiter, error) {
	limiter := &Limiter{
		enabled: config.Enabled,
		rate:    config.RequestsPerSecond,
		burst:   float64(config.Burst),
		maxKeys: config.MaxKeys,
		now:     config.Now,
		buckets: make(map[string]*bucket),
	}
	if limiter.now == nil {
		limiter.now = time.Now
	}
	if !limiter.enabled {
		return limiter, nil
	}
	if limiter.rate <= 0 {
		return nil, errors.New("api-gateway rate limit rps must be greater than 0 when enabled")
	}
	if limiter.burst <= 0 {
		limiter.burst = math.Ceil(limiter.rate)
	}
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
		if !limiter.allow(ctx, info.FullMethod) {
			return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}
		return handler(ctx, request)
	}
}

func (limiter *Limiter) Snapshot() Snapshot {
	if limiter == nil {
		return Snapshot{}
	}
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	return Snapshot{
		Enabled:       limiter.enabled,
		RatePerSecond: limiter.rate,
		Burst:         int(limiter.burst),
		TrackedKeys:   len(limiter.buckets),
		MaxKeys:       limiter.maxKeys,
		TotalAccepted: limiter.totalAccepted,
		TotalLimited:  limiter.totalLimited,
	}
}

func (limiter *Limiter) allow(ctx context.Context, method string) bool {
	now := limiter.now()
	key := requestKey(ctx, method)

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	entry := limiter.buckets[key]
	if entry == nil {
		if len(limiter.buckets) >= limiter.maxKeys {
			limiter.evictOldestLocked()
		}
		entry = &bucket{tokens: limiter.burst, lastSeen: now}
		limiter.buckets[key] = entry
	}
	elapsed := now.Sub(entry.lastSeen).Seconds()
	if elapsed > 0 {
		entry.tokens = math.Min(limiter.burst, entry.tokens+elapsed*limiter.rate)
	}
	entry.lastSeen = now
	if entry.tokens < 1 {
		entry.limited++
		limiter.totalLimited++
		return false
	}
	entry.tokens--
	entry.accepted++
	limiter.totalAccepted++
	return true
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

func requestKey(ctx context.Context, method string) string {
	method = trimKeyPart(method)
	if token := bearerOrGatewayToken(ctx); token != "" {
		digest := sha256.Sum256([]byte(token))
		return method + "|token:" + hex.EncodeToString(digest[:12])
	}
	if peerInfo, ok := peer.FromContext(ctx); ok && peerInfo.Addr != nil {
		return method + "|peer:" + trimKeyPart(peerInfo.Addr.String())
	}
	return method + "|unknown"
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
