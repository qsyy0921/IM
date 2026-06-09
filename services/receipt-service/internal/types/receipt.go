package types

import "time"

const (
	ReceiptVisibilityDetailed  = "DETAILED"
	ReceiptVisibilityCountOnly = "COUNT_ONLY"
	ReceiptVisibilityHidden    = "HIDDEN"
)

type MarkReadCommand struct {
	AuthContext    AuthContext
	ConversationID ConversationID
	ReadSeq        int64
}

func (command MarkReadCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	if command.ReadSeq <= 0 {
		return NewInvalidArgument("read_seq must be positive")
	}
	return nil
}

type MarkReadResult struct {
	TenantID       TenantID
	UserID         UserID
	ConversationID ConversationID
	LastReadSeq    int64
}

type GetReceiptStateCommand struct {
	AuthContext     AuthContext
	ConversationID  ConversationID
	MessageID       string
	ConversationSeq int64
}

func (command GetReceiptStateCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	hasMessageID := command.MessageID != ""
	hasSeq := command.ConversationSeq > 0
	if hasMessageID == hasSeq {
		return NewInvalidArgument("exactly one of message_id or conversation_seq is required")
	}
	return nil
}

type ReceiptUserState struct {
	UserID      UserID
	ReceivedSeq int64
	ReceivedAt  time.Time
	ReadSeq     int64
	ReadAt      time.Time
}

type GetReceiptStateResult struct {
	ConversationID    ConversationID
	ConversationSeq   int64
	MessageID         string
	ReceivedUserCount int
	ReadUserCount     int
	VisibilityMode    string
	Receivers         []ReceiptUserState
}

type ReceiptAccessContext struct {
	TenantID          TenantID
	UserID            UserID
	ConversationID    ConversationID
	VisibilityMode    string
	PermissionVersion int64
}
