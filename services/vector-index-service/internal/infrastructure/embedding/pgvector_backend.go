package embedding

import (
	"context"
	"errors"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/infrastructure/pgvector"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

type PGVectorStore interface {
	Upsert(context.Context, pgvector.Item) error
}

type PGVectorBackend struct {
	store PGVectorStore
}

func NewPGVectorBackend(store PGVectorStore) PGVectorBackend {
	return PGVectorBackend{store: store}
}

func (backend PGVectorBackend) UpsertEmbedding(
	ctx context.Context,
	task types.VectorEmbeddingTask,
	result types.VectorEmbeddingResult,
	item types.VectorItem,
) error {
	if backend.store == nil {
		return errors.New("pgvector backend store is not configured")
	}
	if !result.EmbeddingReturned || len(result.EmbeddingValues) == 0 {
		return errors.New("pgvector backend requires returned embedding values")
	}
	if result.Dimension != len(result.EmbeddingValues) {
		return errors.New("pgvector backend embedding dimension mismatch")
	}
	return backend.store.Upsert(ctx, pgvector.Item{
		TenantID:           string(item.TenantID),
		VectorItemID:       item.VectorItemID,
		BackendVectorID:    "pgvector:" + item.VectorItemID,
		CollectionID:       item.CollectionID,
		EmbeddingModelRef:  item.EmbeddingModelRef,
		EmbeddingValues:    append([]float32(nil), result.EmbeddingValues...),
		Dimension:          result.Dimension,
		SourceRefHash:      item.SourceRefHash,
		ChunkHash:          item.ChunkHash,
		VisibilityScope:    item.VisibilityScope,
		VisibilityVersion:  item.VisibilityVersion,
		PolicyVersion:      item.PolicyVersion,
		DataClass:          item.DataClass,
		TombstoneStatus:    item.TombstoneStatus,
		DeleteProofID:      item.DeleteProofID,
		RetentionPolicyRef: item.RetentionPolicyRef,
		CorrelationID:      firstNonEmpty(item.CorrelationID, task.CorrelationID),
		CausationID:        firstNonEmpty(item.CausationID, task.CausationID, result.InvocationID),
		TraceID:            firstNonEmpty(item.TraceID, task.TraceID),
		UpdatedAt:          item.UpdatedAt,
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
