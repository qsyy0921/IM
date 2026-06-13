package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

func TestProjectionFailureStoreRecordsAndIncrementsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	store := NewProjectionFailureStore(pool)
	record := types.ProjectionFailureRecord{
		ConsumerGroup:    "group-1",
		Topic:            "conversation.timeline.events",
		PartitionID:      0,
		OffsetValue:      41,
		EventID:          "event-1",
		EventType:        types.TimelineEventMessageRevoked,
		TenantID:         "tenant-1",
		ConversationID:   "conv-1",
		AggregateVersion: 7,
		TraceID:          "trace-1",
		FailureClass:     types.ProjectionFailureClassProjectionDependency,
		LastError:        "message revoke has no projected original message",
	}
	if err := store.RecordFailure(ctx, record); err != nil {
		t.Fatalf("record projection failure: %v", err)
	}
	if err := store.RecordFailure(ctx, record); err != nil {
		t.Fatalf("record projection failure second time: %v", err)
	}
	assertProjectionFailureRow(t, ctx, pool, record.ConsumerGroup, record.Topic, record.PartitionID, record.OffsetValue, record.FailureClass, record.LastError, 2, false, 0)
}

func TestProjectionFailureStoreReopensResolvedFailureIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO delivery_projection_failures (
    consumer_group, topic, partition_id, offset_value, event_id, event_type, tenant_id, conversation_id, aggregate_version, trace_id, failure_class, last_error, failure_count, resolved_at, resolved_checkpoint_offset
) VALUES (
    'group-1', 'conversation.timeline.events', 0, 41, 'event-1', 'message.revoked.v1', 'tenant-1', 'conv-1', 7, 'trace-1', 'projection_dependency', 'message revoke has no projected original message', 2, now(), 42
)
`)
	if err != nil {
		t.Fatalf("seed resolved projection failure: %v", err)
	}

	store := NewProjectionFailureStore(pool)
	record := types.ProjectionFailureRecord{
		ConsumerGroup:    "group-1",
		Topic:            "conversation.timeline.events",
		PartitionID:      0,
		OffsetValue:      41,
		EventID:          "event-1",
		EventType:        types.TimelineEventMessageRevoked,
		TenantID:         "tenant-1",
		ConversationID:   "conv-1",
		AggregateVersion: 7,
		TraceID:          "trace-1",
		FailureClass:     types.ProjectionFailureClassProjectionDependency,
		LastError:        "message revoke has no projected original message",
	}
	if err := store.RecordFailure(ctx, record); err != nil {
		t.Fatalf("re-record resolved projection failure: %v", err)
	}
	assertProjectionFailureRow(t, ctx, pool, record.ConsumerGroup, record.Topic, record.PartitionID, record.OffsetValue, record.FailureClass, record.LastError, 3, false, 0)
}

func assertProjectionFailureRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, consumerGroup string, topic string, partitionID int32, offsetValue int64, failureClass string, lastError string, failureCount int64, resolved bool, resolvedCheckpointOffset int64) {
	t.Helper()
	var gotFailureClass string
	var gotLastError string
	var gotFailureCount int64
	var gotResolvedAt *time.Time
	var gotResolvedCheckpointOffset *int64
	if err := pool.QueryRow(ctx, `
SELECT failure_class, last_error, failure_count, resolved_at, resolved_checkpoint_offset
FROM delivery_projection_failures
WHERE consumer_group = $1
  AND topic = $2
  AND partition_id = $3
  AND offset_value = $4
`, consumerGroup, topic, partitionID, offsetValue).Scan(&gotFailureClass, &gotLastError, &gotFailureCount, &gotResolvedAt, &gotResolvedCheckpointOffset); err != nil {
		t.Fatalf("read projection failure row: %v", err)
	}
	if gotFailureClass != failureClass || gotLastError != lastError || gotFailureCount != failureCount {
		t.Fatalf("unexpected projection failure row: class=%s error=%s count=%d", gotFailureClass, gotLastError, gotFailureCount)
	}
	if resolved && gotResolvedAt == nil {
		t.Fatalf("expected resolved projection failure row")
	}
	if !resolved && gotResolvedAt != nil {
		t.Fatalf("expected unresolved projection failure row")
	}
	if resolved {
		if gotResolvedCheckpointOffset == nil || *gotResolvedCheckpointOffset != resolvedCheckpointOffset {
			t.Fatalf("unexpected resolved checkpoint offset: %v", gotResolvedCheckpointOffset)
		}
	} else if gotResolvedCheckpointOffset != nil {
		t.Fatalf("expected nil resolved checkpoint offset, got %v", gotResolvedCheckpointOffset)
	}
}
