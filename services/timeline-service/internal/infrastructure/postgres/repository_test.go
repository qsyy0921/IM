package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/timeline-service/internal/types"
)

func TestRepositoryAllocateSeqBlockIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetTimelineTables(t, ctx, pool)
	repository := NewRepository(pool)

	first, err := repository.AllocateSeqBlock(ctx, types.AllocateSeqBlockCommand{
		TenantID:       "tenant-timeline",
		ConversationID: "conversation-hot",
		RequesterID:    "message-service-a",
		BlockSize:      10,
		IdempotencyKey: "request-1",
	}, time.Minute)
	if err != nil {
		t.Fatalf("allocate first block: %v", err)
	}
	if first.StartSeq != 1 || first.EndSeq != 10 || first.SequencerEpoch != 1 {
		t.Fatalf("unexpected first block: %+v", first)
	}

	replay, err := repository.AllocateSeqBlock(ctx, types.AllocateSeqBlockCommand{
		TenantID:       "tenant-timeline",
		ConversationID: "conversation-hot",
		RequesterID:    "message-service-a",
		BlockSize:      10,
		IdempotencyKey: "request-1",
	}, time.Minute)
	if err != nil {
		t.Fatalf("replay first block: %v", err)
	}
	if !replay.IdempotentReplay || replay.LeaseID != first.LeaseID || replay.StartSeq != first.StartSeq {
		t.Fatalf("unexpected replay block: %+v first=%+v", replay, first)
	}

	second, err := repository.AllocateSeqBlock(ctx, types.AllocateSeqBlockCommand{
		TenantID:       "tenant-timeline",
		ConversationID: "conversation-hot",
		RequesterID:    "message-service-a",
		BlockSize:      5,
		IdempotencyKey: "request-2",
	}, time.Minute)
	if err != nil {
		t.Fatalf("allocate second block: %v", err)
	}
	if second.StartSeq != 11 || second.EndSeq != 15 {
		t.Fatalf("unexpected second block: %+v", second)
	}

	_, err = repository.AllocateSeqBlock(ctx, types.AllocateSeqBlockCommand{
		TenantID:       "tenant-timeline",
		ConversationID: "conversation-hot",
		RequesterID:    "message-service-a",
		BlockSize:      9,
		IdempotencyKey: "request-1",
	}, time.Minute)
	if !errors.Is(err, types.ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
}

func TestRepositoryAllocateSeqBlockHonorsMinimumStartSeqIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetTimelineTables(t, ctx, pool)

	repository := NewRepository(pool)
	lease, err := repository.AllocateSeqBlock(ctx, types.AllocateSeqBlockCommand{
		TenantID:        "tenant-timeline-floor",
		ConversationID:  "conversation-hot",
		RequesterID:     "message-service-a",
		BlockSize:       3,
		IdempotencyKey:  "request-after-floor",
		MinimumStartSeq: 8,
	}, time.Minute)
	if err != nil {
		t.Fatalf("allocate block after floor: %v", err)
	}
	if lease.StartSeq != 8 || lease.EndSeq != 10 {
		t.Fatalf("unexpected floor lease: %+v", lease)
	}
}

func TestRepositoryExpireSeqBlockLeasesIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetTimelineTables(t, ctx, pool)
	repository := NewRepository(pool)

	lease, err := repository.AllocateSeqBlock(ctx, types.AllocateSeqBlockCommand{
		TenantID:       "tenant-expire",
		ConversationID: "conversation-hot",
		RequesterID:    "message-service-a",
		BlockSize:      4,
		IdempotencyKey: "request-expire",
	}, -time.Second)
	if err != nil {
		t.Fatalf("allocate expired lease: %v", err)
	}

	dryRun, err := repository.ExpireSeqBlockLeases(ctx, types.ExpireLeasesCommand{
		Before:     time.Now().UTC(),
		Limit:      10,
		DryRun:     true,
		OperatorID: "operator-test",
		Reason:     "integration dry run",
	})
	if err != nil {
		t.Fatalf("dry run expire leases: %v", err)
	}
	if dryRun.Matched != 1 || dryRun.Expired != 0 || !dryRun.DryRun {
		t.Fatalf("unexpected dry run result: %+v", dryRun)
	}

	result, err := repository.ExpireSeqBlockLeases(ctx, types.ExpireLeasesCommand{
		Before:     time.Now().UTC(),
		Limit:      10,
		OperatorID: "operator-test",
		Reason:     "integration expire",
	})
	if err != nil {
		t.Fatalf("expire leases: %v", err)
	}
	if result.Matched != 1 || result.Expired != 1 {
		t.Fatalf("unexpected expire result: %+v", result)
	}

	var status string
	if err := pool.QueryRow(ctx, `
SELECT status
FROM timeline_seq_block_leases
WHERE lease_id = $1
`, lease.LeaseID).Scan(&status); err != nil {
		t.Fatalf("read lease status: %v", err)
	}
	if status != "EXPIRED" {
		t.Fatalf("lease status = %s, want EXPIRED", status)
	}
}

func TestRepositoryGapMarkerLifecycleIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetTimelineTables(t, ctx, pool)
	repository := NewRepository(pool)

	marker, err := repository.CreateGapMarker(ctx, types.GapMarkerCommand{
		TenantID:       "tenant-gap",
		ConversationID: "conversation-hot",
		StartSeq:       100,
		EndSeq:         120,
		SequencerEpoch: 3,
		LeaseID:        "seqblk-test",
		Reason:         "lease expired before consumption",
		OperatorID:     "operator-test",
	})
	if err != nil {
		t.Fatalf("create gap marker: %v", err)
	}
	if marker.MarkerID == "" || marker.Status != "OPEN" {
		t.Fatalf("unexpected gap marker: %+v", marker)
	}

	rows, err := repository.AuditGapMarkers(ctx, "tenant-gap", "conversation-hot", "OPEN", 10)
	if err != nil {
		t.Fatalf("audit gap markers: %v", err)
	}
	if len(rows) != 1 || rows[0].MarkerID != marker.MarkerID {
		t.Fatalf("unexpected audit rows: %+v", rows)
	}

	closed, err := repository.CloseGapMarker(ctx, types.CloseGapMarkerCommand{
		MarkerID:    marker.MarkerID,
		OperatorID:  "operator-test",
		CloseReason: "projection accepted explicit gap",
	})
	if err != nil {
		t.Fatalf("close gap marker: %v", err)
	}
	if closed.Status != "CLOSED" || closed.ClosedAt == nil {
		t.Fatalf("unexpected closed marker: %+v", closed)
	}
}

func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	t.Cleanup(pool.Close)
	applyTimelineMigrations(t, ctx, pool)
	return pool
}

func applyTimelineMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "timeline")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read timeline migrations: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		sqlBytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read timeline migration %s: %v", entry.Name(), err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("apply timeline migration %s: %v", entry.Name(), err)
		}
	}
}

func resetTimelineTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
TRUNCATE
    timeline_seq_gap_markers,
    timeline_seq_block_leases,
    timeline_sequence_state
`); err != nil {
		t.Fatalf("reset timeline tables: %v", err)
	}
}
