package domain

import (
	"time"

	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

type Message struct {
	TenantID       types.TenantID
	ConversationID types.ConversationID
	Seq            int64
	MessageID      types.MessageID
	SenderID       types.UserID
	DeviceID       types.DeviceID
	ClientMsgID    types.ClientMsgID
	CommandHash    string
	MessageType    types.MessageType
	PayloadJSON    []byte
	Status         MessageStatus
	CreatedAt      time.Time
}

type MessageStatus string

const (
	MessageStatusNormal  MessageStatus = "NORMAL"
	MessageStatusEdited  MessageStatus = "EDITED"
	MessageStatusRevoked MessageStatus = "REVOKED"
	MessageStatusDeleted MessageStatus = "DELETED"
)

func (m Message) CanEdit() bool {
	return m.Status == MessageStatusNormal || m.Status == MessageStatusEdited
}

func (m Message) CanRevoke() bool {
	return m.Status == MessageStatusNormal || m.Status == MessageStatusEdited
}

func (m Message) CanDelete() bool {
	return m.Status == MessageStatusNormal || m.Status == MessageStatusEdited || m.Status == MessageStatusRevoked
}
