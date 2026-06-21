package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

const defaultEmbeddingTaskClaimTimeout = 30 * time.Second
const embeddingTaskCursorPrefix = "embedding-task:"

type EmbeddingTaskSourceConfig struct {
	TenantID     string
	ClaimTimeout time.Duration
}

type EmbeddingTaskSource struct {
	pool   *pgxpool.Pool
	config EmbeddingTaskSourceConfig
}

func NewEmbeddingTaskSource(pool *pgxpool.Pool, config EmbeddingTaskSourceConfig) *EmbeddingTaskSource {
	config.TenantID = strings.TrimSpace(config.TenantID)
	if config.ClaimTimeout <= 0 {
		config.ClaimTimeout = defaultEmbeddingTaskClaimTimeout
	}
	return &EmbeddingTaskSource{pool: pool, config: config}
}

// EnqueueEmbeddingTask persists a first-stage queue task. task.InputText must be
// a redacted preview suitable for storage, not raw document or message body.
func (source *EmbeddingTaskSource) EnqueueEmbeddingTask(ctx context.Context, task types.VectorEmbeddingTask) (bool, error) {
	if source == nil || source.pool == nil {
		return false, types.NewDBWriteFailed("vector embedding task source is not configured")
	}
	task = task.Normalized()
	if err := task.Validate(); err != nil {
		return false, err
	}
	if expected := sha256Ref(task.InputText); expected != task.InputHash {
		return false, types.NewInvalidArgument("embedding input hash mismatch")
	}
	now := time.Now().UTC()
	taskID := embeddingTaskID(task)
	tag, err := source.pool.Exec(ctx, `
INSERT INTO vector_embedding_tasks (
    tenant_id, task_id, source_service, collection_type, source_ref_hash,
    source_id, source_version, source_hash, chunk_hash, input_preview_redacted,
    input_hash, input_schema_version, embedding_model_ref, dimension,
    visibility_scope, visibility_version, policy_version, data_class, delete_proof_id,
    retention_policy_ref, idempotency_key, correlation_id, causation_id, trace_id,
    status, available_at, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10,
    $11, $12, $13, $14,
    $15, $16, $17, $18, $19,
    $20, $21, $22, $23, $24,
    'PENDING', $25, $25, $25
)
ON CONFLICT (tenant_id, source_service, source_id, source_version, embedding_model_ref, idempotency_key)
DO NOTHING
`, string(task.AuthContext.TenantID), taskID, task.SourceService, task.CollectionType, task.SourceRefHash,
		task.SourceID, task.SourceVersion, task.SourceHash, task.ChunkHash, task.InputText,
		task.InputHash, task.InputSchemaVersion, task.EmbeddingModelRef, task.Dimension,
		task.VisibilityScope, task.VisibilityVersion, task.PolicyVersion, task.DataClass, task.DeleteProofID,
		task.RetentionPolicyRef, task.IdempotencyKey, task.CorrelationID, task.CausationID, task.TraceID,
		now)
	if err != nil {
		return false, types.NewDBWriteFailed(err.Error())
	}
	if tag.RowsAffected() == 1 {
		return false, nil
	}
	matches, err := source.embeddingTaskMatches(ctx, task)
	if err != nil {
		return false, err
	}
	if !matches {
		return false, types.NewAlreadyExists("vector embedding task idempotency conflict")
	}
	return true, nil
}

