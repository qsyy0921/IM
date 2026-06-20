package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	vectoreventsv1 "github.com/qsyy0921/IM/schemas/kafka/vector/v1"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const TopicVectorEvents = "im.vector.events"

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
				relay.config.Logf("vector-index-service outbox relay retrying after runtime error: %v", err)
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
		return types.OutboxRelayStats{}, errors.New("vector outbox relay store is not configured")
	}
	if relay.publisher == nil {
		return types.OutboxRelayStats{}, errors.New("vector outbox relay publisher is not configured")
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
	event, err := BuildVectorEvent(message)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(event)
}

func BuildVectorEvent(message types.OutboxMessage) (*vectoreventsv1.VectorEvent, error) {
	if strings.TrimSpace(message.EventID) == "" ||
		strings.TrimSpace(message.EventType) == "" ||
		strings.TrimSpace(string(message.TenantID)) == "" ||
		strings.TrimSpace(message.AggregateID) == "" ||
		message.EventVersion <= 0 ||
		strings.TrimSpace(message.PartitionKey) == "" ||
		strings.TrimSpace(message.Producer) == "" {
		return nil, errors.New("vector outbox envelope is incomplete")
	}
	payload, err := decodeVectorPayload(message.PayloadJSON)
	if err != nil {
		return nil, err
	}
	if err := validateVectorPayloadForEvent(message.EventType, payload); err != nil {
		return nil, err
	}
	event := &vectoreventsv1.VectorEvent{
		EventId:          message.EventID,
		EventType:        message.EventType,
		EventVersion:     int32(message.EventVersion),
		TenantId:         string(message.TenantID),
		AggregateType:    "vector_item",
		AggregateId:      message.AggregateID,
		AggregateVersion: message.AggregateVersion,
		PartitionKey:     message.PartitionKey,
		TraceId:          message.TraceID,
		CorrelationId:    firstNonEmpty(message.CorrelationID, message.EventID),
		CausationId:      firstNonEmpty(message.CausationID, message.EventID),
		Producer:         message.Producer,
		OccurredAt:       timestamppb.New(message.OccurredAt),
	}
	switch message.EventType {
	case "vector.item.indexed.v1":
		event.Payload = &vectoreventsv1.VectorEvent_ItemIndexed{
			ItemIndexed: vectorItemIndexed(payload, string(message.TenantID)),
		}
	case "vector.item.tombstoned.v1":
		event.Payload = &vectoreventsv1.VectorEvent_ItemTombstoned{
			ItemTombstoned: vectorItemTombstoned(payload, string(message.TenantID)),
		}
	default:
		return nil, errors.New("unsupported vector outbox event type")
	}
	return event, nil
}

type vectorPayload struct {
	VectorItemRefHash string `json:"vector_item_ref_hash"`
	CollectionType    string `json:"collection_type"`
	SourceService     string `json:"source_service"`
	SourceRefHash     string `json:"source_ref_hash"`
	EmbeddingModelRef string `json:"embedding_model_ref"`
	Dimension         int32  `json:"dimension"`
	VisibilityVersion int64  `json:"visibility_version"`
	TombstoneStatus   string `json:"tombstone_status"`
	DeleteProofID     string `json:"delete_proof_id"`
	ReasonClass       string `json:"reason_class"`
}

func decodeVectorPayload(payloadJSON []byte) (vectorPayload, error) {
	if containsForbiddenPayloadField(payloadJSON) {
		return vectorPayload{}, errors.New("vector payload contains internal field")
	}
	var payload vectorPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return vectorPayload{}, err
	}
	return payload, nil
}

func validateVectorPayloadForEvent(eventType string, payload vectorPayload) error {
	if payload.VectorItemRefHash == "" ||
		payload.CollectionType == "" ||
		payload.SourceService == "" ||
		payload.SourceRefHash == "" ||
		payload.EmbeddingModelRef == "" ||
		payload.Dimension <= 0 ||
		payload.VisibilityVersion <= 0 ||
		payload.TombstoneStatus == "" {
		return errors.New("vector payload is incomplete")
	}
	switch eventType {
	case "vector.item.indexed.v1":
		return nil
	case "vector.item.tombstoned.v1":
		if payload.DeleteProofID == "" || payload.ReasonClass == "" {
			return errors.New("vector tombstoned payload is incomplete")
		}
		return nil
	default:
		return errors.New("unsupported vector outbox event type")
	}
}

func containsForbiddenPayloadField(payloadJSON []byte) bool {
	lowered := strings.ToLower(string(payloadJSON))
	for _, field := range []string{
		"raw_text",
		"message_body",
		"embedding_vector",
		"vector_array",
		"source_uri",
		"object_key",
		"connector_secret",
		"api_key",
		"authorization",
		"token",
		"password",
		"private_key",
		"dsn",
	} {
		if strings.Contains(lowered, field) {
			return true
		}
	}
	return false
}

func vectorItemIndexed(payload vectorPayload, tenantID string) *vectoreventsv1.VectorItemIndexedV1 {
	return &vectoreventsv1.VectorItemIndexedV1{
		TenantId:          tenantID,
		VectorItemRefHash: payload.VectorItemRefHash,
		CollectionType:    payload.CollectionType,
		SourceService:     payload.SourceService,
		SourceRefHash:     payload.SourceRefHash,
		EmbeddingModelRef: payload.EmbeddingModelRef,
		Dimension:         payload.Dimension,
		VisibilityVersion: payload.VisibilityVersion,
		TombstoneStatus:   payload.TombstoneStatus,
	}
}

func vectorItemTombstoned(payload vectorPayload, tenantID string) *vectoreventsv1.VectorItemTombstonedV1 {
	return &vectoreventsv1.VectorItemTombstonedV1{
		TenantId:          tenantID,
		VectorItemRefHash: payload.VectorItemRefHash,
		CollectionType:    payload.CollectionType,
		SourceService:     payload.SourceService,
		SourceRefHash:     payload.SourceRefHash,
		EmbeddingModelRef: payload.EmbeddingModelRef,
		Dimension:         payload.Dimension,
		VisibilityVersion: payload.VisibilityVersion,
		TombstoneStatus:   payload.TombstoneStatus,
		DeleteProofId:     payload.DeleteProofID,
		ReasonClass:       payload.ReasonClass,
	}
}

func normalizeConfig(config Config) Config {
	if config.Topic == "" {
		config.Topic = TopicVectorEvents
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
