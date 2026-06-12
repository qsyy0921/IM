package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	identityeventsv1 "github.com/qsyy0921/IM/schemas/kafka/identity/v1"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const TopicIdentityEvents = "im.identity.events"

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
	return &Relay{store: store, publisher: publisher, config: normalizeConfig(config)}
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
		return types.OutboxRelayStats{}, errors.New("identity outbox relay store is not configured")
	}
	if relay.publisher == nil {
		return types.OutboxRelayStats{}, errors.New("identity outbox relay publisher is not configured")
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
		records = append(records, types.KafkaPublishRecord{Key: []byte(message.PartitionKey), Value: value})
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
	event, err := BuildIdentityEvent(message)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(event)
}

func BuildIdentityEvent(message types.OutboxMessage) (*identityeventsv1.IdentityEvent, error) {
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
		return nil, errors.New("identity outbox envelope is incomplete")
	}
	event := &identityeventsv1.IdentityEvent{
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
	case types.IdentityEventDeviceRevoked:
		payload, err := decodeIdentityPayload(message.PayloadJSON, false)
		if err != nil {
			return nil, err
		}
		event.Payload = &identityeventsv1.IdentityEvent_DeviceRevoked{
			DeviceRevoked: &identityeventsv1.IdentityDeviceRevokedV1{
				TenantId:  payload.TenantID,
				UserId:    payload.UserID,
				DeviceId:  payload.DeviceID,
				Status:    payload.Status,
				RevokedBy: payload.RevokedBy,
				Reason:    payload.Reason,
				RevokedAt: payload.Timestamp(),
			},
		}
		return event, nil
	case types.IdentityEventSessionRevoked:
		payload, err := decodeIdentityPayload(message.PayloadJSON, true)
		if err != nil {
			return nil, err
		}
		event.Payload = &identityeventsv1.IdentityEvent_SessionRevoked{
			SessionRevoked: &identityeventsv1.IdentitySessionRevokedV1{
				TenantId:  payload.TenantID,
				UserId:    payload.UserID,
				DeviceId:  payload.DeviceID,
				SessionId: payload.SessionID,
				Status:    payload.Status,
				RevokedBy: payload.RevokedBy,
				Reason:    payload.Reason,
				RevokedAt: payload.Timestamp(),
			},
		}
		return event, nil
	default:
		return nil, errors.New("unsupported identity outbox event type")
	}
}

type identityPayload struct {
	TenantID  string `json:"tenant_id"`
	UserID    string `json:"user_id"`
	DeviceID  string `json:"device_id"`
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	RevokedBy string `json:"revoked_by"`
	Reason    string `json:"reason"`
	RevokedAt string `json:"revoked_at"`
}

func (payload identityPayload) Timestamp() *timestamppb.Timestamp {
	revokedAt, _ := time.Parse(time.RFC3339Nano, payload.RevokedAt)
	return timestamppb.New(revokedAt)
}

func decodeIdentityPayload(payloadJSON []byte, requireSession bool) (identityPayload, error) {
	var payload identityPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return identityPayload{}, err
	}
	if payload.TenantID == "" ||
		payload.UserID == "" ||
		payload.DeviceID == "" ||
		payload.Status == "" ||
		payload.RevokedAt == "" {
		return identityPayload{}, errors.New("identity revoked payload is incomplete")
	}
	if requireSession && payload.SessionID == "" {
		return identityPayload{}, errors.New("identity session revoked payload is incomplete")
	}
	if _, err := time.Parse(time.RFC3339Nano, payload.RevokedAt); err != nil {
		return identityPayload{}, errors.New("identity revoked payload revoked_at is invalid")
	}
	return payload, nil
}

func normalizeConfig(config Config) Config {
	if config.Topic == "" {
		config.Topic = TopicIdentityEvents
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
