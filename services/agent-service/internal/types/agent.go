package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	AgentVersion = "agent-service.v1"

	DefaultAgentEvidenceLimit = 12
	MaxAgentEvidenceLimit     = 30
	MaxAgentObjectiveLen      = 1024
	MaxToolNameLen            = 160
	MaxResourceTypeLen        = 120
	MaxResourceIDLen          = 256
	MaxRiskLevelLen           = 64
	MaxIntentLen              = 1024
	MaxProposalEvidenceItems  = 5

	AgentProposalStatusProposed             = "PROPOSED"
	AgentProposalStatusBlocked              = "BLOCKED"
	AgentProposalStatusInsufficientEvidence = "INSUFFICIENT_EVIDENCE"

	ToolActionCall    = "CALL"
	ToolActionApprove = "APPROVE"
	ToolActionExecute = "EXECUTE"

	EvidenceSourceSearchMessage = "SEARCH_MESSAGE"
	EvidenceSourceMemoryEvent   = "MEMORY_EVENT"

	MemoryStatusPending    = "PENDING"
	MemoryStatusActive     = "ACTIVE"
	MemoryStatusSuperseded = "SUPERSEDED"
	MemoryStatusArchived   = "ARCHIVED"
)

type CreateAgentProposalCommand struct {
	AuthContext    AuthContext
	ConversationID ConversationID
	Objective      string
	ToolName       string
	ToolAction     string
	ResourceType   string
	ResourceID     string
	RiskLevel      string
	Intent         string
	AfterSeq       int64
	Limit          int
	IncludeSearch  bool
	IncludeMemory  bool
	MemoryStatuses []string
}

func (command CreateAgentProposalCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(string(command.ConversationID)) == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	if command.NormalizedObjective() == "" {
		return NewInvalidArgument("objective is required")
	}
	if len([]rune(command.NormalizedObjective())) > MaxAgentObjectiveLen {
		return NewInvalidArgument("objective exceeds maximum")
	}
	if command.NormalizedToolName() == "" {
		return NewInvalidArgument("tool_name is required")
	}
	if len([]rune(command.NormalizedToolName())) > MaxToolNameLen {
		return NewInvalidArgument("tool_name exceeds maximum")
	}
	if !isValidToolAction(command.ToolAction) {
		return NewInvalidArgument("tool_action is required")
	}
	if command.NormalizedResourceType() == "" {
		return NewInvalidArgument("resource_type is required")
	}
	if len([]rune(command.NormalizedResourceType())) > MaxResourceTypeLen {
		return NewInvalidArgument("resource_type exceeds maximum")
	}
	if len([]rune(strings.TrimSpace(command.ResourceID))) > MaxResourceIDLen {
		return NewInvalidArgument("resource_id exceeds maximum")
	}
	if len([]rune(command.NormalizedRiskLevel())) > MaxRiskLevelLen {
		return NewInvalidArgument("risk_level exceeds maximum")
	}
	if len([]rune(command.NormalizedIntent())) > MaxIntentLen {
		return NewInvalidArgument("intent exceeds maximum")
	}
	if command.AfterSeq < 0 {
		return NewInvalidArgument("after_seq must be non-negative")
	}
	if command.Limit < 0 || command.Limit > MaxAgentEvidenceLimit {
		return NewInvalidArgument("limit must be between 0 and 30")
	}
	for _, status := range command.MemoryStatuses {
		if !isValidMemoryStatus(status) {
			return NewInvalidArgument("invalid memory status")
		}
	}
	return nil
}

func (command CreateAgentProposalCommand) NormalizedObjective() string {
	return strings.TrimSpace(command.Objective)
}

func (command CreateAgentProposalCommand) NormalizedToolName() string {
	return strings.TrimSpace(command.ToolName)
}

func (command CreateAgentProposalCommand) NormalizedResourceType() string {
	return strings.TrimSpace(command.ResourceType)
}

func (command CreateAgentProposalCommand) NormalizedRiskLevel() string {
	risk := strings.TrimSpace(command.RiskLevel)
	if risk == "" {
		return "LOW"
	}
	return strings.ToUpper(risk)
}

func (command CreateAgentProposalCommand) NormalizedIntent() string {
	intent := strings.TrimSpace(command.Intent)
	if intent == "" {
		return command.NormalizedObjective()
	}
	return intent
}

func (command CreateAgentProposalCommand) RetrievalQuery() string {
	return command.NormalizedObjective()
}

func (command CreateAgentProposalCommand) EffectiveLimit() int {
	if command.Limit <= 0 {
		return DefaultAgentEvidenceLimit
	}
	return command.Limit
}

func (command CreateAgentProposalCommand) ShouldIncludeSearch() bool {
	return command.IncludeSearch || !command.IncludeMemory
}

func (command CreateAgentProposalCommand) ShouldIncludeMemory() bool {
	return command.IncludeMemory || !command.IncludeSearch
}

func (command CreateAgentProposalCommand) EffectiveMemoryStatuses() []string {
	if len(command.MemoryStatuses) > 0 {
		return command.MemoryStatuses
	}
	return []string{MemoryStatusActive}
}

func (command CreateAgentProposalCommand) ProposalID() string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%d|%d",
		command.AuthContext.TenantID,
		command.AuthContext.UserID,
		command.ConversationID,
		command.NormalizedObjective(),
		command.NormalizedToolName(),
		command.ToolAction,
		command.NormalizedResourceType(),
		command.AfterSeq,
		command.EffectiveLimit(),
	)))
	return "ap_" + hex.EncodeToString(sum[:8])
}

type RetrieveEvidenceQuery struct {
	AuthContext    AuthContext
	Query          string
	ConversationID ConversationID
	AfterSeq       int64
	Limit          int
	IncludeSearch  bool
	IncludeMemory  bool
	MemoryStatuses []string
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
	RerankScore       float64
	DedupeReason      string
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

type CheckToolActionCommand struct {
	AuthContext  AuthContext
	ToolName     string
	Action       string
	ResourceType string
	ResourceID   string
	RiskLevel    string
	Intent       string
}

type ToolPolicyDecision struct {
	TenantID          TenantID
	UserID            UserID
	ToolName          string
	Action            string
	ResourceType      string
	ResourceID        string
	RiskLevel         string
	Allowed           bool
	RequiresApproval  bool
	PermissionVersion int64
	Classification    string
	Reason            string
	DecisionSource    string
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

type AgentProposalGenerationRequest struct {
	Objective      string
	ToolName       string
	ToolAction     string
	ResourceType   string
	ResourceID     string
	RiskLevel      string
	Intent         string
	PolicyDecision ToolPolicyDecision
	EvidencePack   EvidencePack
}

type AgentProposalGenerationResult struct {
	ProposalText   string
	Citations      []Citation
	GeneratedByLLM bool
}

type CreateAgentProposalResult struct {
	ProposalID         string
	Status             string
	ProposalText       string
	RequiresApproval   bool
	ToolPolicyDecision ToolPolicyDecision
	Citations          []Citation
	EvidencePack       EvidencePack
	AgentVersion       string
	GeneratedByLLM     bool
}

func isValidToolAction(action string) bool {
	switch action {
	case ToolActionCall, ToolActionApprove, ToolActionExecute:
		return true
	default:
		return false
	}
}

func isValidMemoryStatus(status string) bool {
	switch status {
	case MemoryStatusPending, MemoryStatusActive, MemoryStatusSuperseded, MemoryStatusArchived:
		return true
	default:
		return false
	}
}
