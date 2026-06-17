package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type ChallengeDeliveryStore struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

type ChallengeDeliveryStoreOption func(*ChallengeDeliveryStore)

func NewChallengeDeliveryStore(pool *pgxpool.Pool, opts ...ChallengeDeliveryStoreOption) *ChallengeDeliveryStore {
	store := &ChallengeDeliveryStore{
		pool: pool,
		now:  func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(store)
	}
	return store
}

func WithChallengeDeliveryClock(clock func() time.Time) ChallengeDeliveryStoreOption {
	return func(store *ChallengeDeliveryStore) {
		if clock != nil {
			store.now = clock
		}
	}
}

func (store *ChallengeDeliveryStore) ProcessReadyBatch(
	ctx context.Context,
	limit int,
	maxAttempts int,
	retryBaseDelay time.Duration,
	deliver func(context.Context, []types.ChallengeDeliveryMessage) []error,
) (types.ChallengeDeliveryStats, error) {
	if store == nil || store.pool == nil {
		return types.ChallengeDeliveryStats{}, errors.New("identity challenge delivery store is not configured")
	}
	if deliver == nil {
		return types.ChallengeDeliveryStats{}, errors.New("identity challenge delivery callback is not configured")
	}
	if limit <= 0 {
		limit = 100
	}
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if retryBaseDelay <= 0 {
		retryBaseDelay = time.Second
	}

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return types.ChallengeDeliveryStats{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := store.now()
	canceled, err := store.cancelInactiveLocked(ctx, tx, now)
	if err != nil {
		return types.ChallengeDeliveryStats{}, err
	}
	messages, err := store.fetchReadyLocked(ctx, tx, limit, now)
	if err != nil {
		return types.ChallengeDeliveryStats{}, err
	}
	stats := types.ChallengeDeliveryStats{Fetched: len(messages), Canceled: canceled}
	if len(messages) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return types.ChallengeDeliveryStats{}, types.NewDBWriteFailed(err.Error())
		}
		return stats, nil
	}

	deliveryErrors := deliver(ctx, messages)
	if len(deliveryErrors) != len(messages) {
		return types.ChallengeDeliveryStats{}, errors.New("identity challenge delivery result count mismatch")
	}

	for index, message := range messages {
		if err := deliveryErrors[index]; err != nil {
			attempt := message.RetryCount + 1
			if attempt >= maxAttempts {
				if markErr := store.markDeadLettered(ctx, tx, message, attempt, err, now); markErr != nil {
					return types.ChallengeDeliveryStats{}, markErr
				}
				stats.DeadLettered++
				continue
			}
			nextRetryAt := now.Add(retryDelay(retryBaseDelay, attempt))
			if markErr := store.markRetry(ctx, tx, message, attempt, err, nextRetryAt, now); markErr != nil {
				return types.ChallengeDeliveryStats{}, markErr
			}
			stats.Retried++
			continue
		}
		if err := store.markDelivered(ctx, tx, message, now); err != nil {
			return types.ChallengeDeliveryStats{}, err
		}
		stats.Delivered++
	}

	if err := tx.Commit(ctx); err != nil {
		return types.ChallengeDeliveryStats{}, types.NewDBWriteFailed(err.Error())
	}
	return stats, nil
}

func (store *ChallengeDeliveryStore) cancelInactiveLocked(ctx context.Context, tx pgx.Tx, now time.Time) (int, error) {
	rows, err := tx.Query(ctx, `
SELECT
    current.id,
    current.tenant_id,
    current.user_id,
    current.challenge_id
FROM identity_challenge_delivery_outbox current
JOIN identity_challenges challenge
  ON challenge.tenant_id = current.tenant_id
 AND challenge.user_id = current.user_id
 AND challenge.challenge_id = current.challenge_id
WHERE current.status = 'PENDING'
  AND (
      challenge.status <> 'ACTIVE'
      OR challenge.expires_at <= $1
      OR current.expires_at <= $1
  )
ORDER BY current.id
LIMIT 500
FOR UPDATE OF current, challenge SKIP LOCKED
`, now)
	if err != nil {
		return 0, types.NewDBWriteFailed(err.Error())
	}
	defer rows.Close()

	type expiredRow struct {
		id          int64
		tenantID    types.TenantID
		userID      types.UserID
		challengeID types.ChallengeID
	}
	expired := make([]expiredRow, 0)
	for rows.Next() {
		var row expiredRow
		if err := rows.Scan(&row.id, &row.tenantID, &row.userID, &row.challengeID); err != nil {
			return 0, types.NewDBWriteFailed(err.Error())
		}
		expired = append(expired, row)
	}
	if err := rows.Err(); err != nil {
		return 0, types.NewDBWriteFailed(err.Error())
	}
	for _, row := range expired {
		if _, err := tx.Exec(ctx, `
UPDATE identity_challenge_delivery_outbox
SET status = 'CANCELED',
    last_error = 'challenge no longer active before delivery',
    failure_class = 'inactive',
    next_retry_at = NULL,
    updated_at = $2
WHERE id = $1
`, row.id, now); err != nil {
			return 0, types.NewDBWriteFailed(err.Error())
		}
		if _, err := tx.Exec(ctx, `
UPDATE identity_challenges
SET status = CASE WHEN status = 'ACTIVE' THEN 'EXPIRED' ELSE status END,
    delivery_status = 'FAILED',
    delivery_failed_at = $4,
    delivery_last_error = 'challenge no longer active before delivery',
    delivery_failure_class = 'inactive',
    updated_at = $4
WHERE tenant_id = $1
  AND user_id = $2
  AND challenge_id = $3
`, row.tenantID, row.userID, row.challengeID, now); err != nil {
			return 0, types.NewDBWriteFailed(err.Error())
		}
	}
	return len(expired), nil
}

