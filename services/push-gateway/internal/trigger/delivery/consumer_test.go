package delivery

import (
	"context"
	"errors"
	"testing"

	deliveryeventsv1 "github.com/qsyy0921/IM/schemas/kafka/delivery/v1"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
	"google.golang.org/protobuf/proto"
)

func TestWorkerNotifiesAndCommitsInboxItemCreated(t *testing.T) {
	event := &deliveryeventsv1.DeliveryEvent{
		EventId:       "delivery-event-1",
		EventType:     EventInboxItemCreatedV1,
		EventVersion:  "1.0.0",
		TenantId:      "tenant-1",
		AggregateId:   "conversation-1",
		PartitionKey:  "tenant-1:conversation-1",
		CorrelationId: "corr-1",
		Payload: &deliveryeventsv1.DeliveryEvent_InboxItemCreated{
			InboxItemCreated: &deliveryeventsv1.DeliveryInboxItemCreatedV1{
				TenantId:        "tenant-1",
				UserId:          "user-1",
				ConversationId:  "conversation-1",
				ConversationSeq: 7,
				SourceEventId:   "timeline-event-1",
				MessageId:       "message-1",
			},
		},
	}
	value, _ := proto.Marshal(event)
	consumer := &fakeConsumer{messages: []types.DeliveryEventMessage{{Topic: "im.delivery.events", Offset: 10, Value: value}}}
	notifier := &recordingNotifier{}
	worker := NewWorker(consumer, notifier)

	if err := worker.Run(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("run: %v", err)
	}
	if notifier.calls != 1 ||
		notifier.command.Notification.UserID != "user-1" ||
		notifier.command.Notification.ConversationSeq != 7 {
		t.Fatalf("unexpected notifier=%+v", notifier)
	}
	if consumer.commits != 1 {
		t.Fatalf("expected commit, got %d", consumer.commits)
	}
}

func TestWorkerCommitsAckRecordedWithoutNotify(t *testing.T) {
	event := &deliveryeventsv1.DeliveryEvent{
		EventId:      "delivery-event-1",
		EventType:    EventDeliveryAckRecordedV1,
		EventVersion: "1.0.0",
		TenantId:     "tenant-1",
		AggregateId:  "conversation-1",
		PartitionKey: "tenant-1:conversation-1",
		Payload: &deliveryeventsv1.DeliveryEvent_AckRecorded{
			AckRecorded: &deliveryeventsv1.DeliveryAckRecordedV1{
				TenantId:        "tenant-1",
				UserId:          "user-1",
				DeviceId:        "device-1",
				ConversationId:  "conversation-1",
				LastReceivedSeq: 7,
			},
		},
	}
	value, _ := proto.Marshal(event)
	consumer := &fakeConsumer{messages: []types.DeliveryEventMessage{{Value: value}}}
	notifier := &recordingNotifier{}
	worker := NewWorker(consumer, notifier)

	if err := worker.Run(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("run: %v", err)
	}
	if notifier.calls != 0 {
		t.Fatalf("ack event must not notify clients")
	}
	if consumer.commits != 1 {
		t.Fatalf("expected commit, got %d", consumer.commits)
	}
}

func TestWorkerFailClosedForUnsupportedEvent(t *testing.T) {
	value, _ := proto.Marshal(&deliveryeventsv1.DeliveryEvent{
		EventId:      "delivery-event-1",
		EventType:    "delivery.unknown.v1",
		EventVersion: "1.0.0",
		TenantId:     "tenant-1",
		AggregateId:  "conversation-1",
		PartitionKey: "tenant-1:conversation-1",
	})
	consumer := &fakeConsumer{messages: []types.DeliveryEventMessage{{Value: value}}}
	worker := NewWorker(consumer, &recordingNotifier{})

	if err := worker.Run(context.Background()); !errors.Is(err, types.ErrUnsupportedDeliveryEvent) {
		t.Fatalf("expected unsupported error, got %v", err)
	}
	if consumer.commits != 0 {
		t.Fatalf("unsupported event must not be committed")
	}
}

func TestWorkerFailClosedForEventTypeMismatch(t *testing.T) {
	event := &deliveryeventsv1.DeliveryEvent{
		EventId:      "delivery-event-1",
		EventType:    EventDeliveryAckRecordedV1,
		EventVersion: "1.0.0",
		TenantId:     "tenant-1",
		AggregateId:  "conversation-1",
		PartitionKey: "tenant-1:conversation-1",
		Payload: &deliveryeventsv1.DeliveryEvent_InboxItemCreated{
			InboxItemCreated: &deliveryeventsv1.DeliveryInboxItemCreatedV1{
				TenantId:        "tenant-1",
				UserId:          "user-1",
				ConversationId:  "conversation-1",
				ConversationSeq: 7,
				SourceEventId:   "timeline-event-1",
				MessageId:       "message-1",
			},
		},
	}
	value, _ := proto.Marshal(event)
	consumer := &fakeConsumer{messages: []types.DeliveryEventMessage{{Value: value}}}
	worker := NewWorker(consumer, &recordingNotifier{})

	if err := worker.Run(context.Background()); err == nil {
		t.Fatalf("expected fail-closed error")
	}
	if consumer.commits != 0 {
		t.Fatalf("mismatched event must not be committed")
	}
}

type fakeConsumer struct {
	messages []types.DeliveryEventMessage
	commits  int
}

func (consumer *fakeConsumer) Fetch(ctx context.Context) (types.DeliveryEventMessage, error) {
	if len(consumer.messages) == 0 {
		return types.DeliveryEventMessage{}, context.Canceled
	}
	message := consumer.messages[0]
	consumer.messages = consumer.messages[1:]
	return message, nil
}

func (consumer *fakeConsumer) Commit(ctx context.Context, message types.DeliveryEventMessage) error {
	consumer.commits++
	return nil
}

type recordingNotifier struct {
	calls   int
	command types.NotifyDeliveryCommand
}

func (notifier *recordingNotifier) Execute(ctx context.Context, command types.NotifyDeliveryCommand) (types.NotifyDeliveryResult, error) {
	notifier.calls++
	notifier.command = command
	return types.NotifyDeliveryResult{MatchedSessions: 1, Enqueued: 1}, nil
}
