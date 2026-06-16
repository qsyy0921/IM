package postgres

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type OutboxStore struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

type OutboxStoreOption func(*OutboxStore)

type OutboxRepairValidator func(types.OutboxMessage) error

type OutboxAuditOptions struct {
	OutboxID    *int64
	EventID     string
	TenantID    string
	AggregateID string
	Status      string
	EventType   string
	Limit       int
}

type OutboxAuditRow struct {
	ID               int64
	EventID          string
	TenantID         string
	AggregateType    string
	AggregateID      string
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
	EventID        string
	TenantID       string
	Operator       string
	Outcome        string
	RepairedAfter  *time.Time
	RepairedBefore *time.Time
	Limit          int
}

type OutboxRepairAuditRow struct {
	EventID                string
	TenantID               string
	Operator               string
	Reason                 string
	PreviousStatus         string
	PreviousRetryCount     int
	PreviousLastError      string
	PreviousDeadLetteredAt *time.Time
	Outcome                string
	SkipReason             string
	RepairedAt             time.Time
}

type OutboxRepairCleanupOptions struct {
	EventID  string
	TenantID string
	Operator string
	Outcome  string
	Cutoff   time.Time
	Limit    int
}

type OutboxRepairCleanupStats struct {
	Deleted int64
}

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
		return types.OutboxRelayStats{}, errors.New("policy audit outbox store is not configured")
	}
	if publish == nil {
		return types.OutboxRelayStats{}, errors.New("policy audit outbox publish callback is not configured")
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
		return types.OutboxRelayStats{}, errors.New("policy audit outbox batch publish result count mismatch")
	}

	now := store.now()
	publishedIDs := make([]int64, 0, len(messages))
	for index, message := range messages {
		if err := publishErrors[index]; err != nil {
			attempt := message.RetryCount + 1
			lastError := sanitizePolicyOutboxPublishError(err)
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

func (store *OutboxStore) RepairDLQEvents(ctx context.Context, eventIDs []string, operator string, reason string, validate OutboxRepairValidator) (types.OutboxRepairStats, error) {
	if store == nil || store.pool == nil {
		return types.OutboxRepairStats{}, errors.New("policy audit outbox store is not configured")
	}
	if validate == nil {
		return types.OutboxRepairStats{}, types.NewInvalidArgument("policy audit outbox repair validator is required")
	}
	ids := normalizeEventIDs(eventIDs)
	if len(ids) == 0 {
		return types.OutboxRepairStats{}, types.NewInvalidArgument("event_ids are required")
	}
	if strings.TrimSpace(reason) == "" {
		reason = "manual policy audit outbox repair"
	}
	operator = truncateRepairField(strings.TrimSpace(operator), 128)
	reason = truncateRepairField(strings.TrimSpace(reason), 256)

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return types.OutboxRepairStats{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	targets, err := fetchRepairTargetsLocked(ctx, tx, ids)
	if err != nil {
		return types.OutboxRepairStats{}, err
	}
	stats := types.OutboxRepairStats{
		Requested: len(ids),
		Skipped:   len(ids) - len(targets),
	}
	for _, target := range targets {
		if err := validate(target.message); err != nil {
			if err := insertRepairAudit(ctx, tx, target, operator, reason, "SKIPPED", "validation_failed"); err != nil {
				return types.OutboxRepairStats{}, err
			}
			stats.Skipped++
			stats.Invalid++
			continue
		}
		if err := repairOutboxRow(ctx, tx, target.message.ID); err != nil {
			return types.OutboxRepairStats{}, err
		}
		if err := insertRepairAudit(ctx, tx, target, operator, reason, "REPAIRED", ""); err != nil {
			return types.OutboxRepairStats{}, err
		}
		stats.Repaired++
	}
	if err := tx.Commit(ctx); err != nil {
		return types.OutboxRepairStats{}, types.NewDBWriteFailed(err.Error())
	}
	return stats, nil
}

func (store *OutboxStore) AuditOutbox(ctx context.Context, options OutboxAuditOptions) ([]OutboxAuditRow, error) {
	if store == nil || store.pool == nil {
		return nil, errors.New("policy audit outbox store is not configured")
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
		clauses = append(clauses, "id = $"+strconv.Itoa(len(args)))
	}
	if eventID := strings.TrimSpace(options.EventID); eventID != "" {
		args = append(args, eventID)
		clauses = append(clauses, "event_id = $"+strconv.Itoa(len(args)))
	}
	if tenantID := strings.TrimSpace(options.TenantID); tenantID != "" {
		args = append(args, tenantID)
		clauses = append(clauses, "tenant_id = $"+strconv.Itoa(len(args)))
	}
	if aggregateID := strings.TrimSpace(options.AggregateID); aggregateID != "" {
		args = append(args, aggregateID)
		clauses = append(clauses, "aggregate_id = $"+strconv.Itoa(len(args)))
	}
	if rawStatus := strings.TrimSpace(options.Status); rawStatus != "" {
		status := normalizePolicyOutboxStatus(rawStatus)
		if status == "" {
			return nil, types.NewInvalidArgument("unsupported policy outbox status")
		}
		args = append(args, status)
		clauses = append(clauses, "status = $"+strconv.Itoa(len(args)))
	}
	if eventType := strings.TrimSpace(options.EventType); eventType != "" {
		args = append(args, eventType)
		clauses = append(clauses, "event_type = $"+strconv.Itoa(len(args)))
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
    aggregate_type,
    aggregate_id,
    aggregate_version,
    event_type,
    status,
    retry_count,
    COALESCE(last_error, ''),
    available_at,
    next_retry_at,
    dead_lettered_at,
    published_at,
    created_at
FROM policy_decision_audit_outbox
`+where+`
ORDER BY created_at DESC, id DESC
LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer rows.Close()

	result := make([]OutboxAuditRow, 0, limit)
	for rows.Next() {
		var row OutboxAuditRow
		if err := rows.Scan(
			&row.ID,
			&row.EventID,
			&row.TenantID,
			&row.AggregateType,
			&row.AggregateID,
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
			return nil, types.NewDBWriteFailed(err.Error())
		}
		row.LastError = sanitizePolicyOutboxStoredError(row.LastError)
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func (store *OutboxStore) AuditOutboxRepairs(ctx context.Context, options OutboxRepairAuditOptions) ([]OutboxRepairAuditRow, error) {
	if store == nil || store.pool == nil {
		return nil, errors.New("policy audit outbox store is not configured")
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
	clauses := make([]string, 0, 4)
	if eventID := strings.TrimSpace(options.EventID); eventID != "" {
		args = append(args, eventID)
		clauses = append(clauses, "event_id = $"+strconv.Itoa(len(args)))
	}
	if tenantID := strings.TrimSpace(options.TenantID); tenantID != "" {
		args = append(args, tenantID)
		clauses = append(clauses, "tenant_id = $"+strconv.Itoa(len(args)))
	}
	if operator := strings.TrimSpace(options.Operator); operator != "" {
		args = append(args, operator)
		clauses = append(clauses, "repair_operator = $"+strconv.Itoa(len(args)))
	}
	if rawOutcome := strings.TrimSpace(options.Outcome); rawOutcome != "" {
		outcome := normalizePolicyOutboxRepairOutcome(rawOutcome)
		if outcome == "" {
			return nil, types.NewInvalidArgument("unsupported policy outbox repair outcome")
		}
		args = append(args, outcome)
		clauses = append(clauses, "repair_outcome = $"+strconv.Itoa(len(args)))
	}
	if options.RepairedAfter != nil {
		args = append(args, options.RepairedAfter.UTC())
		clauses = append(clauses, "repaired_at >= $"+strconv.Itoa(len(args)))
	}
	if options.RepairedBefore != nil {
		args = append(args, options.RepairedBefore.UTC())
		clauses = append(clauses, "repaired_at < $"+strconv.Itoa(len(args)))
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
    COALESCE(previous_last_error, ''),
    previous_dead_lettered_at,
    repair_operator,
    repair_reason,
    repair_outcome,
    skip_reason,
    repaired_at
FROM policy_decision_audit_outbox_repair_audit
`+where+`
ORDER BY repaired_at DESC, event_id, id DESC
LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
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
			&row.Operator,
			&row.Reason,
			&row.Outcome,
			&row.SkipReason,
			&row.RepairedAt,
		); err != nil {
			return nil, types.NewDBWriteFailed(err.Error())
		}
		row.PreviousLastError = sanitizePolicyOutboxStoredError(row.PreviousLastError)
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func (store *OutboxStore) CleanupOutboxRepairs(ctx context.Context, options OutboxRepairCleanupOptions) (OutboxRepairCleanupStats, error) {
	if store == nil || store.pool == nil {
		return OutboxRepairCleanupStats{}, errors.New("policy audit outbox store is not configured")
	}
	if options.Limit <= 0 {
		return OutboxRepairCleanupStats{}, nil
	}

	var args []any
	clauses := []string{"repaired_at < $1"}
	args = append(args, options.Cutoff)
	if eventID := strings.TrimSpace(options.EventID); eventID != "" {
		args = append(args, eventID)
		clauses = append(clauses, "event_id = $"+strconv.Itoa(len(args)))
	}
	if tenantID := strings.TrimSpace(options.TenantID); tenantID != "" {
		args = append(args, tenantID)
		clauses = append(clauses, "tenant_id = $"+strconv.Itoa(len(args)))
	}
	if operator := strings.TrimSpace(options.Operator); operator != "" {
		args = append(args, operator)
		clauses = append(clauses, "repair_operator = $"+strconv.Itoa(len(args)))
	}
	if rawOutcome := strings.TrimSpace(options.Outcome); rawOutcome != "" {
		outcome := normalizePolicyOutboxRepairOutcome(rawOutcome)
		if outcome == "" {
			return OutboxRepairCleanupStats{}, types.NewInvalidArgument("unsupported policy outbox repair outcome")
		}
		args = append(args, outcome)
		clauses = append(clauses, "repair_outcome = $"+strconv.Itoa(len(args)))
	}
	args = append(args, options.Limit)
	rows, err := store.pool.Query(ctx, `
WITH doomed AS (
    SELECT id
    FROM policy_decision_audit_outbox_repair_audit
    WHERE `+strings.Join(clauses, " AND ")+`
    ORDER BY repaired_at ASC, event_id ASC, id ASC
    LIMIT $`+strconv.Itoa(len(args))+`
    FOR UPDATE SKIP LOCKED
)
DELETE FROM policy_decision_audit_outbox_repair_audit target
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

type outboxRepairTarget struct {
	message                types.OutboxMessage
	previousStatus         string
	previousRetryCount     int
	previousLastError      string
	previousDeadLetteredAt *time.Time
}

func fetchRepairTargetsLocked(ctx context.Context, tx pgx.Tx, eventIDs []string) ([]outboxRepairTarget, error) {
	rows, err := tx.Query(ctx, `
WITH requested AS (
    SELECT DISTINCT unnest($1::text[]) AS event_id
)
SELECT
    pao.id,
    pao.event_id,
    pao.tenant_id,
    pao.aggregate_type,
    pao.aggregate_id,
    pao.aggregate_version,
    pao.event_type,
    pao.event_version,
    pao.partition_key,
    pao.mapping_version,
    pao.correlation_id,
    pao.causation_id,
    pao.producer,
    pao.payload_json,
    pao.trace_id,
    pao.retry_count,
    pao.created_at,
    pao.status,
    COALESCE(pao.last_error, ''),
    pao.dead_lettered_at
FROM policy_decision_audit_outbox pao
JOIN requested r ON r.event_id = pao.event_id
WHERE pao.status = $2
ORDER BY pao.id
FOR UPDATE OF pao
`, eventIDs, types.OutboxStatusDLQ)
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer rows.Close()

	targets := make([]outboxRepairTarget, 0, len(eventIDs))
	for rows.Next() {
		var target outboxRepairTarget
		if err := rows.Scan(
			&target.message.ID,
			&target.message.EventID,
			&target.message.TenantID,
			&target.message.AggregateType,
			&target.message.AggregateID,
			&target.message.AggregateVersion,
			&target.message.EventType,
			&target.message.EventVersion,
			&target.message.PartitionKey,
			&target.message.MappingVersion,
			&target.message.CorrelationID,
			&target.message.CausationID,
			&target.message.Producer,
			&target.message.PayloadJSON,
			&target.message.TraceID,
			&target.message.RetryCount,
			&target.message.OccurredAt,
			&target.previousStatus,
			&target.previousLastError,
			&target.previousDeadLetteredAt,
		); err != nil {
			return nil, types.NewDBWriteFailed(err.Error())
		}
		target.previousRetryCount = target.message.RetryCount
		target.previousLastError = sanitizePolicyOutboxStoredError(target.previousLastError)
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	return targets, nil
}

func repairOutboxRow(ctx context.Context, tx pgx.Tx, id int64) error {
	tag, err := tx.Exec(ctx, `
UPDATE policy_decision_audit_outbox
SET status = $2,
    retry_count = 0,
    last_error = '',
    next_retry_at = NULL,
    dead_lettered_at = NULL,
    available_at = now(),
    updated_at = now()
WHERE id = $1
`, id, types.OutboxStatusPending)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if tag.RowsAffected() != 1 {
		return types.NewDBWriteFailed("policy audit outbox repair row count mismatch")
	}
	return nil
}

func insertRepairAudit(ctx context.Context, tx pgx.Tx, target outboxRepairTarget, operator string, reason string, outcome string, skipReason string) error {
	_, err := tx.Exec(ctx, `
INSERT INTO policy_decision_audit_outbox_repair_audit (
    event_id,
    tenant_id,
    previous_status,
    previous_retry_count,
    previous_last_error,
    previous_dead_lettered_at,
    repair_operator,
    repair_reason,
    repair_outcome,
    skip_reason
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    $9,
    $10
)
`,
		target.message.EventID,
		target.message.TenantID,
		target.previousStatus,
		target.previousRetryCount,
		target.previousLastError,
		target.previousDeadLetteredAt,
		operator,
		reason,
		outcome,
		truncateRepairField(strings.TrimSpace(skipReason), 128),
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
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
FROM policy_decision_audit_outbox current
WHERE status = 'PENDING'
  AND published_at IS NULL
  AND COALESCE(next_retry_at, available_at) <= now()
  AND NOT EXISTS (
      SELECT 1
      FROM policy_decision_audit_outbox previous
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
UPDATE policy_decision_audit_outbox
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
		return types.NewDBWriteFailed("policy audit outbox published row count mismatch")
	}
	return nil
}

func (store *OutboxStore) markRetry(ctx context.Context, tx pgx.Tx, id int64, retryCount int, lastError string, nextRetryAt time.Time) error {
	_, err := tx.Exec(ctx, `
UPDATE policy_decision_audit_outbox
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
UPDATE policy_decision_audit_outbox
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

func normalizeEventIDs(eventIDs []string) []string {
	seen := make(map[string]struct{}, len(eventIDs))
	ids := make([]string, 0, len(eventIDs))
	for _, eventID := range eventIDs {
		eventID = strings.TrimSpace(eventID)
		if eventID == "" {
			continue
		}
		if _, ok := seen[eventID]; ok {
			continue
		}
		seen[eventID] = struct{}{}
		ids = append(ids, eventID)
	}
	return ids
}

func normalizePolicyOutboxRepairOutcome(outcome string) string {
	switch strings.ToUpper(strings.TrimSpace(outcome)) {
	case "REPAIRED", "SKIPPED":
		return strings.ToUpper(strings.TrimSpace(outcome))
	default:
		return ""
	}
}

func normalizePolicyOutboxStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
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

func truncateRepairField(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
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

func sanitizePolicyOutboxStoredError(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return sanitizePolicyOutboxErrorText(value)
}

func sanitizePolicyOutboxPublishError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "policy audit outbox publish canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "policy audit outbox publish timeout"
	}
	return sanitizePolicyOutboxErrorText(err.Error())
}

func sanitizePolicyOutboxErrorText(value string) string {
	message := strings.ToLower(strings.TrimSpace(value))
	switch {
	case message == "":
		return "policy audit outbox publish failed"
	case strings.Contains(message, "cancel"):
		return "policy audit outbox publish canceled"
	case strings.Contains(message, "timeout") || strings.Contains(message, "deadline exceeded"):
		return "policy audit outbox publish timeout"
	case strings.Contains(message, "unsupported"):
		return "policy audit outbox publish unsupported event"
	case strings.Contains(message, "malformed") ||
		strings.Contains(message, "invalid") ||
		strings.Contains(message, "json") ||
		strings.Contains(message, "decode") ||
		strings.Contains(message, "payload"):
		return "policy audit outbox publish invalid payload"
	case strings.Contains(message, "kafka") ||
		strings.Contains(message, "broker") ||
		strings.Contains(message, "leader") ||
		strings.Contains(message, "connection refused") ||
		strings.Contains(message, "no such host") ||
		strings.Contains(message, "network"):
		return "policy audit outbox publish broker unavailable"
	default:
		return "policy audit outbox publish failed"
	}
}
