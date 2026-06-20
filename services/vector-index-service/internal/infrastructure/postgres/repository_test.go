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
