package postgres

import (
	"context"
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
	assertCheckpointRepairAudit(t, ctx, pool, "group-1", "conversation.timeline.events", 0, types.ProjectionCheckpointRepairModeAudit, checkpointRepairOutcomeAudited, "", 42, 42, "manual audit")
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
	assertCheckpointRepairAudit(t, ctx, pool, "group-1", "conversation.timeline.events", 0, types.ProjectionCheckpointRepairModeRewindNextOffset, checkpointRepairOutcomeMutated, "", 42, 21, "replay after projection fix")
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
	assertCheckpointRepairAudit(t, ctx, pool, "group-1", "conversation.timeline.events", 0, types.ProjectionCheckpointRepairModeRewindNextOffset, checkpointRepairOutcomeSkipped, checkpointRepairSkipTargetNotLower, 42, 42, "noop repair")
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

func assertCheckpointRepairAudit(t *testing.T, ctx context.Context, pool *pgxpool.Pool, consumerGroup string, topic string, partitionID int32, mode string, outcome string, skipReason string, beforeOffset int64, afterOffset int64, reason string) {
	t.Helper()
	var gotMode string
	var gotOutcome string
	var gotSkipReason string
	var gotBeforeOffset int64
	var gotAfterOffset int64
	var gotReason string
	if err := pool.QueryRow(ctx, `
SELECT
    mode,
    outcome,
    skip_reason,
    before_offset_value,
    after_offset_value,
    reason
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
	); err != nil {
		t.Fatalf("read checkpoint repair audit: %v", err)
	}
	if gotMode != mode || gotOutcome != outcome || gotSkipReason != skipReason || gotBeforeOffset != beforeOffset || gotAfterOffset != afterOffset || gotReason != reason {
		t.Fatalf("unexpected checkpoint repair audit row: mode=%s outcome=%s skip=%s before=%d after=%d reason=%s", gotMode, gotOutcome, gotSkipReason, gotBeforeOffset, gotAfterOffset, gotReason)
	}
}
