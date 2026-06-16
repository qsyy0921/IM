package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
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

func (repository *Repository) ListConversations(
	ctx context.Context,
	command types.ListConversationsCommand,
) (types.ListConversationsResult, error) {
	if err := command.Validate(); err != nil {
		return types.ListConversationsResult{}, err
	}
	sort, err := types.NormalizeConversationListSort(command.Sort)
	if err != nil {
		return types.ListConversationsResult{}, err
	}
	tagFilter := ""
	if command.TagFilter != "" {
		tagFilter, err = types.NormalizeConversationTag(command.TagFilter)
		if err != nil {
			return types.ListConversationsResult{}, err
		}
	}
	limit := command.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	cursor, hasCursor, err := decodeListCursor(command.PageCursor, sort, command.IncludeArchived, command.UnreadOnly, command.PinnedOnly, command.MutedOnly, command.DraftOnly, tagFilter)
	if err != nil {
		return types.ListConversationsResult{}, err
	}

	args := []any{
		command.AuthContext.TenantID,
		command.AuthContext.UserID,
		limit + 1,
		command.IncludeArchived,
		command.UnreadOnly,
		command.PinnedOnly,
		command.MutedOnly,
		command.DraftOnly,
		tagFilter,
	}
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
    pinned,
    muted,
    tags,
    draft_text,
    draft_updated_at
FROM user_conversation_summaries
WHERE tenant_id = $1
  AND user_id = $2
  AND ($4 OR archived = FALSE)
  AND (NOT $5 OR unread_count > 0)
  AND (NOT $6 OR pinned = TRUE)
  AND (NOT $7 OR muted = TRUE)
  AND (NOT $8 OR draft_text <> '')
  AND ($9 = '' OR $9 = ANY(tags))
`
	if hasCursor {
		switch sort {
		case types.ConversationListSortPinnedUpdatedAtDesc:
			query += `  AND (
      pinned < $10
      OR (pinned = $10 AND sort_updated_at < $11)
      OR (pinned = $10 AND sort_updated_at = $11 AND conversation_id > $12)
  )
`
			args = append(args, cursor.Pinned, cursor.SortUpdatedAt, cursor.ConversationID)
		case types.ConversationListSortUnreadUpdatedAtDesc:
			query += `  AND (
      (unread_count > 0) < $10
      OR ((unread_count > 0) = $10 AND sort_updated_at < $11)
      OR ((unread_count > 0) = $10 AND sort_updated_at = $11 AND conversation_id > $12)
  )
`
			args = append(args, cursor.Unread, cursor.SortUpdatedAt, cursor.ConversationID)
		default:
			query += `  AND (
      sort_updated_at < $10
      OR (sort_updated_at = $10 AND conversation_id > $11)
  )
`
			args = append(args, cursor.SortUpdatedAt, cursor.ConversationID)
		}
	}
	switch sort {
	case types.ConversationListSortPinnedUpdatedAtDesc:
		query += `ORDER BY pinned DESC, sort_updated_at DESC, conversation_id ASC
LIMIT $3
`
	case types.ConversationListSortUnreadUpdatedAtDesc:
		query += `ORDER BY (unread_count > 0) DESC, sort_updated_at DESC, conversation_id ASC
LIMIT $3
`
	default:
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
		if err := scanConversationSummary(rows, &item); err != nil {
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
			UnreadOnly:      command.UnreadOnly,
			PinnedOnly:      command.PinnedOnly,
			MutedOnly:       command.MutedOnly,
			DraftOnly:       command.DraftOnly,
			TagFilter:       tagFilter,
			Pinned:          last.Pinned,
			Unread:          last.UnreadCount > 0,
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
	row := repository.pool.QueryRow(ctx, `
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
    muted,
    tags,
    draft_text,
    draft_updated_at
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID, command.Archived)
	err := scanConversationSummary(row, &item)
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
	row := repository.pool.QueryRow(ctx, `
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
    muted,
    tags,
    draft_text,
    draft_updated_at
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID, command.Pinned)
	err := scanConversationSummary(row, &item)
	if err == pgx.ErrNoRows {
		return types.PinConversationResult{}, types.NewConversationNotFound("conversation summary not found")
	}
	if err != nil {
		return types.PinConversationResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.PinConversationResult{Conversation: item}, nil
}

func (repository *Repository) MuteConversation(
	ctx context.Context,
	command types.MuteConversationCommand,
) (types.MuteConversationResult, error) {
	if err := command.Validate(); err != nil {
		return types.MuteConversationResult{}, err
	}
	var item types.ConversationSummary
	row := repository.pool.QueryRow(ctx, `
UPDATE user_conversation_summaries
SET muted = $4,
    muted_at = CASE WHEN $4 THEN now() ELSE NULL END,
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
    muted,
    tags,
    draft_text,
    draft_updated_at
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID, command.Muted)
	err := scanConversationSummary(row, &item)
	if err == pgx.ErrNoRows {
		return types.MuteConversationResult{}, types.NewConversationNotFound("conversation summary not found")
	}
	if err != nil {
		return types.MuteConversationResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.MuteConversationResult{Conversation: item}, nil
}

