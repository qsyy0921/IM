package types

import (
	"strings"
	"time"
)

const (
	MemoryScopeConversation = "CONVERSATION"
	MemoryScopeProject      = "PROJECT"
	MemoryScopePersonal     = "PERSONAL"
	MemoryScopeTenant       = "TENANT"

	MemoryEventTypeTask             = "TASK"
	MemoryEventTypeDecision         = "DECISION"
	MemoryEventTypeStatus           = "STATUS"
	MemoryEventTypeBlocker          = "BLOCKER"
	MemoryEventTypeFile             = "FILE"
	MemoryEventTypePreferenceSignal = "PREFERENCE_SIGNAL"
	MemoryEventTypeRoleSignal       = "ROLE_SIGNAL"
	MemoryEventTypeProfileSignal    = "PROFILE_SIGNAL"

	MemoryStatusPending    = "PENDING"
	MemoryStatusActive     = "ACTIVE"
	MemoryStatusSuperseded = "SUPERSEDED"
	MemoryStatusRejected   = "REJECTED"
	MemoryStatusArchived   = "ARCHIVED"
	MemoryStatusDeleted    = "DELETED"

	MemoryReviewUnreviewed  = "UNREVIEWED"
	MemoryReviewNeedsReview = "NEEDS_REVIEW"
	MemoryReviewApproved    = "APPROVED"
	MemoryReviewRejected    = "REJECTED"

	MemorySourceTypeMessage          = "MESSAGE"
	MemorySourceTypeTimelineEvent    = "TIMELINE_EVENT"
	MemorySourceTypeProfileAggregate = "PROFILE_AGGREGATE"
	MemorySourceTypeSystem           = "SYSTEM"

	ProfileAggregateTypeStyle      = "STYLE"
	ProfileAggregateTypeSkill      = "SKILL"
	ProfileAggregateTypeRole       = "ROLE"
	ProfileAggregateTypePreference = "PREFERENCE"
	ProfileAggregateTypeInterest   = "INTEREST"
)

type SourceRef struct {
	SourceType      string
	SourceID        string
	SourceEventID   string
	ConversationID  ConversationID
	ConversationSeq int64
	OccurredAt      time.Time
}

type StructuredMemoryEvent struct {
	MemoryEventID       string
	Scope               string
	ScopeID             string
	ConversationID      ConversationID
	Topic               string
	EventType           string
	Status              string
	ReviewState         string
	FactText            string
	ActorUserIDs        []string
	AudienceUserIDs     []string
	SourceRefs          []SourceRef
	ValidFromSeq        int64
	ValidToSeq          int64
	ValidFromAt         time.Time
	ValidToAt           time.Time
	SupersedesEventIDs  []string
	ContradictsEventIDs []string
	Confidence          float64
	VisibilityVersion   int64
	ExtractionVersion   string
	UpdatedAt           time.Time
}

type MemoryGraphEdge struct {
	EdgeID            string
	FromMemoryEventID string
	ToMemoryEventID   string
	RelationType      string
	Confidence        float64
	SourceRefs        []SourceRef
}

type ProfileAggregate struct {
	ProfileID                string
	SubjectUserID            UserID
	AggregateType            string
	AggregateKey             string
	Status                   string
	ReviewState              string
	SummaryText              string
	SupportingMemoryEventIDs []string
	Confidence               float64
	ValidFromAt              time.Time
	ValidToAt                time.Time
	UpdatedAt                time.Time
}

type QueryMemoryEventsCommand struct {
	AuthContext       AuthContext
	Scope             string
	ScopeID           string
	ConversationID    ConversationID
	ActorUserID       UserID
	Topic             string
	Query             string
	Statuses          []string
	AfterValidFromSeq int64
	AtConversationSeq int64
	Limit             int
}

func (command QueryMemoryEventsCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.Scope != "" && !isValidScope(command.Scope) {
		return NewInvalidArgument("invalid scope")
	}
	if command.Limit < 0 || command.Limit > 100 {
		return NewInvalidArgument("limit must be between 0 and 100")
	}
	if command.AfterValidFromSeq < 0 {
		return NewInvalidArgument("after_valid_from_seq must be non-negative")
	}
	if command.AtConversationSeq < 0 {
		return NewInvalidArgument("at_conversation_seq must be non-negative")
	}
	if len(strings.TrimSpace(command.Query)) > 512 {
		return NewInvalidArgument("query is too long")
	}
	for _, status := range command.Statuses {
		if !isValidMemoryStatus(status) {
			return NewInvalidArgument("invalid memory status")
		}
	}
	return nil
}

