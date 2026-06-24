package types

import (
	"crypto/sha256"
	"encoding/hex"
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

	MemoryReviewDecisionApprove = "APPROVE"
	MemoryReviewDecisionReject  = "REJECT"
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

type SubmitMemoryCandidateCommand struct {
	AuthContext         AuthContext
	CandidateID         string
	Scope               string
	ScopeID             string
	ConversationID      ConversationID
	Topic               string
	EventType           string
	FactText            string
	FactSHA256          string
	ActorUserIDs        []string
	AudienceUserIDs     []string
	SourceRefs          []SourceRef
	ValidFromSeq        int64
	ValidToSeq          int64
	SupersedesEventIDs  []string
	ContradictsEventIDs []string
	Confidence          float64
	VisibilityVersion   int64
	ExtractionVersion   string
}

func (command SubmitMemoryCandidateCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.CandidateID) == "" {
		return NewInvalidArgument("candidate_id is required")
	}
	if command.Scope != MemoryScopeConversation {
		return NewInvalidArgument("candidate scope must be conversation")
	}
	if strings.TrimSpace(command.ScopeID) == "" || strings.TrimSpace(string(command.ConversationID)) == "" {
		return NewInvalidArgument("conversation scope is required")
	}
	if command.ScopeID != string(command.ConversationID) {
		return NewInvalidArgument("scope_id must match conversation_id")
	}
	if !isValidMemoryEventType(command.EventType) {
		return NewInvalidArgument("invalid memory event type")
	}
	if strings.TrimSpace(command.FactText) == "" || len([]rune(command.FactText)) > 2048 {
		return NewInvalidArgument("fact_text is required and must be <= 2048 runes")
	}
	if strings.TrimSpace(command.FactSHA256) == "" || normalizedFactSHA256(command.FactText) != strings.ToLower(strings.TrimSpace(command.FactSHA256)) {
		return NewInvalidArgument("fact_sha256 does not match fact_text")
	}
	if len(command.ActorUserIDs) == 0 {
		return NewInvalidArgument("actor_user_ids is required")
	}
	if command.ValidFromSeq <= 0 {
		return NewInvalidArgument("valid_from_seq must be positive")
	}
	if command.ValidToSeq < 0 || (command.ValidToSeq > 0 && command.ValidToSeq < command.ValidFromSeq) {
		return NewInvalidArgument("valid_to_seq is invalid")
	}
	if command.Confidence < 0 || command.Confidence > 1 {
		return NewInvalidArgument("confidence must be between 0 and 1")
	}
	if command.VisibilityVersion < 0 {
		return NewInvalidArgument("visibility_version must be non-negative")
	}
	if strings.TrimSpace(command.ExtractionVersion) == "" {
		return NewInvalidArgument("extraction_version is required")
	}
	if len(command.SourceRefs) == 0 {
		return NewInvalidArgument("source_refs is required")
	}
	for _, ref := range command.SourceRefs {
		if err := validateCandidateSourceRef(ref); err != nil {
			return err
		}
	}
	if err := validateMemoryEventIDReferences(command.CandidateID, command.SupersedesEventIDs, "supersedes_event_ids"); err != nil {
		return err
	}
	if err := validateMemoryEventIDReferences(command.CandidateID, command.ContradictsEventIDs, "contradicts_event_ids"); err != nil {
		return err
	}
	return nil
}

type SubmitMemoryCandidateResult struct {
	Item StructuredMemoryEvent
}

type ReviewMemoryCandidateCommand struct {
	AuthContext   AuthContext
	MemoryEventID string
	Decision      string
}

func (command ReviewMemoryCandidateCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.MemoryEventID) == "" {
		return NewInvalidArgument("memory_event_id is required")
	}
	if command.Decision != MemoryReviewDecisionApprove && command.Decision != MemoryReviewDecisionReject {
		return NewInvalidArgument("invalid review decision")
	}
	return nil
}

type ReviewMemoryCandidateResult struct {
	Item StructuredMemoryEvent
}

func validateCandidateSourceRef(ref SourceRef) error {
	if ref.SourceType != MemorySourceTypeMessage && ref.SourceType != MemorySourceTypeTimelineEvent {
		return NewInvalidArgument("candidate source ref must be message or timeline event")
	}
	if strings.TrimSpace(ref.SourceID) == "" || strings.TrimSpace(ref.SourceEventID) == "" {
		return NewInvalidArgument("candidate source ids are required")
	}
	if strings.TrimSpace(string(ref.ConversationID)) == "" || ref.ConversationSeq <= 0 {
		return NewInvalidArgument("candidate source conversation is required")
	}
	return nil
}

func validateMemoryEventIDReferences(candidateID string, ids []string, field string) error {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return NewInvalidArgument(field + " must not contain blank ids")
		}
		if id == candidateID {
			return NewInvalidArgument(field + " must not reference candidate_id")
		}
		if _, ok := seen[id]; ok {
			return NewInvalidArgument(field + " must not contain duplicate ids")
		}
		seen[id] = struct{}{}
	}
	return nil
}

func normalizedFactSHA256(value string) string {
	fields := strings.Fields(strings.TrimSpace(value))
	normalized := strings.Join(fields, " ")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
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

func isValidMemoryEventType(eventType string) bool {
	switch eventType {
	case MemoryEventTypeTask, MemoryEventTypeDecision, MemoryEventTypeStatus, MemoryEventTypeBlocker, MemoryEventTypeFile,
		MemoryEventTypePreferenceSignal, MemoryEventTypeRoleSignal, MemoryEventTypeProfileSignal:
		return true
	default:
		return false
	}
}

func isValidMemoryReviewState(state string) bool {
	switch state {
	case MemoryReviewUnreviewed, MemoryReviewNeedsReview, MemoryReviewApproved, MemoryReviewRejected:
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
