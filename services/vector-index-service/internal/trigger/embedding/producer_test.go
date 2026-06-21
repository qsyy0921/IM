package embedding

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

func TestProducerRunOnceEnqueuesAndCompletesTasks(t *testing.T) {
	task := testEmbeddingTask("redacted producer input")
	source := &fakeSource{tasks: []types.VectorEmbeddingTask{task}}
	queue := &fakeQueue{}
	producer := NewProducer(source, queue, Config{BatchSize: 10})

	stats, err := producer.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats.Claimed != 1 || stats.Enqueued != 1 || stats.Replayed != 0 || stats.Completed != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(queue.tasks) != 1 || queue.tasks[0].IdempotencyKey != task.IdempotencyKey {
		t.Fatalf("expected task to be enqueued: %+v", queue.tasks)
	}
	if len(source.completed) != 1 || source.completed[0].IdempotencyKey != task.IdempotencyKey {
		t.Fatalf("source task was not completed: %+v", source.completed)
	}
}

func TestProducerRunOnceTreatsQueueReplayAsCompleted(t *testing.T) {
	task := testEmbeddingTask("redacted replay input")
	source := &fakeSource{tasks: []types.VectorEmbeddingTask{task}}
	queue := &fakeQueue{replayed: true}
	producer := NewProducer(source, queue, Config{})

	stats, err := producer.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats.Claimed != 1 || stats.Enqueued != 0 || stats.Replayed != 1 || stats.Completed != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(source.completed) != 1 {
		t.Fatalf("source replayed task should be completed: %+v", source.completed)
	}
}

func TestProducerRunOnceRejectsInputHashMismatch(t *testing.T) {
	task := testEmbeddingTask("one producer value")
	task.InputHash = sha256Ref("different producer value")
	queue := &fakeQueue{}
	source := &fakeSource{tasks: []types.VectorEmbeddingTask{task}}
	producer := NewProducer(source, queue, Config{})

	stats, err := producer.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected hash mismatch")
	}
	if stats.Claimed != 1 || stats.Enqueued != 0 || stats.Completed != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(queue.tasks) != 0 || len(source.completed) != 0 {
		t.Fatalf("hash mismatch should not enqueue or complete: queue=%+v complete=%+v", queue.tasks, source.completed)
	}
}

func TestProducerRunOnceDoesNotCompleteWhenQueueFails(t *testing.T) {
	task := testEmbeddingTask("redacted queue failure input")
	queue := &fakeQueue{err: errors.New("queue failed")}
	source := &fakeSource{tasks: []types.VectorEmbeddingTask{task}}
	producer := NewProducer(source, queue, Config{})

	stats, err := producer.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected queue failure")
	}
	if stats.Claimed != 1 || stats.Enqueued != 0 || stats.Completed != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(source.completed) != 0 {
		t.Fatalf("failed queue task should not be completed: %+v", source.completed)
	}
}

type fakeQueue struct {
	tasks    []types.VectorEmbeddingTask
	replayed bool
	err      error
}

func (queue *fakeQueue) EnqueueEmbeddingTask(_ context.Context, task types.VectorEmbeddingTask) (bool, error) {
	if queue.err != nil {
		return false, queue.err
	}
	queue.tasks = append(queue.tasks, task)
	return queue.replayed, nil
}
