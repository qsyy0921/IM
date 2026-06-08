package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

type OutboxStore struct {
	pool    *pgxpool.Pool
	now     func() time.Time
	metrics types.LatencyRecorder
}

type OutboxStoreOption func(*OutboxStore)

func NewOutboxStore(pool *pgxpool.Pool, opts ...OutboxStoreOption) *OutboxStore {
	store := &OutboxStore{
		pool:    pool,
		now:     func() time.Time { return time.Now().UTC() },
		metrics: types.NoopLatencyRecorder{},
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

func WithOutboxMetrics(metrics types.LatencyRecorder) OutboxStoreOption {
	return func(store *OutboxStore) {
		if metrics != nil {
			store.metrics = metrics
		}
	}
}

func (s *OutboxStore) ProcessReady(
	ctx context.Context,
	limit int,
	maxAttempts int,
	retryBaseDelay time.Duration,
	publish func(context.Context, types.OutboxMessage) error,
) (types.OutboxRelayStats, error) {
	if s.pool == nil {
		return types.OutboxRelayStats{}, ErrRepositoryNotConfigured
	}
	if publish == nil {
		return types.OutboxRelayStats{}, errors.New("outbox publish callback is not configured")
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

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return types.OutboxRelayStats{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	fetchStarted := time.Now()
	messages, err := s.fetchReadyLocked(ctx, tx, limit)
	s.metrics.ObserveOutboxFetchReady(time.Since(fetchStarted))
	if err != nil {
		return types.OutboxRelayStats{}, err
	}
	stats := types.OutboxRelayStats{Fetched: len(messages)}
	now := s.now()
	publishedIDs := make([]int64, 0, len(messages))

	for _, message := range messages {
		if err := publish(ctx, message); err != nil {
			attempt := message.RetryCount + 1
			if attempt >= maxAttempts {
				if markErr := s.markDeadLettered(ctx, tx, message.ID, attempt, err.Error(), now); markErr != nil {
					return types.OutboxRelayStats{}, markErr
				}
				stats.DeadLettered++
				continue
			}
			nextRetryAt := now.Add(retryDelay(retryBaseDelay, attempt))
			if markErr := s.markRetry(ctx, tx, message.ID, attempt, err.Error(), nextRetryAt); markErr != nil {
				return types.OutboxRelayStats{}, markErr
			}
			stats.Retried++
			continue
		}

		publishedIDs = append(publishedIDs, message.ID)
	}

	if len(publishedIDs) > 0 {
		if err := s.markPublishedBatch(ctx, tx, publishedIDs, now); err != nil {
			return types.OutboxRelayStats{}, err
		}
		stats.Published = len(publishedIDs)
	}

	commitStarted := time.Now()
	if err := tx.Commit(ctx); err != nil {
		s.metrics.ObserveOutboxCommit(time.Since(commitStarted))
		return types.OutboxRelayStats{}, types.NewDBWriteFailed(err.Error())
	}
	s.metrics.ObserveOutboxCommit(time.Since(commitStarted))
	return stats, nil
}

func (s *OutboxStore) fetchReadyLocked(ctx context.Context, tx pgx.Tx, limit int) ([]types.OutboxMessage, error) {
	rows, err := tx.Query(ctx, `
SELECT
    mo.id,
    mo.event_id,
    mo.tenant_id,
    mo.conversation_id,
    mo.aggregate_version,
    mo.event_type,
    mo.event_version,
    mo.partition_key,
    mo.mapping_version,
    mo.correlation_id,
    mo.causation_id,
    mo.producer,
    mo.payload_json,
    mo.trace_id,
    mo.retry_count,
    te.fanout_mode,
    te.fanout_policy_version,
    te.permission_version,
    COALESCE(te.classification, ''),
    te.created_at
FROM message_outbox mo
JOIN conversation_timeline_events te
  ON te.tenant_id = mo.tenant_id
 AND te.conversation_id = mo.conversation_id
 AND te.seq = mo.aggregate_version
 AND te.event_id = mo.event_id
WHERE mo.status = 'PENDING'
  AND mo.published_at IS NULL
  AND COALESCE(mo.next_retry_at, mo.available_at) <= now()
  AND NOT EXISTS (
      SELECT 1
      FROM message_outbox prev
      WHERE prev.tenant_id = mo.tenant_id
        AND prev.conversation_id = mo.conversation_id
        AND prev.aggregate_version < mo.aggregate_version
        AND prev.status IN ('PENDING', 'DLQ')
  )
ORDER BY mo.id
LIMIT $1
FOR UPDATE OF mo SKIP LOCKED
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
			&message.FanoutMode,
			&message.FanoutPolicyVersion,
			&message.PermissionVersion,
			&message.Classification,
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

func (s *OutboxStore) markPublishedBatch(ctx context.Context, tx pgx.Tx, ids []int64, publishedAt time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	started := time.Now()
	defer func() {
		s.metrics.ObserveOutboxMarkPublished(time.Since(started))
	}()
	commandTag, err := tx.Exec(ctx, `
UPDATE message_outbox
SET status = $2,
    published_at = $3,
    last_error = NULL,
    next_retry_at = NULL,
    dead_lettered_at = NULL
WHERE id = ANY($1::bigint[])
`, ids, types.OutboxStatusPublished, publishedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if commandTag.RowsAffected() != int64(len(ids)) {
		return types.NewDBWriteFailed("outbox published row count mismatch")
	}
	return nil
}

func (s *OutboxStore) markRetry(ctx context.Context, tx pgx.Tx, id int64, retryCount int, lastError string, nextRetryAt time.Time) error {
	_, err := tx.Exec(ctx, `
UPDATE message_outbox
SET retry_count = $2,
    last_error = $3,
    next_retry_at = $4
WHERE id = $1
`, id, retryCount, lastError, nextRetryAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func (s *OutboxStore) markDeadLettered(ctx context.Context, tx pgx.Tx, id int64, retryCount int, lastError string, deadLetteredAt time.Time) error {
	_, err := tx.Exec(ctx, `
UPDATE message_outbox
SET status = $2,
    retry_count = $3,
    last_error = $4,
    next_retry_at = NULL,
    dead_lettered_at = $5
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
