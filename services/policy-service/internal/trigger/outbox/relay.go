package outbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	policyeventsv1 "github.com/qsyy0921/IM/schemas/kafka/policy/v1"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const TopicPolicyEvents = "im.policy.events"

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
				relay.config.Logf("policy-service outbox relay retrying after runtime error: %v", err)
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
		return types.OutboxRelayStats{}, errors.New("policy audit outbox relay store is not configured")
	}
	if relay.publisher == nil {
		return types.OutboxRelayStats{}, errors.New("policy audit outbox relay publisher is not configured")
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
	event, err := BuildPolicyEvent(message)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(event)
}

func BuildPolicyEvent(message types.OutboxMessage) (*policyeventsv1.PolicyEvent, error) {
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
		return nil, errors.New("policy audit outbox envelope is incomplete")
	}
	event := &policyeventsv1.PolicyEvent{
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
	case types.PolicyEventMessageActionDecision:
		payload, err := decodeMessageActionDecisionPayload(message.PayloadJSON)
		if err != nil {
			return nil, err
		}
		event.Payload = &policyeventsv1.PolicyEvent_MessageActionDecision{
			MessageActionDecision: &policyeventsv1.PolicyMessageActionDecisionV1{
				TenantId:                 payload.TenantID,
				ActorUserKey:             payload.ActorUserKey,
				DeviceKey:                payload.DeviceKey,
				ConversationKey:          payload.ConversationKey,
				MessageKey:               payload.MessageKey,
				Action:                   payload.Action,
				MessageIdPresent:         payload.MessageIDPresent,
				DirectPeerContextPresent: payload.DirectPeerContextPresent,
				DirectPeerKey:            payload.DirectPeerKey,
				Allowed:                  payload.Allowed,
				PermissionVersion:        payload.PermissionVersion,
				Classification:           payload.Classification,
				ReasonCode:               payload.ReasonCode,
				DecidedAt:                timestamppb.New(payload.decidedAt),
				DecisionSource:           payload.DecisionSource,
			},
		}
		return event, nil
	default:
		return nil, errors.New("unsupported policy audit outbox event type")
	}
}

type messageActionDecisionPayload struct {
	EventID                  string `json:"event_id"`
	TenantID                 string `json:"tenant_id"`
	ActorUserKey             string `json:"actor_user_key"`
	DeviceKey                string `json:"device_key"`
	ConversationKey          string `json:"conversation_key"`
	MessageKey               string `json:"message_key"`
	Action                   string `json:"action"`
	MessageIDPresent         bool   `json:"message_id_present"`
	DirectPeerContextPresent bool   `json:"direct_peer_context_present"`
	DirectPeerKey            string `json:"direct_peer_key"`
	Allowed                  bool   `json:"allowed"`
	PermissionVersion        int64  `json:"permission_version"`
	Classification           string `json:"classification"`
	ReasonCode               string `json:"reason_code"`
	DecisionSource           string `json:"decision_source"`
	TraceID                  string `json:"trace_id"`
	RequestID                string `json:"request_id"`
	DecidedAt                string `json:"decided_at"`
	decidedAt                time.Time
}

func decodeMessageActionDecisionPayload(payloadJSON []byte) (messageActionDecisionPayload, error) {
	var payload messageActionDecisionPayload
	decoder := json.NewDecoder(bytes.NewReader(payloadJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return messageActionDecisionPayload{}, err
	}
	decidedAt, err := time.Parse(time.RFC3339Nano, payload.DecidedAt)
	if err != nil {
		return messageActionDecisionPayload{}, errors.New("policy audit payload decided_at is invalid")
	}
	if payload.EventID == "" ||
		payload.TenantID == "" ||
		payload.ActorUserKey == "" ||
		payload.ConversationKey == "" ||
		payload.Action == "" ||
		payload.PermissionVersion <= 0 ||
		payload.Classification == "" {
		return messageActionDecisionPayload{}, errors.New("policy audit payload is incomplete")
	}
	if payload.MessageIDPresent && payload.MessageKey == "" {
		return messageActionDecisionPayload{}, errors.New("policy audit message key is incomplete")
	}
	if payload.DirectPeerContextPresent && payload.DirectPeerKey == "" {
		return messageActionDecisionPayload{}, errors.New("policy audit direct peer key is incomplete")
	}
	payload.decidedAt = decidedAt
	return payload, nil
}

func normalizeConfig(config Config) Config {
	if config.Topic == "" {
		config.Topic = TopicPolicyEvents
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
