package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

const MessageChangeTypeRevoke = "REVOKE"

type RevokeMessageInput struct {
	Command      types.RevokeMessageCommand
	Permission   types.PermissionDecision
	Conversation types.ConversationSendContext
}

type MessageChangeResult struct {
	MessageID        types.MessageID
	ConversationSeq  int64
	ChangeVersion    int32
	AcceptedAt       time.Time
	IdempotentReplay bool
}

type MessageChangeRecord struct {
	MessageID       types.MessageID
	ConversationID  types.ConversationID
	ConversationSeq int64
	ChangeVersion   int32
	ChangedAt       time.Time
	CommandHash     string
	BeforePayload   []byte
	BeforeStatus    MessageStatus
	AfterStatus     MessageStatus
	ChangeType      string
	Timeline        TimelineEvent
	Outbox          OutboxEvent
}

func NewRevokeMessageRecord(
	input RevokeMessageInput,
	message Message,
	eventID types.EventID,
	seq int64,
	changeVersion int32,
	acceptedAt time.Time,
) (MessageChangeRecord, error) {
	commandHash, err := ComputeRevokeMessageCommandHash(input.Command)
	if err != nil {
		return MessageChangeRecord{}, err
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
	payloadJSON, err := buildMessageRevokedPayload(input, seq, changeVersion, acceptedAt)
	if err != nil {
		return MessageChangeRecord{}, err
	}

	return MessageChangeRecord{
		MessageID:       input.Command.MessageID,
		ConversationID:  input.Command.ConversationID,
		ConversationSeq: seq,
		ChangeVersion:   changeVersion,
		ChangedAt:       acceptedAt,
		CommandHash:     commandHash,
		BeforePayload:   append([]byte(nil), message.PayloadJSON...),
		BeforeStatus:    message.Status,
		AfterStatus:     MessageStatusRevoked,
		ChangeType:      MessageChangeTypeRevoke,
		Timeline: TimelineEvent{
			EventID:             eventID,
			EventType:           types.TimelineEventMessageRevoked,
			EventVersion:        "v1",
			TenantID:            input.Command.AuthContext.TenantID,
			ConversationID:      input.Command.ConversationID,
			ConversationSeq:     seq,
			MessageID:           input.Command.MessageID,
			FanoutMode:          input.Conversation.FanoutMode,
			FanoutPolicyVersion: input.Conversation.FanoutPolicyVersion,
			PermissionVersion:   input.Permission.PermissionVersion,
			Classification:      classification,
			MappingVersion:      "message.revoked.v1",
			TraceID:             traceID,
			PayloadJSON:         payloadJSON,
			CreatedAt:           acceptedAt,
		},
		Outbox: OutboxEvent{
			EventID:          eventID,
			TenantID:         input.Command.AuthContext.TenantID,
			ConversationID:   input.Command.ConversationID,
			AggregateVersion: seq,
			EventType:        types.TimelineEventMessageRevoked,
			EventVersion:     "v1",
			PartitionKey:     partitionKey,
			MappingVersion:   "message.revoked.v1",
			CorrelationID:    firstNonEmpty(input.Command.AuthContext.RequestID, traceID, input.Command.IdempotencyKey),
			CausationID:      firstNonEmpty(input.Command.IdempotencyKey, input.Command.AuthContext.RequestID, traceID),
			Producer:         "message-service",
			PayloadJSON:      payloadJSON,
			TraceID:          traceID,
		},
	}, nil
}

func ComputeRevokeMessageCommandHash(command types.RevokeMessageCommand) (string, error) {
	hashInput := struct {
		TenantID       types.TenantID       `json:"tenant_id"`
		ConversationID types.ConversationID `json:"conversation_id"`
		ActorID        types.UserID         `json:"actor_id"`
		DeviceID       types.DeviceID       `json:"device_id"`
		MessageID      types.MessageID      `json:"message_id"`
		IdempotencyKey string               `json:"idempotency_key"`
		Reason         string               `json:"reason"`
	}{
		TenantID:       command.AuthContext.TenantID,
		ConversationID: command.ConversationID,
		ActorID:        command.AuthContext.UserID,
		DeviceID:       command.AuthContext.DeviceID,
		MessageID:      command.MessageID,
		IdempotencyKey: command.IdempotencyKey,
		Reason:         command.Reason,
	}
	encoded, err := json.Marshal(hashInput)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func buildMessageRevokedPayload(
	input RevokeMessageInput,
	seq int64,
	changeVersion int32,
	acceptedAt time.Time,
) ([]byte, error) {
	return json.Marshal(map[string]any{
		"message_id":       input.Command.MessageID,
		"conversation_id":  input.Command.ConversationID,
		"conversation_seq": seq,
		"change_version":   changeVersion,
		"revoked_by":       input.Command.AuthContext.UserID,
		"reason":           input.Command.Reason,
		"revoked_at":       acceptedAt.UTC().Format(time.RFC3339Nano),
	})
}
