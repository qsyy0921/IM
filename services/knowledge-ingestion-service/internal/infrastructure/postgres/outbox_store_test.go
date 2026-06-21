package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/knowledge-ingestion-service/internal/types"
)

func TestOutboxStoreProcessReadyBatchPublishesKnowledgeEventsIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openKnowledgeTestPool(t)
	resetKnowledgeTables(t, ctx, pool)
	repository := NewRepository(pool)

	sourcePrepared := prepareKnowledgeSource(t, "source-outbox-publish", "ksrc_outbox_publish")
	source, _, err := repository.CreateKnowledgeSource(ctx, sourcePrepared)
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	jobPrepared := prepareIngestionJob(t, source.SourceID, "job-outbox-publish", "kjob_outbox_publish", "kdoc_outbox_publish")
	if _, _, err := repository.SubmitIngestionJob(ctx, jobPrepared); err != nil {
		t.Fatalf("submit job: %v", err)
	}

	store := NewOutboxStore(pool)
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		if len(messages) != 4 {
			t.Fatalf("messages = %d, want 4", len(messages))
		}
		for _, message := range messages {
			if message.Producer != "knowledge-ingestion-service" ||
				message.PartitionKey == "" ||
				message.PayloadJSON == nil {
				t.Fatalf("unexpected message: %+v", message)
			}
		}
		return make([]error, len(messages))
	})
	if err != nil {
		t.Fatalf("process ready batch: %v", err)
	}
	if stats.Fetched != 4 || stats.Published != 4 || stats.Retried != 0 || stats.DeadLettered != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	var published int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM knowledge_outbox WHERE status = 'PUBLISHED'`).Scan(&published); err != nil {
		t.Fatalf("read published count: %v", err)
	}
	if published != 4 {
		t.Fatalf("published = %d, want 4", published)
	}
}

func TestOutboxStoreProcessReadyBatchDeadLettersPublishErrorIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openKnowledgeTestPool(t)
	resetKnowledgeTables(t, ctx, pool)
	if _, err := pool.Exec(ctx, `
INSERT INTO knowledge_outbox (
    event_id, tenant_id, aggregate_type, aggregate_id, event_type, event_version,
    partition_key, payload_json
) VALUES (
    'evt_bad_knowledge', 'tenant-knowledge-test', 'knowledge_chunk', 'chunk-bad',
    'knowledge.chunk.ready.v1', 1, 'tenant-knowledge-test:chunk-bad',
    '{"tenant_id":"tenant-knowledge-test","chunk_id":"chunk-bad","source_id":"source-bad","chunk_hash":"sha256:chunk-bad","visibility_scope":"tenant:tenant-knowledge-test","data_class":"BUSINESS_INTERNAL","policy_version":"policy-local-v1","chunk_version":"v1","tombstone_status":"ACTIVE"}'::jsonb
)
`); err != nil {
		t.Fatalf("insert outbox: %v", err)
	}

	store := NewOutboxStore(pool)
	stats, err := store.ProcessReadyBatch(ctx, 10, 1, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		if len(messages) != 1 {
			t.Fatalf("messages = %d, want 1", len(messages))
		}
		return []error{errors.New("kafka broker unavailable")}
	})
	if err != nil {
		t.Fatalf("process ready batch: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 0 || stats.DeadLettered != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	var status string
	var retryCount int
	if err := pool.QueryRow(ctx, `SELECT status, retry_count FROM knowledge_outbox WHERE event_id = 'evt_bad_knowledge'`).Scan(&status, &retryCount); err != nil {
		t.Fatalf("read outbox status: %v", err)
	}
	if status != types.OutboxStatusDLQ || retryCount != 1 {
		t.Fatalf("unexpected outbox row: status=%s retry=%d", status, retryCount)
	}
}
