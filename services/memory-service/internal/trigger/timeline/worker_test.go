package timeline

import (
	"context"
	"testing"

	conversationtimelinev1 "github.com/qsyy0921/IM/schemas/kafka"
	"github.com/qsyy0921/IM/services/memory-service/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestWorkerRunOnceProjectsAndCommitsMessagePersisted(t *testing.T) {
	payload, err := structpb.NewStruct(map[string]any{"text": "decision: keep source references"})
	if err != nil {
		t.Fatal(err)
	}
	value, err := proto.Marshal(&conversationtimelinev1.ConversationTimelineEvent{
		EventId:          "event-1",
		EventType:        types.TimelineEventMessagePersisted,
		TenantId:         "tenant-1",
		AggregateId:      "conv-1",
		AggregateVersion: 2,
		Payload: &conversationtimelinev1.ConversationTimelineEvent_MessagePersisted{
			MessagePersisted: &conversationtimelinev1.MessagePersistedV1{
				MessageId: "msg-1",
				SenderId:  "user-1",
				Payload:   payload,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	consumer := &fakeConsumer{message: types.TimelineMessage{Topic: TopicConversationTimelineEvents, Partition: 1, Offset: 41, Value: value}}
	projector := &fakeProjector{}
	worker := NewWorker(consumer, projector, "memory-test")
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if !consumer.committed {
		t.Fatal("expected commit")
	}
	if projector.command.EventID != "event-1" || projector.command.MessageID != "msg-1" || projector.command.OffsetValue != 42 {
		t.Fatalf("unexpected command: %+v", projector.command)
	}
	if !projector.command.ProjectMemory ||
		projector.command.MemoryEventType != types.MemoryEventTypeDecision ||
		projector.command.MemoryReviewState != types.MemoryReviewUnreviewed ||
		projector.command.ExtractionVersion != "rules-v0.2" {
		t.Fatalf("unexpected memory extraction metadata: %+v", projector.command)
	}
	if projector.command.FactText != "keep source references" {
		t.Fatalf("unexpected stripped fact text %q", projector.command.FactText)
	}
}

func TestWorkerRunOnceDoesNotProjectOrdinaryChatAsMemory(t *testing.T) {
	payload, err := structpb.NewStruct(map[string]any{"text": "hello, this is just an ordinary chat"})
	if err != nil {
		t.Fatal(err)
	}
	value, err := proto.Marshal(&conversationtimelinev1.ConversationTimelineEvent{
		EventId:          "event-chat",
		EventType:        types.TimelineEventMessagePersisted,
		TenantId:         "tenant-1",
		AggregateId:      "conv-1",
		AggregateVersion: 3,
		Payload: &conversationtimelinev1.ConversationTimelineEvent_MessagePersisted{
			MessagePersisted: &conversationtimelinev1.MessagePersistedV1{
				MessageId: "msg-chat",
				SenderId:  "user-1",
				Payload:   payload,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	consumer := &fakeConsumer{message: types.TimelineMessage{Topic: TopicConversationTimelineEvents, Partition: 1, Offset: 42, Value: value}}
	projector := &fakeProjector{}
	worker := NewWorker(consumer, projector, "memory-test")
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if !consumer.committed {
		t.Fatal("ordinary chat event should still commit after checkpoint projection")
	}
	if !projector.called {
		t.Fatal("ordinary chat should still reach projector for checkpoint handling")
	}
	if projector.command.ProjectMemory || projector.command.FactText != "" {
		t.Fatalf("ordinary chat must not be projected as memory: %+v", projector.command)
	}
}

func TestWorkerRunOnceProfileSignalNeedsReview(t *testing.T) {
	payload, err := structpb.NewStruct(map[string]any{
		"profile_signal": "coordinates incident handoffs",
		"memory_topic":   "incident-response",
	})
	if err != nil {
		t.Fatal(err)
	}
	value, err := proto.Marshal(&conversationtimelinev1.ConversationTimelineEvent{
		EventId:          "event-profile",
		EventType:        types.TimelineEventMessagePersisted,
		TenantId:         "tenant-1",
		AggregateId:      "conv-1",
		AggregateVersion: 4,
		Payload: &conversationtimelinev1.ConversationTimelineEvent_MessagePersisted{
			MessagePersisted: &conversationtimelinev1.MessagePersistedV1{
				MessageId: "msg-profile",
				SenderId:  "user-1",
				Payload:   payload,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	consumer := &fakeConsumer{message: types.TimelineMessage{Topic: TopicConversationTimelineEvents, Partition: 1, Offset: 43, Value: value}}
	projector := &fakeProjector{}
	worker := NewWorker(consumer, projector, "memory-test")
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if !projector.command.ProjectMemory ||
		projector.command.MemoryEventType != types.MemoryEventTypeProfileSignal ||
		projector.command.MemoryReviewState != types.MemoryReviewNeedsReview ||
		projector.command.TopicText != "incident-response" {
		t.Fatalf("unexpected profile signal extraction: %+v", projector.command)
	}
}

func TestWorkerRunOnceMalformedDoesNotCommit(t *testing.T) {
	consumer := &fakeConsumer{message: types.TimelineMessage{Topic: TopicConversationTimelineEvents, Partition: 0, Offset: 1, Value: []byte("bad")}}
	projector := &fakeProjector{}
	worker := NewWorker(consumer, projector, "memory-test")
	if err := worker.RunOnce(context.Background()); err == nil {
		t.Fatal("expected malformed error")
	}
	if consumer.committed {
		t.Fatal("malformed event should not commit")
	}
	if projector.called {
		t.Fatal("malformed event should not project")
	}
}

func TestWorkerRunOnceUnsupportedDoesNotCommit(t *testing.T) {
	value, err := proto.Marshal(&conversationtimelinev1.ConversationTimelineEvent{
		EventId:          "event-unsupported",
		EventType:        "unsupported.v1",
		TenantId:         "tenant-1",
		AggregateId:      "conv-1",
		AggregateVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	consumer := &fakeConsumer{message: types.TimelineMessage{Topic: TopicConversationTimelineEvents, Partition: 0, Offset: 1, Value: value}}
	projector := &fakeProjector{}
	worker := NewWorker(consumer, projector, "memory-test")
	if err := worker.RunOnce(context.Background()); err == nil {
		t.Fatal("expected unsupported error")
	}
	if consumer.committed {
		t.Fatal("unsupported event should not commit")
	}
	if projector.called {
		t.Fatal("unsupported event should not project")
	}
}

type fakeConsumer struct {
	message   types.TimelineMessage
	committed bool
}

func (consumer *fakeConsumer) Fetch(context.Context) (types.TimelineMessage, error) {
	return consumer.message, nil
}

func (consumer *fakeConsumer) Commit(context.Context, types.TimelineMessage) error {
	consumer.committed = true
	return nil
}

type fakeProjector struct {
	called  bool
	command types.ProjectTimelineEventCommand
	err     error
}

func (projector *fakeProjector) Execute(_ context.Context, command types.ProjectTimelineEventCommand) (types.ProjectTimelineEventResult, error) {
	projector.called = true
	projector.command = command
	if projector.err != nil {
		return types.ProjectTimelineEventResult{}, projector.err
	}
	return types.ProjectTimelineEventResult{}, nil
}

var _ Consumer = (*fakeConsumer)(nil)
var _ Projector = (*fakeProjector)(nil)
