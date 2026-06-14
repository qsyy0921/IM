package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
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
		return types.OutboxRelayStats{}, errors.New("identity outbox store is not configured")
	}
	if publish == nil {
		return types.OutboxRelayStats{}, errors.New("identity outbox publish callback is not configured")
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
	defer func() { _ = tx.Rollback(ctx) }()

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
		return types.OutboxRelayStats{}, errors.New("identity outbox batch publish result count mismatch")
	}

	now := store.now()
	publishedIDs := make([]int64, 0, len(messages))
	for index, message := range messages {
		if err := publishErrors[index]; err != nil {
			lastError := sanitizeIdentityOutboxPublishError(err)
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
    aggregate_type,
    aggregate_id,
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
FROM identity_outbox current
WHERE status = 'PENDING'
  AND published_at IS NULL
  AND COALESCE(next_retry_at, available_at) <= now()
  AND NOT EXISTS (
      SELECT 1
      FROM identity_outbox previous
      WHERE previous.tenant_id = current.tenant_id
        AND previous.partition_key = current.partition_key
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
			&message.AggregateType,
			&message.AggregateID,
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
UPDATE identity_outbox
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
		return types.NewDBWriteFailed("identity outbox published row count mismatch")
	}
	return nil
}

func (store *OutboxStore) markRetry(ctx context.Context, tx pgx.Tx, id int64, retryCount int, lastError string, nextRetryAt time.Time) error {
	_, err := tx.Exec(ctx, `
UPDATE identity_outbox
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
UPDATE identity_outbox
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

func sanitizeIdentityOutboxPublishError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "identity outbox publish canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "identity outbox publish timeout"
	}
	return sanitizeIdentityOutboxPublishErrorText(err.Error())
}

func sanitizeIdentityOutboxStoredError(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return sanitizeIdentityOutboxPublishErrorText(value)
}

func sanitizeIdentityOutboxPublishErrorText(value string) string {
	message := strings.ToLower(strings.TrimSpace(value))
	switch {
	case message == "":
		return "identity outbox publish failed"
	case strings.Contains(message, "cancel"):
		return "identity outbox publish canceled"
	case strings.Contains(message, "timeout") || strings.Contains(message, "deadline exceeded"):
		return "identity outbox publish timeout"
	case strings.Contains(message, "unsupported"):
		return "identity outbox publish unsupported event"
	case strings.Contains(message, "malformed") ||
		strings.Contains(message, "invalid") ||
		strings.Contains(message, "json") ||
		strings.Contains(message, "decode") ||
		strings.Contains(message, "payload"):
		return "identity outbox publish invalid payload"
	case strings.Contains(message, "kafka") ||
		strings.Contains(message, "broker") ||
		strings.Contains(message, "leader") ||
		strings.Contains(message, "connection refused") ||
		strings.Contains(message, "no such host") ||
		strings.Contains(message, "network"):
		return "identity outbox publish broker unavailable"
	default:
		return "identity outbox publish failed"
	}
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
