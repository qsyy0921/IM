package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

type ProjectionRepairStore struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

type ProjectionRepairStoreOption func(*ProjectionRepairStore)

func NewProjectionRepairStore(pool *pgxpool.Pool, opts ...ProjectionRepairStoreOption) *ProjectionRepairStore {
	store := &ProjectionRepairStore{
		pool: pool,
		now:  func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(store)
	}
	return store
}

func WithProjectionRepairClock(clock func() time.Time) ProjectionRepairStoreOption {
	return func(store *ProjectionRepairStore) {
		if clock != nil {
			store.now = clock
		}
	}
}

func (store *ProjectionRepairStore) RepairCheckpoint(ctx context.Context, options types.ProjectionCheckpointRepairOptions) (types.ProjectionCheckpointRepairStats, error) {
	if store == nil || store.pool == nil {
		return types.ProjectionCheckpointRepairStats{}, errors.New("delivery projection repair store is not configured")
	}
	mode := normalizeProjectionRepairMode(options.Mode)
	if mode == "" {
		return types.ProjectionCheckpointRepairStats{}, types.NewInvalidArgument("unsupported delivery projection repair mode")
	}
	if strings.TrimSpace(options.ConsumerGroup) == "" {
		return types.ProjectionCheckpointRepairStats{}, types.NewInvalidArgument("consumer_group is required")
	}
	if strings.TrimSpace(options.Topic) == "" {
		return types.ProjectionCheckpointRepairStats{}, types.NewInvalidArgument("topic is required")
	}
	if options.PartitionID < 0 {
		return types.ProjectionCheckpointRepairStats{}, types.NewInvalidArgument("partition_id must be non-negative")
	}
	if mode == types.ProjectionCheckpointRepairModeRewindNextOffset && options.TargetOffset <= 0 {
		return types.ProjectionCheckpointRepairStats{}, types.NewInvalidArgument("target_offset must be positive")
	}
	if mode == types.ProjectionCheckpointRepairModeRewindFailure && options.FailureOffset < 0 {
		return types.ProjectionCheckpointRepairStats{}, types.NewInvalidArgument("failure_offset must be non-negative")
	}
	operator := normalizeProjectionRepairText(options.Operator, "manual")
	reason := normalizeProjectionRepairText(options.Reason, "manual delivery projection checkpoint repair")

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return types.ProjectionCheckpointRepairStats{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	before, err := store.readCheckpointLocked(ctx, tx, options.ConsumerGroup, options.Topic, options.PartitionID)
	if err != nil {
		return types.ProjectionCheckpointRepairStats{}, err
	}
	after := before
	outcome := checkpointRepairOutcomeAudited
	skipReason := ""
	failure := projectionFailureRepairRow{}

	if mode == types.ProjectionCheckpointRepairModeRewindNextOffset {
		if options.TargetOffset >= before.OffsetValue {
			outcome = checkpointRepairOutcomeSkipped
			skipReason = checkpointRepairSkipTargetNotLower
		} else if !options.DryRun {
			if _, err := tx.Exec(ctx, `
UPDATE delivery_kafka_checkpoints
SET offset_value = $4,
    updated_at = now()
WHERE consumer_group = $1
  AND topic = $2
  AND partition_id = $3
`, before.ConsumerGroup, before.Topic, before.PartitionID, options.TargetOffset); err != nil {
				return types.ProjectionCheckpointRepairStats{}, types.NewDBWriteFailed(err.Error())
			}
			after.OffsetValue = options.TargetOffset
			outcome = checkpointRepairOutcomeMutated
		}
	} else if mode == types.ProjectionCheckpointRepairModeRewindFailure {
		failure, err = store.readUnresolvedFailureLocked(ctx, tx, options.ConsumerGroup, options.Topic, options.PartitionID, options.FailureOffset)
		if err != nil {
			return types.ProjectionCheckpointRepairStats{}, err
		}
		if failure.OffsetValue >= before.OffsetValue {
			outcome = checkpointRepairOutcomeSkipped
			skipReason = checkpointRepairSkipTargetNotLower
		} else if !options.DryRun {
			if _, err := tx.Exec(ctx, `
UPDATE delivery_kafka_checkpoints
SET offset_value = $4,
    updated_at = now()
WHERE consumer_group = $1
  AND topic = $2
  AND partition_id = $3
`, before.ConsumerGroup, before.Topic, before.PartitionID, failure.OffsetValue); err != nil {
				return types.ProjectionCheckpointRepairStats{}, types.NewDBWriteFailed(err.Error())
			}
			after.OffsetValue = failure.OffsetValue
			outcome = checkpointRepairOutcomeMutated
		}
	}

	if err := store.insertCheckpointRepairAudit(ctx, tx, before, after, failure, mode, outcome, skipReason, operator, reason, options.DryRun, store.now()); err != nil {
		return types.ProjectionCheckpointRepairStats{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.ProjectionCheckpointRepairStats{}, types.NewDBWriteFailed(err.Error())
	}

	stats := types.ProjectionCheckpointRepairStats{Requested: 1}
	switch outcome {
	case checkpointRepairOutcomeAudited:
		stats.Audited = 1
	case checkpointRepairOutcomeMutated:
		stats.Mutated = 1
	case checkpointRepairOutcomeSkipped:
		stats.Skipped = 1
	}
	return stats, nil
}

type checkpointRepairRow struct {
	ConsumerGroup string
	Topic         string
	PartitionID   int32
	OffsetValue   int64
}

type projectionFailureRepairRow struct {
	OffsetValue  int64
	EventID      string
	FailureClass string
}

const (
	checkpointRepairOutcomeAudited = "AUDITED"
	checkpointRepairOutcomeMutated = "MUTATED"
	checkpointRepairOutcomeSkipped = "SKIPPED"

	checkpointRepairSkipTargetNotLower = "target_offset_is_not_lower"
)

func (store *ProjectionRepairStore) readCheckpointLocked(ctx context.Context, tx pgx.Tx, consumerGroup string, topic string, partitionID int32) (checkpointRepairRow, error) {
	var row checkpointRepairRow
	err := tx.QueryRow(ctx, `
SELECT consumer_group, topic, partition_id, offset_value
FROM delivery_kafka_checkpoints
WHERE consumer_group = $1
  AND topic = $2
  AND partition_id = $3
FOR UPDATE
`, consumerGroup, topic, partitionID).Scan(
		&row.ConsumerGroup,
		&row.Topic,
		&row.PartitionID,
		&row.OffsetValue,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return checkpointRepairRow{}, types.NewInvalidArgument("delivery kafka checkpoint not found")
		}
		return checkpointRepairRow{}, types.NewDBReadFailed(err.Error())
	}
	return row, nil
}

func (store *ProjectionRepairStore) readUnresolvedFailureLocked(ctx context.Context, tx pgx.Tx, consumerGroup string, topic string, partitionID int32, failureOffset int64) (projectionFailureRepairRow, error) {
	var row projectionFailureRepairRow
	err := tx.QueryRow(ctx, `
SELECT offset_value, event_id, failure_class
FROM delivery_projection_failures
WHERE consumer_group = $1
  AND topic = $2
  AND partition_id = $3
  AND offset_value = $4
  AND resolved_at IS NULL
FOR UPDATE
`, consumerGroup, topic, partitionID, failureOffset).Scan(
		&row.OffsetValue,
		&row.EventID,
		&row.FailureClass,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return projectionFailureRepairRow{}, types.NewInvalidArgument("unresolved delivery projection failure not found")
		}
		return projectionFailureRepairRow{}, types.NewDBReadFailed(err.Error())
	}
	return row, nil
}

