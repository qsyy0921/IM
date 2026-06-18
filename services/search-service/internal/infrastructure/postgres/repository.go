package postgres

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/search-service/internal/types"
)

const (
	timelineEventMessagePersisted = "message.persisted.v1"
	timelineEventMessageEdited    = "message.edited.v1"
	timelineEventMessageRevoked   = "message.revoked.v1"
	timelineEventMessageDeleted   = "message.deleted.v1"

	timelineEventMemberJoined           = "conversation.member.joined.v1"
	timelineEventMemberLeft             = "conversation.member.left.v1"
	timelineEventMemberRemoved          = "conversation.member.removed.v1"
	timelineEventMemberRoleChanged      = "conversation.member.role_changed.v1"
	timelineEventMemberOwnerTransferred = "conversation.member.owner_transferred.v1"

	tombstoneNone    = "NONE"
	tombstoneRevoked = "REVOKED"
	tombstoneDeleted = "DELETED"

	memberStatusActive  = "ACTIVE"
	memberStatusLeft    = "LEFT"
	memberStatusRemoved = "REMOVED"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (repository *Repository) SearchMessages(
	ctx context.Context,
	command types.SearchMessagesCommand,
	fetchLimit int,
) ([]types.SearchMessageHit, int64, error) {
	if repository.pool == nil {
		return nil, 0, types.NewDBReadFailed("search repository is not configured")
	}
	if fetchLimit <= 0 {
		return nil, 0, nil
	}

	queryPattern := "%" + escapeLike(command.NormalizedQuery()) + "%"
	args := []any{
		command.AuthContext.TenantID,
		command.AuthContext.UserID,
		queryPattern,
		command.AfterSeq,
		fetchLimit,
	}
	conversationFilter := ""
	if command.ConversationID != "" {
		args = append(args, command.ConversationID)
		conversationFilter = "AND d.conversation_id = $6"
	}

	rows, err := repository.pool.Query(ctx, `
SELECT
	d.conversation_id,
	d.message_id,
	d.conversation_seq,
	d.source_event_id,
	d.sender_id,
	d.message_type,
	d.searchable_text,
	d.occurred_at,
	d.visibility_version,
	COALESCE(MAX(d.visibility_version) OVER (), 0) AS projection_version
FROM search_message_documents d
JOIN search_membership_projection m
  ON m.tenant_id = d.tenant_id
 AND m.conversation_id = d.conversation_id
 AND m.user_id = $2
WHERE d.tenant_id = $1
  AND m.status <> 'BANNED'
  AND d.tombstone_status = 'NONE'
  AND m.join_seq <= d.conversation_seq
  AND (m.leave_seq IS NULL OR m.leave_seq >= d.conversation_seq)
  AND d.searchable_text ILIKE $3 ESCAPE '\'
  AND d.conversation_seq > $4
`+conversationFilter+`
ORDER BY d.conversation_seq ASC, d.message_id ASC
LIMIT $5
`, args...)
	if err != nil {
		return nil, 0, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	items := make([]types.SearchMessageHit, 0, fetchLimit)
	var projectionVersion int64
	for rows.Next() {
		var hit types.SearchMessageHit
		var text string
		if err := rows.Scan(
			&hit.ConversationID,
			&hit.MessageID,
			&hit.ConversationSeq,
			&hit.SourceEventID,
			&hit.SenderID,
			&hit.MessageType,
			&text,
			&hit.OccurredAt,
			&hit.VisibilityVersion,
			&projectionVersion,
		); err != nil {
			return nil, 0, types.NewDBReadFailed(err.Error())
		}
		hit.Snippet, hit.HighlightRanges = buildSnippet(text, command.NormalizedQuery(), 160)
		items = append(items, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, types.NewDBReadFailed(err.Error())
	}
	return items, projectionVersion, nil
}

func (repository *Repository) ProjectTimelineEvent(
	ctx context.Context,
	command types.ProjectTimelineEventCommand,
) (types.ProjectTimelineEventResult, error) {
	if repository.pool == nil {
		return types.ProjectTimelineEventResult{}, types.NewDBWriteFailed("search repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.ProjectTimelineEventResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	result := types.ProjectTimelineEventResult{
		TenantID:          command.TenantID,
		EventID:           command.EventID,
		ConversationID:    command.ConversationID,
		ConversationSeq:   command.ConversationSeq,
		ProjectedDocument: false,
		ProjectedMember:   false,
	}

	switch command.EventType {
	case timelineEventMessagePersisted, timelineEventMessageEdited:
		if err := upsertMessageDocument(ctx, tx, command, tombstoneNone); err != nil {
			return types.ProjectTimelineEventResult{}, err
		}
		result.ProjectedDocument = true
	case timelineEventMessageRevoked:
		if err := tombstoneMessageDocument(ctx, tx, command, tombstoneRevoked); err != nil {
			return types.ProjectTimelineEventResult{}, err
		}
		result.ProjectedDocument = true
	case timelineEventMessageDeleted:
		status := strings.TrimSpace(command.TombstoneStatus)
		if status == "" {
			status = tombstoneDeleted
		}
		if err := tombstoneMessageDocument(ctx, tx, command, status); err != nil {
			return types.ProjectTimelineEventResult{}, err
		}
		result.ProjectedDocument = true
	case timelineEventMemberJoined:
		if err := upsertMembership(ctx, tx, command, memberStatusActive, true); err != nil {
			return types.ProjectTimelineEventResult{}, err
		}
		result.ProjectedMember = true
	case timelineEventMemberLeft:
		if err := upsertMembership(ctx, tx, command, memberStatusLeft, false); err != nil {
			return types.ProjectTimelineEventResult{}, err
		}
		result.ProjectedMember = true
	case timelineEventMemberRemoved:
		if err := upsertMembership(ctx, tx, command, memberStatusRemoved, false); err != nil {
			return types.ProjectTimelineEventResult{}, err
		}
		result.ProjectedMember = true
	case timelineEventMemberRoleChanged, timelineEventMemberOwnerTransferred:
		if err := upsertMembership(ctx, tx, command, memberStatusActive, false); err != nil {
			return types.ProjectTimelineEventResult{}, err
		}
		result.ProjectedMember = true
	default:
		return types.ProjectTimelineEventResult{}, types.NewUnsupportedPayload("unsupported timeline event type")
	}

	if err := upsertCheckpoint(ctx, tx, command); err != nil {
		return types.ProjectTimelineEventResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.ProjectTimelineEventResult{}, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func upsertMessageDocument(
	ctx context.Context,
	tx pgx.Tx,
	command types.ProjectTimelineEventCommand,
	tombstoneStatus string,
) error {
	if command.MessageID == "" {
		return types.NewInvalidArgument("message_id is required")
	}
	if tombstoneStatus == "" {
		tombstoneStatus = tombstoneNone
	}
	_, err := tx.Exec(ctx, `
INSERT INTO search_message_documents (
	tenant_id,
	conversation_id,
	message_id,
	conversation_seq,
	source_event_id,
	searchable_text,
	message_type,
	sender_id,
	tombstone_status,
	change_version,
	visibility_version,
	occurred_at,
	updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 1, $10, now(), now())
ON CONFLICT (tenant_id, conversation_id, message_id) DO UPDATE SET
	conversation_seq = EXCLUDED.conversation_seq,
	source_event_id = EXCLUDED.source_event_id,
	searchable_text = EXCLUDED.searchable_text,
	message_type = EXCLUDED.message_type,
	sender_id = EXCLUDED.sender_id,
	tombstone_status = EXCLUDED.tombstone_status,
	change_version = search_message_documents.change_version + 1,
	visibility_version = EXCLUDED.visibility_version,
	updated_at = now()
`, command.TenantID,
		command.ConversationID,
		command.MessageID,
		command.ConversationSeq,
		command.EventID,
		command.SearchableText,
		command.MessageType,
		command.SenderID,
		tombstoneStatus,
		command.PermissionVersion,
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func tombstoneMessageDocument(
	ctx context.Context,
	tx pgx.Tx,
	command types.ProjectTimelineEventCommand,
	tombstoneStatus string,
) error {
	if command.MessageID == "" {
		return types.NewInvalidArgument("message_id is required")
	}
	if tombstoneStatus == "" {
		tombstoneStatus = tombstoneDeleted
	}
	tag, err := tx.Exec(ctx, `
UPDATE search_message_documents
SET source_event_id = $5,
    tombstone_status = $6,
    searchable_text = CASE WHEN $6 = 'COMPLIANCE_REDACTED' THEN '' ELSE searchable_text END,
    visibility_version = $7,
    updated_at = now()
WHERE tenant_id = $1
  AND conversation_id = $2
  AND message_id = $3
  AND conversation_seq <= $4
`, command.TenantID,
		command.ConversationID,
		command.MessageID,
		command.ConversationSeq,
		command.EventID,
		tombstoneStatus,
		command.PermissionVersion,
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	return upsertMessageDocument(ctx, tx, command, tombstoneStatus)
}

func upsertMembership(
	ctx context.Context,
	tx pgx.Tx,
	command types.ProjectTimelineEventCommand,
	status string,
	resetJoin bool,
) error {
	if command.TargetUserID == "" {
		return types.NewInvalidArgument("target_user_id is required")
	}
	role := strings.TrimSpace(command.MemberRole)
	joinSeq := command.ConversationSeq
	leaveSeq := any(nil)
	if status != memberStatusActive {
		leaveSeq = command.ConversationSeq
	}
	if resetJoin {
		leaveSeq = nil
	}

	_, err := tx.Exec(ctx, `
INSERT INTO search_membership_projection (
	tenant_id,
	conversation_id,
	user_id,
	role,
	status,
	join_seq,
	leave_seq,
	member_version,
	permission_version,
	updated_by_event_id,
	updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
ON CONFLICT (tenant_id, conversation_id, user_id) DO UPDATE SET
	role = CASE WHEN EXCLUDED.role <> '' THEN EXCLUDED.role ELSE search_membership_projection.role END,
	status = EXCLUDED.status,
	join_seq = CASE WHEN $11 THEN EXCLUDED.join_seq ELSE search_membership_projection.join_seq END,
	leave_seq = EXCLUDED.leave_seq,
	member_version = EXCLUDED.member_version,
	permission_version = EXCLUDED.permission_version,
	updated_by_event_id = EXCLUDED.updated_by_event_id,
	updated_at = now()
`, command.TenantID,
		command.ConversationID,
		command.TargetUserID,
		role,
		status,
		joinSeq,
		leaveSeq,
		command.MemberVersion,
		command.PermissionVersion,
		command.EventID,
		resetJoin,
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func upsertCheckpoint(ctx context.Context, tx pgx.Tx, command types.ProjectTimelineEventCommand) error {
	_, err := tx.Exec(ctx, `
INSERT INTO search_projection_checkpoints (
	consumer_group,
	topic,
	partition_id,
	offset_value,
	updated_at
) VALUES ($1, $2, $3, $4, now())
ON CONFLICT (consumer_group, topic, partition_id) DO UPDATE SET
	offset_value = GREATEST(search_projection_checkpoints.offset_value, EXCLUDED.offset_value),
	updated_at = now()
`, command.ConsumerGroup, command.Topic, command.PartitionID, command.OffsetValue)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return value
}

func buildSnippet(text string, query string, maxRunes int) (string, []types.HighlightRange) {
	text = strings.TrimSpace(text)
	query = strings.TrimSpace(query)
	if text == "" || query == "" {
		return truncateRunes(text, maxRunes), nil
	}
	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)
	byteIndex := strings.Index(lowerText, lowerQuery)
	if byteIndex < 0 {
		return truncateRunes(text, maxRunes), nil
	}
	startRune := utf8.RuneCountInString(text[:byteIndex])
	endRune := startRune + utf8.RuneCountInString(text[byteIndex:byteIndex+len(query)])
	snippet := truncateRunes(text, maxRunes)
	if startRune > utf8.RuneCountInString(snippet) {
		return snippet, nil
	}
	if endRune > utf8.RuneCountInString(snippet) {
		endRune = utf8.RuneCountInString(snippet)
	}
	return snippet, []types.HighlightRange{{Start: int32(startRune), End: int32(endRune)}}
}

func truncateRunes(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}

func IsNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

func UTCNow() time.Time {
	return time.Now().UTC()
}
