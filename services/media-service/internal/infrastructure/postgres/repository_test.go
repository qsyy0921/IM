package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qsyy0921/IM/services/media-service/internal/app"
	"github.com/qsyy0921/IM/services/media-service/internal/types"
)

const mediaTestSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestRepositoryUploadCompleteDeleteIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openMediaTestPool(t)
	resetMediaTables(t, ctx, pool)
	repository := NewRepository(pool)

	command := createUploadCommand()
	allocation := app.UploadAllocation{
		AssetID:         "asset-test-1",
		UploadSessionID: "upload-test-1",
		ObjectKey:       "tenant-1/conv-1/internal-object-key",
		ExpiresAt:       time.Now().Add(time.Hour),
	}
	asset, session, err := repository.CreateUploadSession(ctx, command, allocation)
	if err != nil {
		t.Fatalf("create upload session: %v", err)
	}
	if asset.ObjectKey != allocation.ObjectKey {
		t.Fatalf("repository should keep internal object key: %+v", asset)
	}
	if session.CommandHash != command.CommandHash() || session.Status != "PENDING" {
		t.Fatalf("unexpected session: %+v", session)
	}

	replayAsset, replaySession, err := repository.CreateUploadSession(ctx, command, app.UploadAllocation{
		AssetID:         "asset-should-not-win",
		UploadSessionID: "upload-should-not-win",
		ObjectKey:       "tenant-1/conv-1/other-key",
		ExpiresAt:       time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("replay upload session: %v", err)
	}
	if replayAsset.AssetID != asset.AssetID || replaySession.UploadSessionID != session.UploadSessionID {
		t.Fatalf("idempotent replay returned different session: %+v %+v", replayAsset, replaySession)
	}

	conflict := command
	conflict.SizeBytes = 128
	if _, _, err := repository.CreateUploadSession(ctx, conflict, app.UploadAllocation{
		AssetID:         "asset-conflict",
		UploadSessionID: "upload-conflict",
		ObjectKey:       "tenant-1/conv-1/conflict",
		ExpiresAt:       time.Now().Add(time.Hour),
	}); !errors.Is(err, types.ErrAlreadyExists) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}

	processing, err := repository.CompleteUpload(ctx, types.CompleteUploadCommand{
		AuthContext:     command.AuthContext,
		AssetID:         asset.AssetID,
		UploadSessionID: session.UploadSessionID,
		SHA256:          mediaTestSHA,
		SizeBytes:       64,
	}, types.ObjectMetadata{SizeBytes: 64, SHA256: mediaTestSHA})
	if err != nil {
		t.Fatalf("complete upload: %v", err)
	}
	if processing.Status != types.AssetStatusProcessing ||
		processing.ScanStatus != types.ProcessingStatusPending ||
		processing.ThumbnailStatus != types.ProcessingStatusPending ||
		processing.TranscodeStatus != types.ProcessingStatusSkipped {
		t.Fatalf("unexpected processing asset: %+v", processing)
	}
	assertMediaOutboxSafe(t, ctx, pool, asset.AssetID, "media.asset.uploaded.v1", allocation.ObjectKey)
	assertOutboxEventCount(t, ctx, pool, asset.AssetID, "media.asset.ready.v1", 0)
	assertProcessingJobCount(t, ctx, pool, asset.AssetID, 2)

	ready := runRepositoryProcessingJobs(t, ctx, repository, asset.AssetID)
	if ready.Status != types.AssetStatusReady ||
		ready.ScanStatus != types.ProcessingStatusPassed ||
		ready.ThumbnailStatus != types.ProcessingStatusPassed ||
		ready.TranscodeStatus != types.ProcessingStatusSkipped {
		t.Fatalf("unexpected ready asset after processing: %+v", ready)
	}

	replayedReady, err := repository.CompleteUpload(ctx, types.CompleteUploadCommand{
		AuthContext:     command.AuthContext,
		AssetID:         asset.AssetID,
		UploadSessionID: session.UploadSessionID,
		SHA256:          mediaTestSHA,
		SizeBytes:       64,
	}, types.ObjectMetadata{SizeBytes: 64, SHA256: mediaTestSHA})
	if err != nil {
		t.Fatalf("complete upload replay: %v", err)
	}
	if replayedReady.AssetID != ready.AssetID || replayedReady.Status != types.AssetStatusReady {
		t.Fatalf("unexpected replayed ready asset: %+v", replayedReady)
	}

	assertMediaOutboxSafe(t, ctx, pool, asset.AssetID, "media.asset.ready.v1", allocation.ObjectKey)

	if err := repository.RecordAccessAudit(ctx, types.AccessAudit{
		TenantID:       command.AuthContext.TenantID,
		AuditID:        "audit-test-1",
		AssetID:        asset.AssetID,
		UserID:         command.AuthContext.UserID,
		ConversationID: command.ConversationID,
		MessageID:      "msg-1",
		Variant:        types.VariantOriginal,
		Decision:       "ALLOW",
		DecisionSource: "test",
		RequestID:      "req-1",
	}); err != nil {
		t.Fatalf("record access audit: %v", err)
	}

	deleted, err := repository.DeleteMediaAsset(ctx, types.DeleteMediaAssetCommand{
		AuthContext:     command.AuthContext,
		AssetID:         asset.AssetID,
		DeleteRequestID: "delete-1",
		Reason:          "test",
	})
	if err != nil {
		t.Fatalf("delete media asset: %v", err)
	}
	if deleted.Status != types.AssetStatusDeleted || deleted.DeletedAt.IsZero() {
		t.Fatalf("unexpected deleted asset: %+v", deleted)
	}
	assertMediaOutboxSafe(t, ctx, pool, asset.AssetID, "media.asset.deleted.v1", allocation.ObjectKey)
}

