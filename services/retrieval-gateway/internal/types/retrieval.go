package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	RetrievalVersion = "retrieval-gateway.v1.hybrid-source-vector-rrf-graph-depth1"

	DefaultEvidenceLimit = 20
	MaxEvidenceLimit     = 50
	MaxRetrievalQueryLen = 512

	EvidenceSourceSearchMessage    = "SEARCH_MESSAGE"
	EvidenceSourceMemoryEvent      = "MEMORY_EVENT"
	EvidenceSourceProfileAggregate = "PROFILE_AGGREGATE"
	EvidenceSourceVectorItem       = "VECTOR_ITEM"

	EvidenceCoverageNotRequested = "NOT_REQUESTED"
	EvidenceCoverageEmpty        = "EMPTY"
	EvidenceCoverageReturned     = "RETURNED"
	EvidenceCoverageFiltered     = "FILTERED"

	EvidenceDedupeUniqueSource             = "UNIQUE_SOURCE"
	EvidenceDedupeKeptFirstDuplicateSource = "KEPT_FIRST_DUPLICATE_SOURCE"

	RetrievalPolicyToolName                 = "retrieval.evidence"
	RetrievalPolicyIntent                   = "retrieve_evidence"
	RetrievalPolicyResourceTypeConversation = "conversation"
	RetrievalPolicyResourceTypeTenant       = "tenant"
	RetrievalPolicyRiskLow                  = "LOW"

	MemoryStatusPending    = "PENDING"
	MemoryStatusActive     = "ACTIVE"
	MemoryStatusSuperseded = "SUPERSEDED"
	MemoryStatusArchived   = "ARCHIVED"

	VectorCollectionKnowledgeChunk   = "KNOWLEDGE_CHUNK"
	VectorCollectionMemoryEvent      = "MEMORY_EVENT"
	VectorCollectionSearchDocument   = "SEARCH_DOCUMENT"
	VectorCollectionProfileAggregate = "PROFILE_AGGREGATE"
	VectorCollectionEvalFixture      = "EVAL_FIXTURE"
)

