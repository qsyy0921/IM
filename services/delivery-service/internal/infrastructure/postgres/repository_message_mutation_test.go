package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

func TestRepositoryProjectMessageRevokedOnlyTargetsOriginalVisibleUsersIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	repository := NewRepository(pool)

	_, err := repository.ProjectTimelineEvent(ctx, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-delivery",
		EventID:           "member-joined-user-1",
		EventType:         types.TimelineEventConversationMemberJoined,
		ConversationID:    "conv-delivery",
		ConversationSeq:   1,
		MemberUserID:      "user-1",
		MemberRole:        "MEMBER",
		MemberStatus:      types.DeliveryMemberStatusActive,
		MemberVersion:     1,
		PermissionVersion: 1,
	})
	if err != nil {
		t.Fatalf("project user-1 join: %v", err)
	}
	result, err := repository.ProjectTimelineEvent(ctx, messageEvent("message-visible-before-user-2", "msg-visible-before-user-2", 2))
	if err != nil {
		t.Fatalf("project message before user-2 join: %v", err)
	}
	if result.ProjectedInboxCount != 1 {
		t.Fatalf("expected original message to target one user, got %d", result.ProjectedInboxCount)
	}
	_, err = repository.ProjectTimelineEvent(ctx, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-delivery",
		EventID:           "member-joined-user-2",
		EventType:         types.TimelineEventConversationMemberJoined,
		ConversationID:    "conv-delivery",
		ConversationSeq:   3,
		MemberUserID:      "user-2",
		MemberRole:        "MEMBER",
		MemberStatus:      types.DeliveryMemberStatusActive,
		MemberVersion:     2,
		PermissionVersion: 2,
	})
	if err != nil {
		t.Fatalf("project user-2 join: %v", err)
	}
	result, err = repository.ProjectTimelineEvent(ctx, revokeEvent("message-revoked-after-user-2-join", "msg-visible-before-user-2", 4))
	if err != nil {
		t.Fatalf("project revoke after user-2 join: %v", err)
	}
	if result.ProjectedInboxCount != 1 {
		t.Fatalf("expected revoke to target only original visible user, got %d", result.ProjectedInboxCount)
	}

	var user1Revokes int
	err = pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM user_inbox
WHERE tenant_id = 'tenant-delivery'
  AND conversation_id = 'conv-delivery'
  AND user_id = 'user-1'
  AND message_id = 'msg-visible-before-user-2'
  AND event_type = $1
`, types.TimelineEventMessageRevoked).Scan(&user1Revokes)
	if err != nil {
		t.Fatalf("count user-1 revokes: %v", err)
	}
	if user1Revokes != 1 {
		t.Fatalf("expected user-1 revoke tombstone, got %d", user1Revokes)
	}

	var user2Rows int
	err = pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM user_inbox
WHERE tenant_id = 'tenant-delivery'
  AND conversation_id = 'conv-delivery'
  AND user_id = 'user-2'
  AND message_id = 'msg-visible-before-user-2'
`).Scan(&user2Rows)
	if err != nil {
		t.Fatalf("count user-2 rows: %v", err)
	}
	if user2Rows != 0 {
		t.Fatalf("expected user-2 not to see original or revoke tombstone, got %d rows", user2Rows)
	}
}

func TestRepositoryProjectMessageEditedOnlyTargetsOriginalVisibleUsersIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	repository := NewRepository(pool)

	_, err := repository.ProjectTimelineEvent(ctx, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-delivery",
		EventID:           "member-joined-edit-user-1",
		EventType:         types.TimelineEventConversationMemberJoined,
		ConversationID:    "conv-delivery",
		ConversationSeq:   1,
		MemberUserID:      "user-1",
		MemberRole:        "MEMBER",
		MemberStatus:      types.DeliveryMemberStatusActive,
		MemberVersion:     1,
		PermissionVersion: 1,
	})
	if err != nil {
		t.Fatalf("project user-1 join: %v", err)
	}
	result, err := repository.ProjectTimelineEvent(ctx, messageEvent("message-edit-visible-before-user-2", "msg-edit-visible-before-user-2", 2))
	if err != nil {
		t.Fatalf("project message before user-2 join: %v", err)
	}
	if result.ProjectedInboxCount != 1 {
		t.Fatalf("expected original message to target one user, got %d", result.ProjectedInboxCount)
	}
	_, err = repository.ProjectTimelineEvent(ctx, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-delivery",
		EventID:           "member-joined-edit-user-2",
		EventType:         types.TimelineEventConversationMemberJoined,
		ConversationID:    "conv-delivery",
		ConversationSeq:   3,
		MemberUserID:      "user-2",
		MemberRole:        "MEMBER",
		MemberStatus:      types.DeliveryMemberStatusActive,
		MemberVersion:     2,
		PermissionVersion: 2,
	})
	if err != nil {
		t.Fatalf("project user-2 join: %v", err)
	}
	result, err = repository.ProjectTimelineEvent(ctx, editEvent("message-edited-after-user-2-join", "msg-edit-visible-before-user-2", 4))
	if err != nil {
		t.Fatalf("project edit after user-2 join: %v", err)
	}
	if result.ProjectedInboxCount != 1 {
		t.Fatalf("expected edit to target only original visible user, got %d", result.ProjectedInboxCount)
	}

	var user1Edits int
	err = pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM user_inbox
