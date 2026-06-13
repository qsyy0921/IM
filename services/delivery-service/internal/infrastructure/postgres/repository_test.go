package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

func TestRepositoryProjectTimelineEventIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	repository := NewRepository(pool)

	_, err := repository.ProjectTimelineEvent(ctx, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-delivery",
		EventID:           "member-joined-1",
		EventType:         types.TimelineEventConversationMemberJoined,
		ConversationID:    "conv-delivery",
		ConversationSeq:   1,
		MemberUserID:      "user-1",
		MemberRole:        "MEMBER",
		MemberStatus:      types.DeliveryMemberStatusActive,
		MemberVersion:     1,
		PermissionVersion: 1,
		ConsumerGroup:     "delivery-test",
		Topic:             "conversation.timeline.events",
		PartitionID:       2,
		OffsetValue:       11,
	})
	if err != nil {
		t.Fatalf("project join: %v", err)
	}
	result, err := repository.ProjectTimelineEvent(ctx, messageEvent("message-1", "msg-1", 2))
	if err != nil {
		t.Fatalf("project first message: %v", err)
	}
	if result.ProjectedInboxCount != 1 {
		t.Fatalf("expected 1 projected inbox item, got %d", result.ProjectedInboxCount)
	}
	_, err = repository.ProjectTimelineEvent(ctx, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-delivery",
		EventID:           "member-left-1",
		EventType:         types.TimelineEventConversationMemberLeft,
		ConversationID:    "conv-delivery",
		ConversationSeq:   3,
		MemberUserID:      "user-1",
		MemberRole:        "MEMBER",
		MemberStatus:      types.DeliveryMemberStatusLeft,
		MemberVersion:     2,
		PermissionVersion: 2,
	})
	if err != nil {
		t.Fatalf("project leave: %v", err)
	}
	result, err = repository.ProjectTimelineEvent(ctx, messageEvent("message-2", "msg-2", 4))
	if err != nil {
		t.Fatalf("project message while left: %v", err)
	}
	if result.ProjectedInboxCount != 0 {
		t.Fatalf("expected no inbox while left, got %d", result.ProjectedInboxCount)
	}
	_, err = repository.ProjectTimelineEvent(ctx, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-delivery",
		EventID:           "member-joined-2",
		EventType:         types.TimelineEventConversationMemberJoined,
		ConversationID:    "conv-delivery",
		ConversationSeq:   5,
		MemberUserID:      "user-1",
		MemberRole:        "MEMBER",
		MemberStatus:      types.DeliveryMemberStatusActive,
		MemberVersion:     3,
		PermissionVersion: 3,
	})
	if err != nil {
		t.Fatalf("project rejoin: %v", err)
	}
	result, err = repository.ProjectTimelineEvent(ctx, messageEvent("message-3", "msg-3", 6))
	if err != nil {
		t.Fatalf("project message after rejoin: %v", err)
	}
	if result.ProjectedInboxCount != 1 {
		t.Fatalf("expected inbox after rejoin, got %d", result.ProjectedInboxCount)
	}
	result, err = repository.ProjectTimelineEvent(ctx, revokeEvent("message-revoked-1", "msg-3", 7))
	if err != nil {
		t.Fatalf("project revoke after rejoin: %v", err)
	}
	if result.ProjectedInboxCount != 1 {
		t.Fatalf("expected revoke tombstone inbox after rejoin, got %d", result.ProjectedInboxCount)
	}

	var inboxCount int
	err = pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM user_inbox
WHERE tenant_id = 'tenant-delivery'
  AND user_id = 'user-1'
  AND conversation_id = 'conv-delivery'
`).Scan(&inboxCount)
	if err != nil {
		t.Fatalf("count inbox: %v", err)
	}
	if inboxCount != 3 {
		t.Fatalf("expected 3 inbox rows, got %d", inboxCount)
	}
	var revokeEventType string
	var revokePayload string
	err = pool.QueryRow(ctx, `
SELECT event_type, payload_json::text
FROM user_inbox
WHERE tenant_id = 'tenant-delivery'
  AND user_id = 'user-1'
  AND conversation_id = 'conv-delivery'
  AND conversation_seq = 7
`).Scan(&revokeEventType, &revokePayload)
	if err != nil {
		t.Fatalf("read revoke inbox: %v", err)
	}
	if revokeEventType != types.TimelineEventMessageRevoked || !strings.Contains(revokePayload, "revoked_by") {
		t.Fatalf("unexpected revoke inbox event_type=%s payload=%s", revokeEventType, revokePayload)
	}
	var joinSeq int64
	var leaveSeq *int64
	var status string
	err = pool.QueryRow(ctx, `
