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

const commandTypeEdit = "EDIT"

type editReplayResult struct {
	MessageID       types.MessageID      `json:"message_id"`
	ConversationID  types.ConversationID `json:"conversation_id"`
	ConversationSeq int64                `json:"conversation_seq"`
	ChangeVersion   int32                `json:"change_version"`
	AcceptedAt      string               `json:"accepted_at"`
}

func (r *MessageRepository) EditMessage(
	ctx context.Context,
	input domain.EditMessageInput,
) (domain.MessageChangeResult, error) {
	if r.pool == nil {
		return domain.MessageChangeResult{}, ErrRepositoryNotConfigured
	}
	commandHash, err := domain.ComputeEditMessageCommandHash(input.Command)
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

	if err := lockEditIdempotency(ctx, tx, input.Command); err != nil {
		return domain.MessageChangeResult{}, err
	}
	if existing, ok, err := findExistingEditCommand(ctx, tx, input.Command); err != nil {
		return domain.MessageChangeResult{}, err
	} else if ok {
		return replayEditResult(ctx, tx, existing, commandHash)
	}

	message, err := lockMessageForEdit(ctx, tx, input.Command)
	if err != nil {
		return domain.MessageChangeResult{}, err
	}
	if !canMutateMessageOwnership(message, input.Command.AuthContext.UserID, input.Permission) {
		return domain.MessageChangeResult{}, types.NewPermissionDenied("only the original sender can edit this message in phase 1")
	}
	if !message.CanEdit() {
		return domain.MessageChangeResult{}, types.NewInvalidMessageState("message cannot be edited")
	}

	if err := ensureConversationSeqFor(ctx, tx, input.Command.AuthContext.TenantID, input.Command.ConversationID); err != nil {
		return domain.MessageChangeResult{}, err
	}
	seq, err := allocateConversationSeqFor(ctx, tx, input.Command.AuthContext.TenantID, input.Command.ConversationID)
	if err != nil {
		return domain.MessageChangeResult{}, err
	}
	changeVersion, err := nextMessageChangeVersionFor(ctx, tx, input.Command.AuthContext.TenantID, input.Command.ConversationID, input.Command.MessageID)
	if err != nil {
		return domain.MessageChangeResult{}, err
	}
	eventID, err := r.eventID()
	if err != nil {
		return domain.MessageChangeResult{}, err
	}
	acceptedAt := r.now()
	record, err := domain.NewEditMessageRecord(input, message, eventID, seq, changeVersion, acceptedAt)
	if err != nil {
		return domain.MessageChangeResult{}, err
	}

	if err := updateMessageEdited(ctx, tx, record); err != nil {
		return domain.MessageChangeResult{}, err
	}
	if err := insertEditMessageChangeHistory(ctx, tx, input, record); err != nil {
		return domain.MessageChangeResult{}, err
	}
	if err := insertEditCommandResult(ctx, tx, input.Command, record); err != nil {
		return domain.MessageChangeResult{}, err
	}
	if err := insertEditMessageTimelineEvent(ctx, tx, input, record); err != nil {
		return domain.MessageChangeResult{}, err
	}
	if r.shouldWriteOutbox() {
		if err := insertMessageChangeOutboxEvent(ctx, tx, record); err != nil {
			return domain.MessageChangeResult{}, err
		}
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

func lockEditIdempotency(ctx context.Context, tx pgx.Tx, command types.EditMessageCommand) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, editIdempotencyLockKey(command))
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func editIdempotencyLockKey(command types.EditMessageCommand) string {
	return fmt.Sprintf(
		"%s\x1f%s\x1f%s\x1f%s\x1f%s",
		command.AuthContext.TenantID,
		command.ConversationID,
		command.MessageID,
		commandTypeEdit,
		command.IdempotencyKey,
	)
}

type existingEditCommand struct {
	CommandHash string
	ResultJSON  []byte
}

func findExistingEditCommand(
	ctx context.Context,
	tx pgx.Tx,
	command types.EditMessageCommand,
) (existingEditCommand, bool, error) {
	row := tx.QueryRow(ctx, `
SELECT command_hash, result_json
FROM message_command_idempotency
WHERE tenant_id = $1
  AND conversation_id = $2
  AND command_type = $3
  AND message_id = $4
  AND idempotency_key = $5
FOR UPDATE
`, command.AuthContext.TenantID, command.ConversationID, commandTypeEdit, command.MessageID, command.IdempotencyKey)
	var existing existingEditCommand
	if err := row.Scan(&existing.CommandHash, &existing.ResultJSON); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return existingEditCommand{}, false, nil
		}
		return existingEditCommand{}, false, types.NewDBWriteFailed(err.Error())
	}
	return existing, true, nil
}

func replayEditResult(
	ctx context.Context,
	tx pgx.Tx,
	existing existingEditCommand,
	commandHash string,
) (domain.MessageChangeResult, error) {
	if existing.CommandHash != commandHash {
		return domain.MessageChangeResult{}, types.NewIdempotencyConflict("idempotency_key reused with different edit command")
	}
	var replay editReplayResult
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

func lockMessageForEdit(
	ctx context.Context,
	tx pgx.Tx,
	command types.EditMessageCommand,
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

func updateMessageEdited(ctx context.Context, tx pgx.Tx, record domain.MessageChangeRecord) error {
	_, err := tx.Exec(ctx, `
UPDATE message_log
SET status = 'EDITED',
    payload_json = $4::jsonb,
    edited_at = $5
WHERE tenant_id = $1
  AND conversation_id = $2
  AND message_id = $3
`, record.Timeline.TenantID, record.ConversationID, record.MessageID, record.AfterPayload, record.ChangedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertEditMessageChangeHistory(
	ctx context.Context,
	tx pgx.Tx,
	input domain.EditMessageInput,
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
`, record.Timeline.TenantID, record.ConversationID, record.MessageID, record.ChangeVersion, record.ChangeType, record.BeforePayload, record.AfterPayload, record.BeforeStatus, record.AfterStatus, input.Command.AuthContext.UserID, input.Command.Reason, record.Timeline.TraceID, record.ChangedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertEditCommandResult(
	ctx context.Context,
	tx pgx.Tx,
	command types.EditMessageCommand,
	record domain.MessageChangeRecord,
) error {
	result := editReplayResult{
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
`, command.AuthContext.TenantID, command.ConversationID, commandTypeEdit, command.IdempotencyKey, command.MessageID, record.CommandHash, resultJSON, record.ChangedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertEditMessageTimelineEvent(
	ctx context.Context,
	tx pgx.Tx,
	input domain.EditMessageInput,
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
    partition_key,
    correlation_id,
    causation_id,
    producer,
    payload_json,
    created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19::jsonb, $20)
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
		record.Outbox.PartitionKey,
		record.Outbox.CorrelationID,
		record.Outbox.CausationID,
		record.Outbox.Producer,
		record.Timeline.PayloadJSON,
		record.Timeline.CreatedAt,
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}
