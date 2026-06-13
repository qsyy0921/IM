package contact

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	contacteventsv1 "github.com/qsyy0921/IM/schemas/kafka/contacts/v1"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
	"google.golang.org/protobuf/proto"
)

const TopicContactEvents = "im.contact.events"

type Consumer interface {
	Fetch(context.Context) (types.ContactMessage, error)
	Commit(context.Context, types.ContactMessage) error
}

type Projector interface {
	Execute(context.Context, types.ProjectContactEventCommand) (types.ProjectContactEventResult, error)
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
		return errors.New("policy contact consumer is not configured")
	}
	if worker.projector == nil {
		return errors.New("policy contact projector is not configured")
	}
	for {
		err := worker.RunOnce(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			if worker.config.Logf != nil {
				worker.config.Logf("policy-service contact projection worker retrying after error: %v", err)
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
		return errors.New("policy contact consumer is not configured")
	}
	if worker.projector == nil {
		return errors.New("policy contact projector is not configured")
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

func buildCommand(consumerGroup string, message types.ContactMessage) (types.ProjectContactEventCommand, error) {
	var event contacteventsv1.ContactEvent
	if err := proto.Unmarshal(message.Value, &event); err != nil {
		return types.ProjectContactEventCommand{}, err
	}
	command := types.ProjectContactEventCommand{
		TenantID:      types.TenantID(event.GetTenantId()),
		EventID:       event.GetEventId(),
		EventType:     event.GetEventType(),
		ConsumerGroup: consumerGroup,
		Topic:         message.Topic,
		PartitionID:   int32(message.Partition),
		OffsetValue:   message.Offset + 1,
		TraceID:       event.GetTraceId(),
		CorrelationID: event.GetCorrelationId(),
		CausationID:   event.GetCausationId(),
	}
	switch payload := event.GetPayload().(type) {
	case *contacteventsv1.ContactEvent_RequestCreated:
		fillContactRequest(&command, payload.RequestCreated.GetSenderUserId(), payload.RequestCreated.GetReceiverUserId(), payload.RequestCreated.GetStatus())
	case *contacteventsv1.ContactEvent_RequestAccepted:
		fillContactRequest(&command, payload.RequestAccepted.GetSenderUserId(), payload.RequestAccepted.GetReceiverUserId(), payload.RequestAccepted.GetStatus())
		command.EdgeVersion = payload.RequestAccepted.GetEdgeVersion()
	case *contacteventsv1.ContactEvent_RequestDeclined:
		fillContactRequest(&command, payload.RequestDeclined.GetSenderUserId(), payload.RequestDeclined.GetReceiverUserId(), payload.RequestDeclined.GetStatus())
	case *contacteventsv1.ContactEvent_RequestCanceled:
		fillContactRequest(&command, payload.RequestCanceled.GetSenderUserId(), payload.RequestCanceled.GetReceiverUserId(), payload.RequestCanceled.GetStatus())
	case *contacteventsv1.ContactEvent_EdgeDeleted:
		fillContactEdge(&command, payload.EdgeDeleted.GetOwnerUserId(), payload.EdgeDeleted.GetContactUserId(), payload.EdgeDeleted.GetStatus(), payload.EdgeDeleted.GetEdgeVersion())
	case *contacteventsv1.ContactEvent_EdgeBlocked:
		fillContactEdge(&command, payload.EdgeBlocked.GetOwnerUserId(), payload.EdgeBlocked.GetContactUserId(), payload.EdgeBlocked.GetStatus(), payload.EdgeBlocked.GetEdgeVersion())
	case *contacteventsv1.ContactEvent_EdgeUnblocked:
		fillContactEdge(&command, payload.EdgeUnblocked.GetOwnerUserId(), payload.EdgeUnblocked.GetContactUserId(), payload.EdgeUnblocked.GetStatus(), payload.EdgeUnblocked.GetEdgeVersion())
	case *contacteventsv1.ContactEvent_EdgeRemarkUpdated:
		fillContactEdge(&command, payload.EdgeRemarkUpdated.GetOwnerUserId(), payload.EdgeRemarkUpdated.GetContactUserId(), payload.EdgeRemarkUpdated.GetStatus(), payload.EdgeRemarkUpdated.GetEdgeVersion())
	default:
		return types.ProjectContactEventCommand{}, types.NewInvalidArgument("unsupported contact event payload")
	}
	return command, nil
}

func fillContactRequest(command *types.ProjectContactEventCommand, sender string, receiver string, status string) {
	command.SenderUserID = types.UserID(sender)
	command.ReceiverUserID = types.UserID(receiver)
	command.Status = status
}

func fillContactEdge(command *types.ProjectContactEventCommand, owner string, contact string, status string, version int64) {
	command.OwnerUserID = types.UserID(owner)
	command.ContactUserID = types.UserID(contact)
	command.Status = status
	command.EdgeVersion = version
}
