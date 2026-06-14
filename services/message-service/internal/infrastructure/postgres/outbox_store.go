package postgres

import (
	"context"
	"errors"
	"strconv"
	"strings"
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
	EventID        string
	TenantID       string
	ConversationID string
	Limit          int
}

type OutboxRepairCleanupOptions struct {
	EventID        string
	TenantID       string
	ConversationID string
	Cutoff         time.Time
	Limit          int
}

type OutboxRepairCleanupStats struct {
	Deleted int64
}

type OutboxRepairAuditRow struct {
	EventID                string
	TenantID               string
	ConversationID         string
	Reason                 string
	PreviousStatus         string
	PreviousRetryCount     int
	PreviousLastError      string
	PreviousDeadLetteredAt *time.Time
	RepairedAt             time.Time
}

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
	if publish == nil {
		return types.OutboxRelayStats{}, errors.New("outbox publish callback is not configured")
	}
	return s.ProcessReadyBatch(ctx, limit, maxAttempts, retryBaseDelay, func(ctx context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for i, message := range messages {
			errs[i] = publish(ctx, message)
		}
		return errs
	})
}

func (s *OutboxStore) ProcessReadyBatch(
	ctx context.Context,
	limit int,
	maxAttempts int,
	retryBaseDelay time.Duration,
	publish func(context.Context, []types.OutboxMessage) []error,
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
	if len(messages) == 0 {
		commitStarted := time.Now()
		if err := tx.Commit(ctx); err != nil {
			s.metrics.ObserveOutboxCommit(time.Since(commitStarted))
			return types.OutboxRelayStats{}, types.NewDBWriteFailed(err.Error())
		}
		s.metrics.ObserveOutboxCommit(time.Since(commitStarted))
		return stats, nil
	}

	now := s.now()
	publishedIDs := make([]int64, 0, len(messages))
	publishErrors := publish(ctx, messages)
	if len(publishErrors) != len(messages) {
		return types.OutboxRelayStats{}, errors.New("outbox batch publish result count mismatch")
	}

	for index, message := range messages {
		if err := publishErrors[index]; err != nil {
			attempt := message.RetryCount + 1
			lastError := sanitizeOutboxPublishError(err)
			if attempt >= maxAttempts {
				if markErr := s.markDeadLettered(ctx, tx, message.ID, attempt, lastError, now); markErr != nil {
					return types.OutboxRelayStats{}, markErr
				}
				stats.DeadLettered++
				continue
			}
			nextRetryAt := now.Add(retryDelay(retryBaseDelay, attempt))
			if markErr := s.markRetry(ctx, tx, message.ID, attempt, lastError, nextRetryAt); markErr != nil {
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

func (s *OutboxStore) AuditOutbox(ctx context.Context, options OutboxAuditOptions) ([]OutboxAuditRow, error) {
	if s.pool == nil {
		return nil, ErrRepositoryNotConfigured
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
	if conversationID := strings.TrimSpace(options.ConversationID); conversationID != "" {
		args = append(args, conversationID)
		clauses = append(clauses, "conversation_id = $"+strconv.Itoa(len(args)))
	}
	if rawStatus := strings.TrimSpace(options.Status); rawStatus != "" {
		status := normalizeOutboxStatus(rawStatus)
		if status == "" {
			return nil, errors.New("unsupported message outbox status")
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
	rows, err := s.pool.Query(ctx, `
SELECT
    id,
    event_id,
    tenant_id,
    conversation_id,
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
FROM message_outbox
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
			return nil, types.NewDBWriteFailed(err.Error())
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func (s *OutboxStore) RepairDLQEvents(ctx context.Context, eventIDs []string, reason string) (types.OutboxRepairStats, error) {
	if s.pool == nil {
		return types.OutboxRepairStats{}, ErrRepositoryNotConfigured
	}
	ids := make([]string, 0, len(eventIDs))
	seen := make(map[string]struct{}, len(eventIDs))
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
	if len(ids) == 0 {
		return types.OutboxRepairStats{}, nil
	}
	if reason = strings.TrimSpace(reason); reason == "" {
		reason = "manual message outbox repair"
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return types.OutboxRepairStats{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var stats types.OutboxRepairStats
	err = tx.QueryRow(ctx, `
WITH requested AS (
    SELECT DISTINCT UNNEST($1::text[]) AS event_id
),
target AS (
    SELECT
        mo.id,
        mo.event_id,
        mo.tenant_id,
        mo.conversation_id,
        mo.status,
        mo.retry_count,
        COALESCE(mo.last_error, '') AS last_error,
        mo.dead_lettered_at
    FROM message_outbox mo
    JOIN requested r ON r.event_id = mo.event_id
    WHERE mo.status = $3
    FOR UPDATE OF mo
),
updated AS (
    UPDATE message_outbox mo
    SET status = $2,
        retry_count = 0,
        last_error = NULL,
        next_retry_at = NULL,
        dead_lettered_at = NULL,
        available_at = now()
    FROM target t
    WHERE mo.id = t.id
    RETURNING mo.event_id
),
audit AS (
    INSERT INTO message_outbox_repair_audit (
        event_id,
        tenant_id,
        conversation_id,
        previous_status,
        previous_retry_count,
        previous_last_error,
        previous_dead_lettered_at,
        repair_reason
    )
    SELECT
        event_id,
        tenant_id,
        conversation_id,
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

func (s *OutboxStore) AuditOutboxRepairs(ctx context.Context, options OutboxRepairAuditOptions) ([]OutboxRepairAuditRow, error) {
	if s.pool == nil {
		return nil, ErrRepositoryNotConfigured
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
	if conversationID := strings.TrimSpace(options.ConversationID); conversationID != "" {
		args = append(args, conversationID)
		clauses = append(clauses, "conversation_id = $"+strconv.Itoa(len(args)))
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	args = append(args, limit)
	rows, err := s.pool.Query(ctx, `
SELECT
    event_id,
    tenant_id,
    conversation_id,
    previous_status,
    previous_retry_count,
    previous_last_error,
    previous_dead_lettered_at,
    repair_reason,
    repaired_at
FROM message_outbox_repair_audit
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
			&row.ConversationID,
			&row.PreviousStatus,
			&row.PreviousRetryCount,
			&row.PreviousLastError,
			&row.PreviousDeadLetteredAt,
			&row.Reason,
			&row.RepairedAt,
		); err != nil {
			return nil, types.NewDBWriteFailed(err.Error())
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func (s *OutboxStore) CleanupOutboxRepairs(ctx context.Context, options OutboxRepairCleanupOptions) (OutboxRepairCleanupStats, error) {
	if s.pool == nil {
		return OutboxRepairCleanupStats{}, ErrRepositoryNotConfigured
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
	if conversationID := strings.TrimSpace(options.ConversationID); conversationID != "" {
		args = append(args, conversationID)
		clauses = append(clauses, "conversation_id = $"+strconv.Itoa(len(args)))
	}
	args = append(args, options.Limit)
	rows, err := s.pool.Query(ctx, `
WITH doomed AS (
    SELECT id
    FROM message_outbox_repair_audit
    WHERE `+strings.Join(clauses, " AND ")+`
    ORDER BY repaired_at ASC, event_id ASC, id ASC
    LIMIT $`+strconv.Itoa(len(args))+`
    FOR UPDATE SKIP LOCKED
)
DELETE FROM message_outbox_repair_audit target
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

func sanitizeOutboxPublishError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "outbox publish canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "outbox publish timeout"
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case message == "":
		return "outbox publish failed"
	case strings.Contains(message, "timeout") || strings.Contains(message, "deadline exceeded"):
		return "outbox publish timeout"
	case strings.Contains(message, "unsupported"):
		return "outbox publish unsupported event"
	case strings.Contains(message, "malformed") ||
		strings.Contains(message, "invalid") ||
		strings.Contains(message, "json") ||
		strings.Contains(message, "decode"):
		return "outbox publish invalid event"
	case strings.Contains(message, "kafka") ||
		strings.Contains(message, "broker") ||
		strings.Contains(message, "leader") ||
		strings.Contains(message, "connection refused") ||
		strings.Contains(message, "no such host") ||
		strings.Contains(message, "network"):
		return "outbox publish broker unavailable"
	default:
		return "outbox publish failed"
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

func normalizeOutboxStatus(status string) string {
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
