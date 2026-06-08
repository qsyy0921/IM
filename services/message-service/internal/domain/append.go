package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"time"

	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

type AppendMessageInput struct {
	Command      types.SendMessageCommand
	Permission   types.PermissionDecision
	Conversation types.ConversationSendContext
}

type AppendMessageResult struct {
	MessageID        types.MessageID
	ConversationSeq  int64
	AcceptedAt       time.Time
	IdempotentReplay bool
}

type AppendMessageRecord struct {
	Message  Message
	Timeline TimelineEvent
	Outbox   OutboxEvent
}

func NewAppendMessageRecord(
	input AppendMessageInput,
	messageID types.MessageID,
	eventID types.EventID,
	seq int64,
	acceptedAt time.Time,
) (AppendMessageRecord, error) {
	payloadJSON, err := NormalizePayloadJSON(input.Command.PayloadJSON)
	if err != nil {
		return AppendMessageRecord{}, err
	}
	commandHash, err := ComputeSendMessageCommandHash(input.Command)
	if err != nil {
		return AppendMessageRecord{}, err
	}

	classification := input.Permission.Classification
	if classification == "" {
		classification = "UNCLASSIFIED"
	}

	traceID := input.Command.AuthContext.TraceID
	if traceID == "" {
		traceID = input.Command.AuthContext.RequestID
	}

	partitionKey := string(input.Command.AuthContext.TenantID) + ":" + string(input.Command.ConversationID)
	eventPayload, err := buildMessagePersistedPayload(input, messageID, seq, commandHash, payloadJSON, acceptedAt)
	if err != nil {
		return AppendMessageRecord{}, err
	}

	return AppendMessageRecord{
		Message: Message{
			TenantID:       input.Command.AuthContext.TenantID,
			ConversationID: input.Command.ConversationID,
			Seq:            seq,
			MessageID:      messageID,
			SenderID:       input.Command.AuthContext.UserID,
			DeviceID:       input.Command.AuthContext.DeviceID,
			ClientMsgID:    input.Command.ClientMsgID,
			CommandHash:    commandHash,
			MessageType:    input.Command.MessageType,
			PayloadJSON:    payloadJSON,
			Status:         MessageStatusNormal,
			CreatedAt:      acceptedAt,
		},
		Timeline: TimelineEvent{
			EventID:             eventID,
			EventType:           types.TimelineEventMessagePersisted,
			EventVersion:        "v1",
			TenantID:            input.Command.AuthContext.TenantID,
			ConversationID:      input.Command.ConversationID,
			ConversationSeq:     seq,
			MessageID:           messageID,
			FanoutMode:          input.Conversation.FanoutMode,
			FanoutPolicyVersion: input.Conversation.FanoutPolicyVersion,
			PermissionVersion:   input.Permission.PermissionVersion,
			Classification:      classification,
			MappingVersion:      "message.persisted.v1",
			TraceID:             traceID,
			PayloadJSON:         eventPayload,
			CreatedAt:           acceptedAt,
		},
		Outbox: OutboxEvent{
			EventID:          eventID,
			TenantID:         input.Command.AuthContext.TenantID,
			ConversationID:   input.Command.ConversationID,
			AggregateVersion: seq,
			EventType:        types.TimelineEventMessagePersisted,
			EventVersion:     "v1",
			PartitionKey:     partitionKey,
			MappingVersion:   "message.persisted.v1",
			CorrelationID:    firstNonEmpty(input.Command.AuthContext.RequestID, traceID, string(input.Command.ClientMsgID)),
			CausationID:      firstNonEmpty(string(input.Command.ClientMsgID), input.Command.AuthContext.RequestID, traceID),
			Producer:         "message-service",
			PayloadJSON:      eventPayload,
			TraceID:          traceID,
		},
	}, nil
}

func buildMessagePersistedPayload(
	input AppendMessageInput,
	messageID types.MessageID,
	seq int64,
	commandHash string,
	payloadJSON []byte,
	acceptedAt time.Time,
) ([]byte, error) {
	return json.Marshal(map[string]any{
		"message_id":       messageID,
		"conversation_id":  input.Command.ConversationID,
		"conversation_seq": seq,
		"sender_id":        input.Command.AuthContext.UserID,
		"device_id":        input.Command.AuthContext.DeviceID,
		"client_msg_id":    input.Command.ClientMsgID,
		"command_hash":     commandHash,
		"message_type":     input.Command.MessageType,
		"payload":          json.RawMessage(payloadJSON),
		"attachment_ids":   sortedCopy(input.Command.AttachmentIDs),
		"accepted_at":      acceptedAt.UTC().Format(time.RFC3339Nano),
	})
}

func ComputeSendMessageCommandHash(command types.SendMessageCommand) (string, error) {
	payloadJSON, err := NormalizePayloadJSON(command.PayloadJSON)
	if err != nil {
		return "", err
	}
	hashInput := struct {
		TenantID       types.TenantID       `json:"tenant_id"`
		ConversationID types.ConversationID `json:"conversation_id"`
		SenderID       types.UserID         `json:"sender_id"`
		DeviceID       types.DeviceID       `json:"device_id"`
		ClientMsgID    types.ClientMsgID    `json:"client_msg_id"`
		MessageType    types.MessageType    `json:"message_type"`
		Payload        json.RawMessage      `json:"payload"`
		AttachmentIDs  []string             `json:"sorted_attachment_ids"`
	}{
		TenantID:       command.AuthContext.TenantID,
		ConversationID: command.ConversationID,
		SenderID:       command.AuthContext.UserID,
		DeviceID:       command.AuthContext.DeviceID,
		ClientMsgID:    command.ClientMsgID,
		MessageType:    command.MessageType,
		Payload:        json.RawMessage(payloadJSON),
		AttachmentIDs:  sortedCopy(command.AttachmentIDs),
	}
	encoded, err := json.Marshal(hashInput)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func NormalizePayloadJSON(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return []byte("{}"), nil
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, errors.New("payload_json must be valid JSON")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("payload_json must contain exactly one JSON value")
	}
	compact, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return compact, nil
}

func sortedCopy(values []string) []string {
	copied := append([]string(nil), values...)
	sort.Strings(copied)
	return copied
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
