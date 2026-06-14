package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	receipteventsv1 "github.com/qsyy0921/IM/schemas/kafka/receipt/v1"
	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const TopicReceiptEvents = "im.receipt.events"

type Store interface {
	ProcessReadyBatch(
		ctx context.Context,
		limit int,
		maxAttempts int,
		retryBaseDelay time.Duration,
		publish func(context.Context, []types.OutboxMessage) []error,
	) (types.OutboxRelayStats, error)
}

type Publisher interface {
	PublishBatch(ctx context.Context, topic string, records []types.KafkaPublishRecord) error
}

type Relay struct {
	store     Store
	publisher Publisher
	config    Config
	metrics   relayMetrics
}

type Config struct {
	Topic          string
	BatchSize      int
	PollInterval   time.Duration
	MaxAttempts    int
	RetryBaseDelay time.Duration
	ErrorBackoff   time.Duration
	Logf           func(format string, args ...any)
}

type relayMetrics struct {
	totalErrors        atomic.Uint64
	consecutiveErrors  atomic.Uint64
	lastErrorAtMS      atomic.Int64
	lastSuccessAtMS    atomic.Int64
	lastPublishedAtMS  atomic.Int64
	lastErrorBackoffMS atomic.Int64
}

func NewRelay(store Store, publisher Publisher, config Config) *Relay {
	return &Relay{
		store:     store,
		publisher: publisher,
		config:    normalizeConfig(config),
	}
}

func (relay *Relay) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		stats, err := relay.RunOnce(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			if relay.config.Logf != nil {
				relay.config.Logf("receipt-service outbox relay retrying after runtime error: %v", err)
			}
			relay.recordError()
			relay.metrics.lastErrorBackoffMS.Store(relay.config.ErrorBackoff.Milliseconds())
			if err := waitForInterval(ctx, relay.config.ErrorBackoff); err != nil {
				return err
			}
			continue
		}
		relay.recordSuccess(stats)
		if stats.Published > 0 {
			continue
		}
		if err := waitForInterval(ctx, relay.config.PollInterval); err != nil {
			return err
		}
	}
}

func (relay *Relay) Snapshot() types.OutboxRelayWorkerSnapshot {
	return types.OutboxRelayWorkerSnapshot{
		TotalErrors:        relay.metrics.totalErrors.Load(),
		ConsecutiveErrors:  relay.metrics.consecutiveErrors.Load(),
		LastErrorAtMS:      relay.metrics.lastErrorAtMS.Load(),
		LastSuccessAtMS:    relay.metrics.lastSuccessAtMS.Load(),
		LastPublishedAtMS:  relay.metrics.lastPublishedAtMS.Load(),
		LastErrorBackoffMS: relay.metrics.lastErrorBackoffMS.Load(),
	}
}

func (relay *Relay) RunOnce(ctx context.Context) (types.OutboxRelayStats, error) {
	if relay == nil || relay.store == nil {
		return types.OutboxRelayStats{}, errors.New("receipt outbox relay store is not configured")
	}
	if relay.publisher == nil {
		return types.OutboxRelayStats{}, errors.New("receipt outbox relay publisher is not configured")
	}
	return relay.store.ProcessReadyBatch(
		ctx,
		relay.config.BatchSize,
		relay.config.MaxAttempts,
		relay.config.RetryBaseDelay,
		relay.publishMessages,
	)
}

func (relay *Relay) publishMessages(ctx context.Context, messages []types.OutboxMessage) []error {
	errs := make([]error, len(messages))
	records := make([]types.KafkaPublishRecord, 0, len(messages))
	indexes := make([]int, 0, len(messages))
	for index, message := range messages {
		value, err := BuildKafkaValue(message)
		if err != nil {
			errs[index] = err
			continue
		}
		records = append(records, types.KafkaPublishRecord{
			Key:   []byte(message.PartitionKey),
			Value: value,
		})
		indexes = append(indexes, index)
	}
	if len(records) == 0 {
		return errs
	}
	if err := relay.publisher.PublishBatch(ctx, relay.config.Topic, records); err != nil {
		for _, index := range indexes {
			errs[index] = err
		}
	}
	return errs
}

func BuildKafkaValue(message types.OutboxMessage) ([]byte, error) {
	event, err := BuildReceiptEvent(message)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(event)
}

