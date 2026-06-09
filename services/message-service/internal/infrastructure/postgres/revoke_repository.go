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

const commandTypeRevoke = "REVOKE"

type revokeReplayResult struct {
	MessageID       types.MessageID      `json:"message_id"`
	ConversationID  types.ConversationID `json:"conversation_id"`
	ConversationSeq int64                `json:"conversation_seq"`
	ChangeVersion   int32                `json:"change_version"`
	AcceptedAt      string               `json:"accepted_at"`
}

func (r *MessageRepository) RevokeMessage(
	ctx context.Context,
	input domain.RevokeMessageInput,
) (domain.MessageChangeResult, error) {
	if r.pool == nil {
		return domain.MessageChangeResult{}, ErrRepositoryNotConfigured
	}
	commandHash, err := domain.ComputeRevokeMessageCommandHash(input.Command)
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

	if err := lockRevokeIdempotency(ctx, tx, input.Command); err != nil {
		return domain.MessageChangeResult{}, err
	}
	if existing, ok, err := findExistingRevokeCommand(ctx, tx, input.Command); err != nil {
		return domain.MessageChangeResult{}, err
	} else if ok {
		return replayRevokeResult(ctx, tx, existing, commandHash)
	}

	message, err := lockMessageForRevoke(ctx, tx, input.Command)
	if err != nil {
		return domain.MessageChangeResult{}, err
	}
	if message.SenderID != input.Command.AuthContext.UserID {
		return domain.MessageChangeResult{}, types.NewPermissionDenied("only the original sender can revoke this message in phase 1")
	}
	if !message.CanRevoke() {
		return domain.MessageChangeResult{}, types.NewInvalidMessageState("message cannot be revoked")
	}

	if err := ensureConversationSeqFor(ctx, tx, input.Command.AuthContext.TenantID, input.Command.ConversationID); err != nil {
		return domain.MessageChangeResult{}, err
	}
	seq, err := allocateConversationSeqFor(ctx, tx, input.Command.AuthContext.TenantID, input.Command.ConversationID)
	if err != nil {
		return domain.MessageChangeResult{}, err
	}
	changeVersion, err := nextMessageChangeVersion(ctx, tx, input.Command)
	if err != nil {
		return domain.MessageChangeResult{}, err
	}
	eventID, err := r.eventID()
	if err != nil {
		return domain.MessageChangeResult{}, err
	}
	acceptedAt := r.now()
	record, err := domain.NewRevokeMessageRecord(input, message, eventID, seq, changeVersion, acceptedAt)
	if err != nil {
		return domain.MessageChangeResult{}, err
	}

	if err := updateMessageRevoked(ctx, tx, record); err != nil {
		return domain.MessageChangeResult{}, err
	}
	if err := insertMessageChangeHistory(ctx, tx, input, record); err != nil {
		return domain.MessageChangeResult{}, err
	}
	if err := insertRevokeCommandResult(ctx, tx, input.Command, record); err != nil {
		return domain.MessageChangeResult{}, err
	}
	if err := insertMessageChangeTimelineEvent(ctx, tx, input, record); err != nil {
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

func lockRevokeIdempotency(ctx context.Context, tx pgx.Tx, command types.RevokeMessageCommand) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, revokeIdempotencyLockKey(command))
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func revokeIdempotencyLockKey(command types.RevokeMessageCommand) string {
	return fmt.Sprintf(
		"%s\x1f%s\x1f%s\x1f%s\x1f%s",
		command.AuthContext.TenantID,
		command.ConversationID,
		command.MessageID,
		commandTypeRevoke,
		command.IdempotencyKey,
	)
}

type existingRevokeCommand struct {
	CommandHash string
	ResultJSON  []byte
}

func findExistingRevokeCommand(
	ctx context.Context,
	tx pgx.Tx,
	command types.RevokeMessageCommand,
) (existingRevokeCommand, bool, error) {
	row := tx.QueryRow(ctx, `
SELECT command_hash, result_json
FROM message_command_idempotency
WHERE tenant_id = $1
  AND conversation_id = $2
  AND command_type = $3
  AND message_id = $4
  AND idempotency_key = $5
FOR UPDATE
`, command.AuthContext.TenantID, command.ConversationID, commandTypeRevoke, command.MessageID, command.IdempotencyKey)
	var existing existingRevokeCommand
	if err := row.Scan(&existing.CommandHash, &existing.ResultJSON); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return existingRevokeCommand{}, false, nil
		}
		return existingRevokeCommand{}, false, types.NewDBWriteFailed(err.Error())
	}
	return existing, true, nil
}

