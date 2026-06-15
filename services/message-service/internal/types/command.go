package types

import (
	"errors"
	"time"
)

type AuthContext struct {
	TenantID  TenantID
	UserID    UserID
	DeviceID  DeviceID
	SessionID SessionID
	TraceID   string
	RequestID string
}

type SendMessageCommand struct {
	AuthContext    AuthContext
	ConversationID ConversationID
	ClientMsgID    ClientMsgID
	MessageType    MessageType
	PayloadJSON    []byte
	AttachmentIDs  []string
	ReceivedAt     time.Time
}

func (c SendMessageCommand) Validate() error {
	if c.AuthContext.TenantID == "" || c.AuthContext.UserID == "" || c.AuthContext.DeviceID == "" {
		return errors.New("auth context is required")
	}
	if c.ConversationID == "" {
		return errors.New("conversation_id is required")
	}
	if c.ClientMsgID == "" {
		return errors.New("client_msg_id is required")
	}
	if c.MessageType == "" {
		return errors.New("message_type is required")
	}
	if !IsSupportedMessageType(c.MessageType) {
		return NewUnsupportedMessageType("message_type is not supported")
	}
	if MessageTypeRequiresAttachment(c.MessageType) && len(c.AttachmentIDs) == 0 {
		return errors.New("attachment_ids are required for attachment message types")
	}
	return nil
}

type SendMessageResult struct {
	MessageID        MessageID
	ConversationID   ConversationID
	ConversationSeq  int64
	AcceptedAt       time.Time
	IdempotentReplay bool
}

type RevokeMessageCommand struct {
	AuthContext    AuthContext
	ConversationID ConversationID
	MessageID      MessageID
	IdempotencyKey string
	Reason         string
	ReceivedAt     time.Time
}

type DeleteScope string

const (
	DeleteScopeConversationView DeleteScope = "CONVERSATION_VIEW"
	DeleteScopeCompliance       DeleteScope = "COMPLIANCE_RETENTION"
)

type DeleteMessageCommand struct {
	AuthContext    AuthContext
	ConversationID ConversationID
	MessageID      MessageID
	IdempotencyKey string
	DeleteScope    DeleteScope
	Reason         string
	ReceivedAt     time.Time
}

type EditMessageCommand struct {
	AuthContext    AuthContext
	ConversationID ConversationID
	MessageID      MessageID
	IdempotencyKey string
	PayloadJSON    []byte
	Reason         string
	ReceivedAt     time.Time
}

func (c EditMessageCommand) Validate() error {
	if c.AuthContext.TenantID == "" || c.AuthContext.UserID == "" || c.AuthContext.DeviceID == "" {
		return errors.New("auth context is required")
	}
	if c.ConversationID == "" {
		return errors.New("conversation_id is required")
	}
	if c.MessageID == "" {
		return errors.New("message_id is required")
	}
	if c.IdempotencyKey == "" {
		return errors.New("idempotency_key is required")
	}
	return nil
}

func (c DeleteMessageCommand) Validate() error {
	if c.AuthContext.TenantID == "" || c.AuthContext.UserID == "" || c.AuthContext.DeviceID == "" {
		return errors.New("auth context is required")
	}
	if c.ConversationID == "" {
		return errors.New("conversation_id is required")
	}
	if c.MessageID == "" {
		return errors.New("message_id is required")
	}
	if c.IdempotencyKey == "" {
		return errors.New("idempotency_key is required")
	}
	if c.DeleteScope == "" {
		return errors.New("delete_scope is required")
	}
	return nil
}

func (c RevokeMessageCommand) Validate() error {
	if c.AuthContext.TenantID == "" || c.AuthContext.UserID == "" || c.AuthContext.DeviceID == "" {
		return errors.New("auth context is required")
	}
	if c.ConversationID == "" {
		return errors.New("conversation_id is required")
	}
	if c.MessageID == "" {
		return errors.New("message_id is required")
	}
	if c.IdempotencyKey == "" {
		return errors.New("idempotency_key is required")
	}
	return nil
}

type MessageChangeResult struct {
	MessageID        MessageID
	ConversationID   ConversationID
	ConversationSeq  int64
	ChangeVersion    int32
	AcceptedAt       time.Time
	IdempotentReplay bool
}