WHERE tenant_id = 'tenant-delivery'
  AND conversation_id = 'conv-delivery'
  AND user_id = 'user-1'
  AND message_id = 'msg-edit-visible-before-user-2'
  AND event_type = $1
`, types.TimelineEventMessageEdited).Scan(&user1Edits)
	if err != nil {
		t.Fatalf("count user-1 edits: %v", err)
	}
	if user1Edits != 1 {
		t.Fatalf("expected user-1 edit event, got %d", user1Edits)
	}

	var user2Rows int
	err = pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM user_inbox
WHERE tenant_id = 'tenant-delivery'
  AND conversation_id = 'conv-delivery'
  AND user_id = 'user-2'
  AND message_id = 'msg-edit-visible-before-user-2'
`).Scan(&user2Rows)
	if err != nil {
		t.Fatalf("count user-2 rows: %v", err)
	}
	if user2Rows != 0 {
		t.Fatalf("expected user-2 not to see original or edit delta, got %d rows", user2Rows)
	}
}

func TestRepositoryProjectMessageEditedFailsClosedWhenOriginalMissingIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	repository := NewRepository(pool)

	_, err := repository.ProjectTimelineEvent(ctx, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-delivery",
		EventID:           "member-joined-before-edit",
		EventType:         types.TimelineEventConversationMemberJoined,
		ConversationID:    "conv-delivery",
		ConversationSeq:   1,
		MemberUserID:      "user-1",
		MemberRole:        "MEMBER",
		MemberStatus:      types.DeliveryMemberStatusActive,
		MemberVersion:     1,
		PermissionVersion: 1,
	})
	if err != nil {
		t.Fatalf("project member join: %v", err)
	}
	edit := editEvent("message-edited-without-original", "msg-missing-original-edit", 2)
	edit.ConsumerGroup = "delivery-test"
	edit.Topic = "conversation.timeline.events"
	edit.PartitionID = 1
	edit.OffsetValue = 9
	_, err = repository.ProjectTimelineEvent(ctx, edit)
	if !errors.Is(err, types.ErrProjectionDependency) {
		t.Fatalf("expected projection dependency error, got %v", err)
	}

	var inboxRows int
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM user_inbox
WHERE tenant_id = 'tenant-delivery'
  AND conversation_id = 'conv-delivery'
  AND message_id = 'msg-missing-original-edit'
`).Scan(&inboxRows); err != nil {
		t.Fatalf("count inbox rows: %v", err)
	}
	if inboxRows != 0 {
		t.Fatalf("expected no edit without original message, got %d rows", inboxRows)
	}

	var checkpointRows int
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM delivery_kafka_checkpoints
WHERE consumer_group = 'delivery-test'
`).Scan(&checkpointRows); err != nil {
		t.Fatalf("count checkpoint rows: %v", err)
	}
	if checkpointRows != 0 {
		t.Fatalf("expected no checkpoint on fail-closed edit, got %d", checkpointRows)
	}
}

func TestRepositoryProjectMessageDeletedOnlyTargetsOriginalVisibleUsersIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	repository := NewRepository(pool)

	_, err := repository.ProjectTimelineEvent(ctx, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-delivery",
		EventID:           "member-joined-delete-user-1",
		EventType:         types.TimelineEventConversationMemberJoined,
		ConversationID:    "conv-delivery",
		ConversationSeq:   1,
		MemberUserID:      "user-1",
		MemberRole:        "MEMBER",
		MemberStatus:      types.DeliveryMemberStatusActive,
		MemberVersion:     1,
		PermissionVersion: 1,
	})
	if err != nil {
		t.Fatalf("project user-1 join: %v", err)
	}
	result, err := repository.ProjectTimelineEvent(ctx, messageEvent("message-delete-visible-before-user-2", "msg-delete-visible-before-user-2", 2))
	if err != nil {
		t.Fatalf("project message before user-2 join: %v", err)
	}
	if result.ProjectedInboxCount != 1 {
		t.Fatalf("expected original message to target one user, got %d", result.ProjectedInboxCount)
	}
	_, err = repository.ProjectTimelineEvent(ctx, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-delivery",
		EventID:           "member-joined-delete-user-2",
		EventType:         types.TimelineEventConversationMemberJoined,
		ConversationID:    "conv-delivery",
		ConversationSeq:   3,
		MemberUserID:      "user-2",
		MemberRole:        "MEMBER",
		MemberStatus:      types.DeliveryMemberStatusActive,
		MemberVersion:     2,
		PermissionVersion: 2,
	})
	if err != nil {
		t.Fatalf("project user-2 join: %v", err)
	}
	result, err = repository.ProjectTimelineEvent(ctx, deleteEvent("message-deleted-after-user-2-join", "msg-delete-visible-before-user-2", 4))
	if err != nil {
		t.Fatalf("project delete after user-2 join: %v", err)
	}
	if result.ProjectedInboxCount != 1 {
		t.Fatalf("expected delete to target only original visible user, got %d", result.ProjectedInboxCount)
	}

	var user1Deletes int
	err = pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM user_inbox
