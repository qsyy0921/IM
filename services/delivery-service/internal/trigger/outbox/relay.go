package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
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

type ShardedStore interface {
	ProcessReadyShardBatch(
		ctx context.Context,
		limit int,
		maxAttempts int,
		retryBaseDelay time.Duration,
		shardCount int,
		shardID int,
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
	WorkerCount    int
	PollInterval   time.Duration
	MaxAttempts    int
	RetryBaseDelay time.Duration
	ErrorBackoff   time.Duration
	Logf           func(format string, args ...any)
}

type relayMetrics struct {
	totalErrors        atomic.Uint64
	consecutiveErrors  atomic.Uint64
	totalFetched       atomic.Uint64
	totalPublished     atomic.Uint64
	totalRetried       atomic.Uint64
	totalDeadLettered  atomic.Uint64
	lastRunDurationMS  atomic.Int64
	lastPublishMS      atomic.Int64
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
	if relay.config.WorkerCount <= 1 {
		return relay.runLoop(ctx, 0)
	}

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, relay.config.WorkerCount)
	var wg sync.WaitGroup
	for workerID := 0; workerID < relay.config.WorkerCount; workerID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			errCh <- relay.runLoop(workerCtx, id)
		}(workerID)
	}

	err := <-errCh
	cancel()
	wg.Wait()
	if errors.Is(err, context.Canceled) && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func (relay *Relay) runLoop(ctx context.Context, workerID int) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		stats, err := relay.runOnceForWorker(ctx, workerID)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			if relay.config.Logf != nil {
				relay.config.Logf("delivery-service outbox relay worker=%d retrying after runtime error: %v", workerID, err)
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
		TotalFetched:       relay.metrics.totalFetched.Load(),
		TotalPublished:     relay.metrics.totalPublished.Load(),
		TotalRetried:       relay.metrics.totalRetried.Load(),
		TotalDeadLettered:  relay.metrics.totalDeadLettered.Load(),
		WorkerCount:        relay.config.WorkerCount,
		LastRunDurationMS:  relay.metrics.lastRunDurationMS.Load(),
		LastPublishMS:      relay.metrics.lastPublishMS.Load(),
		LastErrorAtMS:      relay.metrics.lastErrorAtMS.Load(),
		LastSuccessAtMS:    relay.metrics.lastSuccessAtMS.Load(),
		LastPublishedAtMS:  relay.metrics.lastPublishedAtMS.Load(),
		LastErrorBackoffMS: relay.metrics.lastErrorBackoffMS.Load(),
	}
}

func (relay *Relay) RunOnce(ctx context.Context) (types.OutboxRelayStats, error) {
	return relay.runOnceForWorker(ctx, 0)
}

