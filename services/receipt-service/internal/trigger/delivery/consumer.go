package delivery

import (
	"context"
	"errors"

	deliveryeventsv1 "github.com/qsyy0921/IM/schemas/kafka/delivery/v1"
	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
	"google.golang.org/protobuf/proto"
)

type Consumer interface {
	Fetch(context.Context) (types.DeliveryMessage, error)
	Commit(context.Context, types.DeliveryMessage) error
}

type Projector interface {
	Execute(context.Context, types.ProjectDeliveryEventCommand) (types.ProjectDeliveryEventResult, error)
}

type Worker struct {
	consumer      Consumer
	projector     Projector
	consumerGroup string
}

func NewWorker(consumer Consumer, projector Projector, consumerGroup string) *Worker {
	return &Worker{consumer: consumer, projector: projector, consumerGroup: consumerGroup}
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
		command, err := buildCommand(worker.consumerGroup, message)
		if err != nil {
			return err
		}
		if _, err := worker.projector.Execute(ctx, command); err != nil {
			return err
		}
		if err := worker.consumer.Commit(ctx, message); err != nil {
			return err
		}
	}
}

func buildCommand(consumerGroup string, message types.DeliveryMessage) (types.ProjectDeliveryEventCommand, error) {
	var event deliveryeventsv1.DeliveryEvent
	if err := proto.Unmarshal(message.Value, &event); err != nil {
		return types.ProjectDeliveryEventCommand{}, err
	}
	command := types.ProjectDeliveryEventCommand{
		TenantID:       types.TenantID(event.GetTenantId()),
		EventID:        event.GetEventId(),
		EventType:      event.GetEventType(),
		ConversationID: types.ConversationID(event.GetAggregateId()),
		ConsumerGroup:  consumerGroup,
		Topic:          message.Topic,
		PartitionID:    int32(message.Partition),
		OffsetValue:    message.Offset + 1,
		TraceID:        event.GetTraceId(),
		CorrelationID:  event.GetCorrelationId(),
		CausationID:    event.GetCausationId(),
	}
	switch payload := event.GetPayload().(type) {
	case *deliveryeventsv1.DeliveryEvent_InboxItemCreated:
		fillInboxItemCreated(&command, payload.InboxItemCreated)
	case *deliveryeventsv1.DeliveryEvent_AckRecorded:
		fillAckRecorded(&command, payload.AckRecorded)
	default:
		return types.ProjectDeliveryEventCommand{}, types.NewInvalidArgument("unsupported delivery payload")
	}
	return command, nil
}

func fillInboxItemCreated(command *types.ProjectDeliveryEventCommand, payload *deliveryeventsv1.DeliveryInboxItemCreatedV1) {
	if payload == nil {
		return
	}
	command.UserID = types.UserID(payload.GetUserId())
	command.ConversationSeq = payload.GetConversationSeq()
	command.SourceEventID = payload.GetSourceEventId()
	command.SourceEventType = payload.GetSourceEventType()
	if command.SourceEventType == "" {
		command.SourceEventType = types.SourceEventMessagePersisted
	}
	command.MessageID = payload.GetMessageId()
	command.SenderID = types.UserID(payload.GetSenderId())
}

func fillAckRecorded(command *types.ProjectDeliveryEventCommand, payload *deliveryeventsv1.DeliveryAckRecordedV1) {
	if payload == nil {
		return
	}
	command.UserID = types.UserID(payload.GetUserId())
	command.DeviceID = payload.GetDeviceId()
	command.LastReceivedSeq = payload.GetLastReceivedSeq()
}
