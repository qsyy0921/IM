package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

type PreparedUpsert struct {
	Command      types.UpsertVectorItemCommand
	CommandHash  string
	CollectionID string
	VectorItemID string
	JobID        string
	CreatedAt    time.Time
}

type PreparedTombstone struct {
	Command     types.TombstoneVectorItemCommand
	CommandHash string
	TombstoneID string
	JobID       string
	CreatedAt   time.Time
}

type PreparedRebuild struct {
	Command      types.RequestVectorRebuildCommand
	CommandHash  string
	CollectionID string
	JobID        string
	CreatedAt    time.Time
}

func PrepareUpsert(
	command types.UpsertVectorItemCommand,
	vectorItemID string,
	jobID string,
	now time.Time,
) (PreparedUpsert, error) {
	normalized := command.Normalized()
	if err := normalized.Validate(); err != nil {
		return PreparedUpsert{}, err
	}
	hash, err := upsertCommandHash(normalized)
	if err != nil {
		return PreparedUpsert{}, err
	}
	return PreparedUpsert{
		Command:      normalized,
		CommandHash:  hash,
		CollectionID: collectionID(normalized),
		VectorItemID: strings.TrimSpace(vectorItemID),
		JobID:        strings.TrimSpace(jobID),
		CreatedAt:    now.UTC(),
	}, nil
}

func PrepareTombstone(
	command types.TombstoneVectorItemCommand,
	tombstoneID string,
	jobID string,
	now time.Time,
) (PreparedTombstone, error) {
	normalized := command.Normalized()
	if err := normalized.Validate(); err != nil {
		return PreparedTombstone{}, err
	}
	hash, err := tombstoneCommandHash(normalized)
	if err != nil {
		return PreparedTombstone{}, err
	}
	return PreparedTombstone{
		Command:     normalized,
		CommandHash: hash,
		TombstoneID: strings.TrimSpace(tombstoneID),
		JobID:       strings.TrimSpace(jobID),
		CreatedAt:   now.UTC(),
	}, nil
}

func PrepareRebuild(
	command types.RequestVectorRebuildCommand,
	jobID string,
	now time.Time,
) (PreparedRebuild, error) {
	normalized := command.Normalized()
	if err := normalized.Validate(); err != nil {
		return PreparedRebuild{}, err
	}
	hash, err := rebuildCommandHash(normalized)
	if err != nil {
		return PreparedRebuild{}, err
	}
	return PreparedRebuild{
		Command:      normalized,
		CommandHash:  hash,
		CollectionID: CollectionID(normalized.AuthContext.TenantID, normalized.CollectionType, normalized.EmbeddingModelRef, normalized.Dimension),
		JobID:        strings.TrimSpace(jobID),
		CreatedAt:    now.UTC(),
	}, nil
}

func CollectionFromPrepared(prepared PreparedUpsert) types.VectorCollection {
	command := prepared.Command
	return types.VectorCollection{
		TenantID:              command.AuthContext.TenantID,
		CollectionID:          prepared.CollectionID,
		CollectionType:        command.CollectionType,
		BackendType:           types.BackendTypePostgresTest,
		Dimension:             command.Dimension,
		EmbeddingModelRef:     command.EmbeddingModelRef,
		Status:                types.CollectionStatusActive,
		MetadataSchemaVersion: 1,
		CreatedAt:             prepared.CreatedAt,
		UpdatedAt:             prepared.CreatedAt,
	}
}

