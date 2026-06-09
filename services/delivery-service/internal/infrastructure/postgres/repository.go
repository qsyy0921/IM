package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/delivery-service/internal/domain"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (repository *Repository) PullInbox(
	ctx context.Context,
	command types.PullInboxCommand,
	fetchLimit int,
) ([]types.InboxItem, error) {
	rows, err := repository.pool.Query(ctx, `
SELECT
    conversation_id,
    conversation_seq,
    event_id,
    event_type,
    message_id,
    sender_id,
    payload_json,
    created_at
FROM user_inbox
WHERE tenant_id = $1
  AND user_id = $2
  AND conversation_id = $3
  AND conversation_seq > $4
ORDER BY conversation_seq ASC
LIMIT $5
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID, command.AfterSeq, fetchLimit)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	items := make([]types.InboxItem, 0, fetchLimit)
	for rows.Next() {
		var item types.InboxItem
		if err := rows.Scan(
			&item.ConversationID,
			&item.ConversationSeq,
			&item.EventID,
			&item.EventType,
			&item.MessageID,
			&item.SenderID,
			&item.PayloadJSON,
			&item.CreatedAt,
		); err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return items, nil
}

func (repository *Repository) AckDelivery(
	ctx context.Context,
	command types.AckDeliveryCommand,
) (types.AckDeliveryResult, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.AckDeliveryResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	current, exists, err := lockDeliveryCursor(ctx, tx, command)
	if err != nil {
		return types.AckDeliveryResult{}, err
	}
	maxVisibleSeq, err := maxVisibleInboxSeq(ctx, tx, command)
	if err != nil {
		return types.AckDeliveryResult{}, err
	}
	if command.ReceivedSeq > maxVisibleSeq {
		return types.AckDeliveryResult{}, types.NewAckOutOfVisibleRange("received_seq exceeds visible inbox")
	}
	next, err := domain.MergeDeliveryCursor(current, command.ReceivedSeq)
	if err != nil {
		return types.AckDeliveryResult{}, err
	}
	if !exists {
		if err := insertDeliveryCursor(ctx, tx, command, next); err != nil {
			return types.AckDeliveryResult{}, err
		}
		if err := insertAckOutbox(ctx, tx, command, next); err != nil {
			return types.AckDeliveryResult{}, err
		}
	} else if next > current {
		if err := updateDeliveryCursor(ctx, tx, command, next); err != nil {
			return types.AckDeliveryResult{}, err
		}
		if err := insertAckOutbox(ctx, tx, command, next); err != nil {
			return types.AckDeliveryResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return types.AckDeliveryResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.AckDeliveryResult{
		TenantID:        command.AuthContext.TenantID,
		UserID:          command.AuthContext.UserID,
		DeviceID:        command.AuthContext.DeviceID,
		ConversationID:  command.ConversationID,
		LastReceivedSeq: next,
	}, nil
}

func maxVisibleInboxSeq(
	ctx context.Context,
	tx pgx.Tx,
	command types.AckDeliveryCommand,
) (int64, error) {
	var maxSeq int64
	err := tx.QueryRow(ctx, `
SELECT COALESCE(MAX(conversation_seq), 0)
FROM user_inbox
WHERE tenant_id = $1
  AND user_id = $2
  AND conversation_id = $3
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID).Scan(&maxSeq)
	if err != nil {
		return 0, types.NewDBReadFailed(err.Error())
	}
	return maxSeq, nil
}

func lockDeliveryCursor(
	ctx context.Context,
	tx pgx.Tx,
	command types.AckDeliveryCommand,
) (int64, bool, error) {
	var current int64
	err := tx.QueryRow(ctx, `
SELECT last_received_seq
FROM device_delivery_cursors
WHERE tenant_id = $1
  AND user_id = $2
  AND device_id = $3
  AND conversation_id = $4
FOR UPDATE
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.AuthContext.DeviceID, command.ConversationID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, types.NewDBReadFailed(err.Error())
	}
	return current, true, nil
}

func insertDeliveryCursor(
	ctx context.Context,
	tx pgx.Tx,
	command types.AckDeliveryCommand,
	receivedSeq int64,
) error {
	_, err := tx.Exec(ctx, `
INSERT INTO device_delivery_cursors (
    tenant_id,
    user_id,
    device_id,
    conversation_id,
    last_received_seq,
    updated_at
) VALUES ($1, $2, $3, $4, $5, now())
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.AuthContext.DeviceID, command.ConversationID, receivedSeq)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func updateDeliveryCursor(
	ctx context.Context,
	tx pgx.Tx,
	command types.AckDeliveryCommand,
	receivedSeq int64,
) error {
	_, err := tx.Exec(ctx, `
UPDATE device_delivery_cursors
SET last_received_seq = $5,
    updated_at = now()
WHERE tenant_id = $1
  AND user_id = $2
  AND device_id = $3
  AND conversation_id = $4
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.AuthContext.DeviceID, command.ConversationID, receivedSeq)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertAckOutbox(
	ctx context.Context,
	tx pgx.Tx,
	command types.AckDeliveryCommand,
	receivedSeq int64,
) error {
	payload := map[string]any{
		"tenant_id":         command.AuthContext.TenantID,
		"user_id":           command.AuthContext.UserID,
		"device_id":         command.AuthContext.DeviceID,
		"conversation_id":   command.ConversationID,
		"last_received_seq": receivedSeq,
		"trace_id":          command.AuthContext.TraceID,
		"correlation_id":    command.AuthContext.RequestID,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	eventID := ackEventID(command, receivedSeq)
	_, err = tx.Exec(ctx, `
INSERT INTO delivery_outbox (
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
) VALUES ($1, $2, $3, $4, 'delivery.ack.recorded.v1', '1.0.0', $5, 1, $6, $7, 'delivery-service', $8, $9, 'PENDING', now(), now(), now())
ON CONFLICT (event_id) DO NOTHING
`,
		eventID,
		command.AuthContext.TenantID,
		command.ConversationID,
		receivedSeq,
		partitionKey(command),
		command.AuthContext.RequestID,
		command.AuthContext.RequestID,
		command.AuthContext.TraceID,
		payloadBytes,
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func partitionKey(command types.AckDeliveryCommand) string {
	return fmt.Sprintf("%s:%s", command.AuthContext.TenantID, command.ConversationID)
}

func ackEventID(command types.AckDeliveryCommand, receivedSeq int64) string {
	raw := fmt.Sprintf(
		"%s\x1f%s\x1f%s\x1f%s\x1f%d",
		command.AuthContext.TenantID,
		command.AuthContext.UserID,
		command.AuthContext.DeviceID,
		command.ConversationID,
		receivedSeq,
	)
	sum := sha256.Sum256([]byte(raw))
	return "evt_delivery_ack_" + hex.EncodeToString(sum[:16])
}
