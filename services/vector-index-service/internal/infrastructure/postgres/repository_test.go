package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/domain"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

func TestRepositoryVectorFirstPathIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openVectorTestPool(t)
	resetVectorTables(t, ctx, pool)
	repository := NewRepository(pool)

	prepared := prepareUpsert(t, "vector-idem-1", "vitem_test_1", "vjob_test_1")
	item, job, replayed, err := repository.UpsertVectorItem(ctx, prepared)
	if err != nil {
		t.Fatalf("upsert vector item: %v", err)
	}
	if replayed || item.Status != types.VectorItemStatusIndexed || job.Status != types.JobStatusIndexed {
		t.Fatalf("unexpected upsert: replayed=%v item=%+v job=%+v", replayed, item, job)
	}
	assertBackendItemState(t, ctx, pool, item.VectorItemID, types.BackendItemStatusActive, types.TombstoneStatusNone)
	replayedItem, _, replayed, err := repository.UpsertVectorItem(ctx, prepared)
	if err != nil {
		t.Fatalf("replay upsert: %v", err)
	}
	if !replayed || replayedItem.VectorItemID != item.VectorItemID {
		t.Fatalf("unexpected replay: replayed=%v item=%+v", replayed, replayedItem)
	}
	conflict := prepareUpsert(t, "vector-idem-1", "vitem_conflict", "vjob_conflict")
	conflict.Command.SourceHash = "sha256:different"
	conflict.CommandHash = "sha256:different"
	if _, _, _, err := repository.UpsertVectorItem(ctx, conflict); !errors.Is(err, types.ErrAlreadyExists) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}

	results, err := repository.SearchVectors(ctx, types.SearchVectorsCommand{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-vector-test",
			ServiceName: types.AllowedCallerRetrieval,
		},
		RequesterRef:       "retrieval:requester",
		RetrievalRequestID: "retrieval-request-1",
		CollectionTypes:    []string{types.CollectionTypeKnowledgeChunk},
		QueryEmbeddingRef:  "embedding-ref:query-1",
		TopK:               10,
		VisibilityScope:    prepared.Command.VisibilityScope,
		PolicyVersion:      prepared.Command.PolicyVersion,
	})
	if err != nil {
		t.Fatalf("search vectors: %v", err)
	}
	if len(results) != 1 || results[0].VectorItemRef != item.VectorItemID || results[0].SourceRefHash != item.SourceRefHash {
		t.Fatalf("unexpected search results: %+v", results)
	}

	tombstonePrepared := prepareTombstone(t, item.VectorItemID, "tombstone-idem-1", "vtomb_test_1", "vjob_tombstone_1")
	tombstoned, tombstoneJob, tombstoneID, replayed, err := repository.TombstoneVectorItem(ctx, tombstonePrepared)
	if err != nil {
		t.Fatalf("tombstone vector item: %v", err)
	}
	if replayed || tombstoneID == "" || tombstoned.Status != types.VectorItemStatusTombstoned || tombstoneJob.Status != types.JobStatusTombstoned {
		t.Fatalf("unexpected tombstone: replayed=%v tombstone=%s item=%+v job=%+v", replayed, tombstoneID, tombstoned, tombstoneJob)
	}
	assertBackendItemState(t, ctx, pool, item.VectorItemID, types.BackendItemStatusDeleted, types.TombstoneStatusTombstoned)
	results, err = repository.SearchVectors(ctx, types.SearchVectorsCommand{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-vector-test",
			ServiceName: types.AllowedCallerRetrieval,
		},
		RequesterRef:       "retrieval:requester",
		RetrievalRequestID: "retrieval-request-2",
		CollectionTypes:    []string{types.CollectionTypeKnowledgeChunk},
		QueryEmbeddingRef:  "embedding-ref:query-1",
		TopK:               10,
		VisibilityScope:    prepared.Command.VisibilityScope,
		PolicyVersion:      prepared.Command.PolicyVersion,
	})
	if err != nil {
		t.Fatalf("search after tombstone: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("tombstoned item should not be returned: %+v", results)
	}

	loadedJob, err := repository.GetVectorIndexJob(ctx, types.GetVectorIndexJobCommand{
		AuthContext: types.AuthContext{TenantID: "tenant-vector-test", ServiceName: types.AllowedCallerVectorIndex},
		JobID:       job.JobID,
	})
	if err != nil {
		t.Fatalf("get vector job: %v", err)
	}
	if loadedJob.JobID != job.JobID {
		t.Fatalf("unexpected loaded job: %+v", loadedJob)
	}
	assertVectorOutboxLowSensitive(t, ctx, pool)
}

func TestRepositoryRequestVectorRebuildIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openVectorTestPool(t)
	resetVectorTables(t, ctx, pool)
	repository := NewRepository(pool)

	upsert := prepareUpsert(t, "vector-rebuild-item", "vitem_rebuild_source", "vjob_rebuild_source")
	if _, _, _, err := repository.UpsertVectorItem(ctx, upsert); err != nil {
		t.Fatalf("seed vector collection: %v", err)
	}

	rebuild := prepareRebuild(t, "vector-rebuild-idem", "vjob_rebuild_1")
	job, checkpoint, replayed, err := repository.RequestVectorRebuild(ctx, rebuild)
	if err != nil {
		t.Fatalf("request vector rebuild: %v", err)
	}
	if replayed || job.JobType != types.JobTypeRebuild || job.Status != types.JobStatusPending {
		t.Fatalf("unexpected rebuild job: replayed=%v job=%+v", replayed, job)
	}
	if checkpoint.RebuildJobID != job.JobID || checkpoint.Status != types.RebuildCheckpointStatusPending || checkpoint.PartitionKey != rebuild.Command.PartitionKey {
		t.Fatalf("unexpected rebuild checkpoint: %+v", checkpoint)
	}

	replayedJob, replayedCheckpoint, replayed, err := repository.RequestVectorRebuild(ctx, rebuild)
	if err != nil {
		t.Fatalf("replay vector rebuild: %v", err)
	}
	if !replayed || replayedJob.JobID != job.JobID || replayedCheckpoint.RebuildJobID != checkpoint.RebuildJobID {
		t.Fatalf("unexpected rebuild replay: replayed=%v job=%+v checkpoint=%+v", replayed, replayedJob, replayedCheckpoint)
	}

	conflict := prepareRebuild(t, "vector-rebuild-idem", "vjob_rebuild_conflict")
	conflict.Command.CursorValue = "cursor-conflict"
	conflict.CommandHash = "sha256:rebuild-conflict"
	if _, _, _, err := repository.RequestVectorRebuild(ctx, conflict); !errors.Is(err, types.ErrAlreadyExists) {
		t.Fatalf("expected rebuild idempotency conflict, got %v", err)
	}

	missingCollection := prepareRebuild(t, "vector-rebuild-missing", "vjob_rebuild_missing")
	missingCollection.Command.EmbeddingModelRef = "model:text-embedding-missing"
	missingCollection.CollectionID = domain.CollectionID(
		missingCollection.Command.AuthContext.TenantID,
		missingCollection.Command.CollectionType,
		missingCollection.Command.EmbeddingModelRef,
		missingCollection.Command.Dimension,
	)
	if _, _, _, err := repository.RequestVectorRebuild(ctx, missingCollection); !errors.Is(err, types.ErrNotFound) {
		t.Fatalf("expected missing collection not found, got %v", err)
	}
}

func TestRepositorySearchRequiresActiveBackendItemIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openVectorTestPool(t)
	resetVectorTables(t, ctx, pool)
	repository := NewRepository(pool)

	prepared := prepareUpsert(t, "vector-backend-search", "vitem_backend_search", "vjob_backend_search")
	if _, _, _, err := repository.UpsertVectorItem(ctx, prepared); err != nil {
		t.Fatalf("seed vector item: %v", err)
	}
	if _, err := pool.Exec(ctx, `
UPDATE vector_backend_items
SET status = 'DELETED',
    tombstone_status = 'TOMBSTONED',
    deleted_at = now(),
    updated_at = now()
WHERE tenant_id = 'tenant-vector-test'
  AND vector_item_id = 'vitem_backend_search'
`); err != nil {
		t.Fatalf("mark backend item deleted: %v", err)
	}

	results, err := repository.SearchVectors(ctx, types.SearchVectorsCommand{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-vector-test",
			ServiceName: types.AllowedCallerRetrieval,
		},
		RequesterRef:       "retrieval:requester",
		RetrievalRequestID: "retrieval-request-backend-state",
		CollectionTypes:    []string{types.CollectionTypeKnowledgeChunk},
		QueryEmbeddingRef:  "embedding-ref:query-1",
		TopK:               10,
		VisibilityScope:    prepared.Command.VisibilityScope,
		PolicyVersion:      prepared.Command.PolicyVersion,
	})
	if err != nil {
		t.Fatalf("search vectors: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("deleted backend item should not be returned: %+v", results)
	}
}

