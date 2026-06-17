package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

func TestProjectionRepairStoreAuditsCheckpointIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	seedDeliveryCheckpoint(t, ctx, pool, "group-1", "conversation.timeline.events", 0, 42)

	store := NewProjectionRepairStore(pool, WithProjectionRepairClock(func() time.Time {
		return time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	}))
	stats, err := store.RepairCheckpoint(ctx, types.ProjectionCheckpointRepairOptions{
		ConsumerGroup: "group-1",
		Topic:         "conversation.timeline.events",
		PartitionID:   0,
		Mode:          types.ProjectionCheckpointRepairModeAudit,
		Operator:      "operator-1",
		Reason:        "manual audit",
	})
	if err != nil {
		t.Fatalf("audit checkpoint: %v", err)
	}
	if stats.Requested != 1 || stats.Audited != 1 || stats.Mutated != 0 || stats.Skipped != 0 {
		t.Fatalf("unexpected checkpoint repair stats: %+v", stats)
	}
	assertCheckpointOffset(t, ctx, pool, "group-1", "conversation.timeline.events", 0, 42)
	assertCheckpointRepairAudit(t, ctx, pool, "group-1", "conversation.timeline.events", 0, types.ProjectionCheckpointRepairModeAudit, checkpointRepairOutcomeAudited, "", 42, 42, "manual audit", 0, "", "")
}

func TestProjectionRepairStoreRewindsCheckpointIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	seedDeliveryCheckpoint(t, ctx, pool, "group-1", "conversation.timeline.events", 0, 42)

	store := NewProjectionRepairStore(pool, WithProjectionRepairClock(func() time.Time {
		return time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	}))
	stats, err := store.RepairCheckpoint(ctx, types.ProjectionCheckpointRepairOptions{
		ConsumerGroup: "group-1",
		Topic:         "conversation.timeline.events",
		PartitionID:   0,
		TargetOffset:  21,
		Mode:          types.ProjectionCheckpointRepairModeRewindNextOffset,
		Operator:      "operator-1",
		Reason:        "replay after projection fix",
	})
	if err != nil {
		t.Fatalf("rewind checkpoint: %v", err)
	}
	if stats.Requested != 1 || stats.Audited != 0 || stats.Mutated != 1 || stats.Skipped != 0 {
		t.Fatalf("unexpected checkpoint repair stats: %+v", stats)
	}
	assertCheckpointOffset(t, ctx, pool, "group-1", "conversation.timeline.events", 0, 21)
	assertCheckpointRepairAudit(t, ctx, pool, "group-1", "conversation.timeline.events", 0, types.ProjectionCheckpointRepairModeRewindNextOffset, checkpointRepairOutcomeMutated, "", 42, 21, "replay after projection fix", 0, "", "")
}

func TestProjectionRepairStoreSkipsWhenTargetIsNotLowerIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	seedDeliveryCheckpoint(t, ctx, pool, "group-1", "conversation.timeline.events", 0, 42)

	store := NewProjectionRepairStore(pool)
	stats, err := store.RepairCheckpoint(ctx, types.ProjectionCheckpointRepairOptions{
		ConsumerGroup: "group-1",
		Topic:         "conversation.timeline.events",
		PartitionID:   0,
		TargetOffset:  42,
		Mode:          types.ProjectionCheckpointRepairModeRewindNextOffset,
		Operator:      "operator-1",
		Reason:        "noop repair",
	})
	if err != nil {
		t.Fatalf("skip checkpoint repair: %v", err)
	}
	if stats.Requested != 1 || stats.Audited != 0 || stats.Mutated != 0 || stats.Skipped != 1 {
		t.Fatalf("unexpected checkpoint repair stats: %+v", stats)
	}
	assertCheckpointOffset(t, ctx, pool, "group-1", "conversation.timeline.events", 0, 42)
	assertCheckpointRepairAudit(t, ctx, pool, "group-1", "conversation.timeline.events", 0, types.ProjectionCheckpointRepairModeRewindNextOffset, checkpointRepairOutcomeSkipped, checkpointRepairSkipTargetNotLower, 42, 42, "noop repair", 0, "", "")
}

