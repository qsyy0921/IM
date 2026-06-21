package rebuild

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

func TestWorkerRunOnceCompletesClaimedTasks(t *testing.T) {
	store := &fakeStore{
		tasks: []types.VectorRebuildTask{
			{
				Job: types.VectorIndexJob{
					TenantID: "tenant-vector",
					JobID:    "vjob_rebuild_1",
					JobType:  types.JobTypeRebuild,
					Status:   types.JobStatusVectorUpserting,
				},
				Checkpoint: types.VectorRebuildCheckpoint{
					TenantID:     "tenant-vector",
					RebuildJobID: "vjob_rebuild_1",
					Status:       types.RebuildCheckpointStatusRunning,
				},
				CollectionType: types.CollectionTypeKnowledgeChunk,
			},
		},
	}
	worker := NewWorker(store, Config{BatchSize: 10})
	stats, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats.Claimed != 1 || stats.Completed != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(store.completed) != 1 || store.completed[0].Job.JobID != "vjob_rebuild_1" {
		t.Fatalf("unexpected completed tasks: %+v", store.completed)
	}
}

func TestWorkerRunOnceRequiresStore(t *testing.T) {
	worker := NewWorker(nil, Config{})
	if _, err := worker.RunOnce(context.Background()); err == nil {
		t.Fatal("expected missing store error")
	}
}

func TestWorkerRunOnceBackfillsBeforeComplete(t *testing.T) {
	task := testRebuildTask()
	store := &fakeStore{tasks: []types.VectorRebuildTask{task}}
	backfiller := &fakeBackfiller{stats: types.RebuildBackfillStats{Backfilled: 2, Embedded: 2, Upserted: 2}}
	worker := NewWorker(store, Config{Backfiller: backfiller})
	stats, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats.Claimed != 1 || stats.Backfilled != 2 || stats.Embedded != 2 || stats.Upserted != 2 || stats.Completed != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(backfiller.tasks) != 1 || backfiller.tasks[0].Job.JobID != task.Job.JobID {
		t.Fatalf("unexpected backfill tasks: %+v", backfiller.tasks)
	}
	if len(store.completed) != 1 {
		t.Fatalf("expected completed task after backfill, got %+v", store.completed)
	}
}

func TestWorkerRunOnceContinuesWhenBackfillHasMore(t *testing.T) {
	task := testRebuildTask()
	store := &fakeStore{tasks: []types.VectorRebuildTask{task}}
	backfiller := &fakeBackfiller{
		stats: types.RebuildBackfillStats{Backfilled: 1, Embedded: 1, Upserted: 1, HasMore: true, NextCursor: "embedding-task:cursor-1"},
	}
	worker := NewWorker(store, Config{Backfiller: backfiller})
	stats, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats.Claimed != 1 || stats.Continued != 1 || stats.Completed != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(store.continued) != 1 || store.continued[0] != "embedding-task:cursor-1" {
		t.Fatalf("unexpected continued cursors: %+v", store.continued)
	}
	if len(store.completed) != 0 {
		t.Fatalf("task should not complete when backfill has more: %+v", store.completed)
	}
}

func TestWorkerRunOnceDoesNotCompleteWhenBackfillFails(t *testing.T) {
	store := &fakeStore{tasks: []types.VectorRebuildTask{testRebuildTask()}}
	worker := NewWorker(store, Config{Backfiller: &fakeBackfiller{err: errors.New("backfill failed")}})
	stats, err := worker.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected backfill error")
	}
	if stats.Claimed != 1 || stats.Completed != 0 || len(store.completed) != 0 {
		t.Fatalf("task should not complete after backfill error: stats=%+v completed=%+v", stats, store.completed)
	}
}

