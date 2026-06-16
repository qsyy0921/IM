package postgres

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

type ProjectionFailureStore struct {
	pool *pgxpool.Pool
}

type ProjectionFailureAuditOptions struct {
	ConsumerGroup  string
	Topic          string
	PartitionID    *int32
	OffsetValue    *int64
	EventID        string
	EventType      string
	FailureClass   string
	LastSeenAfter  *time.Time
	LastSeenBefore *time.Time
	UnresolvedOnly bool
	Limit          int
}

type ProjectionFailureAuditRow struct {
	ConsumerGroup            string
	Topic                    string
	PartitionID              int32
	OffsetValue              int64
	EventID                  string
	EventType                string
	FailureClass             string
	FailureCount             int64
	LastError                string
	LastSeenAt               time.Time
	ResolvedAt               *time.Time
	ResolvedCheckpointOffset *int64
}

type ProjectionFailureCleanupStats struct {
	Deleted int64
}

type ProjectionFailureResolveAuditRow struct {
	ConsumerGroup         string
	Topic                 string
	PartitionID           int32
	OffsetValue           int64
	EventID               string
	FailureClass          string
	Operator              string
	Reason                string
	DryRun                bool
	Outcome               string
	CheckpointOffsetValue *int64
	CreatedAt             time.Time
}

type ProjectionFailureCleanupOptions struct {
	ConsumerGroup string
	Topic         string
	PartitionID   *int32
	FailureClass  string
	Cutoff        time.Time
	Limit         int
}

const (
	projectionFailureResolutionOutcomeAudited  = "AUDITED"
	projectionFailureResolutionOutcomeResolved = "RESOLVED"
)

func NewProjectionFailureStore(pool *pgxpool.Pool) *ProjectionFailureStore {
	return &ProjectionFailureStore{pool: pool}
}

