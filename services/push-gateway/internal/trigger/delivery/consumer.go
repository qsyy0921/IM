package delivery

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	deliveryeventsv1 "github.com/qsyy0921/IM/schemas/kafka/delivery/v1"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
	"google.golang.org/protobuf/proto"
)

const (
	TopicDeliveryEvents         = "im.delivery.events"
	EventInboxItemCreatedV1     = "delivery.inbox_item.created.v1"
	EventDeliveryAckRecordedV1  = "delivery.ack.recorded.v1"
	EventInboxItemHiddenV1      = "delivery.inbox_item.hidden.v1"
	EventConversationSignalV1   = "delivery.conversation.signal.v1"
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
	config   Config
	metrics  workerMetrics
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

func NewWorker(consumer Consumer, notifier Notifier, configs ...Config) *Worker {
	var config Config
	if len(configs) > 0 {
		config = configs[0]
	}
	return &Worker{consumer: consumer, notifier: notifier, config: normalizeConfig(config)}
}

func (worker *Worker) Run(ctx context.Context) error {
	if worker.consumer == nil {
		return errors.New("push delivery consumer is not configured")
	}
	if worker.notifier == nil {
		return errors.New("push delivery notifier is not configured")
	}
	for {
		err := worker.RunOnce(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			if isPermanentError(err) {
				return err
			}
			if worker.config.Logf != nil {
				worker.config.Logf("push-gateway delivery consumer retrying after error: %v", err)
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
		return errors.New("push delivery consumer is not configured")
	}
	if worker.notifier == nil {
		return errors.New("push delivery notifier is not configured")
	}
	message, err := worker.consumer.Fetch(ctx)
	if err != nil {
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
	return nil
}

func (worker *Worker) Snapshot() types.ConsumerWorkerSnapshot {
	return types.ConsumerWorkerSnapshot{
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

func isPermanentError(err error) bool {
	return errors.Is(err, types.ErrInvalidFrame) ||
		errors.Is(err, types.ErrUnsupportedDeliveryEvent)
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
	case *deliveryeventsv1.DeliveryEvent_InboxItemHidden:
		if event.GetEventType() != EventInboxItemHiddenV1 {
			return types.NotifyDeliveryCommand{}, false, types.NewInvalidFrame("delivery event type mismatch")
		}
		hidden := payload.InboxItemHidden
		if hidden == nil {
			return types.NotifyDeliveryCommand{}, false, types.NewInvalidFrame("empty inbox item hidden payload")
		}
		if hidden.GetTenantId() == "" ||
			hidden.GetUserId() == "" ||
			hidden.GetConversationId() == "" ||
			hidden.GetConversationSeq() <= 0 {
			return types.NotifyDeliveryCommand{}, false, types.NewInvalidFrame("inbox item hidden payload is incomplete")
		}
		if event.GetTenantId() != hidden.GetTenantId() || event.GetAggregateId() != hidden.GetConversationId() {
			return types.NotifyDeliveryCommand{}, false, types.NewInvalidFrame("delivery event envelope mismatch")
		}
		return types.NotifyDeliveryCommand{
			Notification: types.DeliveryNotification{
				Kind:            types.DeliveryNotificationKindInboxItemHidden,
				EventID:         event.GetEventId(),
				TenantID:        hidden.GetTenantId(),
				UserID:          hidden.GetUserId(),
				ConversationID:  hidden.GetConversationId(),
				ConversationSeq: hidden.GetConversationSeq(),
				SourceEventID:   event.GetCausationId(),
				SourceEventType: event.GetEventType(),
				MessageID:       hidden.GetMessageId(),
				CorrelationID:   event.GetCorrelationId(),
			},
		}, true, nil
	case *deliveryeventsv1.DeliveryEvent_ConversationSignal:
		if event.GetEventType() != EventConversationSignalV1 {
			return types.NotifyDeliveryCommand{}, false, types.NewInvalidFrame("delivery event type mismatch")
		}
		signal := payload.ConversationSignal
		if signal == nil {
			return types.NotifyDeliveryCommand{}, false, types.NewInvalidFrame("empty conversation signal payload")
		}
		if signal.GetTenantId() == "" ||
			signal.GetConversationId() == "" ||
			signal.GetConversationSeq() <= 0 ||
			signal.GetSourceEventId() == "" ||
			signal.GetSourceEventType() == "" ||
			signal.GetFanoutMode() == "" {
			return types.NotifyDeliveryCommand{}, false, types.NewInvalidFrame("conversation signal payload is incomplete")
		}
		if event.GetTenantId() != signal.GetTenantId() || event.GetAggregateId() != signal.GetConversationId() {
			return types.NotifyDeliveryCommand{}, false, types.NewInvalidFrame("delivery event envelope mismatch")
		}
		return types.NotifyDeliveryCommand{
			Notification: types.DeliveryNotification{
				Kind:            types.DeliveryNotificationKindConversationSignal,
				EventID:         event.GetEventId(),
				TenantID:        signal.GetTenantId(),
				ConversationID:  signal.GetConversationId(),
				ConversationSeq: signal.GetConversationSeq(),
				SourceEventID:   signal.GetSourceEventId(),
				SourceEventType: signal.GetSourceEventType(),
				MessageID:       signal.GetMessageId(),
				CorrelationID:   event.GetCorrelationId(),
			},
		}, true, nil
	default:
		return types.NotifyDeliveryCommand{}, false, types.ErrUnsupportedDeliveryEvent
	}
}
