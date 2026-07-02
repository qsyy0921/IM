package cdc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	kafkainfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/kafka"
	"github.com/qsyy0921/IM/services/message-service/internal/trigger/outbox"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

const (
	DefaultSourceTopic = "nexusim.public.conversation_timeline_events"
	DefaultTargetTopic = "conversation.timeline.events.cdc"
)

type Config struct {
	Brokers      []string
	SourceTopic  string
	TargetTopic  string
	GroupID      string
	ErrorBackoff time.Duration
	// ReorderFlushDelay lets the bridge collect a Debezium burst before
	// publishing by conversation aggregate_version. WAL commit order can differ
	// from allocated conversation_seq under SEQUENCER_BLOCK concurrency, and
	// the delay must cover normal concurrent transaction commit skew.
	ReorderFlushDelay time.Duration
	ReorderMaxRecords int
	Logf              func(format string, args ...any)
}

type Bridge struct {
	reader *kafkago.Reader
	writer *kafkainfra.WriterProducer
	config Config
}

type debziumEnvelope struct {
	Op      string                     `json:"op"`
	After   map[string]json.RawMessage `json:"after"`
	Payload *debeziumPayload           `json:"payload"`
}

type debeziumPayload struct {
	Op    string                     `json:"op"`
	After map[string]json.RawMessage `json:"after"`
}

type recordOrder struct {
	PartitionKey     string
	AggregateVersion int64
}

type batchItem struct {
	source  kafkago.Message
	record  types.KafkaPublishRecord
	order   recordOrder
	publish bool
}

func NewBridge(config Config) (*Bridge, error) {
	config = normalizeConfig(config)
	if len(config.Brokers) == 0 {
		return nil, errors.New("kafka brokers are required")
	}
	if config.SourceTopic == "" {
		return nil, errors.New("cdc source topic is required")
	}
	if config.TargetTopic == "" {
		return nil, errors.New("cdc target topic is required")
	}
	if config.GroupID == "" {
		return nil, errors.New("cdc bridge group id is required")
	}
	writer, err := kafkainfra.NewWriterProducer(config.Brokers)
	if err != nil {
		return nil, err
	}
	return &Bridge{
		writer: writer,
		config: config,
	}, nil
}

func (b *Bridge) Close() error {
	if b == nil {
		return nil
	}
	var joined error
	if b.reader != nil {
		joined = errors.Join(joined, b.reader.Close())
	}
	if b.writer != nil {
		joined = errors.Join(joined, b.writer.Close())
	}
	return joined
}

func (b *Bridge) Run(ctx context.Context) error {
	if b == nil || b.writer == nil {
		return errors.New("cdc bridge is not configured")
	}
	for {
		if err := b.ensureReader(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			if b.config.Logf != nil {
				b.config.Logf("message CDC bridge waiting for source topic after error: %v", err)
			}
			if err := waitForInterval(ctx, b.config.ErrorBackoff); err != nil {
				return err
			}
			continue
		}
		if err := b.runOnce(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			b.closeReader()
			if b.config.Logf != nil {
				b.config.Logf("message CDC bridge retrying after error: %v", err)
			}
			if err := waitForInterval(ctx, b.config.ErrorBackoff); err != nil {
				return err
			}
		}
	}
}

func (b *Bridge) ensureReader(ctx context.Context) error {
	if b.reader != nil {
		return nil
	}
	if err := b.waitForSourceTopic(ctx); err != nil {
		return err
	}
	b.reader = b.newReader()
	return nil
}

func (b *Bridge) newReader() *kafkago.Reader {
	return kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:        b.config.Brokers,
		Topic:          b.config.SourceTopic,
		GroupID:        b.config.GroupID,
		StartOffset:    kafkago.FirstOffset,
		MinBytes:       1,
		MaxBytes:       10e6,
		CommitInterval: 0,
		MaxWait:        500 * time.Millisecond,
	})
}

func (b *Bridge) closeReader() {
	if b.reader == nil {
		return
	}
	_ = b.reader.Close()
	b.reader = nil
}

func (b *Bridge) waitForSourceTopic(ctx context.Context) error {
	for {
		exists, err := topicExists(ctx, b.config.Brokers, b.config.SourceTopic)
		if err == nil && exists {
			if b.config.Logf != nil {
				b.config.Logf("message CDC bridge source topic ready: %s", b.config.SourceTopic)
			}
			return nil
		}
		if b.config.Logf != nil {
			if err != nil {
				b.config.Logf("message CDC bridge waiting for source topic %s: %v", b.config.SourceTopic, err)
			} else {
				b.config.Logf("message CDC bridge waiting for source topic %s", b.config.SourceTopic)
			}
		}
		if err := waitForInterval(ctx, b.config.ErrorBackoff); err != nil {
			return err
		}
	}
}

