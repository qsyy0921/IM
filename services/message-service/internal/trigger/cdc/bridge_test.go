package cdc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	conversationtimelinev1 "github.com/qsyy0921/IM/schemas/kafka"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
	kafkago "github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
)

func TestNewBridgeDefersReaderUntilSourceTopicExists(t *testing.T) {
	bridge, err := NewBridge(Config{
		Brokers:     []string{"127.0.0.1:1"},
		SourceTopic: "source-topic",
		TargetTopic: "target-topic",
		GroupID:     "bridge-test",
	})
	if err != nil {
		t.Fatalf("NewBridge returned error: %v", err)
	}
	defer bridge.Close()
	if bridge.reader != nil {
		t.Fatal("reader should be created after source topic readiness, not during NewBridge")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	if err := bridge.ensureReader(ctx); err == nil {
		t.Fatal("ensureReader unexpectedly succeeded without a reachable broker/source topic")
	}
	if bridge.reader != nil {
		t.Fatal("reader should remain nil when source topic readiness fails")
	}
}

func TestBuildRecordPublishesTimelineInsert(t *testing.T) {
	payloadJSON := `{"message_id":"msg-1","conversation_id":"conv-1","conversation_seq":7,"sender_id":"user-1","device_id":"device-1","client_msg_id":"client-1","command_hash":"hash-1","message_type":"TEXT","payload":{"text":"hello"},"attachment_ids":[],"accepted_at":"2026-07-02T12:00:00Z"}`
	envelope := map[string]any{
		"op": "c",
		"after": map[string]any{
			"tenant_id":             "tenant-1",
			"conversation_id":       "conv-1",
			"seq":                   7,
			"event_id":              "event-1",
			"event_type":            "message.persisted.v1",
			"event_version":         "v1",
			"fanout_mode":           "READ_FANOUT",
			"fanout_policy_version": 3,
			"permission_version":    9,
			"classification":        "INTERNAL",
			"mapping_version":       "message.persisted.v1",
			"trace_id":              "trace-1",
			"partition_key":         "tenant-1:conv-1",
			"correlation_id":        "request-1",
			"causation_id":          "client-1",
			"producer":              "message-service",
			"payload_json":          payloadJSON,
			"created_at":            "2026-07-02T12:00:00Z",
		},
	}
	value, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	record, publish, err := BuildRecord(value)
	if err != nil {
		t.Fatalf("BuildRecord returned error: %v", err)
	}
	if !publish {
		t.Fatal("expected publish")
	}
	if string(record.Key) != "tenant-1:conv-1" {
		t.Fatalf("unexpected key: %q", string(record.Key))
	}

	var event conversationtimelinev1.ConversationTimelineEvent
	if err := proto.Unmarshal(record.Value, &event); err != nil {
		t.Fatalf("unmarshal protobuf: %v", err)
	}
	if event.GetEventId() != "event-1" ||
		event.GetTenantId() != "tenant-1" ||
		event.GetAggregateId() != "conv-1" ||
		event.GetAggregateVersion() != 7 ||
		event.GetPartitionKey() != "tenant-1:conv-1" ||
		event.GetCorrelationId() != "request-1" ||
		event.GetCausationId() != "client-1" ||
		event.GetProducer() != "message-service" {
		t.Fatalf("unexpected event envelope: %+v", &event)
	}
	if event.GetMetadata().GetFanoutMode() != "READ_FANOUT" ||
		event.GetMetadata().GetFanoutPolicyVersion() != 3 ||
		event.GetMetadata().GetPermissionVersion() != 9 {
		t.Fatalf("unexpected metadata: %+v", event.GetMetadata())
	}
	payload := event.GetMessagePersisted()
	if payload == nil {
		t.Fatal("missing message persisted payload")
	}
	if payload.GetMessageId() != "msg-1" ||
		payload.GetConversationSeq() != 7 ||
		payload.GetSenderId() != "user-1" ||
		payload.GetPayload().GetFields()["text"].GetStringValue() != "hello" {
		t.Fatalf("unexpected message persisted payload: %+v", payload)
	}
}

func TestBuildRecordWithOrderReturnsTimelineOrderKey(t *testing.T) {
	value := []byte(`{
		"op":"c",
		"after":{
			"tenant_id":"tenant-1",
			"conversation_id":"conv-1",
			"seq":42,
			"event_id":"event-order",
			"event_type":"message.persisted.v1",
			"event_version":"v1",
			"fanout_mode":"READ_FANOUT",
			"fanout_policy_version":3,
			"permission_version":9,
			"classification":"INTERNAL",
			"mapping_version":"message.persisted.v1",
			"trace_id":"trace-1",
			"partition_key":"tenant-1:conv-1",
			"payload_json":"{\"message_id\":\"msg-order\",\"conversation_id\":\"conv-1\",\"conversation_seq\":42,\"sender_id\":\"user-1\",\"device_id\":\"device-1\",\"client_msg_id\":\"client-order\",\"command_hash\":\"hash-order\",\"message_type\":\"TEXT\",\"payload\":{\"text\":\"ordered\"},\"attachment_ids\":[],\"accepted_at\":\"2026-07-02T12:00:00Z\"}",
			"created_at":"2026-07-02T12:00:00Z"
		}
	}`)
	record, publish, order, err := BuildRecordWithOrder(value)
	if err != nil {
		t.Fatalf("BuildRecordWithOrder returned error: %v", err)
	}
	if !publish {
		t.Fatal("expected publish")
	}
	if string(record.Key) != "tenant-1:conv-1" {
		t.Fatalf("unexpected key: %q", string(record.Key))
	}
	if order.PartitionKey != "tenant-1:conv-1" || order.AggregateVersion != 42 {
		t.Fatalf("unexpected order key: %+v", order)
	}
}

func TestSortBatchForPublishOrdersByConversationSeq(t *testing.T) {
	items := []batchItem{
		{
			source: kafkago.Message{Partition: 0, Offset: 1},
			record: types.KafkaPublishRecord{
				Key:   []byte("tenant:conv"),
				Value: []byte("seq-3"),
			},
			order:   recordOrder{PartitionKey: "tenant:conv", AggregateVersion: 3},
			publish: true,
		},
		{
			source: kafkago.Message{Partition: 0, Offset: 2},
			record: types.KafkaPublishRecord{
				Key:   []byte("tenant:conv"),
				Value: []byte("seq-1"),
			},
			order:   recordOrder{PartitionKey: "tenant:conv", AggregateVersion: 1},
			publish: true,
		},
		{
			source: kafkago.Message{Partition: 0, Offset: 3},
			record: types.KafkaPublishRecord{
				Key:   []byte("tenant:conv"),
				Value: []byte("seq-2"),
			},
			order:   recordOrder{PartitionKey: "tenant:conv", AggregateVersion: 2},
			publish: true,
		},
	}

	sortBatchForPublish(items)

	got := []string{string(items[0].record.Value), string(items[1].record.Value), string(items[2].record.Value)}
	want := []string{"seq-1", "seq-2", "seq-3"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected order at %d: got %v want %v", i, got, want)
		}
	}
}

