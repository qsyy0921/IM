package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/message-service/internal/domain"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

const commandTypeDelete = "DELETE"

type deleteReplayResult struct {
	MessageID       types.MessageID      `json:"message_id"`
	ConversationID  types.ConversationID `json:"conversation_id"`
	ConversationSeq int64                `json:"conversation_seq"`
	ChangeVersion   int32                `json:"change_version"`
	AcceptedAt      string               `json:"accepted_at"`
}

func (r *MessageRepository) DeleteMessage(
	ctx context.Context,
	input domain.DeleteMessageInput,
) (domain.MessageChangeResult, error) {
	if r.pool == nil {
		return domain.MessageChangeResult{}, ErrRepositoryNotConfigured
	}
	commandHash, err := domain.ComputeDeleteMessageCommandHash(input.Command)
	if err != nil {
		return domain.MessageChangeResult{}, err
	}
	if err := r.checkBackpressure(); err != nil {
		return domain.MessageChangeResult{}, err
	}

	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return domain.MessageChangeResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return domain.MessageChangeResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := lockDeleteIdempotency(ctx, tx, input.Command); err != nil {
		return domain.MessageChangeResult{}, err
	}
	if existing, ok, err := findExistingDeleteCommand(ctx, tx, input.Command); err != nil {
		return domain.MessageChangeResult{}, err
	} else if ok {
		return replayDeleteResult(ctx, tx, existing, commandHash)
	}

	message, err := lockMessageForDelete(ctx, tx, input.Command)
	if err != nil {
		return domain.MessageChangeResult{}, err
	}
	if !canMutateMessageOwnership(message, input.Command.AuthContext.UserID, input.Permission) {
		return domain.MessageChangeResult{}, types.NewPermissionDenied("only the original sender can delete this message in phase 1")
	}
	if !message.CanDelete() {
		return domain.MessageChangeResult{}, types.NewInvalidMessageState("message cannot be deleted")
	}
	if err := ensureMessageNotUnderLegalHold(ctx, tx, input.Command); err != nil {
		return domain.MessageChangeResult{}, err
	}
	if input.Command.DeleteScope == types.DeleteScopeCompliance {
		if err := lockApprovedComplianceDeleteApproval(ctx, tx, input.Command); err != nil {
			return domain.MessageChangeResult{}, err
		}
	}

	if err := ensureConversationSeqFor(ctx, tx, input.Command.AuthContext.TenantID, input.Command.ConversationID); err != nil {
		return domain.MessageChangeResult{}, err
	}
	seq, err := allocateConversationSeqFor(ctx, tx, input.Command.AuthContext.TenantID, input.Command.ConversationID)
	if err != nil {
		return domain.MessageChangeResult{}, err
	}
	changeVersion, err := nextDeleteMessageChangeVersion(ctx, tx, input.Command)
	if err != nil {
		return domain.MessageChangeResult{}, err
	}
	eventID, err := r.eventID()
	if err != nil {
		return domain.MessageChangeResult{}, err
	}
	acceptedAt := r.now()
	record, err := domain.NewDeleteMessageRecord(input, message, eventID, seq, changeVersion, acceptedAt)
	if err != nil {
		return domain.MessageChangeResult{}, err
	}

	if err := updateMessageDeleted(ctx, tx, record); err != nil {
		return domain.MessageChangeResult{}, err
	}
	if err := insertDeleteMessageChangeHistory(ctx, tx, input, record); err != nil {
		return domain.MessageChangeResult{}, err
	}
	if input.Command.DeleteScope == types.DeleteScopeCompliance {
		if err := consumeComplianceDeleteApproval(ctx, tx, input.Command, record); err != nil {
			return domain.MessageChangeResult{}, err
		}
	}
	if err := insertDeleteCommandResult(ctx, tx, input.Command, record); err != nil {
		return domain.MessageChangeResult{}, err
	}
	if err := insertDeleteMessageTimelineEvent(ctx, tx, input, record); err != nil {
		return domain.MessageChangeResult{}, err
	}
	if err := insertMessageChangeOutboxEvent(ctx, tx, record); err != nil {
		return domain.MessageChangeResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.MessageChangeResult{}, types.NewDBWriteFailed(err.Error())
	}
	return domain.MessageChangeResult{
		MessageID:        record.MessageID,
		ConversationSeq:  record.ConversationSeq,
		ChangeVersion:    record.ChangeVersion,
		AcceptedAt:       record.ChangedAt,
		IdempotentReplay: false,
	}, nil
}

func lockDeleteIdempotency(ctx context.Context, tx pgx.Tx, command types.DeleteMessageCommand) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, deleteIdempotencyLockKey(command))
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func deleteIdempotencyLockKey(command types.DeleteMessageCommand) string {
	return fmt.Sprintf(
		"%s\x1f%s\x1f%s\x1f%s\x1f%s",
		command.AuthContext.TenantID,
		command.ConversationID,
		command.MessageID,
		commandTypeDelete,
		command.IdempotencyKey,
	)
}