func TestBackendStateStoreConfirmActiveBackendItemIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openVectorTestPool(t)
	resetVectorTables(t, ctx, pool)
	repository := NewRepository(pool)

	prepared := prepareUpsert(t, "vector-backend-state-store", "vitem_backend_state_store", "vjob_backend_state_store")
	item, _, _, err := repository.UpsertVectorItem(ctx, prepared)
	if err != nil {
		t.Fatalf("seed vector item: %v", err)
	}
	store := NewBackendStateStore(pool)
	if err := store.ConfirmActiveBackendItem(ctx, item); err != nil {
		t.Fatalf("confirm active backend item: %v", err)
	}

	item.EmbeddingVectorHash = "sha256:wrong"
	if err := store.ConfirmActiveBackendItem(ctx, item); err == nil {
		t.Fatal("expected embedding hash mismatch to fail")
	}

	if _, err := pool.Exec(ctx, `
UPDATE vector_backend_items
SET status = 'DELETED',
    tombstone_status = 'TOMBSTONED',
    deleted_at = now(),
    updated_at = now()
WHERE tenant_id = 'tenant-vector-test'
  AND vector_item_id = 'vitem_backend_state_store'
`); err != nil {
		t.Fatalf("mark backend item deleted: %v", err)
	}
	item.EmbeddingVectorHash = prepared.Command.EmbeddingVectorHash
	if err := store.ConfirmActiveBackendItem(ctx, item); err == nil {
		t.Fatal("expected deleted backend item to fail")
	}
}