func TestProjectionRepairStoreRewindsCheckpointFromUnresolvedFailureIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	seedDeliveryCheckpoint(t, ctx, pool, "group-1", "conversation.timeline.events", 0, 42)
	seedProjectionFailure(t, ctx, pool, "group-1", "conversation.timeline.events", 0, 21, "event-21", types.ProjectionFailureClassProjectionDependency, false)

	store := NewProjectionRepairStore(pool, WithProjectionRepairClock(func() time.Time {
		return time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	}))
	stats, err := store.RepairCheckpoint(ctx, types.ProjectionCheckpointRepairOptions{
		ConsumerGroup: "group-1",
		Topic:         "conversation.timeline.events",
		PartitionID:   0,
		FailureOffset: 21,
		Mode:          types.ProjectionCheckpointRepairModeRewindFailure,
		Operator:      "operator-1",
		Reason:        "replay unresolved projection failure",
	})
	if err != nil {
		t.Fatalf("rewind checkpoint from failure: %v", err)
	}
	if stats.Requested != 1 || stats.Audited != 0 || stats.Mutated != 1 || stats.Skipped != 0 {
		t.Fatalf("unexpected checkpoint repair stats: %+v", stats)
	}
	assertCheckpointOffset(t, ctx, pool, "group-1", "conversation.timeline.events", 0, 21)
	assertCheckpointRepairAudit(t, ctx, pool, "group-1", "conversation.timeline.events", 0, types.ProjectionCheckpointRepairModeRewindFailure, checkpointRepairOutcomeMutated, "", 42, 21, "replay unresolved projection failure", 21, "event-21", types.ProjectionFailureClassProjectionDependency)
}