type existingDeleteCommand struct {
	CommandHash string
	ResultJSON  []byte
}

func findExistingDeleteCommand(
	ctx context.Context,
	tx pgx.Tx,
	command types.DeleteMessageCommand,
) (existingDeleteCommand, bool, error) {
	row := tx.QueryRow(ctx, `
SELECT command_hash, result_json
FROM message_command_idempotency
WHERE tenant_id = $1
  AND conversation_id = $2
  AND command_type = $3
  AND message_id = $4
  AND idempotency_key = $5
FOR UPDATE
`, command.AuthContext.TenantID, command.ConversationID, commandTypeDelete, command.MessageID, command.IdempotencyKey)
	var existing existingDeleteCommand
	if err := row.Scan(&existing.CommandHash, &existing.ResultJSON); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return existingDeleteCommand{}, false, nil
		}
		return existingDeleteCommand{}, false, types.NewDBWriteFailed(err.Error())
	}
	return existing, true, nil
}

func replayDeleteResult(
	ctx context.Context,
	tx pgx.Tx,
	existing existingDeleteCommand,
	commandHash string,
) (domain.MessageChangeResult, error) {
	if existing.CommandHash != commandHash {
		return domain.MessageChangeResult{}, types.NewIdempotencyConflict("idempotency_key reused with different delete command")
	}
	var replay deleteReplayResult
	if err := json.Unmarshal(existing.ResultJSON, &replay); err != nil {
		return domain.MessageChangeResult{}, types.NewDBWriteFailed(err.Error())
	}
	acceptedAt, err := time.Parse(time.RFC3339Nano, replay.AcceptedAt)
	if err != nil {
		return domain.MessageChangeResult{}, types.NewDBWriteFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.MessageChangeResult{}, types.NewDBWriteFailed(err.Error())
	}
	return domain.MessageChangeResult{
		MessageID:        replay.MessageID,
		ConversationSeq:  replay.ConversationSeq,
		ChangeVersion:    replay.ChangeVersion,
		AcceptedAt:       acceptedAt,
		IdempotentReplay: true,
	}, nil
}

func lockMessageForDelete(
	ctx context.Context,
	tx pgx.Tx,
	command types.DeleteMessageCommand,
) (domain.Message, error) {
	row := tx.QueryRow(ctx, `
SELECT
    conversation_seq,
    message_id,
    sender_id,
    device_id,
    client_msg_id,
    command_hash,
    message_type,
    payload_json,
    status,
    created_at
FROM message_log
WHERE tenant_id = $1
  AND conversation_id = $2
  AND message_id = $3
FOR UPDATE
`, command.AuthContext.TenantID, command.ConversationID, command.MessageID)
	message := domain.Message{
		TenantID:       command.AuthContext.TenantID,
		ConversationID: command.ConversationID,
	}
	if err := row.Scan(
		&message.Seq,
		&message.MessageID,
		&message.SenderID,
		&message.DeviceID,
		&message.ClientMsgID,
		&message.CommandHash,
		&message.MessageType,
		&message.PayloadJSON,
		&message.Status,
		&message.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Message{}, types.NewMessageNotFound("message not found")
		}
		return domain.Message{}, types.NewDBWriteFailed(err.Error())
	}
	return message, nil
}

