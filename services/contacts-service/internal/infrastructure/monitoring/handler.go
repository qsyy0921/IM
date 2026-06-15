package monitoring

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

const serviceName = "contacts-service"

type Handler struct {
	pool                 *pgxpool.Pool
	grpcMetrics          *GRPCMetrics
	outboxRelayStatsFunc func() types.OutboxRelayWorkerSnapshot
	traceStatsFunc       func() TraceSnapshot
}

func NewHandler(pool *pgxpool.Pool, grpcMetrics ...*GRPCMetrics) *Handler {
	handler := &Handler{pool: pool}
	if len(grpcMetrics) > 0 {
		handler.grpcMetrics = grpcMetrics[0]
	}
	return handler
}

func (h *Handler) WithOutboxRelayStats(statsFunc func() types.OutboxRelayWorkerSnapshot) *Handler {
	h.outboxRelayStatsFunc = statsFunc
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
		h.handleReady(w, r)
	case "/debug/metrics":
		writeJSON(w, http.StatusOK, h.snapshot(r.Context()))
	case "/metrics":
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(renderPrometheus(h.snapshot(r.Context()))))
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

func (h *Handler) snapshot(ctx context.Context) Snapshot {
	snapshot := Snapshot{
		Service:       serviceName,
		GeneratedAtMS: time.Now().UnixMilli(),
	}
	if h.grpcMetrics != nil {
		grpcSnapshot := h.grpcMetrics.Snapshot()
		snapshot.GRPC = &grpcSnapshot
	}
	if h.outboxRelayStatsFunc != nil {
		relaySnapshot := h.outboxRelayStatsFunc()
		snapshot.OutboxRelay = &relaySnapshot
	}
	if h.traceStatsFunc != nil {
		traceSnapshot := h.traceStatsFunc()
		snapshot.Trace = &traceSnapshot
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
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		contacts, err := queryContactsSnapshot(ctx, h.pool)
		if err != nil {
			snapshot.ContactsError = "contacts metrics query failed"
		} else {
			snapshot.Contacts = &contacts
		}
		outbox, err := queryOutboxSnapshot(ctx, h.pool)
		if err != nil {
			snapshot.OutboxError = "contacts outbox metrics query failed"
		} else {
			snapshot.Outbox = &outbox
		}
	}
	return snapshot
}

type healthResponse struct {
	Service string `json:"service"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
}

type Snapshot struct {
	Service       string                           `json:"service"`
	GeneratedAtMS int64                            `json:"generated_at_ms"`
	PGPool        *PGPoolSnapshot                  `json:"pg_pool,omitempty"`
	Contacts      *ContactsSnapshot                `json:"contacts,omitempty"`
	ContactsError string                           `json:"contacts_error,omitempty"`
	Outbox        *OutboxSnapshot                  `json:"contacts_outbox,omitempty"`
	OutboxError   string                           `json:"contacts_outbox_error,omitempty"`
	GRPC          *GRPCSnapshot                    `json:"grpc,omitempty"`
	OutboxRelay   *types.OutboxRelayWorkerSnapshot `json:"outbox_relay,omitempty"`
	Trace         *TraceSnapshot                   `json:"trace,omitempty"`
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

type ContactsSnapshot struct {
	Requests                *ContactRequestSnapshot `json:"requests,omitempty"`
	Edges                   *ContactEdgeSnapshot    `json:"edges,omitempty"`
	CommandIdempotencyTotal int64                   `json:"command_idempotency_total"`
}

type ContactRequestSnapshot struct {
	Total    int64                `json:"total"`
	Pending  int64                `json:"pending"`
	Accepted int64                `json:"accepted"`
	Declined int64                `json:"declined"`
	Canceled int64                `json:"canceled"`
	Expired  int64                `json:"expired"`
	ByStatus []GroupCountSnapshot `json:"by_status"`
}

type ContactEdgeSnapshot struct {
	Total      int64                `json:"total"`
	Active     int64                `json:"active"`
	Deleted    int64                `json:"deleted"`
	Blocked    int64                `json:"blocked"`
	WithRemark int64                `json:"with_remark"`
	ByStatus   []GroupCountSnapshot `json:"by_status"`
}

type GroupCountSnapshot struct {
	Value string `json:"value"`
	Total int64  `json:"total"`
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

func queryContactsSnapshot(ctx context.Context, pool *pgxpool.Pool) (ContactsSnapshot, error) {
	var snapshot ContactsSnapshot

	requests, err := queryContactRequestSnapshot(ctx, pool)
	if err != nil {
		return ContactsSnapshot{}, err
	}
	snapshot.Requests = &requests

	edges, err := queryContactEdgeSnapshot(ctx, pool)
	if err != nil {
		return ContactsSnapshot{}, err
	}
	snapshot.Edges = &edges

	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM contact_command_idempotency
`).Scan(&snapshot.CommandIdempotencyTotal); err != nil {
		return ContactsSnapshot{}, err
	}

	return snapshot, nil
}

