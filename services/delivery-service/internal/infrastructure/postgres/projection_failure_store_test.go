package postgres

import (
	"context"
	"testing"

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
	assertProjectionFailureRow(t, ctx, pool, record.ConsumerGroup, record.Topic, record.PartitionID, record.OffsetValue, record.FailureClass, record.LastError, 2)
}

func assertProjectionFailureRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, consumerGroup string, topic string, partitionID int32, offsetValue int64, failureClass string, lastError string, failureCount int64) {
	t.Helper()
	var gotFailureClass string
	var gotLastError string
	var gotFailureCount int64
	if err := pool.QueryRow(ctx, `
SELECT failure_class, last_error, failure_count
FROM delivery_projection_failures
WHERE consumer_group = $1
  AND topic = $2
  AND partition_id = $3
  AND offset_value = $4
`, consumerGroup, topic, partitionID, offsetValue).Scan(&gotFailureClass, &gotLastError, &gotFailureCount); err != nil {
		t.Fatalf("read projection failure row: %v", err)
	}
	if gotFailureClass != failureClass || gotLastError != lastError || gotFailureCount != failureCount {
		t.Fatalf("unexpected projection failure row: class=%s error=%s count=%d", gotFailureClass, gotLastError, gotFailureCount)
	}
}
