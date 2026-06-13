package postgres

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

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
	record.LastError = sanitizeProjectionFailureError(record.LastError)
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
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return result, nil
}

func (store *ProjectionFailureStore) CleanupResolvedFailures(ctx context.Context, cutoff time.Time, limit int) (ProjectionFailureCleanupStats, error) {
	if store == nil || store.pool == nil {
		return ProjectionFailureCleanupStats{}, errors.New("delivery projection failure store is not configured")
	}
	if limit <= 0 {
		return ProjectionFailureCleanupStats{}, nil
	}
	rows, err := store.pool.Query(ctx, `
WITH doomed AS (
    SELECT consumer_group, topic, partition_id, offset_value
    FROM delivery_projection_failures
    WHERE resolved_at IS NOT NULL
      AND resolved_at < $1
    ORDER BY resolved_at ASC, consumer_group, topic, partition_id, offset_value
    LIMIT $2
    FOR UPDATE SKIP LOCKED
)
DELETE FROM delivery_projection_failures target
USING doomed
WHERE target.consumer_group = doomed.consumer_group
  AND target.topic = doomed.topic
  AND target.partition_id = doomed.partition_id
  AND target.offset_value = doomed.offset_value
RETURNING 1
`, cutoff, limit)
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

func sanitizeProjectionFailureError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 256 {
		return value
	}
	return value[:256]
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
