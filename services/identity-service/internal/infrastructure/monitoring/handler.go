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
	pool                     *pgxpool.Pool
	grpcMetrics              *GRPCMetrics
	challengeDeliveryMetrics *ChallengeDeliveryMetrics
	jwkSet                   any
}

func NewHandler(pool *pgxpool.Pool, grpcMetrics ...*GRPCMetrics) *Handler {
	handler := &Handler{pool: pool}
	if len(grpcMetrics) > 0 {
		handler.grpcMetrics = grpcMetrics[0]
	}
	return handler
}

func (h *Handler) WithJWKSet(jwkSet any) *Handler {
	h.jwkSet = jwkSet
	return h
}

func (h *Handler) WithChallengeDeliveryMetrics(metrics *ChallengeDeliveryMetrics) *Handler {
	h.challengeDeliveryMetrics = metrics
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/healthz":
		writeJSON(w, http.StatusOK, healthResponse{Service: serviceName, Status: "ok"})
	case "/readyz":
		h.handleReady(w, r)
	case "/.well-known/jwks.json", "/jwks.json":
		h.handleJWKS(w, r)
	case "/debug/metrics":
		h.handleMetrics(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) handleJWKS(w http.ResponseWriter, r *http.Request) {
	if h.jwkSet == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"keys": []any{}})
		return
	}
	writeJSON(w, http.StatusOK, h.jwkSet)
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
	if h.challengeDeliveryMetrics != nil {
		challengeDeliverySnapshot := h.challengeDeliveryMetrics.Snapshot()
		snapshot.ChallengeDelivery = &challengeDeliverySnapshot
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
		challengeDeliveryOutbox, err := queryChallengeDeliveryOutboxSnapshot(ctx, h.pool)
		if err != nil {
			snapshot.ChallengeDeliveryOutboxError = "challenge delivery outbox metrics query failed"
		} else {
			snapshot.ChallengeDeliveryOutbox = &challengeDeliveryOutbox
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
	Service                      string                           `json:"service"`
	GeneratedAtMS                int64                            `json:"generated_at_ms"`
	PGPool                       *PGPoolSnapshot                  `json:"pg_pool,omitempty"`
	Identity                     *IdentitySnapshot                `json:"identity,omitempty"`
	IdentityError                string                           `json:"identity_error,omitempty"`
	ChallengeDeliveryOutbox      *ChallengeDeliveryOutboxSnapshot `json:"challenge_delivery_outbox,omitempty"`
	ChallengeDeliveryOutboxError string                           `json:"challenge_delivery_outbox_error,omitempty"`
	GRPC                         *GRPCSnapshot                    `json:"grpc,omitempty"`
	ChallengeDelivery            *ChallengeDeliverySnapshot       `json:"challenge_delivery,omitempty"`
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
	Users                        int64 `json:"users"`
	UsersWithFailures            int64 `json:"users_with_failures"`
	PasswordLoginLocked          int64 `json:"password_login_locked"`
	MFARecoveryFailures          int64 `json:"mfa_recovery_failures"`
	MFARecoveryLocked            int64 `json:"mfa_recovery_locked"`
	MFAFactors                   int64 `json:"mfa_factors"`
	MFAFactorsWithFailures       int64 `json:"mfa_factors_with_failures"`
	MFALoginLocked               int64 `json:"mfa_login_locked"`
	ChallengeRequestLimits       int64 `json:"challenge_request_limits"`
	ChallengeRequestLimitsLocked int64 `json:"challenge_request_limits_locked"`
	ActiveDevices                int64 `json:"active_devices"`
	RevokedDevices               int64 `json:"revoked_devices"`
	ActiveSessions               int64 `json:"active_sessions"`
	RevokedSessions              int64 `json:"revoked_sessions"`
	ExpiredSessions              int64 `json:"expired_sessions"`
}

type ChallengeDeliveryOutboxSnapshot struct {
	Total            int64            `json:"total"`
	Pending          int64            `json:"pending"`
	PendingReady     int64            `json:"pending_ready"`
	PendingScheduled int64            `json:"pending_scheduled"`
	PendingExpired   int64            `json:"pending_expired"`
	Delivered        int64            `json:"delivered"`
	DLQ              int64            `json:"dlq"`
	Canceled         int64            `json:"canceled"`
	MaxPendingRetry  int64            `json:"max_pending_retry"`
	FailureClasses   map[string]int64 `json:"failure_classes,omitempty"`
}

func queryIdentitySnapshot(ctx context.Context, pool *pgxpool.Pool) (IdentitySnapshot, error) {
	var snapshot IdentitySnapshot
	err := pool.QueryRow(ctx, `
SELECT
    (SELECT COUNT(*) FROM identity_users),
    (SELECT COUNT(*) FROM identity_users WHERE failed_login_count > 0),
    (SELECT COUNT(*) FROM identity_users WHERE locked_until > now()),
    (SELECT COUNT(*) FROM identity_users WHERE mfa_recovery_failed_count > 0),
    (SELECT COUNT(*) FROM identity_users WHERE mfa_recovery_locked_until > now()),
    (SELECT COUNT(*) FROM identity_mfa_factors),
    (SELECT COUNT(*) FROM identity_mfa_factors WHERE login_failed_count > 0),
    (SELECT COUNT(*) FROM identity_mfa_factors WHERE status = 'ACTIVE' AND login_locked_until > now()),
    (SELECT COUNT(*) FROM identity_challenge_request_limits),
    (SELECT COUNT(*) FROM identity_challenge_request_limits WHERE locked_until > now()),
    (SELECT COUNT(*) FROM identity_devices WHERE status = 'ACTIVE'),
    (SELECT COUNT(*) FROM identity_devices WHERE status = 'REVOKED'),
    (SELECT COUNT(*) FROM identity_sessions WHERE status = 'ACTIVE' AND expires_at > now()),
    (SELECT COUNT(*) FROM identity_sessions WHERE status = 'REVOKED'),
    (SELECT COUNT(*) FROM identity_sessions WHERE status = 'ACTIVE' AND expires_at <= now())
`).Scan(
		&snapshot.Users,
		&snapshot.UsersWithFailures,
		&snapshot.PasswordLoginLocked,
		&snapshot.MFARecoveryFailures,
		&snapshot.MFARecoveryLocked,
		&snapshot.MFAFactors,
		&snapshot.MFAFactorsWithFailures,
		&snapshot.MFALoginLocked,
		&snapshot.ChallengeRequestLimits,
		&snapshot.ChallengeRequestLimitsLocked,
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

func queryChallengeDeliveryOutboxSnapshot(ctx context.Context, pool *pgxpool.Pool) (ChallengeDeliveryOutboxSnapshot, error) {
	var snapshot ChallengeDeliveryOutboxSnapshot
	err := pool.QueryRow(ctx, `
SELECT
    COUNT(*),
    COUNT(*) FILTER (WHERE status = 'PENDING'),
    COUNT(*) FILTER (WHERE status = 'PENDING' AND expires_at > now() AND COALESCE(next_retry_at, available_at) <= now()),
    COUNT(*) FILTER (WHERE status = 'PENDING' AND expires_at > now() AND COALESCE(next_retry_at, available_at) > now()),
    COUNT(*) FILTER (WHERE status = 'PENDING' AND expires_at <= now()),
    COUNT(*) FILTER (WHERE status = 'DELIVERED'),
    COUNT(*) FILTER (WHERE status = 'DLQ'),
    COUNT(*) FILTER (WHERE status = 'CANCELED'),
    COALESCE(MAX(retry_count) FILTER (WHERE status = 'PENDING'), 0)
FROM identity_challenge_delivery_outbox
`).Scan(
		&snapshot.Total,
		&snapshot.Pending,
		&snapshot.PendingReady,
		&snapshot.PendingScheduled,
		&snapshot.PendingExpired,
		&snapshot.Delivered,
		&snapshot.DLQ,
		&snapshot.Canceled,
		&snapshot.MaxPendingRetry,
	)
	if err != nil {
		return ChallengeDeliveryOutboxSnapshot{}, err
	}
	rows, err := pool.Query(ctx, `
SELECT failure_class, COUNT(*)
FROM identity_challenge_delivery_outbox
WHERE failure_class <> ''
GROUP BY failure_class
ORDER BY failure_class
`)
	if err != nil {
		return ChallengeDeliveryOutboxSnapshot{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var failureClass string
		var count int64
		if err := rows.Scan(&failureClass, &count); err != nil {
			return ChallengeDeliveryOutboxSnapshot{}, err
		}
		if snapshot.FailureClasses == nil {
			snapshot.FailureClasses = make(map[string]int64)
		}
		snapshot.FailureClasses[failureClass] = count
	}
	if err := rows.Err(); err != nil {
		return ChallengeDeliveryOutboxSnapshot{}, err
	}
	return snapshot, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
