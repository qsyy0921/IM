package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
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
    read_at,
    (
        SELECT COUNT(*)
        FROM device_received_cursors
        WHERE device_received_cursors.tenant_id = message_receipt_states.tenant_id
          AND device_received_cursors.user_id = message_receipt_states.user_id
          AND device_received_cursors.conversation_id = message_receipt_states.conversation_id
          AND device_received_cursors.last_received_seq >= message_receipt_states.conversation_seq
    ) AS received_device_count
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
			&receiver.ReceivedDeviceCount,
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
	result := types.GetReceiptStateResult{
		ConversationID:    command.ConversationID,
		ConversationSeq:   conversationSeq,
		MessageID:         messageID,
		ReceivedUserCount: receivedCount,
		ReadUserCount:     readCount,
		VisibilityMode:    types.ReceiptVisibilityDetailed,
		Receivers:         receivers,
	}
	if command.IncludeReceivedDevices {
		if err := repository.attachReceivedDeviceDetails(
			ctx,
			command.AuthContext.TenantID,
			command.ConversationID,
			[]*types.GetReceiptStateResult{&result},
			command.ReceivedDeviceLimit(),
		); err != nil {
			return types.GetReceiptStateResult{}, err
		}
	}
	return result, nil
}

func (repository *Repository) ListReceiptStates(
	ctx context.Context,
	command types.ListReceiptStatesCommand,
) (types.ListReceiptStatesResult, error) {
	if err := validateAccessContext(command.AuthContext.TenantID, command.ConversationID, command.AccessContext); err != nil {
		return types.ListReceiptStatesResult{}, err
	}
	if len(command.Items) == 0 {
		return types.ListReceiptStatesResult{}, types.NewInvalidArgument("items are required")
	}
	if len(command.Items) > 50 {
		return types.ListReceiptStatesResult{}, types.NewInvalidArgument("items exceeds max batch size")
	}

	args := []any{command.AuthContext.TenantID, command.ConversationID}
	valueClauses := make([]string, 0, len(command.Items))
	for index, item := range command.Items {
		if err := item.Validate(); err != nil {
			return types.ListReceiptStatesResult{}, err
		}
		var messageID any
		var conversationSeq any
		if item.MessageID != "" {
			messageID = item.MessageID
		} else {
			conversationSeq = item.ConversationSeq
		}
		args = append(args, messageID, conversationSeq)
		valueClauses = append(valueClauses, fmt.Sprintf("(%d, $%d::text, $%d::bigint)", index, len(args)-1, len(args)))
	}

	query := fmt.Sprintf(`
WITH requested(ord, message_id, conversation_seq) AS (
    VALUES %s
),
resolved AS (
    SELECT
        requested.ord,
        matched.message_id,
        matched.conversation_seq
    FROM requested
    JOIN LATERAL (
        SELECT
            receipt_inbox_projection.message_id,
            receipt_inbox_projection.conversation_seq
        FROM receipt_inbox_projection
        WHERE receipt_inbox_projection.tenant_id = $1
          AND receipt_inbox_projection.conversation_id = $2
          AND (
              (requested.message_id IS NOT NULL AND receipt_inbox_projection.message_id = requested.message_id)
              OR (requested.conversation_seq IS NOT NULL AND receipt_inbox_projection.conversation_seq = requested.conversation_seq)
          )
        ORDER BY receipt_inbox_projection.conversation_seq ASC, receipt_inbox_projection.user_id ASC
        LIMIT 1
    ) AS matched ON TRUE
)
SELECT
    resolved.ord,
    resolved.conversation_seq,
    resolved.message_id,
    message_receipt_states.user_id,
    CASE WHEN message_receipt_states.received_at IS NULL THEN 0 ELSE message_receipt_states.conversation_seq END AS received_seq,
    message_receipt_states.received_at,
    CASE WHEN message_receipt_states.read_at IS NULL THEN 0 ELSE message_receipt_states.conversation_seq END AS read_seq,
    message_receipt_states.read_at,
    (
        SELECT COUNT(*)
        FROM device_received_cursors
        WHERE device_received_cursors.tenant_id = message_receipt_states.tenant_id
          AND device_received_cursors.user_id = message_receipt_states.user_id
          AND device_received_cursors.conversation_id = message_receipt_states.conversation_id
          AND device_received_cursors.last_received_seq >= message_receipt_states.conversation_seq
    ) AS received_device_count
FROM resolved
JOIN message_receipt_states
  ON message_receipt_states.tenant_id = $1
 AND message_receipt_states.conversation_id = $2
 AND message_receipt_states.conversation_seq = resolved.conversation_seq
ORDER BY resolved.ord ASC, message_receipt_states.user_id ASC
`, strings.Join(valueClauses, ",\n    "))

	rows, err := repository.pool.Query(ctx, query, args...)
	if err != nil {
		return types.ListReceiptStatesResult{}, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	items := make([]types.GetReceiptStateResult, len(command.Items))
	seen := make([]bool, len(command.Items))
	for rows.Next() {
		var ord int
		var receiver types.ReceiptUserState
		var conversationSeq int64
		var messageID string
		var receivedAt sql.NullTime
		var readAt sql.NullTime
		if err := rows.Scan(
			&ord,
			&conversationSeq,
			&messageID,
			&receiver.UserID,
			&receiver.ReceivedSeq,
			&receivedAt,
			&receiver.ReadSeq,
			&readAt,
			&receiver.ReceivedDeviceCount,
		); err != nil {
			return types.ListReceiptStatesResult{}, types.NewDBReadFailed(err.Error())
		}
		if ord < 0 || ord >= len(items) {
			return types.ListReceiptStatesResult{}, types.NewDBReadFailed("receipt state batch ordinal out of range")
		}
		if !seen[ord] {
			items[ord] = types.GetReceiptStateResult{
				ConversationID:  command.ConversationID,
				ConversationSeq: conversationSeq,
				MessageID:       messageID,
				VisibilityMode:  types.ReceiptVisibilityDetailed,
				Receivers:       make([]types.ReceiptUserState, 0),
			}
			seen[ord] = true
		}
		if receivedAt.Valid {
			receiver.ReceivedAt = receivedAt.Time
		}
		if readAt.Valid {
			receiver.ReadAt = readAt.Time
		}
		if receiver.ReceivedSeq > 0 {
			items[ord].ReceivedUserCount++
		}
		if receiver.ReadSeq > 0 {
			items[ord].ReadUserCount++
		}
		items[ord].Receivers = append(items[ord].Receivers, receiver)
	}
	if err := rows.Err(); err != nil {
		return types.ListReceiptStatesResult{}, types.NewDBReadFailed(err.Error())
	}
	for index, ok := range seen {
		if !ok || len(items[index].Receivers) == 0 {
			return types.ListReceiptStatesResult{}, types.NewReceiptNotFound("receipt state not found")
		}
	}
	if command.IncludeReceivedDevices {
		itemRefs := make([]*types.GetReceiptStateResult, 0, len(items))
		for index := range items {
			itemRefs = append(itemRefs, &items[index])
		}
		if err := repository.attachReceivedDeviceDetails(
			ctx,
			command.AuthContext.TenantID,
			command.ConversationID,
			itemRefs,
			command.ReceivedDeviceLimit(),
		); err != nil {
			return types.ListReceiptStatesResult{}, err
		}
	}
	return types.ListReceiptStatesResult{Items: items}, nil
}

func (repository *Repository) attachReceivedDeviceDetails(
	ctx context.Context,
	tenantID types.TenantID,
	conversationID types.ConversationID,
	items []*types.GetReceiptStateResult,
	limit int,
) error {
	if limit <= 0 {
		return nil
	}
	args := []any{tenantID, conversationID, limit}
	valueClauses := make([]string, 0)
	receiverIndex := make(map[string]*types.ReceiptUserState)
	for itemIndex, item := range items {
		for receiverIndexInItem := range item.Receivers {
			receiver := &item.Receivers[receiverIndexInItem]
			if receiver.ReceivedDeviceCount == 0 {
				continue
			}
			args = append(args, string(receiver.UserID), item.ConversationSeq)
			valueClauses = append(
				valueClauses,
				fmt.Sprintf("(%d, $%d::text, $%d::bigint)", itemIndex, len(args)-1, len(args)),
			)
			receiverIndex[receivedDeviceDetailKey(itemIndex, receiver.UserID)] = receiver
		}
	}
	if len(valueClauses) == 0 {
		return nil
	}
	query := fmt.Sprintf(`
WITH requested(ord, user_id, conversation_seq) AS (
    VALUES %s
),
ranked AS (
    SELECT
        requested.ord,
        requested.user_id,
        device_received_cursors.device_id,
        device_received_cursors.last_received_seq,
        device_received_cursors.updated_at,
        ROW_NUMBER() OVER (
            PARTITION BY requested.ord, requested.user_id
            ORDER BY device_received_cursors.last_received_seq DESC,
                     device_received_cursors.updated_at DESC,
                     device_received_cursors.device_id ASC
        ) AS row_number
    FROM requested
    JOIN device_received_cursors
      ON device_received_cursors.tenant_id = $1
     AND device_received_cursors.conversation_id = $2
     AND device_received_cursors.user_id = requested.user_id
     AND device_received_cursors.last_received_seq >= requested.conversation_seq
)
SELECT ord, user_id, device_id, last_received_seq, updated_at
FROM ranked
WHERE row_number <= $3
ORDER BY ord ASC, user_id ASC, row_number ASC
`, strings.Join(valueClauses, ",\n    "))
	rows, err := repository.pool.Query(ctx, query, args...)
	if err != nil {
		return types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()
	for rows.Next() {
		var ord int
		var userID types.UserID
		var detail types.ReceivedDeviceState
		if err := rows.Scan(
			&ord,
			&userID,
			&detail.DeviceID,
			&detail.LastReceivedSeq,
			&detail.UpdatedAt,
		); err != nil {
			return types.NewDBReadFailed(err.Error())
		}
		receiver := receiverIndex[receivedDeviceDetailKey(ord, userID)]
		if receiver == nil {
			return types.NewDBReadFailed("receipt received device detail ordinal out of range")
		}
		receiver.ReceivedDevices = append(receiver.ReceivedDevices, detail)
	}
	if err := rows.Err(); err != nil {
		return types.NewDBReadFailed(err.Error())
	}
	for _, receiver := range receiverIndex {
		receiver.ReceivedDevicesTruncated = receiver.ReceivedDeviceCount > len(receiver.ReceivedDevices)
	}
	return nil
}

func receivedDeviceDetailKey(itemIndex int, userID types.UserID) string {
	return fmt.Sprintf("%d\x1f%s", itemIndex, userID)
}
