package cdc

import (
	"encoding/json"
	"testing"

	conversationtimelinev1 "github.com/qsyy0921/IM/schemas/kafka"
	"google.golang.org/protobuf/proto"
)

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