func TestWorkerRunOncePropagatesCompleteError(t *testing.T) {
	store := &fakeStore{
		tasks: []types.VectorRebuildTask{{Job: types.VectorIndexJob{JobID: "vjob_rebuild_1"}}},
		err:   errors.New("complete failed"),
	}
	worker := NewWorker(store, Config{})
	stats, err := worker.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected complete error")
	}
	if stats.Claimed != 1 || stats.Completed != 0 {
		t.Fatalf("unexpected stats after error: %+v", stats)
	}
}

func TestEmbeddingTaskBackfillerUpsertsProviderBackend(t *testing.T) {
	task := testEmbeddingTask("redacted rebuild backfill input")
	lister := &fakeEmbeddingTaskLister{tasks: []types.VectorEmbeddingTask{task}}
	embedder := &fakeEmbedder{
		result: types.VectorEmbeddingResult{
			InvocationID:        "minv_rebuild_1",
			ModelID:             "deterministic-embedding-v1",
			EmbeddingValues:     []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8},
			EmbeddingVectorHash: "sha256:embeddinghash",
			Dimension:           8,
			EmbeddingReturned:   true,
		},
	}
	upserter := &fakeUpserter{}
	backend := &fakeVectorBackend{}
	backfiller := NewEmbeddingTaskBackfiller(lister, embedder, upserter, backend, EmbeddingTaskBackfillerConfig{BatchSize: 10})
	stats, err := backfiller.BackfillRebuildTask(context.Background(), testRebuildTask())
	if err != nil {
		t.Fatalf("backfill rebuild task: %v", err)
	}
	if stats.Backfilled != 1 || stats.Embedded != 1 || stats.Upserted != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(embedder.tasks) != 1 || embedder.tasks[0].InputText != task.InputText {
		t.Fatalf("unexpected embedder tasks: %+v", embedder.tasks)
	}
	if embedder.tasks[0].IdempotencyKey != "rebuild-backfill:"+task.IdempotencyKey {
		t.Fatalf("model embedding call should use rebuild-specific idempotency key: %+v", embedder.tasks[0])
	}
	if len(upserter.commands) != 1 || upserter.commands[0].EmbeddingVectorHash != "sha256:embeddinghash" {
		t.Fatalf("unexpected upsert commands: %+v", upserter.commands)
	}
	if upserter.commands[0].IdempotencyKey != task.IdempotencyKey {
		t.Fatalf("vector upsert should keep original item idempotency key: %+v", upserter.commands[0])
	}
	if len(backend.calls) != 1 || backend.calls[0].item.VectorItemID != "vitem_rebuild_backfill" {
		t.Fatalf("unexpected backend calls: %+v", backend.calls)
	}
}

func TestEmbeddingTaskBackfillerPaginatesWhenLimitExceeded(t *testing.T) {
	tasks := []types.VectorEmbeddingTask{
		testEmbeddingTask("redacted rebuild backfill input one"),
		testEmbeddingTask("redacted rebuild backfill input two"),
	}
	tasks[1].IdempotencyKey = "idem-vector-rebuild-backfill-2"
	tasks[1].InputHash = sha256Ref(tasks[1].InputText)
	upserter := &fakeUpserter{}
	backend := &fakeVectorBackend{}
	backfiller := NewEmbeddingTaskBackfiller(
		&fakeEmbeddingTaskLister{tasks: tasks},
		&fakeEmbedder{result: types.VectorEmbeddingResult{
			InvocationID:        "minv_rebuild_page",
			ModelID:             "deterministic-embedding-v1",
			EmbeddingValues:     []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8},
			EmbeddingVectorHash: "sha256:embeddinghash",
			Dimension:           8,
			EmbeddingReturned:   true,
		}},
		upserter,
		backend,
		EmbeddingTaskBackfillerConfig{BatchSize: 1},
	)
	stats, err := backfiller.BackfillRebuildTask(context.Background(), testRebuildTask())
	if err != nil {
		t.Fatalf("backfill page: %v", err)
	}
	if !stats.HasMore || stats.NextCursor != "embedding-task:idem-vector-rebuild-backfill" {
		t.Fatalf("unexpected page stats: %+v", stats)
	}
	if len(upserter.commands) != 1 || len(backend.calls) != 1 {
		t.Fatalf("expected one partial page write, got upserts=%d backend=%d", len(upserter.commands), len(backend.calls))
	}
}

