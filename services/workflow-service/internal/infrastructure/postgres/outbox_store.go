package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
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
		return types.OutboxRelayStats{}, errors.New("workflow outbox store is not configured")
	}
	if publish == nil {
		return types.OutboxRelayStats{}, errors.New("workflow outbox publish callback is not configured")
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
		return types.OutboxRelayStats{}, errors.New("workflow outbox batch publish result count mismatch")
	}

	now := store.now()
	published := make([]types.OutboxMessage, 0, len(messages))
	for index, message := range messages {
		if err := publishErrors[index]; err != nil {
			attempt := message.RetryCount + 1
			if attempt >= maxAttempts {
				if markErr := store.markDeadLettered(ctx, tx, message, attempt, now); markErr != nil {
					return types.OutboxRelayStats{}, markErr
				}
				stats.DeadLettered++
				continue
			}
			nextRetryAt := now.Add(retryDelay(retryBaseDelay, attempt))
			if markErr := store.markRetry(ctx, tx, message, attempt, nextRetryAt); markErr != nil {
				return types.OutboxRelayStats{}, markErr
			}
			stats.Retried++
			continue
		}
		published = append(published, message)
	}

	if len(published) > 0 {
		if err := store.markPublishedBatch(ctx, tx, published, now); err != nil {
			return types.OutboxRelayStats{}, err
		}
		stats.Published = len(published)
	}

	if err := tx.Commit(ctx); err != nil {
		return types.OutboxRelayStats{}, types.NewDBWriteFailed(err.Error())
	}
	return stats, nil
}

func (store *OutboxStore) fetchReadyLocked(ctx context.Context, tx pgx.Tx, limit int) ([]types.OutboxMessage, error) {
	rows, err := tx.Query(ctx, `
SELECT
    event_id,
    tenant_id,
    workflow_id,
    aggregate_type,
    aggregate_id,
    event_type,
    event_version,
    partition_key,
    payload_json,
    retry_count,
    created_at
FROM workflow_outbox current
WHERE status = 'PENDING'
  AND published_at IS NULL
  AND COALESCE(next_retry_at, available_at) <= now()
  AND NOT EXISTS (
      SELECT 1
      FROM workflow_outbox previous
      WHERE previous.tenant_id = current.tenant_id
        AND previous.workflow_id = current.workflow_id
        AND previous.status IN ('PENDING', 'DLQ')
        AND (
            previous.created_at < current.created_at
            OR (previous.created_at = current.created_at AND previous.event_id < current.event_id)
        )
  )
ORDER BY created_at, event_id
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
			&message.EventID,
			&message.TenantID,
			&message.WorkflowID,
			&message.AggregateType,
			&message.AggregateID,
			&message.EventType,
			&message.EventVersion,
			&message.PartitionKey,
			&message.PayloadJSON,
			&message.RetryCount,
			&message.OccurredAt,
		); err != nil {
			return nil, types.NewDBWriteFailed(err.Error())
		}
		message.Producer = "workflow-service"
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	return messages, nil
}

func (store *OutboxStore) markPublishedBatch(ctx context.Context, tx pgx.Tx, messages []types.OutboxMessage, publishedAt time.Time) error {
	tenantIDs, eventIDs := outboxKeys(messages)
	tag, err := tx.Exec(ctx, `
UPDATE workflow_outbox
SET status = $3,
    published_at = $4,
    next_retry_at = NULL,
    dead_lettered_at = NULL,
    updated_at = now()
FROM unnest($1::text[], $2::text[]) AS keys(tenant_id, event_id)
WHERE workflow_outbox.tenant_id = keys.tenant_id
  AND workflow_outbox.event_id = keys.event_id
`, tenantIDs, eventIDs, types.OutboxStatusPublished, publishedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if tag.RowsAffected() != int64(len(messages)) {
		return types.NewDBWriteFailed("workflow outbox published row count mismatch")
	}
	return nil
}

func (store *OutboxStore) markRetry(ctx context.Context, tx pgx.Tx, message types.OutboxMessage, retryCount int, nextRetryAt time.Time) error {
	_, err := tx.Exec(ctx, `
UPDATE workflow_outbox
SET retry_count = $3,
    next_retry_at = $4,
    updated_at = now()
WHERE tenant_id = $1
  AND event_id = $2
`, message.TenantID, message.EventID, retryCount, nextRetryAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func (store *OutboxStore) markDeadLettered(ctx context.Context, tx pgx.Tx, message types.OutboxMessage, retryCount int, deadLetteredAt time.Time) error {
	_, err := tx.Exec(ctx, `
UPDATE workflow_outbox
SET status = $3,
    retry_count = $4,
    next_retry_at = NULL,
    dead_lettered_at = $5,
    updated_at = now()
WHERE tenant_id = $1
  AND event_id = $2
`, message.TenantID, message.EventID, types.OutboxStatusDLQ, retryCount, deadLetteredAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func outboxKeys(messages []types.OutboxMessage) ([]string, []string) {
	tenantIDs := make([]string, 0, len(messages))
	eventIDs := make([]string, 0, len(messages))
	for _, message := range messages {
		tenantIDs = append(tenantIDs, string(message.TenantID))
		eventIDs = append(eventIDs, message.EventID)
	}
	return tenantIDs, eventIDs
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

func sanitizeWorkflowOutboxErrorText(value string) string {
	message := strings.ToLower(strings.TrimSpace(value))
	switch {
	case message == "":
		return "workflow outbox publish failed"
	case strings.Contains(message, "cancel"):
		return "workflow outbox publish canceled"
	case strings.Contains(message, "timeout") || strings.Contains(message, "deadline exceeded"):
		return "workflow outbox publish timeout"
	case strings.Contains(message, "unsupported"):
		return "workflow outbox publish unsupported event"
	case strings.Contains(message, "malformed") ||
		strings.Contains(message, "invalid") ||
		strings.Contains(message, "json") ||
		strings.Contains(message, "decode") ||
		strings.Contains(message, "payload"):
		return "workflow outbox publish invalid payload"
	case strings.Contains(message, "kafka") ||
		strings.Contains(message, "broker") ||
		strings.Contains(message, "leader") ||
		strings.Contains(message, "connection refused") ||
		strings.Contains(message, "no such host") ||
		strings.Contains(message, "network"):
		return "workflow outbox publish broker unavailable"
	default:
		return "workflow outbox publish failed"
	}
}
