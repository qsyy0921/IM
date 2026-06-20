package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	notificationeventsv1 "github.com/qsyy0921/IM/schemas/kafka/notification/v1"
	"github.com/qsyy0921/IM/services/notification-service/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const TopicNotificationEvents = "im.notification.events"

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

type Snapshot struct {
	TotalErrors        uint64
	ConsecutiveErrors  uint64
	LastErrorAtMS      int64
	LastSuccessAtMS    int64
	LastPublishedAtMS  int64
	LastErrorBackoffMS int64
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
				relay.config.Logf("notification-service outbox relay retrying after runtime error: %v", err)
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

func (relay *Relay) Snapshot() Snapshot {
	return Snapshot{
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
		return types.OutboxRelayStats{}, errors.New("notification outbox relay store is not configured")
	}
	if relay.publisher == nil {
		return types.OutboxRelayStats{}, errors.New("notification outbox relay publisher is not configured")
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
	event, err := BuildNotificationEvent(message)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(event)
}

func BuildNotificationEvent(message types.OutboxMessage) (*notificationeventsv1.NotificationEvent, error) {
	if strings.TrimSpace(message.EventID) == "" ||
		strings.TrimSpace(message.EventType) == "" ||
		strings.TrimSpace(string(message.TenantID)) == "" ||
		strings.TrimSpace(message.RequestID) == "" ||
		message.EventVersion <= 0 ||
		strings.TrimSpace(message.PartitionKey) == "" ||
		strings.TrimSpace(message.Producer) == "" {
		return nil, errors.New("notification outbox envelope is incomplete")
	}
	payload, err := decodeNotificationPayload(message.PayloadJSON)
	if err != nil {
		return nil, err
	}
	traceID := firstNonEmpty(message.TraceID, payload.TraceID)
	correlationID := firstNonEmpty(message.CorrelationID, payload.CorrelationID, message.EventID)
	causationID := firstNonEmpty(message.CausationID, payload.CausationID, message.EventID)
	event := &notificationeventsv1.NotificationEvent{
		EventId:          message.EventID,
		EventType:        message.EventType,
		EventVersion:     message.EventVersion,
		TenantId:         string(message.TenantID),
		AggregateType:    "notification_request",
		AggregateId:      message.RequestID,
		AggregateVersion: message.AggregateVersion,
		PartitionKey:     message.PartitionKey,
		TraceId:          traceID,
		CorrelationId:    correlationID,
		CausationId:      causationID,
		Producer:         message.Producer,
		OccurredAt:       timestamppb.New(message.OccurredAt),
	}
	switch message.EventType {
	case types.NotificationEventRequestAccepted:
		event.Payload = &notificationeventsv1.NotificationEvent_RequestAccepted{
			RequestAccepted: notificationRequestAccepted(payload),
		}
	default:
		return nil, errors.New("unsupported notification outbox event type")
	}
	return event, nil
}

type notificationPayload struct {
	TenantID          string `json:"tenant_id"`
	RequestID         string `json:"request_id"`
	RequesterService  string `json:"requester_service"`
	Channel           string `json:"channel"`
	RecipientRef      string `json:"recipient_ref"`
	DestinationMasked string `json:"destination_masked"`
	TemplateKey       string `json:"template_key"`
	TemplateVersion   string `json:"template_version"`
	Locale            string `json:"locale"`
	Priority          string `json:"priority"`
	Status            string `json:"status"`
	CorrelationID     string `json:"correlation_id"`
	CausationID       string `json:"causation_id"`
	TraceID           string `json:"trace_id"`
}

func decodeNotificationPayload(payloadJSON []byte) (notificationPayload, error) {
	if containsForbiddenPayloadField(payloadJSON) {
		return notificationPayload{}, errors.New("notification payload contains internal field")
	}
	var payload notificationPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return notificationPayload{}, err
	}
	if payload.TenantID == "" ||
		payload.RequestID == "" ||
		payload.RequesterService == "" ||
		payload.Channel == "" ||
		payload.RecipientRef == "" ||
		payload.TemplateKey == "" ||
		payload.TemplateVersion == "" ||
		payload.Priority == "" ||
		payload.Status == "" {
		return notificationPayload{}, errors.New("notification request payload is incomplete")
	}
	return payload, nil
}

func containsForbiddenPayloadField(payloadJSON []byte) bool {
	lowered := strings.ToLower(string(payloadJSON))
	for _, field := range []string{
		"destination_ref",
		"destination_hash",
		"secret_payload",
		"provider_body",
		"provider_response",
		"authorization",
		"smtp_transcript",
		"reset_token",
		"challenge_code",
		"totp",
		"recovery_code",
	} {
		if strings.Contains(lowered, field) {
			return true
		}
	}
	return false
}

func notificationRequestAccepted(payload notificationPayload) *notificationeventsv1.NotificationRequestAcceptedV1 {
	return &notificationeventsv1.NotificationRequestAcceptedV1{
		TenantId:          payload.TenantID,
		RequestId:         payload.RequestID,
		RequesterService:  payload.RequesterService,
		Channel:           payload.Channel,
		RecipientRef:      payload.RecipientRef,
		DestinationMasked: payload.DestinationMasked,
		TemplateKey:       payload.TemplateKey,
		TemplateVersion:   payload.TemplateVersion,
		Locale:            payload.Locale,
		Priority:          payload.Priority,
		Status:            payload.Status,
	}
}

func normalizeConfig(config Config) Config {
	if config.Topic == "" {
		config.Topic = TopicNotificationEvents
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
