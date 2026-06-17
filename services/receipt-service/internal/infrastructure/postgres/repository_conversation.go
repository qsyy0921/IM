package postgres

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

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
	tagFilters, err := types.NormalizeConversationTagFilters(command.TagFilters)
	if err != nil {
		return types.ListConversationsResult{}, err
	}
	lastSourceEventTypeFilter, err := types.NormalizeLastSourceEventTypeFilter(command.LastSourceEventTypeFilter)
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
	cursor, hasCursor, err := decodeListCursor(command.PageCursor, sort, command.IncludeArchived, command.ArchivedOnly, command.UnreadOnly, command.PinnedOnly, command.MutedOnly, command.ExcludeMuted, command.DraftOnly, tagFilter, tagFilters, lastSourceEventTypeFilter)
	if err != nil {
		return types.ListConversationsResult{}, err
	}

	args := []any{
		command.AuthContext.TenantID,
		command.AuthContext.UserID,
		limit + 1,
		command.IncludeArchived,
		command.ArchivedOnly,
		command.UnreadOnly,
		command.PinnedOnly,
		command.MutedOnly,
		command.ExcludeMuted,
		command.DraftOnly,
		tagFilter,
		tagFilters,
		lastSourceEventTypeFilter,
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
  AND (($5 AND archived = TRUE) OR (NOT $5 AND ($4 OR archived = FALSE)))
  AND (NOT $6 OR unread_count > 0)
  AND (NOT $7 OR pinned = TRUE)
  AND (NOT $8 OR muted = TRUE)
  AND (NOT $9 OR muted = FALSE)
  AND (NOT $10 OR draft_text <> '')
  AND ($11 = '' OR $11 = ANY(tags))
  AND (cardinality($12::text[]) = 0 OR tags @> $12::text[])
  AND ($13 = '' OR last_source_event_type = $13)
`
	if hasCursor {
		switch sort {
		case types.ConversationListSortPinnedUpdatedAtDesc:
			query += `  AND (
      pinned < $14
      OR (pinned = $14 AND sort_updated_at < $15)
      OR (pinned = $14 AND sort_updated_at = $15 AND conversation_id > $16)
  )
`
			args = append(args, cursor.Pinned, cursor.SortUpdatedAt, cursor.ConversationID)
		case types.ConversationListSortUnreadUpdatedAtDesc:
			query += `  AND (
      (unread_count > 0) < $14
      OR ((unread_count > 0) = $14 AND sort_updated_at < $15)
      OR ((unread_count > 0) = $14 AND sort_updated_at = $15 AND conversation_id > $16)
  )
`
			args = append(args, cursor.Unread, cursor.SortUpdatedAt, cursor.ConversationID)
		case types.ConversationListSortDraftUpdatedAtDesc:
			if cursor.Draft {
				query += `  AND (
      (draft_text <> '') < $14
      OR ((draft_text <> '') = $14 AND draft_updated_at < $15)
      OR ((draft_text <> '') = $14 AND draft_updated_at = $15 AND sort_updated_at < $16)
      OR ((draft_text <> '') = $14 AND draft_updated_at = $15 AND sort_updated_at = $16 AND conversation_id > $17)
  )
`
				args = append(args, cursor.Draft, cursor.DraftUpdatedAt, cursor.SortUpdatedAt, cursor.ConversationID)
			} else {
				query += `  AND (
      (draft_text <> '') < $14
      OR ((draft_text <> '') = $14 AND sort_updated_at < $15)
      OR ((draft_text <> '') = $14 AND sort_updated_at = $15 AND conversation_id > $16)
  )
`
				args = append(args, cursor.Draft, cursor.SortUpdatedAt, cursor.ConversationID)
			}
		default:
			query += `  AND (
      sort_updated_at < $14
      OR (sort_updated_at = $14 AND conversation_id > $15)
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
	case types.ConversationListSortDraftUpdatedAtDesc:
		query += `ORDER BY (draft_text <> '') DESC, draft_updated_at DESC NULLS LAST, sort_updated_at DESC, conversation_id ASC
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
			Version:                   listCursorVersion,
			Sort:                      sort,
			IncludeArchived:           command.IncludeArchived,
			ArchivedOnly:              command.ArchivedOnly,
			UnreadOnly:                command.UnreadOnly,
			PinnedOnly:                command.PinnedOnly,
			MutedOnly:                 command.MutedOnly,
			ExcludeMuted:              command.ExcludeMuted,
			DraftOnly:                 command.DraftOnly,
			TagFilter:                 tagFilter,
			TagFilters:                tagFilters,
			LastSourceEventTypeFilter: lastSourceEventTypeFilter,
			Pinned:                    last.Pinned,
			Unread:                    last.UnreadCount > 0,
			Draft:                     last.DraftText != "",
			DraftUpdatedAt:            last.DraftUpdatedAt,
			SortUpdatedAt:             last.UpdatedAt,
			ConversationID:            string(last.ConversationID),
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
	Version                   int       `json:"v"`
	Sort                      string    `json:"sort"`
	IncludeArchived           bool      `json:"include_archived"`
	ArchivedOnly              bool      `json:"archived_only"`
	UnreadOnly                bool      `json:"unread_only"`
	PinnedOnly                bool      `json:"pinned_only"`
	MutedOnly                 bool      `json:"muted_only"`
	ExcludeMuted              bool      `json:"exclude_muted"`
	DraftOnly                 bool      `json:"draft_only"`
	TagFilter                 string    `json:"tag_filter"`
	TagFilters                []string  `json:"tag_filters,omitempty"`
	LastSourceEventTypeFilter string    `json:"last_source_event_type_filter,omitempty"`
	Pinned                    bool      `json:"pinned"`
	Unread                    bool      `json:"unread"`
	Draft                     bool      `json:"draft"`
	DraftUpdatedAt            time.Time `json:"draft_updated_at"`
	SortUpdatedAt             time.Time `json:"sort_updated_at"`
	ConversationID            string    `json:"conversation_id"`
}

const listCursorVersion = 10

func decodeListCursor(
	value string,
	sort string,
	includeArchived bool,
	archivedOnly bool,
	unreadOnly bool,
	pinnedOnly bool,
	mutedOnly bool,
	excludeMuted bool,
	draftOnly bool,
	tagFilter string,
	tagFilters []string,
	lastSourceEventTypeFilter string,
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
		cursor.ArchivedOnly != archivedOnly ||
		cursor.UnreadOnly != unreadOnly ||
		cursor.PinnedOnly != pinnedOnly ||
		cursor.MutedOnly != mutedOnly ||
		cursor.ExcludeMuted != excludeMuted ||
		cursor.DraftOnly != draftOnly ||
		cursor.TagFilter != tagFilter ||
		!sameStringSlice(cursor.TagFilters, tagFilters) ||
		cursor.LastSourceEventTypeFilter != lastSourceEventTypeFilter {
		return listCursor{}, false, types.NewInvalidArgument("invalid page_cursor")
	}
	if sort == types.ConversationListSortDraftUpdatedAtDesc && cursor.Draft && cursor.DraftUpdatedAt.IsZero() {
		return listCursor{}, false, types.NewInvalidArgument("invalid page_cursor")
	}
	if cursor.SortUpdatedAt.IsZero() || cursor.ConversationID == "" {
		return listCursor{}, false, types.NewInvalidArgument("invalid page_cursor")
	}
	return cursor, true, nil
}

func sameStringSlice(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func encodeListCursor(cursor listCursor) string {
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(encoded)
}
