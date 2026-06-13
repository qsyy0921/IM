package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const serviceName = "policy-service"

type Handler struct {
	pool            *pgxpool.Pool
	requirePostgres bool
	grpcMetrics     *GRPCMetrics
	decisionMetrics *DecisionMetrics
}

func NewHandler(pool *pgxpool.Pool, requirePostgres bool, grpcMetrics *GRPCMetrics, decisionMetrics *DecisionMetrics) *Handler {
	return &Handler{
		pool:            pool,
		requirePostgres: requirePostgres,
		grpcMetrics:     grpcMetrics,
		decisionMetrics: decisionMetrics,
	}
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
		if h.requirePostgres {
			writeJSON(w, http.StatusServiceUnavailable, healthResponse{Service: serviceName, Status: "unready", Error: "postgres pool is not configured"})
			return
		}
		writeJSON(w, http.StatusOK, healthResponse{Service: serviceName, Status: "ready"})
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
	if h.decisionMetrics != nil {
		decisionSnapshot := h.decisionMetrics.Snapshot()
		snapshot.Decisions = &decisionSnapshot
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
		rules, err := queryRuleSnapshot(ctx, h.pool)
		if err != nil {
			snapshot.RuleStoreError = "policy rule metrics query failed"
		} else {
			snapshot.RuleStore = &rules
		}
		auditOutbox, err := queryAuditOutboxSnapshot(ctx, h.pool)
		if err != nil {
			snapshot.AuditOutboxError = "policy audit outbox metrics query failed"
		} else {
			snapshot.AuditOutbox = &auditOutbox
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
	Service          string               `json:"service"`
	GeneratedAtMS    int64                `json:"generated_at_ms"`
	PGPool           *PGPoolSnapshot      `json:"pg_pool,omitempty"`
	RuleStore        *RuleSnapshot        `json:"policy_rule_store,omitempty"`
	RuleStoreError   string               `json:"policy_rule_store_error,omitempty"`
	AuditOutbox      *AuditOutboxSnapshot `json:"policy_decision_audit_outbox,omitempty"`
	AuditOutboxError string               `json:"policy_decision_audit_outbox_error,omitempty"`
	GRPC             *GRPCSnapshot        `json:"grpc,omitempty"`
	Decisions        *DecisionSnapshot    `json:"decisions,omitempty"`
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

type RuleSnapshot struct {
	Total                   int64                 `json:"total"`
	Allow                   int64                 `json:"allow"`
	Deny                    int64                 `json:"deny"`
	Actions                 []RuleActionSnapshot  `json:"actions"`
	ExactMessageActions     *RuleDecisionSnapshot `json:"exact_message_action_rules,omitempty"`
	TenantMessageActions    *RuleDecisionSnapshot `json:"tenant_message_action_rules,omitempty"`
	ConversationRoleActions *RuleRoleSnapshot     `json:"conversation_role_action_rules,omitempty"`
	OwnershipOverrides      *RuleRoleSnapshot     `json:"message_ownership_override_rules,omitempty"`
}

type RuleDecisionSnapshot struct {
	Total   int64                `json:"total"`
	Allow   int64                `json:"allow"`
	Deny    int64                `json:"deny"`
	Actions []RuleActionSnapshot `json:"actions"`
}

type RuleActionSnapshot struct {
	Action string `json:"action"`
	Total  int64  `json:"total"`
	Allow  int64  `json:"allow"`
	Deny   int64  `json:"deny"`
}

type RuleRoleSnapshot struct {
	Total   int64                    `json:"total"`
	Actions []RuleRoleActionSnapshot `json:"actions"`
}

type RuleRoleActionSnapshot struct {
	Action  string `json:"action"`
	MinRole string `json:"min_role"`
	Total   int64  `json:"total"`
}

type AuditOutboxSnapshot struct {
	Total     int64 `json:"total"`
	Pending   int64 `json:"pending"`
	Published int64 `json:"published"`
	DLQ       int64 `json:"dlq"`
}

func queryRuleSnapshot(ctx context.Context, pool *pgxpool.Pool) (RuleSnapshot, error) {
	var snapshot RuleSnapshot
	exact, err := queryExactMessageActionRules(ctx, pool)
	if err != nil {
		return RuleSnapshot{}, err
	}
	snapshot.Total = exact.Total
	snapshot.Allow = exact.Allow
	snapshot.Deny = exact.Deny
	snapshot.Actions = exact.Actions
	snapshot.ExactMessageActions = &exact

	tenant, err := queryTenantMessageActionRules(ctx, pool)
	if err != nil {
		if !isUndefinedTable(err) {
			return RuleSnapshot{}, err
		}
	} else {
		snapshot.TenantMessageActions = &tenant
	}

	role, err := queryConversationRoleRules(ctx, pool)
	if err != nil {
		if !isUndefinedTable(err) {
			return RuleSnapshot{}, err
		}
	} else {
		snapshot.ConversationRoleActions = &role
	}

	ownership, err := queryOwnershipOverrideRules(ctx, pool)
	if err != nil {
		if !isUndefinedTable(err) {
			return RuleSnapshot{}, err
		}
	} else {
		snapshot.OwnershipOverrides = &ownership
	}

	return snapshot, nil
}

func queryExactMessageActionRules(ctx context.Context, pool *pgxpool.Pool) (RuleDecisionSnapshot, error) {
	return queryDecisionRuleSnapshot(ctx, pool, `
SELECT
    COUNT(*),
    COUNT(*) FILTER (WHERE allowed),
    COUNT(*) FILTER (WHERE NOT allowed)
FROM policy_message_action_rules
`, `
SELECT
    action,
    COUNT(*),
    COUNT(*) FILTER (WHERE allowed),
    COUNT(*) FILTER (WHERE NOT allowed)
FROM policy_message_action_rules
GROUP BY action
ORDER BY action
`)
}

func queryTenantMessageActionRules(ctx context.Context, pool *pgxpool.Pool) (RuleDecisionSnapshot, error) {
	return queryDecisionRuleSnapshot(ctx, pool, `
SELECT
    COUNT(*),
    COUNT(*) FILTER (WHERE allowed),
    COUNT(*) FILTER (WHERE NOT allowed)
FROM policy_tenant_message_action_rules
`, `
SELECT
    action,
    COUNT(*),
    COUNT(*) FILTER (WHERE allowed),
    COUNT(*) FILTER (WHERE NOT allowed)
FROM policy_tenant_message_action_rules
GROUP BY action
ORDER BY action
`)
}

func queryDecisionRuleSnapshot(ctx context.Context, pool *pgxpool.Pool, totalQuery string, actionQuery string) (RuleDecisionSnapshot, error) {
	var snapshot RuleDecisionSnapshot
	if err := pool.QueryRow(ctx, totalQuery).Scan(&snapshot.Total, &snapshot.Allow, &snapshot.Deny); err != nil {
		return RuleDecisionSnapshot{}, err
	}
	rows, err := pool.Query(ctx, actionQuery)
	if err != nil {
		return RuleDecisionSnapshot{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var action RuleActionSnapshot
		if err := rows.Scan(&action.Action, &action.Total, &action.Allow, &action.Deny); err != nil {
			return RuleDecisionSnapshot{}, err
		}
		snapshot.Actions = append(snapshot.Actions, action)
	}
	if err := rows.Err(); err != nil {
		return RuleDecisionSnapshot{}, err
	}
	return snapshot, nil
}

func queryConversationRoleRules(ctx context.Context, pool *pgxpool.Pool) (RuleRoleSnapshot, error) {
	return queryRoleRuleSnapshot(ctx, pool, `
SELECT COUNT(*)
FROM policy_conversation_role_action_rules
`, `
SELECT action, min_role, COUNT(*)
FROM policy_conversation_role_action_rules
GROUP BY action, min_role
ORDER BY action, min_role
`)
}

func queryOwnershipOverrideRules(ctx context.Context, pool *pgxpool.Pool) (RuleRoleSnapshot, error) {
	return queryRoleRuleSnapshot(ctx, pool, `
SELECT COUNT(*)
FROM policy_message_ownership_override_rules
`, `
SELECT action, min_role, COUNT(*)
FROM policy_message_ownership_override_rules
GROUP BY action, min_role
ORDER BY action, min_role
`)
}

func queryRoleRuleSnapshot(ctx context.Context, pool *pgxpool.Pool, totalQuery string, actionQuery string) (RuleRoleSnapshot, error) {
	var snapshot RuleRoleSnapshot
	if err := pool.QueryRow(ctx, totalQuery).Scan(&snapshot.Total); err != nil {
		return RuleRoleSnapshot{}, err
	}
	rows, err := pool.Query(ctx, actionQuery)
	if err != nil {
		return RuleRoleSnapshot{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var action RuleRoleActionSnapshot
		if err := rows.Scan(&action.Action, &action.MinRole, &action.Total); err != nil {
			return RuleRoleSnapshot{}, err
		}
		snapshot.Actions = append(snapshot.Actions, action)
	}
	if err := rows.Err(); err != nil {
		return RuleRoleSnapshot{}, err
	}
	return snapshot, nil
}

func isUndefinedTable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42P01"
}

func queryAuditOutboxSnapshot(ctx context.Context, pool *pgxpool.Pool) (AuditOutboxSnapshot, error) {
	var snapshot AuditOutboxSnapshot
	if err := pool.QueryRow(ctx, `
SELECT
    COUNT(*),
    COUNT(*) FILTER (WHERE status = 'PENDING'),
    COUNT(*) FILTER (WHERE status = 'PUBLISHED'),
    COUNT(*) FILTER (WHERE status = 'DLQ')
FROM policy_decision_audit_outbox
`).Scan(&snapshot.Total, &snapshot.Pending, &snapshot.Published, &snapshot.DLQ); err != nil {
		return AuditOutboxSnapshot{}, err
	}
	return snapshot, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
