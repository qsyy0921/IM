package cdc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	Logf         func(format string, args ...any)
}

type Bridge struct {
	reader *kafkago.Reader
	writer *kafkainfra.WriterProducer
	config Config
}

type debziumEnvelope struct {
	Op    string                     `json:"op"`
	After map[string]json.RawMessage `json:"after"`
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
		reader: kafkago.NewReader(kafkago.ReaderConfig{
			Brokers:        config.Brokers,
			Topic:          config.SourceTopic,
			GroupID:        config.GroupID,
			MinBytes:       1,
			MaxBytes:       10e6,
			CommitInterval: 0,
			MaxWait:        500 * time.Millisecond,
		}),
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
	if b == nil || b.reader == nil || b.writer == nil {
		return errors.New("cdc bridge is not configured")
	}
	for {
		if err := b.runOnce(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			if b.config.Logf != nil {
				b.config.Logf("message CDC bridge retrying after error: %v", err)
			}
			if err := waitForInterval(ctx, b.config.ErrorBackoff); err != nil {
				return err
			}
		}
	}
}

func (b *Bridge) runOnce(ctx context.Context) error {
	source, err := b.reader.FetchMessage(ctx)
	if err != nil {
		return err
	}
	record, publish, err := BuildRecord(source.Value)
	if err != nil {
		return err
	}
	if publish {
		if err := b.writer.Publish(ctx, b.config.TargetTopic, record.Key, record.Value); err != nil {
			return err
		}
	}
	return b.reader.CommitMessages(ctx, source)
}

func BuildRecord(value []byte) (types.KafkaPublishRecord, bool, error) {
	if len(value) == 0 || string(value) == "null" {
		return types.KafkaPublishRecord{}, false, nil
	}
	var envelope debziumEnvelope
	if err := json.Unmarshal(value, &envelope); err != nil {
		return types.KafkaPublishRecord{}, false, fmt.Errorf("decode debezium envelope: %w", err)
	}
	switch envelope.Op {
	case "c", "r":
	case "u", "d":
		return types.KafkaPublishRecord{}, false, nil
	default:
		return types.KafkaPublishRecord{}, false, fmt.Errorf("unsupported debezium op %q", envelope.Op)
	}
	if len(envelope.After) == 0 {
		return types.KafkaPublishRecord{}, false, nil
	}
	message, err := outboxMessageFromRow(envelope.After)
	if err != nil {
		return types.KafkaPublishRecord{}, false, err
	}
	valueBytes, err := outbox.BuildKafkaValue(message)
	if err != nil {
		return types.KafkaPublishRecord{}, false, err
	}
	return types.KafkaPublishRecord{
		Key:   []byte(message.PartitionKey),
		Value: valueBytes,
	}, true, nil
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
