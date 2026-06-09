package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

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
		if err := updateConversationSummaryAfterRead(ctx, tx, command, next); err != nil {
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

func (repository *Repository) ListConversations(
	ctx context.Context,
	command types.ListConversationsCommand,
) (types.ListConversationsResult, error) {
	sort, err := types.NormalizeConversationListSort(command.Sort)
	if err != nil {
		return types.ListConversationsResult{}, err
	}
	limit := command.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	cursor, hasCursor, err := decodeListCursor(command.PageCursor, sort, command.IncludeArchived)
	if err != nil {
		return types.ListConversationsResult{}, err
	}

	args := []any{command.AuthContext.TenantID, command.AuthContext.UserID, limit + 1, command.IncludeArchived}
	query := `
SELECT
    conversation_id,
    last_visible_seq,
    last_message_id,
    last_sender_id,
    last_source_event_type,
    unread_count,
    last_read_seq,
    sort_updated_at,
    archived,
    pinned
FROM user_conversation_summaries
WHERE tenant_id = $1
  AND user_id = $2
  AND ($4 OR archived = FALSE)
`
	if hasCursor {
		if sort == types.ConversationListSortPinnedUpdatedAtDesc {
			query += `  AND (
      pinned < $5
      OR (pinned = $5 AND sort_updated_at < $6)
      OR (pinned = $5 AND sort_updated_at = $6 AND conversation_id > $7)
  )
`
			args = append(args, cursor.Pinned, cursor.SortUpdatedAt, cursor.ConversationID)
		} else {
			query += `  AND (
      sort_updated_at < $5
      OR (sort_updated_at = $5 AND conversation_id > $6)
  )
`
			args = append(args, cursor.SortUpdatedAt, cursor.ConversationID)
		}
	}
	if sort == types.ConversationListSortPinnedUpdatedAtDesc {
		query += `ORDER BY pinned DESC, sort_updated_at DESC, conversation_id ASC
LIMIT $3
`
	} else {
		query += `ORDER BY sort_updated_at DESC, conversation_id ASC
LIMIT $3
`
	}

	rows, err := repository.pool.Query(ctx, query, args...)
	if err != nil {
		return types.ListConversationsResult{}, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	items := make([]types.ConversationSummary, 0, limit)
	for rows.Next() {
		var item types.ConversationSummary
		if err := rows.Scan(
			&item.ConversationID,
			&item.LastVisibleSeq,
			&item.LastMessageID,
			&item.LastSenderID,
			&item.LastSourceEventType,
			&item.UnreadCount,
			&item.LastReadSeq,
			&item.UpdatedAt,
			&item.Archived,
			&item.Pinned,
		); err != nil {
			return types.ListConversationsResult{}, types.NewDBReadFailed(err.Error())
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return types.ListConversationsResult{}, types.NewDBReadFailed(err.Error())
	}

	nextCursor := ""
	if len(items) > limit {
		last := items[limit-1]
		nextCursor = encodeListCursor(listCursor{
			Version:         listCursorVersion,
			Sort:            sort,
			IncludeArchived: command.IncludeArchived,
			Pinned:          last.Pinned,
			SortUpdatedAt:   last.UpdatedAt,
			ConversationID:  string(last.ConversationID),
		})
		items = items[:limit]
	}
	watermark, err := repository.conversationSummaryWatermark(ctx)
	if err != nil {
		return types.ListConversationsResult{}, err
	}
	return types.ListConversationsResult{
		Items:               items,
		NextPageCursor:      nextCursor,
		ProjectionWatermark: watermark,
	}, nil
}

func (repository *Repository) ArchiveConversation(
	ctx context.Context,
	command types.ArchiveConversationCommand,
) (types.ArchiveConversationResult, error) {
	if err := command.Validate(); err != nil {
		return types.ArchiveConversationResult{}, err
	}
	var item types.ConversationSummary
	var archivedAt sql.NullTime
	err := repository.pool.QueryRow(ctx, `
UPDATE user_conversation_summaries
SET archived = $4,
    archived_at = CASE WHEN $4 THEN now() ELSE NULL END,
    updated_at = now()
WHERE tenant_id = $1
  AND user_id = $2
  AND conversation_id = $3
RETURNING
    conversation_id,
    last_visible_seq,
    last_message_id,
    last_sender_id,
    last_source_event_type,
    unread_count,
    last_read_seq,
    sort_updated_at,
    archived,
    pinned,
    archived_at
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID, command.Archived).Scan(
		&item.ConversationID,
		&item.LastVisibleSeq,
		&item.LastMessageID,
		&item.LastSenderID,
		&item.LastSourceEventType,
		&item.UnreadCount,
		&item.LastReadSeq,
		&item.UpdatedAt,
		&item.Archived,
		&item.Pinned,
		&archivedAt,
	)
	if err == pgx.ErrNoRows {
		return types.ArchiveConversationResult{}, types.NewConversationNotFound("conversation summary not found")
	}
	if err != nil {
		return types.ArchiveConversationResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.ArchiveConversationResult{Conversation: item}, nil
}

func (repository *Repository) PinConversation(
	ctx context.Context,
	command types.PinConversationCommand,
) (types.PinConversationResult, error) {
	if err := command.Validate(); err != nil {
		return types.PinConversationResult{}, err
	}
	var item types.ConversationSummary
	var pinnedAt sql.NullTime
	err := repository.pool.QueryRow(ctx, `
UPDATE user_conversation_summaries
SET pinned = $4,
    pinned_at = CASE WHEN $4 THEN now() ELSE NULL END,
    updated_at = now()
WHERE tenant_id = $1
  AND user_id = $2
  AND conversation_id = $3
RETURNING
    conversation_id,
    last_visible_seq,
    last_message_id,
    last_sender_id,
    last_source_event_type,
    unread_count,
    last_read_seq,
    sort_updated_at,
    archived,
    pinned,
    pinned_at
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID, command.Pinned).Scan(
		&item.ConversationID,
		&item.LastVisibleSeq,
		&item.LastMessageID,
		&item.LastSenderID,
		&item.LastSourceEventType,
		&item.UnreadCount,
		&item.LastReadSeq,
		&item.UpdatedAt,
		&item.Archived,
		&item.Pinned,
		&pinnedAt,
	)
	if err == pgx.ErrNoRows {
		return types.PinConversationResult{}, types.NewConversationNotFound("conversation summary not found")
	}
	if err != nil {
		return types.PinConversationResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.PinConversationResult{Conversation: item}, nil
}

func (repository *Repository) conversationSummaryWatermark(ctx context.Context) (types.ProjectionWatermark, error) {
	var watermark types.ProjectionWatermark
	var updatedAt sql.NullTime
	err := repository.pool.QueryRow(ctx, `
SELECT
    COALESCE(MAX(offset_value), 0),
    MAX(updated_at)
FROM conversation_summary_checkpoints
`).Scan(&watermark.OffsetValue, &updatedAt)
	if err != nil {
		return types.ProjectionWatermark{}, types.NewDBReadFailed(err.Error())
	}
	watermark.Source = "im.delivery.events"
	if updatedAt.Valid {
		watermark.UpdatedAt = updatedAt.Time
	}
	return watermark, nil
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

type listCursor struct {
	Version         int       `json:"v"`
	Sort            string    `json:"sort"`
	IncludeArchived bool      `json:"include_archived"`
	Pinned          bool      `json:"pinned"`
	SortUpdatedAt   time.Time `json:"sort_updated_at"`
	ConversationID  string    `json:"conversation_id"`
}

const listCursorVersion = 2

func decodeListCursor(value string, sort string, includeArchived bool) (listCursor, bool, error) {
	if value == "" {
		return listCursor{}, false, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return listCursor{}, false, types.NewInvalidArgument("invalid page_cursor")
	}
	var cursor listCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return listCursor{}, false, types.NewInvalidArgument("invalid page_cursor")
	}
	if cursor.Version == 0 && cursor.Sort == "" {
		cursor.Version = 1
		cursor.Sort = types.ConversationListSortUpdatedAtDesc
	}
	if cursor.Version != listCursorVersion || cursor.Sort != sort || cursor.IncludeArchived != includeArchived {
		return listCursor{}, false, types.NewInvalidArgument("invalid page_cursor")
	}
	if cursor.SortUpdatedAt.IsZero() || cursor.ConversationID == "" {
		return listCursor{}, false, types.NewInvalidArgument("invalid page_cursor")
	}
	return cursor, true, nil
}

func encodeListCursor(cursor listCursor) string {
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(encoded)
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

func lockReadKey(ctx context.Context, tx pgx.Tx, command types.MarkReadCommand) error {
	return lockConversationSummaryKey(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID)
}

func lockConversationSummaryKey(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	userID types.UserID,
	conversationID types.ConversationID,
) error {
	key := fmt.Sprintf("%s\x1f%s\x1f%s\x1fconversation_summary", tenantID, userID, conversationID)
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

func updateConversationSummaryAfterRead(ctx context.Context, tx pgx.Tx, command types.MarkReadCommand, readSeq int64) error {
	_, err := tx.Exec(ctx, `
UPDATE user_conversation_summaries
SET last_read_seq = GREATEST(last_read_seq, $4),
    unread_count = (
        SELECT COUNT(*)
        FROM receipt_inbox_projection
        WHERE tenant_id = $1
          AND user_id = $2
          AND conversation_id = $3
          AND source_event_type = 'message.persisted.v1'
          AND conversation_seq > $4
    ),
    updated_at = now()
WHERE tenant_id = $1
  AND user_id = $2
  AND conversation_id = $3
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
