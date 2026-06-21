package pgvector

import (
	"math"
	"testing"
)

func TestVectorLiteralFormatsFiniteValues(t *testing.T) {
	literal, err := VectorLiteral([]float32{0.25, -0.5, 1})
	if err != nil {
		t.Fatalf("vector literal: %v", err)
	}
	if literal != "[0.25,-0.5,1]" {
		t.Fatalf("unexpected literal: %s", literal)
	}
}

func TestVectorLiteralRejectsInvalidValues(t *testing.T) {
	for _, values := range [][]float32{
		nil,
		{},
		{float32(math.NaN())},
		{float32(math.Inf(1))},
	} {
		if _, err := VectorLiteral(values); err == nil {
			t.Fatalf("expected invalid vector values to fail: %+v", values)
		}
	}
}

func TestNewStoreRejectsUnsafeTableName(t *testing.T) {
	if _, err := NewStore(nil, "vector_embedding_items;drop table vector_items"); err == nil {
		t.Fatal("expected unsafe table name to fail")
	}
	if store, err := NewStore(nil, "vector_index.vector_embedding_items"); err != nil {
		t.Fatalf("expected schema-qualified safe table name: %v", err)
	} else if store.table != `"vector_index"."vector_embedding_items"` {
		t.Fatalf("unexpected quoted table: %s", store.table)
	}
}

func TestValidateItemRequiresDimensionMatch(t *testing.T) {
	item := validItem()
	item.Dimension = 4
	if err := validateItem(item); err == nil {
		t.Fatal("expected dimension mismatch")
	}
}

func TestValidateItemAcceptsMinimalActiveItem(t *testing.T) {
	item := normalizeItem(validItem())
	if err := validateItem(item); err != nil {
		t.Fatalf("validate item: %v", err)
	}
	if item.Dimension != len(item.EmbeddingValues) {
		t.Fatalf("dimension was not inferred: %+v", item)
	}
	if item.TombstoneStatus != "NONE" {
		t.Fatalf("unexpected tombstone status: %s", item.TombstoneStatus)
	}
}

func validItem() Item {
	return Item{
		TenantID:           "tenant-vector",
		VectorItemID:       "vitem_1",
		BackendVectorID:    "pgvector:vitem_1",
		CollectionID:       "vcol_1",
		EmbeddingModelRef:  "deterministic-embedding-v1",
		EmbeddingValues:    []float32{0.1, 0.2, 0.3},
		SourceRefHash:      "sha256:sourceref",
		ChunkHash:          "sha256:chunk",
		VisibilityScope:    "tenant:tenant-vector",
		VisibilityVersion:  1,
		PolicyVersion:      "policy-v1",
		DataClass:          "BUSINESS_INTERNAL",
		TombstoneStatus:    "none",
		RetentionPolicyRef: "retain-default",
	}
}
