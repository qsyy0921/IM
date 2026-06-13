package postgres

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type ChallengeDeliveryStore struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

type ChallengeDeliveryRepairAuditOptions struct {
	DeliveryID           *int64
	TenantID             string
	UserID               string
	ChallengeID          string
	Mode                 string
	Outcome              string
	PreviousFailureClass string
	NewFailureClass      string
	Limit                int
}

type ChallengeDeliveryRepairAuditRow struct {
	DeliveryID                      int64
	TenantID                        string
	UserID                          string
	ChallengeID                     string
	Mode                            string
	Outcome                         string
	SkipReason                      string
	Operator                        string
	Reason                          string
	DryRun                          bool
	PreviousDeliveryStatus          string
	PreviousChallengeStatus         string
	PreviousChallengeDeliveryStatus string
	PreviousRetryCount              int
	PreviousLastError               string
	PreviousFailureClass            string
	PreviousDeadLetteredAt          *time.Time
	NewDeliveryStatus               string
	NewChallengeStatus              string
	NewChallengeDeliveryStatus      string
	NewFailureClass                 string
	RepairedAt                      time.Time
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

func (store *ChallengeDeliveryStore) RepairDeliveries(ctx context.Context, options types.ChallengeDeliveryRepairOptions) (types.ChallengeDeliveryRepairStats, error) {
	if store == nil || store.pool == nil {
		return types.ChallengeDeliveryRepairStats{}, errors.New("identity challenge delivery store is not configured")
	}
	mode := normalizeChallengeDeliveryRepairMode(options.Mode)
	if mode == "" {
		return types.ChallengeDeliveryRepairStats{}, types.NewInvalidArgument("unsupported identity challenge delivery repair mode")
	}
	ids := normalizeChallengeDeliveryIDs(options.DeliveryIDs)
	if len(ids) == 0 {
		return types.ChallengeDeliveryRepairStats{}, types.NewInvalidArgument("delivery_ids are required")
	}
	operator := normalizeChallengeDeliveryRepairText(options.Operator, "manual")
	reason := normalizeChallengeDeliveryRepairText(options.Reason, "manual identity challenge delivery repair")

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return types.ChallengeDeliveryRepairStats{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	stats := types.ChallengeDeliveryRepairStats{Requested: len(ids)}
	now := store.now()
	for _, id := range ids {
		result, err := store.repairDeliveryLocked(ctx, tx, id, mode, operator, reason, options.DryRun, now)
		if err != nil {
			return types.ChallengeDeliveryRepairStats{}, err
		}
		switch result {
		case challengeDeliveryRepairOutcomeAudited:
			stats.Audited++
		case challengeDeliveryRepairOutcomeMutated:
			stats.Mutated++
		case challengeDeliveryRepairOutcomeSkipped:
			stats.Skipped++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return types.ChallengeDeliveryRepairStats{}, types.NewDBWriteFailed(err.Error())
	}
	return stats, nil
}

func (store *ChallengeDeliveryStore) AuditDeliveryRepairs(ctx context.Context, options ChallengeDeliveryRepairAuditOptions) ([]ChallengeDeliveryRepairAuditRow, error) {
	if store == nil || store.pool == nil {
		return nil, errors.New("identity challenge delivery store is not configured")
	}
	limit := options.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	var args []any
	clauses := make([]string, 0, 8)
	if options.DeliveryID != nil {
		args = append(args, *options.DeliveryID)
		clauses = append(clauses, "delivery_id = $"+strconv.Itoa(len(args)))
	}
	if tenantID := strings.TrimSpace(options.TenantID); tenantID != "" {
		args = append(args, tenantID)
		clauses = append(clauses, "tenant_id = $"+strconv.Itoa(len(args)))
	}
	if userID := strings.TrimSpace(options.UserID); userID != "" {
		args = append(args, userID)
		clauses = append(clauses, "user_id = $"+strconv.Itoa(len(args)))
	}
	if challengeID := strings.TrimSpace(options.ChallengeID); challengeID != "" {
		args = append(args, challengeID)
		clauses = append(clauses, "challenge_id = $"+strconv.Itoa(len(args)))
	}
	if rawMode := strings.TrimSpace(options.Mode); rawMode != "" {
		mode := normalizeChallengeDeliveryRepairMode(rawMode)
		if mode == "" {
			return nil, types.NewInvalidArgument("unsupported identity challenge delivery repair mode")
		}
		args = append(args, mode)
		clauses = append(clauses, "repair_mode = $"+strconv.Itoa(len(args)))
	}
	if rawOutcome := strings.TrimSpace(options.Outcome); rawOutcome != "" {
		outcome := normalizeChallengeDeliveryRepairOutcome(rawOutcome)
		if outcome == "" {
			return nil, types.NewInvalidArgument("unsupported identity challenge delivery repair outcome")
		}
		args = append(args, outcome)
		clauses = append(clauses, "repair_outcome = $"+strconv.Itoa(len(args)))
	}
	if rawFailureClass := strings.TrimSpace(options.PreviousFailureClass); rawFailureClass != "" {
		failureClass := normalizeChallengeDeliveryFailureClass(rawFailureClass)
		if failureClass == "" {
			return nil, types.NewInvalidArgument("unsupported identity challenge delivery previous failure class")
		}
		args = append(args, failureClass)
		clauses = append(clauses, "previous_failure_class = $"+strconv.Itoa(len(args)))
	}
	if rawFailureClass := strings.TrimSpace(options.NewFailureClass); rawFailureClass != "" {
		failureClass := normalizeChallengeDeliveryFailureClass(rawFailureClass)
		if failureClass == "" {
			return nil, types.NewInvalidArgument("unsupported identity challenge delivery new failure class")
		}
		args = append(args, failureClass)
		clauses = append(clauses, "new_failure_class = $"+strconv.Itoa(len(args)))
	}

	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	args = append(args, limit)
	rows, err := store.pool.Query(ctx, `
SELECT
    delivery_id,
    tenant_id,
    user_id,
    challenge_id,
    previous_delivery_status,
    previous_challenge_status,
    previous_challenge_delivery_status,
    previous_retry_count,
    previous_last_error,
    previous_failure_class,
    previous_dead_lettered_at,
    new_delivery_status,
    new_challenge_status,
    new_challenge_delivery_status,
    new_failure_class,
    repair_mode,
    repair_outcome,
    skip_reason,
    dry_run,
    repair_operator,
    repair_reason,
    repaired_at
FROM identity_challenge_delivery_repair_audit
`+where+`
ORDER BY repaired_at DESC, delivery_id, id DESC
LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	result := make([]ChallengeDeliveryRepairAuditRow, 0, limit)
	for rows.Next() {
		var row ChallengeDeliveryRepairAuditRow
		if err := rows.Scan(
			&row.DeliveryID,
			&row.TenantID,
			&row.UserID,
			&row.ChallengeID,
			&row.PreviousDeliveryStatus,
			&row.PreviousChallengeStatus,
			&row.PreviousChallengeDeliveryStatus,
			&row.PreviousRetryCount,
			&row.PreviousLastError,
			&row.PreviousFailureClass,
			&row.PreviousDeadLetteredAt,
			&row.NewDeliveryStatus,
			&row.NewChallengeStatus,
			&row.NewChallengeDeliveryStatus,
			&row.NewFailureClass,
			&row.Mode,
			&row.Outcome,
			&row.SkipReason,
			&row.DryRun,
			&row.Operator,
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

type challengeDeliveryRepairRow struct {
	id                      int64
	tenantID                types.TenantID
	userID                  types.UserID
	challengeID             types.ChallengeID
	challengeType           types.ChallengeType
	channel                 types.VerificationChannel
	destination             string
	deliveryStatus          string
	retryCount              int
	lastError               string
	failureClass            string
	deadLetteredAt          *time.Time
	deliveryExpiresAt       time.Time
	challengeStatus         string
	challengeDeliveryStatus string
	challengeExpiresAt      time.Time
}

const (
	challengeDeliveryRepairOutcomeAudited = "AUDITED"
	challengeDeliveryRepairOutcomeMutated = "MUTATED"
	challengeDeliveryRepairOutcomeSkipped = "SKIPPED"
)

func (store *ChallengeDeliveryStore) repairDeliveryLocked(
	ctx context.Context,
	tx pgx.Tx,
	id int64,
	mode string,
	operator string,
	reason string,
	dryRun bool,
	now time.Time,
) (string, error) {
	row, err := store.lockChallengeDeliveryForRepair(ctx, tx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return challengeDeliveryRepairOutcomeSkipped, nil
	}
	if err != nil {
		return "", err
	}

	planned := store.planChallengeDeliveryRepair(row, mode, now)
	if planned.outcome == challengeDeliveryRepairOutcomeMutated && !dryRun {
		if err := store.applyChallengeDeliveryRepair(ctx, tx, row, planned, now); err != nil {
			return "", err
		}
	}
	auditOutcome := planned.outcome
	if dryRun && planned.outcome == challengeDeliveryRepairOutcomeMutated {
		auditOutcome = challengeDeliveryRepairOutcomeAudited
	}
	if err := store.insertChallengeDeliveryRepairAudit(ctx, tx, row, planned, mode, auditOutcome, operator, reason, dryRun, now); err != nil {
		return "", err
	}
	return auditOutcome, nil
}

type plannedChallengeDeliveryRepair struct {
	outcome                         string
	skipReason                      string
	newDeliveryStatus               string
	newChallengeStatus              string
	newChallengeDeliveryStatus      string
	newFailureClass                 string
	redriveActivePending            bool
	cancelInactiveChallengeDelivery bool
}

func (store *ChallengeDeliveryStore) planChallengeDeliveryRepair(row challengeDeliveryRepairRow, mode string, now time.Time) plannedChallengeDeliveryRepair {
	planned := plannedChallengeDeliveryRepair{
		outcome:                    challengeDeliveryRepairOutcomeAudited,
		newDeliveryStatus:          row.deliveryStatus,
		newChallengeStatus:         row.challengeStatus,
		newChallengeDeliveryStatus: row.challengeDeliveryStatus,
		newFailureClass:            row.failureClass,
	}
	if mode == types.ChallengeDeliveryRepairModeAudit {
		return planned
	}
	if row.deliveryStatus != types.ChallengeDeliveryStatusPending {
		planned.outcome = challengeDeliveryRepairOutcomeSkipped
		if row.deliveryStatus == types.ChallengeDeliveryStatusDLQ {
			planned.skipReason = "dlq_requires_new_challenge"
		} else {
			planned.skipReason = "delivery_status_not_pending"
		}
		return planned
	}

	switch mode {
	case types.ChallengeDeliveryRepairModeRedriveActivePending:
		if row.challengeStatus != "ACTIVE" {
			planned.outcome = challengeDeliveryRepairOutcomeSkipped
			planned.skipReason = "challenge_not_active"
			return planned
		}
		if !row.deliveryExpiresAt.After(now) || !row.challengeExpiresAt.After(now) {
			planned.outcome = challengeDeliveryRepairOutcomeSkipped
			planned.skipReason = "challenge_or_delivery_expired"
			return planned
		}
		planned.outcome = challengeDeliveryRepairOutcomeMutated
		planned.newDeliveryStatus = types.ChallengeDeliveryStatusPending
		planned.newChallengeStatus = "ACTIVE"
		planned.newChallengeDeliveryStatus = "PENDING"
		planned.newFailureClass = ""
		planned.redriveActivePending = true
		return planned
	case types.ChallengeDeliveryRepairModeCancelInactive:
		if row.challengeStatus == "ACTIVE" && row.challengeExpiresAt.After(now) && row.deliveryExpiresAt.After(now) {
			planned.outcome = challengeDeliveryRepairOutcomeSkipped
			planned.skipReason = "challenge_still_active"
			return planned
		}
		planned.outcome = challengeDeliveryRepairOutcomeMutated
		planned.newDeliveryStatus = types.ChallengeDeliveryStatusCanceled
		planned.newChallengeStatus = row.challengeStatus
		if planned.newChallengeStatus == "ACTIVE" {
			planned.newChallengeStatus = "EXPIRED"
		}
		planned.newChallengeDeliveryStatus = "FAILED"
		planned.newFailureClass = types.ChallengeDeliveryFailureClassInactive
		planned.cancelInactiveChallengeDelivery = true
		return planned
	default:
		planned.outcome = challengeDeliveryRepairOutcomeSkipped
		planned.skipReason = "unsupported_repair_mode"
		return planned
	}
}

func (store *ChallengeDeliveryStore) lockChallengeDeliveryForRepair(ctx context.Context, tx pgx.Tx, id int64) (challengeDeliveryRepairRow, error) {
	var row challengeDeliveryRepairRow
	err := tx.QueryRow(ctx, `
SELECT
    current.id,
    current.tenant_id,
    current.user_id,
    current.challenge_id,
    current.challenge_type,
    current.channel,
    current.destination,
    current.status,
    current.retry_count,
    current.last_error,
    current.failure_class,
    current.dead_lettered_at,
    current.expires_at,
    challenge.status,
    challenge.delivery_status,
    challenge.expires_at
FROM identity_challenge_delivery_outbox current
JOIN identity_challenges challenge
  ON challenge.tenant_id = current.tenant_id
 AND challenge.user_id = current.user_id
 AND challenge.challenge_id = current.challenge_id
WHERE current.id = $1
FOR UPDATE OF current, challenge
`, id).Scan(
		&row.id,
		&row.tenantID,
		&row.userID,
		&row.challengeID,
		&row.challengeType,
		&row.channel,
		&row.destination,
		&row.deliveryStatus,
		&row.retryCount,
		&row.lastError,
		&row.failureClass,
		&row.deadLetteredAt,
		&row.deliveryExpiresAt,
		&row.challengeStatus,
		&row.challengeDeliveryStatus,
		&row.challengeExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return challengeDeliveryRepairRow{}, err
		}
		return challengeDeliveryRepairRow{}, types.NewDBWriteFailed(err.Error())
	}
	return row, nil
}

func (store *ChallengeDeliveryStore) applyChallengeDeliveryRepair(ctx context.Context, tx pgx.Tx, row challengeDeliveryRepairRow, planned plannedChallengeDeliveryRepair, now time.Time) error {
	switch {
	case planned.redriveActivePending:
		return store.redriveActivePendingDelivery(ctx, tx, row, now)
	case planned.cancelInactiveChallengeDelivery:
		return store.cancelSelectedInactiveDelivery(ctx, tx, row, now)
	default:
		return nil
	}
}

func (store *ChallengeDeliveryStore) redriveActivePendingDelivery(ctx context.Context, tx pgx.Tx, row challengeDeliveryRepairRow, now time.Time) error {
	tag, err := tx.Exec(ctx, `
UPDATE identity_challenge_delivery_outbox
SET last_error = '',
    failure_class = '',
    next_retry_at = NULL,
    available_at = $2,
    updated_at = $2
WHERE id = $1
`, row.id, now)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if tag.RowsAffected() != 1 {
		return types.NewDBWriteFailed("identity challenge delivery redrive row count mismatch")
	}

	tag, err = tx.Exec(ctx, `
UPDATE identity_challenges
SET delivery_status = 'PENDING',
    delivery_failed_at = NULL,
    delivery_last_error = '',
    delivery_failure_class = '',
    updated_at = $4
WHERE tenant_id = $1
  AND user_id = $2
  AND challenge_id = $3
`, row.tenantID, row.userID, row.challengeID, now)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if tag.RowsAffected() != 1 {
		return types.NewDBWriteFailed("identity challenge redrive row count mismatch")
	}
	return nil
}

func (store *ChallengeDeliveryStore) cancelSelectedInactiveDelivery(ctx context.Context, tx pgx.Tx, row challengeDeliveryRepairRow, now time.Time) error {
	tag, err := tx.Exec(ctx, `
UPDATE identity_challenge_delivery_outbox
SET status = 'CANCELED',
    last_error = 'challenge no longer active before delivery',
    failure_class = 'inactive',
    next_retry_at = NULL,
    updated_at = $2
WHERE id = $1
`, row.id, now)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if tag.RowsAffected() != 1 {
		return types.NewDBWriteFailed("identity challenge delivery cancel row count mismatch")
	}
	tag, err = tx.Exec(ctx, `
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
`, row.tenantID, row.userID, row.challengeID, now)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if tag.RowsAffected() != 1 {
		return types.NewDBWriteFailed("identity challenge cancel row count mismatch")
	}
	return nil
}

func (store *ChallengeDeliveryStore) insertChallengeDeliveryRepairAudit(
	ctx context.Context,
	tx pgx.Tx,
	row challengeDeliveryRepairRow,
	planned plannedChallengeDeliveryRepair,
	mode string,
	outcome string,
	operator string,
	reason string,
	dryRun bool,
	now time.Time,
) error {
	_, err := tx.Exec(ctx, `
INSERT INTO identity_challenge_delivery_repair_audit (
    delivery_id,
    tenant_id,
    user_id,
    challenge_id,
    previous_delivery_status,
    previous_challenge_status,
    previous_challenge_delivery_status,
    previous_retry_count,
    previous_last_error,
    previous_failure_class,
    previous_dead_lettered_at,
    new_delivery_status,
    new_challenge_status,
    new_challenge_delivery_status,
    new_failure_class,
    repair_mode,
    repair_outcome,
    skip_reason,
    dry_run,
    repair_operator,
    repair_reason,
    repaired_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
    $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
    $21, $22
)
`, row.id, row.tenantID, row.userID, row.challengeID,
		row.deliveryStatus, row.challengeStatus, row.challengeDeliveryStatus,
		row.retryCount, row.lastError, row.failureClass, row.deadLetteredAt,
		planned.newDeliveryStatus, planned.newChallengeStatus, planned.newChallengeDeliveryStatus, planned.newFailureClass,
		mode, outcome, planned.skipReason, dryRun, operator, reason, now)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func normalizeChallengeDeliveryRepairMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return types.ChallengeDeliveryRepairModeAudit
	}
	switch mode {
	case types.ChallengeDeliveryRepairModeAudit,
		types.ChallengeDeliveryRepairModeRedriveActivePending,
		types.ChallengeDeliveryRepairModeCancelInactive:
		return mode
	default:
		return ""
	}
}

func normalizeChallengeDeliveryRepairOutcome(outcome string) string {
	switch strings.ToUpper(strings.TrimSpace(outcome)) {
	case challengeDeliveryRepairOutcomeAudited,
		challengeDeliveryRepairOutcomeMutated,
		challengeDeliveryRepairOutcomeSkipped:
		return strings.ToUpper(strings.TrimSpace(outcome))
	default:
		return ""
	}
}

func normalizeChallengeDeliveryFailureClass(value string) string {
	switch strings.TrimSpace(value) {
	case types.ChallengeDeliveryFailureClassConfiguration,
		types.ChallengeDeliveryFailureClassProviderNonSuccess,
		types.ChallengeDeliveryFailureClassTimeout,
		types.ChallengeDeliveryFailureClassNetwork,
		types.ChallengeDeliveryFailureClassSerialization,
		types.ChallengeDeliveryFailureClassTokenCrypto,
		types.ChallengeDeliveryFailureClassDeliveryFailed,
		types.ChallengeDeliveryFailureClassCanceled,
		types.ChallengeDeliveryFailureClassInactive,
		types.ChallengeDeliveryFailureClassUnknown:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func normalizeChallengeDeliveryIDs(deliveryIDs []int64) []int64 {
	seen := make(map[int64]struct{}, len(deliveryIDs))
	ids := make([]int64, 0, len(deliveryIDs))
	for _, id := range deliveryIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func normalizeChallengeDeliveryRepairText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	if len(value) > 256 {
		return value[:256]
	}
	return value
}
