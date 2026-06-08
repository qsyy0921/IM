package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	conversationtimelinev1 "github.com/qsyy0921/IM/schemas/kafka"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const TopicConversationTimelineEvents = "conversation.timeline.events"

type Store interface {
	ProcessReady(
		ctx context.Context,
		limit int,
		maxAttempts int,
		retryBaseDelay time.Duration,
		publish func(context.Context, types.OutboxMessage) error,
	) (types.OutboxRelayStats, error)
}

type Publisher interface {
	Publish(ctx context.Context, topic string, key []byte, value []byte) error
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

func (r *Relay) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.config.PollInterval)
	defer ticker.Stop()

	for {
		stats, err := r.RunOnce(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		if stats.Fetched > 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *Relay) RunOnce(ctx context.Context) (types.OutboxRelayStats, error) {
	if r.store == nil {
		return types.OutboxRelayStats{}, errors.New("outbox relay store is not configured")
	}
	if r.publisher == nil {
		return types.OutboxRelayStats{}, errors.New("outbox relay publisher is not configured")
	}
	return r.store.ProcessReady(
		ctx,
		r.config.BatchSize,
		r.config.MaxAttempts,
		r.config.RetryBaseDelay,
		r.publishMessage,
	)
}

func (r *Relay) publishMessage(ctx context.Context, message types.OutboxMessage) error {
	value, err := BuildKafkaValue(message)
	if err != nil {
		return err
	}
	return r.publisher.Publish(
		ctx,
		r.config.Topic,
		[]byte(message.PartitionKey),
		value,
	)
}

func BuildKafkaValue(message types.OutboxMessage) ([]byte, error) {
	event, err := BuildConversationTimelineEvent(message)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(event)
}

func BuildConversationTimelineEvent(message types.OutboxMessage) (*conversationtimelinev1.ConversationTimelineEvent, error) {
	switch message.EventType {
	case types.TimelineEventMessagePersisted:
		payload, err := decodeMessagePersistedPayload(message.PayloadJSON)
		if err != nil {
			return nil, err
		}
		acceptedAt, err := time.Parse(time.RFC3339Nano, payload.AcceptedAt)
		if err != nil {
			return nil, err
		}
		payloadStruct, err := structFromRawJSON(payload.Payload)
		if err != nil {
			return nil, err
		}
		occurredAt := message.OccurredAt
		if occurredAt.IsZero() {
			occurredAt = acceptedAt
		}
		return &conversationtimelinev1.ConversationTimelineEvent{
			EventId:          string(message.EventID),
			EventType:        string(message.EventType),
			EventVersion:     message.EventVersion,
			TenantId:         string(message.TenantID),
			AggregateType:    "conversation",
			AggregateId:      string(message.ConversationID),
			AggregateVersion: message.AggregateVersion,
			PartitionKey:     message.PartitionKey,
			MappingVersion:   message.MappingVersion,
			TraceId:          message.TraceID,
			CorrelationId:    message.CorrelationID,
			CausationId:      message.CausationID,
			Producer:         message.Producer,
			OccurredAt:       timestamppb.New(occurredAt),
			Metadata: &conversationtimelinev1.TimelineMetadata{
				FanoutMode:          string(message.FanoutMode),
				FanoutPolicyVersion: message.FanoutPolicyVersion,
				PermissionVersion:   message.PermissionVersion,
				Classification:      message.Classification,
				MappingVersion:      message.MappingVersion,
			},
			Payload: &conversationtimelinev1.ConversationTimelineEvent_MessagePersisted{
				MessagePersisted: &conversationtimelinev1.MessagePersistedV1{
					MessageId:       payload.MessageID,
					ConversationId:  payload.ConversationID,
					ConversationSeq: payload.ConversationSeq,
					SenderId:        payload.SenderID,
					DeviceId:        payload.DeviceID,
					ClientMsgId:     payload.ClientMsgID,
					CommandHash:     payload.CommandHash,
					MessageType:     payload.MessageType,
					Payload:         payloadStruct,
					AttachmentIds:   payload.AttachmentIDs,
					AcceptedAt:      timestamppb.New(acceptedAt),
				},
			},
		}, nil
	default:
		return nil, errors.New("unsupported outbox event type")
	}
}

type messagePersistedPayload struct {
	MessageID       string          `json:"message_id"`
	ConversationID  string          `json:"conversation_id"`
	ConversationSeq int64           `json:"conversation_seq"`
	SenderID        string          `json:"sender_id"`
	DeviceID        string          `json:"device_id"`
	ClientMsgID     string          `json:"client_msg_id"`
	CommandHash     string          `json:"command_hash"`
	MessageType     string          `json:"message_type"`
	Payload         json.RawMessage `json:"payload"`
	AttachmentIDs   []string        `json:"attachment_ids"`
	AcceptedAt      string          `json:"accepted_at"`
}

func decodeMessagePersistedPayload(payloadJSON []byte) (messagePersistedPayload, error) {
	var payload messagePersistedPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return messagePersistedPayload{}, err
	}
	if payload.MessageID == "" ||
		payload.ConversationID == "" ||
		payload.ConversationSeq <= 0 ||
		payload.CommandHash == "" ||
		len(payload.Payload) == 0 ||
		payload.AcceptedAt == "" {
		return messagePersistedPayload{}, errors.New("message persisted payload is incomplete")
	}
	return payload, nil
}

func structFromRawJSON(payload json.RawMessage) (*structpb.Struct, error) {
	var object map[string]any
	if err := json.Unmarshal(payload, &object); err != nil {
		return nil, err
	}
	return structpb.NewStruct(object)
}

func normalizeConfig(config Config) Config {
	if config.Topic == "" {
		config.Topic = TopicConversationTimelineEvents
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
