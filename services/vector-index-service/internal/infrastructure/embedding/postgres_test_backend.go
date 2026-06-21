package embedding

import (
	"context"
	"errors"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

type PostgresTestBackendStateStore interface {
	ConfirmActiveBackendItem(ctx context.Context, item types.VectorItem) error
}

type PostgresTestBackend struct {
	store PostgresTestBackendStateStore
}

func NewPostgresTestBackend(store PostgresTestBackendStateStore) PostgresTestBackend {
	return PostgresTestBackend{store: store}
}

func (backend PostgresTestBackend) UpsertEmbedding(
	ctx context.Context,
	_ types.VectorEmbeddingTask,
	result types.VectorEmbeddingResult,
	item types.VectorItem,
) error {
	if backend.store == nil {
		return errors.New("postgres-test backend state store is not configured")
	}
	if item.BackendVectorID == "" {
		return errors.New("postgres-test backend requires backend vector id")
	}
	if result.Dimension != item.Dimension {
		return errors.New("postgres-test backend embedding dimension mismatch")
	}
	if result.EmbeddingVectorHash != "" && result.EmbeddingVectorHash != item.EmbeddingVectorHash {
		return errors.New("postgres-test backend embedding hash mismatch")
	}
	return backend.store.ConfirmActiveBackendItem(ctx, item)
}
