package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	deliveryeventsv1 "github.com/qsyy0921/IM/schemas/kafka/delivery/v1"
	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
	"google.golang.org/protobuf/proto"
)

func TestBuildCommandFromInboxItemCreated(t *testing.T) {
	value, err := proto.Marshal(&deliveryeventsv1.DeliveryEvent{
		EventId:          "delivery-event-1",
		EventType:        types.DeliveryEventInboxItemCreated,
		TenantId:         "tenant-1",
		AggregateId:      "conversation-1",
		AggregateVersion: 12,
		Payload: &deliveryeventsv1.DeliveryEvent_InboxItemCreated{
			InboxItemCreated: &deliveryeventsv1.DeliveryInboxItemCreatedV1{
				TenantId:        "tenant-1",
				UserId:          "receiver-1",
				ConversationId:  "conversation-1",
				ConversationSeq: 12,
				SourceEventId:   "timeline-event-1",
				SourceEventType: types.SourceEventMessageEdited,
				MessageId:       "message-1",
				SenderId:        "sender-1",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	command, err := buildCommand("receipt-test", types.DeliveryMessage{
		Topic:     "im.delivery.events",
		Partition: 3,
		Offset:    9,
		Value:     value,
	})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if command.UserID != "receiver-1" ||
		command.SenderID != "sender-1" ||
		command.MessageID != "message-1" ||
		command.SourceEventType != types.SourceEventMessageEdited ||
		command.OffsetValue != 10 {
		t.Fatalf("unexpected command: %+v", command)
	}
}

func TestBuildCommandFromAckRecorded(t *testing.T) {
	value, err := proto.Marshal(&deliveryeventsv1.DeliveryEvent{
		EventId:     "delivery-ack-1",
		EventType:   types.DeliveryEventAckRecorded,
		TenantId:    "tenant-1",
		AggregateId: "conversation-1",
		Payload: &deliveryeventsv1.DeliveryEvent_AckRecorded{
			AckRecorded: &deliveryeventsv1.DeliveryAckRecordedV1{
				TenantId:        "tenant-1",
				UserId:          "receiver-1",
				DeviceId:        "device-1",
				ConversationId:  "conversation-1",
				LastReceivedSeq: 12,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	command, err := buildCommand("receipt-test", types.DeliveryMessage{Topic: "im.delivery.events", Value: value})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if command.DeviceID != "device-1" || command.LastReceivedSeq != 12 {
		t.Fatalf("unexpected command: %+v", command)
	}
}

func TestBuildCommandFromConversationSignal(t *testing.T) {
	value, err := proto.Marshal(&deliveryeventsv1.DeliveryEvent{
		EventId:     "delivery-signal-1",
		EventType:   types.DeliveryEventConversationSignal,
		TenantId:    "tenant-1",
		AggregateId: "conversation-1",
		Payload: &deliveryeventsv1.DeliveryEvent_ConversationSignal{
			ConversationSignal: &deliveryeventsv1.DeliveryConversationSignalV1{
				TenantId:        "tenant-1",
				ConversationId:  "conversation-1",
				ConversationSeq: 12,
				SourceEventId:   "timeline-event-1",
				SourceEventType: types.SourceEventMessagePersisted,
				MessageId:       "message-1",
				SenderId:        "sender-1",
				FanoutMode:      "READ_FANOUT",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	command, err := buildCommand("receipt-test", types.DeliveryMessage{
		Topic:     "im.delivery.events",
		Partition: 3,
		Offset:    9,
		Value:     value,
	})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if command.EventType != types.DeliveryEventConversationSignal ||
		command.UserID != "" ||
		command.ConversationSeq != 12 ||
		command.SourceEventID != "timeline-event-1" ||
		command.SourceEventType != types.SourceEventMessagePersisted ||
		command.OffsetValue != 10 {
		t.Fatalf("unexpected command: %+v", command)
	}
}

func TestWorkerRunRetriesAfterProjectorError(t *testing.T) {
	t.Parallel()

	consumer := &fakeConsumer{
		fetches: []fakeFetch{
			{message: deliveryMessageForTest(t, "delivery-event-retry-1")},
			{message: deliveryMessageForTest(t, "delivery-event-retry-1")},
			{err: context.Canceled},
		},
	}
	projector := &fakeProjector{errs: []error{errors.New("projection failed"), nil}}
	worker := NewWorker(
		consumer,
		projector,
		"receipt-test",
		Config{ErrorBackoff: time.Millisecond},
	)

	err := worker.Run(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if consumer.commitCount != 1 {
		t.Fatalf("expected one committed message, got %d", consumer.commitCount)
	}
	if projector.executeCount != 2 {
		t.Fatalf("expected projector retries, got %d", projector.executeCount)
	}
	snapshot := worker.Snapshot()
	if snapshot.TotalErrors != 1 || snapshot.ConsecutiveErrors != 0 {
		t.Fatalf("unexpected worker snapshot: %+v", snapshot)
	}
	if snapshot.LastErrorAtMS == 0 || snapshot.LastSuccessAtMS == 0 || snapshot.LastCommitAtMS == 0 {
		t.Fatalf("expected worker timestamps, got %+v", snapshot)
	}
	if snapshot.LastErrorBackoffMS != time.Millisecond.Milliseconds() {
		t.Fatalf("unexpected error backoff: %+v", snapshot)
	}
}

func TestWorkerRunFailsFastWhenProjectorMissing(t *testing.T) {
	t.Parallel()

	worker := NewWorker(&fakeConsumer{}, nil, "receipt-test")
	err := worker.Run(context.Background())
	if err == nil || err.Error() != "receipt delivery projector is not configured" {
		t.Fatalf("unexpected error: %v", err)
	}
}

type fakeConsumer struct {
	fetches     []fakeFetch
	fetchIndex  int
	commitCount int
}

type fakeFetch struct {
	message types.DeliveryMessage
	err     error
}

func (consumer *fakeConsumer) Fetch(context.Context) (types.DeliveryMessage, error) {
	if consumer.fetchIndex >= len(consumer.fetches) {
		return types.DeliveryMessage{}, context.Canceled
	}
	result := consumer.fetches[consumer.fetchIndex]
	consumer.fetchIndex++
	return result.message, result.err
}

func (consumer *fakeConsumer) Commit(context.Context, types.DeliveryMessage) error {
	consumer.commitCount++
	return nil
}

type fakeProjector struct {
	errs         []error
	executeCount int
}

func (projector *fakeProjector) Execute(context.Context, types.ProjectDeliveryEventCommand) (types.ProjectDeliveryEventResult, error) {
	projector.executeCount++
	if projector.executeCount <= len(projector.errs) {
		return types.ProjectDeliveryEventResult{}, projector.errs[projector.executeCount-1]
	}
	return types.ProjectDeliveryEventResult{}, nil
}

func deliveryMessageForTest(t *testing.T, eventID string) types.DeliveryMessage {
	t.Helper()
	value, err := proto.Marshal(&deliveryeventsv1.DeliveryEvent{
		EventId:          eventID,
		EventType:        types.DeliveryEventInboxItemCreated,
		TenantId:         "tenant-1",
		AggregateId:      "conversation-1",
		AggregateVersion: 12,
		Payload: &deliveryeventsv1.DeliveryEvent_InboxItemCreated{
			InboxItemCreated: &deliveryeventsv1.DeliveryInboxItemCreatedV1{
				TenantId:        "tenant-1",
				UserId:          "receiver-1",
				ConversationId:  "conversation-1",
				ConversationSeq: 12,
				SourceEventId:   "timeline-event-1",
				SourceEventType: types.SourceEventMessagePersisted,
				MessageId:       "message-1",
				SenderId:        "sender-1",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return types.DeliveryMessage{
		Topic:     "im.delivery.events",
		Partition: 3,
		Offset:    9,
		Value:     value,
	}
}
