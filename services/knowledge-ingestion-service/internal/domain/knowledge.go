package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/knowledge-ingestion-service/internal/types"
)

type PreparedKnowledgeSource struct {
	Command     types.CreateKnowledgeSourceCommand
	CommandHash string
	SourceID    string
	CreatedAt   time.Time
}

type PreparedIngestionJob struct {
	Command       types.SubmitIngestionJobCommand
	CommandHash   string
	JobID         string
	DocumentID    string
	ChunkIDs      []string
	CreatedAt     time.Time
	CompletedAt   time.Time
	InitialStatus string
}

func PrepareKnowledgeSource(
	command types.CreateKnowledgeSourceCommand,
	sourceID string,
	now time.Time,
) (PreparedKnowledgeSource, error) {
	normalized := command.Normalized()
	if normalized.SourceURIHash == "" {
		normalized.SourceURIHash = HashRef(normalized.SourceRef)
	}
	if err := normalized.Validate(); err != nil {
		return PreparedKnowledgeSource{}, err
	}
	hash, err := sourceCommandHash(normalized)
	if err != nil {
		return PreparedKnowledgeSource{}, err
	}
	return PreparedKnowledgeSource{
		Command:     normalized,
		CommandHash: hash,
		SourceID:    strings.TrimSpace(sourceID),
		CreatedAt:   now.UTC(),
	}, nil
}

func SourceFromPrepared(prepared PreparedKnowledgeSource) types.KnowledgeSource {
	command := prepared.Command
	return types.KnowledgeSource{
		TenantID:           command.AuthContext.TenantID,
		SourceID:           prepared.SourceID,
		IdempotencyKey:     command.IdempotencyKey,
		CommandHash:        prepared.CommandHash,
		SourceType:         command.SourceType,
		SourceRef:          command.SourceRef,
		SourceRefHash:      command.SourceURIHash,
		MediaObjectRef:     command.MediaObjectRef,
		OwnerRef:           command.OwnerRef,
		VisibilityScope:    command.VisibilityScope,
		DataClass:          command.DataClass,
		ContentHash:        command.ContentHash,
		MimeType:           command.MimeType,
		SizeBytes:          command.SizeBytes,
		SourceVersion:      command.SourceVersion,
		RetentionPolicyRef: command.RetentionPolicyRef,
		Status:             types.SourceStatusActive,
		CorrelationID:      command.CorrelationID,
		CausationID:        command.CausationID,
		TraceID:            command.TraceID,
		CreatedAt:          prepared.CreatedAt,
		UpdatedAt:          prepared.CreatedAt,
	}
}

func PrepareIngestionJob(
	command types.SubmitIngestionJobCommand,
	jobID string,
	documentID string,
	chunkIDs []string,
	now time.Time,
) (PreparedIngestionJob, error) {
	normalized := command.Normalized()
	if err := normalized.Validate(); err != nil {
		return PreparedIngestionJob{}, err
	}
	if len(normalized.Chunks) > 0 && strings.TrimSpace(normalized.DocumentHash) == "" {
		return PreparedIngestionJob{}, types.NewInvalidArgument("document_hash is required with chunk manifest")
	}
	hash, err := jobCommandHash(normalized)
	if err != nil {
		return PreparedIngestionJob{}, err
	}
	status := types.JobStatusPending
	var completedAt time.Time
	if len(normalized.Chunks) > 0 {
		status = types.JobStatusDone
		completedAt = now.UTC()
	}
	return PreparedIngestionJob{
		Command:       normalized,
		CommandHash:   hash,
		JobID:         strings.TrimSpace(jobID),
		DocumentID:    strings.TrimSpace(documentID),
		ChunkIDs:      append([]string(nil), chunkIDs...),
		CreatedAt:     now.UTC(),
		CompletedAt:   completedAt,
		InitialStatus: status,
	}, nil
}

func JobFromPrepared(prepared PreparedIngestionJob) types.KnowledgeIngestionJob {
	command := prepared.Command
	return types.KnowledgeIngestionJob{
		TenantID:           command.AuthContext.TenantID,
		JobID:              prepared.JobID,
		IdempotencyKey:     command.IdempotencyKey,
		CommandHash:        prepared.CommandHash,
		SourceID:           command.SourceID,
		SourceVersion:      command.SourceVersion,
		JobType:            command.JobType,
		ParserProfile:      command.ParserProfile,
		ChunkProfile:       command.ChunkProfile,
		EmbeddingPolicyRef: command.EmbeddingPolicyRef,
		VectorPolicyRef:    command.VectorPolicyRef,
		RequestedBy:        command.RequestedBy,
		Status:             prepared.InitialStatus,
		DocumentID:         prepared.DocumentID,
		ChunkCount:         len(command.Chunks),
		CorrelationID:      command.CorrelationID,
		CausationID:        command.CausationID,
		TraceID:            command.TraceID,
		CreatedAt:          prepared.CreatedAt,
		CompletedAt:        prepared.CompletedAt,
	}
}