func (source *EmbeddingTaskSource) ClaimEmbeddingTasks(ctx context.Context, limit int) ([]types.VectorEmbeddingTask, error) {
	if source == nil || source.pool == nil {
		return nil, types.NewDBWriteFailed("vector embedding task source is not configured")
	}
	if limit <= 0 {
		limit = 50
	}
	tx, err := source.pool.Begin(ctx)
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tasks, err := source.selectReadyEmbeddingTasks(ctx, tx, limit)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	deadline := now.Add(source.config.ClaimTimeout)
	for _, task := range tasks {
		if err := markEmbeddingTaskRunning(ctx, tx, task, now, deadline); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	return tasks, nil
}

func (source *EmbeddingTaskSource) CompleteEmbeddingTask(ctx context.Context, task types.VectorEmbeddingTask) error {
	if source == nil || source.pool == nil {
		return types.NewDBWriteFailed("vector embedding task source is not configured")
	}
	task = task.Normalized()
	if err := task.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	tag, err := source.pool.Exec(ctx, `
UPDATE vector_embedding_tasks
SET status = 'COMPLETED',
    completed_at = $7,
    updated_at = $7
WHERE tenant_id = $1
  AND source_service = $2
  AND source_id = $3
  AND source_version = $4
  AND embedding_model_ref = $5
  AND idempotency_key = $6
  AND status = 'RUNNING'
`, string(task.AuthContext.TenantID), task.SourceService, task.SourceID, task.SourceVersion, task.EmbeddingModelRef, task.IdempotencyKey, now)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if tag.RowsAffected() != 1 {
		return types.NewFailedPrecondition("vector embedding task is not running")
	}
	return nil
}

// ListCompletedEmbeddingTasks returns completed redacted-preview tasks for a rebuild job.
// It intentionally reads only vector-index-service owned queue rows and never reaches
// into upstream service private storage.
func (source *EmbeddingTaskSource) ListCompletedEmbeddingTasks(
	ctx context.Context,
	task types.VectorRebuildTask,
	limit int,
) ([]types.VectorEmbeddingTask, error) {
	if source == nil || source.pool == nil {
		return nil, types.NewDBReadFailed("vector embedding task source is not configured")
	}
	if limit <= 0 {
		limit = 100
	}
	cursor := embeddingTaskCursorValue(task.Checkpoint.CursorValue)
	rows, err := source.pool.Query(ctx, `
SELECT
    tenant_id, source_service, collection_type, source_ref_hash,
    source_id, source_version, source_hash, chunk_hash, input_preview_redacted,
    input_hash, input_schema_version, embedding_model_ref, dimension,
    visibility_scope, visibility_version, policy_version, data_class, delete_proof_id,
    retention_policy_ref, idempotency_key, correlation_id, causation_id, trace_id
FROM vector_embedding_tasks
WHERE tenant_id = $1
  AND source_service = $2
  AND collection_type = $3
  AND embedding_model_ref = $4
  AND dimension = $5
  AND status = 'COMPLETED'
  AND ($6 = '' OR idempotency_key > $6)
ORDER BY idempotency_key, task_id
LIMIT $7
`, string(task.Job.TenantID), task.Checkpoint.SourceService, task.CollectionType,
		task.EmbeddingModelRef, task.Dimension, cursor, limit)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()
	return scanEmbeddingTasks(rows)
}

func embeddingTaskCursorValue(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, embeddingTaskCursorPrefix) {
		return strings.TrimPrefix(value, embeddingTaskCursorPrefix)
	}
	return ""
}

func (source *EmbeddingTaskSource) embeddingTaskMatches(ctx context.Context, task types.VectorEmbeddingTask) (bool, error) {
	var sourceRefHash string
	var sourceHash string
	var chunkHash string
	var inputHash string
	var embeddingModelRef string
	var dimension int
	err := source.pool.QueryRow(ctx, `
SELECT source_ref_hash, source_hash, chunk_hash, input_hash, embedding_model_ref, dimension
FROM vector_embedding_tasks
WHERE tenant_id = $1
  AND source_service = $2
  AND source_id = $3
  AND source_version = $4
  AND embedding_model_ref = $5
  AND idempotency_key = $6
`, string(task.AuthContext.TenantID), task.SourceService, task.SourceID, task.SourceVersion, task.EmbeddingModelRef, task.IdempotencyKey).
		Scan(&sourceRefHash, &sourceHash, &chunkHash, &inputHash, &embeddingModelRef, &dimension)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, types.NewDBReadFailed(err.Error())
	}
	return sourceRefHash == task.SourceRefHash &&
		sourceHash == task.SourceHash &&
		chunkHash == task.ChunkHash &&
		inputHash == task.InputHash &&
		embeddingModelRef == task.EmbeddingModelRef &&
		dimension == task.Dimension, nil
}