SELECT join_seq, leave_seq, status
FROM delivery_membership_projection
WHERE tenant_id = 'tenant-delivery'
  AND conversation_id = 'conv-delivery'
  AND user_id = 'user-1'
`).Scan(&joinSeq, &leaveSeq, &status)
	if err != nil {
		t.Fatalf("read membership projection: %v", err)
	}
	if joinSeq != 5 || leaveSeq != nil || status != types.DeliveryMemberStatusActive {
		t.Fatalf("unexpected membership projection join_seq=%d leave_seq=%v status=%s", joinSeq, leaveSeq, status)
	}
	var checkpoint int64
	err = pool.QueryRow(ctx, `
SELECT offset_value
FROM delivery_kafka_checkpoints
WHERE consumer_group = 'delivery-test'
  AND topic = 'conversation.timeline.events'
  AND partition_id = 2
`).Scan(&checkpoint)
	if err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}
	if checkpoint != 11 {
		t.Fatalf("expected checkpoint 11, got %d", checkpoint)
	}

	result, err = repository.ProjectTimelineEvent(ctx, messageEvent("message-3", "msg-3", 6))
	if err != nil {
		t.Fatalf("replay message: %v", err)
	}
	if result.ProjectedInboxCount != 1 {
		t.Fatalf("replay still sees one target, got %d", result.ProjectedInboxCount)
	}
	err = pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM user_inbox
WHERE tenant_id = 'tenant-delivery'
  AND user_id = 'user-1'
  AND event_id = 'message-3'
`).Scan(&inboxCount)
	if err != nil {
		t.Fatalf("count replay inbox: %v", err)
	}
	if inboxCount != 1 {
		t.Fatalf("expected replay to keep one inbox row, got %d", inboxCount)
	}
}

func TestRepositoryProjectOwnerTransferredIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	repository := NewRepository(pool)

	_, err := repository.ProjectTimelineEvent(ctx, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-owner-transfer",
		EventID:           "owner-joined-1",
		EventType:         types.TimelineEventConversationMemberJoined,
		ConversationID:    "conv-owner-transfer",
		ConversationSeq:   1,
		MemberUserID:      "owner-1",
		MemberRole:        "OWNER",
		MemberStatus:      types.DeliveryMemberStatusActive,
		MemberVersion:     1,
		PermissionVersion: 1,
	})
	if err != nil {
		t.Fatalf("project owner join: %v", err)
	}
	_, err = repository.ProjectTimelineEvent(ctx, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-owner-transfer",
		EventID:           "member-joined-1",
		EventType:         types.TimelineEventConversationMemberJoined,
		ConversationID:    "conv-owner-transfer",
		ConversationSeq:   2,
		MemberUserID:      "user-2",
		MemberRole:        "MEMBER",
		MemberStatus:      types.DeliveryMemberStatusActive,
		MemberVersion:     2,
		PermissionVersion: 2,
	})
	if err != nil {
		t.Fatalf("project member join: %v", err)
	}

	result, err := repository.ProjectTimelineEvent(ctx, types.ProjectTimelineEventCommand{
		TenantID:             "tenant-owner-transfer",
		EventID:              "owner-transfer-1",
		EventType:            types.TimelineEventConversationMemberOwnerTransferred,
		ConversationID:       "conv-owner-transfer",
		ConversationSeq:      3,
		PreviousOwnerUserID:  "owner-1",
		PreviousOwnerNewRole: "ADMIN",
		PreviousOwnerStatus:  types.DeliveryMemberStatusActive,
		NewOwnerUserID:       "user-2",
		NewOwnerNewRole:      "OWNER",
		NewOwnerStatus:       types.DeliveryMemberStatusActive,
		MemberVersion:        3,
		PermissionVersion:    3,
		ConsumerGroup:        "delivery-owner-transfer-test",
		Topic:                "conversation.timeline.events",
		PartitionID:          1,
		OffsetValue:          12,
	})
	if err != nil {
		t.Fatalf("project owner transfer: %v", err)
	}
	if !result.MembershipUpdated {
		t.Fatalf("expected membership update: %+v", result)
	}

	rows, err := pool.Query(ctx, `
SELECT user_id, role, status, member_version, permission_version
FROM delivery_membership_projection
WHERE tenant_id = 'tenant-owner-transfer'
  AND conversation_id = 'conv-owner-transfer'
ORDER BY user_id
`)
	if err != nil {
		t.Fatalf("query membership projection: %v", err)
	}
	defer rows.Close()
	got := map[string]struct {
		role              string
		status            string
		memberVersion     int64
		permissionVersion int64
	}{}
	for rows.Next() {
		var userID string
		value := struct {
			role              string
			status            string
			memberVersion     int64
			permissionVersion int64
		}{}
		if err := rows.Scan(&userID, &value.role, &value.status, &value.memberVersion, &value.permissionVersion); err != nil {
			t.Fatalf("scan membership projection: %v", err)
		}
		got[userID] = value
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("membership rows: %v", err)
	}
	if got["owner-1"].role != "ADMIN" ||
		got["owner-1"].status != types.DeliveryMemberStatusActive ||
		got["owner-1"].memberVersion != 3 ||
		got["user-2"].role != "OWNER" ||
		got["user-2"].status != types.DeliveryMemberStatusActive ||
		got["user-2"].memberVersion != 3 {
		t.Fatalf("unexpected membership projection: %+v", got)
	}

	var checkpoint int64
	err = pool.QueryRow(ctx, `
SELECT offset_value
FROM delivery_kafka_checkpoints
WHERE consumer_group = 'delivery-owner-transfer-test'
  AND topic = 'conversation.timeline.events'
  AND partition_id = 1
`).Scan(&checkpoint)
	if err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}
	if checkpoint != 12 {
		t.Fatalf("expected checkpoint 12, got %d", checkpoint)
	}
}

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

func TestRepositoryAckDeliveryIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	seedInbox(t, ctx, pool, 1)
	seedInbox(t, ctx, pool, 2)
	repository := NewRepository(pool)
	command := ackCommand(3)
	_, err := repository.AckDelivery(ctx, command)
	if !errors.Is(err, types.ErrAckOutOfVisibleRange) {
		t.Fatalf("expected out of visible range, got %v", err)
	}
	assertDeliveryCursor(t, ctx, pool, 0)

	result, err := repository.AckDelivery(ctx, ackCommand(2))
	if err != nil {
		t.Fatalf("ack visible seq: %v", err)
	}
	if result.LastReceivedSeq != 2 {
		t.Fatalf("expected cursor 2, got %d", result.LastReceivedSeq)
	}
	assertDeliveryCursor(t, ctx, pool, 2)
	assertDeliveryOutboxCount(t, ctx, pool, "delivery.ack.recorded.v1", 1)

	result, err = repository.AckDelivery(ctx, ackCommand(1))
	if err != nil {
		t.Fatalf("repeat lower ack should be idempotent: %v", err)
	}
	if result.LastReceivedSeq != 2 {
		t.Fatalf("expected cursor to stay 2, got %d", result.LastReceivedSeq)
	}
	assertDeliveryOutboxCount(t, ctx, pool, "delivery.ack.recorded.v1", 1)
}

func TestRepositoryAckDeliveryConcurrentFirstAckIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	seedInbox(t, ctx, pool, 5)
	repository := NewRepository(pool)

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repository.AckDelivery(ctx, ackCommand(5))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent ack failed: %v", err)
		}
	}
	assertDeliveryCursor(t, ctx, pool, 5)
	assertDeliveryOutboxCount(t, ctx, pool, "delivery.ack.recorded.v1", 1)
}

func messageEvent(eventID string, messageID string, seq int64) types.ProjectTimelineEventCommand {
	return types.ProjectTimelineEventCommand{
		TenantID:          "tenant-delivery",
		EventID:           eventID,
		EventType:         types.TimelineEventMessagePersisted,
		ConversationID:    "conv-delivery",
		ConversationSeq:   seq,
		FanoutMode:        "WRITE_FANOUT",
		PermissionVersion: 1,
		MessageID:         messageID,
		SenderID:          "sender-1",
		PayloadJSON:       []byte(`{"text":"hello"}`),
	}
}

func revokeEvent(eventID string, messageID string, seq int64) types.ProjectTimelineEventCommand {
	return types.ProjectTimelineEventCommand{
		TenantID:          "tenant-delivery",
		EventID:           eventID,
		EventType:         types.TimelineEventMessageRevoked,
		ConversationID:    "conv-delivery",
		ConversationSeq:   seq,
		FanoutMode:        "WRITE_FANOUT",
		PermissionVersion: 1,
		MessageID:         messageID,
		SenderID:          "sender-1",
		PayloadJSON:       []byte(`{"message_id":"msg-3","conversation_seq":7,"change_version":1,"revoked_by":"sender-1"}`),
	}
}

