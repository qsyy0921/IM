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
	return nil
}

type SendMessageResult struct {
	MessageID        MessageID
	ConversationID   ConversationID
	ConversationSeq  int64
	AcceptedAt       time.Time
	IdempotentReplay bool
}