type RetrieveEvidenceCommand struct {
	AuthContext           AuthContext
	Query                 string
	ConversationID        ConversationID
	AfterSeq              int64
	Limit                 int
	IncludeSearch         bool
	IncludeMemory         bool
	IncludeVector         bool
	MemoryStatuses        []string
	AtConversationSeq     int64
	QueryEmbeddingRef     string
	VectorCollections     []string
	VectorVisibilityScope string
	VectorPolicyVersion   string
	VectorMinScore        float64
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
	if command.AtConversationSeq < 0 {
		return NewInvalidArgument("at_conversation_seq must be non-negative")
	}
	if command.Limit < 0 || command.Limit > MaxEvidenceLimit {
		return NewInvalidArgument("limit must be between 0 and 50")
	}
	for _, status := range command.MemoryStatuses {
		if !isValidMemoryStatus(status) {
			return NewInvalidArgument("invalid memory status")
		}
	}
	if command.ShouldIncludeVector() {
		if strings.TrimSpace(command.QueryEmbeddingRef) == "" {
			return NewInvalidArgument("query_embedding_ref is required for vector retrieval")
		}
		if containsSensitiveValue(command.QueryEmbeddingRef) {
			return NewInvalidArgument("query_embedding_ref must be a low-sensitive reference")
		}
		if len(command.EffectiveVectorCollections()) == 0 {
			return NewInvalidArgument("vector_collection_types are required")
		}
		if strings.TrimSpace(command.VectorVisibilityScope) == "" || strings.TrimSpace(command.VectorPolicyVersion) == "" {
			return NewInvalidArgument("vector visibility_scope and policy_version are required")
		}
		if containsSensitiveValue(command.VectorVisibilityScope, command.VectorPolicyVersion) {
			return NewInvalidArgument("vector visibility metadata must be low-sensitive")
		}
		if command.VectorMinScore < 0 || command.VectorMinScore > 1 {
			return NewInvalidArgument("vector_min_score must be between 0 and 1")
		}
		for _, collection := range command.EffectiveVectorCollections() {
			if !isValidVectorCollection(collection) {
				return NewInvalidArgument("invalid vector collection type")
			}
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
	return command.IncludeSearch || (!command.IncludeMemory && !command.IncludeVector)
}

func (command RetrieveEvidenceCommand) ShouldIncludeMemory() bool {
	return command.IncludeMemory || (!command.IncludeSearch && !command.IncludeVector)
}

func (command RetrieveEvidenceCommand) ShouldIncludeVector() bool {
	return command.IncludeVector
}

func (command RetrieveEvidenceCommand) EffectiveMemoryStatuses() []string {
	if len(command.MemoryStatuses) > 0 {
		return command.MemoryStatuses
	}
	return []string{MemoryStatusActive}
}

func (command RetrieveEvidenceCommand) EffectiveVectorCollections() []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(command.VectorCollections))
	for _, collection := range command.VectorCollections {
		normalized := strings.ToUpper(strings.TrimSpace(collection))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func (command RetrieveEvidenceCommand) PackID() string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%d|%d|%d|%t|%t|%t|%s|%s|%s|%s|%.6f",
		command.AuthContext.TenantID,
		command.AuthContext.UserID,
		command.NormalizedQuery(),
		command.AfterSeq,
		command.AtConversationSeq,
		command.EffectiveLimit(),
		command.ShouldIncludeSearch(),
		command.ShouldIncludeMemory(),
		command.ShouldIncludeVector(),
		command.QueryEmbeddingRef,
		strings.Join(command.EffectiveVectorCollections(), ","),
		command.VectorVisibilityScope,
		command.VectorPolicyVersion,
		command.VectorMinScore,
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

type VectorQuery struct {
	AuthContext        AuthContext
	RequesterRef       string
	RetrievalRequestID string
	CollectionTypes    []string
	QueryEmbeddingRef  string
	TopK               int
	MinScore           float64
	VisibilityScope    string
	PolicyVersion      string
	At                 time.Time
}

type VectorResult struct {
	Items []VectorItemEvidence
}

type VectorItemEvidence struct {
	VectorItemRef     string
	SourceRefHash     string
	SourceService     string
	CollectionType    string
	Score             float64
	VisibilityVersion int64
	TombstoneStatus   string
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
	AtConversationSeq int64
	Statuses          []string
	Limit             int
}

type MemoryResult struct {
	Items             []MemoryEventEvidence
	ProjectionVersion int64
}

type MemoryEventLookup struct {
	AuthContext   AuthContext
	MemoryEventID string
}

type MemoryEventLookupResult struct {
	Item       MemoryEventEvidence
	GraphEdges []MemoryGraphEdge
}

type ProfileAggregateQuery struct {
	AuthContext   AuthContext
	SubjectUserID UserID
	AggregateType string
	Statuses      []string
	Limit         int
}

type ProfileAggregateResult struct {
	Items []ProfileAggregateEvidence
}

type RetrievalPolicyCheck struct {
	AuthContext    AuthContext
	ConversationID ConversationID
}

type RetrievalPolicyDecision struct {
	Allowed           bool
	RequiresApproval  bool
	PermissionVersion int64
	Classification    string
	Reason            string
	DecisionSource    string
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
	GraphEdges        []MemoryGraphEdge
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

type ProfileAggregateEvidence struct {
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
	VectorItemRef            string
	VectorSourceRefHash      string
	VectorSourceService      string
	VectorCollectionType     string
	VectorTombstoneStatus    string
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

func isValidMemoryStatus(status string) bool {
	switch status {
	case MemoryStatusPending, MemoryStatusActive, MemoryStatusSuperseded, MemoryStatusArchived:
		return true
	default:
		return false
	}
}

func isValidVectorCollection(collection string) bool {
	switch strings.ToUpper(strings.TrimSpace(collection)) {
	case VectorCollectionKnowledgeChunk, VectorCollectionMemoryEvent, VectorCollectionSearchDocument,
		VectorCollectionProfileAggregate, VectorCollectionEvalFixture:
		return true
	default:
		return false
	}
}

func containsSensitiveValue(values ...string) bool {
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		for _, marker := range []string{"secret", "token", "api_key", "apikey", "password", "private://", "raw:", "dsn=", "postgres://", "http://", "https://", "s3://"} {
			if strings.Contains(normalized, marker) {
				return true
			}
		}
	}
	return false
}