func queryContactRequestSnapshot(ctx context.Context, pool *pgxpool.Pool) (ContactRequestSnapshot, error) {
	var snapshot ContactRequestSnapshot
	if err := pool.QueryRow(ctx, `
SELECT
    COUNT(*),
    COUNT(*) FILTER (WHERE status = 'PENDING'),
    COUNT(*) FILTER (WHERE status = 'ACCEPTED'),
    COUNT(*) FILTER (WHERE status = 'DECLINED'),
    COUNT(*) FILTER (WHERE status = 'CANCELED'),
    COUNT(*) FILTER (WHERE status = 'EXPIRED')
FROM contact_requests
`).Scan(
		&snapshot.Total,
		&snapshot.Pending,
		&snapshot.Accepted,
		&snapshot.Declined,
		&snapshot.Canceled,
		&snapshot.Expired,
	); err != nil {
		return ContactRequestSnapshot{}, err
	}
	byStatus, err := queryGroupCounts(ctx, pool, `
SELECT status, COUNT(*)
FROM contact_requests
GROUP BY status
ORDER BY status
`)
	if err != nil {
		return ContactRequestSnapshot{}, err
	}
	snapshot.ByStatus = byStatus
	return snapshot, nil
}

func queryContactEdgeSnapshot(ctx context.Context, pool *pgxpool.Pool) (ContactEdgeSnapshot, error) {
	var snapshot ContactEdgeSnapshot
	if err := pool.QueryRow(ctx, `
SELECT
    COUNT(*),
    COUNT(*) FILTER (WHERE status = 'ACTIVE'),
    COUNT(*) FILTER (WHERE status = 'DELETED'),
    COUNT(*) FILTER (WHERE status = 'BLOCKED'),
    COUNT(*) FILTER (WHERE remark <> '')
FROM contact_edges
`).Scan(
		&snapshot.Total,
		&snapshot.Active,
		&snapshot.Deleted,
		&snapshot.Blocked,
		&snapshot.WithRemark,
	); err != nil {
		return ContactEdgeSnapshot{}, err
	}
	byStatus, err := queryGroupCounts(ctx, pool, `
SELECT status, COUNT(*)
FROM contact_edges
GROUP BY status
ORDER BY status
`)
	if err != nil {
		return ContactEdgeSnapshot{}, err
	}
	snapshot.ByStatus = byStatus
	return snapshot, nil
}

func queryGroupCounts(ctx context.Context, pool *pgxpool.Pool, query string) ([]GroupCountSnapshot, error) {
	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := make([]GroupCountSnapshot, 0)
	for rows.Next() {
		var entry GroupCountSnapshot
		if err := rows.Scan(&entry.Value, &entry.Total); err != nil {
			return nil, err
		}
		values = append(values, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return values, nil
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
