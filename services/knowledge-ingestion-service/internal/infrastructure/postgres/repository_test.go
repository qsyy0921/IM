package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/knowledge-ingestion-service/internal/domain"
	"github.com/qsyy0921/IM/services/knowledge-ingestion-service/internal/types"
)

func TestRepositoryKnowledgeIngestionIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openKnowledgeTestPool(t)
	resetKnowledgeTables(t, ctx, pool)
	repository := NewRepository(pool)

	sourcePrepared := prepareKnowledgeSource(t, "source-idem-1", "ksrc_test_1")
	source, replayed, err := repository.CreateKnowledgeSource(ctx, sourcePrepared)
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if replayed || source.SourceID != "ksrc_test_1" {
		t.Fatalf("unexpected source create result: replayed=%v %+v", replayed, source)
	}
	sourceReplay, replayed, err := repository.CreateKnowledgeSource(ctx, sourcePrepared)
	if err != nil {
		t.Fatalf("source replay: %v", err)
	}
	if !replayed || sourceReplay.SourceID != source.SourceID {
		t.Fatalf("unexpected source replay: replayed=%v %+v", replayed, sourceReplay)
	}
	conflict := prepareKnowledgeSource(t, "source-idem-1", "ksrc_conflict")
	conflict.Command.ContentHash = "sha256:different"
	conflict.CommandHash = "sha256:different"
	if _, _, err := repository.CreateKnowledgeSource(ctx, conflict); !errors.Is(err, types.ErrFailedPrecondition) {
		t.Fatalf("expected source idempotency conflict, got %v", err)
	}

	jobPrepared := prepareIngestionJob(t, source.SourceID, "job-idem-1", "kjob_test_1", "kdoc_test_1")
	job, replayed, err := repository.SubmitIngestionJob(ctx, jobPrepared)
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}
	if replayed || job.Status != types.JobStatusDone || job.ChunkCount != 2 {
		t.Fatalf("unexpected job result: replayed=%v %+v", replayed, job)
	}
	loaded, err := repository.GetIngestionJob(ctx, "tenant-knowledge-test", job.JobID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if loaded.DocumentID != "kdoc_test_1" || loaded.ChunkCount != 2 {
		t.Fatalf("unexpected loaded job: %+v", loaded)
	}
	chunks, nextToken, err := repository.ListKnowledgeChunks(ctx, types.ListKnowledgeChunksCommand{
		AuthContext: types.AuthContext{TenantID: "tenant-knowledge-test", ServiceName: "admin-service"},
		SourceID:    source.SourceID,
		PageSize:    10,
	})
	if err != nil {
		t.Fatalf("list chunks: %v", err)
	}
	if nextToken != "" || len(chunks) != 2 {
		t.Fatalf("unexpected chunk page: next=%q chunks=%d", nextToken, len(chunks))
	}
	assertKnowledgeOutboxLowSensitive(t, ctx, pool)
}

func prepareKnowledgeSource(t *testing.T, idempotencyKey string, sourceID string) domain.PreparedKnowledgeSource {
	t.Helper()
	prepared, err := domain.PrepareKnowledgeSource(types.CreateKnowledgeSourceCommand{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-knowledge-test",
			ServiceName: "admin-service",
		},
		SourceType:      types.SourceTypeManualMarkdown,
		SourceRef:       "manual://private/object-key-should-not-leak",
		OwnerRef:        "group:docs",
		VisibilityScope: "tenant:tenant-knowledge-test",
		DataClass:       types.DataClassBusinessInternal,
		ContentHash:     "sha256:source-content",
		MimeType:        "text/markdown",
		SizeBytes:       512,
		SourceVersion:   "v1",
		IdempotencyKey:  idempotencyKey,
		CorrelationID:   "corr-knowledge-test",
		TraceID:         "trace-knowledge-test",
	}, sourceID, time.Now().UTC())
	if err != nil {
		t.Fatalf("prepare source: %v", err)
	}
	return prepared
}

func prepareIngestionJob(t *testing.T, sourceID string, idempotencyKey string, jobID string, documentID string) domain.PreparedIngestionJob {
	t.Helper()
	prepared, err := domain.PrepareIngestionJob(types.SubmitIngestionJobCommand{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-knowledge-test",
			ServiceName: "admin-service",
		},
		SourceID:       sourceID,
		SourceVersion:  "v1",
		JobType:        types.JobTypeIngest,
		RequestedBy:    "operator:test",
		IdempotencyKey: idempotencyKey,
		DocumentHash:   "sha256:document",
		MimeType:       "text/markdown",
		SizeBytes:      512,
		PageCount:      1,
		Language:       "en",
		Chunks: []types.ChunkManifestItem{
			{
				ChunkHash:            "sha256:chunk-1",
				ChunkPreviewRedacted: "redacted preview 1",
				VisibilityScope:      "tenant:tenant-knowledge-test",
				DataClass:            types.DataClassBusinessInternal,
				PolicyVersion:        types.DefaultPolicyVersion,
				ChunkVersion:         "v1",
			},
			{
				ChunkHash:            "sha256:chunk-2",
				ChunkPreviewRedacted: "redacted preview 2",
				VisibilityScope:      "tenant:tenant-knowledge-test",
				DataClass:            types.DataClassBusinessInternal,
				PolicyVersion:        types.DefaultPolicyVersion,
				ChunkVersion:         "v1",
			},
		},
	}, jobID, documentID, []string{"kchk_test_1", "kchk_test_2"}, time.Now().UTC())
	if err != nil {
		t.Fatalf("prepare job: %v", err)
	}
	return prepared
}

func openKnowledgeTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is required for knowledge-ingestion postgres integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	t.Cleanup(pool.Close)
	applyKnowledgeMigration(t, context.Background(), pool)
	return pool
}

func applyKnowledgeMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "knowledge-ingestion", "000001_knowledge_ingestion_core.sql")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read knowledge migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(content)); err != nil {
		t.Fatalf("apply knowledge migration: %v", err)
	}
}

func resetKnowledgeTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
TRUNCATE
    knowledge_outbox,
    knowledge_delete_proofs,
    knowledge_ingestion_jobs,
    knowledge_chunks,
    knowledge_documents,
    knowledge_sources
CASCADE
`)
	if err != nil {
		t.Fatalf("reset knowledge tables: %v", err)
	}
}

func assertKnowledgeOutboxLowSensitive(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT aggregate_id, partition_key, payload_json::text FROM knowledge_outbox WHERE tenant_id = 'tenant-knowledge-test'`)
	if err != nil {
		t.Fatalf("query knowledge outbox: %v", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
		var aggregateID string
		var partitionKey string
		var payload string
		if err := rows.Scan(&aggregateID, &partitionKey, &payload); err != nil {
			t.Fatalf("scan knowledge outbox: %v", err)
		}
		for _, forbidden := range []string{
			"manual://private",
			"object-key-should-not-leak",
			"raw parser error",
			"api_key",
			"credential",
			"chunk text",
		} {
			if strings.Contains(payload, forbidden) || strings.Contains(aggregateID, forbidden) || strings.Contains(partitionKey, forbidden) {
				t.Fatalf("knowledge outbox leaked forbidden value %q: aggregate=%s partition=%s payload=%s", forbidden, aggregateID, partitionKey, payload)
			}
		}
		if !strings.Contains(payload, "sha256:") {
			t.Fatalf("knowledge outbox payload missing hash refs: %s", payload)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("knowledge outbox rows: %v", err)
	}
	if count != 4 {
		t.Fatalf("expected four knowledge outbox rows, got %d", count)
	}
}
