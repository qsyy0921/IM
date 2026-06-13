package postgres

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

type OutboxStore struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

type OutboxAuditOptions struct {
	OutboxID       *int64
	EventID        string
	TenantID       string
	ConversationID string
	Status         string
	EventType      string
	Limit          int
}

type OutboxAuditRow struct {
	ID               int64
	EventID          string
	TenantID         string
	ConversationID   string
	AggregateVersion int64
	EventType        string
	Status           string
	RetryCount       int
	LastError        string
	AvailableAt      time.Time
	NextRetryAt      *time.Time
	DeadLetteredAt   *time.Time
	PublishedAt      *time.Time
	CreatedAt        time.Time
}

type OutboxRepairAuditOptions struct {
	EventID  string
	TenantID string
	Limit    int
}

type OutboxRepairAuditRow struct {
	EventID                string
	TenantID               string
	Reason                 string
	PreviousStatus         string
	PreviousRetryCount     int
	PreviousLastError      string
	PreviousDeadLetteredAt *time.Time
	RepairedAt             time.Time
}

type OutboxStoreOption func(*OutboxStore)

func NewOutboxStore(pool *pgxpool.Pool, opts ...OutboxStoreOption) *OutboxStore {
	store := &OutboxStore{
		pool: pool,
		now:  func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(store)
	}
	return store
}

func WithOutboxClock(clock func() time.Time) OutboxStoreOption {
	return func(store *OutboxStore) {
		if clock != nil {
			store.now = clock
		}
	}
}

func (store *OutboxStore) ProcessReadyBatch(
	ctx context.Context,
	limit int,
	maxAttempts int,
	retryBaseDelay time.Duration,
	publish func(context.Context, []types.OutboxMessage) []error,
) (types.OutboxRelayStats, error) {
	if store == nil || store.pool == nil {
		return types.OutboxRelayStats{}, errors.New("receipt outbox store is not configured")
	}
	if publish == nil {
		return types.OutboxRelayStats{}, errors.New("receipt outbox publish callback is not configured")
	}
	if limit <= 0 {
		limit = 500
	}
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if retryBaseDelay <= 0 {
		retryBaseDelay = time.Second
	}

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return types.OutboxRelayStats{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	messages, err := store.fetchReadyLocked(ctx, tx, limit)
	if err != nil {
		return types.OutboxRelayStats{}, err
	}
	stats := types.OutboxRelayStats{Fetched: len(messages)}
	if len(messages) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return types.OutboxRelayStats{}, types.NewDBWriteFailed(err.Error())
		}
		return stats, nil
	}

	publishErrors := publish(ctx, messages)
	if len(publishErrors) != len(messages) {
		return types.OutboxRelayStats{}, errors.New("receipt outbox batch publish result count mismatch")
	}

	now := store.now()
	publishedIDs := make([]int64, 0, len(messages))
	for index, message := range messages {
		if err := publishErrors[index]; err != nil {
			attempt := message.RetryCount + 1
			if attempt >= maxAttempts {
				if markErr := store.markDeadLettered(ctx, tx, message.ID, attempt, err.Error(), now); markErr != nil {
					return types.OutboxRelayStats{}, markErr
				}
				stats.DeadLettered++
				continue
			}
			nextRetryAt := now.Add(retryDelay(retryBaseDelay, attempt))
			if markErr := store.markRetry(ctx, tx, message.ID, attempt, err.Error(), nextRetryAt); markErr != nil {
				return types.OutboxRelayStats{}, markErr
			}
			stats.Retried++
			continue
		}
		publishedIDs = append(publishedIDs, message.ID)
	}

	if len(publishedIDs) > 0 {
		if err := store.markPublishedBatch(ctx, tx, publishedIDs, now); err != nil {
			return types.OutboxRelayStats{}, err
		}
		stats.Published = len(publishedIDs)
	}

	if err := tx.Commit(ctx); err != nil {
		return types.OutboxRelayStats{}, types.NewDBWriteFailed(err.Error())
	}
	return stats, nil
}

