package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/knowledge-ingestion-service/internal/domain"
	"github.com/qsyy0921/IM/services/knowledge-ingestion-service/internal/types"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (repository *Repository) CreateKnowledgeSource(
	ctx context.Context,
	prepared domain.PreparedKnowledgeSource,
) (types.KnowledgeSource, bool, error) {
	if repository.pool == nil {
		return types.KnowledgeSource{}, false, types.NewDBWriteFailed("knowledge repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.KnowledgeSource{}, false, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	existing, found, err := findSourceByIdempotency(ctx, tx, prepared.Command.AuthContext.TenantID, prepared.Command.IdempotencyKey)
	if err != nil {
		return types.KnowledgeSource{}, false, err
	}
	if found {
		if existing.CommandHash != prepared.CommandHash {
			return types.KnowledgeSource{}, false, types.NewFailedPrecondition("knowledge source idempotency conflict")
		}
		if err := tx.Commit(ctx); err != nil {
			return types.KnowledgeSource{}, false, types.NewDBWriteFailed(err.Error())
		}
		return existing, true, nil
	}

	source := domain.SourceFromPrepared(prepared)
	if err := insertSource(ctx, tx, source); err != nil {
		return types.KnowledgeSource{}, false, err
	}
	if err := insertSourceCreatedOutbox(ctx, tx, source); err != nil {
		return types.KnowledgeSource{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.KnowledgeSource{}, false, types.NewDBWriteFailed(err.Error())
	}
	return source, false, nil
}

func (repository *Repository) SubmitIngestionJob(
	ctx context.Context,
	prepared domain.PreparedIngestionJob,
) (types.KnowledgeIngestionJob, bool, error) {
	if repository.pool == nil {
		return types.KnowledgeIngestionJob{}, false, types.NewDBWriteFailed("knowledge repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.KnowledgeIngestionJob{}, false, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	existing, found, err := findJobByIdempotency(ctx, tx, prepared.Command.AuthContext.TenantID, prepared.Command.SourceID, prepared.Command.IdempotencyKey)
	if err != nil {
		return types.KnowledgeIngestionJob{}, false, err
	}
	if found {
		if existing.CommandHash != prepared.CommandHash {
			return types.KnowledgeIngestionJob{}, false, types.NewFailedPrecondition("knowledge job idempotency conflict")
		}
		if err := tx.Commit(ctx); err != nil {
			return types.KnowledgeIngestionJob{}, false, types.NewDBWriteFailed(err.Error())
		}
		return existing, true, nil
	}

	if err := lockSourceForJob(ctx, tx, prepared.Command.AuthContext.TenantID, prepared.Command.SourceID, prepared.Command.SourceVersion); err != nil {
		return types.KnowledgeIngestionJob{}, false, err
	}

	job := domain.JobFromPrepared(prepared)
	if err := insertJob(ctx, tx, job); err != nil {
		return types.KnowledgeIngestionJob{}, false, err
	}
	if len(prepared.Command.Chunks) > 0 {
		document := domain.DocumentFromPrepared(prepared)
		if err := insertDocument(ctx, tx, document); err != nil {
			return types.KnowledgeIngestionJob{}, false, err
		}
		chunks := domain.ChunksFromPrepared(prepared)
		for _, chunk := range chunks {
			if err := insertChunk(ctx, tx, chunk); err != nil {
				return types.KnowledgeIngestionJob{}, false, err
			}
			if err := insertChunkReadyOutbox(ctx, tx, chunk); err != nil {
				return types.KnowledgeIngestionJob{}, false, err
			}
		}
		if err := insertDocumentParsedOutbox(ctx, tx, document, len(chunks)); err != nil {
			return types.KnowledgeIngestionJob{}, false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return types.KnowledgeIngestionJob{}, false, types.NewDBWriteFailed(err.Error())
	}
	return job, false, nil
}

func (repository *Repository) GetIngestionJob(
	ctx context.Context,
	tenantID types.TenantID,
	jobID string,
) (types.KnowledgeIngestionJob, error) {
	if repository.pool == nil {
		return types.KnowledgeIngestionJob{}, types.NewDBReadFailed("knowledge repository is not configured")
	}
	row := repository.pool.QueryRow(ctx, selectJobSQL()+`
WHERE tenant_id = $1 AND job_id = $2
`, string(tenantID), jobID)
	job, err := scanJob(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.KnowledgeIngestionJob{}, types.NewNotFound("knowledge ingestion job not found")
		}
		return types.KnowledgeIngestionJob{}, types.NewDBReadFailed(err.Error())
	}
	return job, nil
}

func (repository *Repository) ListKnowledgeChunks(
	ctx context.Context,
	command types.ListKnowledgeChunksCommand,
) ([]types.KnowledgeChunk, string, error) {
	if repository.pool == nil {
		return nil, "", types.NewDBReadFailed("knowledge repository is not configured")
	}
	offset := 0
	if command.PageToken != "" {
		parsed, err := strconv.Atoi(command.PageToken)
		if err != nil || parsed < 0 {
			return nil, "", types.NewInvalidArgument("page_token is invalid")
		}
		offset = parsed
	}
	limit := command.PageSize
	if limit <= 0 {
		limit = 50
	}
	rows, err := repository.pool.Query(ctx, selectChunkSQL()+`
WHERE tenant_id = $1
  AND ($2 = '' OR source_id = $2)
  AND ($3 = '' OR document_id = $3)
ORDER BY source_id, document_id, chunk_index
LIMIT $4 OFFSET $5
`, string(command.AuthContext.TenantID), command.SourceID, command.DocumentID, limit+1, offset)
	if err != nil {
		return nil, "", types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	chunks := make([]types.KnowledgeChunk, 0, limit)
	hasMore := false
	for rows.Next() {
		chunk, err := scanChunk(rows)
		if err != nil {
			return nil, "", types.NewDBReadFailed(err.Error())
		}
		if len(chunks) < limit {
			chunks = append(chunks, chunk)
		} else {
			hasMore = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, "", types.NewDBReadFailed(err.Error())
	}
	nextToken := ""
	if hasMore {
		nextToken = strconv.Itoa(offset + limit)
	}
	return chunks, nextToken, nil
}

func findSourceByIdempotency(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	idempotencyKey string,
) (types.KnowledgeSource, bool, error) {
	row := tx.QueryRow(ctx, selectSourceSQL()+`
WHERE tenant_id = $1 AND idempotency_key = $2
LIMIT 1
`, string(tenantID), idempotencyKey)
	source, err := scanSource(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.KnowledgeSource{}, false, nil
		}
		return types.KnowledgeSource{}, false, types.NewDBReadFailed(err.Error())
	}
	return source, true, nil
}

func findJobByIdempotency(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	sourceID string,
	idempotencyKey string,
) (types.KnowledgeIngestionJob, bool, error) {
	row := tx.QueryRow(ctx, selectJobSQL()+`
WHERE tenant_id = $1 AND source_id = $2 AND idempotency_key = $3
LIMIT 1
`, string(tenantID), sourceID, idempotencyKey)
	job, err := scanJob(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.KnowledgeIngestionJob{}, false, nil
		}
		return types.KnowledgeIngestionJob{}, false, types.NewDBReadFailed(err.Error())
	}
	return job, true, nil
}

func lockSourceForJob(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, sourceID string, sourceVersion string) error {
	var status string
	err := tx.QueryRow(ctx, `
SELECT status
FROM knowledge_sources
WHERE tenant_id = $1 AND source_id = $2 AND source_version = $3
FOR UPDATE
`, string(tenantID), sourceID, sourceVersion).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.NewNotFound("knowledge source not found")
		}
		return types.NewDBReadFailed(err.Error())
	}
	if status != types.SourceStatusActive {
		return types.NewFailedPrecondition("knowledge source is not active")
	}
	return nil
}

func insertSource(ctx context.Context, tx pgx.Tx, source types.KnowledgeSource) error {
	_, err := tx.Exec(ctx, `
INSERT INTO knowledge_sources (
    tenant_id, source_id, idempotency_key, command_hash, source_type,
    source_ref, source_ref_hash, media_object_ref, owner_ref, visibility_scope,
    data_class, content_hash, mime_type, size_bytes, source_version,
    retention_policy_ref, status, correlation_id, causation_id, trace_id,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10,
    $11, $12, $13, $14, $15,
    $16, $17, $18, $19, $20,
    $21, $22
)
`, string(source.TenantID), source.SourceID, source.IdempotencyKey, source.CommandHash,
		source.SourceType, source.SourceRef, source.SourceRefHash, source.MediaObjectRef,
		source.OwnerRef, source.VisibilityScope, source.DataClass, source.ContentHash,
		source.MimeType, source.SizeBytes, source.SourceVersion, source.RetentionPolicyRef,
		source.Status, source.CorrelationID, source.CausationID, source.TraceID,
		source.CreatedAt, source.UpdatedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertJob(ctx context.Context, tx pgx.Tx, job types.KnowledgeIngestionJob) error {
	var completedAt any
	if !job.CompletedAt.IsZero() {
		completedAt = job.CompletedAt
	}
	_, err := tx.Exec(ctx, `
INSERT INTO knowledge_ingestion_jobs (
    tenant_id, job_id, idempotency_key, command_hash, source_id,
    source_version, job_type, parser_profile, chunk_profile,
    embedding_policy_ref, vector_policy_ref, requested_by, status,
    failure_class, public_error, document_id, chunk_count,
    correlation_id, causation_id, trace_id, created_at, completed_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9,
    $10, $11, $12, $13,
    $14, $15, $16, $17,
    $18, $19, $20, $21, $22
)
`, string(job.TenantID), job.JobID, job.IdempotencyKey, job.CommandHash, job.SourceID,
		job.SourceVersion, job.JobType, job.ParserProfile, job.ChunkProfile,
		job.EmbeddingPolicyRef, job.VectorPolicyRef, job.RequestedBy, job.Status,
		job.FailureClass, job.PublicError, job.DocumentID, job.ChunkCount,
		job.CorrelationID, job.CausationID, job.TraceID, job.CreatedAt, completedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertDocument(ctx context.Context, tx pgx.Tx, document types.KnowledgeDocument) error {
	_, err := tx.Exec(ctx, `
INSERT INTO knowledge_documents (
    tenant_id, document_id, source_id, source_version, parser_profile,
    mime_type, size_bytes, page_count, language, document_hash,
    parse_status, parser_failure_class, created_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10,
    $11, $12, $13
)
`, string(document.TenantID), document.DocumentID, document.SourceID, document.SourceVersion,
		document.ParserProfile, document.MimeType, document.SizeBytes, document.PageCount,
		document.Language, document.DocumentHash, document.ParseStatus, document.ParserFailureClass,
		document.CreatedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertChunk(ctx context.Context, tx pgx.Tx, chunk types.KnowledgeChunk) error {
	_, err := tx.Exec(ctx, `
INSERT INTO knowledge_chunks (
    tenant_id, chunk_id, document_id, source_id, source_version,
    chunk_index, chunk_hash, chunk_preview_redacted, visibility_scope,
    data_class, policy_version, chunk_version, status, tombstone_status,
    delete_proof_id, embedding_status, vector_status, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9,
    $10, $11, $12, $13, $14,
    $15, $16, $17, $18, $19
)
`, string(chunk.TenantID), chunk.ChunkID, chunk.DocumentID, chunk.SourceID, chunk.SourceVersion,
		chunk.ChunkIndex, chunk.ChunkHash, chunk.ChunkPreviewRedacted, chunk.VisibilityScope,
		chunk.DataClass, chunk.PolicyVersion, chunk.ChunkVersion, chunk.Status,
		chunk.TombstoneStatus, chunk.DeleteProofID, chunk.EmbeddingStatus, chunk.VectorStatus,
		chunk.CreatedAt, chunk.UpdatedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertSourceCreatedOutbox(ctx context.Context, tx pgx.Tx, source types.KnowledgeSource) error {
	payload := map[string]any{
		"tenant_id":        string(source.TenantID),
		"source_id":        source.SourceID,
		"source_type":      source.SourceType,
		"source_ref_hash":  source.SourceRefHash,
		"visibility_scope": source.VisibilityScope,
		"data_class":       source.DataClass,
		"content_hash":     source.ContentHash,
		"source_version":   source.SourceVersion,
		"correlation_id":   source.CorrelationID,
		"causation_id":     source.CausationID,
		"trace_id":         source.TraceID,
	}
	return insertOutbox(ctx, tx, "evt_"+source.SourceID, source.TenantID, "knowledge_source", source.SourceID, "knowledge.source.created.v1", string(source.TenantID)+":"+source.SourceID, payload)
}

func insertDocumentParsedOutbox(ctx context.Context, tx pgx.Tx, document types.KnowledgeDocument, chunkCount int) error {
	payload := map[string]any{
		"tenant_id":      string(document.TenantID),
		"document_id":    document.DocumentID,
		"source_id":      document.SourceID,
		"source_version": document.SourceVersion,
		"document_hash":  document.DocumentHash,
		"parser_profile": document.ParserProfile,
		"mime_type":      document.MimeType,
		"page_count":     document.PageCount,
		"chunk_count":    chunkCount,
	}
	return insertOutbox(ctx, tx, "evt_"+document.DocumentID, document.TenantID, "knowledge_document", document.DocumentID, "knowledge.document.parsed.v1", string(document.TenantID)+":"+document.DocumentID, payload)
}

func insertChunkReadyOutbox(ctx context.Context, tx pgx.Tx, chunk types.KnowledgeChunk) error {
	payload := map[string]any{
		"tenant_id":        string(chunk.TenantID),
		"chunk_id":         chunk.ChunkID,
		"document_id":      chunk.DocumentID,
		"source_id":        chunk.SourceID,
		"source_version":   chunk.SourceVersion,
		"chunk_index":      chunk.ChunkIndex,
		"chunk_hash":       chunk.ChunkHash,
		"visibility_scope": chunk.VisibilityScope,
		"data_class":       chunk.DataClass,
		"policy_version":   chunk.PolicyVersion,
		"chunk_version":    chunk.ChunkVersion,
		"tombstone_status": chunk.TombstoneStatus,
	}
	return insertOutbox(ctx, tx, "evt_"+chunk.ChunkID, chunk.TenantID, "knowledge_chunk", chunk.ChunkID, "knowledge.chunk.ready.v1", string(chunk.TenantID)+":"+chunk.ChunkID, payload)
}

func insertOutbox(
	ctx context.Context,
	tx pgx.Tx,
	eventID string,
	tenantID types.TenantID,
	aggregateType string,
	aggregateID string,
	eventType string,
	partitionKey string,
	payload map[string]any,
) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	_, err = tx.Exec(ctx, `
INSERT INTO knowledge_outbox (
    event_id, tenant_id, aggregate_type, aggregate_id, event_type,
    event_version, partition_key, payload_json, status, available_at, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5,
    1, $6, $7::jsonb, 'PENDING', now(), now(), now()
)
ON CONFLICT (event_id) DO NOTHING
`, eventID, string(tenantID), aggregateType, aggregateID, eventType, partitionKey, string(encoded))
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func selectSourceSQL() string {
	return `
SELECT tenant_id, source_id, idempotency_key, command_hash, source_type,
       source_ref, source_ref_hash, media_object_ref, owner_ref, visibility_scope,
       data_class, content_hash, mime_type, size_bytes, source_version,
       retention_policy_ref, status, correlation_id, causation_id, trace_id,
       created_at, updated_at
FROM knowledge_sources
`
}

func selectJobSQL() string {
	return `
SELECT tenant_id, job_id, idempotency_key, command_hash, source_id,
       source_version, job_type, parser_profile, chunk_profile,
       embedding_policy_ref, vector_policy_ref, requested_by, status,
       retry_count, failure_class, public_error, document_id, chunk_count,
       correlation_id, causation_id, trace_id, created_at, completed_at
FROM knowledge_ingestion_jobs
`
}

func selectChunkSQL() string {
	return `
SELECT tenant_id, chunk_id, document_id, source_id, source_version,
       chunk_index, chunk_hash, chunk_preview_redacted, visibility_scope,
       data_class, policy_version, chunk_version, status, tombstone_status,
       delete_proof_id, embedding_status, vector_status, created_at, updated_at
FROM knowledge_chunks
`
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSource(row scanner) (types.KnowledgeSource, error) {
	var source types.KnowledgeSource
	err := row.Scan(
		&source.TenantID, &source.SourceID, &source.IdempotencyKey, &source.CommandHash,
		&source.SourceType, &source.SourceRef, &source.SourceRefHash, &source.MediaObjectRef,
		&source.OwnerRef, &source.VisibilityScope, &source.DataClass, &source.ContentHash,
		&source.MimeType, &source.SizeBytes, &source.SourceVersion, &source.RetentionPolicyRef,
		&source.Status, &source.CorrelationID, &source.CausationID, &source.TraceID,
		&source.CreatedAt, &source.UpdatedAt,
	)
	return source, err
}

func scanJob(row scanner) (types.KnowledgeIngestionJob, error) {
	var job types.KnowledgeIngestionJob
	var completedAt *time.Time
	err := row.Scan(
		&job.TenantID, &job.JobID, &job.IdempotencyKey, &job.CommandHash, &job.SourceID,
		&job.SourceVersion, &job.JobType, &job.ParserProfile, &job.ChunkProfile,
		&job.EmbeddingPolicyRef, &job.VectorPolicyRef, &job.RequestedBy, &job.Status,
		&job.RetryCount, &job.FailureClass, &job.PublicError, &job.DocumentID, &job.ChunkCount,
		&job.CorrelationID, &job.CausationID, &job.TraceID, &job.CreatedAt, &completedAt,
	)
	if completedAt != nil {
		job.CompletedAt = *completedAt
	}
	return job, err
}

func scanChunk(row scanner) (types.KnowledgeChunk, error) {
	var chunk types.KnowledgeChunk
	err := row.Scan(
		&chunk.TenantID, &chunk.ChunkID, &chunk.DocumentID, &chunk.SourceID, &chunk.SourceVersion,
		&chunk.ChunkIndex, &chunk.ChunkHash, &chunk.ChunkPreviewRedacted, &chunk.VisibilityScope,
		&chunk.DataClass, &chunk.PolicyVersion, &chunk.ChunkVersion, &chunk.Status,
		&chunk.TombstoneStatus, &chunk.DeleteProofID, &chunk.EmbeddingStatus, &chunk.VectorStatus,
		&chunk.CreatedAt, &chunk.UpdatedAt,
	)
	return chunk, err
}