func TestRepositoryClaimAndCompleteRebuildTaskIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openVectorTestPool(t)
	resetVectorTables(t, ctx, pool)
	repository := NewRepository(pool)

	upsert := prepareUpsert(t, "vector-rebuild-worker-item", "vitem_rebuild_worker_source", "vjob_rebuild_worker_source")
	if _, _, _, err := repository.UpsertVectorItem(ctx, upsert); err != nil {
		t.Fatalf("seed vector collection: %v", err)
	}
	rebuild := prepareRebuild(t, "vector-rebuild-worker-idem", "vjob_rebuild_worker_1")
	job, checkpoint, replayed, err := repository.RequestVectorRebuild(ctx, rebuild)
	if err != nil {
		t.Fatalf("request vector rebuild: %v", err)
	}
	if replayed || job.Status != types.JobStatusPending || checkpoint.Status != types.RebuildCheckpointStatusPending {
		t.Fatalf("unexpected pending rebuild: replayed=%v job=%+v checkpoint=%+v", replayed, job, checkpoint)
	}

	tasks, err := repository.ClaimRebuildTasks(ctx, 10)
	if err != nil {
		t.Fatalf("claim rebuild tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one rebuild task, got %d", len(tasks))
	}
	task := tasks[0]
	if task.Job.JobID != job.JobID ||
		task.Job.Status != types.JobStatusVectorUpserting ||
		task.Checkpoint.Status != types.RebuildCheckpointStatusRunning ||
		task.CollectionType != types.CollectionTypeKnowledgeChunk ||
		task.EmbeddingModelRef != rebuild.Command.EmbeddingModelRef ||
		task.Dimension != rebuild.Command.Dimension ||
		task.BackendType != types.BackendTypePostgresTest {
		t.Fatalf("unexpected claimed task: %+v", task)
	}
	claimedJob, err := repository.GetVectorIndexJob(ctx, types.GetVectorIndexJobCommand{
		AuthContext: types.AuthContext{TenantID: "tenant-vector-test", ServiceName: types.AllowedCallerVectorIndex},
		JobID:       job.JobID,
	})
	if err != nil {
		t.Fatalf("get claimed rebuild job: %v", err)
	}
	if claimedJob.Status != types.JobStatusVectorUpserting {
		t.Fatalf("unexpected claimed job status: %+v", claimedJob)
	}
	if got := rebuildCheckpointStatus(t, ctx, pool, job.JobID, rebuild.Command.PartitionKey); got != types.RebuildCheckpointStatusRunning {
		t.Fatalf("unexpected running checkpoint status: %s", got)
	}

	if err := repository.ContinueRebuildTask(ctx, task, "embedding-task:cursor-test"); err != nil {
		t.Fatalf("continue rebuild task: %v", err)
	}
	if got := rebuildCheckpointCursor(t, ctx, pool, job.JobID, rebuild.Command.PartitionKey); got != "embedding-task:cursor-test" {
		t.Fatalf("unexpected continued checkpoint cursor: %s", got)
	}
	tasks, err = repository.ClaimRebuildTasks(ctx, 10)
	if err != nil {
		t.Fatalf("claim running rebuild task: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Checkpoint.CursorValue != "embedding-task:cursor-test" ||
		tasks[0].Job.Status != types.JobStatusVectorUpserting ||
		tasks[0].Checkpoint.Status != types.RebuildCheckpointStatusRunning {
		t.Fatalf("unexpected running rebuild task reclaim: %+v", tasks)
	}
	task = tasks[0]

	if err := repository.CompleteRebuildTask(ctx, task); err != nil {
		t.Fatalf("complete rebuild task: %v", err)
	}
	completedJob, err := repository.GetVectorIndexJob(ctx, types.GetVectorIndexJobCommand{
		AuthContext: types.AuthContext{TenantID: "tenant-vector-test", ServiceName: types.AllowedCallerVectorIndex},
		JobID:       job.JobID,
	})
	if err != nil {
		t.Fatalf("get completed rebuild job: %v", err)
	}
	if completedJob.Status != types.JobStatusIndexed || completedJob.CompletedAt.IsZero() {
		t.Fatalf("unexpected completed job: %+v", completedJob)
	}
	if got := rebuildCheckpointStatus(t, ctx, pool, job.JobID, rebuild.Command.PartitionKey); got != types.RebuildCheckpointStatusComplete {
		t.Fatalf("unexpected completed checkpoint status: %s", got)
	}
	assertRebuildOutboxLowSensitive(t, ctx, pool, job.JobID)
}

func TestEmbeddingTaskSourceClaimCompleteIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openVectorTestPool(t)
	resetVectorTables(t, ctx, pool)
	source := NewEmbeddingTaskSource(pool, EmbeddingTaskSourceConfig{
		TenantID:     "tenant-vector-test",
		ClaimTimeout: time.Minute,
	})
	task := prepareEmbeddingTask("queue-1", "redacted queue chunk")

	replayed, err := source.EnqueueEmbeddingTask(ctx, task)
	if err != nil {
		t.Fatalf("enqueue embedding task: %v", err)
	}
	if replayed {
		t.Fatal("first enqueue should not replay")
	}
	replayed, err = source.EnqueueEmbeddingTask(ctx, task)
	if err != nil {
		t.Fatalf("replay enqueue embedding task: %v", err)
	}
	if !replayed {
		t.Fatal("second enqueue should replay")
	}
	conflict := task
	conflict.ChunkHash = sha256Ref("different-chunk")
	if _, err := source.EnqueueEmbeddingTask(ctx, conflict); !errors.Is(err, types.ErrAlreadyExists) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}

	tasks, err := source.ClaimEmbeddingTasks(ctx, 10)
	if err != nil {
		t.Fatalf("claim embedding task: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one claimed task, got %d", len(tasks))
	}
	claimed := tasks[0]
	if claimed.InputText != "redacted queue chunk" || claimed.InputHash != sha256Ref("redacted queue chunk") {
		t.Fatalf("unexpected claimed task input: %+v", claimed)
	}
	assertEmbeddingTaskState(t, ctx, pool, task.IdempotencyKey, types.EmbeddingTaskStatusRunning, 1)

	tasks, err = source.ClaimEmbeddingTasks(ctx, 10)
	if err != nil {
		t.Fatalf("claim before deadline: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("task should not be reclaimed before claim deadline: %+v", tasks)
	}
	if _, err := pool.Exec(ctx, `
UPDATE vector_embedding_tasks
SET claim_deadline = now() - interval '1 second'
WHERE tenant_id = 'tenant-vector-test'
  AND idempotency_key = $1
`, task.IdempotencyKey); err != nil {
		t.Fatalf("expire claim deadline: %v", err)
	}
	tasks, err = source.ClaimEmbeddingTasks(ctx, 10)
	if err != nil {
		t.Fatalf("reclaim expired embedding task: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected expired task to be reclaimed, got %d", len(tasks))
	}
	assertEmbeddingTaskState(t, ctx, pool, task.IdempotencyKey, types.EmbeddingTaskStatusRunning, 2)

	if err := source.CompleteEmbeddingTask(ctx, tasks[0]); err != nil {
		t.Fatalf("complete embedding task: %v", err)
	}
	assertEmbeddingTaskState(t, ctx, pool, task.IdempotencyKey, types.EmbeddingTaskStatusComplete, 2)
	tasks, err = source.ClaimEmbeddingTasks(ctx, 10)
	if err != nil {
		t.Fatalf("claim after complete: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("completed task should not be claimed: %+v", tasks)
	}
}

func TestEmbeddingTaskSourceListCompletedTasksForRebuildIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openVectorTestPool(t)
	resetVectorTables(t, ctx, pool)
	repository := NewRepository(pool)
	source := NewEmbeddingTaskSource(pool, EmbeddingTaskSourceConfig{
		TenantID:     "tenant-vector-test",
		ClaimTimeout: time.Minute,
	})

	upsert := prepareUpsert(t, "vector-rebuild-backfill-item", "vitem_rebuild_backfill_source", "vjob_rebuild_backfill_source")
	if _, _, _, err := repository.UpsertVectorItem(ctx, upsert); err != nil {
		t.Fatalf("seed vector collection: %v", err)
	}
	rebuild := prepareRebuild(t, "vector-rebuild-backfill-idem", "vjob_rebuild_backfill")
	if _, _, _, err := repository.RequestVectorRebuild(ctx, rebuild); err != nil {
		t.Fatalf("request vector rebuild: %v", err)
	}
	rebuildTasks, err := repository.ClaimRebuildTasks(ctx, 10)
	if err != nil {
		t.Fatalf("claim rebuild task: %v", err)
	}
	if len(rebuildTasks) != 1 {
		t.Fatalf("expected one rebuild task, got %d", len(rebuildTasks))
	}
	rebuildTask := rebuildTasks[0]

	completed := prepareEmbeddingTaskForRebuild("backfill-completed", "redacted completed rebuild chunk", rebuildTask)
	if _, err := source.EnqueueEmbeddingTask(ctx, completed); err != nil {
		t.Fatalf("enqueue completed candidate: %v", err)
	}
	claimed, err := source.ClaimEmbeddingTasks(ctx, 10)
	if err != nil {
		t.Fatalf("claim completed candidate: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected one claimed embedding task, got %d", len(claimed))
	}
	if err := source.CompleteEmbeddingTask(ctx, claimed[0]); err != nil {
		t.Fatalf("complete embedding task: %v", err)
	}

	pending := prepareEmbeddingTaskForRebuild("backfill-pending", "redacted pending rebuild chunk", rebuildTask)
	if _, err := source.EnqueueEmbeddingTask(ctx, pending); err != nil {
		t.Fatalf("enqueue pending candidate: %v", err)
	}
	wrongDimension := prepareEmbeddingTaskForRebuild("backfill-wrong-dimension", "redacted wrong dimension rebuild chunk", rebuildTask)
	wrongDimension.Dimension = rebuildTask.Dimension + 1
	if _, err := source.EnqueueEmbeddingTask(ctx, wrongDimension); err != nil {
		t.Fatalf("enqueue wrong dimension candidate: %v", err)
	}
	claimed, err = source.ClaimEmbeddingTasks(ctx, 10)
	if err != nil {
		t.Fatalf("claim mixed candidates: %v", err)
	}
	for _, candidate := range claimed {
		if candidate.IdempotencyKey == wrongDimension.IdempotencyKey {
			if err := source.CompleteEmbeddingTask(ctx, candidate); err != nil {
				t.Fatalf("complete wrong dimension candidate: %v", err)
			}
		}
	}

	listed, err := source.ListCompletedEmbeddingTasks(ctx, rebuildTask, 10)
	if err != nil {
		t.Fatalf("list completed rebuild embedding tasks: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected only completed matching task, got %+v", listed)
	}
	if listed[0].IdempotencyKey != completed.IdempotencyKey || listed[0].InputText != completed.InputText {
		t.Fatalf("unexpected listed task: %+v", listed[0])
	}
	rebuildTask.Checkpoint.CursorValue = "embedding-task:" + completed.IdempotencyKey
	listed, err = source.ListCompletedEmbeddingTasks(ctx, rebuildTask, 10)
	if err != nil {
		t.Fatalf("list completed rebuild embedding tasks after cursor: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("cursor should exclude already backfilled task: %+v", listed)
	}
}

func prepareUpsert(t *testing.T, idempotencyKey string, vectorItemID string, jobID string) domain.PreparedUpsert {
	t.Helper()
	prepared, err := domain.PrepareUpsert(types.UpsertVectorItemCommand{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-vector-test",
			ServiceName: types.AllowedCallerKnowledgeIngestion,
		},
		SourceService:       types.AllowedCallerKnowledgeIngestion,
		CollectionType:      types.CollectionTypeKnowledgeChunk,
		SourceRefHash:       "sha256:source-ref-1",
		SourceID:            "knowledge-chunk-1",
		SourceVersion:       1,
		SourceHash:          "sha256:source-1",
		ChunkHash:           "sha256:chunk-1",
		EmbeddingModelRef:   "model:text-embedding-local",
		EmbeddingVectorHash: "sha256:embedding-1",
		Dimension:           3,
		VisibilityScope:     "conversation:conv-1",
		VisibilityVersion:   1,
		PolicyVersion:       "policy:v1",
		DataClass:           "LOW",
		IdempotencyKey:      idempotencyKey,
		CorrelationID:       "corr-vector-test",
	}, vectorItemID, jobID, time.Now().UTC())
	if err != nil {
		t.Fatalf("prepare upsert: %v", err)
	}
	return prepared
}

