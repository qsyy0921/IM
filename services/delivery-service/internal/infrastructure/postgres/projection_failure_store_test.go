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

func TestProjectionFailureStoreAuditReturnsUnresolvedRowsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO delivery_projection_failures (
    consumer_group, topic, partition_id, offset_value, event_id, event_type, tenant_id, conversation_id, aggregate_version, trace_id, failure_class, last_error, failure_count, first_seen_at, last_seen_at, resolved_at, resolved_checkpoint_offset
) VALUES
    ('group-1', 'conversation.timeline.events', 0, 41, 'event-1', 'message.revoked.v1', 'tenant-1', 'conv-1', 7, 'trace-1', 'projection_dependency', 'message revoke has no projected original message', 2, now(), now() - interval '1 minute', NULL, NULL),
    ('group-1', 'conversation.timeline.events', 1, 42, 'event-2', 'message.edited.v1', 'tenant-1', 'conv-1', 8, 'trace-2', 'db_write_failed', 'write timeout', 1, now(), now(), now(), 43)
`)
	if err != nil {
		t.Fatalf("seed projection failures: %v", err)
	}

	store := NewProjectionFailureStore(pool)
	rows, err := store.AuditFailures(ctx, ProjectionFailureAuditOptions{
		Topic:          "conversation.timeline.events",
		UnresolvedOnly: true,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("audit projection failures: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one unresolved row, got %d", len(rows))
	}
	if rows[0].OffsetValue != 41 || rows[0].ResolvedAt != nil || rows[0].FailureClass != types.ProjectionFailureClassProjectionDependency {
		t.Fatalf("unexpected unresolved row: %+v", rows[0])
	}
}

func TestProjectionFailureStoreAuditIncludesResolvedWhenRequestedIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO delivery_projection_failures (
    consumer_group, topic, partition_id, offset_value, event_id, event_type, tenant_id, conversation_id, aggregate_version, trace_id, failure_class, last_error, failure_count, first_seen_at, last_seen_at, resolved_at, resolved_checkpoint_offset
) VALUES
    ('group-2', 'conversation.timeline.events', 0, 51, 'event-3', 'message.deleted.v1', 'tenant-1', 'conv-1', 9, 'trace-3', 'decode_failed', 'decode failed', 1, now(), now(), NULL, NULL),
    ('group-2', 'conversation.timeline.events', 0, 52, 'event-4', 'message.edited.v1', 'tenant-1', 'conv-1', 10, 'trace-4', 'db_write_failed', 'write timeout', 3, now(), now() + interval '1 second', now(), 53)
`)
	if err != nil {
		t.Fatalf("seed projection failures: %v", err)
	}

	store := NewProjectionFailureStore(pool)
	rows, err := store.AuditFailures(ctx, ProjectionFailureAuditOptions{
		ConsumerGroup:  "group-2",
		Topic:          "conversation.timeline.events",
		UnresolvedOnly: false,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("audit projection failures with resolved rows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two rows, got %d", len(rows))
	}
	if rows[0].OffsetValue != 52 || rows[0].ResolvedAt == nil {
		t.Fatalf("expected resolved row first by last_seen_at, got %+v", rows[0])
	}
	if rows[1].OffsetValue != 51 || rows[1].ResolvedAt != nil {
		t.Fatalf("expected unresolved row second, got %+v", rows[1])
	}
}

func TestProjectionFailureStoreAuditFiltersByFailureClassIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO delivery_projection_failures (
    consumer_group, topic, partition_id, offset_value, event_id, event_type, tenant_id, conversation_id, aggregate_version, trace_id, failure_class, last_error, failure_count, first_seen_at, last_seen_at, resolved_at, resolved_checkpoint_offset
) VALUES
    ('group-3', 'conversation.timeline.events', 0, 61, 'event-1', 'message.edited.v1', 'tenant-1', 'conv-1', 7, 'trace-1', 'db_write_failed', 'write failed', 1, now(), now(), NULL, NULL),
    ('group-3', 'conversation.timeline.events', 0, 62, 'event-2', 'message.deleted.v1', 'tenant-1', 'conv-1', 8, 'trace-2', 'decode_failed', 'decode failed', 1, now(), now(), NULL, NULL)
