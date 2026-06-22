package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

type OutboxStore struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

type OutboxRepairAuditOptions struct {
	OutboxID       *int64
	EventID        string
	TenantID       string
	ConversationID string
	Mode           string
	Outcome        string
	RepairedAfter  *time.Time
	RepairedBefore *time.Time
	Limit          int
}

type OutboxAuditOptions struct {
	OutboxID       *int64
	EventID        string
	TenantID       string
	ConversationID string
	Status         string
	EventType      string
	CreatedAfter   *time.Time
	CreatedBefore  *time.Time
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

type OutboxRepairCleanupOptions struct {
	OutboxID       *int64
	EventID        string
	TenantID       string
	ConversationID string
	Mode           string
	Outcome        string
	Cutoff         time.Time
	Limit          int
	DryRun         bool
}

type OutboxRepairCleanupStats struct {
	Deleted int64
}

type OutboxRepairAuditRow struct {
	OutboxID             int64
	EventID              string
	TenantID             string
	ConversationID       string
	AggregateVersion     int64
	Mode                 string
	Outcome              string
	SkipReason           string
	Operator             string
	Reason               string
	DryRun               bool
	BeforeStatus         string
	BeforeRetryCount     int
	BeforeLastError      string
	BeforeNextRetryAt    *time.Time
	BeforeDeadLetteredAt *time.Time
	AfterStatus          string
	AfterRetryCount      int
	AfterLastError       string
	AfterNextRetryAt     *time.Time
	AfterDeadLetteredAt  *time.Time
	CreatedAt            time.Time
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
		return types.OutboxRelayStats{}, errors.New("delivery outbox store is not configured")
	}
	if publish == nil {
		return types.OutboxRelayStats{}, errors.New("delivery outbox publish callback is not configured")
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
		return types.OutboxRelayStats{}, errors.New("delivery outbox batch publish result count mismatch")
	}

	now := store.now()
	publishedIDs := make([]int64, 0, len(messages))
	for index, message := range messages {
		if err := publishErrors[index]; err != nil {
			lastError := sanitizeDeliveryOutboxPublishError(err)
			attempt := message.RetryCount + 1
			if attempt >= maxAttempts {
				if markErr := store.markDeadLettered(ctx, tx, message.ID, attempt, lastError, now); markErr != nil {
					return types.OutboxRelayStats{}, markErr
				}
				stats.DeadLettered++
				continue
			}
			nextRetryAt := now.Add(retryDelay(retryBaseDelay, attempt))
			if markErr := store.markRetry(ctx, tx, message.ID, attempt, lastError, nextRetryAt); markErr != nil {
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
FROM delivery_outbox current
WHERE status = 'PENDING'
  AND published_at IS NULL
  AND COALESCE(next_retry_at, available_at) <= now()
  AND NOT EXISTS (
      SELECT 1
      FROM delivery_outbox previous
      WHERE previous.tenant_id = current.tenant_id
        AND previous.conversation_id = current.conversation_id
        AND previous.aggregate_version < current.aggregate_version
        AND previous.status IN ('PENDING', 'DLQ')
  )
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
UPDATE delivery_outbox
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
		return types.NewDBWriteFailed("delivery outbox published row count mismatch")
	}
	return nil
}

func (store *OutboxStore) markRetry(ctx context.Context, tx pgx.Tx, id int64, retryCount int, lastError string, nextRetryAt time.Time) error {
	_, err := tx.Exec(ctx, `
UPDATE delivery_outbox
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
UPDATE delivery_outbox
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

func sanitizeDeliveryOutboxPublishError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "delivery outbox publish canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "delivery outbox publish timeout"
	}
	return sanitizeDeliveryOutboxPublishErrorText(err.Error())
}

func sanitizeDeliveryOutboxStoredError(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return sanitizeDeliveryOutboxPublishErrorText(value)
}

func sanitizeDeliveryOutboxPublishErrorText(value string) string {
	message := strings.ToLower(strings.TrimSpace(value))
	switch {
	case message == "":
		return "delivery outbox publish failed"
	case strings.Contains(message, "cancel"):
		return "delivery outbox publish canceled"
	case strings.Contains(message, "timeout") || strings.Contains(message, "deadline exceeded"):
		return "delivery outbox publish timeout"
	case strings.Contains(message, "unsupported"):
		return "delivery outbox publish unsupported event"
	case strings.Contains(message, "malformed") ||
		strings.Contains(message, "invalid") ||
		strings.Contains(message, "json") ||
		strings.Contains(message, "decode") ||
		strings.Contains(message, "payload"):
		return "delivery outbox publish invalid payload"
	case strings.Contains(message, "kafka") ||
		strings.Contains(message, "broker") ||
		strings.Contains(message, "leader") ||
		strings.Contains(message, "connection refused") ||
		strings.Contains(message, "no such host") ||
		strings.Contains(message, "network"):
		return "delivery outbox publish broker unavailable"
	default:
		return "delivery outbox publish failed"
	}
}

func (store *OutboxStore) AuditOutbox(ctx context.Context, options OutboxAuditOptions) ([]OutboxAuditRow, error) {
	if store == nil || store.pool == nil {
		return nil, errors.New("delivery outbox store is not configured")
	}
	limit := options.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	if options.CreatedAfter != nil && options.CreatedBefore != nil && !options.CreatedAfter.Before(*options.CreatedBefore) {
		return nil, types.NewInvalidArgument("created_after must be before created_before")
	}

	var args []any
	clauses := make([]string, 0, 8)
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
			return nil, types.NewInvalidArgument("unsupported delivery outbox status")
		}
		args = append(args, status)
		clauses = append(clauses, "status = $"+itoa(len(args)))
	}
	if eventType := strings.TrimSpace(options.EventType); eventType != "" {
		args = append(args, eventType)
		clauses = append(clauses, "event_type = $"+itoa(len(args)))
	}
	if options.CreatedAfter != nil {
		args = append(args, options.CreatedAfter.UTC())
		clauses = append(clauses, "created_at >= $"+itoa(len(args)))
	}
	if options.CreatedBefore != nil {
		args = append(args, options.CreatedBefore.UTC())
		clauses = append(clauses, "created_at < $"+itoa(len(args)))
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
FROM delivery_outbox
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
		row.LastError = sanitizeDeliveryOutboxStoredError(row.LastError)
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return result, nil
}

func (store *OutboxStore) AuditOutboxRepairs(ctx context.Context, options OutboxRepairAuditOptions) ([]OutboxRepairAuditRow, error) {
	if store == nil || store.pool == nil {
		return nil, errors.New("delivery outbox store is not configured")
	}
	if options.RepairedAfter != nil && options.RepairedBefore != nil && !options.RepairedAfter.Before(*options.RepairedBefore) {
		return nil, types.NewInvalidArgument("repaired_after must be before repaired_before")
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
		clauses = append(clauses, "outbox_id = $"+itoa(len(args)))
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
	if rawMode := strings.TrimSpace(options.Mode); rawMode != "" {
		mode := normalizeOutboxRepairMode(rawMode)
		if mode == "" {
			return nil, types.NewInvalidArgument("unsupported delivery outbox repair mode")
		}
		args = append(args, mode)
		clauses = append(clauses, "mode = $"+itoa(len(args)))
	}
	if rawOutcome := strings.TrimSpace(options.Outcome); rawOutcome != "" {
		outcome := normalizeOutboxRepairOutcome(rawOutcome)
		if outcome == "" {
			return nil, types.NewInvalidArgument("unsupported delivery outbox repair outcome")
		}
		args = append(args, outcome)
		clauses = append(clauses, "outcome = $"+itoa(len(args)))
	}
	if options.RepairedAfter != nil {
		args = append(args, options.RepairedAfter.UTC())
		clauses = append(clauses, "created_at >= $"+itoa(len(args)))
	}
	if options.RepairedBefore != nil {
		args = append(args, options.RepairedBefore.UTC())
		clauses = append(clauses, "created_at < $"+itoa(len(args)))
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	args = append(args, limit)
	rows, err := store.pool.Query(ctx, `
SELECT
    outbox_id,
    event_id,
    tenant_id,
    conversation_id,
    aggregate_version,
    mode,
    outcome,
    skip_reason,
    operator,
    reason,
    dry_run,
    before_status,
    before_retry_count,
    before_last_error,
    before_next_retry_at,
    before_dead_lettered_at,
    after_status,
    after_retry_count,
    after_last_error,
    after_next_retry_at,
    after_dead_lettered_at,
    created_at
FROM delivery_outbox_repair_audit
`+where+`
ORDER BY created_at DESC, outbox_id, id DESC
LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	result := make([]OutboxRepairAuditRow, 0, limit)
	for rows.Next() {
		var row OutboxRepairAuditRow
		if err := rows.Scan(
			&row.OutboxID,
			&row.EventID,
			&row.TenantID,
			&row.ConversationID,
			&row.AggregateVersion,
			&row.Mode,
			&row.Outcome,
			&row.SkipReason,
			&row.Operator,
			&row.Reason,
			&row.DryRun,
			&row.BeforeStatus,
			&row.BeforeRetryCount,
			&row.BeforeLastError,
			&row.BeforeNextRetryAt,
			&row.BeforeDeadLetteredAt,
			&row.AfterStatus,
			&row.AfterRetryCount,
			&row.AfterLastError,
			&row.AfterNextRetryAt,
			&row.AfterDeadLetteredAt,
			&row.CreatedAt,
		); err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		row.BeforeLastError = sanitizeDeliveryOutboxStoredError(row.BeforeLastError)
		row.AfterLastError = sanitizeDeliveryOutboxStoredError(row.AfterLastError)
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return result, nil
}

func (store *OutboxStore) CleanupOutboxRepairs(ctx context.Context, options OutboxRepairCleanupOptions) (OutboxRepairCleanupStats, error) {
	if store == nil || store.pool == nil {
		return OutboxRepairCleanupStats{}, errors.New("delivery outbox store is not configured")
	}
	if options.Limit <= 0 {
		return OutboxRepairCleanupStats{}, nil
	}

	var args []any
	clauses := []string{"created_at < $1"}
	args = append(args, options.Cutoff)
	if options.OutboxID != nil {
		args = append(args, *options.OutboxID)
		clauses = append(clauses, "outbox_id = $"+itoa(len(args)))
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
	if rawMode := strings.TrimSpace(options.Mode); rawMode != "" {
		mode := normalizeOutboxRepairMode(rawMode)
		if mode == "" {
			return OutboxRepairCleanupStats{}, types.NewInvalidArgument("unsupported delivery outbox repair mode")
		}
		args = append(args, mode)
		clauses = append(clauses, "mode = $"+itoa(len(args)))
	}
	if rawOutcome := strings.TrimSpace(options.Outcome); rawOutcome != "" {
		outcome := normalizeOutboxRepairOutcome(rawOutcome)
		if outcome == "" {
			return OutboxRepairCleanupStats{}, types.NewInvalidArgument("unsupported delivery outbox repair outcome")
		}
		args = append(args, outcome)
		clauses = append(clauses, "outcome = $"+itoa(len(args)))
	}
	args = append(args, options.Limit)
	if options.DryRun {
		var stats OutboxRepairCleanupStats
		err := store.pool.QueryRow(ctx, `
WITH doomed AS (
    SELECT id
    FROM delivery_outbox_repair_audit
    WHERE `+strings.Join(clauses, " AND ")+`
    ORDER BY created_at ASC, outbox_id ASC, id ASC
    LIMIT $`+itoa(len(args))+`
)
SELECT count(*) FROM doomed
`, args...).Scan(&stats.Deleted)
		if err != nil {
			return OutboxRepairCleanupStats{}, types.NewDBReadFailed(err.Error())
		}
		return stats, nil
	}
	rows, err := store.pool.Query(ctx, `
WITH doomed AS (
    SELECT id
    FROM delivery_outbox_repair_audit
    WHERE `+strings.Join(clauses, " AND ")+`
    ORDER BY created_at ASC, outbox_id ASC, id ASC
    LIMIT $`+itoa(len(args))+`
    FOR UPDATE SKIP LOCKED
)
DELETE FROM delivery_outbox_repair_audit target
USING doomed
WHERE target.id = doomed.id
RETURNING 1
`, args...)
	if err != nil {
		return OutboxRepairCleanupStats{}, types.NewDBWriteFailed(err.Error())
	}
	defer rows.Close()

	var stats OutboxRepairCleanupStats
	for rows.Next() {
		stats.Deleted++
	}
	if err := rows.Err(); err != nil {
		return OutboxRepairCleanupStats{}, types.NewDBWriteFailed(err.Error())
	}
	return stats, nil
}

func (store *OutboxStore) RepairOutbox(ctx context.Context, options types.OutboxRepairOptions) (types.OutboxRepairStats, error) {
	if store == nil || store.pool == nil {
		return types.OutboxRepairStats{}, errors.New("delivery outbox store is not configured")
	}
	mode := normalizeOutboxRepairMode(options.Mode)
	if mode == "" {
		return types.OutboxRepairStats{}, types.NewInvalidArgument("unsupported delivery outbox repair mode")
	}
	ids := normalizeOutboxIDs(options.OutboxIDs)
	if len(ids) == 0 {
		return types.OutboxRepairStats{}, types.NewInvalidArgument("outbox_ids are required")
	}
	operator := normalizeOutboxRepairText(options.Operator, "manual")
	reason := normalizeOutboxRepairText(options.Reason, "manual delivery outbox repair")

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return types.OutboxRepairStats{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	stats := types.OutboxRepairStats{Requested: len(ids)}
	now := store.now()
	for _, id := range ids {
		outcome, err := store.repairOutboxLocked(ctx, tx, id, mode, operator, reason, options.DryRun, now)
		if err != nil {
			return types.OutboxRepairStats{}, err
		}
		switch outcome {
		case outboxRepairOutcomeAudited:
			stats.Audited++
		case outboxRepairOutcomeMutated:
			stats.Mutated++
		case outboxRepairOutcomeSkipped:
			stats.Skipped++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return types.OutboxRepairStats{}, types.NewDBWriteFailed(err.Error())
	}
	return stats, nil
}

type outboxRepairRow struct {
	ID               int64
	EventID          string
	TenantID         string
	ConversationID   string
	AggregateVersion int64
	Status           string
	RetryCount       int
	LastError        string
	NextRetryAt      *time.Time
	DeadLetteredAt   *time.Time
}

const (
	outboxRepairOutcomeAudited = "AUDITED"
	outboxRepairOutcomeMutated = "MUTATED"
	outboxRepairOutcomeSkipped = "SKIPPED"

	outboxRepairSkipStatusNotDLQ = "status_is_not_dlq"
)

func (store *OutboxStore) repairOutboxLocked(ctx context.Context, tx pgx.Tx, id int64, mode string, operator string, reason string, dryRun bool, now time.Time) (string, error) {
	row, err := store.readRepairRowLocked(ctx, tx, id)
	if err != nil {
		return "", err
	}
	outcome := outboxRepairOutcomeAudited
	skipReason := ""
	after := row

	if mode == types.OutboxRepairModeRedriveDLQPending {
		if row.Status != types.OutboxStatusDLQ {
			outcome = outboxRepairOutcomeSkipped
			skipReason = outboxRepairSkipStatusNotDLQ
		} else if !dryRun {
			if _, err := tx.Exec(ctx, `
UPDATE delivery_outbox
SET status = $2,
    retry_count = 0,
    last_error = '',
    available_at = $3,
    next_retry_at = NULL,
    dead_lettered_at = NULL,
    updated_at = now()
WHERE id = $1
`, row.ID, types.OutboxStatusPending, now); err != nil {
				return "", types.NewDBWriteFailed(err.Error())
			}
			after.Status = types.OutboxStatusPending
			after.RetryCount = 0
			after.LastError = ""
			after.NextRetryAt = nil
			after.DeadLetteredAt = nil
			outcome = outboxRepairOutcomeMutated
		}
	}

	if err := store.insertOutboxRepairAudit(ctx, tx, row, after, mode, outcome, skipReason, operator, reason, dryRun, now); err != nil {
		return "", err
	}
	return outcome, nil
}

func (store *OutboxStore) readRepairRowLocked(ctx context.Context, tx pgx.Tx, id int64) (outboxRepairRow, error) {
	var row outboxRepairRow
	err := tx.QueryRow(ctx, `
SELECT
    id,
    event_id,
    tenant_id,
    conversation_id,
    aggregate_version,
    status,
    retry_count,
    last_error,
    next_retry_at,
    dead_lettered_at
FROM delivery_outbox
WHERE id = $1
FOR UPDATE
`, id).Scan(
		&row.ID,
		&row.EventID,
		&row.TenantID,
		&row.ConversationID,
		&row.AggregateVersion,
		&row.Status,
		&row.RetryCount,
		&row.LastError,
		&row.NextRetryAt,
		&row.DeadLetteredAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return outboxRepairRow{}, types.NewInvalidArgument("delivery outbox row not found")
		}
		return outboxRepairRow{}, types.NewDBReadFailed(err.Error())
	}
	row.LastError = sanitizeDeliveryOutboxStoredError(row.LastError)
	return row, nil
}

func (store *OutboxStore) insertOutboxRepairAudit(ctx context.Context, tx pgx.Tx, before outboxRepairRow, after outboxRepairRow, mode string, outcome string, skipReason string, operator string, reason string, dryRun bool, now time.Time) error {
	before.LastError = sanitizeDeliveryOutboxStoredError(before.LastError)
	after.LastError = sanitizeDeliveryOutboxStoredError(after.LastError)
	_, err := tx.Exec(ctx, `
INSERT INTO delivery_outbox_repair_audit (
    outbox_id,
    event_id,
    tenant_id,
    conversation_id,
    aggregate_version,
    mode,
    outcome,
    skip_reason,
    operator,
    reason,
    dry_run,
    before_status,
    before_retry_count,
    before_last_error,
    before_next_retry_at,
    before_dead_lettered_at,
    after_status,
    after_retry_count,
    after_last_error,
    after_next_retry_at,
    after_dead_lettered_at,
    created_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10, $11,
    $12, $13, $14, $15, $16,
    $17, $18, $19, $20, $21, $22
)
`,
		before.ID,
		before.EventID,
		before.TenantID,
		before.ConversationID,
		before.AggregateVersion,
		mode,
		outcome,
		skipReason,
		operator,
		reason,
		dryRun,
		before.Status,
		before.RetryCount,
		before.LastError,
		before.NextRetryAt,
		before.DeadLetteredAt,
		after.Status,
		after.RetryCount,
		after.LastError,
		after.NextRetryAt,
		after.DeadLetteredAt,
		now,
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func normalizeOutboxRepairMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "":
		return types.OutboxRepairModeAudit
	case types.OutboxRepairModeAudit:
		return types.OutboxRepairModeAudit
	case types.OutboxRepairModeRedriveDLQPending:
		return types.OutboxRepairModeRedriveDLQPending
	default:
		return ""
	}
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

func normalizeOutboxRepairOutcome(outcome string) string {
	switch strings.ToUpper(strings.TrimSpace(outcome)) {
	case "":
		return ""
	case outboxRepairOutcomeAudited:
		return outboxRepairOutcomeAudited
	case outboxRepairOutcomeMutated:
		return outboxRepairOutcomeMutated
	case outboxRepairOutcomeSkipped:
		return outboxRepairOutcomeSkipped
	default:
		return ""
	}
}

func normalizeOutboxIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func normalizeOutboxRepairText(value string, defaultText string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue
	}
	return value
}