func prepareRebuild(t *testing.T, idempotencyKey string, jobID string) domain.PreparedRebuild {
	t.Helper()
	prepared, err := domain.PrepareRebuild(types.RequestVectorRebuildCommand{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-vector-test",
			ServiceName: types.AllowedCallerVectorIndex,
		},
		CollectionType:    types.CollectionTypeKnowledgeChunk,
		EmbeddingModelRef: "model:text-embedding-local",
		Dimension:         3,
		SourceService:     types.AllowedCallerKnowledgeIngestion,
		PartitionKey:      "knowledge-ingestion-service:tenant-vector-test",
		CursorValue:       "cursor:start",
		IdempotencyKey:    idempotencyKey,
		CorrelationID:     "corr-vector-rebuild-test",
	}, jobID, time.Now().UTC())
	if err != nil {
		t.Fatalf("prepare rebuild: %v", err)
	}
	return prepared
}

func prepareTombstone(t *testing.T, vectorItemID string, idempotencyKey string, tombstoneID string, jobID string) domain.PreparedTombstone {
	t.Helper()
	prepared, err := domain.PrepareTombstone(types.TombstoneVectorItemCommand{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-vector-test",
			ServiceName: types.AllowedCallerKnowledgeIngestion,
		},
		VectorItemID:   vectorItemID,
		DeleteProofID:  "delete-proof-1",
		ReasonClass:    "SOURCE_DELETED",
		IdempotencyKey: idempotencyKey,
		CorrelationID:  "corr-vector-tombstone-test",
	}, tombstoneID, jobID, time.Now().UTC())
	if err != nil {
		t.Fatalf("prepare tombstone: %v", err)
	}
	return prepared
}