type fakeStore struct {
	tasks     []types.VectorRebuildTask
	continued []string
	completed []types.VectorRebuildTask
	err       error
}

func (store *fakeStore) ClaimRebuildTasks(_ context.Context, limit int) ([]types.VectorRebuildTask, error) {
	if limit < len(store.tasks) {
		return store.tasks[:limit], nil
	}
	return store.tasks, nil
}

func (store *fakeStore) ContinueRebuildTask(_ context.Context, _ types.VectorRebuildTask, cursorValue string) error {
	if store.err != nil {
		return store.err
	}
	store.continued = append(store.continued, cursorValue)
	return nil
}

func (store *fakeStore) CompleteRebuildTask(_ context.Context, task types.VectorRebuildTask) error {
	if store.err != nil {
		return store.err
	}
	store.completed = append(store.completed, task)
	return nil
}

type fakeBackfiller struct {
	stats types.RebuildBackfillStats
	tasks []types.VectorRebuildTask
	err   error
}

func (backfiller *fakeBackfiller) BackfillRebuildTask(_ context.Context, task types.VectorRebuildTask) (types.RebuildBackfillStats, error) {
	if backfiller.err != nil {
		return types.RebuildBackfillStats{}, backfiller.err
	}
	backfiller.tasks = append(backfiller.tasks, task)
	return backfiller.stats, nil
}

type fakeEmbeddingTaskLister struct {
	tasks []types.VectorEmbeddingTask
}

func (lister *fakeEmbeddingTaskLister) ListCompletedEmbeddingTasks(_ context.Context, _ types.VectorRebuildTask, limit int) ([]types.VectorEmbeddingTask, error) {
	if limit < len(lister.tasks) {
		return lister.tasks[:limit], nil
	}
	return lister.tasks, nil
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
		VectorItemID:      "vitem_rebuild_backfill",
		CollectionID:      "vcol_rebuild_backfill",
		CollectionType:    command.CollectionType,
		BackendVectorID:   "vitem_rebuild_backfill",
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

type fakeVectorBackend struct {
	calls []backendCall
	err   error
}

func (backend *fakeVectorBackend) UpsertEmbedding(
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

func testRebuildTask() types.VectorRebuildTask {
	return types.VectorRebuildTask{
		Job: types.VectorIndexJob{
			TenantID:     "tenant-vector",
			JobID:        "vjob_rebuild_1",
			CollectionID: "vcol_rebuild_1",
			JobType:      types.JobTypeRebuild,
			Status:       types.JobStatusVectorUpserting,
		},
		Checkpoint: types.VectorRebuildCheckpoint{
			TenantID:      "tenant-vector",
			RebuildJobID:  "vjob_rebuild_1",
			CollectionID:  "vcol_rebuild_1",
			SourceService: types.AllowedCallerKnowledgeIngestion,
			PartitionKey:  "partition-1",
			Status:        types.RebuildCheckpointStatusRunning,
		},
		CollectionType:    types.CollectionTypeKnowledgeChunk,
		BackendType:       types.BackendTypePGVector,
		EmbeddingModelRef: "deterministic-embedding-v1",
		Dimension:         8,
	}
}

func testEmbeddingTask(input string) types.VectorEmbeddingTask {
	return types.VectorEmbeddingTask{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-vector",
			ServiceName: types.AllowedCallerVectorIndex,
			RequestID:   "req-vector-rebuild-backfill",
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
		IdempotencyKey:     "idem-vector-rebuild-backfill",
		CorrelationID:      "corr-vector-rebuild",
		TraceID:            "trace-vector-rebuild",
	}
}
