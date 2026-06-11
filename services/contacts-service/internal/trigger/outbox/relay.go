package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	contacteventsv1 "github.com/qsyy0921/IM/schemas/kafka/contacts/v1"
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const TopicContactEvents = "im.contact.events"

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
		return types.OutboxRelayStats{}, errors.New("contacts outbox relay store is not configured")
	}
	if relay.publisher == nil {
		return types.OutboxRelayStats{}, errors.New("contacts outbox relay publisher is not configured")
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
	event, err := BuildContactEvent(message)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(event)
}

func BuildContactEvent(message types.OutboxMessage) (*contacteventsv1.ContactEvent, error) {
	if message.EventID == "" ||
		message.EventType == "" ||
		message.EventVersion == "" ||
		message.TenantID == "" ||
		message.AggregateType == "" ||
		message.AggregateID == "" ||
		message.AggregateVersion <= 0 ||
		message.PartitionKey == "" ||
		message.MappingVersion <= 0 ||
		message.Producer == "" {
		return nil, errors.New("contacts outbox envelope is incomplete")
	}
	event := &contacteventsv1.ContactEvent{
		EventId:          message.EventID,
		EventType:        message.EventType,
		EventVersion:     message.EventVersion,
		TenantId:         string(message.TenantID),
		AggregateType:    message.AggregateType,
		AggregateId:      message.AggregateID,
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
	case types.ContactEventRequestCreated:
		payload, err := decodeContactRequestPayload(message.PayloadJSON)
		if err != nil {
			return nil, err
		}
		event.Payload = &contacteventsv1.ContactEvent_RequestCreated{
			RequestCreated: &contacteventsv1.ContactRequestCreatedV1{
				TenantId:       payload.TenantID,
				RequestId:      payload.RequestID,
				SenderUserId:   payload.SenderUserID,
				ReceiverUserId: payload.ReceiverUserID,
				Status:         payload.Status,
				Message:        payload.Message,
				OccurredAt:     payload.Timestamp(),
			},
		}
		return event, nil
	case types.ContactEventRequestAccepted:
		payload, err := decodeContactResponsePayload(message.PayloadJSON, true)
		if err != nil {
			return nil, err
		}
		event.Payload = &contacteventsv1.ContactEvent_RequestAccepted{
			RequestAccepted: &contacteventsv1.ContactRequestAcceptedV1{
				TenantId:       payload.TenantID,
				RequestId:      payload.RequestID,
				SenderUserId:   payload.SenderUserID,
				ReceiverUserId: payload.ReceiverUserID,
				Status:         payload.Status,
				EdgeVersion:    payload.EdgeVersion,
				OccurredAt:     payload.Timestamp(),
			},
		}
		return event, nil
	case types.ContactEventRequestDeclined:
		payload, err := decodeContactResponsePayload(message.PayloadJSON, false)
		if err != nil {
			return nil, err
		}
		event.Payload = &contacteventsv1.ContactEvent_RequestDeclined{
			RequestDeclined: &contacteventsv1.ContactRequestDeclinedV1{
				TenantId:       payload.TenantID,
				RequestId:      payload.RequestID,
				SenderUserId:   payload.SenderUserID,
				ReceiverUserId: payload.ReceiverUserID,
				Status:         payload.Status,
				OccurredAt:     payload.Timestamp(),
			},
		}
		return event, nil
	case types.ContactEventEdgeDeleted:
		payload, err := decodeContactEdgePayload(message.PayloadJSON, true)
		if err != nil {
			return nil, err
		}
		event.Payload = &contacteventsv1.ContactEvent_EdgeDeleted{
			EdgeDeleted: &contacteventsv1.ContactEdgeDeletedV1{
				TenantId:       payload.TenantID,
				OwnerUserId:    payload.OwnerUserID,
				ContactUserId:  payload.ContactUserID,
				PreviousStatus: payload.PreviousStatus,
				Status:         payload.Status,
				EdgeVersion:    payload.EdgeVersion,
				OccurredAt:     payload.Timestamp(),
			},
		}
		return event, nil
	case types.ContactEventEdgeBlocked:
		payload, err := decodeContactEdgePayload(message.PayloadJSON, true)
		if err != nil {
			return nil, err
		}
		event.Payload = &contacteventsv1.ContactEvent_EdgeBlocked{
			EdgeBlocked: &contacteventsv1.ContactEdgeBlockedV1{
				TenantId:       payload.TenantID,
				OwnerUserId:    payload.OwnerUserID,
				ContactUserId:  payload.ContactUserID,
				PreviousStatus: payload.PreviousStatus,
				Status:         payload.Status,
				EdgeVersion:    payload.EdgeVersion,
				Reason:         payload.Reason,
				OccurredAt:     payload.Timestamp(),
			},
		}
		return event, nil
	case types.ContactEventRemarkUpdated:
		payload, err := decodeContactEdgePayload(message.PayloadJSON, false)
		if err != nil {
			return nil, err
		}
		event.Payload = &contacteventsv1.ContactEvent_EdgeRemarkUpdated{
			EdgeRemarkUpdated: &contacteventsv1.ContactEdgeRemarkUpdatedV1{
				TenantId:      payload.TenantID,
				OwnerUserId:   payload.OwnerUserID,
				ContactUserId: payload.ContactUserID,
				Status:        payload.Status,
				EdgeVersion:   payload.EdgeVersion,
				Remark:        payload.Remark,
				OccurredAt:    payload.Timestamp(),
			},
		}
		return event, nil
	default:
		return nil, errors.New("unsupported contacts outbox event type")
	}
}