func replayRevokeResult(
	ctx context.Context,
	tx pgx.Tx,
	existing existingRevokeCommand,
	commandHash string,
) (domain.MessageChangeResult, error) {
	if existing.CommandHash != commandHash {
		return domain.MessageChangeResult{}, types.NewIdempotencyConflict("idempotency_key reused with different revoke command")
	}
	var replay revokeReplayResult
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

func lockMessageForRevoke(
	ctx context.Context,
	tx pgx.Tx,
	command types.RevokeMessageCommand,
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

func ensureConversationSeqFor(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, conversationID types.ConversationID) error {
	_, err := tx.Exec(ctx, `
INSERT INTO conversation_seq (tenant_id, conversation_id, current_seq)
VALUES ($1, $2, 0)
ON CONFLICT (tenant_id, conversation_id) DO NOTHING
`, tenantID, conversationID)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func allocateConversationSeqFor(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, conversationID types.ConversationID) (int64, error) {
	row := tx.QueryRow(ctx, `
UPDATE conversation_seq
SET current_seq = current_seq + 1,
    updated_at = now()
WHERE tenant_id = $1
  AND conversation_id = $2
RETURNING current_seq
`, tenantID, conversationID)
	var seq int64
	if err := row.Scan(&seq); err != nil {
		return 0, types.NewDBWriteFailed(err.Error())
	}
	return seq, nil
}

func nextMessageChangeVersion(
	ctx context.Context,
	tx pgx.Tx,
	command types.RevokeMessageCommand,
) (int32, error) {
	var version int32
	err := tx.QueryRow(ctx, `
SELECT COALESCE(MAX(change_version), 0) + 1
FROM message_change_history
WHERE tenant_id = $1
  AND conversation_id = $2
  AND message_id = $3
`, command.AuthContext.TenantID, command.ConversationID, command.MessageID).Scan(&version)
	if err != nil {
		return 0, types.NewDBWriteFailed(err.Error())
	}
	return version, nil
}

func updateMessageRevoked(ctx context.Context, tx pgx.Tx, record domain.MessageChangeRecord) error {
	_, err := tx.Exec(ctx, `
UPDATE message_log
SET status = 'REVOKED',
    revoked_at = $4
WHERE tenant_id = $1
  AND conversation_id = $2
  AND message_id = $3
`, record.Timeline.TenantID, record.ConversationID, record.MessageID, record.ChangedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertMessageChangeHistory(
	ctx context.Context,
	tx pgx.Tx,
	input domain.RevokeMessageInput,
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
) VALUES ($1, $2, $3, $4, $5, $6::jsonb, NULL, $7, $8, $9, $10, $11, $12)
`, record.Timeline.TenantID, record.ConversationID, record.MessageID, record.ChangeVersion, record.ChangeType, record.BeforePayload, record.BeforeStatus, record.AfterStatus, input.Command.AuthContext.UserID, input.Command.Reason, record.Timeline.TraceID, record.ChangedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertRevokeCommandResult(
	ctx context.Context,
	tx pgx.Tx,
	command types.RevokeMessageCommand,
	record domain.MessageChangeRecord,
) error {
	result := revokeReplayResult{
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
`, command.AuthContext.TenantID, command.ConversationID, commandTypeRevoke, command.IdempotencyKey, command.MessageID, record.CommandHash, resultJSON, record.ChangedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertMessageChangeTimelineEvent(
	ctx context.Context,
	tx pgx.Tx,
	input domain.RevokeMessageInput,
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

func insertMessageChangeOutboxEvent(ctx context.Context, tx pgx.Tx, record domain.MessageChangeRecord) error {
	_, err := tx.Exec(ctx, `
INSERT INTO message_outbox (
    event_id,
    tenant_id,
    conversation_id,
    aggregate_version,
    event_type,
    event_version,
    partition_key,
    mapping_version,
    correlation_id,
    causation_id,
    producer,
    payload_json,
    trace_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13)
`,
		record.Outbox.EventID,
		record.Outbox.TenantID,
		record.Outbox.ConversationID,
		record.Outbox.AggregateVersion,
		record.Outbox.EventType,
		record.Outbox.EventVersion,
		record.Outbox.PartitionKey,
		record.Outbox.MappingVersion,
		record.Outbox.CorrelationID,
		record.Outbox.CausationID,
		record.Outbox.Producer,
		record.Outbox.PayloadJSON,
		record.Outbox.TraceID,
	)
	if err != nil {
		return types.NewOutboxWriteFailed(err.Error())
	}
	return nil
}