func (relay *Relay) runOnceForWorker(ctx context.Context, workerID int) (types.OutboxRelayStats, error) {
	if relay == nil || relay.store == nil {
		return types.OutboxRelayStats{}, errors.New("delivery outbox relay store is not configured")
	}
	if relay.publisher == nil {
		return types.OutboxRelayStats{}, errors.New("delivery outbox relay publisher is not configured")
	}
	startedAt := time.Now()
	var stats types.OutboxRelayStats
	var err error
	if relay.config.WorkerCount > 1 {
		shardedStore, ok := relay.store.(ShardedStore)
		if !ok {
			return types.OutboxRelayStats{}, errors.New("delivery outbox relay store does not support sharded workers")
		}
		stats, err = shardedStore.ProcessReadyShardBatch(
			ctx,
			relay.config.BatchSize,
			relay.config.MaxAttempts,
			relay.config.RetryBaseDelay,
			relay.config.WorkerCount,
			workerID,
			relay.publishMessages,
		)
	} else {
		stats, err = relay.store.ProcessReadyBatch(
			ctx,
			relay.config.BatchSize,
			relay.config.MaxAttempts,
			relay.config.RetryBaseDelay,
			relay.publishMessages,
		)
	}
	relay.metrics.lastRunDurationMS.Store(time.Since(startedAt).Milliseconds())
	return stats, err
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
	startedAt := time.Now()
	if err := relay.publisher.PublishBatch(ctx, relay.config.Topic, records); err != nil {
		for _, index := range indexes {
			errs[index] = err
		}
	}
	relay.metrics.lastPublishMS.Store(time.Since(startedAt).Milliseconds())
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
				SourceEventType: payload.SourceEventType,
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
	case types.DeliveryEventInboxItemHidden:
		payload, err := decodeInboxItemHiddenPayload(message.PayloadJSON)
		if err != nil {
			return nil, err
		}
		event.Payload = &deliveryeventsv1.DeliveryEvent_InboxItemHidden{
			InboxItemHidden: &deliveryeventsv1.DeliveryInboxItemHiddenV1{
				TenantId:        payload.TenantID,
				UserId:          payload.UserID,
				DeviceId:        payload.DeviceID,
				ConversationId:  payload.ConversationID,
				ConversationSeq: payload.ConversationSeq,
				MessageId:       payload.MessageID,
			},
		}
		return event, nil
	case types.DeliveryEventConversationSignal:
		payload, err := decodeConversationSignalPayload(message.PayloadJSON)
		if err != nil {
			return nil, err
		}
		event.Payload = &deliveryeventsv1.DeliveryEvent_ConversationSignal{
			ConversationSignal: &deliveryeventsv1.DeliveryConversationSignalV1{
				TenantId:        payload.TenantID,
				ConversationId:  payload.ConversationID,
				ConversationSeq: payload.ConversationSeq,
				SourceEventId:   payload.SourceEventID,
				SourceEventType: payload.SourceEventType,
				MessageId:       payload.MessageID,
				SenderId:        payload.SenderID,
				FanoutMode:      payload.FanoutMode,
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
	SourceEventType string `json:"source_event_type"`
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
	if payload.SourceEventType == "" {
		payload.SourceEventType = "message.persisted.v1"
	}
	if payload.TenantID == "" ||
		payload.UserID == "" ||
		payload.ConversationID == "" ||
		payload.ConversationSeq <= 0 ||
		payload.SourceEventID == "" ||
		payload.SourceEventType == "" ||
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

type inboxItemHiddenPayload struct {
	TenantID        string `json:"tenant_id"`
	UserID          string `json:"user_id"`
	DeviceID        string `json:"device_id"`
	ConversationID  string `json:"conversation_id"`
	ConversationSeq int64  `json:"conversation_seq"`
	MessageID       string `json:"message_id"`
}

func decodeInboxItemHiddenPayload(payloadJSON []byte) (inboxItemHiddenPayload, error) {
	var payload inboxItemHiddenPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return inboxItemHiddenPayload{}, err
	}
	if payload.TenantID == "" ||
		payload.UserID == "" ||
		payload.DeviceID == "" ||
		payload.ConversationID == "" ||
		payload.ConversationSeq <= 0 {
		return inboxItemHiddenPayload{}, errors.New("delivery inbox item hidden payload is incomplete")
	}
	return payload, nil
}

type conversationSignalPayload struct {
	TenantID        string `json:"tenant_id"`
	ConversationID  string `json:"conversation_id"`
	ConversationSeq int64  `json:"conversation_seq"`
	SourceEventID   string `json:"source_event_id"`
	SourceEventType string `json:"source_event_type"`
	MessageID       string `json:"message_id"`
	SenderID        string `json:"sender_id"`
	FanoutMode      string `json:"fanout_mode"`
}

func decodeConversationSignalPayload(payloadJSON []byte) (conversationSignalPayload, error) {
	var payload conversationSignalPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return conversationSignalPayload{}, err
	}
	if payload.TenantID == "" ||
		payload.ConversationID == "" ||
		payload.ConversationSeq <= 0 ||
		payload.SourceEventID == "" ||
		payload.SourceEventType == "" ||
		payload.MessageID == "" ||
		payload.SenderID == "" ||
		payload.FanoutMode == "" {
		return conversationSignalPayload{}, errors.New("delivery conversation signal payload is incomplete")
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
	if config.WorkerCount <= 0 {
		config.WorkerCount = 1
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
	if stats.Fetched > 0 {
		relay.metrics.totalFetched.Add(uint64(stats.Fetched))
	}
	if stats.Published > 0 {
		relay.metrics.totalPublished.Add(uint64(stats.Published))
	}
	if stats.Retried > 0 {
		relay.metrics.totalRetried.Add(uint64(stats.Retried))
	}
	if stats.DeadLettered > 0 {
		relay.metrics.totalDeadLettered.Add(uint64(stats.DeadLettered))
	}
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
