package timeline

import (
	"context"
	"errors"
	"testing"

	conversationtimelinev1 "github.com/qsyy0921/IM/schemas/kafka"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
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
	return context.Canceled
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
	return types.ProjectTimelineEventResult{ProjectedInboxCount: 1}, projector.err
}

func TestWorkerProjectsAndCommitsMessagePersisted(t *testing.T) {
	payload, err := structpb.NewStruct(map[string]any{"text": "hello"})
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	value := mustMarshalTimelineEvent(t, &conversationtimelinev1.ConversationTimelineEvent{
		EventId:          "event-1",
		EventType:        types.TimelineEventMessagePersisted,
		TenantId:         "tenant-1",
		AggregateId:      "conv-1",
		AggregateVersion: 10,
		CorrelationId:    "request-1",
		Metadata: &conversationtimelinev1.TimelineMetadata{
			FanoutMode:        "WRITE_FANOUT",
			PermissionVersion: 7,
		},
		Payload: &conversationtimelinev1.ConversationTimelineEvent_MessagePersisted{
			MessagePersisted: &conversationtimelinev1.MessagePersistedV1{
				MessageId: "msg-1",
				SenderId:  "sender-1",
				Payload:   payload,
			},
		},
	})
	consumer := &fakeConsumer{message: types.TimelineMessage{Topic: "conversation.timeline.events", Partition: 3, Offset: 41, Value: value}}
	projector := &fakeProjector{}
	err = NewWorker(consumer, projector, "delivery-test").Run(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected stop after fake commit, got %v", err)
	}
	if !consumer.committed {
		t.Fatal("expected message commit")
	}
	if projector.command.EventID != "event-1" ||
		projector.command.MessageID != "msg-1" ||
		projector.command.OffsetValue != 42 ||
		projector.command.ConsumerGroup != "delivery-test" {
		t.Fatalf("unexpected command: %+v", projector.command)
	}
}

func TestWorkerProjectsAndCommitsMessageRevoked(t *testing.T) {
	value := mustMarshalTimelineEvent(t, &conversationtimelinev1.ConversationTimelineEvent{
		EventId:          "event-revoke-1",
		EventType:        types.TimelineEventMessageRevoked,
		TenantId:         "tenant-1",
		AggregateId:      "conv-1",
		AggregateVersion: 11,
		CorrelationId:    "request-1",
		Metadata: &conversationtimelinev1.TimelineMetadata{
			FanoutMode:        "WRITE_FANOUT",
			PermissionVersion: 7,
		},
		Payload: &conversationtimelinev1.ConversationTimelineEvent_MessageRevoked{
			MessageRevoked: &conversationtimelinev1.MessageRevokedV1{
				MessageId:       "msg-1",
				ConversationSeq: 11,
				ChangeVersion:   1,
				RevokedBy:       "sender-1",
			},
		},
	})
	consumer := &fakeConsumer{message: types.TimelineMessage{Topic: "conversation.timeline.events", Partition: 3, Offset: 42, Value: value}}
	projector := &fakeProjector{}
	err := NewWorker(consumer, projector, "delivery-test").Run(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected stop after fake commit, got %v", err)
	}
	if !consumer.committed {
		t.Fatal("expected message commit")
	}
	if projector.command.EventID != "event-revoke-1" ||
		projector.command.EventType != types.TimelineEventMessageRevoked ||
		projector.command.MessageID != "msg-1" ||
		projector.command.SenderID != "sender-1" ||
		projector.command.OffsetValue != 43 {
		t.Fatalf("unexpected command: %+v", projector.command)
	}
}

func TestWorkerProjectsAndCommitsMessageEdited(t *testing.T) {
	beforePayload, err := structpb.NewStruct(map[string]any{"text": "hello"})
	if err != nil {
		t.Fatalf("before payload: %v", err)
	}
	afterPayload, err := structpb.NewStruct(map[string]any{"text": "hello edited"})
	if err != nil {
		t.Fatalf("after payload: %v", err)
	}
	value := mustMarshalTimelineEvent(t, &conversationtimelinev1.ConversationTimelineEvent{
		EventId:          "event-edit-1",
		EventType:        types.TimelineEventMessageEdited,
		TenantId:         "tenant-1",
		AggregateId:      "conv-1",
		AggregateVersion: 11,
		CorrelationId:    "request-1",
		Metadata: &conversationtimelinev1.TimelineMetadata{
			FanoutMode:        "WRITE_FANOUT",
			PermissionVersion: 7,
		},
		Payload: &conversationtimelinev1.ConversationTimelineEvent_MessageEdited{
			MessageEdited: &conversationtimelinev1.MessageEditedV1{
				MessageId:       "msg-1",
				ConversationSeq: 11,
				ChangeVersion:   1,
				EditedBy:        "sender-1",
				BeforePayload:   beforePayload,
				AfterPayload:    afterPayload,
			},
		},
	})
	consumer := &fakeConsumer{message: types.TimelineMessage{Topic: "conversation.timeline.events", Partition: 3, Offset: 42, Value: value}}
	projector := &fakeProjector{}
	err = NewWorker(consumer, projector, "delivery-test").Run(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected stop after fake commit, got %v", err)
	}
	if !consumer.committed {
		t.Fatal("expected message commit")
	}
	if projector.command.EventID != "event-edit-1" ||
		projector.command.EventType != types.TimelineEventMessageEdited ||
		projector.command.MessageID != "msg-1" ||
		projector.command.SenderID != "sender-1" ||
		projector.command.OffsetValue != 43 {
		t.Fatalf("unexpected command: %+v", projector.command)
	}
}

func TestWorkerDoesNotCommitWhenProjectionFails(t *testing.T) {
	value := mustMarshalTimelineEvent(t, &conversationtimelinev1.ConversationTimelineEvent{
		EventId:          "event-1",
		EventType:        types.TimelineEventConversationMemberJoined,
		TenantId:         "tenant-1",
		AggregateId:      "conv-1",
		AggregateVersion: 10,
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
	consumer := &fakeConsumer{message: types.TimelineMessage{Topic: "conversation.timeline.events", Value: value}}
	projector := &fakeProjector{err: types.NewDBWriteFailed("boom")}
	err := NewWorker(consumer, projector, "delivery-test").Run(context.Background())
	if !errors.Is(err, types.ErrDBWriteFailed) {
		t.Fatalf("expected projection error, got %v", err)
	}
	if consumer.committed {
		t.Fatal("did not expect commit")
	}
}

func TestWorkerRejectsMalformedEvent(t *testing.T) {
	consumer := &fakeConsumer{message: types.TimelineMessage{Value: []byte("bad")}}
	projector := &fakeProjector{}
	err := NewWorker(consumer, projector, "delivery-test").Run(context.Background())
	if err == nil {
		t.Fatal("expected malformed event error")
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
