package monitoring

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

type Handler struct {
	pool                     *pgxpool.Pool
	grpcMetrics              *GRPCMetrics
	deliveryWorkerSnapshotFn func() types.ProjectionWorkerSnapshot
	outboxRelaySnapshotFn    func() types.OutboxRelayWorkerSnapshot
	traceStatsFunc           func() TraceSnapshot
}

func NewHandler(pool *pgxpool.Pool, grpcMetrics ...*GRPCMetrics) *Handler {
	handler := &Handler{pool: pool}
	if len(grpcMetrics) > 0 {
		handler.grpcMetrics = grpcMetrics[0]
	}
	return handler
}

func (h *Handler) WithDeliveryProjectionWorkerStats(snapshotFunc func() types.ProjectionWorkerSnapshot) *Handler {
	h.deliveryWorkerSnapshotFn = snapshotFunc
	return h
}

func (h *Handler) WithOutboxRelayStats(snapshotFunc func() types.OutboxRelayWorkerSnapshot) *Handler {
	h.outboxRelaySnapshotFn = snapshotFunc
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
	if h.deliveryWorkerSnapshotFn != nil {
		workerSnapshot := h.deliveryWorkerSnapshotFn()
		snapshot.DeliveryProjectionWorker = &workerSnapshot
	}
	if h.outboxRelaySnapshotFn != nil {
		relaySnapshot := h.outboxRelaySnapshotFn()
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
		receipt, err := queryReceiptSnapshot(ctx, h.pool)
		if err != nil {
			snapshot.ReceiptError = "receipt metrics query failed"
		} else {
			snapshot.Receipt = &receipt
		}
		outbox, err := queryReceiptOutboxSnapshot(ctx, h.pool)
		if err != nil {
			snapshot.OutboxError = "receipt outbox metrics query failed"
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
	Service                  string                           `json:"service"`
	GeneratedAtMS            int64                            `json:"generated_at_ms"`
	PGPool                   *PGPoolSnapshot                  `json:"pg_pool,omitempty"`
	Receipt                  *ReceiptSnapshot                 `json:"receipt,omitempty"`
	ReceiptError             string                           `json:"receipt_error,omitempty"`
	Outbox                   *OutboxSnapshot                  `json:"receipt_outbox,omitempty"`
	OutboxError              string                           `json:"receipt_outbox_error,omitempty"`
	GRPC                     *GRPCSnapshot                    `json:"grpc,omitempty"`
	DeliveryProjectionWorker *types.ProjectionWorkerSnapshot  `json:"delivery_projection_worker,omitempty"`
	OutboxRelay              *types.OutboxRelayWorkerSnapshot `json:"outbox_relay,omitempty"`
	Trace                    *TraceSnapshot                   `json:"trace,omitempty"`
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

type ReceiptSnapshot struct {
	InboxProjectionTotal        int64 `json:"inbox_projection_total"`
	DeviceReceivedCursors       int64 `json:"device_received_cursors"`
	UserReceivedCursors         int64 `json:"user_received_cursors"`
	UserReadCursors             int64 `json:"user_read_cursors"`
	MessageReceiptStates        int64 `json:"message_receipt_states"`
	ConversationSummaries       int64 `json:"conversation_summaries"`
	ArchivedConversationCount   int64 `json:"archived_conversations"`
	PinnedConversationCount     int64 `json:"pinned_conversations"`
	MutedConversationCount      int64 `json:"muted_conversations"`
	UnreadConversationCount     int64 `json:"unread_conversations"`
	KafkaCheckpoints            int64 `json:"kafka_checkpoints"`
	KafkaConsumerGroups         int64 `json:"kafka_consumer_groups"`
	ConversationListCheckpoints int64 `json:"conversation_list_checkpoints"`
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

func queryReceiptSnapshot(ctx context.Context, pool *pgxpool.Pool) (ReceiptSnapshot, error) {
	var snapshot ReceiptSnapshot
	err := pool.QueryRow(ctx, `
SELECT
    (SELECT COUNT(*) FROM receipt_inbox_projection),
    (SELECT COUNT(*) FROM device_received_cursors),
    (SELECT COUNT(*) FROM user_received_cursors),
    (SELECT COUNT(*) FROM user_read_cursors),
    (SELECT COUNT(*) FROM message_receipt_states),
    (SELECT COUNT(*) FROM user_conversation_summaries),
    (SELECT COUNT(*) FROM user_conversation_summaries WHERE archived),
    (SELECT COUNT(*) FROM user_conversation_summaries WHERE pinned),
    (SELECT COUNT(*) FROM user_conversation_summaries WHERE muted),
    (SELECT COUNT(*) FROM user_conversation_summaries WHERE unread_count > 0),
    (SELECT COUNT(*) FROM receipt_kafka_checkpoints),
    (SELECT COUNT(DISTINCT consumer_group) FROM receipt_kafka_checkpoints),
    (SELECT COUNT(*) FROM conversation_summary_checkpoints)
`).Scan(
		&snapshot.InboxProjectionTotal,
		&snapshot.DeviceReceivedCursors,
		&snapshot.UserReceivedCursors,
		&snapshot.UserReadCursors,
		&snapshot.MessageReceiptStates,
		&snapshot.ConversationSummaries,
		&snapshot.ArchivedConversationCount,
		&snapshot.PinnedConversationCount,
		&snapshot.MutedConversationCount,
		&snapshot.UnreadConversationCount,
		&snapshot.KafkaCheckpoints,
		&snapshot.KafkaConsumerGroups,
		&snapshot.ConversationListCheckpoints,
	)
	if err != nil {
		return ReceiptSnapshot{}, err
	}
	return snapshot, nil
}

func queryReceiptOutboxSnapshot(ctx context.Context, pool *pgxpool.Pool) (OutboxSnapshot, error) {
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
FROM receipt_outbox
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
