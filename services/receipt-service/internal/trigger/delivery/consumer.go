package delivery

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

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
	config        Config
	metrics       workerMetrics
}

type Config struct {
	ErrorBackoff time.Duration
	Logf         func(format string, args ...any)
}

type workerMetrics struct {
	totalErrors        atomic.Uint64
	consecutiveErrors  atomic.Uint64
	lastErrorAtMS      atomic.Int64
	lastSuccessAtMS    atomic.Int64
	lastCommitAtMS     atomic.Int64
	lastErrorBackoffMS atomic.Int64
}

func NewWorker(consumer Consumer, projector Projector, consumerGroup string, configs ...Config) *Worker {
	var config Config
	if len(configs) > 0 {
		config = configs[0]
	}
	return &Worker{
		consumer:      consumer,
		projector:     projector,
		consumerGroup: consumerGroup,
		config:        normalizeConfig(config),
	}
}

func (worker *Worker) Run(ctx context.Context) error {
	if worker.consumer == nil {
		return errors.New("receipt delivery consumer is not configured")
	}
	if worker.projector == nil {
		return errors.New("receipt delivery projector is not configured")
	}
	for {
		err := worker.RunOnce(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			if worker.config.Logf != nil {
				worker.config.Logf("receipt-service delivery projection worker retrying after error: %v", err)
			}
			worker.recordError()
			worker.metrics.lastErrorBackoffMS.Store(worker.config.ErrorBackoff.Milliseconds())
			if err := waitForInterval(ctx, worker.config.ErrorBackoff); err != nil {
				return err
			}
			continue
		}
		worker.recordSuccess()
	}
}

func (worker *Worker) RunOnce(ctx context.Context) error {
	if worker.consumer == nil {
		return errors.New("receipt delivery consumer is not configured")
	}
	if worker.projector == nil {
		return errors.New("receipt delivery projector is not configured")
	}
	message, err := worker.consumer.Fetch(ctx)
	if err != nil {
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
	return nil
}

func (worker *Worker) Snapshot() types.ProjectionWorkerSnapshot {
	return types.ProjectionWorkerSnapshot{
		TotalErrors:        worker.metrics.totalErrors.Load(),
		ConsecutiveErrors:  worker.metrics.consecutiveErrors.Load(),
		LastErrorAtMS:      worker.metrics.lastErrorAtMS.Load(),
		LastSuccessAtMS:    worker.metrics.lastSuccessAtMS.Load(),
		LastCommitAtMS:     worker.metrics.lastCommitAtMS.Load(),
		LastErrorBackoffMS: worker.metrics.lastErrorBackoffMS.Load(),
	}
}

func normalizeConfig(config Config) Config {
	if config.ErrorBackoff <= 0 {
		config.ErrorBackoff = time.Second
	}
	return config
}

func (worker *Worker) recordError() {
	worker.metrics.totalErrors.Add(1)
	worker.metrics.consecutiveErrors.Add(1)
	worker.metrics.lastErrorAtMS.Store(time.Now().UnixMilli())
}

func (worker *Worker) recordSuccess() {
	worker.metrics.consecutiveErrors.Store(0)
	now := time.Now().UnixMilli()
	worker.metrics.lastSuccessAtMS.Store(now)
	worker.metrics.lastCommitAtMS.Store(now)
}

func waitForInterval(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
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
	case *deliveryeventsv1.DeliveryEvent_ConversationSignal:
		fillConversationSignal(&command, payload.ConversationSignal)
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

func fillConversationSignal(command *types.ProjectDeliveryEventCommand, payload *deliveryeventsv1.DeliveryConversationSignalV1) {
	if payload == nil {
		return
	}
	command.ConversationSeq = payload.GetConversationSeq()
	command.SourceEventID = payload.GetSourceEventId()
	command.SourceEventType = payload.GetSourceEventType()
	command.MessageID = payload.GetMessageId()
	command.SenderID = types.UserID(payload.GetSenderId())
}