func lockApprovedComplianceDeleteApproval(ctx context.Context, tx pgx.Tx, command types.DeleteMessageCommand) error {
	var approvalID string
	err := tx.QueryRow(ctx, `
SELECT approval.approval_id
FROM message_compliance_delete_approvals approval
JOIN message_compliance_external_proofs proof
  ON proof.tenant_id = approval.tenant_id
 AND proof.external_proof_ref = approval.external_proof_ref
 AND proof.status = 'VERIFIED'
WHERE approval.tenant_id = $1
  AND approval.conversation_id = $2
  AND approval.message_id = $3
  AND approval.approval_id = $4
  AND approval.external_proof_ref = $5
  AND approval.status = 'APPROVED'
FOR UPDATE OF approval, proof
`, command.AuthContext.TenantID, command.ConversationID, command.MessageID, command.ComplianceApprovalID, command.ExternalProofRef).Scan(&approvalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return types.NewPermissionDenied("compliance delete approval is required")
	}
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func consumeComplianceDeleteApproval(ctx context.Context, tx pgx.Tx, command types.DeleteMessageCommand, record domain.MessageChangeRecord) error {
	commandTag, err := tx.Exec(ctx, `
UPDATE message_compliance_delete_approvals
SET status = 'CONSUMED',
    consumed_by = $6,
    consumed_event_id = $7,
    consumed_at = $8,
    updated_at = $8
WHERE tenant_id = $1
  AND conversation_id = $2
  AND message_id = $3
  AND approval_id = $4
  AND external_proof_ref = $5
  AND status = 'APPROVED'
`, command.AuthContext.TenantID, command.ConversationID, command.MessageID, command.ComplianceApprovalID, command.ExternalProofRef, command.AuthContext.UserID, record.Timeline.EventID, record.ChangedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if commandTag.RowsAffected() != 1 {
		return types.NewPermissionDenied("compliance delete approval is required")
	}
	return nil
}

func ensureMessageNotUnderLegalHold(ctx context.Context, tx pgx.Tx, command types.DeleteMessageCommand) error {
	var activeHoldCount int64
	if err := tx.QueryRow(ctx, `
SELECT count(*)
FROM message_legal_holds
WHERE tenant_id = $1
  AND conversation_id = $2
  AND message_id = $3
  AND status = 'ACTIVE'
`, command.AuthContext.TenantID, command.ConversationID, command.MessageID).Scan(&activeHoldCount); err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if activeHoldCount > 0 {
		return types.NewInvalidMessageState("message is under legal hold")
	}
	return nil
}

func nextDeleteMessageChangeVersion(
	ctx context.Context,
	tx pgx.Tx,
	command types.DeleteMessageCommand,
) (int32, error) {
	return nextMessageChangeVersionFor(ctx, tx, command.AuthContext.TenantID, command.ConversationID, command.MessageID)
}

func updateMessageDeleted(ctx context.Context, tx pgx.Tx, record domain.MessageChangeRecord) error {
	_, err := tx.Exec(ctx, `
UPDATE message_log
SET status = 'DELETED',
    deleted_at = $4,
    payload_json = CASE WHEN $5::jsonb IS NULL THEN payload_json ELSE $5::jsonb END
WHERE tenant_id = $1
  AND conversation_id = $2
  AND message_id = $3
`, record.Timeline.TenantID, record.ConversationID, record.MessageID, record.ChangedAt, nullableJSON(record.AfterPayload))
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertDeleteMessageChangeHistory(
	ctx context.Context,
	tx pgx.Tx,
	input domain.DeleteMessageInput,
	record domain.MessageChangeRecord,
) error {
	_, err := tx.Exec(ctx, `
INSERT INTO message_change_history (
    tenant_id,
    conversation_id,
    message_id,
    change_version,
    change_type,
    before_payload_json,
    after_payload_json,
    before_status,
    after_status,
    changed_by,
    reason,
    trace_id,
    changed_at
) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8, $9, $10, $11, $12, $13)
`, record.Timeline.TenantID, record.ConversationID, record.MessageID, record.ChangeVersion, record.ChangeType, nullableJSON(record.BeforePayload), nullableJSON(record.AfterPayload), record.BeforeStatus, record.AfterStatus, input.Command.AuthContext.UserID, input.Command.Reason, record.Timeline.TraceID, record.ChangedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func nullableJSON(payload []byte) any {
	if len(payload) == 0 {
		return nil
	}
	return payload
}

func insertDeleteCommandResult(
	ctx context.Context,
	tx pgx.Tx,
	command types.DeleteMessageCommand,
	record domain.MessageChangeRecord,
) error {
	result := deleteReplayResult{
		MessageID:       record.MessageID,
		ConversationID:  record.ConversationID,
		ConversationSeq: record.ConversationSeq,
		ChangeVersion:   record.ChangeVersion,
		AcceptedAt:      record.ChangedAt.UTC().Format(time.RFC3339Nano),
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	_, err = tx.Exec(ctx, `
INSERT INTO message_command_idempotency (
    tenant_id,
    conversation_id,
    command_type,
    idempotency_key,
    message_id,
    command_hash,
    result_json,
    created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8)
`, command.AuthContext.TenantID, command.ConversationID, commandTypeDelete, command.IdempotencyKey, command.MessageID, record.CommandHash, resultJSON, record.ChangedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertDeleteMessageTimelineEvent(
	ctx context.Context,
	tx pgx.Tx,
	input domain.DeleteMessageInput,
	record domain.MessageChangeRecord,
) error {
	_, err := tx.Exec(ctx, `
INSERT INTO conversation_timeline_events (
    tenant_id,
    conversation_id,
    seq,
    event_id,
    event_type,
    event_version,
    message_id,
    actor_id,
    fanout_mode,
    fanout_policy_version,
    permission_version,
    classification,
    mapping_version,
    trace_id,
    payload_json,
    created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15::jsonb, $16)
`,
		record.Timeline.TenantID,
		record.Timeline.ConversationID,
		record.Timeline.ConversationSeq,
		record.Timeline.EventID,
		record.Timeline.EventType,
		record.Timeline.EventVersion,
		record.Timeline.MessageID,
		input.Command.AuthContext.UserID,
		record.Timeline.FanoutMode,
		record.Timeline.FanoutPolicyVersion,
		record.Timeline.PermissionVersion,
		record.Timeline.Classification,
		record.Timeline.MappingVersion,
		record.Timeline.TraceID,
		record.Timeline.PayloadJSON,
		record.Timeline.CreatedAt,
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}
