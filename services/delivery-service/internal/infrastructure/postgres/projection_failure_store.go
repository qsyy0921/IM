package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

type ProjectionFailureStore struct {
	pool *pgxpool.Pool
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
    failure_count
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, now(), now(), 1
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
    failure_count = delivery_projection_failures.failure_count + 1
`, record.ConsumerGroup, record.Topic, record.PartitionID, record.OffsetValue, record.EventID, record.EventType, record.TenantID, record.ConversationID, record.AggregateVersion, record.TraceID, record.FailureClass, record.LastError)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func sanitizeProjectionFailureError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 256 {
		return value
	}
	return value[:256]
}
