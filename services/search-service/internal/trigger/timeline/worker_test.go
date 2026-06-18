package timeline

import (
	"context"
	"errors"
	"testing"

	conversationtimelinev1 "github.com/qsyy0921/IM/schemas/kafka"
	"github.com/qsyy0921/IM/services/search-service/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

type fakeConsumer struct {
	message   types.TimelineMessage
	fetchErr  error
	committed bool
}

func (consumer *fakeConsumer) Fetch(context.Context) (types.TimelineMessage, error) {
	return consumer.message, consumer.fetchErr
}

func (consumer *fakeConsumer) Commit(context.Context, types.TimelineMessage) error {
	consumer.committed = true
	return nil
}

type fakeProjector struct {
	command types.ProjectTimelineEventCommand
	err     error
}

func (projector *fakeProjector) Execute(
	_ context.Context,
	command types.ProjectTimelineEventCommand,
) (types.ProjectTimelineEventResult, error) {
	projector.command = command
	return types.ProjectTimelineEventResult{ProjectedDocument: true}, projector.err
}

func TestWorkerProjectsAndCommitsMessagePersisted(t *testing.T) {
	payload, err := structpb.NewStruct(map[string]any{
		"text": "hello searchable world",
	})
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	value := mustMarshalTimelineEvent(t, &conversationtimelinev1.ConversationTimelineEvent{
		EventId:          "event-1",
		EventType:        types.TimelineEventMessagePersisted,
		TenantId:         "tenant-1",
		AggregateId:      "conv-1",
		AggregateVersion: 10,
		TraceId:          "trace-1",
		Metadata: &conversationtimelinev1.TimelineMetadata{
			PermissionVersion: 7,
		},
		Payload: &conversationtimelinev1.ConversationTimelineEvent_MessagePersisted{
			MessagePersisted: &conversationtimelinev1.MessagePersistedV1{
				MessageId:   "msg-1",
				SenderId:    "sender-1",
				MessageType: "TEXT",
				Payload:     payload,
			},
		},
	})
	consumer := &fakeConsumer{message: types.TimelineMessage{Topic: TopicConversationTimelineEvents, Partition: 1, Offset: 41, Value: value}}
	projector := &fakeProjector{}

	if err := NewWorker(consumer, projector, "search-test").RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if !consumer.committed {
		t.Fatal("expected commit")
	}
	if projector.command.EventID != "event-1" ||
		projector.command.MessageID != "msg-1" ||
		projector.command.SearchableText != "hello searchable world" ||
		projector.command.MessageType != "TEXT" ||
		projector.command.PermissionVersion != 7 ||
		projector.command.OffsetValue != 42 ||
		projector.command.TraceID != "trace-1" {
		t.Fatalf("unexpected command: %+v", projector.command)
	}
}

func TestWorkerProjectsMessageEditedFromAfterPayload(t *testing.T) {
	beforePayload, err := structpb.NewStruct(map[string]any{"text": "old text"})
	if err != nil {
		t.Fatalf("before payload: %v", err)
	}
	afterPayload, err := structpb.NewStruct(map[string]any{"content": "new indexed text"})
	if err != nil {
		t.Fatalf("after payload: %v", err)
	}
	value := mustMarshalTimelineEvent(t, &conversationtimelinev1.ConversationTimelineEvent{
		EventId:          "event-edit",
		EventType:        types.TimelineEventMessageEdited,
		TenantId:         "tenant-1",
		AggregateId:      "conv-1",
		AggregateVersion: 11,
		Payload: &conversationtimelinev1.ConversationTimelineEvent_MessageEdited{
			MessageEdited: &conversationtimelinev1.MessageEditedV1{
				MessageId:     "msg-1",
				EditedBy:      "editor-1",
				BeforePayload: beforePayload,
				AfterPayload:  afterPayload,
			},
		},
	})
	consumer := &fakeConsumer{message: types.TimelineMessage{Topic: TopicConversationTimelineEvents, Value: value}}
	projector := &fakeProjector{}

	if err := NewWorker(consumer, projector, "search-test").RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if projector.command.SearchableText != "new indexed text" ||
		projector.command.SenderID != "editor-1" ||
		projector.command.MessageType != "TEXT" {
		t.Fatalf("unexpected edit command: %+v", projector.command)
	}
}

func TestWorkerProjectsDeletedComplianceTombstone(t *testing.T) {
	value := mustMarshalTimelineEvent(t, &conversationtimelinev1.ConversationTimelineEvent{
		EventId:          "event-delete",
		EventType:        types.TimelineEventMessageDeleted,
		TenantId:         "tenant-1",
		AggregateId:      "conv-1",
		AggregateVersion: 12,
		Payload: &conversationtimelinev1.ConversationTimelineEvent_MessageDeleted{
			MessageDeleted: &conversationtimelinev1.MessageDeletedV1{
				MessageId:   "msg-1",
				DeletedBy:   "moderator-1",
				DeleteScope: conversationtimelinev1.MessageDeleteScope_MESSAGE_DELETE_SCOPE_COMPLIANCE_RETENTION,
			},
		},
	})
	consumer := &fakeConsumer{message: types.TimelineMessage{Topic: TopicConversationTimelineEvents, Value: value}}
	projector := &fakeProjector{}

	if err := NewWorker(consumer, projector, "search-test").RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if projector.command.TombstoneStatus != types.SearchTombstoneComplianceRedacted ||
		projector.command.SenderID != "moderator-1" {
		t.Fatalf("unexpected delete command: %+v", projector.command)
	}
}

func TestWorkerProjectsMemberJoined(t *testing.T) {
	value := mustMarshalTimelineEvent(t, &conversationtimelinev1.ConversationTimelineEvent{
		EventId:          "event-join",
		EventType:        types.TimelineEventConversationMemberJoined,
		TenantId:         "tenant-1",
		AggregateId:      "conv-1",
		AggregateVersion: 1,
		Payload: &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberJoined{
			ConversationMemberJoined: &conversationtimelinev1.ConversationMemberJoinedV1{
				TargetUserId:      "user-1",
				NewRole:           conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_MEMBER,
				NewStatus:         conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE,
				MemberVersion:     2,
				PermissionVersion: 3,
			},
		},
	})
	consumer := &fakeConsumer{message: types.TimelineMessage{Topic: TopicConversationTimelineEvents, Value: value}}
	projector := &fakeProjector{}

	if err := NewWorker(consumer, projector, "search-test").RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if projector.command.TargetUserID != "user-1" ||
		projector.command.MemberRole != types.SearchMemberRoleMember ||
		projector.command.MemberStatus != types.SearchMemberStatusActive ||
		projector.command.MemberVersion != 2 ||
		projector.command.PermissionVersion != 3 {
		t.Fatalf("unexpected member command: %+v", projector.command)
	}
}

func TestWorkerProjectsOwnerTransferred(t *testing.T) {
	value := mustMarshalTimelineEvent(t, &conversationtimelinev1.ConversationTimelineEvent{
		EventId:          "event-owner",
		EventType:        types.TimelineEventConversationMemberOwnerTransferred,
		TenantId:         "tenant-1",
		AggregateId:      "conv-1",
		AggregateVersion: 5,
		Payload: &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberOwnerTransferred{
			ConversationMemberOwnerTransferred: &conversationtimelinev1.ConversationMemberOwnerTransferredV1{
				PreviousOwnerUserId:  "owner-1",
				NewOwnerUserId:       "user-2",
				PreviousOwnerNewRole: conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_ADMIN,
				PreviousOwnerStatus:  conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE,
				NewOwnerNewRole:      conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_OWNER,
				NewOwnerStatus:       conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE,
				MemberVersion:        6,
				PermissionVersion:    7,
			},
		},
	})
	consumer := &fakeConsumer{message: types.TimelineMessage{Topic: TopicConversationTimelineEvents, Value: value}}
	projector := &fakeProjector{}

	if err := NewWorker(consumer, projector, "search-test").RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if projector.command.PreviousOwnerUserID != "owner-1" ||
		projector.command.PreviousOwnerRole != types.SearchMemberRoleAdmin ||
		projector.command.NewOwnerUserID != "user-2" ||
		projector.command.NewOwnerRole != types.SearchMemberRoleOwner ||
		projector.command.MemberVersion != 6 ||
		projector.command.PermissionVersion != 7 {
		t.Fatalf("unexpected owner transfer command: %+v", projector.command)
	}
}

func TestWorkerDoesNotCommitMalformedEvent(t *testing.T) {
	consumer := &fakeConsumer{message: types.TimelineMessage{Topic: TopicConversationTimelineEvents, Value: []byte("bad")}}
	projector := &fakeProjector{}

	if err := NewWorker(consumer, projector, "search-test").RunOnce(context.Background()); err == nil {
		t.Fatal("expected malformed event error")
	}
	if consumer.committed {
		t.Fatal("did not expect commit")
	}
}

func TestWorkerDoesNotCommitProjectionFailure(t *testing.T) {
	value := mustMarshalTimelineEvent(t, &conversationtimelinev1.ConversationTimelineEvent{
		EventId:          "event-join",
		EventType:        types.TimelineEventConversationMemberJoined,
		TenantId:         "tenant-1",
		AggregateId:      "conv-1",
		AggregateVersion: 1,
		Payload: &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberJoined{
			ConversationMemberJoined: &conversationtimelinev1.ConversationMemberJoinedV1{
				TargetUserId:  "user-1",
				NewRole:       conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_MEMBER,
				NewStatus:     conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE,
				MemberVersion: 2,
			},
		},
	})
	consumer := &fakeConsumer{message: types.TimelineMessage{Topic: TopicConversationTimelineEvents, Value: value}}
	projector := &fakeProjector{err: types.NewDBWriteFailed("write failed")}

	err := NewWorker(consumer, projector, "search-test").RunOnce(context.Background())
	if !errors.Is(err, types.ErrDBWriteFailed) {
		t.Fatalf("expected projection error, got %v", err)
	}
	if consumer.committed {
		t.Fatal("did not expect commit")
	}
}

func mustMarshalTimelineEvent(t *testing.T, event *conversationtimelinev1.ConversationTimelineEvent) []byte {
	t.Helper()
	value, err := proto.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return value
}