`)
	if err != nil {
		t.Fatalf("seed projection failures: %v", err)
	}

	store := NewProjectionFailureStore(pool)
	rows, err := store.AuditFailures(ctx, ProjectionFailureAuditOptions{
		ConsumerGroup:  "group-3",
		Topic:          "conversation.timeline.events",
		FailureClass:   types.ProjectionFailureClassDBWrite,
		UnresolvedOnly: true,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("audit projection failures by class: %v", err)
	}
	if len(rows) != 1 || rows[0].OffsetValue != 61 || rows[0].FailureClass != types.ProjectionFailureClassDBWrite {
		t.Fatalf("unexpected filtered audit rows: %+v", rows)
	}
}

func TestProjectionFailureStoreAuditFiltersByOffsetIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO delivery_projection_failures (
    consumer_group, topic, partition_id, offset_value, event_id, event_type, tenant_id, conversation_id, aggregate_version, trace_id, failure_class, last_error, failure_count, first_seen_at, last_seen_at, resolved_at, resolved_checkpoint_offset
) VALUES
    ('group-5', 'conversation.timeline.events', 0, 81, 'event-1', 'message.edited.v1', 'tenant-1', 'conv-1', 7, 'trace-1', 'db_write_failed', 'write failed', 1, now(), now(), NULL, NULL),
    ('group-5', 'conversation.timeline.events', 0, 82, 'event-2', 'message.deleted.v1', 'tenant-1', 'conv-1', 8, 'trace-2', 'decode_failed', 'decode failed', 1, now(), now(), NULL, NULL)
`)
	if err != nil {
		t.Fatalf("seed projection failures: %v", err)
	}

	targetOffset := int64(82)
	store := NewProjectionFailureStore(pool)
	rows, err := store.AuditFailures(ctx, ProjectionFailureAuditOptions{
		ConsumerGroup:  "group-5",
		Topic:          "conversation.timeline.events",
		OffsetValue:    &targetOffset,
		UnresolvedOnly: true,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("audit projection failures by offset: %v", err)
	}
	if len(rows) != 1 || rows[0].OffsetValue != 82 || rows[0].EventID != "event-2" {
		t.Fatalf("unexpected filtered audit rows: %+v", rows)
	}
}

func TestProjectionFailureStoreAuditFiltersByEventIDAndTypeIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO delivery_projection_failures (
    consumer_group, topic, partition_id, offset_value, event_id, event_type, tenant_id, conversation_id, aggregate_version, trace_id, failure_class, last_error, failure_count, first_seen_at, last_seen_at, resolved_at, resolved_checkpoint_offset
) VALUES
    ('group-6', 'conversation.timeline.events', 0, 91, 'event-a', 'message.edited.v1', 'tenant-1', 'conv-1', 7, 'trace-1', 'db_write_failed', 'write failed', 1, now(), now(), NULL, NULL),
    ('group-6', 'conversation.timeline.events', 0, 92, 'event-b', 'message.deleted.v1', 'tenant-1', 'conv-1', 8, 'trace-2', 'decode_failed', 'decode failed', 1, now(), now(), NULL, NULL)
`)
	if err != nil {
		t.Fatalf("seed projection failures: %v", err)
	}

	store := NewProjectionFailureStore(pool)
	rows, err := store.AuditFailures(ctx, ProjectionFailureAuditOptions{
		ConsumerGroup:  "group-6",
		Topic:          "conversation.timeline.events",
		EventID:        "event-a",
		EventType:      types.TimelineEventMessageEdited,
		UnresolvedOnly: true,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("audit projection failures by event id and type: %v", err)
	}
	if len(rows) != 1 || rows[0].OffsetValue != 91 || rows[0].EventID != "event-a" || rows[0].EventType != types.TimelineEventMessageEdited {
		t.Fatalf("unexpected filtered audit rows: %+v", rows)
	}
}

func TestProjectionFailureStoreCleanupResolvedFailuresDeletesOnlyExpiredResolvedRowsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO delivery_projection_failures (
    consumer_group, topic, partition_id, offset_value, event_id, event_type, tenant_id, conversation_id, aggregate_version, trace_id, failure_class, last_error, failure_count, first_seen_at, last_seen_at, resolved_at, resolved_checkpoint_offset
) VALUES
    ('group-1', 'conversation.timeline.events', 0, 41, 'event-1', 'message.revoked.v1', 'tenant-1', 'conv-1', 7, 'trace-1', 'projection_dependency', 'old resolved', 2, now(), now(), now() - interval '2 days', 42),
    ('group-1', 'conversation.timeline.events', 0, 42, 'event-2', 'message.edited.v1', 'tenant-1', 'conv-1', 8, 'trace-2', 'db_write_failed', 'recent resolved', 1, now(), now(), now() - interval '1 hour', 43),
    ('group-1', 'conversation.timeline.events', 0, 43, 'event-3', 'message.deleted.v1', 'tenant-1', 'conv-1', 9, 'trace-3', 'decode_failed', 'still unresolved', 1, now(), now(), NULL, NULL)
`)
	if err != nil {
		t.Fatalf("seed projection failures: %v", err)
	}

	store := NewProjectionFailureStore(pool)
	stats, err := store.CleanupResolvedFailures(ctx, ProjectionFailureCleanupOptions{
		Topic:  "conversation.timeline.events",
		Cutoff: time.Now().UTC().Add(-24 * time.Hour),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("cleanup resolved projection failures: %v", err)
	}
	if stats.Deleted != 1 {
		t.Fatalf("expected one deleted row, got %d", stats.Deleted)
	}

	assertProjectionFailureMissing(t, ctx, pool, "group-1", "conversation.timeline.events", 0, 41)
	assertProjectionFailureRow(t, ctx, pool, "group-1", "conversation.timeline.events", 0, 42, types.ProjectionFailureClassDBWrite, "recent resolved", 1, true, 43)
	assertProjectionFailureRow(t, ctx, pool, "group-1", "conversation.timeline.events", 0, 43, types.ProjectionFailureClassDecode, "still unresolved", 1, false, 0)
}

