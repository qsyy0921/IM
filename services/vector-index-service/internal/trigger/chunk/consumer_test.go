package chunk

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

func TestWorkerRunOnceEnqueuesResolvedKnowledgeChunkTask(t *testing.T) {
	task := testChunkTask()
	consumer := &fakeConsumer{message: knowledgeChunkMessage(EventKnowledgeChunkReady)}
	resolver := &fakeResolver{task: task}
	queue := &fakeQueue{}
	worker := NewWorker(consumer, resolver, queue, Config{})

	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if len(resolver.events) != 1 || resolver.events[0].ChunkID != "kchunk_1" {
		t.Fatalf("expected resolver to receive chunk event: %+v", resolver.events)
	}
	if len(queue.tasks) != 1 || queue.tasks[0].IdempotencyKey != task.IdempotencyKey {
		t.Fatalf("expected task enqueue: %+v", queue.tasks)
	}
	if consumer.commits != 1 {
		t.Fatalf("expected commit after enqueue, got %d", consumer.commits)
	}
}

func TestWorkerRunOnceRejectsUnsupportedEventWithoutCommit(t *testing.T) {
	consumer := &fakeConsumer{message: knowledgeChunkMessage("knowledge.source.created.v1")}
	worker := NewWorker(consumer, &fakeResolver{task: testChunkTask()}, &fakeQueue{}, Config{})

	if err := worker.RunOnce(context.Background()); err == nil {
		t.Fatal("expected unsupported event error")
	}
	if consumer.commits != 0 {
		t.Fatalf("unsupported event must not commit, got %d", consumer.commits)
	}
}

func TestWorkerRunOnceDoesNotCommitWhenResolverFails(t *testing.T) {
	consumer := &fakeConsumer{message: knowledgeChunkMessage(EventKnowledgeChunkReady)}
	worker := NewWorker(consumer, &fakeResolver{err: errors.New("resolver failed")}, &fakeQueue{}, Config{})

	if err := worker.RunOnce(context.Background()); err == nil {
		t.Fatal("expected resolver error")
	}
	if consumer.commits != 0 {
		t.Fatalf("resolver failure must not commit, got %d", consumer.commits)
	}
}

func TestDecodeKnowledgeChunkReadyUsesEventTypeFromPayload(t *testing.T) {
	message := knowledgeChunkMessage(EventKnowledgeChunkReady)
	message.EventType = ""
	event, err := DecodeKnowledgeChunkReady(message)
	if err != nil {
		t.Fatalf("decode event: %v", err)
	}
	if event.TenantID != "tenant-vector" || event.ChunkID != "kchunk_1" {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func knowledgeChunkMessage(eventType string) types.ChunkEventMessage {
	payload := knowledgeChunkReadyPayload{
		EventID:         "evt_kchunk_1",
		EventType:       eventType,
		TenantID:        "tenant-vector",
		ChunkID:         "kchunk_1",
		DocumentID:      "kdoc_1",
		SourceID:        "ksrc_1",
		SourceVersion:   "2",
		ChunkIndex:      0,
		ChunkHash:       "sha256:chunkhash",
		VisibilityScope: "tenant:tenant-vector",
		DataClass:       "BUSINESS_INTERNAL",
		PolicyVersion:   "policy-v1",
		ChunkVersion:    "chunk-v1",
		TombstoneStatus: "ACTIVE",
		CorrelationID:   "corr-vector",
		CausationID:     "cause-vector",
		TraceID:         "trace-vector",
	}
	encoded, _ := json.Marshal(payload)
	return types.ChunkEventMessage{
		Topic:     TopicKnowledgeEvents,
		Partition: 0,
		Offset:    42,
		EventType: eventType,
		Value:     encoded,
	}
}

func testChunkTask() types.VectorEmbeddingTask {
	return types.VectorEmbeddingTask{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-vector",
			ServiceName: types.AllowedCallerVectorIndex,
			RequestID:   "req-vector-chunk",
		},
		SourceService:      types.AllowedCallerKnowledgeIngestion,
		CollectionType:     types.CollectionTypeKnowledgeChunk,
		SourceRefHash:      "sha256:sourceref",
		SourceID:           "ksrc_1:kdoc_1:kchunk_1",
		SourceVersion:      2,
		SourceHash:         "sha256:sourcehash",
		ChunkHash:          "sha256:chunkhash",
		InputText:          "redacted chunk preview",
		InputHash:          "sha256:inputhash",
		InputSchemaVersion: 1,
		EmbeddingModelRef:  "deterministic-embedding-v1",
		Dimension:          8,
		VisibilityScope:    "tenant:tenant-vector",
		VisibilityVersion:  1,
		PolicyVersion:      "policy-v1",
		DataClass:          "BUSINESS_INTERNAL",
		IdempotencyKey:     "knowledge-chunk:kchunk_1:deterministic-embedding-v1",
		CorrelationID:      "corr-vector",
		CausationID:        "kchunk_1",
		TraceID:            "trace-vector",
	}
}

type fakeConsumer struct {
	message types.ChunkEventMessage
	commits int
	err     error
}

func (consumer *fakeConsumer) Fetch(context.Context) (types.ChunkEventMessage, error) {
	if consumer.err != nil {
		return types.ChunkEventMessage{}, consumer.err
	}
	return consumer.message, nil
}

func (consumer *fakeConsumer) Commit(context.Context, types.ChunkEventMessage) error {
	consumer.commits++
	return nil
}

type fakeResolver struct {
	task   types.VectorEmbeddingTask
	events []types.KnowledgeChunkReadyEvent
	err    error
}

func (resolver *fakeResolver) ResolveKnowledgeChunkTask(_ context.Context, event types.KnowledgeChunkReadyEvent) (types.VectorEmbeddingTask, error) {
	if resolver.err != nil {
		return types.VectorEmbeddingTask{}, resolver.err
	}
	resolver.events = append(resolver.events, event)
	return resolver.task, nil
}

type fakeQueue struct {
	tasks []types.VectorEmbeddingTask
	err   error
}

func (queue *fakeQueue) EnqueueEmbeddingTask(_ context.Context, task types.VectorEmbeddingTask) (bool, error) {
	if queue.err != nil {
		return false, queue.err
	}
	queue.tasks = append(queue.tasks, task)
	return false, nil
}