func (store *ProjectionFailureStore) RecordFailure(ctx context.Context, record types.ProjectionFailureRecord) error {
	if store == nil || store.pool == nil {
		return errors.New("delivery projection failure store is not configured")
	}
	if strings.TrimSpace(record.ConsumerGroup) == "" {
		return types.NewInvalidArgument("consumer_group is required")
	}
	if strings.TrimSpace(record.Topic) == "" {
		return types.NewInvalidArgument("topic is required")
	}
	if record.PartitionID < 0 {
		return types.NewInvalidArgument("partition_id must be non-negative")
	}
	if record.OffsetValue < 0 {
		return types.NewInvalidArgument("offset_value must be non-negative")
	}
	if strings.TrimSpace(record.FailureClass) == "" {
		record.FailureClass = types.ProjectionFailureClassUnknown
	}
	record.LastError = sanitizeProjectionFailureError(record.FailureClass, record.LastError)
	_, err := store.pool.Exec(ctx, `
INSERT INTO delivery_projection_failures (
    consumer_group,
    topic,
    partition_id,
    offset_value,
    event_id,
    event_type,
    tenant_id,
    conversation_id,
    aggregate_version,
    trace_id,
    failure_class,
    last_error,
    first_seen_at,
    last_seen_at,
    failure_count,
    resolved_at,
    resolved_checkpoint_offset
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, now(), now(), 1, NULL, NULL
)
ON CONFLICT (consumer_group, topic, partition_id, offset_value) DO UPDATE
SET event_id = EXCLUDED.event_id,
    event_type = EXCLUDED.event_type,
    tenant_id = EXCLUDED.tenant_id,
    conversation_id = EXCLUDED.conversation_id,
    aggregate_version = EXCLUDED.aggregate_version,
    trace_id = EXCLUDED.trace_id,
    failure_class = EXCLUDED.failure_class,
    last_error = EXCLUDED.last_error,
    last_seen_at = now(),
    failure_count = delivery_projection_failures.failure_count + 1,
    resolved_at = NULL,
    resolved_checkpoint_offset = NULL
`, record.ConsumerGroup, record.Topic, record.PartitionID, record.OffsetValue, record.EventID, record.EventType, record.TenantID, record.ConversationID, record.AggregateVersion, record.TraceID, record.FailureClass, record.LastError)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func (store *ProjectionFailureStore) AuditFailures(ctx context.Context, options ProjectionFailureAuditOptions) ([]ProjectionFailureAuditRow, error) {
	if store == nil || store.pool == nil {
		return nil, errors.New("delivery projection failure store is not configured")
	}
	limit := options.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	if options.LastSeenAfter != nil && options.LastSeenBefore != nil && !options.LastSeenAfter.Before(*options.LastSeenBefore) {
		return nil, types.NewInvalidArgument("last_seen_after must be before last_seen_before")
	}
	topic := strings.TrimSpace(options.Topic)
	if topic == "" {
		topic = "conversation.timeline.events"
	}

	var args []any
	clauses := []string{"topic = $1"}
	args = append(args, topic)
	if consumerGroup := strings.TrimSpace(options.ConsumerGroup); consumerGroup != "" {
		args = append(args, consumerGroup)
		clauses = append(clauses, "consumer_group = $"+itoa(len(args)))
	}
	if options.PartitionID != nil {
		args = append(args, *options.PartitionID)
		clauses = append(clauses, "partition_id = $"+itoa(len(args)))
	}
	if options.OffsetValue != nil {
		args = append(args, *options.OffsetValue)
		clauses = append(clauses, "offset_value = $"+itoa(len(args)))
	}
	if eventID := strings.TrimSpace(options.EventID); eventID != "" {
		args = append(args, eventID)
		clauses = append(clauses, "event_id = $"+itoa(len(args)))
	}
	if eventType := strings.TrimSpace(options.EventType); eventType != "" {
		args = append(args, eventType)
		clauses = append(clauses, "event_type = $"+itoa(len(args)))
	}
	if failureClass := strings.TrimSpace(options.FailureClass); failureClass != "" {
		args = append(args, failureClass)
		clauses = append(clauses, "failure_class = $"+itoa(len(args)))
	}
	if options.LastSeenAfter != nil {
		args = append(args, options.LastSeenAfter.UTC())
		clauses = append(clauses, "last_seen_at >= $"+itoa(len(args)))
	}
	if options.LastSeenBefore != nil {
		args = append(args, options.LastSeenBefore.UTC())
		clauses = append(clauses, "last_seen_at < $"+itoa(len(args)))
	}
	if options.UnresolvedOnly {
		clauses = append(clauses, "resolved_at IS NULL")
	}
	args = append(args, limit)
	query := `
SELECT
    consumer_group,
    topic,
    partition_id,
    offset_value,
    event_id,
    event_type,
    failure_class,
    failure_count,
    last_error,
    last_seen_at,
    resolved_at,
    resolved_checkpoint_offset
FROM delivery_projection_failures
WHERE ` + strings.Join(clauses, " AND ") + `
ORDER BY last_seen_at DESC, consumer_group, partition_id, offset_value
LIMIT $` + itoa(len(args))
	rows, err := store.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	result := make([]ProjectionFailureAuditRow, 0, limit)
	for rows.Next() {
		var row ProjectionFailureAuditRow
		if err := rows.Scan(
			&row.ConsumerGroup,
			&row.Topic,
			&row.PartitionID,
			&row.OffsetValue,
			&row.EventID,
			&row.EventType,
			&row.FailureClass,
			&row.FailureCount,
			&row.LastError,
			&row.LastSeenAt,
			&row.ResolvedAt,
			&row.ResolvedCheckpointOffset,
		); err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		row.LastError = sanitizeProjectionFailureError(row.FailureClass, row.LastError)
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return result, nil
}

func (store *ProjectionFailureStore) ResolveFailure(ctx context.Context, options types.ProjectionFailureResolveOptions) (types.ProjectionFailureResolveStats, error) {
	if store == nil || store.pool == nil {
		return types.ProjectionFailureResolveStats{}, errors.New("delivery projection failure store is not configured")
	}
	if strings.TrimSpace(options.ConsumerGroup) == "" {
		return types.ProjectionFailureResolveStats{}, types.NewInvalidArgument("consumer_group is required")
	}
	topic := strings.TrimSpace(options.Topic)
	if topic == "" {
		topic = "conversation.timeline.events"
	}
	if options.PartitionID < 0 {
		return types.ProjectionFailureResolveStats{}, types.NewInvalidArgument("partition_id must be non-negative")
	}
	if options.OffsetValue < 0 {
		return types.ProjectionFailureResolveStats{}, types.NewInvalidArgument("offset_value must be non-negative")
	}
	operator := normalizeProjectionFailureResolveText(options.Operator, "manual")
	reason := normalizeProjectionFailureResolveText(options.Reason, "manual delivery projection failure resolution")

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return types.ProjectionFailureResolveStats{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	failure, err := readProjectionFailureForResolution(ctx, tx, options.ConsumerGroup, topic, options.PartitionID, options.OffsetValue)
	if err != nil {
		return types.ProjectionFailureResolveStats{}, err
	}
	checkpointOffset, err := readProjectionCheckpointOffset(ctx, tx, options.ConsumerGroup, topic, options.PartitionID)
	if err != nil {
		return types.ProjectionFailureResolveStats{}, err
	}
	outcome := projectionFailureResolutionOutcomeAudited
	if !options.DryRun {
		_, err := tx.Exec(ctx, `
UPDATE delivery_projection_failures
SET resolved_at = now(),
    resolved_checkpoint_offset = $5
WHERE consumer_group = $1
  AND topic = $2
  AND partition_id = $3
  AND offset_value = $4
  AND resolved_at IS NULL
`, options.ConsumerGroup, topic, options.PartitionID, options.OffsetValue, checkpointOffset)
		if err != nil {
			return types.ProjectionFailureResolveStats{}, types.NewDBWriteFailed(err.Error())
		}
		outcome = projectionFailureResolutionOutcomeResolved
	}
	if err := insertProjectionFailureResolutionAudit(ctx, tx, failure, operator, reason, options.DryRun, outcome, checkpointOffset); err != nil {
		return types.ProjectionFailureResolveStats{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.ProjectionFailureResolveStats{}, types.NewDBWriteFailed(err.Error())
	}
	stats := types.ProjectionFailureResolveStats{Requested: 1}
	if options.DryRun {
		stats.Audited = 1
	} else {
		stats.Resolved = 1
	}
	return stats, nil
}

func (store *ProjectionFailureStore) CleanupResolvedFailures(ctx context.Context, options ProjectionFailureCleanupOptions) (ProjectionFailureCleanupStats, error) {
	if store == nil || store.pool == nil {
		return ProjectionFailureCleanupStats{}, errors.New("delivery projection failure store is not configured")
	}
	if options.Limit <= 0 {
		return ProjectionFailureCleanupStats{}, nil
	}
	topic := strings.TrimSpace(options.Topic)
	if topic == "" {
		topic = "conversation.timeline.events"
	}
	var args []any
	clauses := []string{"topic = $1", "resolved_at IS NOT NULL", "resolved_at < $2"}
	args = append(args, topic, options.Cutoff)
	if consumerGroup := strings.TrimSpace(options.ConsumerGroup); consumerGroup != "" {
		args = append(args, consumerGroup)
		clauses = append(clauses, "consumer_group = $"+itoa(len(args)))
	}
	if options.PartitionID != nil {
		args = append(args, *options.PartitionID)
		clauses = append(clauses, "partition_id = $"+itoa(len(args)))
	}
	if failureClass := strings.TrimSpace(options.FailureClass); failureClass != "" {
		args = append(args, failureClass)
		clauses = append(clauses, "failure_class = $"+itoa(len(args)))
	}
	args = append(args, options.Limit)
	rows, err := store.pool.Query(ctx, `
WITH doomed AS (
    SELECT consumer_group, topic, partition_id, offset_value
    FROM delivery_projection_failures
    WHERE `+strings.Join(clauses, " AND ")+`
    ORDER BY resolved_at ASC, consumer_group, topic, partition_id, offset_value
    LIMIT $`+itoa(len(args))+`
    FOR UPDATE SKIP LOCKED
)
DELETE FROM delivery_projection_failures target
USING doomed
WHERE target.consumer_group = doomed.consumer_group
  AND target.topic = doomed.topic
  AND target.partition_id = doomed.partition_id
  AND target.offset_value = doomed.offset_value
RETURNING 1
`, args...)
	if err != nil {
		return ProjectionFailureCleanupStats{}, types.NewDBWriteFailed(err.Error())
	}
	defer rows.Close()

	var stats ProjectionFailureCleanupStats
	for rows.Next() {
		stats.Deleted++
	}
	if err := rows.Err(); err != nil {
		return ProjectionFailureCleanupStats{}, types.NewDBWriteFailed(err.Error())
	}
	return stats, nil
}

type projectionFailureResolutionRow struct {
	ConsumerGroup string
	Topic         string
	PartitionID   int32
	OffsetValue   int64
	EventID       string
	FailureClass  string
}

func readProjectionFailureForResolution(ctx context.Context, tx pgx.Tx, consumerGroup string, topic string, partitionID int32, offsetValue int64) (projectionFailureResolutionRow, error) {
	var row projectionFailureResolutionRow
	err := tx.QueryRow(ctx, `
SELECT consumer_group, topic, partition_id, offset_value, event_id, failure_class
FROM delivery_projection_failures
WHERE consumer_group = $1
  AND topic = $2
  AND partition_id = $3
  AND offset_value = $4
  AND resolved_at IS NULL
FOR UPDATE
`, consumerGroup, topic, partitionID, offsetValue).Scan(
		&row.ConsumerGroup,
		&row.Topic,
		&row.PartitionID,
		&row.OffsetValue,
		&row.EventID,
		&row.FailureClass,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return projectionFailureResolutionRow{}, types.NewInvalidArgument("unresolved delivery projection failure not found")
		}
		return projectionFailureResolutionRow{}, types.NewDBReadFailed(err.Error())
	}
	return row, nil
}

func readProjectionCheckpointOffset(ctx context.Context, tx pgx.Tx, consumerGroup string, topic string, partitionID int32) (*int64, error) {
	var value int64
	err := tx.QueryRow(ctx, `
SELECT offset_value
FROM delivery_kafka_checkpoints
WHERE consumer_group = $1
  AND topic = $2
  AND partition_id = $3
`, consumerGroup, topic, partitionID).Scan(&value)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, types.NewDBReadFailed(err.Error())
	}
	return &value, nil
}

func insertProjectionFailureResolutionAudit(
	ctx context.Context,
	tx pgx.Tx,
	failure projectionFailureResolutionRow,
	operator string,
	reason string,
	dryRun bool,
	outcome string,
	checkpointOffset *int64,
) error {
	_, err := tx.Exec(ctx, `
INSERT INTO delivery_projection_failure_resolution_audit (
    consumer_group,
    topic,
    partition_id,
    offset_value,
    event_id,
    failure_class,
    operator,
    reason,
    dry_run,
    outcome,
    checkpoint_offset_value,
    created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now())
`,
		failure.ConsumerGroup,
		failure.Topic,
		failure.PartitionID,
		failure.OffsetValue,
		failure.EventID,
		failure.FailureClass,
		operator,
		reason,
		dryRun,
		outcome,
		checkpointOffset,
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func normalizeProjectionFailureResolveText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func sanitizeProjectionFailureError(failureClass string, _ string) string {
	return types.ProjectionFailurePublicMessage(failureClass)
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
