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

func (repository *Repository) ProjectTimelineEvent(
	ctx context.Context,
	command types.ProjectTimelineEventCommand,
) (types.ProjectTimelineEventResult, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.ProjectTimelineEventResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	result := types.ProjectTimelineEventResult{}
	switch command.EventType {
	case types.TimelineEventMessagePersisted:
		count, err := projectMessagePersisted(ctx, tx, command)
		if err != nil {
			return types.ProjectTimelineEventResult{}, err
		}
		result.ProjectedInboxCount = count
	case types.TimelineEventMessageEdited:
		count, err := projectMessageChangedForOriginalRecipients(ctx, tx, command, "message edit has no projected original message")
		if err != nil {
			return types.ProjectTimelineEventResult{}, err
		}
		result.ProjectedInboxCount = count
	case types.TimelineEventMessageRevoked:
		count, err := projectMessageChangedForOriginalRecipients(ctx, tx, command, "message revoke has no projected original message")
		if err != nil {
			return types.ProjectTimelineEventResult{}, err
		}
		result.ProjectedInboxCount = count
	case types.TimelineEventMessageDeleted:
		count, err := projectMessageChangedForOriginalRecipients(ctx, tx, command, "message delete has no projected original message")
		if err != nil {
			return types.ProjectTimelineEventResult{}, err
		}
		result.ProjectedInboxCount = count
	case types.TimelineEventConversationMemberJoined,
		types.TimelineEventConversationMemberLeft,
		types.TimelineEventConversationMemberRemoved,
		types.TimelineEventConversationMemberRoleChanged:
		if err := upsertMembershipProjection(ctx, tx, command); err != nil {
			return types.ProjectTimelineEventResult{}, err
		}
		result.MembershipUpdated = true
	case types.TimelineEventConversationMemberOwnerTransferred:
		if err := projectOwnerTransferred(ctx, tx, command); err != nil {
			return types.ProjectTimelineEventResult{}, err
		}
		result.MembershipUpdated = true
	case types.TimelineEventConversationMemberBoundaryCancelled:
		// First phase records no compensating inbox mutation for cancelled boundaries.
	default:
		return types.ProjectTimelineEventResult{}, types.NewInvalidArgument("unsupported timeline event type")
	}
	if command.ShouldCheckpoint() {
		if err := upsertKafkaCheckpoint(ctx, tx, command); err != nil {
			return types.ProjectTimelineEventResult{}, err
		}
		if err := resolveProjectionFailure(ctx, tx, command); err != nil {
			return types.ProjectTimelineEventResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return types.ProjectTimelineEventResult{}, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func projectMessagePersisted(
	ctx context.Context,
	tx pgx.Tx,
	command types.ProjectTimelineEventCommand,
) (int, error) {
	plan, err := domain.BuildFanoutPlan(command.FanoutMode)
	if err != nil {
		return 0, err
	}
	if err := insertTimelineItem(ctx, tx, command); err != nil {
		return 0, err
	}
	if !plan.MaterializesUserInbox {
		if err := insertConversationSignalOutbox(ctx, tx, command); err != nil {
			return 0, err
		}
		return countVisibleTimelineRecipients(ctx, tx, command)
	}
	rows, err := tx.Query(ctx, `
SELECT user_id
FROM delivery_membership_projection
WHERE tenant_id = $1
  AND conversation_id = $2
  AND status = 'ACTIVE'
  AND join_seq <= $3
  AND (leave_seq IS NULL OR leave_seq >= $3)
ORDER BY user_id
`, command.TenantID, command.ConversationID, command.ConversationSeq)
	if err != nil {
		return 0, types.NewDBReadFailed(err.Error())
	}

	userIDs := make([]types.UserID, 0)
	for rows.Next() {
		var userID types.UserID
		if err := rows.Scan(&userID); err != nil {
			rows.Close()
			return 0, types.NewDBReadFailed(err.Error())
		}
		userIDs = append(userIDs, userID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, types.NewDBReadFailed(err.Error())
	}

	if err := insertInboxItems(ctx, tx, command, userIDs); err != nil {
		return 0, err
	}
	projected := 0
	for _, userID := range userIDs {
		if err := insertInboxOutbox(ctx, tx, command, userID); err != nil {
			return 0, err
		}
		projected++
	}
	return projected, nil
}

func projectMessageChangedForOriginalRecipients(
	ctx context.Context,
	tx pgx.Tx,
	command types.ProjectTimelineEventCommand,
	missingMessage string,
) (int, error) {
	plan, err := domain.BuildFanoutPlan(command.FanoutMode)
	if err != nil {
		return 0, err
	}
	if err := insertTimelineItem(ctx, tx, command); err != nil {
		return 0, err
	}
	if !plan.MaterializesUserInbox {
		if err := insertConversationSignalOutbox(ctx, tx, command); err != nil {
			return 0, err
		}
		return countVisibleTimelineRecipients(ctx, tx, command)
	}
	rows, err := tx.Query(ctx, `
SELECT DISTINCT user_id
FROM user_inbox
WHERE tenant_id = $1
  AND conversation_id = $2
  AND message_id = $3
  AND event_type = $4
ORDER BY user_id
`, command.TenantID, command.ConversationID, command.MessageID, types.TimelineEventMessagePersisted)
	if err != nil {
		return 0, types.NewDBReadFailed(err.Error())
	}

	userIDs := make([]types.UserID, 0)
	for rows.Next() {
		var userID types.UserID
		if err := rows.Scan(&userID); err != nil {
			rows.Close()
			return 0, types.NewDBReadFailed(err.Error())
		}
		userIDs = append(userIDs, userID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, types.NewDBReadFailed(err.Error())
	}
	if len(userIDs) == 0 {
		return 0, types.NewProjectionDependencyMissing(missingMessage)
	}

	if err := insertInboxItems(ctx, tx, command, userIDs); err != nil {
		return 0, err
	}
	projected := 0
	for _, userID := range userIDs {
		if err := insertInboxOutbox(ctx, tx, command, userID); err != nil {
			return 0, err
		}
		projected++
	}
	return projected, nil
}

func insertTimelineItem(
	ctx context.Context,
	tx pgx.Tx,
	command types.ProjectTimelineEventCommand,
) error {
	payloadJSON := command.PayloadJSON
	if len(payloadJSON) == 0 {
		payloadJSON = json.RawMessage(`{}`)
	}
	_, err := tx.Exec(ctx, `
INSERT INTO delivery_timeline_items (
    tenant_id,
    conversation_id,
    conversation_seq,
    event_id,
    event_type,
    message_id,
    sender_id,
    payload_json,
    fanout_mode,
    permission_version,
    created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
ON CONFLICT (tenant_id, conversation_id, conversation_seq) DO NOTHING
`, command.TenantID, command.ConversationID, command.ConversationSeq, command.EventID, command.EventType, command.MessageID, command.SenderID, payloadJSON, command.FanoutMode, command.PermissionVersion)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func countVisibleTimelineRecipients(
	ctx context.Context,
	tx pgx.Tx,
	command types.ProjectTimelineEventCommand,
) (int, error) {
	var count int
	err := tx.QueryRow(ctx, `
SELECT COUNT(*)
FROM delivery_membership_projection
WHERE tenant_id = $1
  AND conversation_id = $2
  AND status = 'ACTIVE'
  AND join_seq <= $3
  AND (leave_seq IS NULL OR leave_seq >= $3)
`, command.TenantID, command.ConversationID, command.ConversationSeq).Scan(&count)
	if err != nil {
		return 0, types.NewDBReadFailed(err.Error())
	}
	return count, nil
}

func insertInboxItems(
	ctx context.Context,
	tx pgx.Tx,
	command types.ProjectTimelineEventCommand,
	userIDs []types.UserID,
) error {
	if len(userIDs) == 0 {
		return nil
	}
	payloadJSON := command.PayloadJSON
	if len(payloadJSON) == 0 {
		payloadJSON = json.RawMessage(`{}`)
	}
	recipients := userIDsAsStrings(userIDs)
	_, err := tx.Exec(ctx, `
INSERT INTO user_inbox (
    tenant_id,
    user_id,
    conversation_id,
    conversation_seq,
    event_id,
    event_type,
    message_id,
    sender_id,
    payload_json,
    fanout_mode,
    permission_version,
    created_at
)
SELECT
    $1,
    recipient.user_id,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    $9,
    $10,
    $11,
    now()
FROM unnest($2::text[]) AS recipient(user_id)
ORDER BY recipient.user_id
ON CONFLICT (tenant_id, user_id, event_id) DO NOTHING
`, command.TenantID, recipients, command.ConversationID, command.ConversationSeq, command.EventID, command.EventType, command.MessageID, command.SenderID, payloadJSON, command.FanoutMode, command.PermissionVersion)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func userIDsAsStrings(userIDs []types.UserID) []string {
	recipients := make([]string, 0, len(userIDs))
	for _, userID := range userIDs {
		recipients = append(recipients, string(userID))
	}
	return recipients
}

func upsertMembershipProjection(
	ctx context.Context,
	tx pgx.Tx,
	command types.ProjectTimelineEventCommand,
) error {
	role := command.MemberRole
	if role == "" {
		role = "MEMBER"
	}
	status := command.MemberStatus
	if status == "" {
		status = memberStatusForEvent(command.EventType)
	}
	joinSeq := command.ConversationSeq
	var leaveSeq *int64
	if status != types.DeliveryMemberStatusActive {
		leaveSeq = &command.ConversationSeq
	}
	_, err := tx.Exec(ctx, `
INSERT INTO delivery_membership_projection (
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
ON CONFLICT (tenant_id, conversation_id, user_id) DO UPDATE
SET role = EXCLUDED.role,
    status = EXCLUDED.status,
    join_seq = CASE
        WHEN $11 = 'JOIN' THEN EXCLUDED.join_seq
        ELSE delivery_membership_projection.join_seq
    END,
    leave_seq = EXCLUDED.leave_seq,
    member_version = EXCLUDED.member_version,
    permission_version = EXCLUDED.permission_version,
    updated_by_event_id = EXCLUDED.updated_by_event_id,
    updated_at = now()
WHERE delivery_membership_projection.member_version <= EXCLUDED.member_version
`, command.TenantID, command.ConversationID, command.MemberUserID, role, status, joinSeq, leaveSeq, command.MemberVersion, command.PermissionVersion, command.EventID, memberChangeKind(command.EventType))
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func projectOwnerTransferred(ctx context.Context, tx pgx.Tx, command types.ProjectTimelineEventCommand) error {
	previousOwner := command
	previousOwner.MemberUserID = command.PreviousOwnerUserID
	previousOwner.MemberRole = command.PreviousOwnerNewRole
	previousOwner.MemberStatus = command.PreviousOwnerStatus
	if err := upsertMembershipProjection(ctx, tx, previousOwner); err != nil {
		return err
	}

	newOwner := command
	newOwner.MemberUserID = command.NewOwnerUserID
	newOwner.MemberRole = command.NewOwnerNewRole
	newOwner.MemberStatus = command.NewOwnerStatus
	if err := upsertMembershipProjection(ctx, tx, newOwner); err != nil {
		return err
	}
	return nil
}

func memberChangeKind(eventType string) string {
	switch eventType {
	case types.TimelineEventConversationMemberJoined:
		return "JOIN"
	case types.TimelineEventConversationMemberLeft:
		return "LEAVE"
	case types.TimelineEventConversationMemberRemoved:
		return "REMOVE"
	case types.TimelineEventConversationMemberRoleChanged:
		return "ROLE_CHANGED"
	case types.TimelineEventConversationMemberOwnerTransferred:
		return "OWNER_TRANSFER"
	default:
		return ""
	}
}

func memberStatusForEvent(eventType string) string {
	switch eventType {
	case types.TimelineEventConversationMemberLeft:
		return types.DeliveryMemberStatusLeft
	case types.TimelineEventConversationMemberRemoved:
		return types.DeliveryMemberStatusBanned
	default:
		return types.DeliveryMemberStatusActive
	}
}

func insertInboxOutbox(
	ctx context.Context,
	tx pgx.Tx,
	command types.ProjectTimelineEventCommand,
	userID types.UserID,
) error {
	payload := map[string]any{
		"tenant_id":         command.TenantID,
		"user_id":           userID,
		"conversation_id":   command.ConversationID,
		"conversation_seq":  command.ConversationSeq,
		"source_event_id":   command.EventID,
		"source_event_type": command.EventType,
		"message_id":        command.MessageID,
		"sender_id":         command.SenderID,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	eventID := inboxEventID(command, userID)
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
) VALUES ($1, $2, $3, $4, 'delivery.inbox_item.created.v1', '1.0.0', $5, 1, $6, $7, 'delivery-service', $8, $9, 'PENDING', now(), now(), now())
ON CONFLICT (event_id) DO NOTHING
`, eventID, command.TenantID, command.ConversationID, command.ConversationSeq, partitionKeyFor(command.TenantID, command.ConversationID), command.CorrelationID, command.EventID, command.TraceID, payloadBytes)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertConversationSignalOutbox(
	ctx context.Context,
	tx pgx.Tx,
	command types.ProjectTimelineEventCommand,
) error {
	payload := map[string]any{
		"tenant_id":         command.TenantID,
		"conversation_id":   command.ConversationID,
		"conversation_seq":  command.ConversationSeq,
		"source_event_id":   command.EventID,
		"source_event_type": command.EventType,
		"message_id":        command.MessageID,
		"sender_id":         command.SenderID,
		"fanout_mode":       command.FanoutMode,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	eventID := conversationSignalEventID(command)
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
) VALUES ($1, $2, $3, $4, 'delivery.conversation.signal.v1', '1.0.0', $5, 1, $6, $7, 'delivery-service', $8, $9, 'PENDING', now(), now(), now())
ON CONFLICT (event_id) DO NOTHING
`, eventID, command.TenantID, command.ConversationID, command.ConversationSeq, partitionKeyFor(command.TenantID, command.ConversationID), command.CorrelationID, command.EventID, command.TraceID, payloadBytes)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func upsertKafkaCheckpoint(
	ctx context.Context,
	tx pgx.Tx,
	command types.ProjectTimelineEventCommand,
) error {
	_, err := tx.Exec(ctx, `
INSERT INTO delivery_kafka_checkpoints (
    consumer_group,
    topic,
    partition_id,
    offset_value,
    updated_at
) VALUES ($1, $2, $3, $4, now())
ON CONFLICT (consumer_group, topic, partition_id) DO UPDATE
SET offset_value = GREATEST(delivery_kafka_checkpoints.offset_value, EXCLUDED.offset_value),
    updated_at = now()
`, command.ConsumerGroup, command.Topic, command.PartitionID, command.OffsetValue)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func resolveProjectionFailure(
	ctx context.Context,
	tx pgx.Tx,
	command types.ProjectTimelineEventCommand,
) error {
	if !command.ShouldCheckpoint() || command.OffsetValue <= 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `
UPDATE delivery_projection_failures
SET resolved_at = now(),
    resolved_checkpoint_offset = $5
WHERE consumer_group = $1
  AND topic = $2
  AND partition_id = $3
  AND offset_value = $4
  AND resolved_at IS NULL
`, command.ConsumerGroup, command.Topic, command.PartitionID, command.OffsetValue-1, command.OffsetValue)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func (repository *Repository) PullInbox(
	ctx context.Context,
	command types.PullInboxCommand,
	fetchLimit int,
) ([]types.InboxItem, error) {
	rows, err := repository.pool.Query(ctx, `
WITH materialized AS (
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
      AND hidden_at IS NULL
),
timeline_visible AS (
    SELECT
        item.conversation_id,
        item.conversation_seq,
        item.event_id,
        item.event_type,
        item.message_id,
        item.sender_id,
        item.payload_json,
        item.created_at
    FROM delivery_timeline_items item
    JOIN delivery_membership_projection member
      ON member.tenant_id = item.tenant_id
     AND member.conversation_id = item.conversation_id
     AND member.user_id = $2
     AND member.status = 'ACTIVE'
     AND member.join_seq <= item.conversation_seq
     AND (member.leave_seq IS NULL OR member.leave_seq >= item.conversation_seq)
    LEFT JOIN user_inbox inbox
      ON inbox.tenant_id = item.tenant_id
     AND inbox.user_id = $2
     AND inbox.conversation_id = item.conversation_id
     AND inbox.conversation_seq = item.conversation_seq
    LEFT JOIN delivery_user_hidden_timeline_items hidden
      ON hidden.tenant_id = item.tenant_id
     AND hidden.user_id = $2
     AND hidden.conversation_id = item.conversation_id
     AND hidden.conversation_seq = item.conversation_seq
    WHERE item.tenant_id = $1
      AND item.conversation_id = $3
      AND item.conversation_seq > $4
      AND item.fanout_mode IN ('READ_FANOUT', 'BROADCAST_SIGNAL')
      AND inbox.tenant_id IS NULL
      AND hidden.tenant_id IS NULL
)
SELECT
    conversation_id,
    conversation_seq,
    event_id,
    event_type,
    message_id,
    sender_id,
    payload_json,
    created_at
FROM materialized
UNION ALL
SELECT
    conversation_id,
    conversation_seq,
    event_id,
    event_type,
    message_id,
    sender_id,
    payload_json,
    created_at
FROM timeline_visible
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

func (repository *Repository) HideInboxItem(
	ctx context.Context,
	command types.HideInboxItemCommand,
) (types.HideInboxItemResult, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.HideInboxItemResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	alreadyHidden, messageID, found, err := hideMaterializedInboxItem(ctx, tx, command)
	if err != nil {
		return types.HideInboxItemResult{}, err
	}
	if !found {
		alreadyHidden, messageID, err = hideTimelineItem(ctx, tx, command)
		if err != nil {
			return types.HideInboxItemResult{}, err
		}
	}
	if !alreadyHidden {
		if err := insertHideInboxOutbox(ctx, tx, command, messageID); err != nil {
			return types.HideInboxItemResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return types.HideInboxItemResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.HideInboxItemResult{
		TenantID:        command.AuthContext.TenantID,
		UserID:          command.AuthContext.UserID,
		ConversationID:  command.ConversationID,
		ConversationSeq: command.ConversationSeq,
		AlreadyHidden:   alreadyHidden,
	}, nil
}

func hideMaterializedInboxItem(
	ctx context.Context,
	tx pgx.Tx,
	command types.HideInboxItemCommand,
) (bool, string, bool, error) {
	var alreadyHidden bool
	var messageID string
	err := tx.QueryRow(ctx, `
WITH target AS (
    SELECT hidden_at, message_id
    FROM user_inbox
    WHERE tenant_id = $1
      AND user_id = $2
      AND conversation_id = $3
      AND conversation_seq = $4
    FOR UPDATE
), updated AS (
    UPDATE user_inbox item
    SET hidden_at = COALESCE(item.hidden_at, now()),
        hidden_by_device_id = CASE
            WHEN item.hidden_at IS NULL THEN $5
            ELSE item.hidden_by_device_id
        END,
        hide_reason = CASE
            WHEN item.hidden_at IS NULL THEN $6
            ELSE item.hide_reason
        END
    FROM target
    WHERE item.tenant_id = $1
      AND item.user_id = $2
      AND item.conversation_id = $3
      AND item.conversation_seq = $4
    RETURNING target.hidden_at IS NOT NULL, target.message_id
)
SELECT already_hidden, message_id
FROM updated AS result(already_hidden, message_id)
`,
		command.AuthContext.TenantID,
		command.AuthContext.UserID,
		command.ConversationID,
		command.ConversationSeq,
		command.AuthContext.DeviceID,
		command.Reason,
	).Scan(&alreadyHidden, &messageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, "", false, nil
	}
	if err != nil {
		return false, "", false, types.NewDBWriteFailed(err.Error())
	}
	return alreadyHidden, messageID, true, nil
}

func hideTimelineItem(
	ctx context.Context,
	tx pgx.Tx,
	command types.HideInboxItemCommand,
) (bool, string, error) {
	var messageID string
	err := tx.QueryRow(ctx, `
SELECT item.message_id
FROM delivery_timeline_items item
JOIN delivery_membership_projection member
  ON member.tenant_id = item.tenant_id
 AND member.conversation_id = item.conversation_id
 AND member.user_id = $2
 AND member.status = 'ACTIVE'
 AND member.join_seq <= item.conversation_seq
 AND (member.leave_seq IS NULL OR member.leave_seq >= item.conversation_seq)
WHERE item.tenant_id = $1
  AND item.conversation_id = $3
  AND item.conversation_seq = $4
  AND item.fanout_mode IN ('READ_FANOUT', 'BROADCAST_SIGNAL')
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID, command.ConversationSeq).Scan(&messageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, "", types.NewInboxItemNotFound("inbox item not found")
	}
	if err != nil {
		return false, "", types.NewDBReadFailed(err.Error())
	}
	tag, err := tx.Exec(ctx, `
INSERT INTO delivery_user_hidden_timeline_items (
    tenant_id,
    user_id,
    device_id,
    conversation_id,
    conversation_seq,
    message_id,
    reason,
    hidden_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, now())
ON CONFLICT (tenant_id, user_id, conversation_id, conversation_seq) DO NOTHING
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.AuthContext.DeviceID, command.ConversationID, command.ConversationSeq, messageID, command.Reason)
	if err != nil {
		return false, "", types.NewDBWriteFailed(err.Error())
	}
	return tag.RowsAffected() == 0, messageID, nil
}

func insertHideInboxOutbox(
	ctx context.Context,
	tx pgx.Tx,
	command types.HideInboxItemCommand,
	messageID string,
) error {
	payload := map[string]any{
		"tenant_id":        command.AuthContext.TenantID,
		"user_id":          command.AuthContext.UserID,
		"device_id":        command.AuthContext.DeviceID,
		"conversation_id":  command.ConversationID,
		"conversation_seq": command.ConversationSeq,
		"message_id":       messageID,
		"trace_id":         command.AuthContext.TraceID,
		"correlation_id":   command.AuthContext.RequestID,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	eventID := hideEventID(command)
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
) VALUES ($1, $2, $3, $4, 'delivery.inbox_item.hidden.v1', '1.0.0', $5, 1, $6, $7, 'delivery-service', $8, $9, 'PENDING', now(), now(), now())
ON CONFLICT (event_id) DO NOTHING
`,
		eventID,
		command.AuthContext.TenantID,
		command.ConversationID,
		command.ConversationSeq,
		partitionKeyFor(command.AuthContext.TenantID, command.ConversationID),
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

	if err := lockAckKey(ctx, tx, command); err != nil {
		return types.AckDeliveryResult{}, err
	}
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
SELECT GREATEST(
    (
        SELECT COALESCE(MAX(conversation_seq), 0)
        FROM user_inbox
        WHERE tenant_id = $1
          AND user_id = $2
          AND conversation_id = $3
    ),
    (
        SELECT COALESCE(MAX(item.conversation_seq), 0)
        FROM delivery_timeline_items item
        JOIN delivery_membership_projection member
          ON member.tenant_id = item.tenant_id
         AND member.conversation_id = item.conversation_id
         AND member.user_id = $2
         AND member.status = 'ACTIVE'
         AND member.join_seq <= item.conversation_seq
         AND (member.leave_seq IS NULL OR member.leave_seq >= item.conversation_seq)
        LEFT JOIN delivery_user_hidden_timeline_items hidden
          ON hidden.tenant_id = item.tenant_id
         AND hidden.user_id = $2
         AND hidden.conversation_id = item.conversation_id
         AND hidden.conversation_seq = item.conversation_seq
        WHERE item.tenant_id = $1
          AND item.conversation_id = $3
          AND item.fanout_mode IN ('READ_FANOUT', 'BROADCAST_SIGNAL')
          AND hidden.tenant_id IS NULL
    )
)
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID).Scan(&maxSeq)
	if err != nil {
		return 0, types.NewDBReadFailed(err.Error())
	}
	return maxSeq, nil
}

func lockAckKey(
	ctx context.Context,
	tx pgx.Tx,
	command types.AckDeliveryCommand,
) error {
	key := fmt.Sprintf(
		"%s\x1f%s\x1f%s\x1f%s",
		command.AuthContext.TenantID,
		command.AuthContext.UserID,
		command.AuthContext.DeviceID,
		command.ConversationID,
	)
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
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
		partitionKeyFor(command.AuthContext.TenantID, command.ConversationID),
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

func partitionKeyFor(tenantID types.TenantID, conversationID types.ConversationID) string {
	return fmt.Sprintf("%s:%s", tenantID, conversationID)
}

func inboxEventID(command types.ProjectTimelineEventCommand, userID types.UserID) string {
	raw := fmt.Sprintf(
		"%s\x1f%s\x1f%s\x1f%d\x1f%s",
		command.TenantID,
		userID,
		command.ConversationID,
		command.ConversationSeq,
		command.EventID,
	)
	sum := sha256.Sum256([]byte(raw))
	return "evt_delivery_inbox_" + hex.EncodeToString(sum[:16])
}

func conversationSignalEventID(command types.ProjectTimelineEventCommand) string {
	raw := fmt.Sprintf(
		"%s\x1f%s\x1f%d\x1f%s",
		command.TenantID,
		command.ConversationID,
		command.ConversationSeq,
		command.EventID,
	)
	sum := sha256.Sum256([]byte(raw))
	return "evt_delivery_signal_" + hex.EncodeToString(sum[:16])
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

func hideEventID(command types.HideInboxItemCommand) string {
	raw := fmt.Sprintf(
		"%s\x1f%s\x1f%s\x1f%s\x1f%d",
		command.AuthContext.TenantID,
		command.AuthContext.UserID,
		command.AuthContext.DeviceID,
		command.ConversationID,
		command.ConversationSeq,
	)
	sum := sha256.Sum256([]byte(raw))
	return "evt_delivery_hide_" + hex.EncodeToString(sum[:16])
}