type contactPayload struct {
	TenantID       string `json:"tenant_id"`
	RequestID      string `json:"request_id"`
	SenderUserID   string `json:"sender_user_id"`
	ReceiverUserID string `json:"receiver_user_id"`
	Status         string `json:"status"`
	Message        string `json:"message"`
	EdgeVersion    int64  `json:"edge_version"`
	OccurredAt     string `json:"occurred_at"`
	OwnerUserID    string `json:"owner_user_id"`
	ContactUserID  string `json:"contact_user_id"`
	PreviousStatus string `json:"previous_status"`
	Reason         string `json:"reason"`
	Remark         string `json:"remark"`
}

func (payload contactPayload) Timestamp() *timestamppb.Timestamp {
	occurredAt, _ := time.Parse(time.RFC3339Nano, payload.OccurredAt)
	return timestamppb.New(occurredAt)
}

func decodeContactRequestPayload(payloadJSON []byte) (contactPayload, error) {
	var payload contactPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return contactPayload{}, err
	}
	if payload.TenantID == "" ||
		payload.RequestID == "" ||
		payload.SenderUserID == "" ||
		payload.ReceiverUserID == "" ||
		payload.Status == "" ||
		payload.OccurredAt == "" {
		return contactPayload{}, errors.New("contact request payload is incomplete")
	}
	if _, err := time.Parse(time.RFC3339Nano, payload.OccurredAt); err != nil {
		return contactPayload{}, errors.New("contact request payload occurred_at is invalid")
	}
	return payload, nil
}

func decodeContactResponsePayload(payloadJSON []byte, requireEdgeVersion bool) (contactPayload, error) {
	payload, err := decodeContactRequestPayload(payloadJSON)
	if err != nil {
		return contactPayload{}, err
	}
	if requireEdgeVersion && payload.EdgeVersion <= 0 {
		return contactPayload{}, errors.New("contact accepted payload is incomplete")
	}
	return payload, nil
}

func decodeContactEdgePayload(payloadJSON []byte, requirePreviousStatus bool) (contactPayload, error) {
	var payload contactPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return contactPayload{}, err
	}
	if payload.TenantID == "" ||
		payload.OwnerUserID == "" ||
		payload.ContactUserID == "" ||
		payload.Status == "" ||
		payload.EdgeVersion <= 0 ||
		payload.OccurredAt == "" {
		return contactPayload{}, errors.New("contact edge payload is incomplete")
	}
	if requirePreviousStatus && payload.PreviousStatus == "" {
		return contactPayload{}, errors.New("contact edge payload previous_status is incomplete")
	}
	if _, err := time.Parse(time.RFC3339Nano, payload.OccurredAt); err != nil {
		return contactPayload{}, errors.New("contact edge payload occurred_at is invalid")
	}
	return payload, nil
}

func normalizeConfig(config Config) Config {
	if config.Topic == "" {
		config.Topic = TopicContactEvents
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
