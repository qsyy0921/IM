package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/receipt-service/internal/domain"
	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (repository *Repository) ProjectDeliveryEvent(
	ctx context.Context,
	command types.ProjectDeliveryEventCommand,
) (types.ProjectDeliveryEventResult, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.ProjectDeliveryEventResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	result := types.ProjectDeliveryEventResult{}
	switch command.EventType {
	case types.DeliveryEventInboxItemCreated:
		if err := insertInboxProjection(ctx, tx, command); err != nil {
			return types.ProjectDeliveryEventResult{}, err
		}
		if err := upsertInitialReceiptState(ctx, tx, command); err != nil {
			return types.ProjectDeliveryEventResult{}, err
		}
		result.ProjectedInboxItem = true
	case types.DeliveryEventAckRecorded:
		if err := lockReceivedKey(ctx, tx, command); err != nil {
			return types.ProjectDeliveryEventResult{}, err
		}
		if err := upsertReceivedCursors(ctx, tx, command); err != nil {
			return types.ProjectDeliveryEventResult{}, err
		}
		receivedRows, err := markReceivedStates(ctx, tx, command)
		if err != nil {
			return types.ProjectDeliveryEventResult{}, err
		}
		if receivedRows > 0 {
			if err := insertReceivedOutbox(ctx, tx, command); err != nil {
				return types.ProjectDeliveryEventResult{}, err
			}
		}
		result.AdvancedReceived = true
	default:
		return types.ProjectDeliveryEventResult{}, types.NewInvalidArgument("unsupported delivery event type")
	}
	if command.ShouldCheckpoint() {
		if err := upsertKafkaCheckpoint(ctx, tx, command); err != nil {
			return types.ProjectDeliveryEventResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return types.ProjectDeliveryEventResult{}, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func (repository *Repository) MarkRead(
	ctx context.Context,
	command types.MarkReadCommand,
) (types.MarkReadResult, error) {
	if err := validateAccessContext(command.AuthContext.TenantID, command.ConversationID, command.AccessContext); err != nil {
		return types.MarkReadResult{}, err
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.MarkReadResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := lockReadKey(ctx, tx, command); err != nil {
		return types.MarkReadResult{}, err
	}
	current, err := lockReadCursor(ctx, tx, command)
	if err != nil {
		return types.MarkReadResult{}, err
	}
	maxVisible, err := maxVisibleSeq(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID)
	if err != nil {
		return types.MarkReadResult{}, err
	}
	maxReceived, err := maxReceivedSeq(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID)
	if err != nil {
		return types.MarkReadResult{}, err
	}
	next, err := domain.MergeReadCursor(current, command.ReadSeq, maxVisible, maxReceived)
	if err != nil {
		return types.MarkReadResult{}, err
	}
	if next > current {
		if err := upsertReadCursor(ctx, tx, command, next); err != nil {
			return types.MarkReadResult{}, err
		}
		if err := markReadStates(ctx, tx, command, next); err != nil {
			return types.MarkReadResult{}, err
		}
		if err := insertReadOutbox(ctx, tx, command, next); err != nil {
			return types.MarkReadResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return types.MarkReadResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.MarkReadResult{
		TenantID:       command.AuthContext.TenantID,
		UserID:         command.AuthContext.UserID,
		ConversationID: command.ConversationID,
		LastReadSeq:    next,
	}, nil
}

func (repository *Repository) GetReceiptState(
	ctx context.Context,
	command types.GetReceiptStateCommand,
) (types.GetReceiptStateResult, error) {
	if err := validateAccessContext(command.AuthContext.TenantID, command.ConversationID, command.AccessContext); err != nil {
		return types.GetReceiptStateResult{}, err
	}
	conversationSeq := command.ConversationSeq
	messageID := command.MessageID
	if conversationSeq == 0 {
		err := repository.pool.QueryRow(ctx, `
SELECT conversation_seq
FROM receipt_inbox_projection
WHERE tenant_id = $1
  AND conversation_id = $2
  AND message_id = $3
ORDER BY conversation_seq ASC
LIMIT 1
`, command.AuthContext.TenantID, command.ConversationID, command.MessageID).Scan(&conversationSeq)
		if err == pgx.ErrNoRows {
			return types.GetReceiptStateResult{}, types.NewReceiptNotFound("receipt state not found")
		}
		if err != nil {
			return types.GetReceiptStateResult{}, types.NewDBReadFailed(err.Error())
		}
	}
	if messageID == "" {
		err := repository.pool.QueryRow(ctx, `
SELECT message_id
FROM receipt_inbox_projection
WHERE tenant_id = $1
  AND conversation_id = $2
  AND conversation_seq = $3
ORDER BY user_id ASC
LIMIT 1
`, command.AuthContext.TenantID, command.ConversationID, conversationSeq).Scan(&messageID)
		if err == pgx.ErrNoRows {
			return types.GetReceiptStateResult{}, types.NewReceiptNotFound("receipt state not found")
		}
		if err != nil {
			return types.GetReceiptStateResult{}, types.NewDBReadFailed(err.Error())
		}
	}

	rows, err := repository.pool.Query(ctx, `
SELECT
    user_id,
    CASE WHEN received_at IS NULL THEN 0 ELSE conversation_seq END AS received_seq,
    received_at,
    CASE WHEN read_at IS NULL THEN 0 ELSE conversation_seq END AS read_seq,
    read_at
FROM message_receipt_states
WHERE tenant_id = $1
  AND conversation_id = $2
  AND conversation_seq = $3
ORDER BY user_id ASC
`, command.AuthContext.TenantID, command.ConversationID, conversationSeq)
	if err != nil {
		return types.GetReceiptStateResult{}, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	receivers := make([]types.ReceiptUserState, 0)
	receivedCount := 0
	readCount := 0
	for rows.Next() {
		var receiver types.ReceiptUserState
		var receivedAt sql.NullTime
		var readAt sql.NullTime
		if err := rows.Scan(
			&receiver.UserID,
			&receiver.ReceivedSeq,
			&receivedAt,
			&receiver.ReadSeq,
			&readAt,
		); err != nil {
			return types.GetReceiptStateResult{}, types.NewDBReadFailed(err.Error())
		}
		if receivedAt.Valid {
			receiver.ReceivedAt = receivedAt.Time
		}
		if readAt.Valid {
			receiver.ReadAt = readAt.Time
		}
		if receiver.ReceivedSeq > 0 {
			receivedCount++
		}
		if receiver.ReadSeq > 0 {
			readCount++
		}
		receivers = append(receivers, receiver)
	}
	if err := rows.Err(); err != nil {
		return types.GetReceiptStateResult{}, types.NewDBReadFailed(err.Error())
	}
	if len(receivers) == 0 {
		return types.GetReceiptStateResult{}, types.NewReceiptNotFound("receipt state not found")
	}
	return types.GetReceiptStateResult{
		ConversationID:    command.ConversationID,
		ConversationSeq:   conversationSeq,
		MessageID:         messageID,
		ReceivedUserCount: receivedCount,
		ReadUserCount:     readCount,
		VisibilityMode:    types.ReceiptVisibilityDetailed,
		Receivers:         receivers,
	}, nil
}

func validateAccessContext(tenantID types.TenantID, conversationID types.ConversationID, access types.ReceiptAccessContext) error {
	if access.TenantID == "" && access.ConversationID == "" {
		return nil
	}
	if access.TenantID != tenantID || access.ConversationID != conversationID {
		return types.NewPermissionDenied("receipt access context mismatch")
	}
	return nil
}

func insertInboxProjection(ctx context.Context, tx pgx.Tx, command types.ProjectDeliveryEventCommand) error {
	_, err := tx.Exec(ctx, `
INSERT INTO receipt_inbox_projection (
    tenant_id,
    user_id,
    conversation_id,
    conversation_seq,
    source_event_id,
    delivery_event_id,
    message_id,
    sender_id,
    created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
ON CONFLICT (tenant_id, user_id, delivery_event_id) DO NOTHING
`, command.TenantID, command.UserID, command.ConversationID, command.ConversationSeq, command.SourceEventID, command.EventID, command.MessageID, command.SenderID)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func upsertInitialReceiptState(ctx context.Context, tx pgx.Tx, command types.ProjectDeliveryEventCommand) error {
	deviceID, alreadyReceived, err := receivedDeviceForSeq(ctx, tx, command.TenantID, command.UserID, command.ConversationID, command.ConversationSeq)
	if err != nil {
		return err
	}
	var receivedAtExpression string
	if alreadyReceived {
		receivedAtExpression = "now()"
	} else {
		receivedAtExpression = "NULL"
	}
	_, err = tx.Exec(ctx, fmt.Sprintf(`
INSERT INTO message_receipt_states (
    tenant_id,
    conversation_id,
    conversation_seq,
    message_id,
    user_id,
    received_at,
    updated_at
) VALUES ($1, $2, $3, $4, $5, %s, now())
ON CONFLICT (tenant_id, conversation_id, conversation_seq, user_id) DO UPDATE
SET message_id = EXCLUDED.message_id,
    received_at = COALESCE(message_receipt_states.received_at, EXCLUDED.received_at),
    updated_at = now()
`, receivedAtExpression), command.TenantID, command.ConversationID, command.ConversationSeq, command.MessageID, command.UserID)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if alreadyReceived {
		receivedCommand := command
		receivedCommand.DeviceID = deviceID
		receivedCommand.LastReceivedSeq = command.ConversationSeq
		if err := insertReceivedOutbox(ctx, tx, receivedCommand); err != nil {
			return err
		}
	}
	return nil
}

func receivedDeviceForSeq(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	userID types.UserID,
	conversationID types.ConversationID,
	seq int64,
) (string, bool, error) {
	var deviceID string
	err := tx.QueryRow(ctx, `
SELECT device_id
FROM device_received_cursors
WHERE tenant_id = $1
  AND user_id = $2
  AND conversation_id = $3
  AND last_received_seq >= $4
ORDER BY last_received_seq DESC, updated_at ASC
LIMIT 1
`, tenantID, userID, conversationID, seq).Scan(&deviceID)
	if err == pgx.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, types.NewDBReadFailed(err.Error())
	}
	return deviceID, true, nil
}

func lockReceivedKey(ctx context.Context, tx pgx.Tx, command types.ProjectDeliveryEventCommand) error {
	key := fmt.Sprintf("%s\x1f%s\x1f%s\x1freceived", command.TenantID, command.UserID, command.ConversationID)
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func upsertReceivedCursors(ctx context.Context, tx pgx.Tx, command types.ProjectDeliveryEventCommand) error {
	_, err := tx.Exec(ctx, `
INSERT INTO device_received_cursors (
    tenant_id,
    user_id,
    device_id,
    conversation_id,
    last_received_seq,
    updated_at
) VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (tenant_id, user_id, device_id, conversation_id) DO UPDATE
SET last_received_seq = GREATEST(device_received_cursors.last_received_seq, EXCLUDED.last_received_seq),
    updated_at = now()
`, command.TenantID, command.UserID, command.DeviceID, command.ConversationID, command.LastReceivedSeq)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	_, err = tx.Exec(ctx, `
INSERT INTO user_received_cursors (
    tenant_id,
    user_id,
    conversation_id,
    last_received_seq,
    updated_at
) VALUES ($1, $2, $3, $4, now())
ON CONFLICT (tenant_id, user_id, conversation_id) DO UPDATE
SET last_received_seq = GREATEST(user_received_cursors.last_received_seq, EXCLUDED.last_received_seq),
    updated_at = now()
`, command.TenantID, command.UserID, command.ConversationID, command.LastReceivedSeq)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func markReceivedStates(ctx context.Context, tx pgx.Tx, command types.ProjectDeliveryEventCommand) (int64, error) {
	tag, err := tx.Exec(ctx, `
UPDATE message_receipt_states
SET received_at = COALESCE(received_at, now()),
    updated_at = now()
WHERE tenant_id = $1
  AND user_id = $2
  AND conversation_id = $3
  AND conversation_seq <= $4
`, command.TenantID, command.UserID, command.ConversationID, command.LastReceivedSeq)
	if err != nil {
		return 0, types.NewDBWriteFailed(err.Error())
	}
	return tag.RowsAffected(), nil
}

func lockReadKey(ctx context.Context, tx pgx.Tx, command types.MarkReadCommand) error {
	key := fmt.Sprintf("%s\x1f%s\x1f%s\x1fread", command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID)
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func lockReadCursor(ctx context.Context, tx pgx.Tx, command types.MarkReadCommand) (int64, error) {
	var current int64
	err := tx.QueryRow(ctx, `
SELECT COALESCE(last_read_seq, 0)
FROM user_read_cursors
WHERE tenant_id = $1
  AND user_id = $2
  AND conversation_id = $3
FOR UPDATE
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID).Scan(&current)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, types.NewDBReadFailed(err.Error())
	}
	return current, nil
}

func maxVisibleSeq(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, conversationID types.ConversationID) (int64, error) {
	var maxSeq int64
	err := tx.QueryRow(ctx, `
SELECT COALESCE(MAX(conversation_seq), 0)
FROM receipt_inbox_projection
WHERE tenant_id = $1
  AND user_id = $2
  AND conversation_id = $3
`, tenantID, userID, conversationID).Scan(&maxSeq)
	if err != nil {
		return 0, types.NewDBReadFailed(err.Error())
	}
	return maxSeq, nil
}

func maxReceivedSeq(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, conversationID types.ConversationID) (int64, error) {
	var maxSeq int64
	err := tx.QueryRow(ctx, `
SELECT COALESCE(last_received_seq, 0)
FROM user_received_cursors
WHERE tenant_id = $1
  AND user_id = $2
  AND conversation_id = $3
`, tenantID, userID, conversationID).Scan(&maxSeq)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, types.NewDBReadFailed(err.Error())
	}
	return maxSeq, nil
}

func upsertReadCursor(ctx context.Context, tx pgx.Tx, command types.MarkReadCommand, readSeq int64) error {
	_, err := tx.Exec(ctx, `
INSERT INTO user_read_cursors (
    tenant_id,
    user_id,
    conversation_id,
    last_read_seq,
    updated_at
) VALUES ($1, $2, $3, $4, now())
ON CONFLICT (tenant_id, user_id, conversation_id) DO UPDATE
SET last_read_seq = GREATEST(user_read_cursors.last_read_seq, EXCLUDED.last_read_seq),
    updated_at = now()
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID, readSeq)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func markReadStates(ctx context.Context, tx pgx.Tx, command types.MarkReadCommand, readSeq int64) error {
	_, err := tx.Exec(ctx, `
UPDATE message_receipt_states
SET read_at = COALESCE(read_at, now()),
    updated_at = now()
WHERE tenant_id = $1
  AND user_id = $2
  AND conversation_id = $3
  AND conversation_seq <= $4
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID, readSeq)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func upsertKafkaCheckpoint(ctx context.Context, tx pgx.Tx, command types.ProjectDeliveryEventCommand) error {
	_, err := tx.Exec(ctx, `
INSERT INTO receipt_kafka_checkpoints (
    consumer_group,
    topic,
    partition_id,
    offset_value,
    updated_at
) VALUES ($1, $2, $3, $4, now())
ON CONFLICT (consumer_group, topic, partition_id) DO UPDATE
SET offset_value = GREATEST(receipt_kafka_checkpoints.offset_value, EXCLUDED.offset_value),
    updated_at = now()
`, command.ConsumerGroup, command.Topic, command.PartitionID, command.OffsetValue)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertReceivedOutbox(ctx context.Context, tx pgx.Tx, command types.ProjectDeliveryEventCommand) error {
	return insertReceiptOutbox(ctx, tx, receiptOutboxInput{
		EventID:          receivedEventID(command),
		TenantID:         command.TenantID,
		ConversationID:   command.ConversationID,
		AggregateVersion: command.LastReceivedSeq,
		EventType:        "receipt.message.received.v1",
		CorrelationID:    command.CorrelationID,
		CausationID:      command.EventID,
		TraceID:          command.TraceID,
		Payload: map[string]any{
			"tenant_id":        command.TenantID,
			"conversation_id":  command.ConversationID,
			"conversation_seq": command.LastReceivedSeq,
			"message_id":       "",
			"user_id":          command.UserID,
			"device_id":        command.DeviceID,
			"cursor_seq":       command.LastReceivedSeq,
			"source_event_id":  command.EventID,
		},
	})
}

func insertReadOutbox(ctx context.Context, tx pgx.Tx, command types.MarkReadCommand, readSeq int64) error {
	return insertReceiptOutbox(ctx, tx, receiptOutboxInput{
		EventID:          readEventID(command, readSeq),
		TenantID:         command.AuthContext.TenantID,
		ConversationID:   command.ConversationID,
		AggregateVersion: readSeq,
		EventType:        "receipt.message.read.v1",
		CorrelationID:    command.AuthContext.RequestID,
		CausationID:      command.AuthContext.RequestID,
		TraceID:          command.AuthContext.TraceID,
		Payload: map[string]any{
			"tenant_id":        command.AuthContext.TenantID,
			"conversation_id":  command.ConversationID,
			"conversation_seq": readSeq,
			"message_id":       "",
			"user_id":          command.AuthContext.UserID,
			"device_id":        command.AuthContext.DeviceID,
			"cursor_seq":       readSeq,
			"source_event_id":  command.AuthContext.RequestID,
		},
	})
}

type receiptOutboxInput struct {
	EventID          string
	TenantID         types.TenantID
	ConversationID   types.ConversationID
	AggregateVersion int64
	EventType        string
	CorrelationID    string
	CausationID      string
	TraceID          string
	Payload          map[string]any
}

func insertReceiptOutbox(ctx context.Context, tx pgx.Tx, input receiptOutboxInput) error {
	payloadBytes, err := json.Marshal(input.Payload)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	_, err = tx.Exec(ctx, `
INSERT INTO receipt_outbox (
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
    trace_id,
    payload_json,
    status,
    available_at,
    created_at,
    updated_at
) VALUES ($1, $2, $3, $4, $5, '1.0.0', $6, 1, $7, $8, 'receipt-service', $9, $10, 'PENDING', now(), now(), now())
ON CONFLICT (event_id) DO NOTHING
`, input.EventID, input.TenantID, input.ConversationID, input.AggregateVersion, input.EventType, partitionKeyFor(input.TenantID, input.ConversationID), input.CorrelationID, input.CausationID, input.TraceID, payloadBytes)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func partitionKeyFor(tenantID types.TenantID, conversationID types.ConversationID) string {
	return fmt.Sprintf("%s:%s", tenantID, conversationID)
}

func receivedEventID(command types.ProjectDeliveryEventCommand) string {
	raw := fmt.Sprintf("%s\x1f%s\x1f%s\x1f%s\x1f%d", command.TenantID, command.UserID, command.DeviceID, command.ConversationID, command.LastReceivedSeq)
	sum := sha256.Sum256([]byte(raw))
	return "evt_receipt_received_" + hex.EncodeToString(sum[:16])
}

func readEventID(command types.MarkReadCommand, readSeq int64) string {
	raw := fmt.Sprintf("%s\x1f%s\x1f%s\x1f%d", command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID, readSeq)
	sum := sha256.Sum256([]byte(raw))
	return "evt_receipt_read_" + hex.EncodeToString(sum[:16])
}