func (store *ProjectionRepairStore) insertCheckpointRepairAudit(ctx context.Context, tx pgx.Tx, before checkpointRepairRow, after checkpointRepairRow, failure projectionFailureRepairRow, mode string, outcome string, skipReason string, operator string, reason string, dryRun bool, now time.Time) error {
	_, err := tx.Exec(ctx, `
INSERT INTO delivery_projection_checkpoint_repair_audit (
    consumer_group,
    topic,
    partition_id,
    mode,
    outcome,
    skip_reason,
    operator,
    reason,
    dry_run,
    before_offset_value,
    after_offset_value,
    failure_offset_value,
    failure_event_id,
    failure_class,
    created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
`,
		before.ConsumerGroup,
		before.Topic,
		before.PartitionID,
		mode,
		outcome,
		skipReason,
		operator,
		reason,
		dryRun,
		before.OffsetValue,
		after.OffsetValue,
		nullableInt64(failure.OffsetValue, mode == types.ProjectionCheckpointRepairModeRewindFailure),
		failure.EventID,
		failure.FailureClass,
		now,
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func normalizeProjectionRepairMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "":
		return types.ProjectionCheckpointRepairModeAudit
	case types.ProjectionCheckpointRepairModeAudit:
		return types.ProjectionCheckpointRepairModeAudit
	case types.ProjectionCheckpointRepairModeRewindNextOffset:
		return types.ProjectionCheckpointRepairModeRewindNextOffset
	case types.ProjectionCheckpointRepairModeRewindFailure:
		return types.ProjectionCheckpointRepairModeRewindFailure
	default:
		return ""
	}
}

func normalizeProjectionRepairText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func nullableInt64(value int64, ok bool) any {
	if !ok {
		return nil
	}
	return value
}
