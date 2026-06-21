package embedding

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

func TestWorkerRunOnceEmbedsAndUpsertsHashOnlyMetadata(t *testing.T) {
	input := "redacted chunk text for local embedding worker"
	task := testEmbeddingTask(input)
	source := &fakeSource{tasks: []types.VectorEmbeddingTask{task}}
	embedder := &fakeEmbedder{
		result: types.VectorEmbeddingResult{
			InvocationID:        "minv_embed_1",
			ModelID:             "deterministic-embedding-v1",
			EmbeddingValues:     []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8},
			EmbeddingVectorHash: "sha256:embeddinghash",
			Dimension:           8,
			EmbeddingReturned:   true,
		},
	}
	upserter := &fakeUpserter{}
	worker := NewWorker(source, embedder, upserter, Config{BatchSize: 10})

	stats, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats.Claimed != 1 || stats.Embedded != 1 || stats.Upserted != 1 || stats.Completed != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(embedder.tasks) != 1 || embedder.tasks[0].InputText != input {
		t.Fatalf("embedder did not receive in-memory input text: %+v", embedder.tasks)
	}
	if len(upserter.commands) != 1 {
		t.Fatalf("expected one upsert command, got %d", len(upserter.commands))
	}
	command := upserter.commands[0]
	if command.EmbeddingVectorHash != "sha256:embeddinghash" || command.EmbeddingModelRef != "deterministic-embedding-v1" {
		t.Fatalf("unexpected embedding metadata: %+v", command)
	}
	if command.SourceID == input || command.SourceRefHash == input || command.ChunkHash == input {
		t.Fatalf("upsert command leaked raw input into refs: %+v", command)
	}
	if len(source.completed) != 1 || source.completed[0].IdempotencyKey != task.IdempotencyKey {
		t.Fatalf("task was not completed: %+v", source.completed)
	}
}

func TestWorkerRunOnceUpsertsOptionalBackend(t *testing.T) {
	input := "redacted chunk text for backend handoff"
	task := testEmbeddingTask(input)
	source := &fakeSource{tasks: []types.VectorEmbeddingTask{task}}
	embedder := &fakeEmbedder{
		result: types.VectorEmbeddingResult{
			InvocationID:        "minv_embed_1",
			ModelID:             "deterministic-embedding-v1",
			EmbeddingValues:     []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8},
			EmbeddingVectorHash: "sha256:embeddinghash",
			Dimension:           8,
			EmbeddingReturned:   true,
		},
	}
	backend := &fakeBackend{}
	worker := NewWorker(source, embedder, &fakeUpserter{}, Config{BatchSize: 10, Backend: backend})

	stats, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats.Upserted != 1 || stats.Completed != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(backend.calls) != 1 {
		t.Fatalf("expected one backend call, got %d", len(backend.calls))
	}
	call := backend.calls[0]
	if call.item.VectorItemID != "vitem_fake" || call.result.EmbeddingVectorHash != "sha256:embeddinghash" {
		t.Fatalf("unexpected backend handoff: %+v", call)
	}
	if len(call.result.EmbeddingValues) != 8 || call.task.InputText != input {
		t.Fatalf("backend did not receive embedding values and worker task: %+v", call)
	}
}

func TestWorkerRunOnceRejectsInputHashMismatch(t *testing.T) {
	task := testEmbeddingTask("one value")
	task.InputHash = sha256Ref("different value")
	worker := NewWorker(
		&fakeSource{tasks: []types.VectorEmbeddingTask{task}},
		&fakeEmbedder{},
		&fakeUpserter{},
		Config{},
	)

	stats, err := worker.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected hash mismatch")
	}
	if stats.Claimed != 1 || stats.Embedded != 0 || stats.Upserted != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestWorkerRunOnceDoesNotUpsertWhenEmbeddingFails(t *testing.T) {
	task := testEmbeddingTask("embed failure input")
	upserter := &fakeUpserter{}
	worker := NewWorker(
		&fakeSource{tasks: []types.VectorEmbeddingTask{task}},
		&fakeEmbedder{err: errors.New("provider failed")},
		upserter,
		Config{},
	)

	stats, err := worker.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected embedder error")
	}
	if stats.Claimed != 1 || stats.Embedded != 0 || stats.Upserted != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(upserter.commands) != 0 {
		t.Fatalf("upsert should not be called after embedder failure: %+v", upserter.commands)
	}
}

