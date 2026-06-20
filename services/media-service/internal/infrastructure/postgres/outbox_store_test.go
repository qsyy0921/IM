package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/media-service/internal/types"
)

func TestOutboxStoreProcessReadyBatchPublishesInAssetOrderIntegration(t *testing.T) {
	pool := openMediaTestPool(t)
	ctx := context.Background()
	resetMediaTables(t, ctx, pool)

	const tenantID = "tenant-media-outbox-order"
	const assetID = "asset-order-1"
	insertMediaOutboxAsset(t, ctx, pool, tenantID, assetID)
	firstID := insertMediaOutboxEvent(t, ctx, pool, tenantID, assetID, "evt-order-uploaded", types.MediaEventAssetUploaded)
	secondID := insertMediaOutboxEvent(t, ctx, pool, tenantID, assetID, "evt-order-ready", types.MediaEventAssetReady)

	store := NewOutboxStore(pool)
	var published []string
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for _, message := range messages {
			published = append(published, message.EventID)
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process first batch: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 1 || len(published) != 1 || published[0] != "evt-order-uploaded" {
		t.Fatalf("unexpected first batch stats=%+v published=%v", stats, published)
	}
	assertMediaOutboxStatus(t, ctx, pool, firstID, types.OutboxStatusPublished)
	assertMediaOutboxStatus(t, ctx, pool, secondID, types.OutboxStatusPending)

	published = nil
	stats, err = store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for _, message := range messages {
			published = append(published, message.EventID)
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process second batch: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 1 || len(published) != 1 || published[0] != "evt-order-ready" {
		t.Fatalf("unexpected second batch stats=%+v published=%v", stats, published)
	}
	assertMediaOutboxStatus(t, ctx, pool, secondID, types.OutboxStatusPublished)
}

func TestOutboxStoreDLQBlocksLaterAssetEventsIntegration(t *testing.T) {
	pool := openMediaTestPool(t)
	ctx := context.Background()
	resetMediaTables(t, ctx, pool)

	const tenantID = "tenant-media-outbox-dlq"
	const assetID = "asset-dlq-1"
	insertMediaOutboxAsset(t, ctx, pool, tenantID, assetID)
	firstID := insertMediaOutboxEvent(t, ctx, pool, tenantID, assetID, "evt-dlq-uploaded", types.MediaEventAssetUploaded)
	secondID := insertMediaOutboxEvent(t, ctx, pool, tenantID, assetID, "evt-dlq-ready", types.MediaEventAssetReady)

	store := NewOutboxStore(pool)
	stats, err := store.ProcessReadyBatch(ctx, 10, 1, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for index := range errs {
			errs[index] = errors.New("kafka broker raw failure with internal detail")
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process failing batch: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 0 || stats.DeadLettered != 1 {
		t.Fatalf("unexpected failing batch stats: %+v", stats)
	}
	assertMediaOutboxStatus(t, ctx, pool, firstID, types.OutboxStatusDLQ)
	assertMediaOutboxStatus(t, ctx, pool, secondID, types.OutboxStatusPending)
	assertMediaOutboxLastError(t, ctx, pool, firstID, "media outbox publish broker unavailable")

	stats, err = store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		t.Fatalf("later event must stay blocked while prior event is DLQ: %+v", messages)
		return nil
	})
	if err != nil {
		t.Fatalf("process blocked batch: %v", err)
	}
	if stats.Fetched != 0 || stats.Published != 0 {
		t.Fatalf("expected no ready rows while lower id is DLQ, got %+v", stats)
	}
	assertMediaOutboxStatus(t, ctx, pool, secondID, types.OutboxStatusPending)
}

func TestOutboxStoreRetryKeepsStablePublicErrorIntegration(t *testing.T) {
	pool := openMediaTestPool(t)
	ctx := context.Background()
	resetMediaTables(t, ctx, pool)

	const tenantID = "tenant-media-outbox-retry"
	const assetID = "asset-retry-1"
	insertMediaOutboxAsset(t, ctx, pool, tenantID, assetID)
	eventID := insertMediaOutboxEvent(t, ctx, pool, tenantID, assetID, "evt-retry-uploaded", types.MediaEventAssetUploaded)

	store := NewOutboxStore(pool, WithOutboxClock(func() time.Time {
		return time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	}))
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for index := range errs {
			errs[index] = errors.New("duplicate key value violates unique constraint media_secret")
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process retry batch: %v", err)
	}
	if stats.Fetched != 1 || stats.Retried != 1 || stats.DeadLettered != 0 {
		t.Fatalf("unexpected retry stats: %+v", stats)
	}
	assertMediaOutboxStatus(t, ctx, pool, eventID, types.OutboxStatusPending)
	assertMediaOutboxRetry(t, ctx, pool, eventID, 1)
	assertMediaOutboxLastError(t, ctx, pool, eventID, "media outbox publish failed")
}

func insertMediaOutboxAsset(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID string, assetID string) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO media_assets (
    tenant_id,
    asset_id,
    owner_user_id,
    conversation_id,
    media_kind,
    content_type,
    file_name,
    size_bytes,
    sha256,
    object_key,
    status,
    scan_status,
    thumbnail_status,
    transcode_status
) VALUES ($1, $2, 'user-1', 'conv-1', 'IMAGE', 'image/png', 'image.png', 64, $3, $4, 'READY', 'PASSED', 'SKIPPED', 'SKIPPED')
`, tenantID, assetID, strings.Repeat("a", 64), tenantID+"/"+assetID+"/image.png")
	if err != nil {
		t.Fatalf("insert media asset: %v", err)
	}
}

func insertMediaOutboxEvent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID string, assetID string, eventID string, eventType string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(ctx, `
INSERT INTO media_outbox (
    event_id,
    tenant_id,
    asset_id,
    event_type,
    event_version,
    partition_key,
    payload_json
) VALUES ($1, $2, $3, $4, 1, $5, $6::jsonb)
RETURNING id
`, eventID, tenantID, assetID, eventType, tenantID+":"+assetID, mediaOutboxPayload(tenantID, assetID)).Scan(&id)
	if err != nil {
		t.Fatalf("insert media outbox event: %v", err)
	}
	return id
}

func mediaOutboxPayload(tenantID string, assetID string) string {
	return `{"tenant_id":"` + tenantID + `","asset_id":"` + assetID + `","conversation_id":"conv-1","media_kind":"IMAGE","content_type":"image/png","size_bytes":64,"sha256":"` + strings.Repeat("a", 64) + `","status":"READY"}`
}

func assertMediaOutboxStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id int64, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `SELECT status FROM media_outbox WHERE id = $1`, id).Scan(&got); err != nil {
		t.Fatalf("query media outbox status: %v", err)
	}
	if got != want {
		t.Fatalf("unexpected media outbox status for id %d: got %s want %s", id, got, want)
	}
}

func assertMediaOutboxRetry(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id int64, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, `SELECT retry_count FROM media_outbox WHERE id = $1`, id).Scan(&got); err != nil {
		t.Fatalf("query media outbox retry count: %v", err)
	}
	if got != want {
		t.Fatalf("unexpected media outbox retry count for id %d: got %d want %d", id, got, want)
	}
}

func assertMediaOutboxLastError(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id int64, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `SELECT last_error FROM media_outbox WHERE id = $1`, id).Scan(&got); err != nil {
		t.Fatalf("query media outbox last_error: %v", err)
	}
	if got != want {
		t.Fatalf("unexpected media outbox last_error for id %d: got %q want %q", id, got, want)
	}
}