func prepareEmbeddingTask(id string, preview string) types.VectorEmbeddingTask {
	return types.VectorEmbeddingTask{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-vector-test",
			ServiceName: types.AllowedCallerVectorIndex,
			TraceID:     "trace-vector-embedding-test",
			RequestID:   "request-vector-embedding-test",
		},
		SourceService:      types.AllowedCallerKnowledgeIngestion,
		CollectionType:     types.CollectionTypeKnowledgeChunk,
		SourceRefHash:      sha256Ref("source-ref-" + id),
		SourceID:           "knowledge-source:document:" + id,
		SourceVersion:      1,
		SourceHash:         sha256Ref("source-" + id),
		ChunkHash:          sha256Ref("chunk-" + id),
		InputText:          preview,
		InputHash:          sha256Ref(preview),
		InputSchemaVersion: 1,
		EmbeddingModelRef:  "deterministic-embedding-v1",
		Dimension:          8,
		VisibilityScope:    "conversation:vector-embedding-test",
		VisibilityVersion:  1,
		PolicyVersion:      "policy-vector-embedding-test",
		DataClass:          "BUSINESS_INTERNAL",
		IdempotencyKey:     "embedding-task-" + id,
		CorrelationID:      "corr-vector-embedding-test",
		CausationID:        "cause-vector-embedding-test",
		TraceID:            "trace-vector-embedding-test",
	}
}