func TestWorkerRunOnceRequiresDependencies(t *testing.T) {
	worker := NewWorker(nil, nil, nil, Config{})
	if _, err := worker.RunOnce(context.Background()); err == nil {
		t.Fatal("expected missing dependency error")
	}
}

func testEmbeddingTask(input string) types.VectorEmbeddingTask {
	return types.VectorEmbeddingTask{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-vector",
			ServiceName: types.AllowedCallerVectorIndex,
			RequestID:   "req-vector-embed",
		},
		SourceService:      types.AllowedCallerKnowledgeIngestion,
		CollectionType:     types.CollectionTypeKnowledgeChunk,
		SourceRefHash:      "sha256:sourceref",
		SourceID:           "ksrc_1:kchunk_1",
		SourceVersion:      1,
		SourceHash:         "sha256:sourcehash",
		ChunkHash:          "sha256:chunkhash",
		InputText:          input,
		InputHash:          sha256Ref(input),
		InputSchemaVersion: 1,
		EmbeddingModelRef:  "deterministic-embedding-v1",
		Dimension:          8,
		VisibilityScope:    "tenant:tenant-vector",
		VisibilityVersion:  1,
		PolicyVersion:      "policy-v1",
		DataClass:          "BUSINESS_INTERNAL",
		IdempotencyKey:     "idem-vector-embed",
		CorrelationID:      "corr-vector",
		TraceID:            "trace-vector",
	}
}

type fakeSource struct {
	tasks     []types.VectorEmbeddingTask
	completed []types.VectorEmbeddingTask
}

func (source *fakeSource) ClaimEmbeddingTasks(_ context.Context, limit int) ([]types.VectorEmbeddingTask, error) {
	if limit < len(source.tasks) {
		return source.tasks[:limit], nil
	}
	return source.tasks, nil
}

func (source *fakeSource) CompleteEmbeddingTask(_ context.Context, task types.VectorEmbeddingTask) error {
	source.completed = append(source.completed, task)
	return nil
}

type fakeEmbedder struct {
	result types.VectorEmbeddingResult
	tasks  []types.VectorEmbeddingTask
	err    error
}

func (embedder *fakeEmbedder) Embed(_ context.Context, task types.VectorEmbeddingTask) (types.VectorEmbeddingResult, error) {
	if embedder.err != nil {
		return types.VectorEmbeddingResult{}, embedder.err
	}
	embedder.tasks = append(embedder.tasks, task)
	return embedder.result, nil
}

type fakeUpserter struct {
	commands []types.UpsertVectorItemCommand
	err      error
}

func (upserter *fakeUpserter) UpsertVectorItem(_ context.Context, command types.UpsertVectorItemCommand) (types.VectorItem, bool, error) {
	if upserter.err != nil {
		return types.VectorItem{}, false, upserter.err
	}
	upserter.commands = append(upserter.commands, command)
	return types.VectorItem{
		TenantID:          command.AuthContext.TenantID,
		VectorItemID:      "vitem_fake",
		CollectionID:      "vcol_fake",
		CollectionType:    command.CollectionType,
		BackendVectorID:   "vitem_fake",
		Dimension:         command.Dimension,
		VisibilityScope:   command.VisibilityScope,
		VisibilityVersion: command.VisibilityVersion,
		PolicyVersion:     command.PolicyVersion,
		DataClass:         command.DataClass,
	}, false, nil
}

type backendCall struct {
	task   types.VectorEmbeddingTask
	result types.VectorEmbeddingResult
	item   types.VectorItem
}

type fakeBackend struct {
	calls []backendCall
	err   error
}

func (backend *fakeBackend) UpsertEmbedding(
	_ context.Context,
	task types.VectorEmbeddingTask,
	result types.VectorEmbeddingResult,
	item types.VectorItem,
) error {
	if backend.err != nil {
		return backend.err
	}
	backend.calls = append(backend.calls, backendCall{task: task, result: result, item: item})
	return nil
}