func DocumentFromPrepared(prepared PreparedIngestionJob) types.KnowledgeDocument {
	command := prepared.Command
	return types.KnowledgeDocument{
		TenantID:      command.AuthContext.TenantID,
		DocumentID:    prepared.DocumentID,
		SourceID:      command.SourceID,
		SourceVersion: command.SourceVersion,
		ParserProfile: command.ParserProfile,
		MimeType:      command.MimeType,
		SizeBytes:     command.SizeBytes,
		PageCount:     command.PageCount,
		Language:      command.Language,
		DocumentHash:  command.DocumentHash,
		ParseStatus:   "PARSED",
		CreatedAt:     prepared.CreatedAt,
	}
}

func ChunksFromPrepared(prepared PreparedIngestionJob) []types.KnowledgeChunk {
	command := prepared.Command
	chunks := make([]types.KnowledgeChunk, 0, len(command.Chunks))
	for index, item := range command.Chunks {
		chunkID := ""
		if index < len(prepared.ChunkIDs) {
			chunkID = prepared.ChunkIDs[index]
		}
		chunks = append(chunks, types.KnowledgeChunk{
			TenantID:             command.AuthContext.TenantID,
			ChunkID:              chunkID,
			DocumentID:           prepared.DocumentID,
			SourceID:             command.SourceID,
			SourceVersion:        command.SourceVersion,
			ChunkIndex:           index,
			ChunkHash:            item.ChunkHash,
			ChunkPreviewRedacted: item.ChunkPreviewRedacted,
			VisibilityScope:      item.VisibilityScope,
			DataClass:            item.DataClass,
			PolicyVersion:        item.PolicyVersion,
			ChunkVersion:         item.ChunkVersion,
			Status:               types.ChunkStatusReady,
			TombstoneStatus:      types.TombstoneStatusActive,
			EmbeddingStatus:      types.EmbeddingStatusPending,
			VectorStatus:         types.VectorStatusPending,
			CreatedAt:            prepared.CreatedAt,
			UpdatedAt:            prepared.CreatedAt,
		})
	}
	return chunks
}

func HashRef(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func sourceCommandHash(command types.CreateKnowledgeSourceCommand) (string, error) {
	payload := map[string]any{
		"tenant_id":            string(command.AuthContext.TenantID),
		"source_type":          command.SourceType,
		"source_ref_hash":      command.SourceURIHash,
		"media_object_ref":     command.MediaObjectRef,
		"owner_ref":            command.OwnerRef,
		"visibility_scope":     command.VisibilityScope,
		"data_class":           command.DataClass,
		"content_hash":         command.ContentHash,
		"mime_type":            command.MimeType,
		"size_bytes":           command.SizeBytes,
		"source_version":       command.SourceVersion,
		"retention_policy_ref": command.RetentionPolicyRef,
		"idempotency_key":      command.IdempotencyKey,
		"correlation_id":       command.CorrelationID,
		"causation_id":         command.CausationID,
		"trace_id":             command.TraceID,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", types.NewInvalidArgument("knowledge source command hash payload invalid")
	}
	return HashRef(string(encoded)), nil
}

func jobCommandHash(command types.SubmitIngestionJobCommand) (string, error) {
	payload := map[string]any{
		"tenant_id":            string(command.AuthContext.TenantID),
		"source_id":            command.SourceID,
		"source_version":       command.SourceVersion,
		"job_type":             command.JobType,
		"parser_profile":       command.ParserProfile,
		"chunk_profile":        command.ChunkProfile,
		"embedding_policy_ref": command.EmbeddingPolicyRef,
		"vector_policy_ref":    command.VectorPolicyRef,
		"requested_by":         command.RequestedBy,
		"idempotency_key":      command.IdempotencyKey,
		"document_hash":        command.DocumentHash,
		"mime_type":            command.MimeType,
		"size_bytes":           command.SizeBytes,
		"page_count":           command.PageCount,
		"language":             command.Language,
		"chunks":               command.Chunks,
		"correlation_id":       command.CorrelationID,
		"causation_id":         command.CausationID,
		"trace_id":             command.TraceID,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", types.NewInvalidArgument("knowledge job command hash payload invalid")
	}
	return HashRef(string(encoded)), nil
}
