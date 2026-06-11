package monitoring

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const serviceName = "contacts-service"

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
		outbox, err := queryOutboxSnapshot(ctx, h.pool)
		if err != nil {
			snapshot.OutboxError = "contacts outbox metrics query failed"
		} else {
			snapshot.Outbox = &outbox
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
	Service       string          `json:"service"`
	GeneratedAtMS int64           `json:"generated_at_ms"`
	PGPool        *PGPoolSnapshot `json:"pg_pool,omitempty"`
	Outbox        *OutboxSnapshot `json:"contacts_outbox,omitempty"`
	OutboxError   string          `json:"contacts_outbox_error,omitempty"`
	GRPC          *GRPCSnapshot   `json:"grpc,omitempty"`
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

type OutboxSnapshot struct {
	Total              int64  `json:"total"`
	Pending            int64  `json:"pending"`
	Published          int64  `json:"published"`
	DLQ                int64  `json:"dlq"`
	ReadyPending       int64  `json:"ready_pending"`
	OldestPendingAgeMS *int64 `json:"oldest_pending_age_ms,omitempty"`
	OldestDLQAgeMS     *int64 `json:"oldest_dlq_age_ms,omitempty"`
}

func queryOutboxSnapshot(ctx context.Context, pool *pgxpool.Pool) (OutboxSnapshot, error) {
	var snapshot OutboxSnapshot
	var oldestPendingAge *float64
	var oldestDLQAge *float64
	err := pool.QueryRow(ctx, `
SELECT
    COUNT(*) AS total,
    COUNT(*) FILTER (WHERE status = 'PENDING') AS pending,
    COUNT(*) FILTER (WHERE status = 'PUBLISHED') AS published,
    COUNT(*) FILTER (WHERE status = 'DLQ') AS dlq,
    COUNT(*) FILTER (
        WHERE status = 'PENDING'
          AND available_at <= now()
          AND (next_retry_at IS NULL OR next_retry_at <= now())
    ) AS ready_pending,
    EXTRACT(EPOCH FROM (now() - MIN(CASE WHEN status = 'PENDING' THEN COALESCE(next_retry_at, available_at, created_at) END))) * 1000 AS oldest_pending_age_ms,
    EXTRACT(EPOCH FROM (now() - MIN(CASE WHEN status = 'DLQ' THEN COALESCE(dead_lettered_at, updated_at, created_at) END))) * 1000 AS oldest_dlq_age_ms
FROM contacts_outbox
`).Scan(
		&snapshot.Total,
		&snapshot.Pending,
		&snapshot.Published,
		&snapshot.DLQ,
		&snapshot.ReadyPending,
		&oldestPendingAge,
		&oldestDLQAge,
	)
	if err != nil {
		return OutboxSnapshot{}, err
	}
	snapshot.OldestPendingAgeMS = floatMillisToIntPtr(oldestPendingAge)
	snapshot.OldestDLQAgeMS = floatMillisToIntPtr(oldestDLQAge)
	return snapshot, nil
}

func floatMillisToIntPtr(value *float64) *int64 {
	if value == nil {
		return nil
	}
	converted := int64(*value)
	if converted < 0 {
		converted = 0
	}
	return &converted
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