func (repository *Repository) SetConversationTags(
	ctx context.Context,
	command types.SetConversationTagsCommand,
) (types.SetConversationTagsResult, error) {
	if err := command.Validate(); err != nil {
		return types.SetConversationTagsResult{}, err
	}
	tags, err := types.NormalizeConversationTags(command.Tags)
	if err != nil {
		return types.SetConversationTagsResult{}, err
	}
	var item types.ConversationSummary
	row := repository.pool.QueryRow(ctx, `
UPDATE user_conversation_summaries
SET tags = $4,
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
    muted,
    tags,
    draft_text,
    draft_updated_at
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID, tags)
	err = scanConversationSummary(row, &item)
	if err == pgx.ErrNoRows {
		return types.SetConversationTagsResult{}, types.NewConversationNotFound("conversation summary not found")
	}
	if err != nil {
		return types.SetConversationTagsResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.SetConversationTagsResult{Conversation: item}, nil
}

func (repository *Repository) SetConversationDraft(
	ctx context.Context,
	command types.SetConversationDraftCommand,
) (types.SetConversationDraftResult, error) {
	if err := command.Validate(); err != nil {
		return types.SetConversationDraftResult{}, err
	}
	draftText, err := types.NormalizeConversationDraft(command.DraftText)
	if err != nil {
		return types.SetConversationDraftResult{}, err
	}
	var item types.ConversationSummary
	row := repository.pool.QueryRow(ctx, `
UPDATE user_conversation_summaries
SET draft_text = $4,
    draft_updated_at = CASE WHEN $4 = '' THEN NULL ELSE now() END,
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
    muted,
    tags,
    draft_text,
    draft_updated_at
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID, draftText)
	err = scanConversationSummary(row, &item)
	if err == pgx.ErrNoRows {
		return types.SetConversationDraftResult{}, types.NewConversationNotFound("conversation summary not found")
	}
	if err != nil {
		return types.SetConversationDraftResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.SetConversationDraftResult{Conversation: item}, nil
}

type conversationSummaryScanner interface {
	Scan(dest ...any) error
}

func scanConversationSummary(scanner conversationSummaryScanner, item *types.ConversationSummary) error {
	var draftUpdatedAt sql.NullTime
	if err := scanner.Scan(
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
		&item.Muted,
		&item.Tags,
		&item.DraftText,
		&draftUpdatedAt,
	); err != nil {
		return err
	}
	if draftUpdatedAt.Valid {
		item.DraftUpdatedAt = draftUpdatedAt.Time
	}
	return nil
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
	UnreadOnly      bool      `json:"unread_only"`
	PinnedOnly      bool      `json:"pinned_only"`
	MutedOnly       bool      `json:"muted_only"`
	DraftOnly       bool      `json:"draft_only"`
	TagFilter       string    `json:"tag_filter"`
	Pinned          bool      `json:"pinned"`
	Unread          bool      `json:"unread"`
	SortUpdatedAt   time.Time `json:"sort_updated_at"`
	ConversationID  string    `json:"conversation_id"`
}

const listCursorVersion = 5

func decodeListCursor(
	value string,
	sort string,
	includeArchived bool,
	unreadOnly bool,
	pinnedOnly bool,
	mutedOnly bool,
	draftOnly bool,
	tagFilter string,
) (listCursor, bool, error) {
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
	if cursor.Version != listCursorVersion {
		return listCursor{}, false, types.NewInvalidArgument("invalid page_cursor")
	}
	if cursor.Sort != sort ||
		cursor.IncludeArchived != includeArchived ||
		cursor.UnreadOnly != unreadOnly ||
		cursor.PinnedOnly != pinnedOnly ||
		cursor.MutedOnly != mutedOnly ||
		cursor.DraftOnly != draftOnly ||
		cursor.TagFilter != tagFilter {
		return listCursor{}, false, types.NewInvalidArgument("invalid page_cursor")
	}
	if sort == types.ConversationListSortUnreadUpdatedAtDesc && cursor.Version < listCursorVersion {
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
