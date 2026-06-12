package monitoring

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const serviceName = "identity-service"

type Handler struct {
	pool        *pgxpool.Pool
	grpcMetrics *GRPCMetrics
}

func NewHandler(pool *pgxpool.Pool, grpcMetrics ...*GRPCMetrics) *Handler {
	handler := &Handler{pool: pool}
	if len(grpcMetrics) > 0 {
		handler.grpcMetrics = grpcMetrics[0]
	}
	return handler
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/healthz":
		writeJSON(w, http.StatusOK, healthResponse{Service: serviceName, Status: "ok"})
	case "/readyz":
		h.handleReady(w, r)
	case "/debug/metrics":
		h.handleMetrics(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) handleReady(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		writeJSON(w, http.StatusServiceUnavailable, healthResponse{Service: serviceName, Status: "unready", Error: "postgres pool is not configured"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	if err := h.pool.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, healthResponse{Service: serviceName, Status: "unready", Error: "postgres ping failed"})
		return
	}
	writeJSON(w, http.StatusOK, healthResponse{Service: serviceName, Status: "ready"})
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
	if h.pool != nil {
		stats := h.pool.Stat()
		snapshot.PGPool = &PGPoolSnapshot{
			AcquireCount:         stats.AcquireCount(),
			AcquireDurationMS:    int64(stats.AcquireDuration() / time.Millisecond),
			AcquiredConns:        stats.AcquiredConns(),
			CanceledAcquireCount: stats.CanceledAcquireCount(),
			ConstructingConns:    stats.ConstructingConns(),
			EmptyAcquireCount:    stats.EmptyAcquireCount(),
			IdleConns:            stats.IdleConns(),
			MaxConns:             stats.MaxConns(),
			TotalConns:           stats.TotalConns(),
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		identity, err := queryIdentitySnapshot(ctx, h.pool)
		if err != nil {
			snapshot.IdentityError = "identity metrics query failed"
		} else {
			snapshot.Identity = &identity
		}
	}
	writeJSON(w, http.StatusOK, snapshot)
}

type healthResponse struct {
	Service string `json:"service"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
}

type Snapshot struct {
	Service       string            `json:"service"`
	GeneratedAtMS int64             `json:"generated_at_ms"`
	PGPool        *PGPoolSnapshot   `json:"pg_pool,omitempty"`
	Identity      *IdentitySnapshot `json:"identity,omitempty"`
	IdentityError string            `json:"identity_error,omitempty"`
	GRPC          *GRPCSnapshot     `json:"grpc,omitempty"`
}

type PGPoolSnapshot struct {
	AcquireCount         int64 `json:"acquire_count"`
	AcquireDurationMS    int64 `json:"acquire_duration_ms"`
	AcquiredConns        int32 `json:"acquired_conns"`
	CanceledAcquireCount int64 `json:"canceled_acquire_count"`
	ConstructingConns    int32 `json:"constructing_conns"`
	EmptyAcquireCount    int64 `json:"empty_acquire_count"`
	IdleConns            int32 `json:"idle_conns"`
	MaxConns             int32 `json:"max_conns"`
	TotalConns           int32 `json:"total_conns"`
}

type IdentitySnapshot struct {
	Users           int64 `json:"users"`
	ActiveDevices   int64 `json:"active_devices"`
	RevokedDevices  int64 `json:"revoked_devices"`
	ActiveSessions  int64 `json:"active_sessions"`
	RevokedSessions int64 `json:"revoked_sessions"`
	ExpiredSessions int64 `json:"expired_sessions"`
}

func queryIdentitySnapshot(ctx context.Context, pool *pgxpool.Pool) (IdentitySnapshot, error) {
	var snapshot IdentitySnapshot
	err := pool.QueryRow(ctx, `
SELECT
    (SELECT COUNT(*) FROM identity_users),
    (SELECT COUNT(*) FROM identity_devices WHERE status = 'ACTIVE'),
    (SELECT COUNT(*) FROM identity_devices WHERE status = 'REVOKED'),
    (SELECT COUNT(*) FROM identity_sessions WHERE status = 'ACTIVE' AND expires_at > now()),
    (SELECT COUNT(*) FROM identity_sessions WHERE status = 'REVOKED'),
    (SELECT COUNT(*) FROM identity_sessions WHERE status = 'ACTIVE' AND expires_at <= now())
`).Scan(
		&snapshot.Users,
		&snapshot.ActiveDevices,
		&snapshot.RevokedDevices,
		&snapshot.ActiveSessions,
		&snapshot.RevokedSessions,
		&snapshot.ExpiredSessions,
	)
	if err != nil {
		return IdentitySnapshot{}, err
	}
	return snapshot, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