func prepareEmbeddingTaskForRebuild(id string, preview string, rebuildTask types.VectorRebuildTask) types.VectorEmbeddingTask {
	task := prepareEmbeddingTask(id, preview)
	task.SourceService = rebuildTask.Checkpoint.SourceService
	task.CollectionType = rebuildTask.CollectionType
	task.EmbeddingModelRef = rebuildTask.EmbeddingModelRef
	task.Dimension = rebuildTask.Dimension
	return task
}

func openVectorTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is required for vector-index postgres integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	t.Cleanup(pool.Close)
	applyVectorMigration(t, context.Background(), pool)
	return pool
}

func applyVectorMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "vector-index")
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		t.Fatalf("list vector migrations: %v", err)
	}
	sort.Strings(files)
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read vector migration %s: %v", path, err)
		}
		if _, err := pool.Exec(ctx, string(content)); err != nil {
			t.Fatalf("apply vector migration %s: %v", path, err)
		}
	}
}

func resetVectorTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
TRUNCATE
    vector_embedding_tasks,
    vector_outbox,
    vector_rebuild_checkpoints,
    vector_tombstones,
    vector_index_jobs,
    vector_backend_items,
    vector_items,
    vector_collections
CASCADE
`)
	if err != nil {
		t.Fatalf("reset vector tables: %v", err)
	}
}

func assertBackendItemState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, vectorItemID string, expectedStatus string, expectedTombstone string) {
	t.Helper()
	var backendType string
	var backendVectorID string
	var status string
	var tombstoneStatus string
	if err := pool.QueryRow(ctx, `
SELECT backend_type, backend_vector_id, status, tombstone_status
FROM vector_backend_items
WHERE tenant_id = 'tenant-vector-test'
  AND vector_item_id = $1
`, vectorItemID).Scan(&backendType, &backendVectorID, &status, &tombstoneStatus); err != nil {
		t.Fatalf("query vector backend item: %v", err)
	}
	if backendType != types.BackendTypePostgresTest || backendVectorID == "" ||
		status != expectedStatus || tombstoneStatus != expectedTombstone {
		t.Fatalf("unexpected backend item state: backend_type=%s backend_vector_id=%s status=%s tombstone=%s",
			backendType, backendVectorID, status, tombstoneStatus)
	}
}

func assertVectorOutboxLowSensitive(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT aggregate_id, partition_key, payload_json::text FROM vector_outbox WHERE tenant_id = 'tenant-vector-test' ORDER BY event_type`)
	if err != nil {
		t.Fatalf("query vector outbox: %v", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
		var aggregateID string
		var partitionKey string
		var payload string
		if err := rows.Scan(&aggregateID, &partitionKey, &payload); err != nil {
			t.Fatalf("scan vector outbox: %v", err)
		}
		for _, forbidden := range []string{
			"raw:",
			"http://",
			"https://",
			"s3://",
			"object-key",
			"embedding_vector",
			"provider body",
			"secret",
			"token",
			"knowledge-chunk-1",
			"vitem_test_1",
		} {
			if strings.Contains(payload, forbidden) || strings.Contains(aggregateID, forbidden) || strings.Contains(partitionKey, forbidden) {
				t.Fatalf("vector outbox leaked forbidden value %q: aggregate=%s partition=%s payload=%s", forbidden, aggregateID, partitionKey, payload)
			}
		}
		if !strings.Contains(payload, "sha256:") {
			t.Fatalf("vector outbox payload missing hash refs: %s", payload)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("vector outbox rows: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected two vector outbox rows, got %d", count)
	}
}

func assertEmbeddingTaskState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, idempotencyKey string, expectedStatus string, expectedAttempts int) {
	t.Helper()
	var status string
	var attempts int
	var preview string
	if err := pool.QueryRow(ctx, `
SELECT status, attempt_count, input_preview_redacted
FROM vector_embedding_tasks
WHERE tenant_id = 'tenant-vector-test'
  AND idempotency_key = $1
`, idempotencyKey).Scan(&status, &attempts, &preview); err != nil {
		t.Fatalf("query embedding task state: %v", err)
	}
	if status != expectedStatus || attempts != expectedAttempts {
		t.Fatalf("unexpected task state: status=%s attempts=%d", status, attempts)
	}
	for _, forbidden := range []string{"http://", "https://", "s3://", "object-key", "secret"} {
		if strings.Contains(preview, forbidden) {
			t.Fatalf("embedding task preview leaked forbidden raw ref %q: %s", forbidden, preview)
		}
	}
}

func rebuildCheckpointStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, jobID string, partitionKey string) string {
	t.Helper()
	var status string
	if err := pool.QueryRow(ctx, `
SELECT status
FROM vector_rebuild_checkpoints
WHERE tenant_id = 'tenant-vector-test'
  AND rebuild_job_id = $1
  AND partition_key = $2
`, jobID, partitionKey).Scan(&status); err != nil {
		t.Fatalf("query rebuild checkpoint status: %v", err)
	}
	return status
}

func rebuildCheckpointCursor(t *testing.T, ctx context.Context, pool *pgxpool.Pool, jobID string, partitionKey string) string {
	t.Helper()
	var value string
	if err := pool.QueryRow(ctx, `
SELECT cursor_value
FROM vector_rebuild_checkpoints
WHERE tenant_id = 'tenant-vector-test'
  AND rebuild_job_id = $1
  AND partition_key = $2
`, jobID, partitionKey).Scan(&value); err != nil {
		t.Fatalf("query rebuild checkpoint cursor: %v", err)
	}
	return value
}

func assertRebuildOutboxLowSensitive(t *testing.T, ctx context.Context, pool *pgxpool.Pool, jobID string) {
	t.Helper()
	aggregateID := domain.HashRef(jobID)
	rows, err := pool.Query(ctx, `
SELECT event_type, aggregate_type, aggregate_id, partition_key, payload_json::text
FROM vector_outbox
WHERE tenant_id = 'tenant-vector-test'
  AND aggregate_id = $1
ORDER BY event_version, event_type
`, aggregateID)
	if err != nil {
		t.Fatalf("query rebuild outbox: %v", err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var eventType string
		var aggregateType string
		var aggregateIDValue string
		var partitionKey string
		var payload string
		if err := rows.Scan(&eventType, &aggregateType, &aggregateIDValue, &partitionKey, &payload); err != nil {
			t.Fatalf("scan rebuild outbox: %v", err)
		}
		if aggregateType != "vector_rebuild" || aggregateIDValue != aggregateID {
			t.Fatalf("unexpected rebuild outbox aggregate: type=%s id=%s", aggregateType, aggregateIDValue)
		}
		for _, forbidden := range []string{
			jobID,
			"vjob_rebuild_worker_1",
			"knowledge-ingestion-service:tenant-vector-test",
			"cursor:start",
			"raw:",
			"http://",
			"https://",
			"s3://",
			"embedding_vector",
			"secret",
			"token",
		} {
			if strings.Contains(payload, forbidden) || strings.Contains(aggregateIDValue, forbidden) || strings.Contains(partitionKey, forbidden) {
				t.Fatalf("rebuild outbox leaked forbidden value %q: aggregate=%s partition=%s payload=%s", forbidden, aggregateIDValue, partitionKey, payload)
			}
		}
		if !strings.Contains(payload, `"rebuild_job_ref_hash"`) ||
			!strings.Contains(payload, `"collection_id_hash"`) ||
			!strings.Contains(payload, `"partition_key_hash"`) ||
			!strings.Contains(payload, `"cursor_hash"`) {
			t.Fatalf("rebuild outbox missing hashed fields: %s", payload)
		}
		seen[eventType] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rebuild outbox rows: %v", err)
	}
	if !seen["vector.rebuild.started.v1"] || !seen["vector.rebuild.completed.v1"] || len(seen) != 2 {
		t.Fatalf("unexpected rebuild outbox event types: %+v", seen)
	}
}
