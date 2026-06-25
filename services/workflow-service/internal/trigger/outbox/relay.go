package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

const TopicWorkflowEvents = "im.workflow.events"

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

type workflowEventEnvelope struct {
	SchemaVersion string          `json:"schema_version"`
	EventID       string          `json:"event_id"`
	EventType     string          `json:"event_type"`
	EventVersion  int32           `json:"event_version"`
	TenantID      string          `json:"tenant_id"`
	WorkflowID    string          `json:"workflow_id"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id"`
	PartitionKey  string          `json:"partition_key"`
	Producer      string          `json:"producer"`
	OccurredAtMS  int64           `json:"occurred_at_ms"`
	PayloadJSON   json.RawMessage `json:"payload_json"`
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
				relay.config.Logf("workflow-service outbox relay retrying after runtime error: %v", err)
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

func (relay *Relay) RunOnce(ctx context.Context) (types.OutboxRelayStats, error) {
	if relay == nil || relay.store == nil {
		return types.OutboxRelayStats{}, errors.New("workflow outbox relay store is not configured")
	}
	if relay.publisher == nil {
		return types.OutboxRelayStats{}, errors.New("workflow outbox relay publisher is not configured")
	}
	return relay.store.ProcessReadyBatch(
		ctx,
		relay.config.BatchSize,
		relay.config.MaxAttempts,
		relay.config.RetryBaseDelay,
		relay.publishMessages,
	)
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
	if strings.TrimSpace(message.EventID) == "" ||
		strings.TrimSpace(message.EventType) == "" ||
		strings.TrimSpace(string(message.TenantID)) == "" ||
		strings.TrimSpace(message.WorkflowID) == "" ||
		strings.TrimSpace(message.AggregateType) == "" ||
		strings.TrimSpace(message.AggregateID) == "" ||
		strings.TrimSpace(message.PartitionKey) == "" ||
		strings.TrimSpace(message.Producer) == "" ||
		message.EventVersion <= 0 {
		return nil, errors.New("workflow outbox envelope is incomplete")
	}
	if !isSupportedEventType(message.EventType) {
		return nil, errors.New("unsupported workflow outbox event type")
	}
	if containsForbiddenPayloadField(message.PayloadJSON) {
		return nil, errors.New("workflow outbox payload contains internal field")
	}
	if !json.Valid(message.PayloadJSON) {
		return nil, errors.New("workflow outbox payload is invalid json")
	}
	var payload map[string]any
	if err := json.Unmarshal(message.PayloadJSON, &payload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(asString(payload["tenant_id"])) == "" ||
		strings.TrimSpace(asString(payload["workflow_id"])) == "" {
		return nil, errors.New("workflow outbox payload is incomplete")
	}
	envelope := workflowEventEnvelope{
		SchemaVersion: "nexusim.workflow.event_envelope.v1",
		EventID:       message.EventID,
		EventType:     message.EventType,
		EventVersion:  message.EventVersion,
		TenantID:      string(message.TenantID),
		WorkflowID:    message.WorkflowID,
		AggregateType: message.AggregateType,
		AggregateID:   message.AggregateID,
		PartitionKey:  message.PartitionKey,
		Producer:      message.Producer,
		OccurredAtMS:  message.OccurredAt.UTC().UnixMilli(),
		PayloadJSON:   json.RawMessage(message.PayloadJSON),
	}
	return json.Marshal(envelope)
}

func isSupportedEventType(eventType string) bool {
	switch eventType {
	case types.WorkflowEventSubmitted,
		types.WorkflowEventDecisionRecorded,
		types.WorkflowEventTimedOut,
		types.WorkflowEventCompensationRequested,
		types.WorkflowEventCompensationSucceeded,
		types.WorkflowEventCompensationFailed,
		types.WorkflowEventExternalCallbackDelivered,
		types.WorkflowEventExternalCallbackDLQ,
		types.WorkflowEventExternalCallbackRedriven:
		return true
	default:
		return false
	}
}

func containsForbiddenPayloadField(payloadJSON []byte) bool {
	lowered := strings.ToLower(string(payloadJSON))
	for _, field := range []string{
		"password",
		"secret",
		"access_token",
		"refresh_token",
		"reset_token",
		"raw_token",
		"api_key",
		"apikey",
		"authorization",
		"provider_body",
		"provider_response",
		"raw_provider",
		"raw_payload",
		"prompt",
		"evidence_pack",
		"private://",
		"postgres://",
		"dsn=",
	} {
		if strings.Contains(lowered, field) {
			return true
		}
	}
	return false
}

func asString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func normalizeConfig(config Config) Config {
	if config.Topic == "" {
		config.Topic = TopicWorkflowEvents
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
