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
	path := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "timeline", "000001_timeline_seq_blocks.sql")
	sqlBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read timeline migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
		t.Fatalf("apply timeline migration: %v", err)
	}
}

func resetTimelineTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
TRUNCATE
    timeline_seq_block_leases,
    timeline_sequence_state
`); err != nil {
		t.Fatalf("reset timeline tables: %v", err)
	}
}
