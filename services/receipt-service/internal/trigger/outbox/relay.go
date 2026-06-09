package outbox

import (
	"context"
	"encoding/json"
	"errors"
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
}

type Config struct {
	Topic          string
	BatchSize      int
	PollInterval   time.Duration
	MaxAttempts    int
	RetryBaseDelay time.Duration
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
		stats, err := relay.RunOnce(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		if stats.Published > 0 {
			continue
		}
		timer := time.NewTimer(relay.config.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
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
	return config
}
