package delivery

import (
	"context"
	"errors"

	deliveryeventsv1 "github.com/qsyy0921/IM/schemas/kafka/delivery/v1"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
	"google.golang.org/protobuf/proto"
)

const (
	TopicDeliveryEvents         = "im.delivery.events"
	EventInboxItemCreatedV1     = "delivery.inbox_item.created.v1"
	EventDeliveryAckRecordedV1  = "delivery.ack.recorded.v1"
	SourceEventMessagePersisted = "message.persisted.v1"
)

type Consumer interface {
	Fetch(context.Context) (types.DeliveryEventMessage, error)
	Commit(context.Context, types.DeliveryEventMessage) error
}

type Notifier interface {
	Execute(context.Context, types.NotifyDeliveryCommand) (types.NotifyDeliveryResult, error)
}

type Worker struct {
	consumer Consumer
	notifier Notifier
}

func NewWorker(consumer Consumer, notifier Notifier) *Worker {
	return &Worker{consumer: consumer, notifier: notifier}
}

func (worker *Worker) Run(ctx context.Context) error {
	for {
		message, err := worker.consumer.Fetch(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			return err
		}
		command, shouldNotify, err := buildCommand(message)
		if err != nil {
			return err
		}
		if shouldNotify {
			if _, err := worker.notifier.Execute(ctx, command); err != nil {
				return err
			}
		}
		if err := worker.consumer.Commit(ctx, message); err != nil {
			return err
		}
	}
}

func buildCommand(message types.DeliveryEventMessage) (types.NotifyDeliveryCommand, bool, error) {
	var event deliveryeventsv1.DeliveryEvent
	if err := proto.Unmarshal(message.Value, &event); err != nil {
		return types.NotifyDeliveryCommand{}, false, err
	}
	if event.GetEventId() == "" ||
		event.GetEventVersion() == "" ||
		event.GetTenantId() == "" ||
		event.GetAggregateId() == "" ||
		event.GetPartitionKey() == "" {
		return types.NotifyDeliveryCommand{}, false, types.NewInvalidFrame("delivery event envelope is incomplete")
	}
	switch payload := event.GetPayload().(type) {
	case *deliveryeventsv1.DeliveryEvent_InboxItemCreated:
		if event.GetEventType() != EventInboxItemCreatedV1 {
			return types.NotifyDeliveryCommand{}, false, types.NewInvalidFrame("delivery event type mismatch")
		}
		created := payload.InboxItemCreated
		if created == nil {
			return types.NotifyDeliveryCommand{}, false, types.NewInvalidFrame("empty inbox item payload")
		}
		if created.GetTenantId() == "" ||
			created.GetUserId() == "" ||
			created.GetConversationId() == "" ||
			created.GetConversationSeq() <= 0 ||
			created.GetSourceEventId() == "" {
			return types.NotifyDeliveryCommand{}, false, types.NewInvalidFrame("inbox item payload is incomplete")
		}
		if event.GetTenantId() != created.GetTenantId() || event.GetAggregateId() != created.GetConversationId() {
			return types.NotifyDeliveryCommand{}, false, types.NewInvalidFrame("delivery event envelope mismatch")
		}
		sourceEventType := created.GetSourceEventType()
		if sourceEventType == "" {
			sourceEventType = SourceEventMessagePersisted
		}
		return types.NotifyDeliveryCommand{
			Notification: types.DeliveryNotification{
				EventID:         event.GetEventId(),
				TenantID:        created.GetTenantId(),
				UserID:          created.GetUserId(),
				ConversationID:  created.GetConversationId(),
				ConversationSeq: created.GetConversationSeq(),
				SourceEventID:   created.GetSourceEventId(),
				SourceEventType: sourceEventType,
				MessageID:       created.GetMessageId(),
				CorrelationID:   event.GetCorrelationId(),
			},
		}, true, nil
	case *deliveryeventsv1.DeliveryEvent_AckRecorded:
		if event.GetEventType() != EventDeliveryAckRecordedV1 {
			return types.NotifyDeliveryCommand{}, false, types.NewInvalidFrame("delivery event type mismatch")
		}
		return types.NotifyDeliveryCommand{}, false, nil
	default:
		return types.NotifyDeliveryCommand{}, false, types.ErrUnsupportedDeliveryEvent
	}
}