func TestBuildRecordPublishesObjectPayloadJSON(t *testing.T) {
	value := []byte(`{
		"op":"c",
		"after":{
			"tenant_id":"tenant-1",
			"conversation_id":"conv-1",
			"seq":8,
			"event_id":"event-2",
			"event_type":"message.persisted.v1",
			"event_version":"v1",
			"fanout_mode":"READ_FANOUT",
			"fanout_policy_version":3,
			"permission_version":9,
			"classification":"INTERNAL",
			"mapping_version":"message.persisted.v1",
			"trace_id":"trace-1",
			"payload_json":{
				"message_id":"msg-2",
				"conversation_id":"conv-1",
				"conversation_seq":8,
				"sender_id":"user-1",
				"device_id":"device-1",
				"client_msg_id":"client-2",
				"command_hash":"hash-2",
				"message_type":"TEXT",
				"payload":{"text":"object"},
				"attachment_ids":[],
				"accepted_at":"2026-07-02T12:00:01Z"
			},
			"created_at":"2026-07-02T12:00:01Z"
		}
	}`)

	record, publish, err := BuildRecord(value)
	if err != nil {
		t.Fatalf("BuildRecord returned error: %v", err)
	}
	if !publish {
		t.Fatal("expected publish")
	}
	if string(record.Key) != "tenant-1:conv-1" {
		t.Fatalf("unexpected fallback key: %q", string(record.Key))
	}
}

func TestBuildRecordPublishesSchemaWrappedEnvelope(t *testing.T) {
	value := []byte(`{
		"schema":{"type":"struct"},
		"payload":{
			"op":"c",
			"after":{
				"tenant_id":"tenant-1",
				"conversation_id":"conv-1",
				"seq":9,
				"event_id":"event-3",
				"event_type":"message.persisted.v1",
				"event_version":"v1",
				"fanout_mode":"READ_FANOUT",
				"fanout_policy_version":3,
				"permission_version":9,
				"classification":"INTERNAL",
				"mapping_version":"message.persisted.v1",
				"trace_id":"trace-1",
				"partition_key":"tenant-1:conv-1",
				"correlation_id":"request-3",
				"causation_id":"client-3",
				"producer":"message-service",
				"payload_json":"{\"message_id\":\"msg-3\",\"conversation_id\":\"conv-1\",\"conversation_seq\":9,\"sender_id\":\"user-1\",\"device_id\":\"device-1\",\"client_msg_id\":\"client-3\",\"command_hash\":\"hash-3\",\"message_type\":\"TEXT\",\"payload\":{\"text\":\"wrapped\"},\"attachment_ids\":[],\"accepted_at\":\"2026-07-02T12:00:02Z\"}",
				"created_at":"2026-07-02T12:00:02Z"
			}
		}
	}`)

	record, publish, err := BuildRecord(value)
	if err != nil {
		t.Fatalf("BuildRecord returned error: %v", err)
	}
	if !publish {
		t.Fatal("expected publish")
	}
	var event conversationtimelinev1.ConversationTimelineEvent
	if err := proto.Unmarshal(record.Value, &event); err != nil {
		t.Fatalf("unmarshal protobuf: %v", err)
	}
	if event.GetEventId() != "event-3" || event.GetMessagePersisted().GetPayload().GetFields()["text"].GetStringValue() != "wrapped" {
		t.Fatalf("unexpected schema-wrapped event: %+v", &event)
	}
}

func TestBuildRecordSkipsDeletesAndTombstones(t *testing.T) {
	for _, value := range [][]byte{
		nil,
		[]byte(`null`),
		[]byte(`{"op":"d","after":null}`),
		[]byte(`{"op":"u","after":{"tenant_id":"tenant-1"}}`),
	} {
		_, publish, err := BuildRecord(value)
		if err != nil {
			t.Fatalf("BuildRecord returned error for %s: %v", string(value), err)
		}
		if publish {
			t.Fatalf("expected skip for %s", string(value))
		}
	}
}
