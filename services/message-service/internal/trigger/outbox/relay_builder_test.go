package outbox

import (
	"testing"

	conversationtimelinev1 "github.com/qsyy0921/IM/schemas/kafka"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

func TestBuildConversationTimelineEventMessagePersisted(t *testing.T) {
	message := testOutboxMessage()

	event, err := BuildConversationTimelineEvent(message)
	if err != nil {
		t.Fatalf("build event: %v", err)
	}
	if event.EventId != string(message.EventID) ||
		event.EventType != string(types.TimelineEventMessagePersisted) ||
		event.AggregateType != "conversation" ||
		event.AggregateVersion != message.AggregateVersion ||
		event.PartitionKey != message.PartitionKey {
		t.Fatalf("unexpected envelope: %+v", event)
	}
	if event.Metadata == nil ||
		event.Metadata.PermissionVersion != message.PermissionVersion ||
		event.Metadata.FanoutMode != string(message.FanoutMode) {
		t.Fatalf("unexpected metadata: %+v", event.Metadata)
	}
	payload := event.GetMessagePersisted()
	if payload == nil {
		t.Fatalf("expected message_persisted payload")
	}
	if payload.CommandHash != "hash-1" ||
		payload.MessageId != "msg-1" ||
		payload.ConversationSeq != 1 ||
		payload.Payload.GetFields()["text"].GetStringValue() != "hello" {
		t.Fatalf("unexpected message payload: %+v", payload)
	}
}

func TestBuildConversationTimelineEventMessageRevoked(t *testing.T) {
	message := testRevokedOutboxMessage()

	event, err := BuildConversationTimelineEvent(message)
	if err != nil {
		t.Fatalf("build event: %v", err)
	}
	if event.EventId != string(message.EventID) ||
		event.EventType != string(types.TimelineEventMessageRevoked) ||
		event.AggregateVersion != message.AggregateVersion ||
		event.PartitionKey != message.PartitionKey {
		t.Fatalf("unexpected envelope: %+v", event)
	}
	payload := event.GetMessageRevoked()
	if payload == nil {
		t.Fatalf("expected message_revoked payload")
	}
	if payload.MessageId != "msg-1" ||
		payload.ConversationSeq != 2 ||
		payload.ChangeVersion != 1 ||
		payload.RevokedBy != "user-1" ||
		payload.RevokedAt == nil {
		t.Fatalf("unexpected revoke payload: %+v", payload)
	}
}

func TestBuildConversationTimelineEventMessageEdited(t *testing.T) {
	message := testEditedOutboxMessage()

	event, err := BuildConversationTimelineEvent(message)
	if err != nil {
		t.Fatalf("build event: %v", err)
	}
	if event.EventId != string(message.EventID) ||
		event.EventType != string(types.TimelineEventMessageEdited) ||
		event.AggregateVersion != message.AggregateVersion ||
		event.PartitionKey != message.PartitionKey {
		t.Fatalf("unexpected envelope: %+v", event)
	}
	payload := event.GetMessageEdited()
	if payload == nil {
		t.Fatalf("expected message_edited payload")
	}
	if payload.MessageId != "msg-1" ||
		payload.ConversationSeq != 2 ||
		payload.ChangeVersion != 1 ||
		payload.EditedBy != "user-1" ||
		payload.BeforePayload.GetFields()["text"].GetStringValue() != "hello" ||
		payload.AfterPayload.GetFields()["text"].GetStringValue() != "hello edited" ||
		payload.EditedAt == nil {
		t.Fatalf("unexpected edit payload: %+v", payload)
	}
}

func TestBuildConversationTimelineEventMessageDeleted(t *testing.T) {
	message := testDeletedOutboxMessage()

	event, err := BuildConversationTimelineEvent(message)
	if err != nil {
		t.Fatalf("build event: %v", err)
	}
	if event.EventId != string(message.EventID) ||
		event.EventType != string(types.TimelineEventMessageDeleted) ||
		event.AggregateVersion != message.AggregateVersion ||
		event.PartitionKey != message.PartitionKey {
		t.Fatalf("unexpected envelope: %+v", event)
	}
	payload := event.GetMessageDeleted()
	if payload == nil {
		t.Fatalf("expected message_deleted payload")
	}
	if payload.MessageId != "msg-1" ||
		payload.ConversationSeq != 2 ||
		payload.ChangeVersion != 1 ||
		payload.DeletedBy != "user-1" ||
		payload.DeleteScope != conversationtimelinev1.MessageDeleteScope_MESSAGE_DELETE_SCOPE_CONVERSATION_VIEW ||
		payload.DeletedAt == nil {
		t.Fatalf("unexpected delete payload: %+v", payload)
	}
}

func TestBuildConversationTimelineEventMemberBoundaryPayloads(t *testing.T) {
	tests := []struct {
		name      string
		eventType types.TimelineEventType
		assert    func(*conversationtimelinev1.ConversationTimelineEvent) *conversationtimelinev1.ConversationMemberJoinedV1
	}{
		{
			name:      "joined",
			eventType: types.TimelineEventConversationMemberJoined,
			assert: func(event *conversationtimelinev1.ConversationTimelineEvent) *conversationtimelinev1.ConversationMemberJoinedV1 {
				return event.GetConversationMemberJoined()
			},
		},
		{
			name:      "left",
			eventType: types.TimelineEventConversationMemberLeft,
			assert: func(event *conversationtimelinev1.ConversationTimelineEvent) *conversationtimelinev1.ConversationMemberJoinedV1 {
				payload := event.GetConversationMemberLeft()
				if payload == nil {
					return nil
				}
				return &conversationtimelinev1.ConversationMemberJoinedV1{
					ChangeId:          payload.ChangeId,
					ConversationId:    payload.ConversationId,
					BoundarySeq:       payload.BoundarySeq,
					TargetUserId:      payload.TargetUserId,
					OperatorUserId:    payload.OperatorUserId,
					ChangeType:        payload.ChangeType,
					OldRole:           payload.OldRole,
					NewRole:           payload.NewRole,
					OldStatus:         payload.OldStatus,
					NewStatus:         payload.NewStatus,
					MemberVersion:     payload.MemberVersion,
					PermissionVersion: payload.PermissionVersion,
					Reason:            payload.Reason,
					OccurredAt:        payload.OccurredAt,
				}
			},
		},
		{
			name:      "removed",
			eventType: types.TimelineEventConversationMemberRemoved,
			assert: func(event *conversationtimelinev1.ConversationTimelineEvent) *conversationtimelinev1.ConversationMemberJoinedV1 {
				payload := event.GetConversationMemberRemoved()
				if payload == nil {
					return nil
				}
				return &conversationtimelinev1.ConversationMemberJoinedV1{
					ChangeId:          payload.ChangeId,
					ConversationId:    payload.ConversationId,
					BoundarySeq:       payload.BoundarySeq,
					TargetUserId:      payload.TargetUserId,
					OperatorUserId:    payload.OperatorUserId,
					ChangeType:        payload.ChangeType,
					OldRole:           payload.OldRole,
					NewRole:           payload.NewRole,
					OldStatus:         payload.OldStatus,
					NewStatus:         payload.NewStatus,
					MemberVersion:     payload.MemberVersion,
					PermissionVersion: payload.PermissionVersion,
					Reason:            payload.Reason,
					OccurredAt:        payload.OccurredAt,
				}
			},
		},
		{
			name:      "role_changed",
			eventType: types.TimelineEventConversationMemberRoleChanged,
			assert: func(event *conversationtimelinev1.ConversationTimelineEvent) *conversationtimelinev1.ConversationMemberJoinedV1 {
				payload := event.GetConversationMemberRoleChanged()
				if payload == nil {
					return nil
				}
				return &conversationtimelinev1.ConversationMemberJoinedV1{
					ChangeId:          payload.ChangeId,
					ConversationId:    payload.ConversationId,
					BoundarySeq:       payload.BoundarySeq,
					TargetUserId:      payload.TargetUserId,
					OperatorUserId:    payload.OperatorUserId,
					ChangeType:        payload.ChangeType,
					OldRole:           payload.OldRole,
					NewRole:           payload.NewRole,
					OldStatus:         payload.OldStatus,
					NewStatus:         payload.NewStatus,
					MemberVersion:     payload.MemberVersion,
					PermissionVersion: payload.PermissionVersion,
					Reason:            payload.Reason,
					OccurredAt:        payload.OccurredAt,
				}
			},
		},
		{
			name:      "boundary_cancelled",
			eventType: types.TimelineEventConversationMemberBoundaryCancelled,
			assert: func(event *conversationtimelinev1.ConversationTimelineEvent) *conversationtimelinev1.ConversationMemberJoinedV1 {
				payload := event.GetConversationMemberBoundaryCancelled()
				if payload == nil {
					return nil
				}
				return &conversationtimelinev1.ConversationMemberJoinedV1{
					ChangeId:          payload.ChangeId,
					ConversationId:    payload.ConversationId,
					BoundarySeq:       payload.BoundarySeq,
					TargetUserId:      payload.TargetUserId,
					OperatorUserId:    payload.OperatorUserId,
					ChangeType:        payload.ChangeType,
					OldRole:           payload.OldRole,
					NewRole:           payload.NewRole,
					OldStatus:         payload.OldStatus,
					NewStatus:         payload.NewStatus,
					MemberVersion:     payload.MemberVersion,
					PermissionVersion: payload.PermissionVersion,
					Reason:            payload.Reason,
					OccurredAt:        payload.OccurredAt,
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := testMemberBoundaryOutboxMessage(tt.eventType)
			event, err := BuildConversationTimelineEvent(message)
			if err != nil {
				t.Fatalf("build member boundary event: %v", err)
			}
			if event.EventId != string(message.EventID) ||
				event.EventType != string(tt.eventType) ||
				event.AggregateVersion != message.AggregateVersion ||
				event.Producer != "conversation-service" ||
				event.Metadata.PermissionVersion != 8 {
				t.Fatalf("unexpected envelope: %+v", event)
			}
			payload := tt.assert(event)
			if payload == nil {
				t.Fatalf("expected member boundary payload")
			}
			if payload.ChangeId != "change-1" ||
				payload.ConversationId != "conv-1" ||
				payload.BoundarySeq != 2 ||
				payload.TargetUserId != "user-2" ||
				payload.OperatorUserId != "owner-1" ||
				payload.ChangeType != conversationtimelinev1.ConversationMemberChangeType_CONVERSATION_MEMBER_CHANGE_TYPE_JOIN ||
				payload.OldRole != conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_MEMBER ||
				payload.NewRole != conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_ADMIN ||
				payload.OldStatus != conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_LEFT ||
				payload.NewStatus != conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE ||
				payload.MemberVersion != 7 ||
				payload.PermissionVersion != 8 ||
				payload.OccurredAt == nil {
				t.Fatalf("unexpected payload: %+v", payload)
			}
		})
	}
}

func TestBuildConversationTimelineEventOwnerTransferredPayload(t *testing.T) {
	message := testOwnerTransferredOutboxMessage()

	event, err := BuildConversationTimelineEvent(message)
	if err != nil {
		t.Fatalf("build owner transferred event: %v", err)
	}
	if event.EventId != string(message.EventID) ||
		event.EventType != string(types.TimelineEventConversationMemberOwnerTransferred) ||
		event.AggregateVersion != message.AggregateVersion ||
		event.Producer != "conversation-service" ||
		event.Metadata.PermissionVersion != 9 {
		t.Fatalf("unexpected envelope: %+v", event)
	}
	payload := event.GetConversationMemberOwnerTransferred()
	if payload == nil {
		t.Fatalf("expected owner transferred payload")
	}
	if payload.ChangeId != "change-owner-1" ||
		payload.ConversationId != "conv-1" ||
		payload.BoundarySeq != 3 ||
		payload.PreviousOwnerUserId != "owner-1" ||
		payload.NewOwnerUserId != "user-2" ||
		payload.OperatorUserId != "owner-1" ||
		payload.ChangeType != conversationtimelinev1.ConversationMemberChangeType_CONVERSATION_MEMBER_CHANGE_TYPE_OWNER_TRANSFER ||
		payload.PreviousOwnerOldRole != conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_OWNER ||
		payload.PreviousOwnerNewRole != conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_ADMIN ||
		payload.NewOwnerOldRole != conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_MEMBER ||
		payload.NewOwnerNewRole != conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_OWNER ||
		payload.PreviousOwnerStatus != conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE ||
		payload.NewOwnerStatus != conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE ||
		payload.MemberVersion != 8 ||
		payload.PermissionVersion != 9 ||
		payload.OccurredAt == nil {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestBuildConversationTimelineEventMalformedOwnerTransferredPayloadFailsClosed(t *testing.T) {
	message := testOwnerTransferredOutboxMessage()
	message.PayloadJSON = []byte(`{
		"change_id":"change-owner-1",
		"conversation_id":"conv-1",
		"boundary_seq":3,
		"previous_owner_user_id":"owner-1",
		"new_owner_user_id":"user-2",
		"operator_user_id":"owner-1",
		"change_type":"OWNER_TRANSFER",
		"previous_owner_old_role":"OWNER",
		"previous_owner_new_role":"ADMIN",
		"new_owner_old_role":"OWNER",
		"new_owner_new_role":"OWNER",
		"previous_owner_status":"ACTIVE",
		"new_owner_status":"ACTIVE",
		"member_version":8,
		"permission_version":9,
		"occurred_at":"2026-06-09T09:31:00Z"
	}`)

	if _, err := BuildConversationTimelineEvent(message); err == nil {
		t.Fatalf("expected malformed owner transferred payload error")
	}
}

func TestBuildConversationTimelineEventUnsupportedEventFailsClosed(t *testing.T) {
	message := testMemberBoundaryOutboxMessage("conversation.member.unknown.v1")

	if _, err := BuildConversationTimelineEvent(message); err == nil {
		t.Fatalf("expected unsupported event error")
	}
}

func TestBuildConversationTimelineEventMalformedMemberPayloadFailsClosed(t *testing.T) {
	message := testMemberBoundaryOutboxMessage(types.TimelineEventConversationMemberJoined)
	message.PayloadJSON = []byte(`{"change_id":"change-1"}`)

	if _, err := BuildConversationTimelineEvent(message); err == nil {
		t.Fatalf("expected malformed member payload error")
	}
}
