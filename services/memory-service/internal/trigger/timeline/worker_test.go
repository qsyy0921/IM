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
	if projector.command.FactText != "decision: keep source references" {
		t.Fatalf("unexpected fact text %q", projector.command.FactText)
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