func TestRepositoryProcessingJobFailureRetriesAndDLQIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openMediaTestPool(t)
	resetMediaTables(t, ctx, pool)
	repository := NewRepository(pool)

	command := createUploadCommand()
	asset, session, err := repository.CreateUploadSession(ctx, command, app.UploadAllocation{
		AssetID:         "asset-processing-failure",
		UploadSessionID: "upload-processing-failure",
		ObjectKey:       "tenant-1/conv-1/processing-failure-key",
		ExpiresAt:       time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create upload session: %v", err)
	}
	if _, err := repository.CompleteUpload(ctx, types.CompleteUploadCommand{
		AuthContext:     command.AuthContext,
		AssetID:         asset.AssetID,
		UploadSessionID: session.UploadSessionID,
		SHA256:          mediaTestSHA,
		SizeBytes:       64,
	}, types.ObjectMetadata{SizeBytes: 64, SHA256: mediaTestSHA}); err != nil {
		t.Fatalf("complete upload: %v", err)
	}
	jobs, err := repository.ClaimProcessingJobs(ctx, 1)
	if err != nil {
		t.Fatalf("claim processing jobs: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Status != types.ProcessingJobStatusRunning || jobs[0].AttemptCount != 1 {
		t.Fatalf("unexpected claimed job: %+v", jobs)
	}
	deadLettered, err := repository.MarkProcessingJobFailed(ctx, jobs[0], errors.New("raw provider error should not leak"), 2, time.Millisecond)
	if err != nil {
		t.Fatalf("mark processing failed: %v", err)
	}
	if deadLettered {
		t.Fatalf("first failure should retry, not DLQ")
	}
	assertProcessingJobState(t, ctx, pool, jobs[0].JobID, types.ProcessingJobStatusFailed, "media processing failed")

	time.Sleep(2 * time.Millisecond)
	jobs, err = repository.ClaimProcessingJobs(ctx, 1)
	if err != nil {
		t.Fatalf("claim retry processing job: %v", err)
	}
	if len(jobs) != 1 || jobs[0].AttemptCount != 2 {
		t.Fatalf("unexpected retry job: %+v", jobs)
	}
	deadLettered, err = repository.MarkProcessingJobFailed(ctx, jobs[0], errors.New("second raw provider error"), 2, time.Millisecond)
	if err != nil {
		t.Fatalf("mark processing dlq: %v", err)
	}
	if !deadLettered {
		t.Fatalf("second failure should DLQ")
	}
	assertProcessingJobState(t, ctx, pool, jobs[0].JobID, types.ProcessingJobStatusDLQ, "media processing failed")
}

func TestRepositoryCompleteUploadMismatchDoesNotWriteOutboxIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openMediaTestPool(t)
	resetMediaTables(t, ctx, pool)
	repository := NewRepository(pool)

	command := createUploadCommand()
	asset, session, err := repository.CreateUploadSession(ctx, command, app.UploadAllocation{
		AssetID:         "asset-mismatch",
		UploadSessionID: "upload-mismatch",
		ObjectKey:       "tenant-1/conv-1/mismatch-key",
		ExpiresAt:       time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create upload session: %v", err)
	}

	_, err = repository.CompleteUpload(ctx, types.CompleteUploadCommand{
		AuthContext:     command.AuthContext,
		AssetID:         asset.AssetID,
		UploadSessionID: session.UploadSessionID,
		SHA256:          strings.Repeat("b", 64),
		SizeBytes:       64,
	}, types.ObjectMetadata{SizeBytes: 64, SHA256: strings.Repeat("b", 64)})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected metadata mismatch invalid argument, got %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM media_outbox
WHERE tenant_id = $1
  AND asset_id = $2
`, command.AuthContext.TenantID, asset.AssetID).Scan(&count); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if count != 0 {
		t.Fatalf("metadata mismatch should not write outbox, got %d rows", count)
	}
}

func createUploadCommand() types.CreateUploadSessionCommand {
	return types.CreateUploadSessionCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID: "conv-1",
		MediaKind:      types.MediaKindImage,
		FileName:       "image.png",
		ContentType:    "image/png",
		SizeBytes:      64,
		SHA256:         mediaTestSHA,
		IdempotencyKey: "idem-1",
	}.Normalized()
}

func assertMediaOutboxSafe(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	assetID string,
	eventType string,
	objectKey string,
) {
	t.Helper()
	var payloadRaw []byte
	if err := pool.QueryRow(ctx, `
SELECT payload_json
FROM media_outbox
WHERE tenant_id = 'tenant-1'
  AND asset_id = $1
  AND event_type = $2
`, assetID, eventType).Scan(&payloadRaw); err != nil {
		t.Fatalf("query outbox %s: %v", eventType, err)
	}
	payloadText := string(payloadRaw)
	if strings.Contains(payloadText, objectKey) ||
		strings.Contains(payloadText, "object_key") ||
		strings.Contains(payloadText, "download_url") {
		t.Fatalf("media outbox payload leaked internal data: %s", payloadText)
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadRaw, &payload); err != nil {
		t.Fatalf("decode outbox payload: %v", err)
	}
	if payload["asset_id"] != assetID {
		t.Fatalf("unexpected payload asset id: %+v", payload)
	}
}

func assertOutboxEventCount(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	assetID string,
	eventType string,
	expected int,
) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM media_outbox
WHERE tenant_id = 'tenant-1'
  AND asset_id = $1
  AND event_type = $2
`, assetID, eventType).Scan(&count); err != nil {
		t.Fatalf("count outbox %s: %v", eventType, err)
	}
	if count != expected {
		t.Fatalf("expected %d %s events, got %d", expected, eventType, count)
	}
}

func assertProcessingJobCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, assetID string, expected int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM media_processing_jobs
WHERE tenant_id = 'tenant-1'
  AND asset_id = $1
`, assetID).Scan(&count); err != nil {
		t.Fatalf("count processing jobs: %v", err)
	}
	if count != expected {
		t.Fatalf("expected %d processing jobs, got %d", expected, count)
	}
}

func runRepositoryProcessingJobs(
	t *testing.T,
	ctx context.Context,
	repository *Repository,
	assetID string,
) types.MediaAsset {
	t.Helper()
	var ready types.MediaAsset
	for {
		jobs, err := repository.ClaimProcessingJobs(ctx, 10)
		if err != nil {
			t.Fatalf("claim processing jobs: %v", err)
		}
		if len(jobs) == 0 {
			break
		}
		for _, job := range jobs {
			asset, err := repository.MarkProcessingJobSucceeded(ctx, job)
			if err != nil {
				t.Fatalf("mark processing succeeded %s: %v", job.JobType, err)
			}
			if asset.AssetID == assetID {
				ready = asset
			}
		}
	}
	return ready
}

func assertProcessingJobState(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	jobID string,
	expectedStatus string,
	expectedLastError string,
) {
	t.Helper()
	var status string
	var lastError string
	if err := pool.QueryRow(ctx, `
SELECT status, last_error
FROM media_processing_jobs
WHERE tenant_id = 'tenant-1'
  AND job_id = $1
`, jobID).Scan(&status, &lastError); err != nil {
		t.Fatalf("query processing job state: %v", err)
	}
	if status != expectedStatus || lastError != expectedLastError {
		t.Fatalf("unexpected processing job state: status=%s last_error=%q", status, lastError)
	}
}

func openMediaTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	t.Cleanup(pool.Close)
	applyMediaMigrations(t, context.Background(), pool)
	return pool
}

func applyMediaMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "media")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migration dir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", entry.Name(), err)
		}
		if _, err := pool.Exec(ctx, string(content)); err != nil {
			t.Fatalf("apply migration %s: %v", entry.Name(), err)
		}
	}
}

func resetMediaTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
TRUNCATE
	media_access_audit,
	media_outbox,
	media_processing_jobs,
	media_upload_sessions,
	media_assets
`); err != nil {
		t.Fatalf("reset media tables: %v", err)
	}
}