func (store *ChallengeDeliveryStore) fetchReadyLocked(ctx context.Context, tx pgx.Tx, limit int, now time.Time) ([]types.ChallengeDeliveryMessage, error) {
	rows, err := tx.Query(ctx, `
SELECT
    current.id,
    current.tenant_id,
    current.user_id,
    current.challenge_id,
    current.challenge_type,
    current.channel,
    current.destination,
    current.token_ciphertext,
    current.token_nonce,
    current.token_key_version,
    current.expires_at,
    current.trace_id,
    current.request_id,
    current.retry_count,
    current.created_at
FROM identity_challenge_delivery_outbox current
JOIN identity_challenges challenge
  ON challenge.tenant_id = current.tenant_id
 AND challenge.user_id = current.user_id
 AND challenge.challenge_id = current.challenge_id
WHERE current.status = 'PENDING'
  AND challenge.status = 'ACTIVE'
  AND challenge.expires_at > $2
  AND current.expires_at > $2
  AND COALESCE(current.next_retry_at, current.available_at) <= $2
ORDER BY current.id
LIMIT $1
FOR UPDATE OF current, challenge SKIP LOCKED
`, limit, now)
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer rows.Close()

	messages := make([]types.ChallengeDeliveryMessage, 0)
	for rows.Next() {
		var message types.ChallengeDeliveryMessage
		if err := rows.Scan(
			&message.ID,
			&message.TenantID,
			&message.UserID,
			&message.ChallengeID,
			&message.Type,
			&message.Channel,
			&message.Destination,
			&message.EncryptedToken.Ciphertext,
			&message.EncryptedToken.Nonce,
			&message.EncryptedToken.KeyVersion,
			&message.ExpiresAt,
			&message.TraceID,
			&message.RequestID,
			&message.RetryCount,
			&message.CreatedAt,
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

func (store *ChallengeDeliveryStore) markDelivered(ctx context.Context, tx pgx.Tx, message types.ChallengeDeliveryMessage, deliveredAt time.Time) error {
	tag, err := tx.Exec(ctx, `
UPDATE identity_challenge_delivery_outbox
SET status = 'DELIVERED',
    delivered_at = $2,
    last_error = '',
    failure_class = '',
    next_retry_at = NULL,
    updated_at = $2
WHERE id = $1
`, message.ID, deliveredAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if tag.RowsAffected() != 1 {
		return types.NewDBWriteFailed("identity challenge delivery delivered row count mismatch")
	}
	_, err = tx.Exec(ctx, `
UPDATE identity_challenges
SET delivery_status = 'DELIVERED',
    delivery_attempt_count = delivery_attempt_count + 1,
    delivered_at = $4,
    delivery_failed_at = NULL,
    delivery_last_error = '',
    delivery_failure_class = '',
    updated_at = $4
WHERE tenant_id = $1
  AND user_id = $2
  AND challenge_id = $3
`, message.TenantID, message.UserID, message.ChallengeID, deliveredAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func (store *ChallengeDeliveryStore) markRetry(ctx context.Context, tx pgx.Tx, message types.ChallengeDeliveryMessage, retryCount int, deliveryErr error, nextRetryAt time.Time, failedAt time.Time) error {
	lastError := sanitizeChallengeDeliveryError(deliveryErr.Error())
	failureClass := types.ClassifyChallengeDeliveryFailure(deliveryErr)
	_, err := tx.Exec(ctx, `
UPDATE identity_challenge_delivery_outbox
SET retry_count = $2,
    last_error = $3,
    next_retry_at = $4,
    updated_at = $5,
    failure_class = $6
WHERE id = $1
`, message.ID, retryCount, lastError, nextRetryAt, failedAt, failureClass)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	_, err = tx.Exec(ctx, `
UPDATE identity_challenges
SET delivery_attempt_count = delivery_attempt_count + 1,
    delivery_failed_at = $4,
    delivery_last_error = $5,
    delivery_failure_class = $6,
    updated_at = $4
WHERE tenant_id = $1
  AND user_id = $2
  AND challenge_id = $3
`, message.TenantID, message.UserID, message.ChallengeID, failedAt, lastError, failureClass)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func (store *ChallengeDeliveryStore) markDeadLettered(ctx context.Context, tx pgx.Tx, message types.ChallengeDeliveryMessage, retryCount int, deliveryErr error, deadLetteredAt time.Time) error {
	lastError := sanitizeChallengeDeliveryError(deliveryErr.Error())
	failureClass := types.ClassifyChallengeDeliveryFailure(deliveryErr)
	_, err := tx.Exec(ctx, `
UPDATE identity_challenge_delivery_outbox
SET status = 'DLQ',
    retry_count = $2,
    last_error = $3,
    failure_class = $5,
    next_retry_at = NULL,
    dead_lettered_at = $4,
    updated_at = $4
WHERE id = $1
`, message.ID, retryCount, lastError, deadLetteredAt, failureClass)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	_, err = tx.Exec(ctx, `
UPDATE identity_challenges
SET status = CASE WHEN status = 'ACTIVE' THEN 'EXPIRED' ELSE status END,
    delivery_status = 'FAILED',
    delivery_attempt_count = delivery_attempt_count + 1,
    delivered_at = NULL,
    delivery_failed_at = $4,
    delivery_last_error = $5,
    delivery_failure_class = $6,
    updated_at = $4
WHERE tenant_id = $1
  AND user_id = $2
  AND challenge_id = $3
`, message.TenantID, message.UserID, message.ChallengeID, deadLetteredAt, lastError, failureClass)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}
