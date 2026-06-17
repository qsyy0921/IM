package outbox

import (
	"encoding/json"
	"errors"
	"time"

	conversationtimelinev1 "github.com/qsyy0921/IM/schemas/kafka"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func BuildKafkaValue(message types.OutboxMessage) ([]byte, error) {
	event, err := BuildConversationTimelineEvent(message)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(event)
}

func BuildConversationTimelineEvent(message types.OutboxMessage) (*conversationtimelinev1.ConversationTimelineEvent, error) {
	switch message.EventType {
	case types.TimelineEventMessagePersisted:
		payload, err := decodeMessagePersistedPayload(message.PayloadJSON)
		if err != nil {
			return nil, err
		}
		acceptedAt, err := time.Parse(time.RFC3339Nano, payload.AcceptedAt)
		if err != nil {
			return nil, err
		}
		payloadStruct, err := structFromRawJSON(payload.Payload)
		if err != nil {
			return nil, err
		}
		occurredAt := message.OccurredAt
		if occurredAt.IsZero() {
			occurredAt = acceptedAt
		}
		event := buildTimelineEnvelope(message, occurredAt)
		event.Payload = &conversationtimelinev1.ConversationTimelineEvent_MessagePersisted{
			MessagePersisted: &conversationtimelinev1.MessagePersistedV1{
				MessageId:       payload.MessageID,
				ConversationId:  payload.ConversationID,
				ConversationSeq: payload.ConversationSeq,
				SenderId:        payload.SenderID,
				DeviceId:        payload.DeviceID,
				ClientMsgId:     payload.ClientMsgID,
				CommandHash:     payload.CommandHash,
				MessageType:     payload.MessageType,
				Payload:         payloadStruct,
				AttachmentIds:   payload.AttachmentIDs,
				AcceptedAt:      timestamppb.New(acceptedAt),
			},
		}
		return event, nil
	case types.TimelineEventMessageEdited:
		payload, err := decodeMessageEditedPayload(message.PayloadJSON)
		if err != nil {
			return nil, err
		}
		editedAt, err := time.Parse(time.RFC3339Nano, payload.EditedAt)
		if err != nil {
			return nil, err
		}
		beforePayload, err := structFromRawJSON(payload.BeforePayload)
		if err != nil {
			return nil, err
		}
		afterPayload, err := structFromRawJSON(payload.AfterPayload)
		if err != nil {
			return nil, err
		}
		occurredAt := message.OccurredAt
		if occurredAt.IsZero() {
			occurredAt = editedAt
		}
		event := buildTimelineEnvelope(message, occurredAt)
		event.Payload = &conversationtimelinev1.ConversationTimelineEvent_MessageEdited{
			MessageEdited: &conversationtimelinev1.MessageEditedV1{
				MessageId:       payload.MessageID,
				ConversationId:  payload.ConversationID,
				ConversationSeq: payload.ConversationSeq,
				ChangeVersion:   payload.ChangeVersion,
				EditedBy:        payload.EditedBy,
				BeforePayload:   beforePayload,
				AfterPayload:    afterPayload,
				Reason:          payload.Reason,
				EditedAt:        timestamppb.New(editedAt),
			},
		}
		return event, nil
	case types.TimelineEventMessageRevoked:
		payload, err := decodeMessageRevokedPayload(message.PayloadJSON)
		if err != nil {
			return nil, err
		}
		revokedAt, err := time.Parse(time.RFC3339Nano, payload.RevokedAt)
		if err != nil {
			return nil, err
		}
		occurredAt := message.OccurredAt
		if occurredAt.IsZero() {
			occurredAt = revokedAt
		}
		event := buildTimelineEnvelope(message, occurredAt)
		event.Payload = &conversationtimelinev1.ConversationTimelineEvent_MessageRevoked{
			MessageRevoked: &conversationtimelinev1.MessageRevokedV1{
				MessageId:       payload.MessageID,
				ConversationId:  payload.ConversationID,
				ConversationSeq: payload.ConversationSeq,
				ChangeVersion:   payload.ChangeVersion,
				RevokedBy:       payload.RevokedBy,
				Reason:          payload.Reason,
				RevokedAt:       timestamppb.New(revokedAt),
			},
		}
		return event, nil
	case types.TimelineEventMessageDeleted:
		payload, err := decodeMessageDeletedPayload(message.PayloadJSON)
		if err != nil {
			return nil, err
		}
		deletedAt, err := time.Parse(time.RFC3339Nano, payload.DeletedAt)
		if err != nil {
			return nil, err
		}
		deleteScope, err := messageDeleteScope(payload.DeleteScope)
		if err != nil {
			return nil, err
		}
		occurredAt := message.OccurredAt
		if occurredAt.IsZero() {
			occurredAt = deletedAt
		}
		event := buildTimelineEnvelope(message, occurredAt)
		event.Payload = &conversationtimelinev1.ConversationTimelineEvent_MessageDeleted{
			MessageDeleted: &conversationtimelinev1.MessageDeletedV1{
				MessageId:       payload.MessageID,
				ConversationId:  payload.ConversationID,
				ConversationSeq: payload.ConversationSeq,
				ChangeVersion:   payload.ChangeVersion,
				DeletedBy:       payload.DeletedBy,
				DeleteScope:     deleteScope,
				Reason:          payload.Reason,
				DeletedAt:       timestamppb.New(deletedAt),
			},
		}
		return event, nil
	case types.TimelineEventConversationMemberJoined,
		types.TimelineEventConversationMemberLeft,
		types.TimelineEventConversationMemberRemoved,
		types.TimelineEventConversationMemberRoleChanged,
		types.TimelineEventConversationMemberBoundaryCancelled,
		types.TimelineEventConversationMemberOwnerTransferred:
		return buildMemberBoundaryTimelineEvent(message)
	default:
		return nil, errors.New("unsupported outbox event type")
	}
}

func buildTimelineEnvelope(message types.OutboxMessage, occurredAt time.Time) *conversationtimelinev1.ConversationTimelineEvent {
	return &conversationtimelinev1.ConversationTimelineEvent{
		EventId:          string(message.EventID),
		EventType:        string(message.EventType),
		EventVersion:     message.EventVersion,
		TenantId:         string(message.TenantID),
		AggregateType:    "conversation",
		AggregateId:      string(message.ConversationID),
		AggregateVersion: message.AggregateVersion,
		PartitionKey:     message.PartitionKey,
		MappingVersion:   message.MappingVersion,
		TraceId:          message.TraceID,
		CorrelationId:    message.CorrelationID,
		CausationId:      message.CausationID,
		Producer:         message.Producer,
		OccurredAt:       timestamppb.New(occurredAt),
		Metadata: &conversationtimelinev1.TimelineMetadata{
			FanoutMode:          string(message.FanoutMode),
			FanoutPolicyVersion: message.FanoutPolicyVersion,
			PermissionVersion:   message.PermissionVersion,
			Classification:      message.Classification,
			MappingVersion:      message.MappingVersion,
		},
	}
}

type messagePersistedPayload struct {
	MessageID       string          `json:"message_id"`
	ConversationID  string          `json:"conversation_id"`
	ConversationSeq int64           `json:"conversation_seq"`
	SenderID        string          `json:"sender_id"`
	DeviceID        string          `json:"device_id"`
	ClientMsgID     string          `json:"client_msg_id"`
	CommandHash     string          `json:"command_hash"`
	MessageType     string          `json:"message_type"`
	Payload         json.RawMessage `json:"payload"`
	AttachmentIDs   []string        `json:"attachment_ids"`
	AcceptedAt      string          `json:"accepted_at"`
}

type messageRevokedPayload struct {
	MessageID       string `json:"message_id"`
	ConversationID  string `json:"conversation_id"`
	ConversationSeq int64  `json:"conversation_seq"`
	ChangeVersion   int32  `json:"change_version"`
	RevokedBy       string `json:"revoked_by"`
	Reason          string `json:"reason"`
	RevokedAt       string `json:"revoked_at"`
}

type messageEditedPayload struct {
	MessageID       string          `json:"message_id"`
	ConversationID  string          `json:"conversation_id"`
	ConversationSeq int64           `json:"conversation_seq"`
	ChangeVersion   int32           `json:"change_version"`
	EditedBy        string          `json:"edited_by"`
	BeforePayload   json.RawMessage `json:"before_payload"`
	AfterPayload    json.RawMessage `json:"after_payload"`
	Reason          string          `json:"reason"`
	EditedAt        string          `json:"edited_at"`
}

type messageDeletedPayload struct {
	MessageID       string `json:"message_id"`
	ConversationID  string `json:"conversation_id"`
	ConversationSeq int64  `json:"conversation_seq"`
	ChangeVersion   int32  `json:"change_version"`
	DeletedBy       string `json:"deleted_by"`
	DeleteScope     string `json:"delete_scope"`
	Reason          string `json:"reason"`
	DeletedAt       string `json:"deleted_at"`
}

func decodeMessagePersistedPayload(payloadJSON []byte) (messagePersistedPayload, error) {
	var payload messagePersistedPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return messagePersistedPayload{}, err
	}
	if payload.MessageID == "" ||
		payload.ConversationID == "" ||
		payload.ConversationSeq <= 0 ||
		payload.CommandHash == "" ||
		len(payload.Payload) == 0 ||
		payload.AcceptedAt == "" {
		return messagePersistedPayload{}, errors.New("message persisted payload is incomplete")
	}
	return payload, nil
}

