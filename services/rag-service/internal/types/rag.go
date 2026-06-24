package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	RAGVersion = "rag-service.v1"

	DefaultAnswerEvidenceLimit = 8
	MaxAnswerEvidenceLimit     = 20
	MaxRAGQuestionLen          = 512
	MaxExtractiveAnswerItems   = 3

	AnswerStatusGrounded             = "GROUNDED"
	AnswerStatusInsufficientEvidence = "INSUFFICIENT_EVIDENCE"

	EvidenceSourceSearchMessage    = "SEARCH_MESSAGE"
	EvidenceSourceMemoryEvent      = "MEMORY_EVENT"
	EvidenceSourceProfileAggregate = "PROFILE_AGGREGATE"

	MemoryStatusPending    = "PENDING"
	MemoryStatusActive     = "ACTIVE"
	MemoryStatusSuperseded = "SUPERSEDED"
	MemoryStatusArchived   = "ARCHIVED"
)

type AnswerQuestionCommand struct {
	AuthContext       AuthContext
	Question          string
	ConversationID    ConversationID
	AfterSeq          int64
	AtConversationSeq int64
	Limit             int
	IncludeSearch     bool
	IncludeMemory     bool
	MemoryStatuses    []string
}

func (command AnswerQuestionCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.Question) == "" {
		return NewInvalidArgument("question is required")
	}
	if len([]rune(command.NormalizedQuestion())) > MaxRAGQuestionLen {
		return NewInvalidArgument("question exceeds maximum")
	}
	if command.AfterSeq < 0 {
		return NewInvalidArgument("after_seq must be non-negative")
	}
	if command.AtConversationSeq < 0 {
		return NewInvalidArgument("at_conversation_seq must be non-negative")
	}
	if command.Limit < 0 || command.Limit > MaxAnswerEvidenceLimit {
		return NewInvalidArgument("limit must be between 0 and 20")
	}
	for _, status := range command.MemoryStatuses {
		if !isValidMemoryStatus(status) {
			return NewInvalidArgument("invalid memory status")
		}
	}
	return nil
}

func (command AnswerQuestionCommand) NormalizedQuestion() string {
	return strings.TrimSpace(command.Question)
}

func (command AnswerQuestionCommand) EffectiveLimit() int {
	if command.Limit <= 0 {
		return DefaultAnswerEvidenceLimit
	}
	return command.Limit
}

func (command AnswerQuestionCommand) ShouldIncludeSearch() bool {
	return command.IncludeSearch || !command.IncludeMemory
}

func (command AnswerQuestionCommand) ShouldIncludeMemory() bool {
	return command.IncludeMemory || !command.IncludeSearch
}

func (command AnswerQuestionCommand) EffectiveMemoryStatuses() []string {
	if len(command.MemoryStatuses) > 0 {
		return command.MemoryStatuses
	}
	return []string{MemoryStatusActive}
}

func (command AnswerQuestionCommand) AnswerID() string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s|%d|%d|%d",
		command.AuthContext.TenantID,
		command.AuthContext.UserID,
		command.ConversationID,
		command.NormalizedQuestion(),
		command.AfterSeq,
		command.AtConversationSeq,
		command.EffectiveLimit(),
	)))
	return "ans_" + hex.EncodeToString(sum[:8])
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

type AnswerQuestionResult struct {
	AnswerID       string
	Status         string
	AnswerText     string
	Confidence     float64
	Citations      []Citation
	EvidencePack   EvidencePack
	RAGVersion     string
	GeneratedByLLM bool
}

type AnswerGenerationRequest struct {
	Question     string
	EvidencePack EvidencePack
}

type AnswerGenerationResult struct {
	Status         string
	AnswerText     string
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
