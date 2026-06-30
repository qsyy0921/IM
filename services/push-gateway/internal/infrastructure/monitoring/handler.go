package monitoring

import (
	"encoding/json"
	"net/http"
	"time"

	authinfra "github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/auth"
	"github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/memory"
	redisroute "github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/redisroute"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
)

const serviceName = "push-gateway"

type Handler struct {
	memoryMetricsFunc          func() memory.Metrics
	redisRegistryMetricsFunc   func() redisroute.Metrics
	redisSubscriberMetricsFunc func() redisroute.Metrics
	redisSubscriberWorkerFunc  func() types.RedisSubscriberWorkerSnapshot
	authJWKStatsFunc           func() *authinfra.JWKStats
	deliveryWorkerStatsFunc    func() types.ConsumerWorkerSnapshot
	identityWorkerStatsFunc    func() types.ConsumerWorkerSnapshot
	webSocketWriterStatsFunc   func() types.WebSocketWriterSnapshot
	traceStatsFunc             func() TraceSnapshot
}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) WithMemoryMetrics(metricsFunc func() memory.Metrics) *Handler {
	h.memoryMetricsFunc = metricsFunc
	return h
}

func (h *Handler) WithRedisRegistryMetrics(metricsFunc func() redisroute.Metrics) *Handler {
	h.redisRegistryMetricsFunc = metricsFunc
	return h
}

func (h *Handler) WithRedisSubscriberMetrics(metricsFunc func() redisroute.Metrics) *Handler {
	h.redisSubscriberMetricsFunc = metricsFunc
	return h
}

func (h *Handler) WithRedisSubscriberWorkerStats(statsFunc func() types.RedisSubscriberWorkerSnapshot) *Handler {
	h.redisSubscriberWorkerFunc = statsFunc
	return h
}

func (h *Handler) WithAuthJWKStats(statsFunc func() *authinfra.JWKStats) *Handler {
	h.authJWKStatsFunc = statsFunc
	return h
}

func (h *Handler) WithDeliveryConsumerStats(statsFunc func() types.ConsumerWorkerSnapshot) *Handler {
	h.deliveryWorkerStatsFunc = statsFunc
	return h
}

func (h *Handler) WithIdentityConsumerStats(statsFunc func() types.ConsumerWorkerSnapshot) *Handler {
	h.identityWorkerStatsFunc = statsFunc
	return h
}

func (h *Handler) WithWebSocketWriterStats(statsFunc func() types.WebSocketWriterSnapshot) *Handler {
	h.webSocketWriterStatsFunc = statsFunc
	return h
}

func (h *Handler) WithTraceStats(statsFunc func() TraceSnapshot) *Handler {
	h.traceStatsFunc = statsFunc
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/healthz":
		writeJSON(w, http.StatusOK, healthResponse{Service: serviceName, Status: "ok"})
	case "/readyz":
		writeJSON(w, http.StatusOK, healthResponse{Service: serviceName, Status: "ready"})
	case "/debug/metrics":
		writeJSON(w, http.StatusOK, h.snapshot())
	case "/metrics":
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(renderPrometheus(h.snapshot())))
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) snapshot() Snapshot {
	snapshot := Snapshot{
		Service:       serviceName,
		GeneratedAtMS: time.Now().UnixMilli(),
	}
	if h.memoryMetricsFunc != nil {
		memoryMetrics := h.memoryMetricsFunc()
		snapshot.Memory = &memoryMetrics
	}
	if h.redisRegistryMetricsFunc != nil {
		redisMetrics := h.redisRegistryMetricsFunc()
		snapshot.RedisRegistryMetrics = &redisMetrics
	}
	if h.redisSubscriberMetricsFunc != nil {
		redisMetrics := h.redisSubscriberMetricsFunc()
		snapshot.RedisSubscriberStats = &redisMetrics
	}
	if h.redisSubscriberWorkerFunc != nil {
		stats := h.redisSubscriberWorkerFunc()
		snapshot.RedisSubscriberWorker = &stats
	}
	if h.authJWKStatsFunc != nil {
		snapshot.AuthJWKStats = h.authJWKStatsFunc()
	}
	if h.deliveryWorkerStatsFunc != nil {
		stats := h.deliveryWorkerStatsFunc()
		snapshot.DeliveryConsumer = &stats
	}
	if h.identityWorkerStatsFunc != nil {
		stats := h.identityWorkerStatsFunc()
		snapshot.IdentityConsumer = &stats
	}
	if h.webSocketWriterStatsFunc != nil {
		stats := h.webSocketWriterStatsFunc()
		snapshot.WebSocketWriter = &stats
	}
	if h.traceStatsFunc != nil {
		stats := h.traceStatsFunc()
		snapshot.Trace = &stats
	}
	return snapshot
}

type healthResponse struct {
	Service string `json:"service"`
	Status  string `json:"status"`
}

type Snapshot struct {
	Service               string                               `json:"service"`
	GeneratedAtMS         int64                                `json:"generated_at_ms"`
	Memory                *memory.Metrics                      `json:"memory,omitempty"`
	RedisRegistryMetrics  *redisroute.Metrics                  `json:"redis_registry_metrics,omitempty"`
	RedisSubscriberStats  *redisroute.Metrics                  `json:"redis_subscriber_metrics,omitempty"`
	RedisSubscriberWorker *types.RedisSubscriberWorkerSnapshot `json:"redis_subscriber_worker,omitempty"`
	AuthJWKStats          *authinfra.JWKStats                  `json:"auth_jwks,omitempty"`
	DeliveryConsumer      *types.ConsumerWorkerSnapshot        `json:"delivery_consumer,omitempty"`
	IdentityConsumer      *types.ConsumerWorkerSnapshot        `json:"identity_consumer,omitempty"`
	WebSocketWriter       *types.WebSocketWriterSnapshot       `json:"websocket_writer,omitempty"`
	Trace                 *TraceSnapshot                       `json:"trace,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
