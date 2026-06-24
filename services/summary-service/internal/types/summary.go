package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	SummaryVersion = "summary-service.v1"

	DefaultSummaryEvidenceLimit = 12
	MaxSummaryEvidenceLimit     = 30
	MaxSummaryFocusLen          = 512
	MaxExtractiveSummaryItems   = 5

	SummaryStatusGrounded             = "GROUNDED"
	SummaryStatusInsufficientEvidence = "INSUFFICIENT_EVIDENCE"

	EvidenceSourceSearchMessage    = "SEARCH_MESSAGE"
	EvidenceSourceMemoryEvent      = "MEMORY_EVENT"
	EvidenceSourceProfileAggregate = "PROFILE_AGGREGATE"

	MemoryStatusPending    = "PENDING"
	MemoryStatusActive     = "ACTIVE"
	MemoryStatusSuperseded = "SUPERSEDED"
	MemoryStatusArchived   = "ARCHIVED"
)

type GenerateConversationSummaryCommand struct {
	AuthContext       AuthContext
	ConversationID    ConversationID
	Focus             string
	AfterSeq          int64
	AtConversationSeq int64
	Limit             int
	IncludeSearch     bool
	IncludeMemory     bool
	MemoryStatuses    []string
}

func (command GenerateConversationSummaryCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(string(command.ConversationID)) == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	if len([]rune(command.NormalizedFocus())) > MaxSummaryFocusLen {
		return NewInvalidArgument("focus exceeds maximum")
	}
	if command.AfterSeq < 0 {
		return NewInvalidArgument("after_seq must be non-negative")
	}
	if command.AtConversationSeq < 0 {
		return NewInvalidArgument("at_conversation_seq must be non-negative")
	}
	if command.Limit < 0 || command.Limit > MaxSummaryEvidenceLimit {
		return NewInvalidArgument("limit must be between 0 and 30")
	}
	for _, status := range command.MemoryStatuses {
		if !isValidMemoryStatus(status) {
			return NewInvalidArgument("invalid memory status")
		}
	}
	return nil
}

func (command GenerateConversationSummaryCommand) NormalizedFocus() string {
	return strings.TrimSpace(command.Focus)
}

func (command GenerateConversationSummaryCommand) RetrievalQuery() string {
	if focus := command.NormalizedFocus(); focus != "" {
		return focus
	}
	return "conversation summary"
}

func (command GenerateConversationSummaryCommand) EffectiveLimit() int {
	if command.Limit <= 0 {
		return DefaultSummaryEvidenceLimit
	}
	return command.Limit
}

func (command GenerateConversationSummaryCommand) ShouldIncludeSearch() bool {
	return command.IncludeSearch || !command.IncludeMemory
}

func (command GenerateConversationSummaryCommand) ShouldIncludeMemory() bool {
	return command.IncludeMemory || !command.IncludeSearch
}

func (command GenerateConversationSummaryCommand) EffectiveMemoryStatuses() []string {
	if len(command.MemoryStatuses) > 0 {
		return command.MemoryStatuses
	}
	return []string{MemoryStatusActive}
}

func (command GenerateConversationSummaryCommand) SummaryID() string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s|%d|%d|%d",
		command.AuthContext.TenantID,
		command.AuthContext.UserID,
		command.ConversationID,
		command.RetrievalQuery(),
		command.AfterSeq,
		command.AtConversationSeq,
		command.EffectiveLimit(),
	)))
	return "sum_" + hex.EncodeToString(sum[:8])
}

type RetrieveEvidenceQuery struct {
	AuthContext       AuthContext
	Query             string
	ConversationID    ConversationID
	AfterSeq          int64
	AtConversationSeq int64
	Limit             int
	IncludeSearch     bool
	IncludeMemory     bool
	MemoryStatuses    []string
}

type EvidenceSourceRef struct {
	SourceType      string
	SourceID        string
	SourceEventID   string
	ConversationID  ConversationID
	ConversationSeq int64
	OccurredAt      time.Time
}

type MemoryGraphEdge struct {
	EdgeID            string
	FromMemoryEventID string
	ToMemoryEventID   string
	RelationType      string
	Confidence        float64
	SourceRefs        []EvidenceSourceRef
}

type EvidenceItem struct {
	EvidenceID               string
	SourceType               string
	SourceID                 string
	ConversationID           ConversationID
	ConversationSeq          int64
	Text                     string
	Score                    float64
	SpeakerUserID            UserID
	MessageID                string
	MemoryEventID            string
	OccurredAt               time.Time
	ValidFromSeq             int64
	ValidToSeq               int64
	VisibilityVersion        int64
	SourceRefs               []EvidenceSourceRef
	ActorUserIDs             []string
	AudienceUserIDs          []string
	TemporalStatus           string
	ReviewState              string
	ExtractionVersion        string
	RerankScore              float64
	DedupeReason             string
	MemoryGraphEdges         []MemoryGraphEdge
	ProfileID                string
	ProfileSubjectUserID     UserID
	ProfileAggregateType     string
	ProfileAggregateKey      string
	SupportingMemoryEventIDs []string
	ProfileValidFromAt       time.Time
	ProfileValidToAt         time.Time
	ProfileUpdatedAt         time.Time
}

type EvidenceSourceCount struct {
	SourceType string
	Count      int
}

type EvidenceSourceCoverage struct {
	SourceType     string
	Requested      bool
	CandidateCount int
	ReturnedCount  int
	DedupedCount   int
	Status         string
}

type EvidencePack struct {
	PackID                  string
	TenantID                TenantID
	Query                   string
	ConversationID          ConversationID
	Items                   []EvidenceItem
	SourceCounts            []EvidenceSourceCount
	SearchProjectionVersion int64
	MemoryProjectionVersion int64
	RetrievalVersion        string
	SourceCoverage          []EvidenceSourceCoverage
}

type RetrieveEvidenceResult struct {
	Pack EvidencePack
}

type Citation struct {
	EvidenceID      string
	SourceType      string
	SourceID        string
	SourceEventID   string
	ConversationID  ConversationID
	ConversationSeq int64
	OccurredAt      time.Time
}

type GenerateConversationSummaryResult struct {
	SummaryID      string
	Status         string
	SummaryText    string
	Confidence     float64
	Citations      []Citation
	EvidencePack   EvidencePack
	SummaryVersion string
	GeneratedByLLM bool
}

type SummaryGenerationRequest struct {
	Focus        string
	EvidencePack EvidencePack
}

type SummaryGenerationResult struct {
	Status         string
	SummaryText    string
	Confidence     float64
	Citations      []Citation
	GeneratedByLLM bool
}

func isValidMemoryStatus(status string) bool {
	switch status {
	case MemoryStatusPending, MemoryStatusActive, MemoryStatusSuperseded, MemoryStatusArchived:
		return true
	default:
		return false
	}
}