func editEvent(eventID string, messageID string, seq int64) types.ProjectTimelineEventCommand {
	return types.ProjectTimelineEventCommand{
		TenantID:          "tenant-delivery",
		EventID:           eventID,
		EventType:         types.TimelineEventMessageEdited,
		ConversationID:    "conv-delivery",
		ConversationSeq:   seq,
		FanoutMode:        "WRITE_FANOUT",
		PermissionVersion: 1,
		MessageID:         messageID,
		SenderID:          "sender-1",
		PayloadJSON:       []byte(`{"message_id":"msg-3","conversation_seq":7,"change_version":1,"edited_by":"sender-1","before_payload":{"text":"old"},"after_payload":{"text":"new"}}`),
	}
}

func deleteEvent(eventID string, messageID string, seq int64) types.ProjectTimelineEventCommand {
	return types.ProjectTimelineEventCommand{
		TenantID:          "tenant-delivery",
		EventID:           eventID,
		EventType:         types.TimelineEventMessageDeleted,
		ConversationID:    "conv-delivery",
		ConversationSeq:   seq,
		FanoutMode:        "WRITE_FANOUT",
		PermissionVersion: 1,
		MessageID:         messageID,
		SenderID:          "sender-1",
		PayloadJSON:       []byte(`{"message_id":"msg-3","conversation_seq":7,"change_version":1,"deleted_by":"sender-1","delete_scope":"CONVERSATION_VIEW"}`),
	}
}

func ackCommand(receivedSeq int64) types.AckDeliveryCommand {
	return types.AckDeliveryCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-delivery",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID: "conv-delivery",
		ReceivedSeq:    receivedSeq,
	}
}

func seedInbox(t *testing.T, ctx context.Context, pool *pgxpool.Pool, seq int64) {
	t.Helper()
	_, err := pool.Exec(ctx, `
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
    permission_version
) VALUES ($1, 'user-1', 'conv-delivery', $2, $3, 'message.persisted.v1', $4, 'sender-1', '{}'::jsonb, 'WRITE_FANOUT', 1)
`, "tenant-delivery", seq, fmt.Sprintf("seed-event-%d", seq), fmt.Sprintf("seed-msg-%d", seq))
	if err != nil {
		t.Fatalf("seed inbox: %v", err)
	}
}

func assertDeliveryCursor(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int64) {
	t.Helper()
	var got int64
	err := pool.QueryRow(ctx, `
SELECT COALESCE(MAX(last_received_seq), 0)
FROM device_delivery_cursors
WHERE tenant_id = 'tenant-delivery'
  AND user_id = 'user-1'
  AND device_id = 'device-1'
  AND conversation_id = 'conv-delivery'
`).Scan(&got)
	if err != nil {
		t.Fatalf("read cursor: %v", err)
	}
	if got != want {
		t.Fatalf("expected cursor %d, got %d", want, got)
	}
}

func assertDeliveryOutboxCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventType string, want int) {
	t.Helper()
	var got int
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM delivery_outbox
WHERE tenant_id = 'tenant-delivery'
  AND event_type = $1
`, eventType).Scan(&got)
	if err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if got != want {
		t.Fatalf("expected %d outbox rows for %s, got %d", want, eventType, got)
	}
}

func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	t.Cleanup(pool.Close)
	applyDeliveryMigration(t, ctx, pool)
	return pool
}

func applyDeliveryMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	root := findRepoRoot(t)
	migrationDir := filepath.Join(root, "migrations", "postgres", "delivery")
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		t.Fatalf("read migration dir: %v", err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		files = append(files, entry.Name())
	}
	slices.Sort(files)
	for _, file := range files {
		sqlBytes, err := os.ReadFile(filepath.Join(migrationDir, file))
		if err != nil {
			t.Fatalf("read migration %s: %v", file, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("apply migration %s: %v", file, err)
		}
	}
}

func resetDeliveryTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
TRUNCATE
    delivery_outbox_repair_audit,
    delivery_outbox,
    device_delivery_cursors,
    user_inbox,
    delivery_membership_projection,
    delivery_kafka_checkpoints
RESTART IDENTITY
`)
	if err != nil {
		t.Fatalf("reset delivery tables: %v", err)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("repo root not found")
		}
		wd = parent
	}
}
