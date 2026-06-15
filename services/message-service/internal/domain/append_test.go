package domain

import (
	"encoding/json"
	"testing"

	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

func TestComputeSendMessageCommandHashCanonicalizesPayloadAndAttachments(t *testing.T) {
	base := testCommand()
	base.PayloadJSON = []byte(`{"z":{"b":2,"a":1},"a":[{"d":4,"c":3}]}`)
	base.AttachmentIDs = []string{"att-2", "att-1"}

	same := testCommand()
	same.PayloadJSON = []byte(`{ "a" : [ { "c" : 3, "d" : 4 } ], "z" : { "a" : 1, "b" : 2 } }`)
	same.AttachmentIDs = []string{"att-1", "att-2"}
	same.AuthContext.TraceID = "different-trace"
	same.AuthContext.RequestID = "different-request"

	baseHash, err := ComputeSendMessageCommandHash(base)
	if err != nil {
		t.Fatalf("hash base command: %v", err)
	}
	sameHash, err := ComputeSendMessageCommandHash(same)
	if err != nil {
		t.Fatalf("hash same command: %v", err)
	}
	if baseHash != sameHash {
		t.Fatalf("expected equivalent commands to have same hash: %s != %s", baseHash, sameHash)
	}
}

func TestComputeSendMessageCommandHashChangesWithPayload(t *testing.T) {
	first := testCommand()
	first.PayloadJSON = []byte(`{"text":"one"}`)
	second := testCommand()
	second.PayloadJSON = []byte(`{"text":"two"}`)

	firstHash, err := ComputeSendMessageCommandHash(first)
	if err != nil {
		t.Fatalf("hash first command: %v", err)
	}
	secondHash, err := ComputeSendMessageCommandHash(second)
	if err != nil {
		t.Fatalf("hash second command: %v", err)
	}
	if firstHash == secondHash {
		t.Fatalf("expected different payloads to produce different hashes")
	}
}

func TestNewAppendMessageRecordBuildsPersistedEvent(t *testing.T) {
	input := AppendMessageInput{
		Command: testCommand(),
		Permission: types.PermissionDecision{
			Allowed:           true,
			PermissionVersion: 7,
			Classification:    "INTERNAL",
		},
		Conversation: types.ConversationSendContext{
			ConversationMode:    types.ConversationModeLocalRowLock,
			FanoutMode:          types.FanoutModeWriteFanout,
			FanoutPolicyVersion: 3,
		},
	}

	record, err := NewAppendMessageRecord(input, "msg-1", "event-1", 10, input.Command.ReceivedAt)
	if err != nil {
		t.Fatalf("build append record: %v", err)
	}
	if record.Message.Seq != 10 || record.Timeline.ConversationSeq != 10 || record.Outbox.AggregateVersion != 10 {
		t.Fatalf("expected seq propagated to message, timeline, and outbox")
	}
	if record.Message.CommandHash == "" {
		t.Fatalf("expected command hash")
	}
	if record.Timeline.EventType != types.TimelineEventMessagePersisted {
		t.Fatalf("unexpected timeline event type: %s", record.Timeline.EventType)
	}
	if record.Outbox.PartitionKey != "tenant-1:conv-1" {
		t.Fatalf("unexpected partition key: %s", record.Outbox.PartitionKey)
	}

	var payload map[string]any
	if err := json.Unmarshal(record.Outbox.PayloadJSON, &payload); err != nil {
		t.Fatalf("decode outbox payload: %v", err)
	}
	if payload["command_hash"] != record.Message.CommandHash {
		t.Fatalf("outbox payload should include command_hash")
	}
	if payload["message_id"] != "msg-1" || int64(payload["conversation_seq"].(float64)) != 10 {
		t.Fatalf("unexpected outbox payload: %+v", payload)
	}
	if _, ok := payload["tenant_id"]; ok {
		t.Fatalf("outbox payload should be MessagePersistedV1 body, not full envelope: %+v", payload)
	}
}

func TestNewAppendMessageRecordBuildsAttachmentMessageEvent(t *testing.T) {
	command := testCommand()
	command.MessageType = types.MessageTypeImage
	command.PayloadJSON = []byte(`{"caption":"hello","height":480,"width":640}`)
	command.AttachmentIDs = []string{"image-2", "image-1"}
	input := AppendMessageInput{
		Command: command,
		Permission: types.PermissionDecision{
			Allowed:           true,
			PermissionVersion: 7,
			Classification:    "INTERNAL",
		},
		Conversation: types.ConversationSendContext{
			ConversationMode:    types.ConversationModeLocalRowLock,
			FanoutMode:          types.FanoutModeWriteFanout,
			FanoutPolicyVersion: 3,
		},
	}

	record, err := NewAppendMessageRecord(input, "msg-image", "event-image", 11, input.Command.ReceivedAt)
	if err != nil {
		t.Fatalf("build image append record: %v", err)
	}
	if record.Message.MessageType != types.MessageTypeImage {
		t.Fatalf("unexpected message type: %s", record.Message.MessageType)
	}
	var payload map[string]any
	if err := json.Unmarshal(record.Outbox.PayloadJSON, &payload); err != nil {
		t.Fatalf("decode outbox payload: %v", err)
	}
	if payload["message_type"] != string(types.MessageTypeImage) {
		t.Fatalf("unexpected event message type: %+v", payload)
	}
	attachments, ok := payload["attachment_ids"].([]any)
	if !ok || len(attachments) != 2 || attachments[0] != "image-1" || attachments[1] != "image-2" {
		t.Fatalf("unexpected sorted attachments: %+v", payload["attachment_ids"])
	}
}

func testCommand() types.SendMessageCommand {
	return types.SendMessageCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-1",
			UserID:    "user-1",
			DeviceID:  "device-1",
			SessionID: "session-1",
			TraceID:   "trace-1",
			RequestID: "request-1",
		},
		ConversationID: "conv-1",
		ClientMsgID:    "client-1",
		MessageType:    types.MessageTypeText,
		PayloadJSON:    []byte(`{"text":"hello"}`),
	}
}
