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
		task.CollectionType != types.CollectionTypeKnowledgeChunk {
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
    vector_items,
    vector_collections
CASCADE
`)
	if err != nil {
		t.Fatalf("reset vector tables: %v", err)
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
