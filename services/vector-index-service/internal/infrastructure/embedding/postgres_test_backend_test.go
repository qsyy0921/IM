package embedding

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

func TestPostgresTestBackendConfirmsActiveBackendState(t *testing.T) {
	store := &fakePostgresBackendStateStore{}
	backend := NewPostgresTestBackend(store)
	item := types.VectorItem{
		TenantID:            "tenant-vector",
		VectorItemID:        "vitem_1",
		BackendVectorID:     "vitem_1",
		EmbeddingVectorHash: "sha256:embedding",
		Dimension:           8,
	}
	result := types.VectorEmbeddingResult{
		EmbeddingVectorHash: "sha256:embedding",
		Dimension:           8,
	}

	if err := backend.UpsertEmbedding(context.Background(), types.VectorEmbeddingTask{}, result, item); err != nil {
		t.Fatalf("confirm backend state: %v", err)
	}
	if len(store.items) != 1 || store.items[0].VectorItemID != "vitem_1" {
		t.Fatalf("backend state store was not called: %+v", store.items)
	}
}

func TestPostgresTestBackendValidatesEmbeddingMetadata(t *testing.T) {
	backend := NewPostgresTestBackend(&fakePostgresBackendStateStore{})
	item := types.VectorItem{
		TenantID:            "tenant-vector",
		VectorItemID:        "vitem_1",
		BackendVectorID:     "vitem_1",
		EmbeddingVectorHash: "sha256:embedding",
		Dimension:           8,
	}

	err := backend.UpsertEmbedding(context.Background(), types.VectorEmbeddingTask{}, types.VectorEmbeddingResult{
		EmbeddingVectorHash: "sha256:other",
		Dimension:           8,
	}, item)
	if err == nil {
		t.Fatal("expected embedding hash mismatch to fail")
	}

	err = backend.UpsertEmbedding(context.Background(), types.VectorEmbeddingTask{}, types.VectorEmbeddingResult{
		EmbeddingVectorHash: "sha256:embedding",
		Dimension:           16,
	}, item)
	if err == nil {
		t.Fatal("expected embedding dimension mismatch to fail")
	}
}

func TestPostgresTestBackendPropagatesStoreError(t *testing.T) {
	expected := errors.New("state missing")
	backend := NewPostgresTestBackend(&fakePostgresBackendStateStore{err: expected})
	err := backend.UpsertEmbedding(context.Background(), types.VectorEmbeddingTask{}, types.VectorEmbeddingResult{
		EmbeddingVectorHash: "sha256:embedding",
		Dimension:           8,
	}, types.VectorItem{
		TenantID:            "tenant-vector",
		VectorItemID:        "vitem_1",
		BackendVectorID:     "vitem_1",
		EmbeddingVectorHash: "sha256:embedding",
		Dimension:           8,
	})
	if !errors.Is(err, expected) {
		t.Fatalf("expected store error, got %v", err)
	}
}

type fakePostgresBackendStateStore struct {
	items []types.VectorItem
	err   error
}

func (store *fakePostgresBackendStateStore) ConfirmActiveBackendItem(_ context.Context, item types.VectorItem) error {
	if store.err != nil {
		return store.err
	}
	store.items = append(store.items, item)
	return nil
}
