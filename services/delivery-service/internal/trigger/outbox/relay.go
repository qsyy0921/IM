package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	deliveryeventsv1 "github.com/qsyy0921/IM/schemas/kafka/delivery/v1"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const TopicDeliveryEvents = "im.delivery.events"

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
		return types.OutboxRelayStats{}, errors.New("delivery outbox relay store is not configured")
	}
	if relay.publisher == nil {
		return types.OutboxRelayStats{}, errors.New("delivery outbox relay publisher is not configured")
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
	event, err := BuildDeliveryEvent(message)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(event)
}

func BuildDeliveryEvent(message types.OutboxMessage) (*deliveryeventsv1.DeliveryEvent, error) {
	event := &deliveryeventsv1.DeliveryEvent{
		EventId:          message.EventID,
		EventType:        message.EventType,
		EventVersion:     message.EventVersion,
		TenantId:         string(message.TenantID),
		AggregateType:    "delivery",
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
	case types.DeliveryEventInboxItemCreated:
		payload, err := decodeInboxItemCreatedPayload(message.PayloadJSON)
		if err != nil {
			return nil, err
		}
		event.Payload = &deliveryeventsv1.DeliveryEvent_InboxItemCreated{
			InboxItemCreated: &deliveryeventsv1.DeliveryInboxItemCreatedV1{
				TenantId:        payload.TenantID,
				UserId:          payload.UserID,
				ConversationId:  payload.ConversationID,
				ConversationSeq: payload.ConversationSeq,
				SourceEventId:   payload.SourceEventID,
				MessageId:       payload.MessageID,
				SenderId:        payload.SenderID,
			},
		}
		return event, nil
	case types.DeliveryEventAckRecorded:
		payload, err := decodeAckRecordedPayload(message.PayloadJSON)
		if err != nil {
			return nil, err
		}
		event.Payload = &deliveryeventsv1.DeliveryEvent_AckRecorded{
			AckRecorded: &deliveryeventsv1.DeliveryAckRecordedV1{
				TenantId:        payload.TenantID,
				UserId:          payload.UserID,
				DeviceId:        payload.DeviceID,
				ConversationId:  payload.ConversationID,
				LastReceivedSeq: payload.LastReceivedSeq,
			},
		}
		return event, nil
	default:
		return nil, errors.New("unsupported delivery outbox event type")
	}
}

type inboxItemCreatedPayload struct {
	TenantID        string `json:"tenant_id"`
	UserID          string `json:"user_id"`
	ConversationID  string `json:"conversation_id"`
	ConversationSeq int64  `json:"conversation_seq"`
	SourceEventID   string `json:"source_event_id"`
	LegacyEventID   string `json:"event_id"`
	MessageID       string `json:"message_id"`
	SenderID        string `json:"sender_id"`
}

func decodeInboxItemCreatedPayload(payloadJSON []byte) (inboxItemCreatedPayload, error) {
	var payload inboxItemCreatedPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return inboxItemCreatedPayload{}, err
	}
	if payload.SourceEventID == "" {
		payload.SourceEventID = payload.LegacyEventID
	}
	if payload.TenantID == "" ||
		payload.UserID == "" ||
		payload.ConversationID == "" ||
		payload.ConversationSeq <= 0 ||
		payload.SourceEventID == "" ||
		payload.MessageID == "" ||
		payload.SenderID == "" {
		return inboxItemCreatedPayload{}, errors.New("delivery inbox item payload is incomplete")
	}
	return payload, nil
}

type ackRecordedPayload struct {
	TenantID        string `json:"tenant_id"`
	UserID          string `json:"user_id"`
	DeviceID        string `json:"device_id"`
	ConversationID  string `json:"conversation_id"`
	LastReceivedSeq int64  `json:"last_received_seq"`
}

func decodeAckRecordedPayload(payloadJSON []byte) (ackRecordedPayload, error) {
	var payload ackRecordedPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return ackRecordedPayload{}, err
	}
	if payload.TenantID == "" ||
		payload.UserID == "" ||
		payload.DeviceID == "" ||
		payload.ConversationID == "" ||
		payload.LastReceivedSeq <= 0 {
		return ackRecordedPayload{}, errors.New("delivery ack payload is incomplete")
	}
	return payload, nil
}

func normalizeConfig(config Config) Config {
	if config.Topic == "" {
		config.Topic = TopicDeliveryEvents
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
