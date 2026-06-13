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
	jwkStatsFunc       func() gatewayauth.JWKStats
	rateLimitStatsFunc func() ratelimit.Snapshot
}

func NewHandler(grpcMetrics *GRPCMetrics) *Handler {
	return &Handler{grpcMetrics: grpcMetrics}
}

func (h *Handler) WithAuthJWKStats(statsFunc func() gatewayauth.JWKStats) *Handler {
	h.jwkStatsFunc = statsFunc
	return h
}

func (h *Handler) WithRateLimitStats(statsFunc func() ratelimit.Snapshot) *Handler {
	h.rateLimitStatsFunc = statsFunc
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
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	snapshot := Snapshot{
		Service:       serviceName,
		GeneratedAtMS: time.Now().UnixMilli(),
	}
	if h.grpcMetrics != nil {
		grpcSnapshot := h.grpcMetrics.Snapshot()
		snapshot.GRPC = &grpcSnapshot
	}
	if h.jwkStatsFunc != nil {
		stats := h.jwkStatsFunc()
		snapshot.AuthJWKs = &stats
	}
	if h.rateLimitStatsFunc != nil {
		stats := h.rateLimitStatsFunc()
		snapshot.RateLimit = &stats
	}
	writeJSON(w, http.StatusOK, snapshot)
}

type healthResponse struct {
	Service string `json:"service"`
	Status  string `json:"status"`
}

type Snapshot struct {
	Service       string                `json:"service"`
	GeneratedAtMS int64                 `json:"generated_at_ms"`
	GRPC          *GRPCSnapshot         `json:"grpc,omitempty"`
	AuthJWKs      *gatewayauth.JWKStats `json:"auth_jwks,omitempty"`
	RateLimit     *ratelimit.Snapshot   `json:"rate_limit,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
