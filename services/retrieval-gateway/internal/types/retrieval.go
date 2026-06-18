package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	RetrievalVersion = "retrieval-gateway.v1"

	DefaultEvidenceLimit = 20
	MaxEvidenceLimit     = 50
	MaxRetrievalQueryLen = 512

	EvidenceSourceSearchMessage = "SEARCH_MESSAGE"
	EvidenceSourceMemoryEvent   = "MEMORY_EVENT"

	MemoryStatusPending    = "PENDING"
	MemoryStatusActive     = "ACTIVE"
	MemoryStatusSuperseded = "SUPERSEDED"
	MemoryStatusArchived   = "ARCHIVED"
)

type RetrieveEvidenceCommand struct {
	AuthContext    AuthContext
	Query          string
	ConversationID ConversationID
	AfterSeq       int64
	Limit          int
	IncludeSearch  bool
	IncludeMemory  bool
	MemoryStatuses []string
}

func (command RetrieveEvidenceCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.Query) == "" {
		return NewInvalidArgument("query is required")
	}
	if len([]rune(command.NormalizedQuery())) > MaxRetrievalQueryLen {
		return NewInvalidArgument("query exceeds maximum")
	}
	if command.AfterSeq < 0 {
		return NewInvalidArgument("after_seq must be non-negative")
	}
	if command.Limit < 0 || command.Limit > MaxEvidenceLimit {
		return NewInvalidArgument("limit must be between 0 and 50")
	}
	for _, status := range command.MemoryStatuses {
		if !isValidMemoryStatus(status) {
			return NewInvalidArgument("invalid memory status")
		}
	}
	return nil
}

func (command RetrieveEvidenceCommand) NormalizedQuery() string {
	return strings.TrimSpace(command.Query)
}

func (command RetrieveEvidenceCommand) EffectiveLimit() int {
	if command.Limit <= 0 {
		return DefaultEvidenceLimit
	}
	return command.Limit
}

func (command RetrieveEvidenceCommand) ShouldIncludeSearch() bool {
	return command.IncludeSearch || !command.IncludeMemory
}

func (command RetrieveEvidenceCommand) ShouldIncludeMemory() bool {
	return command.IncludeMemory || !command.IncludeSearch
}

func (command RetrieveEvidenceCommand) EffectiveMemoryStatuses() []string {
	if len(command.MemoryStatuses) > 0 {
		return command.MemoryStatuses
	}
	return []string{MemoryStatusPending, MemoryStatusActive}
}

func (command RetrieveEvidenceCommand) PackID() string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%d|%d|%t|%t",
		command.AuthContext.TenantID,
		command.AuthContext.UserID,
		command.NormalizedQuery(),
		command.AfterSeq,
		command.EffectiveLimit(),
		command.ShouldIncludeSearch(),
		command.ShouldIncludeMemory(),
	)))
	return "ep_" + hex.EncodeToString(sum[:8])
}

type SearchQuery struct {
	AuthContext    AuthContext
	Query          string
	ConversationID ConversationID
	AfterSeq       int64
	Limit          int
}

type SearchResult struct {
	Items             []SearchMessageEvidence
	ProjectionVersion int64
}

type SearchMessageEvidence struct {
	ConversationID    ConversationID
	MessageID         string
	ConversationSeq   int64
	SourceEventID     string
	SenderID          UserID
	MessageType       string
	Snippet           string
	OccurredAt        time.Time
	VisibilityVersion int64
}

type MemoryQuery struct {
	AuthContext       AuthContext
	Query             string
	ConversationID    ConversationID
	AfterValidFromSeq int64
	Statuses          []string
	Limit             int
}

type MemoryResult struct {
	Items             []MemoryEventEvidence
	ProjectionVersion int64
}

type MemoryEventEvidence struct {
	MemoryEventID     string
	ConversationID    ConversationID
	Topic             string
	Status            string
	ReviewState       string
	FactText          string
	ActorUserIDs      []string
	AudienceUserIDs   []string
	SourceRefs        []EvidenceSourceRef
	ValidFromSeq      int64
	ValidToSeq        int64
	ValidFromAt       time.Time
	Confidence        float64
	VisibilityVersion int64
	ExtractionVersion string
}

type EvidenceSourceRef struct {
	SourceType      string
	SourceID        string
	SourceEventID   string
	ConversationID  ConversationID
	ConversationSeq int64
	OccurredAt      time.Time
}

type EvidenceItem struct {
	EvidenceID        string
	SourceType        string
	SourceID          string
	ConversationID    ConversationID
	ConversationSeq   int64
	Text              string
	Score             float64
	SpeakerUserID     UserID
	MessageID         string
	MemoryEventID     string
	OccurredAt        time.Time
	ValidFromSeq      int64
	ValidToSeq        int64
	VisibilityVersion int64
	SourceRefs        []EvidenceSourceRef
	ActorUserIDs      []string
	AudienceUserIDs   []string
	TemporalStatus    string
	ReviewState       string
	ExtractionVersion string
}

type EvidenceSourceCount struct {
	SourceType string
	Count      int
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
}

type RetrieveEvidenceResult struct {
	Pack EvidencePack
}

func isValidMemoryStatus(status string) bool {
	switch status {
	case MemoryStatusPending, MemoryStatusActive, MemoryStatusSuperseded, MemoryStatusArchived:
		return true
	default:
		return false
	}
}