func ItemFromPrepared(prepared PreparedUpsert) types.VectorItem {
	command := prepared.Command
	return types.VectorItem{
		TenantID:            command.AuthContext.TenantID,
		VectorItemID:        prepared.VectorItemID,
		CollectionID:        prepared.CollectionID,
		CollectionType:      command.CollectionType,
		SourceService:       command.SourceService,
		SourceRefHash:       command.SourceRefHash,
		SourceID:            command.SourceID,
		SourceVersion:       command.SourceVersion,
		SourceHash:          command.SourceHash,
		ChunkHash:           command.ChunkHash,
		EmbeddingModelRef:   command.EmbeddingModelRef,
		EmbeddingVectorHash: command.EmbeddingVectorHash,
		Dimension:           command.Dimension,
		VisibilityScope:     command.VisibilityScope,
		VisibilityVersion:   command.VisibilityVersion,
		PolicyVersion:       command.PolicyVersion,
		DataClass:           command.DataClass,
		TombstoneStatus:     types.TombstoneStatusNone,
		DeleteProofID:       command.DeleteProofID,
		RetentionPolicyRef:  command.RetentionPolicyRef,
		Status:              types.VectorItemStatusIndexed,
		IdempotencyKey:      command.IdempotencyKey,
		CommandHash:         prepared.CommandHash,
		CorrelationID:       command.CorrelationID,
		CausationID:         command.CausationID,
		TraceID:             command.TraceID,
		CreatedAt:           prepared.CreatedAt,
		UpdatedAt:           prepared.CreatedAt,
	}
}

func UpsertJobFromPrepared(prepared PreparedUpsert) types.VectorIndexJob {
	return types.VectorIndexJob{
		TenantID:       prepared.Command.AuthContext.TenantID,
		JobID:          prepared.JobID,
		CollectionID:   prepared.CollectionID,
		VectorItemID:   prepared.VectorItemID,
		JobType:        types.JobTypeUpsert,
		Status:         types.JobStatusIndexed,
		IdempotencyKey: prepared.Command.IdempotencyKey,
		CommandHash:    prepared.CommandHash,
		CorrelationID:  prepared.Command.CorrelationID,
		CausationID:    prepared.Command.CausationID,
		TraceID:        prepared.Command.TraceID,
		CreatedAt:      prepared.CreatedAt,
		CompletedAt:    prepared.CreatedAt,
	}
}

func TombstoneFromPrepared(prepared PreparedTombstone, item types.VectorItem) types.VectorTombstone {
	return types.VectorTombstone{
		TenantID:            item.TenantID,
		TombstoneID:         prepared.TombstoneID,
		VectorItemID:        item.VectorItemID,
		SourceRefHash:       item.SourceRefHash,
		DeleteProofID:       prepared.Command.DeleteProofID,
		ReasonClass:         prepared.Command.ReasonClass,
		BackendDeleteStatus: "DELETED",
		IdempotencyKey:      prepared.Command.IdempotencyKey,
		CommandHash:         prepared.CommandHash,
		CreatedAt:           prepared.CreatedAt,
	}
}

func TombstoneJobFromPrepared(prepared PreparedTombstone, item types.VectorItem) types.VectorIndexJob {
	return types.VectorIndexJob{
		TenantID:       item.TenantID,
		JobID:          prepared.JobID,
		CollectionID:   item.CollectionID,
		VectorItemID:   item.VectorItemID,
		JobType:        types.JobTypeTombstone,
		Status:         types.JobStatusTombstoned,
		IdempotencyKey: prepared.Command.IdempotencyKey,
		CommandHash:    prepared.CommandHash,
		CorrelationID:  prepared.Command.CorrelationID,
		CausationID:    prepared.Command.CausationID,
		TraceID:        prepared.Command.TraceID,
		CreatedAt:      prepared.CreatedAt,
		CompletedAt:    prepared.CreatedAt,
	}
}

func RebuildJobFromPrepared(prepared PreparedRebuild) types.VectorIndexJob {
	return types.VectorIndexJob{
		TenantID:       prepared.Command.AuthContext.TenantID,
		JobID:          prepared.JobID,
		CollectionID:   prepared.CollectionID,
		VectorItemID:   prepared.CollectionID,
		JobType:        types.JobTypeRebuild,
		Status:         types.JobStatusPending,
		IdempotencyKey: prepared.Command.IdempotencyKey,
		CommandHash:    prepared.CommandHash,
		CorrelationID:  prepared.Command.CorrelationID,
		CausationID:    prepared.Command.CausationID,
		TraceID:        prepared.Command.TraceID,
		CreatedAt:      prepared.CreatedAt,
	}
}