func decodeMessageRevokedPayload(payloadJSON []byte) (messageRevokedPayload, error) {
	var payload messageRevokedPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return messageRevokedPayload{}, err
	}
	if payload.MessageID == "" ||
		payload.ConversationID == "" ||
		payload.ConversationSeq <= 0 ||
		payload.ChangeVersion <= 0 ||
		payload.RevokedBy == "" ||
		payload.RevokedAt == "" {
		return messageRevokedPayload{}, errors.New("message revoked payload is incomplete")
	}
	return payload, nil
}

func decodeMessageEditedPayload(payloadJSON []byte) (messageEditedPayload, error) {
	var payload messageEditedPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return messageEditedPayload{}, err
	}
	if payload.MessageID == "" ||
		payload.ConversationID == "" ||
		payload.ConversationSeq <= 0 ||
		payload.ChangeVersion <= 0 ||
		payload.EditedBy == "" ||
		len(payload.BeforePayload) == 0 ||
		len(payload.AfterPayload) == 0 ||
		payload.EditedAt == "" {
		return messageEditedPayload{}, errors.New("message edited payload is incomplete")
	}
	return payload, nil
}

func decodeMessageDeletedPayload(payloadJSON []byte) (messageDeletedPayload, error) {
	var payload messageDeletedPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return messageDeletedPayload{}, err
	}
	if payload.MessageID == "" ||
		payload.ConversationID == "" ||
		payload.ConversationSeq <= 0 ||
		payload.ChangeVersion <= 0 ||
		payload.DeletedBy == "" ||
		payload.DeleteScope == "" ||
		payload.DeletedAt == "" {
		return messageDeletedPayload{}, errors.New("message deleted payload is incomplete")
	}
	return payload, nil
}

func messageDeleteScope(scope string) (conversationtimelinev1.MessageDeleteScope, error) {
	switch scope {
	case "CONVERSATION_VIEW":
		return conversationtimelinev1.MessageDeleteScope_MESSAGE_DELETE_SCOPE_CONVERSATION_VIEW, nil
	case "COMPLIANCE_RETENTION":
		return conversationtimelinev1.MessageDeleteScope_MESSAGE_DELETE_SCOPE_COMPLIANCE_RETENTION, nil
	default:
		return conversationtimelinev1.MessageDeleteScope_MESSAGE_DELETE_SCOPE_UNSPECIFIED, errors.New("unsupported message delete scope")
	}
}

func structFromRawJSON(payload json.RawMessage) (*structpb.Struct, error) {
	var object map[string]any
	if err := json.Unmarshal(payload, &object); err != nil {
		return nil, err
	}
	return structpb.NewStruct(object)
}
