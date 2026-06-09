package types

import (
	"encoding/json"
	"time"
)

const (
	DefaultPullLimit = 50
	MaxPullLimit     = 500
)

type PullInboxCommand struct {
	AuthContext    AuthContext
	ConversationID ConversationID
	AfterSeq       int64
	Limit          int
}

func (command PullInboxCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	if command.AfterSeq < 0 {
		return NewInvalidArgument("after_seq must be non-negative")
	}
	if command.Limit < 0 {
		return NewInvalidArgument("limit must be non-negative")
	}
	if command.Limit > MaxPullLimit {
		return NewInvalidArgument("limit exceeds maximum")
	}
	return nil
}

func (command PullInboxCommand) EffectiveLimit() int {
	if command.Limit == 0 {
		return DefaultPullLimit
	}
	return command.Limit
}

type InboxItem struct {
	ConversationID  ConversationID
	ConversationSeq int64
	EventID         string
	EventType       string
	MessageID       string
	SenderID        UserID
	PayloadJSON     json.RawMessage
	CreatedAt       time.Time
}

type PullInboxResult struct {
	Items   []InboxItem
	NextSeq int64
	HasMore bool
}

type AckDeliveryCommand struct {
	AuthContext    AuthContext
	ConversationID ConversationID
	ReceivedSeq    int64
}

func (command AckDeliveryCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	if command.ReceivedSeq <= 0 {
		return NewInvalidArgument("received_seq must be positive")
	}
	return nil
}

type AckDeliveryResult struct {
	TenantID        TenantID
	UserID          UserID
	DeviceID        string
	ConversationID  ConversationID
	LastReceivedSeq int64
}
