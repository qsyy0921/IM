package types

import (
	"strings"
	"time"
)

const (
	DefaultSearchLimit = 20
	MaxSearchLimit     = 100
	MaxSearchQueryLen  = 256
)

type SearchMessagesCommand struct {
	AuthContext    AuthContext
	Query          string
	ConversationID ConversationID
	AfterSeq       int64
	Limit          int
}

func (command SearchMessagesCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.Query) == "" {
		return NewInvalidArgument("query is required")
	}
	if len([]rune(strings.TrimSpace(command.Query))) > MaxSearchQueryLen {
		return NewInvalidArgument("query exceeds maximum")
	}
	if command.AfterSeq < 0 {
		return NewInvalidArgument("after_seq must be non-negative")
	}
	if command.Limit < 0 {
		return NewInvalidArgument("limit must be non-negative")
	}
	if command.Limit > MaxSearchLimit {
		return NewInvalidArgument("limit exceeds maximum")
	}
	return nil
}

func (command SearchMessagesCommand) EffectiveLimit() int {
	if command.Limit == 0 {
		return DefaultSearchLimit
	}
	return command.Limit
}

func (command SearchMessagesCommand) NormalizedQuery() string {
	return strings.TrimSpace(command.Query)
}

type HighlightRange struct {
	Start int32
	End   int32
}

type SearchMessageHit struct {
	ConversationID    ConversationID
	MessageID         string
	ConversationSeq   int64
	SourceEventID     string
	SenderID          UserID
	MessageType       string
	Snippet           string
	HighlightRanges   []HighlightRange
	OccurredAt        time.Time
	VisibilityVersion int64
}

type SearchMessageCandidate struct {
	ConversationID ConversationID
	MessageID      string
}

type SearchMessagesResult struct {
	Items             []SearchMessageHit
	NextCursor        string
	ProjectionVersion int64
	HasMore           bool
}
