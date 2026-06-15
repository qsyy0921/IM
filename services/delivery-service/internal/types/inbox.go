package types

import (
	"encoding/json"
	"strings"
	"time"
)

const (
	DefaultPullLimit = 50
	MaxPullLimit     = 500
	MaxHideReasonLen = 512
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

type HideInboxItemCommand struct {
	AuthContext     AuthContext
	ConversationID  ConversationID
	ConversationSeq int64
	Reason          string
}

func (command HideInboxItemCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	if command.ConversationSeq <= 0 {
		return NewInvalidArgument("conversation_seq must be positive")
	}
	if len(strings.TrimSpace(command.Reason)) > MaxHideReasonLen {
		return NewInvalidArgument("reason exceeds maximum")
	}
	return nil
}

type HideInboxItemResult struct {
	TenantID        TenantID
	UserID          UserID
	ConversationID  ConversationID
	ConversationSeq int64
	AlreadyHidden   bool
}