func (source *EmbeddingTaskSource) selectReadyEmbeddingTasks(ctx context.Context, tx pgx.Tx, limit int) ([]types.VectorEmbeddingTask, error) {
	query := `
SELECT
    tenant_id, source_service, collection_type, source_ref_hash,
    source_id, source_version, source_hash, chunk_hash, input_preview_redacted,
    input_hash, input_schema_version, embedding_model_ref, dimension,
    visibility_scope, visibility_version, policy_version, data_class, delete_proof_id,
    retention_policy_ref, idempotency_key, correlation_id, causation_id, trace_id
FROM vector_embedding_tasks
WHERE status IN ('PENDING', 'RUNNING')
  AND (status = 'PENDING' OR claim_deadline <= now())
  AND available_at <= now()
  AND ($1 = '' OR tenant_id = $1)
ORDER BY created_at, task_id
LIMIT $2
FOR UPDATE SKIP LOCKED
`
	rows, err := tx.Query(ctx, query, source.config.TenantID, limit)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	return scanEmbeddingTasks(rows)
}

type embeddingTaskRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanEmbeddingTasks(rows embeddingTaskRows) ([]types.VectorEmbeddingTask, error) {
	tasks := []types.VectorEmbeddingTask{}
	for rows.Next() {
		var task types.VectorEmbeddingTask
		var tenantID string
		err := rows.Scan(
			&tenantID, &task.SourceService, &task.CollectionType, &task.SourceRefHash,
			&task.SourceID, &task.SourceVersion, &task.SourceHash, &task.ChunkHash,
			&task.InputText, &task.InputHash, &task.InputSchemaVersion,
			&task.EmbeddingModelRef, &task.Dimension, &task.VisibilityScope,
			&task.VisibilityVersion, &task.PolicyVersion, &task.DataClass,
			&task.DeleteProofID, &task.RetentionPolicyRef, &task.IdempotencyKey,
			&task.CorrelationID, &task.CausationID, &task.TraceID,
		)
		if err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		task.AuthContext = types.AuthContext{
			TenantID:    types.TenantID(tenantID),
			ServiceName: types.AllowedCallerVectorIndex,
			InstanceRef: "vector-embedding-worker",
			TraceID:     task.TraceID,
			RequestID:   "vector-embedding-queue",
		}
		tasks = append(tasks, task.Normalized())
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return tasks, nil
}

func markEmbeddingTaskRunning(ctx context.Context, tx pgx.Tx, task types.VectorEmbeddingTask, claimedAt time.Time, claimDeadline time.Time) error {
	tag, err := tx.Exec(ctx, `
UPDATE vector_embedding_tasks
SET status = 'RUNNING',
    attempt_count = attempt_count + 1,
    claimed_at = $7,
    claim_deadline = $8,
    updated_at = $7
WHERE tenant_id = $1
  AND source_service = $2
  AND source_id = $3
  AND source_version = $4
  AND embedding_model_ref = $5
  AND idempotency_key = $6
  AND status IN ('PENDING', 'RUNNING')
`, string(task.AuthContext.TenantID), task.SourceService, task.SourceID, task.SourceVersion, task.EmbeddingModelRef, task.IdempotencyKey, claimedAt, claimDeadline)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if tag.RowsAffected() != 1 {
		return types.NewFailedPrecondition("vector embedding task is not claimable")
	}
	return nil
}

func embeddingTaskID(task types.VectorEmbeddingTask) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		string(task.AuthContext.TenantID),
		task.SourceService,
		task.SourceID,
		task.EmbeddingModelRef,
		task.IdempotencyKey,
	}, "|")))
	return "vemb_" + hex.EncodeToString(sum[:])[:24]
}

func sha256Ref(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return "sha256:" + hex.EncodeToString(sum[:])
}
