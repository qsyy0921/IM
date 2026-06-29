package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/timeline-service/internal/types"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (repository *Repository) AllocateSeqBlock(
	ctx context.Context,
	command types.AllocateSeqBlockCommand,
	leaseTTL time.Duration,
) (types.SeqBlockLease, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.SeqBlockLease{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	commandHash := seqBlockCommandHash(command)
	existing, found, err := lockExistingSeqBlockLease(ctx, tx, command)
	if err != nil {
		return types.SeqBlockLease{}, err
	}
	if found {
		if existing.commandHash != commandHash {
			return types.SeqBlockLease{}, types.NewIdempotencyConflict("same idempotency key with different command")
		}
		if err := tx.Commit(ctx); err != nil {
			return types.SeqBlockLease{}, types.NewDBWriteFailed(err.Error())
		}
		return existing.lease, nil
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO timeline_sequence_state (
    tenant_id,
    conversation_id,
    next_seq,
    sequencer_epoch,
    updated_at
) VALUES (
    $1,
    $2,
    GREATEST($3, 1),
    1,
    now()
)
ON CONFLICT (tenant_id, conversation_id) DO NOTHING
`, command.TenantID, command.ConversationID, command.MinimumStartSeq); err != nil {
		return types.SeqBlockLease{}, types.NewDBWriteFailed(err.Error())
	}

	var nextSeq int64
	var epoch int64
	if err := tx.QueryRow(ctx, `
SELECT next_seq, sequencer_epoch
FROM timeline_sequence_state
WHERE tenant_id = $1
  AND conversation_id = $2
FOR UPDATE
`, command.TenantID, command.ConversationID).Scan(&nextSeq, &epoch); err != nil {
		if err == pgx.ErrNoRows {
			return types.SeqBlockLease{}, types.NewDBReadFailed("timeline sequence state not found")
		}
		return types.SeqBlockLease{}, types.NewDBReadFailed(err.Error())
	}

	startSeq := maxInt64(nextSeq, maxInt64(command.MinimumStartSeq, 1))
	endSeq := startSeq + int64(command.BlockSize) - 1
	leaseID, err := newLeaseID()
	if err != nil {
		return types.SeqBlockLease{}, types.NewDBWriteFailed(err.Error())
	}
	expiresAt := time.Now().UTC().Add(leaseTTL)

	if _, err := tx.Exec(ctx, `
UPDATE timeline_sequence_state
SET next_seq = $3,
    updated_at = now()
WHERE tenant_id = $1
  AND conversation_id = $2
`, command.TenantID, command.ConversationID, endSeq+1); err != nil {
		return types.SeqBlockLease{}, types.NewDBWriteFailed(err.Error())
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO timeline_seq_block_leases (
    lease_id,
    tenant_id,
    conversation_id,
    start_seq,
    end_seq,
    block_size,
    sequencer_epoch,
    requester_id,
    idempotency_key,
    command_hash,
    expires_at,
    created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now())
`, leaseID, command.TenantID, command.ConversationID, startSeq, endSeq, command.BlockSize, epoch, command.RequesterID, command.IdempotencyKey, commandHash, expiresAt); err != nil {
		return types.SeqBlockLease{}, types.NewDBWriteFailed(err.Error())
	}

	if err := tx.Commit(ctx); err != nil {
		return types.SeqBlockLease{}, types.NewDBWriteFailed(err.Error())
	}
	return types.SeqBlockLease{
		TenantID:       command.TenantID,
		ConversationID: command.ConversationID,
		StartSeq:       startSeq,
		EndSeq:         endSeq,
		BlockSize:      command.BlockSize,
		SequencerEpoch: epoch,
		LeaseID:        leaseID,
		ExpiresAt:      expiresAt,
	}, nil
}

type storedSeqBlockLease struct {
	lease       types.SeqBlockLease
	commandHash string
}

func lockExistingSeqBlockLease(
	ctx context.Context,
	tx pgx.Tx,
	command types.AllocateSeqBlockCommand,
) (storedSeqBlockLease, bool, error) {
	var stored storedSeqBlockLease
	if err := tx.QueryRow(ctx, `
SELECT
    lease_id,
    tenant_id,
    conversation_id,
    start_seq,
    end_seq,
    block_size,
    sequencer_epoch,
    expires_at,
    command_hash
FROM timeline_seq_block_leases
WHERE tenant_id = $1
  AND conversation_id = $2
  AND requester_id = $3
  AND idempotency_key = $4
FOR UPDATE
`, command.TenantID, command.ConversationID, command.RequesterID, command.IdempotencyKey).Scan(
		&stored.lease.LeaseID,
		&stored.lease.TenantID,
		&stored.lease.ConversationID,
		&stored.lease.StartSeq,
		&stored.lease.EndSeq,
		&stored.lease.BlockSize,
		&stored.lease.SequencerEpoch,
		&stored.lease.ExpiresAt,
		&stored.commandHash,
	); err != nil {
		if err == pgx.ErrNoRows {
			return storedSeqBlockLease{}, false, nil
		}
		return storedSeqBlockLease{}, false, types.NewDBReadFailed(err.Error())
	}
	stored.lease.IdempotentReplay = true
	return stored, true, nil
}

func seqBlockCommandHash(command types.AllocateSeqBlockCommand) string {
	plain := fmt.Sprintf(
		"%s\x00%s\x00%s\x00%d",
		command.TenantID,
		command.ConversationID,
		command.RequesterID,
		command.BlockSize,
	)
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func maxInt64(left int64, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func newLeaseID() (string, error) {
	var randomBytes [16]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		return "", err
	}
	return "seqblk_" + hex.EncodeToString(randomBytes[:]), nil
}