func topicExists(ctx context.Context, brokers []string, topic string) (bool, error) {
	if len(brokers) == 0 {
		return false, errors.New("kafka brokers are required")
	}
	dialer := &kafkago.Dialer{Timeout: 5 * time.Second}
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := dialer.DialContext(dialCtx, "tcp", brokers[0])
	if err != nil {
		return false, err
	}
	defer conn.Close()
	partitions, err := conn.ReadPartitions(topic)
	if err != nil {
		return false, err
	}
	for _, partition := range partitions {
		if partition.Topic == topic {
			return true, nil
		}
	}
	return false, nil
}

func (b *Bridge) runOnce(ctx context.Context) error {
	source, err := b.reader.FetchMessage(ctx)
	if err != nil {
		return err
	}
	sources := []kafkago.Message{source}
	for len(sources) < b.config.ReorderMaxRecords {
		fetchCtx, cancel := context.WithTimeout(ctx, b.config.ReorderFlushDelay)
		next, err := b.reader.FetchMessage(fetchCtx)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				break
			}
			if errors.Is(ctx.Err(), context.Canceled) {
				return context.Canceled
			}
			return err
		}
		sources = append(sources, next)
	}
	return b.publishAndCommitBatch(ctx, sources)
}

func (b *Bridge) publishAndCommitBatch(ctx context.Context, sources []kafkago.Message) error {
	items := make([]batchItem, 0, len(sources))
	for _, source := range sources {
		record, publish, order, err := BuildRecordWithOrder(source.Value)
		if err != nil {
			return err
		}
		items = append(items, batchItem{
			source:  source,
			record:  record,
			order:   order,
			publish: publish,
		})
	}
	publishable := make([]batchItem, 0, len(items))
	for _, item := range items {
		if item.publish {
			publishable = append(publishable, item)
		}
	}
	sortBatchForPublish(publishable)
	for _, item := range publishable {
		if err := b.writer.Publish(ctx, b.config.TargetTopic, item.record.Key, item.record.Value); err != nil {
			return err
		}
	}
	return b.reader.CommitMessages(ctx, sources...)
}

func sortBatchForPublish(items []batchItem) {
	sort.SliceStable(items, func(i int, j int) bool {
		left := items[i]
		right := items[j]
		if left.order.PartitionKey != right.order.PartitionKey {
			return left.order.PartitionKey < right.order.PartitionKey
		}
		if left.order.AggregateVersion != right.order.AggregateVersion {
			return left.order.AggregateVersion < right.order.AggregateVersion
		}
		if left.source.Partition != right.source.Partition {
			return left.source.Partition < right.source.Partition
		}
		return left.source.Offset < right.source.Offset
	})
}

func BuildRecord(value []byte) (types.KafkaPublishRecord, bool, error) {
	record, publish, _, err := BuildRecordWithOrder(value)
	return record, publish, err
}

func BuildRecordWithOrder(value []byte) (types.KafkaPublishRecord, bool, recordOrder, error) {
	if len(value) == 0 || string(value) == "null" {
		return types.KafkaPublishRecord{}, false, recordOrder{}, nil
	}
	var envelope debziumEnvelope
	if err := json.Unmarshal(value, &envelope); err != nil {
		return types.KafkaPublishRecord{}, false, recordOrder{}, fmt.Errorf("decode debezium envelope: %w", err)
	}
	op := envelope.Op
	after := envelope.After
	if envelope.Payload != nil {
		op = envelope.Payload.Op
		after = envelope.Payload.After
	}
	switch op {
	case "c", "r":
	case "u", "d":
		return types.KafkaPublishRecord{}, false, recordOrder{}, nil
	default:
		return types.KafkaPublishRecord{}, false, recordOrder{}, fmt.Errorf("unsupported debezium op %q", op)
	}
	if len(after) == 0 {
		return types.KafkaPublishRecord{}, false, recordOrder{}, nil
	}
	message, err := outboxMessageFromRow(after)
	if err != nil {
		return types.KafkaPublishRecord{}, false, recordOrder{}, err
	}
	valueBytes, err := outbox.BuildKafkaValue(message)
	if err != nil {
		return types.KafkaPublishRecord{}, false, recordOrder{}, err
	}
	return types.KafkaPublishRecord{
			Key:   []byte(message.PartitionKey),
			Value: valueBytes,
		}, true, recordOrder{
			PartitionKey:     message.PartitionKey,
			AggregateVersion: message.AggregateVersion,
		}, nil
}