func (store *OutboxStore) fetchReadyLocked(ctx context.Context, tx pgx.Tx, limit int) ([]types.OutboxMessage, error) {
	rows, err := tx.Query(ctx, `
SELECT
    id,
    event_id,
    tenant_id,
    conversation_id,
    aggregate_version,
    event_type,
    event_version,
    partition_key,
    mapping_version,
    correlation_id,
    causation_id,
    producer,
    payload_json,
    trace_id,
    retry_count,
    created_at
FROM receipt_outbox current
WHERE status = 'PENDING'
  AND published_at IS NULL
  AND COALESCE(next_retry_at, available_at) <= now()
ORDER BY id
LIMIT $1
FOR UPDATE OF current SKIP LOCKED
`, limit)
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer rows.Close()

	messages := make([]types.OutboxMessage, 0)
	for rows.Next() {
		var message types.OutboxMessage
		if err := rows.Scan(
			&message.ID,
			&message.EventID,
			&message.TenantID,
			&message.ConversationID,
			&message.AggregateVersion,
			&message.EventType,
			&message.EventVersion,
			&message.PartitionKey,
			&message.MappingVersion,
			&message.CorrelationID,
			&message.CausationID,
			&message.Producer,
			&message.PayloadJSON,
			&message.TraceID,
			&message.RetryCount,
			&message.OccurredAt,
		); err != nil {
			return nil, types.NewDBWriteFailed(err.Error())
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	return messages, nil
}

func (store *OutboxStore) markPublishedBatch(ctx context.Context, tx pgx.Tx, ids []int64, publishedAt time.Time) error {
	tag, err := tx.Exec(ctx, `
UPDATE receipt_outbox
SET status = $2,
    published_at = $3,
    last_error = '',
    next_retry_at = NULL,
    dead_lettered_at = NULL,
    updated_at = now()
WHERE id = ANY($1::bigint[])
`, ids, types.OutboxStatusPublished, publishedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if tag.RowsAffected() != int64(len(ids)) {
		return types.NewDBWriteFailed("receipt outbox published row count mismatch")
	}
	return nil
}

func (store *OutboxStore) markRetry(ctx context.Context, tx pgx.Tx, id int64, retryCount int, lastError string, nextRetryAt time.Time) error {
	_, err := tx.Exec(ctx, `
UPDATE receipt_outbox
SET retry_count = $2,
    last_error = $3,
    next_retry_at = $4,
    updated_at = now()
WHERE id = $1
`, id, retryCount, lastError, nextRetryAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func (store *OutboxStore) markDeadLettered(ctx context.Context, tx pgx.Tx, id int64, retryCount int, lastError string, deadLetteredAt time.Time) error {
	_, err := tx.Exec(ctx, `
UPDATE receipt_outbox
SET status = $2,
    retry_count = $3,
    last_error = $4,
    next_retry_at = NULL,
    dead_lettered_at = $5,
    updated_at = now()
WHERE id = $1
`, id, types.OutboxStatusDLQ, retryCount, lastError, deadLetteredAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func retryDelay(base time.Duration, attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	exponent := attempt - 1
	if exponent > 10 {
		exponent = 10
	}
	return base * time.Duration(1<<exponent)
}

func (store *OutboxStore) AuditOutbox(ctx context.Context, options OutboxAuditOptions) ([]OutboxAuditRow, error) {
	if store == nil || store.pool == nil {
		return nil, errors.New("receipt outbox store is not configured")
	}
	limit := options.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	var args []any
	clauses := make([]string, 0, 6)
	if options.OutboxID != nil {
		args = append(args, *options.OutboxID)
		clauses = append(clauses, "id = $"+itoa(len(args)))
	}
	if eventID := strings.TrimSpace(options.EventID); eventID != "" {
		args = append(args, eventID)
		clauses = append(clauses, "event_id = $"+itoa(len(args)))
	}
	if tenantID := strings.TrimSpace(options.TenantID); tenantID != "" {
		args = append(args, tenantID)
		clauses = append(clauses, "tenant_id = $"+itoa(len(args)))
	}
	if conversationID := strings.TrimSpace(options.ConversationID); conversationID != "" {
		args = append(args, conversationID)
		clauses = append(clauses, "conversation_id = $"+itoa(len(args)))
	}
	if rawStatus := strings.TrimSpace(options.Status); rawStatus != "" {
		status := normalizeOutboxStatus(rawStatus)
		if status == "" {
			return nil, types.NewInvalidArgument("unsupported receipt outbox status")
		}
		args = append(args, status)
		clauses = append(clauses, "status = $"+itoa(len(args)))
	}
	if eventType := strings.TrimSpace(options.EventType); eventType != "" {
		args = append(args, eventType)
		clauses = append(clauses, "event_type = $"+itoa(len(args)))
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	args = append(args, limit)
	rows, err := store.pool.Query(ctx, `
SELECT
    id,
    event_id,
    tenant_id,
    conversation_id,
    aggregate_version,
    event_type,
    status,
    retry_count,
    last_error,
    available_at,
    next_retry_at,
    dead_lettered_at,
    published_at,
    created_at
FROM receipt_outbox
`+where+`
ORDER BY created_at DESC, id DESC
LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	result := make([]OutboxAuditRow, 0, limit)
	for rows.Next() {
		var row OutboxAuditRow
		if err := rows.Scan(
			&row.ID,
			&row.EventID,
			&row.TenantID,
			&row.ConversationID,
			&row.AggregateVersion,
			&row.EventType,
			&row.Status,
			&row.RetryCount,
			&row.LastError,
			&row.AvailableAt,
			&row.NextRetryAt,
			&row.DeadLetteredAt,
			&row.PublishedAt,
			&row.CreatedAt,
		); err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return result, nil
}

func (store *OutboxStore) RepairDLQEvents(ctx context.Context, eventIDs []string, reason string) (types.OutboxRepairStats, error) {
	if store == nil || store.pool == nil {
		return types.OutboxRepairStats{}, errors.New("receipt outbox store is not configured")
	}
	ids := normalizeEventIDs(eventIDs)
	if len(ids) == 0 {
		return types.OutboxRepairStats{}, types.NewInvalidArgument("event_ids are required")
	}
	if strings.TrimSpace(reason) == "" {
		reason = "manual receipt outbox repair"
	}

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return types.OutboxRepairStats{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var stats types.OutboxRepairStats
	err = tx.QueryRow(ctx, `
WITH requested AS (
    SELECT DISTINCT unnest($1::text[]) AS event_id
),
target AS (
    SELECT
        ro.id,
        ro.event_id,
        ro.tenant_id,
        ro.status,
        ro.retry_count,
        ro.last_error,
        ro.dead_lettered_at
    FROM receipt_outbox ro
    JOIN requested r ON r.event_id = ro.event_id
    WHERE ro.status = $3
    FOR UPDATE OF ro
),
updated AS (
    UPDATE receipt_outbox ro
    SET status = $2,
        retry_count = 0,
        last_error = '',
        next_retry_at = NULL,
        dead_lettered_at = NULL,
        available_at = now(),
        updated_at = now()
    FROM target t
    WHERE ro.id = t.id
    RETURNING ro.event_id
),
audit AS (
    INSERT INTO receipt_outbox_repair_audit (
        event_id,
        tenant_id,
        previous_status,
        previous_retry_count,
        previous_last_error,
        previous_dead_lettered_at,
        repair_reason
    )
    SELECT
        event_id,
        tenant_id,
        status,
        retry_count,
        last_error,
        dead_lettered_at,
        $4
    FROM target
    RETURNING event_id
)
SELECT
    (SELECT COUNT(*) FROM requested) AS requested,
    (SELECT COUNT(*) FROM updated) AS repaired
`, ids, types.OutboxStatusPending, types.OutboxStatusDLQ, reason).Scan(&stats.Requested, &stats.Repaired)
	if err != nil {
		return types.OutboxRepairStats{}, types.NewDBWriteFailed(err.Error())
	}
	stats.Skipped = stats.Requested - stats.Repaired
	if stats.Skipped < 0 {
		stats.Skipped = 0
	}
	if err := tx.Commit(ctx); err != nil {
		return types.OutboxRepairStats{}, types.NewDBWriteFailed(err.Error())
	}
	return stats, nil
}

func (store *OutboxStore) AuditOutboxRepairs(ctx context.Context, options OutboxRepairAuditOptions) ([]OutboxRepairAuditRow, error) {
	if store == nil || store.pool == nil {
		return nil, errors.New("receipt outbox store is not configured")
	}
	limit := options.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	var args []any
	clauses := make([]string, 0, 4)
	if eventID := strings.TrimSpace(options.EventID); eventID != "" {
		args = append(args, eventID)
		clauses = append(clauses, "event_id = $"+itoa(len(args)))
	}
	if tenantID := strings.TrimSpace(options.TenantID); tenantID != "" {
		args = append(args, tenantID)
		clauses = append(clauses, "tenant_id = $"+itoa(len(args)))
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	args = append(args, limit)
	rows, err := store.pool.Query(ctx, `
SELECT
    event_id,
    tenant_id,
    previous_status,
    previous_retry_count,
    previous_last_error,
    previous_dead_lettered_at,
    repair_reason,
    repaired_at
FROM receipt_outbox_repair_audit
`+where+`
ORDER BY repaired_at DESC, event_id, id DESC
LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	result := make([]OutboxRepairAuditRow, 0, limit)
	for rows.Next() {
		var row OutboxRepairAuditRow
		if err := rows.Scan(
			&row.EventID,
			&row.TenantID,
			&row.PreviousStatus,
			&row.PreviousRetryCount,
			&row.PreviousLastError,
			&row.PreviousDeadLetteredAt,
			&row.Reason,
			&row.RepairedAt,
		); err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return result, nil
}

func normalizeOutboxStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "":
		return ""
	case types.OutboxStatusPending:
		return types.OutboxStatusPending
	case types.OutboxStatusPublished:
		return types.OutboxStatusPublished
	case types.OutboxStatusDLQ:
		return types.OutboxStatusDLQ
	default:
		return ""
	}
}

func itoa(value int) string {
	return strconv.Itoa(value)
}

func normalizeEventIDs(eventIDs []string) []string {
	seen := make(map[string]struct{}, len(eventIDs))
	result := make([]string, 0, len(eventIDs))
	for _, eventID := range eventIDs {
		normalized := strings.TrimSpace(eventID)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}