WHERE tenant_id = 'tenant-delivery'
  AND conversation_id = 'conv-delivery'
  AND user_id = 'user-1'
  AND message_id = 'msg-delete-visible-before-user-2'
  AND event_type = $1
`, types.TimelineEventMessageDeleted).Scan(&user1Deletes)
	if err != nil {
		t.Fatalf("count user-1 deletes: %v", err)
	}
	if user1Deletes != 1 {
		t.Fatalf("expected user-1 delete event, got %d", user1Deletes)
	}

	var user2Rows int
	err = pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM user_inbox
WHERE tenant_id = 'tenant-delivery'
  AND conversation_id = 'conv-delivery'
  AND user_id = 'user-2'
  AND message_id = 'msg-delete-visible-before-user-2'
`).Scan(&user2Rows)
	if err != nil {
		t.Fatalf("count user-2 rows: %v", err)
	}
	if user2Rows != 0 {
		t.Fatalf("expected user-2 not to see original or delete delta, got %d rows", user2Rows)
	}
}

func TestRepositoryProjectMessageDeletedFailsClosedWhenOriginalMissingIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	repository := NewRepository(pool)

	_, err := repository.ProjectTimelineEvent(ctx, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-delivery",
		EventID:           "member-joined-before-delete",
		EventType:         types.TimelineEventConversationMemberJoined,
		ConversationID:    "conv-delivery",
		ConversationSeq:   1,
		MemberUserID:      "user-1",
		MemberRole:        "MEMBER",
		MemberStatus:      types.DeliveryMemberStatusActive,
		MemberVersion:     1,
		PermissionVersion: 1,
	})
	if err != nil {
		t.Fatalf("project member join: %v", err)
	}
	deleted := deleteEvent("message-deleted-without-original", "msg-missing-original-delete", 2)
	deleted.ConsumerGroup = "delivery-test"
	deleted.Topic = "conversation.timeline.events"
	deleted.PartitionID = 1
	deleted.OffsetValue = 9
	_, err = repository.ProjectTimelineEvent(ctx, deleted)
	if !errors.Is(err, types.ErrProjectionDependency) {
		t.Fatalf("expected projection dependency error, got %v", err)
	}

	var inboxRows int
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM user_inbox
WHERE tenant_id = 'tenant-delivery'
  AND conversation_id = 'conv-delivery'
  AND message_id = 'msg-missing-original-delete'
`).Scan(&inboxRows); err != nil {
		t.Fatalf("count inbox rows: %v", err)
	}
	if inboxRows != 0 {
		t.Fatalf("expected no delete without original message, got %d rows", inboxRows)
	}

	var checkpointRows int
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM delivery_kafka_checkpoints
WHERE consumer_group = 'delivery-test'
`).Scan(&checkpointRows); err != nil {
		t.Fatalf("count checkpoint rows: %v", err)
	}
	if checkpointRows != 0 {
		t.Fatalf("expected no checkpoint on fail-closed delete, got %d", checkpointRows)
	}
}

func TestRepositoryProjectMessageRevokedFailsClosedWhenOriginalMissingIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	repository := NewRepository(pool)

	_, err := repository.ProjectTimelineEvent(ctx, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-delivery",
		EventID:           "member-joined-before-revoke",
		EventType:         types.TimelineEventConversationMemberJoined,
		ConversationID:    "conv-delivery",
		ConversationSeq:   1,
		MemberUserID:      "user-1",
		MemberRole:        "MEMBER",
		MemberStatus:      types.DeliveryMemberStatusActive,
		MemberVersion:     1,
		PermissionVersion: 1,
	})
	if err != nil {
		t.Fatalf("project member join: %v", err)
	}
	revoke := revokeEvent("message-revoked-without-original", "msg-missing-original", 2)
	revoke.ConsumerGroup = "delivery-test"
	revoke.Topic = "conversation.timeline.events"
	revoke.PartitionID = 1
	revoke.OffsetValue = 9
	_, err = repository.ProjectTimelineEvent(ctx, revoke)
	if !errors.Is(err, types.ErrProjectionDependency) {
		t.Fatalf("expected projection dependency error, got %v", err)
	}

	var inboxRows int
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM user_inbox
WHERE tenant_id = 'tenant-delivery'
  AND conversation_id = 'conv-delivery'
  AND message_id = 'msg-missing-original'
`).Scan(&inboxRows); err != nil {
		t.Fatalf("count inbox rows: %v", err)
	}
	if inboxRows != 0 {
		t.Fatalf("expected no tombstone without original message, got %d rows", inboxRows)
	}

	var checkpointRows int
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM delivery_kafka_checkpoints
WHERE consumer_group = 'delivery-test'
`).Scan(&checkpointRows); err != nil {
		t.Fatalf("count checkpoint rows: %v", err)
	}
	if checkpointRows != 0 {
		t.Fatalf("expected no checkpoint on fail-closed revoke, got %d", checkpointRows)
	}
}