func TestProjectionRepairStoreRejectsMissingUnresolvedFailureIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	seedDeliveryCheckpoint(t, ctx, pool, "group-1", "conversation.timeline.events", 0, 42)

	store := NewProjectionRepairStore(pool)
	_, err := store.RepairCheckpoint(ctx, types.ProjectionCheckpointRepairOptions{
		ConsumerGroup: "group-1",
		Topic:         "conversation.timeline.events",
		PartitionID:   0,
		FailureOffset: 21,
		Mode:          types.ProjectionCheckpointRepairModeRewindFailure,
		Operator:      "operator-1",
		Reason:        "missing unresolved failure",
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestProjectionRepairStoreRewindsCheckpointFromEarliestUnresolvedFailureIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	seedDeliveryCheckpoint(t, ctx, pool, "group-2", "conversation.timeline.events", 0, 42)
	seedProjectionFailure(t, ctx, pool, "group-2", "conversation.timeline.events", 0, 31, "event-31", types.ProjectionFailureClassDBWrite, false)
	seedProjectionFailure(t, ctx, pool, "group-2", "conversation.timeline.events", 0, 21, "event-21", types.ProjectionFailureClassProjectionDependency, false)
	seedProjectionFailure(t, ctx, pool, "group-2", "conversation.timeline.events", 0, 11, "event-11", types.ProjectionFailureClassDecode, true)

	store := NewProjectionRepairStore(pool, WithProjectionRepairClock(func() time.Time {
		return time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	}))
	stats, err := store.RepairCheckpoint(ctx, types.ProjectionCheckpointRepairOptions{
		ConsumerGroup: "group-2",
		Topic:         "conversation.timeline.events",
		PartitionID:   0,
		Mode:          types.ProjectionCheckpointRepairModeRewindEarliest,
		Operator:      "operator-1",
		Reason:        "replay earliest unresolved projection failure",
	})
	if err != nil {
		t.Fatalf("rewind checkpoint from earliest unresolved failure: %v", err)
	}
	if stats.Requested != 1 || stats.Audited != 0 || stats.Mutated != 1 || stats.Skipped != 0 {
		t.Fatalf("unexpected checkpoint repair stats: %+v", stats)
	}
	assertCheckpointOffset(t, ctx, pool, "group-2", "conversation.timeline.events", 0, 21)
	assertCheckpointRepairAudit(t, ctx, pool, "group-2", "conversation.timeline.events", 0, types.ProjectionCheckpointRepairModeRewindEarliest, checkpointRepairOutcomeMutated, "", 42, 21, "replay earliest unresolved projection failure", 21, "event-21", types.ProjectionFailureClassProjectionDependency)
}

func TestProjectionRepairStoreRejectsMissingEarliestUnresolvedFailureIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	seedDeliveryCheckpoint(t, ctx, pool, "group-3", "conversation.timeline.events", 0, 42)

	store := NewProjectionRepairStore(pool)
	_, err := store.RepairCheckpoint(ctx, types.ProjectionCheckpointRepairOptions{
		ConsumerGroup: "group-3",
		Topic:         "conversation.timeline.events",
		PartitionID:   0,
		Mode:          types.ProjectionCheckpointRepairModeRewindEarliest,
		Operator:      "operator-1",
		Reason:        "missing earliest unresolved failure",
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestProjectionRepairStoreAuditCheckpointRepairsReturnsLatestRowsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO delivery_projection_checkpoint_repair_audit (
    consumer_group, topic, partition_id, mode, outcome, skip_reason, operator, reason, dry_run, before_offset_value, after_offset_value, failure_offset_value, failure_event_id, failure_class, created_at
) VALUES
    ('group-a', 'conversation.timeline.events', 0, 'rewind-unresolved-failure', 'MUTATED', '', 'operator-a', 'repair-a', false, 42, 21, 21, 'event-21', 'projection_dependency', now() - interval '1 minute'),
    ('group-a', 'conversation.timeline.events', 0, 'audit', 'AUDITED', '', 'operator-b', 'repair-b', true, 42, 42, NULL, '', '', now())
`)
	if err != nil {
		t.Fatalf("seed checkpoint repair audit: %v", err)
	}

	store := NewProjectionRepairStore(pool)
	rows, err := store.AuditCheckpointRepairs(ctx, ProjectionRepairAuditOptions{
		ConsumerGroup: "group-a",
		Topic:         "conversation.timeline.events",
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("audit checkpoint repairs: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two rows, got %d", len(rows))
	}
	if rows[0].Mode != types.ProjectionCheckpointRepairModeAudit || rows[0].Outcome != checkpointRepairOutcomeAudited || !rows[0].DryRun {
		t.Fatalf("unexpected latest repair audit row: %+v", rows[0])
	}
	if rows[1].Mode != types.ProjectionCheckpointRepairModeRewindFailure || rows[1].Outcome != checkpointRepairOutcomeMutated || rows[1].FailureOffset == nil || *rows[1].FailureOffset != 21 {
		t.Fatalf("unexpected older repair audit row: %+v", rows[1])
	}
}

func TestProjectionRepairStoreAuditCheckpointRepairsFiltersModeAndOutcomeIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO delivery_projection_checkpoint_repair_audit (
    consumer_group, topic, partition_id, mode, outcome, skip_reason, operator, reason, dry_run, before_offset_value, after_offset_value, failure_offset_value, failure_event_id, failure_class, created_at
) VALUES
    ('group-b', 'conversation.timeline.events', 0, 'rewind-earliest-unresolved-failure', 'MUTATED', '', 'operator-a', 'repair-a', false, 42, 21, 21, 'event-21', 'projection_dependency', now()),
    ('group-b', 'conversation.timeline.events', 0, 'rewind-next-offset', 'SKIPPED', 'target_offset_is_not_lower', 'operator-b', 'repair-b', false, 42, 42, NULL, '', '', now() - interval '1 minute')
`)
	if err != nil {
		t.Fatalf("seed checkpoint repair audit: %v", err)
	}

	store := NewProjectionRepairStore(pool)
	rows, err := store.AuditCheckpointRepairs(ctx, ProjectionRepairAuditOptions{
		ConsumerGroup: "group-b",
		Topic:         "conversation.timeline.events",
		Mode:          types.ProjectionCheckpointRepairModeRewindEarliest,
		Outcome:       checkpointRepairOutcomeMutated,
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("audit checkpoint repairs by mode/outcome: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if rows[0].Mode != types.ProjectionCheckpointRepairModeRewindEarliest || rows[0].Outcome != checkpointRepairOutcomeMutated {
		t.Fatalf("unexpected filtered repair audit row: %+v", rows[0])
	}
}

func TestProjectionRepairStoreCleanupCheckpointRepairsDeletesOnlyExpiredRowsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO delivery_projection_checkpoint_repair_audit (
    consumer_group, topic, partition_id, mode, outcome, skip_reason, operator, reason, dry_run, before_offset_value, after_offset_value, failure_offset_value, failure_event_id, failure_class, created_at
) VALUES
    ('group-c', 'conversation.timeline.events', 0, 'audit', 'AUDITED', '', 'operator-a', 'repair-a', true, 42, 42, NULL, '', '', now() - interval '10 days'),
    ('group-c', 'conversation.timeline.events', 0, 'rewind-next-offset', 'MUTATED', '', 'operator-b', 'repair-b', false, 42, 21, NULL, '', '', now() - interval '1 day')
`)
	if err != nil {
		t.Fatalf("seed checkpoint repair audit: %v", err)
	}

	store := NewProjectionRepairStore(pool)
	stats, err := store.CleanupCheckpointRepairs(ctx, ProjectionRepairCleanupOptions{
		ConsumerGroup: "group-c",
		Topic:         "conversation.timeline.events",
		Cutoff:        time.Now().UTC().Add(-7 * 24 * time.Hour),
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("cleanup checkpoint repairs: %v", err)
	}
	if stats.Deleted != 1 {
		t.Fatalf("unexpected deleted count: %+v", stats)
	}
	assertCheckpointRepairAuditCount(t, ctx, pool, "group-c", "conversation.timeline.events", 0, 1)
}

func TestProjectionRepairStoreCleanupCheckpointRepairsDryRunDoesNotDeleteIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO delivery_projection_checkpoint_repair_audit (
    consumer_group, topic, partition_id, mode, outcome, skip_reason, operator, reason, dry_run, before_offset_value, after_offset_value, failure_offset_value, failure_event_id, failure_class, created_at
) VALUES
    ('group-cleanup-dry-run', 'conversation.timeline.events', 0, 'audit', 'AUDITED', '', 'operator-a', 'repair-a', true, 42, 42, NULL, '', '', now() - interval '10 days'),
    ('group-cleanup-dry-run', 'conversation.timeline.events', 0, 'rewind-next-offset', 'MUTATED', '', 'operator-b', 'repair-b', false, 42, 21, NULL, '', '', now() - interval '9 days'),
    ('group-cleanup-dry-run', 'conversation.timeline.events', 0, 'audit', 'AUDITED', '', 'operator-a', 'recent repair', true, 42, 42, NULL, '', '', now() - interval '1 day')
`)
	if err != nil {
		t.Fatalf("seed checkpoint repair dry-run audit: %v", err)
	}

	store := NewProjectionRepairStore(pool)
	stats, err := store.CleanupCheckpointRepairs(ctx, ProjectionRepairCleanupOptions{
		ConsumerGroup: "group-cleanup-dry-run",
		Topic:         "conversation.timeline.events",
		Cutoff:        time.Now().UTC().Add(-7 * 24 * time.Hour),
		Limit:         10,
		DryRun:        true,
	})
	if err != nil {
		t.Fatalf("dry-run cleanup checkpoint repairs: %v", err)
	}
	if stats.Deleted != 2 {
		t.Fatalf("unexpected dry-run deleted count: %+v", stats)
	}
	assertCheckpointRepairAuditCount(t, ctx, pool, "group-cleanup-dry-run", "conversation.timeline.events", 0, 3)
}

func TestProjectionRepairStoreCleanupCheckpointRepairsHonorsBatchLimitIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO delivery_projection_checkpoint_repair_audit (
    consumer_group, topic, partition_id, mode, outcome, skip_reason, operator, reason, dry_run, before_offset_value, after_offset_value, failure_offset_value, failure_event_id, failure_class, created_at
) VALUES
    ('group-d', 'conversation.timeline.events', 0, 'audit', 'AUDITED', '', 'operator-a', 'repair-a', true, 42, 42, NULL, '', '', now() - interval '10 days'),
    ('group-d', 'conversation.timeline.events', 0, 'rewind-next-offset', 'MUTATED', '', 'operator-b', 'repair-b', false, 42, 21, NULL, '', '', now() - interval '9 days')
`)
	if err != nil {
		t.Fatalf("seed checkpoint repair audit: %v", err)
	}

	store := NewProjectionRepairStore(pool)
	stats, err := store.CleanupCheckpointRepairs(ctx, ProjectionRepairCleanupOptions{
		ConsumerGroup: "group-d",
		Topic:         "conversation.timeline.events",
		Cutoff:        time.Now().UTC().Add(-7 * 24 * time.Hour),
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("cleanup checkpoint repairs: %v", err)
	}
	if stats.Deleted != 1 {
		t.Fatalf("unexpected deleted count: %+v", stats)
	}
	assertCheckpointRepairAuditCount(t, ctx, pool, "group-d", "conversation.timeline.events", 0, 1)
}

func TestProjectionRepairStoreCleanupCheckpointRepairsFiltersModeAndOutcomeIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO delivery_projection_checkpoint_repair_audit (
    consumer_group, topic, partition_id, mode, outcome, skip_reason, operator, reason, dry_run, before_offset_value, after_offset_value, failure_offset_value, failure_event_id, failure_class, created_at
) VALUES
    ('group-e', 'conversation.timeline.events', 0, 'audit', 'AUDITED', '', 'operator-a', 'repair-a', true, 42, 42, NULL, '', '', now() - interval '10 days'),
    ('group-e', 'conversation.timeline.events', 0, 'rewind-next-offset', 'MUTATED', '', 'operator-b', 'repair-b', false, 42, 21, NULL, '', '', now() - interval '10 days')
`)
	if err != nil {
		t.Fatalf("seed checkpoint repair audit: %v", err)
	}

	store := NewProjectionRepairStore(pool)
	stats, err := store.CleanupCheckpointRepairs(ctx, ProjectionRepairCleanupOptions{
		ConsumerGroup: "group-e",
		Topic:         "conversation.timeline.events",
		Mode:          types.ProjectionCheckpointRepairModeAudit,
		Outcome:       checkpointRepairOutcomeAudited,
		Cutoff:        time.Now().UTC().Add(-7 * 24 * time.Hour),
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("cleanup checkpoint repairs with filters: %v", err)
	}
	if stats.Deleted != 1 {
		t.Fatalf("unexpected deleted count: %+v", stats)
	}

	rows, err := store.AuditCheckpointRepairs(ctx, ProjectionRepairAuditOptions{
		ConsumerGroup: "group-e",
		Topic:         "conversation.timeline.events",
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("audit checkpoint repairs after cleanup: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row after cleanup, got %d", len(rows))
	}
	if rows[0].Mode != types.ProjectionCheckpointRepairModeRewindNextOffset || rows[0].Outcome != checkpointRepairOutcomeMutated {
		t.Fatalf("unexpected remaining repair audit row: %+v", rows[0])
	}
}

func seedDeliveryCheckpoint(t *testing.T, ctx context.Context, pool *pgxpool.Pool, consumerGroup string, topic string, partitionID int32, offsetValue int64) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
INSERT INTO delivery_kafka_checkpoints (
    consumer_group, topic, partition_id, offset_value
) VALUES ($1, $2, $3, $4)
`, consumerGroup, topic, partitionID, offsetValue); err != nil {
		t.Fatalf("seed delivery checkpoint: %v", err)
	}
}

func seedProjectionFailure(t *testing.T, ctx context.Context, pool *pgxpool.Pool, consumerGroup string, topic string, partitionID int32, offsetValue int64, eventID string, failureClass string, resolved bool) {
	t.Helper()
	var resolvedAt any
	var resolvedCheckpointOffset any
	if resolved {
		resolvedAt = time.Date(2026, 6, 14, 11, 0, 0, 0, time.UTC)
		resolvedCheckpointOffset = offsetValue + 1
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO delivery_projection_failures (
    consumer_group, topic, partition_id, offset_value, event_id, event_type, tenant_id, conversation_id, aggregate_version, trace_id, failure_class, last_error, failure_count, resolved_at, resolved_checkpoint_offset
) VALUES ($1, $2, $3, $4, $5, 'message.revoked.v1', 'tenant-1', 'conv-1', 7, 'trace-1', $6, 'projection failure', 1, $7, $8)
`, consumerGroup, topic, partitionID, offsetValue, eventID, failureClass, resolvedAt, resolvedCheckpointOffset); err != nil {
		t.Fatalf("seed projection failure: %v", err)
	}
}

