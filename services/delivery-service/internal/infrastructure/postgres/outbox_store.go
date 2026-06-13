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
	return row, nil
}

func (store *OutboxStore) insertOutboxRepairAudit(ctx context.Context, tx pgx.Tx, before outboxRepairRow, after outboxRepairRow, mode string, outcome string, skipReason string, operator string, reason string, dryRun bool, now time.Time) error {
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

func normalizeOutboxRepairText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
