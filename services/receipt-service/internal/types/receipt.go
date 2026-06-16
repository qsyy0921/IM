package types

import "time"

const (
	ReceiptVisibilityDetailed  = "DETAILED"
	ReceiptVisibilityCountOnly = "COUNT_ONLY"
	ReceiptVisibilityHidden    = "HIDDEN"
)

const (
	DefaultReceivedDeviceDetailLimit = 10
	MaxReceivedDeviceDetailLimit     = 50
)

const (
	MaxConversationTags      = 10
	MaxConversationTagSize   = 32
	MaxConversationDraftSize = 4096
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
	AuthContext             AuthContext
	AccessContext           ReceiptAccessContext
	ConversationID          ConversationID
	MessageID               string
	ConversationSeq         int64
	IncludeReceivedDevices  bool
	ReceivedDeviceLimitHint int
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
	if err := validateReceivedDeviceDetailLimit(command.IncludeReceivedDevices, command.ReceivedDeviceLimitHint); err != nil {
		return err
	}
	return nil
}

func (command GetReceiptStateCommand) ReceivedDeviceLimit() int {
	return normalizeReceivedDeviceDetailLimit(command.IncludeReceivedDevices, command.ReceivedDeviceLimitHint)
}

type ReceiptUserState struct {
	UserID                   UserID
	ReceivedSeq              int64
	ReceivedAt               time.Time
	ReadSeq                  int64
	ReadAt                   time.Time
	ReceivedDeviceCount      int
	ReceivedDevices          []ReceivedDeviceState
	ReceivedDevicesTruncated bool
}

type ReceivedDeviceState struct {
	DeviceID        string
	LastReceivedSeq int64
	UpdatedAt       time.Time
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
	AuthContext             AuthContext
	AccessContext           ReceiptAccessContext
	ConversationID          ConversationID
	Items                   []ReceiptStateQuery
	IncludeReceivedDevices  bool
	ReceivedDeviceLimitHint int
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
	if err := validateReceivedDeviceDetailLimit(command.IncludeReceivedDevices, command.ReceivedDeviceLimitHint); err != nil {
		return err
	}
	return nil
}

func (command ListReceiptStatesCommand) ReceivedDeviceLimit() int {
	return normalizeReceivedDeviceDetailLimit(command.IncludeReceivedDevices, command.ReceivedDeviceLimitHint)
}

type ListReceiptStatesResult struct {
	Items []GetReceiptStateResult
}

func validateReceivedDeviceDetailLimit(include bool, limit int) error {
	if limit < 0 {
		return NewInvalidArgument("received_device_limit must be non-negative")
	}
	if !include && limit > 0 {
		return NewInvalidArgument("received_device_limit requires include_received_devices")
	}
	if include && limit > MaxReceivedDeviceDetailLimit {
		return NewInvalidArgument("received_device_limit exceeds max")
	}
	return nil
}

func normalizeReceivedDeviceDetailLimit(include bool, limit int) int {
	if !include {
		return 0
	}
	if limit == 0 {
		return DefaultReceivedDeviceDetailLimit
	}
	return limit
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
	TagFilter       string
	DraftOnly       bool
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
	if command.TagFilter != "" {
		if _, err := NormalizeConversationTag(command.TagFilter); err != nil {
			return err
		}
	}
	return nil
}

const (
	ConversationListSortUpdatedAtDesc       = "updated_at_desc"
	ConversationListSortPinnedUpdatedAtDesc = "pinned_updated_at_desc"
	ConversationListSortUnreadUpdatedAtDesc = "unread_updated_at_desc"
	ConversationListSortDraftUpdatedAtDesc  = "draft_updated_at_desc"
)

func NormalizeConversationListSort(sort string) (string, error) {
	if sort == "" {
		return ConversationListSortPinnedUpdatedAtDesc, nil
	}
	if sort == ConversationListSortUpdatedAtDesc ||
		sort == ConversationListSortPinnedUpdatedAtDesc ||
		sort == ConversationListSortUnreadUpdatedAtDesc ||
		sort == ConversationListSortDraftUpdatedAtDesc {
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
	Tags                []string
	DraftText           string
	DraftUpdatedAt      time.Time
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

type SetConversationTagsCommand struct {
	AuthContext    AuthContext
	ConversationID ConversationID
	Tags           []string
}

func (command SetConversationTagsCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	_, err := NormalizeConversationTags(command.Tags)
	return err
}

type SetConversationTagsResult struct {
	Conversation ConversationSummary
}

type SetConversationDraftCommand struct {
	AuthContext    AuthContext
	ConversationID ConversationID
	DraftText      string
}

func (command SetConversationDraftCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	_, err := NormalizeConversationDraft(command.DraftText)
	return err
}

type SetConversationDraftResult struct {
	Conversation ConversationSummary
}

func NormalizeConversationDraft(draft string) (string, error) {
	if len(draft) > MaxConversationDraftSize {
		return "", NewInvalidArgument("draft_text is too long")
	}
	for _, char := range draft {
		if char == 0 {
			return "", NewInvalidArgument("draft_text contains unsupported characters")
		}
	}
	return draft, nil
}

func NormalizeConversationTags(tags []string) ([]string, error) {
	if len(tags) > MaxConversationTags {
		return nil, NewInvalidArgument("tags exceeds max")
	}
	normalized := make([]string, 0, len(tags))
	seen := make(map[string]bool, len(tags))
	for _, tag := range tags {
		value, err := NormalizeConversationTag(tag)
		if err != nil {
			return nil, err
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	return normalized, nil
}

func NormalizeConversationTag(tag string) (string, error) {
	if tag == "" {
		return "", NewInvalidArgument("tag must be non-empty")
	}
	if len(tag) > MaxConversationTagSize {
		return "", NewInvalidArgument("tag is too long")
	}
	for _, char := range tag {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '_' ||
			char == '-' ||
			char == '.' {
			continue
		}
		return "", NewInvalidArgument("tag contains unsupported characters")
	}
	return tag, nil
}