func assertCheckpointOffset(t *testing.T, ctx context.Context, pool *pgxpool.Pool, consumerGroup string, topic string, partitionID int32, wantOffset int64) {
	t.Helper()
	var gotOffset int64
	if err := pool.QueryRow(ctx, `
SELECT offset_value
FROM delivery_kafka_checkpoints
WHERE consumer_group = $1
  AND topic = $2
  AND partition_id = $3
`, consumerGroup, topic, partitionID).Scan(&gotOffset); err != nil {
		t.Fatalf("read delivery checkpoint: %v", err)
	}
	if gotOffset != wantOffset {
		t.Fatalf("unexpected delivery checkpoint offset: got=%d want=%d", gotOffset, wantOffset)
	}
}

func assertCheckpointRepairAudit(t *testing.T, ctx context.Context, pool *pgxpool.Pool, consumerGroup string, topic string, partitionID int32, mode string, outcome string, skipReason string, beforeOffset int64, afterOffset int64, reason string, failureOffset int64, failureEventID string, failureClass string) {
	t.Helper()
	var gotMode string
	var gotOutcome string
	var gotSkipReason string
	var gotBeforeOffset int64
	var gotAfterOffset int64
	var gotReason string
	var gotFailureOffset *int64
	var gotFailureEventID string
	var gotFailureClass string
	if err := pool.QueryRow(ctx, `
SELECT
    mode,
    outcome,
    skip_reason,
    before_offset_value,
    after_offset_value,
    reason,
    failure_offset_value,
    failure_event_id,
    failure_class
FROM delivery_projection_checkpoint_repair_audit
WHERE consumer_group = $1
  AND topic = $2
  AND partition_id = $3
ORDER BY id DESC
LIMIT 1
	`, consumerGroup, topic, partitionID).Scan(
		&gotMode,
		&gotOutcome,
		&gotSkipReason,
		&gotBeforeOffset,
		&gotAfterOffset,
		&gotReason,
		&gotFailureOffset,
		&gotFailureEventID,
		&gotFailureClass,
	); err != nil {
		t.Fatalf("read checkpoint repair audit: %v", err)
	}
	if gotMode != mode || gotOutcome != outcome || gotSkipReason != skipReason || gotBeforeOffset != beforeOffset || gotAfterOffset != afterOffset || gotReason != reason {
		t.Fatalf("unexpected checkpoint repair audit row: mode=%s outcome=%s skip=%s before=%d after=%d reason=%s", gotMode, gotOutcome, gotSkipReason, gotBeforeOffset, gotAfterOffset, gotReason)
	}
	if failureEventID == "" && failureClass == "" {
		if gotFailureOffset != nil || gotFailureEventID != "" || gotFailureClass != "" {
			t.Fatalf("expected empty failure audit fields, got offset=%v event=%s class=%s", gotFailureOffset, gotFailureEventID, gotFailureClass)
		}
		return
	}
	if gotFailureOffset == nil || *gotFailureOffset != failureOffset || gotFailureEventID != failureEventID || gotFailureClass != failureClass {
		t.Fatalf("unexpected failure audit fields: offset=%v event=%s class=%s", gotFailureOffset, gotFailureEventID, gotFailureClass)
	}
}

func assertCheckpointRepairAuditCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, consumerGroup string, topic string, partitionID int32, wantCount int64) {
	t.Helper()
	var gotCount int64
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM delivery_projection_checkpoint_repair_audit
WHERE consumer_group = $1
  AND topic = $2
  AND partition_id = $3
`, consumerGroup, topic, partitionID).Scan(&gotCount); err != nil {
		t.Fatalf("count checkpoint repair audit rows: %v", err)
	}
	if gotCount != wantCount {
		t.Fatalf("unexpected checkpoint repair audit count: got=%d want=%d", gotCount, wantCount)
	}
}
