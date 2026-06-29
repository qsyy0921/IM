package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

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
		if err := lockConversationSummaryKey(ctx, tx, command.TenantID, command.UserID, command.ConversationID); err != nil {
			return types.ProjectDeliveryEventResult{}, err
		}
		if err := insertInboxProjection(ctx, tx, command); err != nil {
			return types.ProjectDeliveryEventResult{}, err
		}
		if command.SourceEventType == types.SourceEventMessagePersisted {
			if err := upsertInitialReceiptState(ctx, tx, command); err != nil {
				return types.ProjectDeliveryEventResult{}, err
			}
		}
		if err := upsertConversationSummaryFromInbox(ctx, tx, command); err != nil {
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
	case types.DeliveryEventConversationSignal:
		// Conversation-level delivery signal is a push/PullInbox wakeup event.
		// receipt-service has no per-user receipt projection for it, but must
		// still checkpoint the Kafka offset so the shared topic can progress.
	default:
		return types.ProjectDeliveryEventResult{}, types.NewInvalidArgument("unsupported delivery event type")
	}
	if command.ShouldCheckpoint() {
		if err := upsertKafkaCheckpoint(ctx, tx, command); err != nil {
			return types.ProjectDeliveryEventResult{}, err
		}
		if err := upsertConversationSummaryCheckpoint(ctx, tx, command); err != nil {
			return types.ProjectDeliveryEventResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return types.ProjectDeliveryEventResult{}, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func insertInboxProjection(ctx context.Context, tx pgx.Tx, command types.ProjectDeliveryEventCommand) error {
	_, err := tx.Exec(ctx, `
INSERT INTO receipt_inbox_projection (
    tenant_id,
    user_id,
    conversation_id,
    conversation_seq,
    source_event_id,
    source_event_type,
    delivery_event_id,
    message_id,
    sender_id,
    created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
ON CONFLICT (tenant_id, user_id, delivery_event_id) DO NOTHING
`, command.TenantID, command.UserID, command.ConversationID, command.ConversationSeq, command.SourceEventID, command.SourceEventType, command.EventID, command.MessageID, command.SenderID)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func upsertConversationSummaryFromInbox(ctx context.Context, tx pgx.Tx, command types.ProjectDeliveryEventCommand) error {
	_, err := tx.Exec(ctx, `
WITH read_cursor AS (
    SELECT COALESCE(MAX(last_read_seq), 0) AS last_read_seq
    FROM user_read_cursors
    WHERE tenant_id = $1
      AND user_id = $2
      AND conversation_id = $3
),
unread AS (
    SELECT COUNT(*) AS unread_count
    FROM receipt_inbox_projection
    WHERE tenant_id = $1
      AND user_id = $2
      AND conversation_id = $3
      AND source_event_type = 'message.persisted.v1'
      AND conversation_seq > (SELECT last_read_seq FROM read_cursor)
)
INSERT INTO user_conversation_summaries (
    tenant_id,
    user_id,
    conversation_id,
    last_visible_seq,
    last_message_id,
    last_sender_id,
    last_source_event_type,
    last_read_seq,
    unread_count,
    sort_updated_at,
    updated_at
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    (SELECT last_read_seq FROM read_cursor),
    (SELECT unread_count FROM unread),
    now(),
    now()
)
ON CONFLICT (tenant_id, user_id, conversation_id) DO UPDATE
SET last_visible_seq = GREATEST(user_conversation_summaries.last_visible_seq, EXCLUDED.last_visible_seq),
    last_message_id = CASE
        WHEN EXCLUDED.last_visible_seq >= user_conversation_summaries.last_visible_seq THEN EXCLUDED.last_message_id
        ELSE user_conversation_summaries.last_message_id
    END,
    last_sender_id = CASE
        WHEN EXCLUDED.last_visible_seq >= user_conversation_summaries.last_visible_seq THEN EXCLUDED.last_sender_id
        ELSE user_conversation_summaries.last_sender_id
    END,
    last_source_event_type = CASE
        WHEN EXCLUDED.last_visible_seq >= user_conversation_summaries.last_visible_seq THEN EXCLUDED.last_source_event_type
        ELSE user_conversation_summaries.last_source_event_type
    END,
    last_read_seq = EXCLUDED.last_read_seq,
    unread_count = EXCLUDED.unread_count,
    sort_updated_at = CASE
        WHEN EXCLUDED.last_visible_seq >= user_conversation_summaries.last_visible_seq THEN EXCLUDED.sort_updated_at
        ELSE user_conversation_summaries.sort_updated_at
    END,
    updated_at = now()
`, command.TenantID, command.UserID, command.ConversationID, command.ConversationSeq, command.MessageID, command.SenderID, command.SourceEventType)
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

func upsertConversationSummaryCheckpoint(ctx context.Context, tx pgx.Tx, command types.ProjectDeliveryEventCommand) error {
	_, err := tx.Exec(ctx, `
INSERT INTO conversation_summary_checkpoints (
    consumer_group,
    topic,
    partition_id,
    offset_value,
    updated_at
) VALUES ($1, $2, $3, $4, now())
ON CONFLICT (consumer_group, topic, partition_id) DO UPDATE
SET offset_value = GREATEST(conversation_summary_checkpoints.offset_value, EXCLUDED.offset_value),
    updated_at = now()
`, command.ConsumerGroup, command.Topic, command.PartitionID, command.OffsetValue)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}
