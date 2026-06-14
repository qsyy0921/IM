package monitoring

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type Handler struct {
	pool                   *pgxpool.Pool
	grpcMetrics            *GRPCMetrics
	memberChangeWorkerFunc func() types.MemberChangeWorkerSnapshot
	traceStatsFunc         func() TraceSnapshot
}

func NewHandler(pool *pgxpool.Pool, grpcMetrics ...*GRPCMetrics) *Handler {
	handler := &Handler{pool: pool}
	if len(grpcMetrics) > 0 {
		handler.grpcMetrics = grpcMetrics[0]
	}
	return handler
}

func (h *Handler) WithMemberChangeWorkerStats(snapshotFunc func() types.MemberChangeWorkerSnapshot) *Handler {
	h.memberChangeWorkerFunc = snapshotFunc
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
	if h.memberChangeWorkerFunc != nil {
		workerSnapshot := h.memberChangeWorkerFunc()
		snapshot.MemberChangeWorker = &workerSnapshot
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
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		conversation, err := queryConversationSnapshot(ctx, h.pool)
		if err != nil {
			snapshot.ConversationError = "conversation metrics query failed"
		} else {
			snapshot.Conversation = &conversation
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
	Service            string                            `json:"service"`
	GeneratedAtMS      int64                             `json:"generated_at_ms"`
	PGPool             *PGPoolSnapshot                   `json:"pg_pool,omitempty"`
	Conversation       *ConversationSnapshot             `json:"conversation,omitempty"`
	ConversationError  string                            `json:"conversation_error,omitempty"`
	GRPC               *GRPCSnapshot                     `json:"grpc,omitempty"`
	MemberChangeWorker *types.MemberChangeWorkerSnapshot `json:"member_change_worker,omitempty"`
	Trace              *TraceSnapshot                    `json:"trace,omitempty"`
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

type ConversationSnapshot struct {
	Conversations *ConversationStoreSnapshot  `json:"conversations,omitempty"`
	Members       *ConversationMemberSnapshot `json:"members,omitempty"`
	MemberChanges *MemberChangeSagaSnapshot   `json:"member_changes,omitempty"`
}

type ConversationStoreSnapshot struct {
	Total    int64                `json:"total"`
	Active   int64                `json:"active"`
	Archived int64                `json:"archived"`
	Deleted  int64                `json:"deleted"`
	ByType   []GroupCountSnapshot `json:"by_type"`
	ByStatus []GroupCountSnapshot `json:"by_status"`
}

type ConversationMemberSnapshot struct {
	Total        int64                `json:"total"`
	Active       int64                `json:"active"`
	Left         int64                `json:"left"`
	Banned       int64                `json:"banned"`
	ByRole       []GroupCountSnapshot `json:"by_role"`
	ByStatus     []GroupCountSnapshot `json:"by_status"`
	ByRoleStatus []RoleStatusCount    `json:"by_role_status"`
}

type MemberChangeSagaSnapshot struct {
	Total             int64                `json:"total"`
	PendingBoundary   int64                `json:"pending_boundary"`
	BoundaryAllocated int64                `json:"boundary_allocated"`
	MemberUpdated     int64                `json:"member_updated"`
	OutboxEnqueued    int64                `json:"outbox_enqueued"`
	EventPublished    int64                `json:"event_published"`
	Done              int64                `json:"done"`
	FailedCompensated int64                `json:"failed_compensated"`
	ByStatus          []GroupCountSnapshot `json:"by_status"`
}

type GroupCountSnapshot struct {
	Value string `json:"value"`
	Total int64  `json:"total"`
}

type RoleStatusCount struct {
	Role   string `json:"role"`
	Status string `json:"status"`
	Total  int64  `json:"total"`
}

func queryConversationSnapshot(ctx context.Context, pool *pgxpool.Pool) (ConversationSnapshot, error) {
	var snapshot ConversationSnapshot

	conversations, err := queryConversationStoreSnapshot(ctx, pool)
	if err != nil {
		return ConversationSnapshot{}, err
	}
	snapshot.Conversations = &conversations

	members, err := queryConversationMemberSnapshot(ctx, pool)
	if err != nil {
		return ConversationSnapshot{}, err
	}
	snapshot.Members = &members

	memberChanges, err := queryMemberChangeSagaSnapshot(ctx, pool)
	if err != nil {
		return ConversationSnapshot{}, err
	}
	snapshot.MemberChanges = &memberChanges

	return snapshot, nil
}

func queryConversationStoreSnapshot(ctx context.Context, pool *pgxpool.Pool) (ConversationStoreSnapshot, error) {
	var snapshot ConversationStoreSnapshot
	err := pool.QueryRow(ctx, `
SELECT
    COUNT(*),
    COUNT(*) FILTER (WHERE status = 'ACTIVE'),
    COUNT(*) FILTER (WHERE status = 'ARCHIVED'),
    COUNT(*) FILTER (WHERE status = 'DELETED')
FROM conversations
`).Scan(
		&snapshot.Total,
		&snapshot.Active,
		&snapshot.Archived,
		&snapshot.Deleted,
	)
	if err != nil {
		return ConversationStoreSnapshot{}, err
	}
	byType, err := queryGroupCounts(ctx, pool, `
SELECT conversation_type, COUNT(*)
FROM conversations
GROUP BY conversation_type
ORDER BY conversation_type
`)
	if err != nil {
		return ConversationStoreSnapshot{}, err
	}
	snapshot.ByType = byType
	byStatus, err := queryGroupCounts(ctx, pool, `
SELECT status, COUNT(*)
FROM conversations
GROUP BY status
ORDER BY status
`)
	if err != nil {
		return ConversationStoreSnapshot{}, err
	}
	snapshot.ByStatus = byStatus
	return snapshot, nil
}

func queryConversationMemberSnapshot(ctx context.Context, pool *pgxpool.Pool) (ConversationMemberSnapshot, error) {
	var snapshot ConversationMemberSnapshot
	err := pool.QueryRow(ctx, `
SELECT
    COUNT(*),
    COUNT(*) FILTER (WHERE status = 'ACTIVE'),
    COUNT(*) FILTER (WHERE status = 'LEFT'),
    COUNT(*) FILTER (WHERE status = 'BANNED')
FROM conversation_members
`).Scan(
		&snapshot.Total,
		&snapshot.Active,
		&snapshot.Left,
		&snapshot.Banned,
	)
	if err != nil {
		return ConversationMemberSnapshot{}, err
	}
	byRole, err := queryGroupCounts(ctx, pool, `
SELECT role, COUNT(*)
FROM conversation_members
GROUP BY role
ORDER BY role
`)
	if err != nil {
		return ConversationMemberSnapshot{}, err
	}
	snapshot.ByRole = byRole
	byStatus, err := queryGroupCounts(ctx, pool, `
SELECT status, COUNT(*)
FROM conversation_members
GROUP BY status
ORDER BY status
`)
	if err != nil {
		return ConversationMemberSnapshot{}, err
	}
	snapshot.ByStatus = byStatus
	byRoleStatus, err := queryRoleStatusCounts(ctx, pool, `
SELECT role, status, COUNT(*)
FROM conversation_members
GROUP BY role, status
ORDER BY role, status
`)
	if err != nil {
		return ConversationMemberSnapshot{}, err
	}
	snapshot.ByRoleStatus = byRoleStatus
	return snapshot, nil
}

func queryMemberChangeSagaSnapshot(ctx context.Context, pool *pgxpool.Pool) (MemberChangeSagaSnapshot, error) {
	var snapshot MemberChangeSagaSnapshot
	err := pool.QueryRow(ctx, `
SELECT
    COUNT(*),
    COUNT(*) FILTER (WHERE status = 'PENDING_BOUNDARY'),
    COUNT(*) FILTER (WHERE status = 'BOUNDARY_ALLOCATED'),
    COUNT(*) FILTER (WHERE status = 'MEMBER_UPDATED'),
    COUNT(*) FILTER (WHERE status = 'OUTBOX_ENQUEUED'),
    COUNT(*) FILTER (WHERE status = 'EVENT_PUBLISHED'),
    COUNT(*) FILTER (WHERE status = 'DONE'),
    COUNT(*) FILTER (WHERE status = 'FAILED_COMPENSATED')
FROM member_change_saga
`).Scan(
		&snapshot.Total,
		&snapshot.PendingBoundary,
		&snapshot.BoundaryAllocated,
		&snapshot.MemberUpdated,
		&snapshot.OutboxEnqueued,
		&snapshot.EventPublished,
		&snapshot.Done,
		&snapshot.FailedCompensated,
	)
	if err != nil {
		return MemberChangeSagaSnapshot{}, err
	}
	byStatus, err := queryGroupCounts(ctx, pool, `
SELECT status, COUNT(*)
FROM member_change_saga
GROUP BY status
ORDER BY status
`)
	if err != nil {
		return MemberChangeSagaSnapshot{}, err
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

func queryRoleStatusCounts(ctx context.Context, pool *pgxpool.Pool, query string) ([]RoleStatusCount, error) {
	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := make([]RoleStatusCount, 0)
	for rows.Next() {
		var entry RoleStatusCount
		if err := rows.Scan(&entry.Role, &entry.Status, &entry.Total); err != nil {
			return nil, err
		}
		values = append(values, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