func RebuildCheckpointFromPrepared(prepared PreparedRebuild) types.VectorRebuildCheckpoint {
	return types.VectorRebuildCheckpoint{
		TenantID:      prepared.Command.AuthContext.TenantID,
		RebuildJobID:  prepared.JobID,
		CollectionID:  prepared.CollectionID,
		SourceService: prepared.Command.SourceService,
		PartitionKey:  prepared.Command.PartitionKey,
		CursorValue:   prepared.Command.CursorValue,
		Status:        types.RebuildCheckpointStatusPending,
		UpdatedAt:     prepared.CreatedAt,
	}
}

func HashRef(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func collectionID(command types.UpsertVectorItemCommand) string {
	return CollectionID(command.AuthContext.TenantID, command.CollectionType, command.EmbeddingModelRef, command.Dimension)
}

func CollectionID(tenantID types.TenantID, collectionType string, embeddingModelRef string, dimension int) string {
	return "vc_" + strings.TrimPrefix(HashRef(string(tenantID)+"|"+collectionType+"|"+embeddingModelRef+"|"+strconv.Itoa(dimension)), "sha256:")[:24]
}

func upsertCommandHash(command types.UpsertVectorItemCommand) (string, error) {
	payload := map[string]any{
		"tenant_id":             string(command.AuthContext.TenantID),
		"source_service":        command.SourceService,
		"collection_type":       command.CollectionType,
		"source_ref_hash":       command.SourceRefHash,
		"source_id":             command.SourceID,
		"source_version":        command.SourceVersion,
		"source_hash":           command.SourceHash,
		"chunk_hash":            command.ChunkHash,
		"embedding_model_ref":   command.EmbeddingModelRef,
		"embedding_vector_hash": command.EmbeddingVectorHash,
		"dimension":             command.Dimension,
		"visibility_scope":      command.VisibilityScope,
		"visibility_version":    command.VisibilityVersion,
		"policy_version":        command.PolicyVersion,
		"data_class":            command.DataClass,
		"delete_proof_id":       command.DeleteProofID,
		"retention_policy_ref":  command.RetentionPolicyRef,
		"idempotency_key":       command.IdempotencyKey,
		"correlation_id":        command.CorrelationID,
		"causation_id":          command.CausationID,
		"trace_id":              command.TraceID,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", types.NewInvalidArgument("vector upsert command hash payload invalid")
	}
	return HashRef(string(encoded)), nil
}

func tombstoneCommandHash(command types.TombstoneVectorItemCommand) (string, error) {
	payload := map[string]any{
		"tenant_id":       string(command.AuthContext.TenantID),
		"vector_item_id":  command.VectorItemID,
		"delete_proof_id": command.DeleteProofID,
		"reason_class":    command.ReasonClass,
		"idempotency_key": command.IdempotencyKey,
		"correlation_id":  command.CorrelationID,
		"causation_id":    command.CausationID,
		"trace_id":        command.TraceID,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", types.NewInvalidArgument("vector tombstone command hash payload invalid")
	}
	return HashRef(string(encoded)), nil
}

func rebuildCommandHash(command types.RequestVectorRebuildCommand) (string, error) {
	payload := map[string]any{
		"tenant_id":           string(command.AuthContext.TenantID),
		"collection_type":     command.CollectionType,
		"embedding_model_ref": command.EmbeddingModelRef,
		"dimension":           command.Dimension,
		"source_service":      command.SourceService,
		"partition_key":       command.PartitionKey,
		"cursor_value":        command.CursorValue,
		"idempotency_key":     command.IdempotencyKey,
		"correlation_id":      command.CorrelationID,
		"causation_id":        command.CausationID,
		"trace_id":            command.TraceID,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", types.NewInvalidArgument("vector rebuild command hash payload invalid")
	}
	return HashRef(string(encoded)), nil
}
