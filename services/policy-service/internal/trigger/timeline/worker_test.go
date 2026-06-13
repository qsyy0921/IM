package timeline

import (
	"testing"

	conversationtimelinev1 "github.com/qsyy0921/IM/schemas/kafka"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
	"google.golang.org/protobuf/proto"
)

func TestBuildCommandFromConversationMemberJoined(t *testing.T) {
	command, err := buildCommand("policy-timeline-test", types.TimelineMessage{
		Topic:     TopicConversationTimelineEvents,
		Partition: 2,
		Offset:    40,
		Value: mustMarshalTimelineEvent(t, &conversationtimelinev1.ConversationTimelineEvent{
			EventId:          "member-joined-1",
			EventType:        types.TimelineEventConversationMemberJoined,
			TenantId:         "tenant-1",
			AggregateId:      "conv-1",
			AggregateVersion: 3,
			Payload: &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberJoined{
				ConversationMemberJoined: &conversationtimelinev1.ConversationMemberJoinedV1{
					TargetUserId:      "user-1",
					NewRole:           conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_ADMIN,
					NewStatus:         conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE,
					MemberVersion:     4,
					PermissionVersion: 8,
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if command.EventID != "member-joined-1" ||
		command.EventType != types.TimelineEventConversationMemberJoined ||
		command.ConversationID != "conv-1" ||
		command.MemberUserID != "user-1" ||
		command.MemberRole != types.ConversationMemberRoleAdmin ||
		command.MemberStatus != types.ConversationMemberStatusActive ||
		command.MemberVersion != 4 ||
		command.PermissionVersion != 8 ||
		command.OffsetValue != 41 {
		t.Fatalf("unexpected command: %+v", command)
	}
}

func TestBuildCommandFromOwnerTransferred(t *testing.T) {
	command, err := buildCommand("policy-timeline-test", types.TimelineMessage{
		Topic:  TopicConversationTimelineEvents,
		Offset: 7,
		Value: mustMarshalTimelineEvent(t, &conversationtimelinev1.ConversationTimelineEvent{
			EventId:          "owner-transfer-1",
			EventType:        types.TimelineEventConversationMemberOwnerTransferred,
			TenantId:         "tenant-1",
			AggregateId:      "conv-1",
			AggregateVersion: 9,
			Payload: &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberOwnerTransferred{
				ConversationMemberOwnerTransferred: &conversationtimelinev1.ConversationMemberOwnerTransferredV1{
					PreviousOwnerUserId:  "alice",
					NewOwnerUserId:       "bob",
					PreviousOwnerNewRole: conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_ADMIN,
					NewOwnerNewRole:      conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_OWNER,
					PreviousOwnerStatus:  conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE,
					NewOwnerStatus:       conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE,
					MemberVersion:        10,
					PermissionVersion:    11,
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if command.PreviousOwnerUserID != "alice" ||
		command.PreviousOwnerNewRole != types.ConversationMemberRoleAdmin ||
		command.NewOwnerUserID != "bob" ||
		command.NewOwnerNewRole != types.ConversationMemberRoleOwner ||
		command.MemberVersion != 10 ||
		command.PermissionVersion != 11 ||
		command.OffsetValue != 8 {
		t.Fatalf("unexpected command: %+v", command)
	}
}

func TestBuildCommandIgnoresMessageTimelineEvent(t *testing.T) {
	command, err := buildCommand("policy-timeline-test", types.TimelineMessage{
		Topic:  TopicConversationTimelineEvents,
		Offset: 12,
		Value: mustMarshalTimelineEvent(t, &conversationtimelinev1.ConversationTimelineEvent{
			EventId:          "message-1",
			EventType:        types.TimelineEventMessagePersisted,
			TenantId:         "tenant-1",
			AggregateId:      "conv-1",
			AggregateVersion: 2,
			Payload: &conversationtimelinev1.ConversationTimelineEvent_MessagePersisted{
				MessagePersisted: &conversationtimelinev1.MessagePersistedV1{
					MessageId: "msg-1",
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if !command.IgnoredMessageEvent() || command.OffsetValue != 13 {
		t.Fatalf("expected ignored message event with checkpoint, got %+v", command)
	}
}

func TestBuildCommandRejectsMalformedTimelineEvent(t *testing.T) {
	_, err := buildCommand("policy-timeline-test", types.TimelineMessage{Value: []byte("bad")})
	if err == nil {
		t.Fatal("expected malformed event error")
	}
}

func mustMarshalTimelineEvent(t *testing.T, event *conversationtimelinev1.ConversationTimelineEvent) []byte {
	t.Helper()
	value, err := proto.Marshal(event)
	if err != nil {
		t.Fatalf("marshal timeline event: %v", err)
	}
	return value
}
