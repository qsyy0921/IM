package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

func TestPrepareUpsertRejectsRawOrUntrustedMetadata(t *testing.T) {
	command := validUpsertCommand()
	command.AuthContext.ServiceName = "rag-service"
	if _, err := PrepareUpsert(command, "vitem_test", "vjob_test", time.Now()); !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected permission denied for untrusted caller, got %v", err)
	}

	command = validUpsertCommand()
	command.SourceRefHash = "http://example.com/raw"
	if _, err := PrepareUpsert(command, "vitem_test", "vjob_test", time.Now()); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument for raw source ref, got %v", err)
	}

	command = validUpsertCommand()
	prepared, err := PrepareUpsert(command, "vitem_test", "vjob_test", time.Now())
	if err != nil {
		t.Fatalf("prepare valid upsert: %v", err)
	}
	if prepared.CollectionID == "" || prepared.CommandHash == "" {
		t.Fatalf("expected collection id and command hash: %+v", prepared)
	}
}

func TestSearchVectorsRequiresRetrievalCaller(t *testing.T) {
	command := types.SearchVectorsCommand{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-vector-test",
			ServiceName: "rag-service",
		},
		RequesterRef:       "retrieval:requester",
		RetrievalRequestID: "retrieval-request-1",
		CollectionTypes:    []string{types.CollectionTypeKnowledgeChunk},
		QueryEmbeddingRef:  "embedding-ref:query-1",
		TopK:               10,
		VisibilityScope:    "conversation:conv-1",
		PolicyVersion:      "policy:v1",
	}
	if err := command.Validate(); !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected permission denied, got %v", err)
	}
	command.AuthContext.ServiceName = types.AllowedCallerRetrieval
	if err := command.Validate(); err != nil {
		t.Fatalf("valid retrieval caller should pass: %v", err)
	}
}

func TestPrepareRebuildRequiresVectorIndexCaller(t *testing.T) {
	command := validRebuildCommand()
	command.AuthContext.ServiceName = types.AllowedCallerKnowledgeIngestion
	if _, err := PrepareRebuild(command, "vjob_rebuild_test", time.Now()); !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected permission denied for non vector-index caller, got %v", err)
	}
	command = validRebuildCommand()
	prepared, err := PrepareRebuild(command, "vjob_rebuild_test", time.Now())
	if err != nil {
		t.Fatalf("prepare valid rebuild: %v", err)
	}
	if prepared.CollectionID == "" || prepared.CommandHash == "" || prepared.JobID != "vjob_rebuild_test" {
		t.Fatalf("unexpected prepared rebuild: %+v", prepared)
	}
}

func validUpsertCommand() types.UpsertVectorItemCommand {
	return types.UpsertVectorItemCommand{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-vector-test",
			ServiceName: types.AllowedCallerKnowledgeIngestion,
		},
		SourceService:       types.AllowedCallerKnowledgeIngestion,
		CollectionType:      types.CollectionTypeKnowledgeChunk,
		SourceRefHash:       "sha256:source-ref",
		SourceID:            "chunk-1",
		SourceVersion:       1,
		SourceHash:          "sha256:source",
		ChunkHash:           "sha256:chunk",
		EmbeddingModelRef:   "model:text-embedding-local",
		EmbeddingVectorHash: "sha256:embedding",
		Dimension:           3,
		VisibilityScope:     "conversation:conv-1",
		VisibilityVersion:   1,
		PolicyVersion:       "policy:v1",
		DataClass:           "LOW",
		IdempotencyKey:      "idem-1",
	}
}

func validRebuildCommand() types.RequestVectorRebuildCommand {
	return types.RequestVectorRebuildCommand{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-vector-domain",
			ServiceName: types.AllowedCallerVectorIndex,
		},
		CollectionType:    types.CollectionTypeKnowledgeChunk,
		EmbeddingModelRef: "model:text-embedding-local",
		Dimension:         3,
		SourceService:     types.AllowedCallerKnowledgeIngestion,
		PartitionKey:      "knowledge-ingestion-service:tenant-vector-domain",
		CursorValue:       "cursor:start",
		IdempotencyKey:    "rebuild-idem-1",
	}
}
