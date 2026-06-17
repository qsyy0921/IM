package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

func insertReceivedOutbox(ctx context.Context, tx pgx.Tx, command types.ProjectDeliveryEventCommand) error {
	messageID, sourceEventID, err := receiptEventMessageRef(
		ctx,
		tx,
		command.TenantID,
		command.UserID,
		command.ConversationID,
		command.LastReceivedSeq,
	)
	if err != nil {
		return err
	}
	return insertReceiptOutbox(ctx, tx, receiptOutboxInput{
		EventID:          receivedEventID(command),
		TenantID:         command.TenantID,
		ConversationID:   command.ConversationID,
		AggregateVersion: command.LastReceivedSeq,
		EventType:        types.ReceiptEventMessageReceived,
		CorrelationID:    command.CorrelationID,
		CausationID:      command.EventID,
		TraceID:          command.TraceID,
		Payload: map[string]any{
			"tenant_id":        command.TenantID,
			"conversation_id":  command.ConversationID,
			"conversation_seq": command.LastReceivedSeq,
			"message_id":       messageID,
			"user_id":          command.UserID,
			"device_id":        command.DeviceID,
			"cursor_seq":       command.LastReceivedSeq,
			"source_event_id":  sourceEventID,
		},
	})
}

func insertReadOutbox(ctx context.Context, tx pgx.Tx, command types.MarkReadCommand, readSeq int64) error {
	messageID, sourceEventID, err := receiptEventMessageRef(
		ctx,
		tx,
		command.AuthContext.TenantID,
		command.AuthContext.UserID,
		command.ConversationID,
		readSeq,
	)
	if err != nil {
		return err
	}
	return insertReceiptOutbox(ctx, tx, receiptOutboxInput{
		EventID:          readEventID(command, readSeq),
		TenantID:         command.AuthContext.TenantID,
		ConversationID:   command.ConversationID,
		AggregateVersion: readSeq,
		EventType:        types.ReceiptEventMessageRead,
		CorrelationID:    command.AuthContext.RequestID,
		CausationID:      command.AuthContext.RequestID,
		TraceID:          command.AuthContext.TraceID,
		Payload: map[string]any{
			"tenant_id":        command.AuthContext.TenantID,
			"conversation_id":  command.ConversationID,
			"conversation_seq": readSeq,
			"message_id":       messageID,
			"user_id":          command.AuthContext.UserID,
			"device_id":        command.AuthContext.DeviceID,
			"cursor_seq":       readSeq,
			"source_event_id":  sourceEventID,
		},
	})
}

func receiptEventMessageRef(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	userID types.UserID,
	conversationID types.ConversationID,
	conversationSeq int64,
) (string, string, error) {
	var messageID string
	var sourceEventID string
	err := tx.QueryRow(ctx, `
SELECT message_id, source_event_id
FROM receipt_inbox_projection
WHERE tenant_id = $1
  AND user_id = $2
  AND conversation_id = $3
  AND conversation_seq = $4
`, tenantID, userID, conversationID, conversationSeq).Scan(&messageID, &sourceEventID)
	if err == pgx.ErrNoRows {
		return "", "", types.NewProjectionLagging("receipt message reference not projected")
	}
	if err != nil {
		return "", "", types.NewDBReadFailed(err.Error())
	}
	return messageID, sourceEventID, nil
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
