package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

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
				SourceEventType: "message.edited.v1",
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
		notifier.command.Notification.ConversationSeq != 7 ||
		notifier.command.Notification.SourceEventType != "message.edited.v1" {
		t.Fatalf("unexpected notifier=%+v", notifier)
	}
	if consumer.commits != 1 {
		t.Fatalf("expected commit, got %d", consumer.commits)
	}
}

func TestWorkerDefaultsMissingSourceEventTypeForOlderEvents(t *testing.T) {
	event := &deliveryeventsv1.DeliveryEvent{
		EventId:      "delivery-event-1",
		EventType:    EventInboxItemCreatedV1,
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
	notifier := &recordingNotifier{}
	worker := NewWorker(consumer, notifier)

	if err := worker.Run(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("run: %v", err)
	}
	if notifier.command.Notification.SourceEventType != SourceEventMessagePersisted {
		t.Fatalf("expected persisted recovery, got %+v", notifier.command.Notification)
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

func TestWorkerNotifiesAndCommitsInboxItemHidden(t *testing.T) {
	event := &deliveryeventsv1.DeliveryEvent{
		EventId:       "delivery-event-hide-1",
		EventType:     EventInboxItemHiddenV1,
		EventVersion:  "1.0.0",
		TenantId:      "tenant-1",
		AggregateId:   "conversation-1",
		PartitionKey:  "tenant-1:conversation-1",
		CorrelationId: "corr-1",
		CausationId:   "hide-request-1",
		Payload: &deliveryeventsv1.DeliveryEvent_InboxItemHidden{
			InboxItemHidden: &deliveryeventsv1.DeliveryInboxItemHiddenV1{
				TenantId:        "tenant-1",
				UserId:          "user-1",
				DeviceId:        "device-1",
				ConversationId:  "conversation-1",
				ConversationSeq: 7,
				MessageId:       "message-1",
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
	if notifier.calls != 1 ||
		notifier.command.Notification.Kind != types.DeliveryNotificationKindInboxItemHidden ||
		notifier.command.Notification.UserID != "user-1" ||
		notifier.command.Notification.ConversationSeq != 7 ||
		notifier.command.Notification.SourceEventType != EventInboxItemHiddenV1 ||
		notifier.command.Notification.SourceEventID != "hide-request-1" {
		t.Fatalf("unexpected notifier=%+v", notifier)
	}
	if consumer.commits != 1 {
		t.Fatalf("expected commit, got %d", consumer.commits)
	}
}

func TestWorkerCommitsConversationSignalWithoutUserNotify(t *testing.T) {
	event := &deliveryeventsv1.DeliveryEvent{
		EventId:       "delivery-signal-1",
		EventType:     EventConversationSignalV1,
		EventVersion:  "1.0.0",
		TenantId:      "tenant-1",
		AggregateId:   "conversation-1",
		PartitionKey:  "tenant-1:conversation-1",
		CorrelationId: "corr-1",
		Payload: &deliveryeventsv1.DeliveryEvent_ConversationSignal{
			ConversationSignal: &deliveryeventsv1.DeliveryConversationSignalV1{
				TenantId:        "tenant-1",
				ConversationId:  "conversation-1",
				ConversationSeq: 7,
				SourceEventId:   "timeline-event-1",
				SourceEventType: SourceEventMessagePersisted,
				MessageId:       "message-1",
				SenderId:        "sender-1",
				FanoutMode:      "READ_FANOUT",
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
		t.Fatalf("conversation signal must not fabricate user-level notify")
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

func TestWorkerRetriesTransientNotifierError(t *testing.T) {
	event := &deliveryeventsv1.DeliveryEvent{
		EventId:      "delivery-event-1",
		EventType:    EventInboxItemCreatedV1,
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
	consumer := &fakeConsumer{messages: []types.DeliveryEventMessage{
		{Value: value},
		{Value: value},
	}}
	notifier := &recordingNotifier{errs: []error{types.NewDeliveryUnavailable("temporary"), nil}}
	worker := NewWorker(consumer, notifier, Config{ErrorBackoff: time.Millisecond})

	if err := worker.Run(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("run: %v", err)
	}
	if notifier.calls != 2 {
		t.Fatalf("expected retry notifier calls, got %d", notifier.calls)
	}
	if consumer.commits != 1 {
		t.Fatalf("expected single commit after retry, got %d", consumer.commits)
	}
	snapshot := worker.Snapshot()
	if snapshot.TotalErrors != 1 || snapshot.ConsecutiveErrors != 0 || snapshot.LastErrorBackoffMS != time.Millisecond.Milliseconds() {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestWorkerFailsFastWhenNotifierMissing(t *testing.T) {
	worker := NewWorker(&fakeConsumer{}, nil)
	err := worker.Run(context.Background())
	if err == nil || err.Error() != "push delivery notifier is not configured" {
		t.Fatalf("unexpected error: %v", err)
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
	errs    []error
}

func (notifier *recordingNotifier) Execute(ctx context.Context, command types.NotifyDeliveryCommand) (types.NotifyDeliveryResult, error) {
	notifier.calls++
	notifier.command = command
	if notifier.calls <= len(notifier.errs) {
		return types.NotifyDeliveryResult{}, notifier.errs[notifier.calls-1]
	}
	return types.NotifyDeliveryResult{MatchedSessions: 1, Enqueued: 1}, nil
}
