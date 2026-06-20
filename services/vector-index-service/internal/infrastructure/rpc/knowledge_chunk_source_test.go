package rpc

import (
	"context"
	"testing"

	knowledgev1 "github.com/qsyy0921/IM/api/proto/nexusim/knowledge/v1"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
	"google.golang.org/grpc"
)

func TestKnowledgeChunkTaskSourceClaimsRedactedPreviewTasks(t *testing.T) {
	client := &fakeKnowledgeClient{
		pages: []*knowledgev1.ListKnowledgeChunksResponse{
			{
				Chunks: []*knowledgev1.KnowledgeChunk{
					{
						TenantId:             "tenant-vector",
						ChunkId:              "kchunk_1",
						SourceId:             "ksrc_1",
						DocumentId:           "kdoc_1",
						ChunkIndex:           0,
						ChunkHash:            "sha256:chunkhash",
						ChunkPreviewRedacted: "redacted preview for embedding",
						SourceVersion:        "2",
						VisibilityScope:      "tenant:tenant-vector",
						DataClass:            "BUSINESS_INTERNAL",
						ChunkVersion:         "chunk-v1",
						Status:               "READY",
						TombstoneStatus:      "ACTIVE",
						PolicyVersion:        "policy-v1",
						UpdatedAtUnixMs:      1234,
					},
				},
			},
		},
	}
	source, err := NewKnowledgeChunkTaskSource(client, KnowledgeChunkSourceConfig{
		TenantID:          "tenant-vector",
		SourceID:          "ksrc_1",
		EmbeddingModelRef: "deterministic-embedding-v1",
		Dimension:         8,
		TraceID:           "trace-vector",
	}, 0)
	if err != nil {
		t.Fatalf("new source: %v", err)
	}

	tasks, err := source.ClaimEmbeddingTasks(context.Background(), 10)
	if err != nil {
		t.Fatalf("claim tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one task, got %d", len(tasks))
	}
	task := tasks[0]
	if task.AuthContext.ServiceName != types.AllowedCallerVectorIndex || task.SourceService != types.AllowedCallerKnowledgeIngestion {
		t.Fatalf("unexpected service refs: %+v", task)
	}
	if task.InputText != "redacted preview for embedding" || task.InputHash != sha256Ref(task.InputText) {
		t.Fatalf("unexpected input mapping: %+v", task)
	}
	if task.SourceVersion != 2 || task.VisibilityVersion != 1 {
		t.Fatalf("unexpected version mapping: %+v", task)
	}
	if task.SourceRefHash == task.SourceID || task.SourceHash == task.SourceID {
		t.Fatalf("source hashes should not expose raw source id: %+v", task)
	}

	if err := source.CompleteEmbeddingTask(context.Background(), task); err != nil {
		t.Fatalf("complete task: %v", err)
	}
	tasks, err = source.ClaimEmbeddingTasks(context.Background(), 10)
	if err != nil {
		t.Fatalf("claim after complete: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("completed or drained source should not claim more tasks: %+v", tasks)
	}
}

func TestKnowledgeChunkTaskSourceSkipsChunksWithoutPreview(t *testing.T) {
	client := &fakeKnowledgeClient{
		pages: []*knowledgev1.ListKnowledgeChunksResponse{
			{Chunks: []*knowledgev1.KnowledgeChunk{{ChunkId: "kchunk_empty", SourceId: "ksrc_1", DocumentId: "kdoc_1", ChunkHash: "sha256:chunk"}}},
		},
	}
	source, err := NewKnowledgeChunkTaskSource(client, KnowledgeChunkSourceConfig{
		TenantID:   "tenant-vector",
		DocumentID: "kdoc_1",
	}, 0)
	if err != nil {
		t.Fatalf("new source: %v", err)
	}
	tasks, err := source.ClaimEmbeddingTasks(context.Background(), 10)
	if err != nil {
		t.Fatalf("claim tasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("empty preview chunk should not produce task: %+v", tasks)
	}
}

type fakeKnowledgeClient struct {
	knowledgev1.KnowledgeIngestionServiceClient
	pages []*knowledgev1.ListKnowledgeChunksResponse
	calls int
}

func (client *fakeKnowledgeClient) ListKnowledgeChunks(
	_ context.Context,
	request *knowledgev1.ListKnowledgeChunksRequest,
	_ ...grpc.CallOption,
) (*knowledgev1.ListKnowledgeChunksResponse, error) {
	client.calls++
	if request.GetAuthContext().GetServiceName() != types.AllowedCallerVectorIndex {
		return nil, nil
	}
	if client.calls > len(client.pages) {
		return &knowledgev1.ListKnowledgeChunksResponse{}, nil
	}
	return client.pages[client.calls-1], nil
}
