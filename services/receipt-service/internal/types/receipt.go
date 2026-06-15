package types

import "time"

const (
	ReceiptVisibilityDetailed  = "DETAILED"
	ReceiptVisibilityCountOnly = "COUNT_ONLY"
	ReceiptVisibilityHidden    = "HIDDEN"
)

type MarkReadCommand struct {
	AuthContext    AuthContext
	AccessContext  ReceiptAccessContext
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
	AccessContext   ReceiptAccessContext
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

type ReceiptStateQuery struct {
	MessageID       string
	ConversationSeq int64
}

func (query ReceiptStateQuery) Validate() error {
	hasMessageID := query.MessageID != ""
	hasSeq := query.ConversationSeq > 0
	if hasMessageID == hasSeq {
		return NewInvalidArgument("exactly one of message_id or conversation_seq is required")
	}
	return nil
}

type ListReceiptStatesCommand struct {
	AuthContext    AuthContext
	AccessContext  ReceiptAccessContext
	ConversationID ConversationID
	Items          []ReceiptStateQuery
}

func (command ListReceiptStatesCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	if len(command.Items) == 0 {
		return NewInvalidArgument("items are required")
	}
	if len(command.Items) > 50 {
		return NewInvalidArgument("items exceeds max batch size")
	}
	for _, item := range command.Items {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type ListReceiptStatesResult struct {
	Items []GetReceiptStateResult
}

type ReceiptAccessContext struct {
	TenantID          TenantID
	UserID            UserID
	ConversationID    ConversationID
	VisibilityMode    string
	PermissionVersion int64
	MemberJoinSeq     int64
	MemberLeaveSeq    int64
}

type ListConversationsCommand struct {
	AuthContext     AuthContext
	Limit           int
	PageCursor      string
	Sort            string
	IncludeArchived bool
	UnreadOnly      bool
	PinnedOnly      bool
	MutedOnly       bool
}

func (command ListConversationsCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.Limit < 0 {
		return NewInvalidArgument("limit must be non-negative")
	}
	if _, err := NormalizeConversationListSort(command.Sort); err != nil {
		return err
	}
	return nil
}

const (
	ConversationListSortUpdatedAtDesc       = "updated_at_desc"
	ConversationListSortPinnedUpdatedAtDesc = "pinned_updated_at_desc"
)

func NormalizeConversationListSort(sort string) (string, error) {
	if sort == "" {
		return ConversationListSortPinnedUpdatedAtDesc, nil
	}
	if sort == ConversationListSortUpdatedAtDesc || sort == ConversationListSortPinnedUpdatedAtDesc {
		return sort, nil
	}
	return "", NewInvalidArgument("unsupported conversation list sort")
}

type ConversationSummary struct {
	ConversationID      ConversationID
	LastVisibleSeq      int64
	LastMessageID       string
	LastSenderID        UserID
	LastSourceEventType string
	UnreadCount         int64
	LastReadSeq         int64
	UpdatedAt           time.Time
	Archived            bool
	Pinned              bool
	Muted               bool
}

type ProjectionWatermark struct {
	Source      string
	OffsetValue int64
	UpdatedAt   time.Time
}

type ListConversationsResult struct {
	Items               []ConversationSummary
	NextPageCursor      string
	ProjectionWatermark ProjectionWatermark
}

type ArchiveConversationCommand struct {
	AuthContext    AuthContext
	ConversationID ConversationID
	Archived       bool
}

func (command ArchiveConversationCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	return nil
}

type ArchiveConversationResult struct {
	Conversation ConversationSummary
}

type PinConversationCommand struct {
	AuthContext    AuthContext
	ConversationID ConversationID
	Pinned         bool
}

func (command PinConversationCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	return nil
}

type PinConversationResult struct {
	Conversation ConversationSummary
}

type MuteConversationCommand struct {
	AuthContext    AuthContext
	ConversationID ConversationID
	Muted          bool
}

func (command MuteConversationCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	return nil
}

type MuteConversationResult struct {
	Conversation ConversationSummary
}
