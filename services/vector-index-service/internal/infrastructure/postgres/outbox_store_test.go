package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

func TestOutboxStoreProcessReadyBatchPublishesIndexedIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openVectorTestPool(t)
	resetVectorTables(t, ctx, pool)
	repository := NewRepository(pool)
	prepared := prepareUpsert(t, "vector-outbox-publish", "vitem_outbox_publish", "vjob_outbox_publish")
	if _, _, _, err := repository.UpsertVectorItem(ctx, prepared); err != nil {
		t.Fatalf("upsert vector item: %v", err)
	}

	store := NewOutboxStore(pool)
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		if len(messages) != 1 {
			t.Fatalf("messages = %d, want 1", len(messages))
		}
		if messages[0].EventType != "vector.item.indexed.v1" ||
			messages[0].Producer != "vector-index-service" ||
			messages[0].AggregateID == "" {
			t.Fatalf("unexpected message: %+v", messages[0])
		}
		return []error{nil}
	})
	if err != nil {
		t.Fatalf("process ready batch: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 1 || stats.Retried != 0 || stats.DeadLettered != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM vector_outbox WHERE event_type = 'vector.item.indexed.v1'`).Scan(&status); err != nil {
		t.Fatalf("read outbox status: %v", err)
	}
	if status != types.OutboxStatusPublished {
		t.Fatalf("status = %s", status)
	}
}

func TestOutboxStoreProcessReadyBatchDeadLettersInvalidPayloadIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openVectorTestPool(t)
	resetVectorTables(t, ctx, pool)
	repository := NewRepository(pool)
	prepared := prepareUpsert(t, "vector-outbox-dlq", "vitem_outbox_dlq", "vjob_outbox_dlq")
	if _, _, _, err := repository.UpsertVectorItem(ctx, prepared); err != nil {
		t.Fatalf("upsert vector item: %v", err)
	}

	store := NewOutboxStore(pool)
	stats, err := store.ProcessReadyBatch(ctx, 10, 1, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		if len(messages) != 1 {
			t.Fatalf("messages = %d, want 1", len(messages))
		}
		return []error{errors.New("raw kafka provider body should be sanitized")}
	})
	if err != nil {
		t.Fatalf("process ready batch: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 0 || stats.DeadLettered != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	var status string
	var lastError string
	if err := pool.QueryRow(ctx, `SELECT status, last_error FROM vector_outbox WHERE event_type = 'vector.item.indexed.v1'`).Scan(&status, &lastError); err != nil {
		t.Fatalf("read outbox status: %v", err)
	}
	if status != types.OutboxStatusDLQ {
		t.Fatalf("status = %s", status)
	}
	if lastError != "vector outbox publish broker unavailable" && lastError != "vector outbox publish failed" {
		t.Fatalf("last_error was not sanitized: %q", lastError)
	}
}
