package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/domain"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (repository *Repository) UpsertVectorItem(
	ctx context.Context,
	prepared domain.PreparedUpsert,
) (types.VectorItem, types.VectorIndexJob, bool, error) {
	if repository.pool == nil {
		return types.VectorItem{}, types.VectorIndexJob{}, false, types.NewDBWriteFailed("vector repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.VectorItem{}, types.VectorIndexJob{}, false, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	existing, found, err := findItemByIdempotency(ctx, tx, prepared.Command)
	if err != nil {
		return types.VectorItem{}, types.VectorIndexJob{}, false, err
	}
	if found {
		if existing.CommandHash != prepared.CommandHash {
			return types.VectorItem{}, types.VectorIndexJob{}, false, types.NewAlreadyExists("vector item idempotency conflict")
		}
		job, err := findJobByIdempotency(ctx, tx, existing.TenantID, existing.VectorItemID, types.JobTypeUpsert, existing.IdempotencyKey)
		if err != nil {
			return types.VectorItem{}, types.VectorIndexJob{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return types.VectorItem{}, types.VectorIndexJob{}, false, types.NewDBWriteFailed(err.Error())
		}
		return existing, job, true, nil
	}

	collection := domain.CollectionFromPrepared(prepared)
	item := domain.ItemFromPrepared(prepared)
	job := domain.UpsertJobFromPrepared(prepared)
	if err := upsertCollection(ctx, tx, collection); err != nil {
		return types.VectorItem{}, types.VectorIndexJob{}, false, err
	}
	if err := insertItem(ctx, tx, item); err != nil {
		return types.VectorItem{}, types.VectorIndexJob{}, false, err
	}
	if err := insertJob(ctx, tx, job); err != nil {
		return types.VectorItem{}, types.VectorIndexJob{}, false, err
	}
	if err := insertIndexedOutbox(ctx, tx, item); err != nil {
		return types.VectorItem{}, types.VectorIndexJob{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.VectorItem{}, types.VectorIndexJob{}, false, types.NewDBWriteFailed(err.Error())
	}
	return item, job, false, nil
}

func (repository *Repository) TombstoneVectorItem(
	ctx context.Context,
	prepared domain.PreparedTombstone,
) (types.VectorItem, types.VectorIndexJob, string, bool, error) {
	if repository.pool == nil {
		return types.VectorItem{}, types.VectorIndexJob{}, "", false, types.NewDBWriteFailed("vector repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.VectorItem{}, types.VectorIndexJob{}, "", false, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	existingTombstone, found, err := findTombstoneByIdempotency(ctx, tx, prepared.Command)
	if err != nil {
		return types.VectorItem{}, types.VectorIndexJob{}, "", false, err
	}
	if found {
		if existingTombstone.CommandHash != prepared.CommandHash {
			return types.VectorItem{}, types.VectorIndexJob{}, "", false, types.NewAlreadyExists("vector tombstone idempotency conflict")
		}
		item, err := getItemForUpdate(ctx, tx, prepared.Command.AuthContext.TenantID, prepared.Command.VectorItemID)
		if err != nil {
			return types.VectorItem{}, types.VectorIndexJob{}, "", false, err
		}
		job, err := findJobByIdempotency(ctx, tx, item.TenantID, item.VectorItemID, types.JobTypeTombstone, prepared.Command.IdempotencyKey)
		if err != nil {
			return types.VectorItem{}, types.VectorIndexJob{}, "", false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return types.VectorItem{}, types.VectorIndexJob{}, "", false, types.NewDBWriteFailed(err.Error())
		}
		return item, job, existingTombstone.TombstoneID, true, nil
	}

	item, err := getItemForUpdate(ctx, tx, prepared.Command.AuthContext.TenantID, prepared.Command.VectorItemID)
	if err != nil {
		return types.VectorItem{}, types.VectorIndexJob{}, "", false, err
	}
	if item.TombstoneStatus == types.TombstoneStatusTombstoned || item.Status == types.VectorItemStatusTombstoned {
		return types.VectorItem{}, types.VectorIndexJob{}, "", false, types.NewFailedPrecondition("vector item is already tombstoned")
	}
	tombstone := domain.TombstoneFromPrepared(prepared, item)
	job := domain.TombstoneJobFromPrepared(prepared, item)
	updated, err := markItemTombstoned(ctx, tx, item, prepared.Command.DeleteProofID, prepared.CreatedAt)
	if err != nil {
		return types.VectorItem{}, types.VectorIndexJob{}, "", false, err
	}
	if err := insertTombstone(ctx, tx, tombstone); err != nil {
		return types.VectorItem{}, types.VectorIndexJob{}, "", false, err
	}
	if err := insertJob(ctx, tx, job); err != nil {
		return types.VectorItem{}, types.VectorIndexJob{}, "", false, err
	}
	if err := insertTombstonedOutbox(ctx, tx, updated, tombstone); err != nil {
		return types.VectorItem{}, types.VectorIndexJob{}, "", false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.VectorItem{}, types.VectorIndexJob{}, "", false, types.NewDBWriteFailed(err.Error())
	}
	return updated, job, tombstone.TombstoneID, false, nil
}

func (repository *Repository) SearchVectors(
	ctx context.Context,
	command types.SearchVectorsCommand,
) ([]types.VectorSearchResult, error) {
	if repository.pool == nil {
		return nil, types.NewDBReadFailed("vector repository is not configured")
	}
	if command.MinScore > 1 {
		return nil, nil
	}
	rows, err := repository.pool.Query(ctx, `
SELECT vi.vector_item_id, vi.source_ref_hash, vi.source_service, vc.collection_type,
       vi.visibility_version, vi.tombstone_status
FROM vector_items vi
JOIN vector_collections vc
  ON vc.tenant_id = vi.tenant_id AND vc.collection_id = vi.collection_id
WHERE vi.tenant_id = $1
  AND vi.status = 'INDEXED'
  AND vi.tombstone_status = 'NONE'
  AND vi.delete_proof_id = ''
  AND vi.visibility_scope = $2
  AND vi.policy_version = $3
  AND (cardinality($4::text[]) = 0 OR vc.collection_type = ANY($4::text[]))
ORDER BY vi.updated_at DESC, vi.vector_item_id
LIMIT $5
`, string(command.AuthContext.TenantID), command.VisibilityScope, command.PolicyVersion, command.CollectionTypes, command.TopK)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	results := []types.VectorSearchResult{}
	for rows.Next() {
		var result types.VectorSearchResult
		if err := rows.Scan(&result.VectorItemRef, &result.SourceRefHash, &result.SourceService, &result.CollectionType, &result.VisibilityVersion, &result.TombstoneStatus); err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		result.Score = 1.0
		if result.Score >= command.MinScore {
			results = append(results, result)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return results, nil
}

func (repository *Repository) GetVectorIndexJob(
	ctx context.Context,
	command types.GetVectorIndexJobCommand,
) (types.VectorIndexJob, error) {
	if repository.pool == nil {
		return types.VectorIndexJob{}, types.NewDBReadFailed("vector repository is not configured")
	}
	row := repository.pool.QueryRow(ctx, selectJobSQL()+`
WHERE tenant_id = $1 AND job_id = $2
`, string(command.AuthContext.TenantID), command.JobID)
	job, err := scanJob(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.VectorIndexJob{}, types.NewNotFound("vector job not found")
		}
		return types.VectorIndexJob{}, types.NewDBReadFailed(err.Error())
	}
	return job, nil
}

func upsertCollection(ctx context.Context, tx pgx.Tx, collection types.VectorCollection) error {
	_, err := tx.Exec(ctx, `
INSERT INTO vector_collections (
    tenant_id, collection_id, collection_type, backend_type, dimension,
    embedding_model_ref, route_policy_ref, status, metadata_schema_version,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9,
    $10, $11
)
ON CONFLICT (tenant_id, collection_id) DO UPDATE
SET status = EXCLUDED.status,
    updated_at = EXCLUDED.updated_at
`, string(collection.TenantID), collection.CollectionID, collection.CollectionType, collection.BackendType,
		collection.Dimension, collection.EmbeddingModelRef, collection.RoutePolicyRef, collection.Status,
		collection.MetadataSchemaVersion, collection.CreatedAt, collection.UpdatedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertItem(ctx context.Context, tx pgx.Tx, item types.VectorItem) error {
	_, err := tx.Exec(ctx, `
INSERT INTO vector_items (
    tenant_id, vector_item_id, collection_id, source_service, source_ref_hash,
    source_id, source_version, source_hash, chunk_hash, embedding_model_ref,
    embedding_vector_hash, dimension, visibility_scope, visibility_version,
    policy_version, data_class, tombstone_status, delete_proof_id,
    retention_policy_ref, status, idempotency_key, command_hash,
    correlation_id, causation_id, trace_id, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10,
    $11, $12, $13, $14,
    $15, $16, $17, $18,
    $19, $20, $21, $22,
    $23, $24, $25, $26, $27
)
`, string(item.TenantID), item.VectorItemID, item.CollectionID, item.SourceService, item.SourceRefHash,
		item.SourceID, item.SourceVersion, item.SourceHash, item.ChunkHash, item.EmbeddingModelRef,
		item.EmbeddingVectorHash, item.Dimension, item.VisibilityScope, item.VisibilityVersion,
		item.PolicyVersion, item.DataClass, item.TombstoneStatus, item.DeleteProofID,
		item.RetentionPolicyRef, item.Status, item.IdempotencyKey, item.CommandHash,
		item.CorrelationID, item.CausationID, item.TraceID, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertJob(ctx context.Context, tx pgx.Tx, job types.VectorIndexJob) error {
	var completedAt any
	if !job.CompletedAt.IsZero() {
		completedAt = job.CompletedAt
	}
	_, err := tx.Exec(ctx, `
INSERT INTO vector_index_jobs (
    tenant_id, job_id, collection_id, vector_item_id, job_type, status,
    retry_count, failure_class, public_error, idempotency_key, command_hash,
    correlation_id, causation_id, trace_id, created_at, completed_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11,
    $12, $13, $14, $15, $16
)
`, string(job.TenantID), job.JobID, job.CollectionID, job.VectorItemID, job.JobType, job.Status,
		job.RetryCount, job.FailureClass, job.PublicError, job.IdempotencyKey, job.CommandHash,
		job.CorrelationID, job.CausationID, job.TraceID, job.CreatedAt, completedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertTombstone(ctx context.Context, tx pgx.Tx, tombstone types.VectorTombstone) error {
	_, err := tx.Exec(ctx, `
INSERT INTO vector_tombstones (
    tenant_id, tombstone_id, vector_item_id, source_ref_hash, delete_proof_id,
    reason_class, backend_delete_status, idempotency_key, command_hash, created_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10
)
`, string(tombstone.TenantID), tombstone.TombstoneID, tombstone.VectorItemID, tombstone.SourceRefHash,
		tombstone.DeleteProofID, tombstone.ReasonClass, tombstone.BackendDeleteStatus,
		tombstone.IdempotencyKey, tombstone.CommandHash, tombstone.CreatedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func markItemTombstoned(ctx context.Context, tx pgx.Tx, item types.VectorItem, deleteProofID string, now time.Time) (types.VectorItem, error) {
	_, err := tx.Exec(ctx, `
UPDATE vector_items
SET tombstone_status = 'TOMBSTONED',
    delete_proof_id = $3,
    status = 'TOMBSTONED',
    updated_at = $4
WHERE tenant_id = $1 AND vector_item_id = $2
`, string(item.TenantID), item.VectorItemID, deleteProofID, now)
	if err != nil {
		return types.VectorItem{}, types.NewDBWriteFailed(err.Error())
	}
	item.TombstoneStatus = types.TombstoneStatusTombstoned
	item.DeleteProofID = deleteProofID
	item.Status = types.VectorItemStatusTombstoned
	item.UpdatedAt = now
	return item, nil
}

func insertIndexedOutbox(ctx context.Context, tx pgx.Tx, item types.VectorItem) error {
	payload := itemPayload(item)
	return insertOutbox(ctx, tx, "evt_"+item.VectorItemID, item, "vector.item.indexed.v1", payload)
}

func insertTombstonedOutbox(ctx context.Context, tx pgx.Tx, item types.VectorItem, tombstone types.VectorTombstone) error {
	payload := itemPayload(item)
	payload["delete_proof_id"] = tombstone.DeleteProofID
	payload["reason_class"] = tombstone.ReasonClass
	return insertOutbox(ctx, tx, "evt_"+tombstone.TombstoneID, item, "vector.item.tombstoned.v1", payload)
}

func insertOutbox(ctx context.Context, tx pgx.Tx, eventID string, item types.VectorItem, eventType string, payload map[string]any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	_, err = tx.Exec(ctx, `
INSERT INTO vector_outbox (
    event_id, tenant_id, aggregate_type, aggregate_id, event_type,
    event_version, partition_key, payload_json, status, available_at, created_at, updated_at
) VALUES (
    $1, $2, 'vector_item', $3, $4,
    1, $5, $6::jsonb, 'PENDING', now(), now(), now()
)
ON CONFLICT (event_id) DO NOTHING
`, eventID, string(item.TenantID), domain.HashRef(item.VectorItemID), eventType,
		string(item.TenantID)+":"+domain.HashRef(item.VectorItemID), string(encoded))
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func itemPayload(item types.VectorItem) map[string]any {
	return map[string]any{
		"vector_item_ref_hash": domain.HashRef(item.VectorItemID),
		"collection_type":      item.CollectionType,
		"source_service":       item.SourceService,
		"source_ref_hash":      item.SourceRefHash,
		"embedding_model_ref":  item.EmbeddingModelRef,
		"dimension":            item.Dimension,
		"visibility_version":   item.VisibilityVersion,
		"tombstone_status":     item.TombstoneStatus,
	}
}

func findItemByIdempotency(ctx context.Context, tx pgx.Tx, command types.UpsertVectorItemCommand) (types.VectorItem, bool, error) {
	row := tx.QueryRow(ctx, selectItemSQL()+`
WHERE vi.tenant_id = $1
  AND vi.source_service = $2
  AND vi.source_id = $3
  AND vi.source_version = $4
  AND vi.embedding_model_ref = $5
  AND vi.idempotency_key = $6
LIMIT 1
`, string(command.AuthContext.TenantID), command.SourceService, command.SourceID,
		command.SourceVersion, command.EmbeddingModelRef, command.IdempotencyKey)
	item, err := scanItem(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.VectorItem{}, false, nil
		}
		return types.VectorItem{}, false, types.NewDBReadFailed(err.Error())
	}
	return item, true, nil
}

func getItemForUpdate(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, vectorItemID string) (types.VectorItem, error) {
	row := tx.QueryRow(ctx, selectItemSQL()+`
WHERE vi.tenant_id = $1 AND vi.vector_item_id = $2
FOR UPDATE OF vi
`, string(tenantID), vectorItemID)
	item, err := scanItem(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.VectorItem{}, types.NewNotFound("vector item not found")
		}
		return types.VectorItem{}, types.NewDBReadFailed(err.Error())
	}
	return item, nil
}

func findJobByIdempotency(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, vectorItemID string, jobType string, key string) (types.VectorIndexJob, error) {
	row := tx.QueryRow(ctx, selectJobSQL()+`
WHERE tenant_id = $1 AND vector_item_id = $2 AND job_type = $3 AND idempotency_key = $4
LIMIT 1
`, string(tenantID), vectorItemID, jobType, key)
	job, err := scanJob(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.VectorIndexJob{}, types.NewNotFound("vector job not found")
		}
		return types.VectorIndexJob{}, types.NewDBReadFailed(err.Error())
	}
	return job, nil
}

func findTombstoneByIdempotency(ctx context.Context, tx pgx.Tx, command types.TombstoneVectorItemCommand) (types.VectorTombstone, bool, error) {
	row := tx.QueryRow(ctx, `
SELECT tenant_id, tombstone_id, vector_item_id, source_ref_hash, delete_proof_id,
       reason_class, backend_delete_status, idempotency_key, command_hash, created_at
FROM vector_tombstones
WHERE tenant_id = $1 AND vector_item_id = $2 AND idempotency_key = $3
LIMIT 1
`, string(command.AuthContext.TenantID), command.VectorItemID, command.IdempotencyKey)
	tombstone, err := scanTombstone(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.VectorTombstone{}, false, nil
		}
		return types.VectorTombstone{}, false, types.NewDBReadFailed(err.Error())
	}
	return tombstone, true, nil
}

func selectItemSQL() string {
	return `
SELECT vi.tenant_id, vi.vector_item_id, vi.collection_id, vc.collection_type,
       vi.source_service, vi.source_ref_hash, vi.source_id, vi.source_version,
       vi.source_hash, vi.chunk_hash, vi.embedding_model_ref, vi.embedding_vector_hash,
       vi.dimension, vi.visibility_scope, vi.visibility_version, vi.policy_version,
       vi.data_class, vi.tombstone_status, vi.delete_proof_id, vi.retention_policy_ref,
       vi.status, vi.idempotency_key, vi.command_hash, vi.correlation_id,
       vi.causation_id, vi.trace_id, vi.created_at, vi.updated_at
FROM vector_items vi
JOIN vector_collections vc
  ON vc.tenant_id = vi.tenant_id AND vc.collection_id = vi.collection_id
`
}

func selectJobSQL() string {
	return `
SELECT tenant_id, job_id, collection_id, vector_item_id, job_type, status,
       retry_count, failure_class, public_error, idempotency_key, command_hash,
       correlation_id, causation_id, trace_id, created_at, completed_at
FROM vector_index_jobs
`
}

type scanner interface {
	Scan(dest ...any) error
}

func scanItem(row scanner) (types.VectorItem, error) {
	var item types.VectorItem
	err := row.Scan(
		&item.TenantID, &item.VectorItemID, &item.CollectionID, &item.CollectionType,
		&item.SourceService, &item.SourceRefHash, &item.SourceID, &item.SourceVersion,
		&item.SourceHash, &item.ChunkHash, &item.EmbeddingModelRef, &item.EmbeddingVectorHash,
		&item.Dimension, &item.VisibilityScope, &item.VisibilityVersion, &item.PolicyVersion,
		&item.DataClass, &item.TombstoneStatus, &item.DeleteProofID, &item.RetentionPolicyRef,
		&item.Status, &item.IdempotencyKey, &item.CommandHash, &item.CorrelationID,
		&item.CausationID, &item.TraceID, &item.CreatedAt, &item.UpdatedAt,
	)
	return item, err
}

func scanJob(row scanner) (types.VectorIndexJob, error) {
	var job types.VectorIndexJob
	var completedAt *time.Time
	err := row.Scan(
		&job.TenantID, &job.JobID, &job.CollectionID, &job.VectorItemID, &job.JobType,
		&job.Status, &job.RetryCount, &job.FailureClass, &job.PublicError,
		&job.IdempotencyKey, &job.CommandHash, &job.CorrelationID, &job.CausationID,
		&job.TraceID, &job.CreatedAt, &completedAt,
	)
	if completedAt != nil {
		job.CompletedAt = *completedAt
	}
	return job, err
}

func scanTombstone(row scanner) (types.VectorTombstone, error) {
	var tombstone types.VectorTombstone
	err := row.Scan(
		&tombstone.TenantID, &tombstone.TombstoneID, &tombstone.VectorItemID,
		&tombstone.SourceRefHash, &tombstone.DeleteProofID, &tombstone.ReasonClass,
		&tombstone.BackendDeleteStatus, &tombstone.IdempotencyKey, &tombstone.CommandHash,
		&tombstone.CreatedAt,
	)
	return tombstone, err
}
