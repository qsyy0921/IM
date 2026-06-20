package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	mediaeventsv1 "github.com/qsyy0921/IM/schemas/kafka/media/v1"
	"github.com/qsyy0921/IM/services/media-service/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const TopicMediaEvents = "im.media.events"

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
				relay.config.Logf("media-service outbox relay retrying after runtime error: %v", err)
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
		return types.OutboxRelayStats{}, errors.New("media outbox relay store is not configured")
	}
	if relay.publisher == nil {
		return types.OutboxRelayStats{}, errors.New("media outbox relay publisher is not configured")
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
	event, err := BuildMediaEvent(message)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(event)
}

func BuildMediaEvent(message types.OutboxMessage) (*mediaeventsv1.MediaEvent, error) {
	if strings.TrimSpace(message.EventID) == "" ||
		strings.TrimSpace(message.EventType) == "" ||
		strings.TrimSpace(string(message.TenantID)) == "" ||
		strings.TrimSpace(message.AssetID) == "" ||
		message.EventVersion <= 0 ||
		strings.TrimSpace(message.PartitionKey) == "" ||
		strings.TrimSpace(message.Producer) == "" {
		return nil, errors.New("media outbox envelope is incomplete")
	}
	event := &mediaeventsv1.MediaEvent{
		EventId:          message.EventID,
		EventType:        message.EventType,
		EventVersion:     message.EventVersion,
		TenantId:         string(message.TenantID),
		AggregateType:    "media_asset",
		AggregateId:      message.AssetID,
		AggregateVersion: message.AggregateVersion,
		PartitionKey:     message.PartitionKey,
		TraceId:          message.TraceID,
		CorrelationId:    message.CorrelationID,
		CausationId:      message.CausationID,
		Producer:         message.Producer,
		OccurredAt:       timestamppb.New(message.OccurredAt),
	}
	payload, err := decodeMediaAssetPayload(message.PayloadJSON)
	if err != nil {
		return nil, err
	}
	switch message.EventType {
	case types.MediaEventAssetUploaded:
		event.Payload = &mediaeventsv1.MediaEvent_AssetUploaded{
			AssetUploaded: mediaAssetUploaded(payload),
		}
	case types.MediaEventAssetReady:
		event.Payload = &mediaeventsv1.MediaEvent_AssetReady{
			AssetReady: mediaAssetReady(payload),
		}
	case types.MediaEventAssetDeleted:
		event.Payload = &mediaeventsv1.MediaEvent_AssetDeleted{
			AssetDeleted: mediaAssetDeleted(payload),
		}
	case types.MediaEventAssetQuarantined:
		event.Payload = &mediaeventsv1.MediaEvent_AssetQuarantined{
			AssetQuarantined: mediaAssetQuarantined(payload),
		}
	default:
		return nil, errors.New("unsupported media outbox event type")
	}
	return event, nil
}

type mediaAssetPayload struct {
	TenantID       string `json:"tenant_id"`
	AssetID        string `json:"asset_id"`
	ConversationID string `json:"conversation_id"`
	MediaKind      string `json:"media_kind"`
	ContentType    string `json:"content_type"`
	SizeBytes      int64  `json:"size_bytes"`
	SHA256         string `json:"sha256"`
	Status         string `json:"status"`
	Reason         string `json:"reason"`
}

func decodeMediaAssetPayload(payloadJSON []byte) (mediaAssetPayload, error) {
	if containsForbiddenPayloadField(payloadJSON) {
		return mediaAssetPayload{}, errors.New("media payload contains internal field")
	}
	var payload mediaAssetPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return mediaAssetPayload{}, err
	}
	if payload.TenantID == "" ||
		payload.AssetID == "" ||
		payload.ConversationID == "" ||
		payload.MediaKind == "" ||
		payload.ContentType == "" ||
		payload.SizeBytes <= 0 ||
		payload.SHA256 == "" ||
		payload.Status == "" {
		return mediaAssetPayload{}, errors.New("media asset payload is incomplete")
	}
	return payload, nil
}

func containsForbiddenPayloadField(payloadJSON []byte) bool {
	lowered := strings.ToLower(string(payloadJSON))
	return strings.Contains(lowered, "object_key") ||
		strings.Contains(lowered, "download_url")
}

func mediaAssetUploaded(payload mediaAssetPayload) *mediaeventsv1.MediaAssetUploadedV1 {
	return &mediaeventsv1.MediaAssetUploadedV1{
		TenantId:       payload.TenantID,
		AssetId:        payload.AssetID,
		ConversationId: payload.ConversationID,
		MediaKind:      payload.MediaKind,
		ContentType:    payload.ContentType,
		SizeBytes:      payload.SizeBytes,
		Sha256:         payload.SHA256,
		Status:         payload.Status,
	}
}

func mediaAssetReady(payload mediaAssetPayload) *mediaeventsv1.MediaAssetReadyV1 {
	return &mediaeventsv1.MediaAssetReadyV1{
		TenantId:       payload.TenantID,
		AssetId:        payload.AssetID,
		ConversationId: payload.ConversationID,
		MediaKind:      payload.MediaKind,
		ContentType:    payload.ContentType,
		SizeBytes:      payload.SizeBytes,
		Sha256:         payload.SHA256,
		Status:         payload.Status,
	}
}

func mediaAssetDeleted(payload mediaAssetPayload) *mediaeventsv1.MediaAssetDeletedV1 {
	return &mediaeventsv1.MediaAssetDeletedV1{
		TenantId:       payload.TenantID,
		AssetId:        payload.AssetID,
		ConversationId: payload.ConversationID,
		MediaKind:      payload.MediaKind,
		ContentType:    payload.ContentType,
		SizeBytes:      payload.SizeBytes,
		Sha256:         payload.SHA256,
		Status:         payload.Status,
	}
}

func mediaAssetQuarantined(payload mediaAssetPayload) *mediaeventsv1.MediaAssetQuarantinedV1 {
	return &mediaeventsv1.MediaAssetQuarantinedV1{
		TenantId:       payload.TenantID,
		AssetId:        payload.AssetID,
		ConversationId: payload.ConversationID,
		MediaKind:      payload.MediaKind,
		ContentType:    payload.ContentType,
		SizeBytes:      payload.SizeBytes,
		Sha256:         payload.SHA256,
		Status:         payload.Status,
		Reason:         payload.Reason,
	}
}

func normalizeConfig(config Config) Config {
	if config.Topic == "" {
		config.Topic = TopicMediaEvents
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