func TestProjectionFailureStoreCleanupResolvedFailuresHonorsBatchLimitIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO delivery_projection_failures (
    consumer_group, topic, partition_id, offset_value, event_id, event_type, tenant_id, conversation_id, aggregate_version, trace_id, failure_class, last_error, failure_count, first_seen_at, last_seen_at, resolved_at, resolved_checkpoint_offset
) VALUES
    ('group-2', 'conversation.timeline.events', 0, 51, 'event-1', 'message.revoked.v1', 'tenant-1', 'conv-1', 7, 'trace-1', 'projection_dependency', 'resolved 1', 2, now(), now(), now() - interval '3 days', 52),
    ('group-2', 'conversation.timeline.events', 0, 52, 'event-2', 'message.edited.v1', 'tenant-1', 'conv-1', 8, 'trace-2', 'db_write_failed', 'resolved 2', 1, now(), now(), now() - interval '2 days', 53)
`)
	if err != nil {
		t.Fatalf("seed projection failures: %v", err)
	}

	store := NewProjectionFailureStore(pool)
	stats, err := store.CleanupResolvedFailures(ctx, ProjectionFailureCleanupOptions{
		Topic:  "conversation.timeline.events",
		Cutoff: time.Now().UTC().Add(-24 * time.Hour),
		Limit:  1,
	})
	if err != nil {
		t.Fatalf("cleanup resolved projection failures with limit: %v", err)
	}
	if stats.Deleted != 1 {
		t.Fatalf("expected one deleted row, got %d", stats.Deleted)
	}

	var remaining int
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM delivery_projection_failures
WHERE consumer_group = 'group-2'
`).Scan(&remaining); err != nil {
		t.Fatalf("count remaining projection failures: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("expected one remaining row after limited cleanup, got %d", remaining)
	}
}

func TestProjectionFailureStoreCleanupResolvedFailuresFiltersByFailureClassIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO delivery_projection_failures (
    consumer_group, topic, partition_id, offset_value, event_id, event_type, tenant_id, conversation_id, aggregate_version, trace_id, failure_class, last_error, failure_count, first_seen_at, last_seen_at, resolved_at, resolved_checkpoint_offset
) VALUES
    ('group-4', 'conversation.timeline.events', 0, 71, 'event-1', 'message.edited.v1', 'tenant-1', 'conv-1', 7, 'trace-1', 'db_write_failed', 'resolved db write', 1, now(), now(), now() - interval '2 days', 72),
    ('group-4', 'conversation.timeline.events', 0, 72, 'event-2', 'message.deleted.v1', 'tenant-1', 'conv-1', 8, 'trace-2', 'decode_failed', 'resolved decode', 1, now(), now(), now() - interval '2 days', 73)
`)
	if err != nil {
		t.Fatalf("seed projection failures: %v", err)
	}

	store := NewProjectionFailureStore(pool)
	stats, err := store.CleanupResolvedFailures(ctx, ProjectionFailureCleanupOptions{
		ConsumerGroup: "group-4",
		Topic:         "conversation.timeline.events",
		FailureClass:  types.ProjectionFailureClassDBWrite,
		Cutoff:        time.Now().UTC().Add(-24 * time.Hour),
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("cleanup resolved projection failures by class: %v", err)
	}
	if stats.Deleted != 1 {
		t.Fatalf("expected one deleted row, got %d", stats.Deleted)
	}

	assertProjectionFailureMissing(t, ctx, pool, "group-4", "conversation.timeline.events", 0, 71)
	assertProjectionFailureRow(t, ctx, pool, "group-4", "conversation.timeline.events", 0, 72, types.ProjectionFailureClassDecode, "resolved decode", 1, true, 73)
}

func assertProjectionFailureMissing(t *testing.T, ctx context.Context, pool *pgxpool.Pool, consumerGroup string, topic string, partitionID int32, offsetValue int64) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM delivery_projection_failures
WHERE consumer_group = $1
  AND topic = $2
  AND partition_id = $3
  AND offset_value = $4
`, consumerGroup, topic, partitionID, offsetValue).Scan(&count); err != nil {
		t.Fatalf("count projection failure row: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected projection failure row to be deleted, count=%d", count)
	}
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
