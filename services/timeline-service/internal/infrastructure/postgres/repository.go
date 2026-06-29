package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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

func (repository *Repository) ExpireSeqBlockLeases(
	ctx context.Context,
	command types.ExpireLeasesCommand,
) (types.ExpireLeasesResult, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.ExpireLeasesResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	rows, err := tx.Query(ctx, `
SELECT lease_id
FROM timeline_seq_block_leases
WHERE status = 'ACTIVE'
  AND expires_at <= $1
ORDER BY expires_at, tenant_id, conversation_id
LIMIT $2
FOR UPDATE SKIP LOCKED
`, command.Before, command.Limit)
	if err != nil {
		return types.ExpireLeasesResult{}, types.NewDBReadFailed(err.Error())
	}
	leaseIDs := make([]string, 0, command.Limit)
	for rows.Next() {
		var leaseID string
		if err := rows.Scan(&leaseID); err != nil {
			rows.Close()
			return types.ExpireLeasesResult{}, types.NewDBReadFailed(err.Error())
		}
		leaseIDs = append(leaseIDs, leaseID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return types.ExpireLeasesResult{}, types.NewDBReadFailed(err.Error())
	}
	rows.Close()

	if command.DryRun {
		if err := tx.Commit(ctx); err != nil {
			return types.ExpireLeasesResult{}, types.NewDBWriteFailed(err.Error())
		}
		return types.ExpireLeasesResult{Matched: len(leaseIDs), DryRun: true}, nil
	}

	expired := 0
	for _, leaseID := range leaseIDs {
		tag, err := tx.Exec(ctx, `
UPDATE timeline_seq_block_leases
SET status = 'EXPIRED',
    expired_at = now(),
    expired_by = $2,
    expire_reason = $3
WHERE lease_id = $1
  AND status = 'ACTIVE'
`, leaseID, command.OperatorID, command.Reason)
		if err != nil {
			return types.ExpireLeasesResult{}, types.NewDBWriteFailed(err.Error())
		}
		expired += int(tag.RowsAffected())
	}
	if err := tx.Commit(ctx); err != nil {
		return types.ExpireLeasesResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.ExpireLeasesResult{Matched: len(leaseIDs), Expired: expired}, nil
}

func (repository *Repository) CreateGapMarker(
	ctx context.Context,
	command types.GapMarkerCommand,
) (types.GapMarker, error) {
	markerID, err := newGapMarkerID()
	if err != nil {
		return types.GapMarker{}, types.NewDBWriteFailed(err.Error())
	}
	marker := types.GapMarker{
		MarkerID:       markerID,
		TenantID:       command.TenantID,
		ConversationID: command.ConversationID,
		StartSeq:       command.StartSeq,
		EndSeq:         command.EndSeq,
		SequencerEpoch: command.SequencerEpoch,
		LeaseID:        command.LeaseID,
		Reason:         command.Reason,
		Status:         "OPEN",
		CreatedBy:      command.OperatorID,
		CreatedAt:      time.Now().UTC(),
	}
	if command.DryRun {
		return marker, nil
	}
	inserted, err := scanGapMarker(repository.pool.QueryRow(ctx, `
INSERT INTO timeline_seq_gap_markers (
    marker_id,
    tenant_id,
    conversation_id,
    start_seq,
    end_seq,
    sequencer_epoch,
    lease_id,
    reason,
    status,
    created_by,
    created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'OPEN', $9, now())
RETURNING marker_id, tenant_id, conversation_id, start_seq, end_seq, sequencer_epoch, lease_id, reason, status, created_by, created_at, closed_by, closed_at, close_reason
`,
		marker.MarkerID,
		command.TenantID,
		command.ConversationID,
		command.StartSeq,
		command.EndSeq,
		command.SequencerEpoch,
		command.LeaseID,
		command.Reason,
		command.OperatorID,
	))
	if err != nil {
		return types.GapMarker{}, types.NewDBWriteFailed(err.Error())
	}
	return inserted, nil
}

func (repository *Repository) CloseGapMarker(
	ctx context.Context,
	command types.CloseGapMarkerCommand,
) (types.GapMarker, error) {
	marker, err := repository.getGapMarker(ctx, command.MarkerID)
	if err != nil {
		return types.GapMarker{}, err
	}
	if command.DryRun {
		return marker, nil
	}
	closed, err := scanGapMarker(repository.pool.QueryRow(ctx, `
UPDATE timeline_seq_gap_markers
SET status = 'CLOSED',
    closed_by = $2,
    closed_at = now(),
    close_reason = $3
WHERE marker_id = $1
  AND status = 'OPEN'
RETURNING marker_id, tenant_id, conversation_id, start_seq, end_seq, sequencer_epoch, lease_id, reason, status, created_by, created_at, closed_by, closed_at, close_reason
`, command.MarkerID, command.OperatorID, command.CloseReason))
	if err != nil {
		if err == pgx.ErrNoRows {
			return types.GapMarker{}, types.NewInvalidArgument("gap marker is not open")
		}
		return types.GapMarker{}, types.NewDBWriteFailed(err.Error())
	}
	return closed, nil
}

func (repository *Repository) AuditGapMarkers(
	ctx context.Context,
	tenantID string,
	conversationID string,
	status string,
	limit int,
) ([]types.GapMarker, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := repository.pool.Query(ctx, `
SELECT marker_id, tenant_id, conversation_id, start_seq, end_seq, sequencer_epoch, lease_id, reason, status, created_by, created_at, closed_by, closed_at, close_reason
FROM timeline_seq_gap_markers
WHERE ($1 = '' OR tenant_id = $1)
  AND ($2 = '' OR conversation_id = $2)
  AND ($3 = '' OR status = $3)
ORDER BY created_at DESC
LIMIT $4
`, tenantID, conversationID, status, limit)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()
	markers := make([]types.GapMarker, 0)
	for rows.Next() {
		marker, err := scanGapMarker(rows)
		if err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		markers = append(markers, marker)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return markers, nil
}

func (repository *Repository) getGapMarker(ctx context.Context, markerID string) (types.GapMarker, error) {
	marker, err := scanGapMarker(repository.pool.QueryRow(ctx, `
SELECT marker_id, tenant_id, conversation_id, start_seq, end_seq, sequencer_epoch, lease_id, reason, status, created_by, created_at, closed_by, closed_at, close_reason
FROM timeline_seq_gap_markers
WHERE marker_id = $1
`, markerID))
	if err != nil {
		if err == pgx.ErrNoRows {
			return types.GapMarker{}, types.NewInvalidArgument("gap marker not found")
		}
		return types.GapMarker{}, types.NewDBReadFailed(err.Error())
	}
	return marker, nil
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

func newGapMarkerID() (string, error) {
	var randomBytes [16]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		return "", err
	}
	return "gap_" + hex.EncodeToString(randomBytes[:]), nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanGapMarker(scanner rowScanner) (types.GapMarker, error) {
	var marker types.GapMarker
	var closedBy pgtype.Text
	var closedAt pgtype.Timestamptz
	var closeReason pgtype.Text
	if err := scanner.Scan(
		&marker.MarkerID,
		&marker.TenantID,
		&marker.ConversationID,
		&marker.StartSeq,
		&marker.EndSeq,
		&marker.SequencerEpoch,
		&marker.LeaseID,
		&marker.Reason,
		&marker.Status,
		&marker.CreatedBy,
		&marker.CreatedAt,
		&closedBy,
		&closedAt,
		&closeReason,
	); err != nil {
		return types.GapMarker{}, err
	}
	if closedBy.Valid {
		marker.ClosedBy = closedBy.String
	}
	if closedAt.Valid {
		value := closedAt.Time
		marker.ClosedAt = &value
	}
	if closeReason.Valid {
		marker.CloseReason = closeReason.String
	}
	return marker, nil
}