func BuildReceiptEvent(message types.OutboxMessage) (*receipteventsv1.ReceiptEvent, error) {
	event := &receipteventsv1.ReceiptEvent{
		EventId:          message.EventID,
		EventType:        message.EventType,
		EventVersion:     message.EventVersion,
		TenantId:         string(message.TenantID),
		AggregateType:    "receipt",
		AggregateId:      string(message.ConversationID),
		AggregateVersion: message.AggregateVersion,
		PartitionKey:     message.PartitionKey,
		MappingVersion:   message.MappingVersion,
		TraceId:          message.TraceID,
		CorrelationId:    message.CorrelationID,
		CausationId:      message.CausationID,
		Producer:         message.Producer,
		OccurredAt:       timestamppb.New(message.OccurredAt),
	}

	switch message.EventType {
	case types.ReceiptEventMessageReceived:
		payload, err := decodeReceiptPayload(message.PayloadJSON)
		if err != nil {
			return nil, err
		}
		event.Payload = &receipteventsv1.ReceiptEvent_MessageReceived{
			MessageReceived: &receipteventsv1.ReceiptMessageReceivedV1{
				TenantId:        payload.TenantID,
				ConversationId:  payload.ConversationID,
				ConversationSeq: payload.ConversationSeq,
				MessageId:       payload.MessageID,
				UserId:          payload.UserID,
				DeviceId:        payload.DeviceID,
				CursorSeq:       payload.CursorSeq,
				SourceEventId:   payload.SourceEventID,
			},
		}
		return event, nil
	case types.ReceiptEventMessageRead:
		payload, err := decodeReceiptPayload(message.PayloadJSON)
		if err != nil {
			return nil, err
		}
		event.Payload = &receipteventsv1.ReceiptEvent_MessageRead{
			MessageRead: &receipteventsv1.ReceiptMessageReadV1{
				TenantId:        payload.TenantID,
				ConversationId:  payload.ConversationID,
				ConversationSeq: payload.ConversationSeq,
				MessageId:       payload.MessageID,
				UserId:          payload.UserID,
				DeviceId:        payload.DeviceID,
				CursorSeq:       payload.CursorSeq,
				SourceEventId:   payload.SourceEventID,
			},
		}
		return event, nil
	default:
		return nil, errors.New("unsupported receipt outbox event type")
	}
}

type receiptPayload struct {
	TenantID        string `json:"tenant_id"`
	ConversationID  string `json:"conversation_id"`
	ConversationSeq int64  `json:"conversation_seq"`
	MessageID       string `json:"message_id"`
	UserID          string `json:"user_id"`
	DeviceID        string `json:"device_id"`
	CursorSeq       int64  `json:"cursor_seq"`
	SourceEventID   string `json:"source_event_id"`
}

func decodeReceiptPayload(payloadJSON []byte) (receiptPayload, error) {
	var payload receiptPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return receiptPayload{}, err
	}
	if payload.TenantID == "" ||
		payload.ConversationID == "" ||
		payload.ConversationSeq <= 0 ||
		payload.MessageID == "" ||
		payload.UserID == "" ||
		payload.DeviceID == "" ||
		payload.CursorSeq <= 0 ||
		payload.SourceEventID == "" {
		return receiptPayload{}, errors.New("receipt payload is incomplete")
	}
	return payload, nil
}

func normalizeConfig(config Config) Config {
	if config.Topic == "" {
		config.Topic = TopicReceiptEvents
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 500
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 5
	}
	if config.RetryBaseDelay <= 0 {
		config.RetryBaseDelay = time.Second
	}
	if config.ErrorBackoff <= 0 {
		config.ErrorBackoff = time.Second
	}
	return config
}

func (relay *Relay) recordError() {
	relay.metrics.totalErrors.Add(1)
	relay.metrics.consecutiveErrors.Add(1)
	relay.metrics.lastErrorAtMS.Store(time.Now().UnixMilli())
}

func (relay *Relay) recordSuccess(stats types.OutboxRelayStats) {
	relay.metrics.consecutiveErrors.Store(0)
	now := time.Now().UnixMilli()
	relay.metrics.lastSuccessAtMS.Store(now)
	if stats.Published > 0 {
		relay.metrics.lastPublishedAtMS.Store(now)
	}
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
