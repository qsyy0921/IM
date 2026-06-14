package monitoring

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

type Handler struct {
	pool                     *pgxpool.Pool
	grpcMetrics              *GRPCMetrics
	timelineWorkerSnapshotFn func() types.ProjectionWorkerSnapshot
	outboxRelaySnapshotFn    func() types.OutboxRelayWorkerSnapshot
}

func NewHandler(pool *pgxpool.Pool, grpcMetrics ...*GRPCMetrics) *Handler {
	handler := &Handler{pool: pool}
	if len(grpcMetrics) > 0 {
		handler.grpcMetrics = grpcMetrics[0]
	}
	return handler
}

func (h *Handler) WithTimelineProjectionWorkerStats(snapshotFunc func() types.ProjectionWorkerSnapshot) *Handler {
	h.timelineWorkerSnapshotFn = snapshotFunc
	return h
}

func (h *Handler) WithOutboxRelayStats(snapshotFunc func() types.OutboxRelayWorkerSnapshot) *Handler {
	h.outboxRelaySnapshotFn = snapshotFunc
	return h
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
	if h.timelineWorkerSnapshotFn != nil {
		workerSnapshot := h.timelineWorkerSnapshotFn()
		snapshot.TimelineProjectionWorker = &workerSnapshot
	}
	if h.outboxRelaySnapshotFn != nil {
		relaySnapshot := h.outboxRelaySnapshotFn()
		snapshot.OutboxRelay = &relaySnapshot
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
		delivery, err := queryDeliverySnapshot(ctx, h.pool)
		if err != nil {
			snapshot.DeliveryError = "delivery metrics query failed"
		} else {
			snapshot.Delivery = &delivery
		}
		deliveryOutbox, err := queryDeliveryOutboxSnapshot(ctx, h.pool)
		if err != nil {
			snapshot.DeliveryOutboxError = "delivery outbox metrics query failed"
		} else {
			snapshot.DeliveryOutbox = &deliveryOutbox
		}
		projectionFailures, err := queryProjectionFailureSnapshot(ctx, h.pool)
		if err != nil {
			snapshot.ProjectionFailuresError = "delivery projection failure metrics query failed"
		} else {
			snapshot.ProjectionFailures = &projectionFailures
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
	Service                  string                           `json:"service"`
	GeneratedAtMS            int64                            `json:"generated_at_ms"`
	PGPool                   *PGPoolSnapshot                  `json:"pg_pool,omitempty"`
	Delivery                 *DeliverySnapshot                `json:"delivery,omitempty"`
	DeliveryError            string                           `json:"delivery_error,omitempty"`
	DeliveryOutbox           *DeliveryOutboxSnapshot          `json:"delivery_outbox,omitempty"`
	DeliveryOutboxError      string                           `json:"delivery_outbox_error,omitempty"`
	ProjectionFailures       *ProjectionFailureSnapshot       `json:"projection_failures,omitempty"`
	ProjectionFailuresError  string                           `json:"projection_failures_error,omitempty"`
	GRPC                     *GRPCSnapshot                    `json:"grpc,omitempty"`
	TimelineProjectionWorker *types.ProjectionWorkerSnapshot  `json:"timeline_projection_worker,omitempty"`
	OutboxRelay              *types.OutboxRelayWorkerSnapshot `json:"outbox_relay,omitempty"`
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

type DeliverySnapshot struct {
	UserInboxTotal                 int64 `json:"user_inbox_total"`
	UserInboxDistinctUsers         int64 `json:"user_inbox_distinct_users"`
	UserInboxDistinctConversations int64 `json:"user_inbox_distinct_conversations"`
	MembershipProjectionTotal      int64 `json:"membership_projection_total"`
	MembershipProjectionActive     int64 `json:"membership_projection_active"`
	MembershipProjectionInactive   int64 `json:"membership_projection_inactive"`
	DeviceDeliveryCursors          int64 `json:"device_delivery_cursors"`
	KafkaCheckpoints               int64 `json:"kafka_checkpoints"`
	KafkaConsumerGroups            int64 `json:"kafka_consumer_groups"`
}

type DeliveryOutboxSnapshot struct {
	Total            int64 `json:"total"`
	Pending          int64 `json:"pending"`
	PendingReady     int64 `json:"pending_ready"`
	PendingScheduled int64 `json:"pending_scheduled"`
	Published        int64 `json:"published"`
	DLQ              int64 `json:"dlq"`
	MaxPendingRetry  int64 `json:"max_pending_retry"`
}

type ProjectionFailureSnapshot struct {
	Total                int64 `json:"total"`
	DecodeFailed         int64 `json:"decode_failed"`
	InvalidArgument      int64 `json:"invalid_argument"`
	ProjectionDependency int64 `json:"projection_dependency"`
	DBReadFailed         int64 `json:"db_read_failed"`
	DBWriteFailed        int64 `json:"db_write_failed"`
	Unknown              int64 `json:"unknown"`
	MaxFailureCount      int64 `json:"max_failure_count"`
	ResolvedTotal        int64 `json:"resolved_total"`
}

func queryDeliverySnapshot(ctx context.Context, pool *pgxpool.Pool) (DeliverySnapshot, error) {
	var snapshot DeliverySnapshot
	err := pool.QueryRow(ctx, `
SELECT
    (SELECT COUNT(*) FROM user_inbox),
    (SELECT COUNT(DISTINCT user_id) FROM user_inbox),
    (SELECT COUNT(DISTINCT tenant_id || ':' || conversation_id) FROM user_inbox),
    (SELECT COUNT(*) FROM delivery_membership_projection),
    (SELECT COUNT(*) FROM delivery_membership_projection WHERE status = 'ACTIVE'),
    (SELECT COUNT(*) FROM delivery_membership_projection WHERE status <> 'ACTIVE'),
    (SELECT COUNT(*) FROM device_delivery_cursors),
    (SELECT COUNT(*) FROM delivery_kafka_checkpoints),
    (SELECT COUNT(DISTINCT consumer_group) FROM delivery_kafka_checkpoints)
`).Scan(
		&snapshot.UserInboxTotal,
		&snapshot.UserInboxDistinctUsers,
		&snapshot.UserInboxDistinctConversations,
		&snapshot.MembershipProjectionTotal,
		&snapshot.MembershipProjectionActive,
		&snapshot.MembershipProjectionInactive,
		&snapshot.DeviceDeliveryCursors,
		&snapshot.KafkaCheckpoints,
		&snapshot.KafkaConsumerGroups,
	)
	if err != nil {
		return DeliverySnapshot{}, err
	}
	return snapshot, nil
}

func queryDeliveryOutboxSnapshot(ctx context.Context, pool *pgxpool.Pool) (DeliveryOutboxSnapshot, error) {
	var snapshot DeliveryOutboxSnapshot
	err := pool.QueryRow(ctx, `
SELECT
    COUNT(*),
    COUNT(*) FILTER (WHERE status = 'PENDING'),
    COUNT(*) FILTER (WHERE status = 'PENDING' AND COALESCE(next_retry_at, available_at) <= now()),
    COUNT(*) FILTER (WHERE status = 'PENDING' AND COALESCE(next_retry_at, available_at) > now()),
    COUNT(*) FILTER (WHERE status = 'PUBLISHED'),
    COUNT(*) FILTER (WHERE status = 'DLQ'),
    COALESCE(MAX(retry_count) FILTER (WHERE status = 'PENDING'), 0)
FROM delivery_outbox
`).Scan(
		&snapshot.Total,
		&snapshot.Pending,
		&snapshot.PendingReady,
		&snapshot.PendingScheduled,
		&snapshot.Published,
		&snapshot.DLQ,
		&snapshot.MaxPendingRetry,
	)
	if err != nil {
		return DeliveryOutboxSnapshot{}, err
	}
	return snapshot, nil
}

func queryProjectionFailureSnapshot(ctx context.Context, pool *pgxpool.Pool) (ProjectionFailureSnapshot, error) {
	var snapshot ProjectionFailureSnapshot
	err := pool.QueryRow(ctx, `
SELECT
    COUNT(*) FILTER (WHERE resolved_at IS NULL),
    COUNT(*) FILTER (WHERE resolved_at IS NULL AND failure_class = 'decode_failed'),
    COUNT(*) FILTER (WHERE resolved_at IS NULL AND failure_class = 'invalid_argument'),
    COUNT(*) FILTER (WHERE resolved_at IS NULL AND failure_class = 'projection_dependency'),
    COUNT(*) FILTER (WHERE resolved_at IS NULL AND failure_class = 'db_read_failed'),
    COUNT(*) FILTER (WHERE resolved_at IS NULL AND failure_class = 'db_write_failed'),
    COUNT(*) FILTER (WHERE resolved_at IS NULL AND failure_class = 'unknown'),
    COALESCE(MAX(failure_count) FILTER (WHERE resolved_at IS NULL), 0),
    COUNT(*) FILTER (WHERE resolved_at IS NOT NULL)
FROM delivery_projection_failures
`).Scan(
		&snapshot.Total,
		&snapshot.DecodeFailed,
		&snapshot.InvalidArgument,
		&snapshot.ProjectionDependency,
		&snapshot.DBReadFailed,
		&snapshot.DBWriteFailed,
		&snapshot.Unknown,
		&snapshot.MaxFailureCount,
		&snapshot.ResolvedTotal,
	)
	if err != nil {
		return ProjectionFailureSnapshot{}, err
	}
	return snapshot, nil
}

func writeJSON(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}
