package monitoring

import (
	"encoding/json"
	"net/http"
	"time"

	gatewayauth "github.com/qsyy0921/IM/internal/gatewayauth"
	"github.com/qsyy0921/IM/services/api-gateway/internal/infrastructure/ratelimit"
)

const serviceName = "api-gateway"

type Handler struct {
	grpcMetrics        *GRPCMetrics
	httpBFFStatsFunc   func() HTTPBFFSnapshot
	jwkStatsFunc       func() gatewayauth.JWKStats
	rateLimitStatsFunc func() ratelimit.Snapshot
	runtimeStatsFunc   func() RuntimeSnapshot
	traceStatsFunc     func() TraceSnapshot
}

func NewHandler(grpcMetrics *GRPCMetrics) *Handler {
	return &Handler{grpcMetrics: grpcMetrics}
}

func (h *Handler) WithHTTPBFFStats(statsFunc func() HTTPBFFSnapshot) *Handler {
	h.httpBFFStatsFunc = statsFunc
	return h
}

func (h *Handler) WithAuthJWKStats(statsFunc func() gatewayauth.JWKStats) *Handler {
	h.jwkStatsFunc = statsFunc
	return h
}

func (h *Handler) WithRateLimitStats(statsFunc func() ratelimit.Snapshot) *Handler {
	h.rateLimitStatsFunc = statsFunc
	return h
}

func (h *Handler) WithRuntimeStats(statsFunc func() RuntimeSnapshot) *Handler {
	h.runtimeStatsFunc = statsFunc
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
		h.handleMetrics(w, r)
	case "/metrics":
		h.handlePrometheusMetrics(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.snapshot())
}

func (h *Handler) handlePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(renderPrometheus(h.snapshot())))
}

func (h *Handler) snapshot() Snapshot {
	snapshot := Snapshot{
		Service:       serviceName,
		GeneratedAtMS: time.Now().UnixMilli(),
	}
	if h.grpcMetrics != nil {
		grpcSnapshot := h.grpcMetrics.Snapshot()
		snapshot.GRPC = &grpcSnapshot
	}
	if h.httpBFFStatsFunc != nil {
		stats := h.httpBFFStatsFunc()
		snapshot.HTTPBFF = &stats
	}
	if h.jwkStatsFunc != nil {
		stats := h.jwkStatsFunc()
		snapshot.AuthJWKs = &stats
	}
	if h.rateLimitStatsFunc != nil {
		stats := h.rateLimitStatsFunc()
		snapshot.RateLimit = &stats
	}
	if h.runtimeStatsFunc != nil {
		stats := h.runtimeStatsFunc()
		snapshot.Runtime = &stats
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
	Service       string                `json:"service"`
	GeneratedAtMS int64                 `json:"generated_at_ms"`
	GRPC          *GRPCSnapshot         `json:"grpc,omitempty"`
	HTTPBFF       *HTTPBFFSnapshot      `json:"http_bff,omitempty"`
	AuthJWKs      *gatewayauth.JWKStats `json:"auth_jwks,omitempty"`
	RateLimit     *ratelimit.Snapshot   `json:"rate_limit,omitempty"`
	Runtime       *RuntimeSnapshot      `json:"runtime,omitempty"`
	Trace         *TraceSnapshot        `json:"trace,omitempty"`
}

type RuntimeSnapshot struct {
	RegisterLegacyDescriptors       bool  `json:"register_legacy_descriptors"`
	LegacyDescriptorsAllowedUntilMS int64 `json:"legacy_descriptors_allowed_until_unix_ms,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