func (command QueryMemoryEventsCommand) EffectiveLimit() int {
	if command.Limit <= 0 {
		return 20
	}
	return command.Limit
}

func (command QueryMemoryEventsCommand) NormalizedQuery() string {
	return strings.TrimSpace(command.Query)
}

type QueryMemoryEventsResult struct {
	Items             []StructuredMemoryEvent
	NextCursor        string
	ProjectionVersion int64
}

type GetMemoryEventCommand struct {
	AuthContext   AuthContext
	MemoryEventID string
}

func (command GetMemoryEventCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.MemoryEventID) == "" {
		return NewInvalidArgument("memory_event_id is required")
	}
	return nil
}

type GetMemoryEventResult struct {
	Item       StructuredMemoryEvent
	GraphEdges []MemoryGraphEdge
}

type ListProfileAggregatesCommand struct {
	AuthContext   AuthContext
	SubjectUserID UserID
	AggregateType string
	Statuses      []string
	Limit         int
}

func (command ListProfileAggregatesCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(string(command.SubjectUserID)) == "" {
		return NewInvalidArgument("subject_user_id is required")
	}
	if command.SubjectUserID != command.AuthContext.UserID {
		return ErrPermissionDenied
	}
	if command.Limit < 0 || command.Limit > 100 {
		return NewInvalidArgument("limit must be between 0 and 100")
	}
	for _, status := range command.Statuses {
		if !isValidMemoryStatus(status) {
			return NewInvalidArgument("invalid memory status")
		}
	}
	return nil
}

func (command ListProfileAggregatesCommand) EffectiveLimit() int {
	if command.Limit <= 0 {
		return 20
	}
	return command.Limit
}

type ListProfileAggregatesResult struct {
	Items      []ProfileAggregate
	NextCursor string
}

type RecomputeProfileAggregateCommand struct {
	AuthContext     AuthContext
	SubjectUserID   UserID
	AggregateType   string
	AggregateKey    string
	MinSupportCount int
}

func (command RecomputeProfileAggregateCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(string(command.SubjectUserID)) == "" {
		return NewInvalidArgument("subject_user_id is required")
	}
	if command.SubjectUserID != command.AuthContext.UserID {
		return ErrPermissionDenied
	}
	if !isValidProfileAggregateType(command.AggregateType) {
		return NewInvalidArgument("invalid aggregate type")
	}
	if strings.TrimSpace(command.AggregateKey) == "" {
		return NewInvalidArgument("aggregate_key is required")
	}
	if command.MinSupportCount < 0 || command.MinSupportCount > 20 {
		return NewInvalidArgument("min_support_count must be between 0 and 20")
	}
	return nil
}

func (command RecomputeProfileAggregateCommand) EffectiveMinSupportCount() int {
	if command.MinSupportCount <= 0 {
		return 2
	}
	return command.MinSupportCount
}

type RecomputeProfileAggregateResult struct {
	Item         ProfileAggregate
	SupportCount int
	Active       bool
}

func isValidScope(scope string) bool {
	switch scope {
	case MemoryScopeConversation, MemoryScopeProject, MemoryScopePersonal, MemoryScopeTenant:
		return true
	default:
		return false
	}
}

func isValidMemoryStatus(status string) bool {
	switch status {
	case MemoryStatusPending, MemoryStatusActive, MemoryStatusSuperseded, MemoryStatusRejected, MemoryStatusArchived, MemoryStatusDeleted:
		return true
	default:
		return false
	}
}

func isValidProfileAggregateType(value string) bool {
	switch value {
	case ProfileAggregateTypeStyle, ProfileAggregateTypeSkill, ProfileAggregateTypeRole, ProfileAggregateTypePreference, ProfileAggregateTypeInterest:
		return true
	default:
		return false
	}
}