func outboxMessageFromRow(row map[string]json.RawMessage) (types.OutboxMessage, error) {
	tenantID, err := requiredString(row, "tenant_id")
	if err != nil {
		return types.OutboxMessage{}, err
	}
	conversationID, err := requiredString(row, "conversation_id")
	if err != nil {
		return types.OutboxMessage{}, err
	}
	seq, err := requiredInt64(row, "seq")
	if err != nil {
		return types.OutboxMessage{}, err
	}
	eventID, err := requiredString(row, "event_id")
	if err != nil {
		return types.OutboxMessage{}, err
	}
	eventType, err := requiredString(row, "event_type")
	if err != nil {
		return types.OutboxMessage{}, err
	}
	eventVersion, err := requiredString(row, "event_version")
	if err != nil {
		return types.OutboxMessage{}, err
	}
	mappingVersion, err := requiredString(row, "mapping_version")
	if err != nil {
		return types.OutboxMessage{}, err
	}
	traceID, err := requiredString(row, "trace_id")
	if err != nil {
		return types.OutboxMessage{}, err
	}
	payloadJSON, err := requiredJSON(row, "payload_json")
	if err != nil {
		return types.OutboxMessage{}, err
	}
	fanoutPolicyVersion, err := requiredInt64(row, "fanout_policy_version")
	if err != nil {
		return types.OutboxMessage{}, err
	}
	permissionVersion, err := optionalInt64(row, "permission_version")
	if err != nil {
		return types.OutboxMessage{}, err
	}
	occurredAt, err := optionalTime(row, "created_at")
	if err != nil {
		return types.OutboxMessage{}, err
	}
	partitionKey := optionalString(row, "partition_key")
	if partitionKey == "" {
		partitionKey = tenantID + ":" + conversationID
	}
	correlationID := optionalString(row, "correlation_id")
	if correlationID == "" {
		correlationID = traceID
	}
	causationID := optionalString(row, "causation_id")
	if causationID == "" {
		causationID = eventID
	}
	producer := optionalString(row, "producer")
	if producer == "" {
		producer = "message-service"
	}
	return types.OutboxMessage{
		EventID:             types.EventID(eventID),
		TenantID:            types.TenantID(tenantID),
		ConversationID:      types.ConversationID(conversationID),
		AggregateVersion:    seq,
		EventType:           types.TimelineEventType(eventType),
		EventVersion:        eventVersion,
		PartitionKey:        partitionKey,
		MappingVersion:      mappingVersion,
		CorrelationID:       correlationID,
		CausationID:         causationID,
		Producer:            producer,
		PayloadJSON:         payloadJSON,
		TraceID:             traceID,
		FanoutMode:          types.FanoutMode(optionalString(row, "fanout_mode")),
		FanoutPolicyVersion: fanoutPolicyVersion,
		PermissionVersion:   permissionVersion,
		Classification:      optionalString(row, "classification"),
		OccurredAt:          occurredAt,
	}, nil
}

func normalizeConfig(config Config) Config {
	if config.SourceTopic == "" {
		config.SourceTopic = DefaultSourceTopic
	}
	if config.TargetTopic == "" {
		config.TargetTopic = DefaultTargetTopic
	}
	if config.GroupID == "" {
		config.GroupID = "message-cdc-bridge"
	}
	if config.ErrorBackoff <= 0 {
		config.ErrorBackoff = time.Second
	}
	if config.ReorderFlushDelay <= 0 {
		config.ReorderFlushDelay = 3 * time.Second
	}
	if config.ReorderMaxRecords <= 0 {
		config.ReorderMaxRecords = 10000
	}
	return config
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

func requiredString(row map[string]json.RawMessage, key string) (string, error) {
	value := optionalString(row, key)
	if value == "" {
		return "", fmt.Errorf("debezium row missing %s", key)
	}
	return value, nil
}

func optionalString(row map[string]json.RawMessage, key string) string {
	raw, ok := row[key]
	if !ok || isJSONNull(raw) {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return value
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		return number.String()
	}
	return strings.Trim(string(raw), `"`)
}

func requiredInt64(row map[string]json.RawMessage, key string) (int64, error) {
	value, err := optionalInt64(row, key)
	if err != nil {
		return 0, err
	}
	if value == 0 {
		return 0, fmt.Errorf("debezium row missing %s", key)
	}
	return value, nil
}

func optionalInt64(row map[string]json.RawMessage, key string) (int64, error) {
	raw, ok := row[key]
	if !ok || isJSONNull(raw) {
		return 0, nil
	}
	var number int64
	if err := json.Unmarshal(raw, &number); err == nil {
		return number, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if text == "" {
			return 0, nil
		}
		value, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse %s: %w", key, err)
		}
		return value, nil
	}
	return 0, fmt.Errorf("parse %s: unsupported JSON value", key)
}

func requiredJSON(row map[string]json.RawMessage, key string) ([]byte, error) {
	raw, ok := row[key]
	if !ok || isJSONNull(raw) {
		return nil, fmt.Errorf("debezium row missing %s", key)
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("debezium row missing %s", key)
		}
		return []byte(text), nil
	}
	return append([]byte(nil), raw...), nil
}

func optionalTime(row map[string]json.RawMessage, key string) (time.Time, error) {
	raw, ok := row[key]
	if !ok || isJSONNull(raw) {
		return time.Time{}, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if text == "" {
			return time.Time{}, nil
		}
		if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
			return parsed, nil
		}
		if parsed, err := time.Parse("2006-01-02T15:04:05.999999Z07:00", text); err == nil {
			return parsed, nil
		}
		return time.Time{}, fmt.Errorf("parse %s: unsupported timestamp %q", key, text)
	}
	return time.Time{}, nil
}

func isJSONNull(raw json.RawMessage) bool {
	return len(raw) == 0 || string(raw) == "null"
}
