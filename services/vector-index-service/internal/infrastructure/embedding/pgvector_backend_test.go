package embedding

import (
	"context"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/infrastructure/pgvector"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

func TestPGVectorBackendUpsertsEmbeddingValues(t *testing.T) {
	store := &fakePGVectorStore{}
	backend := NewPGVectorBackend(store)
	now := time.Now().UTC()

	err := backend.UpsertEmbedding(
		context.Background(),
		types.VectorEmbeddingTask{
			CorrelationID: "corr-task",
			CausationID:   "cause-task",
			TraceID:       "trace-task",
		},
		types.VectorEmbeddingResult{
			InvocationID:        "minv_1",
			EmbeddingValues:     []float32{0.1, 0.2, 0.3},
			EmbeddingVectorHash: "sha256:embedding",
			Dimension:           3,
			EmbeddingReturned:   true,
		},
		types.VectorItem{
			TenantID:           "tenant-vector",
			VectorItemID:       "vitem_1",
			CollectionID:       "vcol_1",
			EmbeddingModelRef:  "deterministic-embedding-v1",
			SourceRefHash:      "sha256:sourceref",
			ChunkHash:          "sha256:chunk",
			VisibilityScope:    "tenant:tenant-vector",
			VisibilityVersion:  1,
			PolicyVersion:      "policy-v1",
			DataClass:          "BUSINESS_INTERNAL",
			TombstoneStatus:    types.TombstoneStatusNone,
			RetentionPolicyRef: "retain-default",
			UpdatedAt:          now,
		},
	)
	if err != nil {
		t.Fatalf("upsert embedding: %v", err)
	}
	if len(store.items) != 1 {
		t.Fatalf("expected one item, got %d", len(store.items))
	}
	item := store.items[0]
	if item.BackendVectorID != "pgvector:vitem_1" || item.VectorItemID != "vitem_1" {
		t.Fatalf("unexpected ids: %+v", item)
	}
	if len(item.EmbeddingValues) != 3 || item.Dimension != 3 {
		t.Fatalf("embedding values were not handed off: %+v", item)
	}
	if item.CorrelationID != "corr-task" || item.CausationID != "cause-task" || item.TraceID != "trace-task" {
		t.Fatalf("recovery trace metadata missing: %+v", item)
	}
}

func TestPGVectorBackendRequiresReturnedEmbeddingValues(t *testing.T) {
	backend := NewPGVectorBackend(&fakePGVectorStore{})
	err := backend.UpsertEmbedding(context.Background(), types.VectorEmbeddingTask{}, types.VectorEmbeddingResult{
		EmbeddingVectorHash: "sha256:embedding",
		Dimension:           3,
		EmbeddingReturned:   false,
	}, types.VectorItem{VectorItemID: "vitem_1"})
	if err == nil {
		t.Fatal("expected returned embedding values to be required")
	}
}

type fakePGVectorStore struct {
	items []pgvector.Item
	err   error
}

func (store *fakePGVectorStore) Upsert(_ context.Context, item pgvector.Item) error {
	if store.err != nil {
		return store.err
	}
	store.items = append(store.items, item)
	return nil
}
