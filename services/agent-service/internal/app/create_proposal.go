package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/qsyy0921/IM/services/agent-service/internal/types"
)

type CreateAgentProposalUseCase struct {
	retrieval RetrievalPort
	prepare   ToolPreparePort
	provider  ProposalProvider
	store     ProposalRepository
}

func NewCreateAgentProposalUseCase(
	retrieval RetrievalPort,
	prepare ToolPreparePort,
) CreateAgentProposalUseCase {
	return NewCreateAgentProposalUseCaseWithProvider(retrieval, prepare, ExtractiveProposalProvider{})
}

func NewCreateAgentProposalUseCaseWithProvider(
	retrieval RetrievalPort,
	prepare ToolPreparePort,
	provider ProposalProvider,
) CreateAgentProposalUseCase {
	return CreateAgentProposalUseCase{retrieval: retrieval, prepare: prepare, provider: provider}
}

func NewCreateAgentProposalUseCaseWithRepository(
	retrieval RetrievalPort,
	prepare ToolPreparePort,
	provider ProposalProvider,
	store ProposalRepository,
) CreateAgentProposalUseCase {
	return CreateAgentProposalUseCase{retrieval: retrieval, prepare: prepare, provider: provider, store: store}
}

func (usecase CreateAgentProposalUseCase) Execute(
	ctx context.Context,
	command types.CreateAgentProposalCommand,
) (types.CreateAgentProposalResult, error) {
	if err := command.Validate(); err != nil {
		return types.CreateAgentProposalResult{}, err
	}
	if usecase.prepare == nil {
		return types.CreateAgentProposalResult{}, types.ErrToolPrepareUnavailable
	}
	if usecase.retrieval == nil {
		return types.CreateAgentProposalResult{}, types.ErrRetrievalUnavailable
	}
	if usecase.provider == nil {
		return types.CreateAgentProposalResult{}, types.ErrAgentUnavailable
	}

	prepareResult, err := usecase.prepare.PrepareToolCall(ctx, types.PrepareToolCallCommand{
		AuthContext:    command.AuthContext,
		SkillID:        command.NormalizedSkillID(),
		ToolName:       command.NormalizedToolName(),
		Action:         command.ToolAction,
		ResourceType:   command.NormalizedResourceType(),
		ResourceID:     strings.TrimSpace(command.ResourceID),
		RiskLevel:      command.NormalizedRiskLevel(),
		Intent:         command.NormalizedIntent(),
		InputJSON:      prepareInputJSON(command),
		IdempotencyKey: command.ProposalID(),
	})
	if err != nil {
		return types.CreateAgentProposalResult{}, err
	}
	policyDecision := prepareResult.PolicyDecision()
	if !policyDecision.Allowed {
		result := types.CreateAgentProposalResult{
			ProposalID:         command.ProposalID(),
			Status:             types.AgentProposalStatusBlocked,
			ProposalText:       "Agent proposal blocked by tool policy.",
			RequiresApproval:   false,
			SkillID:            prepareResult.SkillID,
			PreparedAuditID:    prepareResult.AuditID,
			ToolPolicyDecision: policyDecision,
			AgentVersion:       types.AgentVersion,
			GeneratedByLLM:     false,
		}
		return result, usecase.storeProposal(ctx, command, result)
	}

	evidence, err := usecase.retrieval.RetrieveEvidence(ctx, types.RetrieveEvidenceQuery{
		AuthContext:       command.AuthContext,
		Query:             command.RetrievalQuery(),
		ConversationID:    command.ConversationID,
		AfterSeq:          command.AfterSeq,
		AtConversationSeq: command.AtConversationSeq,
		Limit:             command.EffectiveLimit(),
		IncludeSearch:     command.ShouldIncludeSearch(),
		IncludeMemory:     command.ShouldIncludeMemory(),
		MemoryStatuses:    command.EffectiveMemoryStatuses(),
	})
	if err != nil {
		return types.CreateAgentProposalResult{}, err
	}
	if len(evidence.Pack.Items) == 0 {
		result := types.CreateAgentProposalResult{
			ProposalID:         command.ProposalID(),
			Status:             types.AgentProposalStatusInsufficientEvidence,
			ProposalText:       "I do not have enough visible evidence to create an agent proposal.",
			RequiresApproval:   false,
			SkillID:            prepareResult.SkillID,
			PreparedAuditID:    prepareResult.AuditID,
			ToolPolicyDecision: policyDecision,
			EvidencePack:       evidence.Pack,
			AgentVersion:       types.AgentVersion,
			GeneratedByLLM:     false,
		}
		return result, usecase.storeProposal(ctx, command, result)
	}

	generation, err := usecase.provider.GenerateProposal(ctx, types.AgentProposalGenerationRequest{
		Objective:      command.NormalizedObjective(),
		ToolName:       command.NormalizedToolName(),
		ToolAction:     command.ToolAction,
		ResourceType:   command.NormalizedResourceType(),
		ResourceID:     strings.TrimSpace(command.ResourceID),
		RiskLevel:      command.NormalizedRiskLevel(),
		Intent:         command.NormalizedIntent(),
		PolicyDecision: policyDecision,
		EvidencePack:   evidence.Pack,
	})
	if err != nil {
		return types.CreateAgentProposalResult{}, err
	}
	if err := verifyProposalCitations(evidence.Pack, generation); err != nil {
		return types.CreateAgentProposalResult{}, err
	}
	result := types.CreateAgentProposalResult{
		ProposalID:         command.ProposalID(),
		Status:             types.AgentProposalStatusProposed,
		ProposalText:       generation.ProposalText,
		RequiresApproval:   true,
		SkillID:            prepareResult.SkillID,
		PreparedAuditID:    prepareResult.AuditID,
		ToolPolicyDecision: policyDecision,
		Citations:          generation.Citations,
		EvidencePack:       evidence.Pack,
		AgentVersion:       types.AgentVersion,
		GeneratedByLLM:     generation.GeneratedByLLM,
	}
	return result, usecase.storeProposal(ctx, command, result)
}

func (usecase CreateAgentProposalUseCase) storeProposal(
	ctx context.Context,
	command types.CreateAgentProposalCommand,
	result types.CreateAgentProposalResult,
) error {
	if usecase.store == nil {
		return nil
	}
	citationsJSON, err := encodeCitations(result.Citations)
	if err != nil {
		return err
	}
	return usecase.store.StoreAgentProposal(ctx, types.ProposalFromCreateResult(command, result, citationsJSON))
}

func encodeCitations(citations []types.Citation) (string, error) {
	payload := make([]map[string]any, 0, len(citations))
	for _, citation := range citations {
		payload = append(payload, map[string]any{
			"evidence_id":         citation.EvidenceID,
			"source_type":         citation.SourceType,
			"source_id":           citation.SourceID,
			"source_event_id":     citation.SourceEventID,
			"conversation_id":     string(citation.ConversationID),
			"conversation_seq":    citation.ConversationSeq,
			"occurred_at_unix_ms": unixMillis(citation.OccurredAt),
		})
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func unixMillis(value interface {
	IsZero() bool
	UnixMilli() int64
}) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UnixMilli()
}

func prepareInputJSON(command types.CreateAgentProposalCommand) string {
	payload := map[string]any{
		"source":                  "agent-service",
		"proposal_id":             command.ProposalID(),
		"conversation_id_present": strings.TrimSpace(string(command.ConversationID)) != "",
		"resource_id_present":     strings.TrimSpace(command.ResourceID) != "",
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

type ExtractiveProposalProvider struct{}

func (provider ExtractiveProposalProvider) GenerateProposal(
	_ context.Context,
	request types.AgentProposalGenerationRequest,
) (types.AgentProposalGenerationResult, error) {
	if len(request.EvidencePack.Items) == 0 {
		return types.AgentProposalGenerationResult{
			ProposalText:   "I do not have enough visible evidence to create an agent proposal.",
			GeneratedByLLM: false,
		}, nil
	}
	return types.AgentProposalGenerationResult{
		ProposalText:   buildExtractiveProposal(request),
		Citations:      citationsFromEvidence(request.EvidencePack.Items),
		GeneratedByLLM: false,
	}, nil
}

func buildExtractiveProposal(request types.AgentProposalGenerationRequest) string {
	lines := []string{
		"Agent proposal based on visible evidence:",
		fmt.Sprintf("- Objective: %s", compactText(request.Objective, 220)),
		fmt.Sprintf("- Tool precheck: %s %s on %s; risk=%s; policy=%s; approval required before execution.",
			request.ToolAction,
			request.ToolName,
			request.ResourceType,
			request.RiskLevel,
			policySummary(request.PolicyDecision),
		),
		"- Evidence:",
	}
	limit := len(request.EvidencePack.Items)
	if limit > types.MaxProposalEvidenceItems {
		limit = types.MaxProposalEvidenceItems
	}
	for i := 0; i < limit; i++ {
		item := request.EvidencePack.Items[i]
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		lines = append(lines, "- "+compactText(text, 180))
	}
	lines = append(lines, "- No business mutation has been executed by agent-service.")
	return strings.Join(lines, "\n")
}

func compactText(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "..."
}

func policySummary(decision types.ToolPolicyDecision) string {
	if decision.Allowed && decision.RequiresApproval {
		return "allowed_with_approval"
	}
	if decision.Allowed {
		return "allowed"
	}
	return "denied"
}

func citationsFromEvidence(items []types.EvidenceItem) []types.Citation {
	limit := len(items)
	if limit > types.MaxProposalEvidenceItems {
		limit = types.MaxProposalEvidenceItems
	}
	citations := make([]types.Citation, 0, limit)
	for i := 0; i < limit; i++ {
		item := items[i]
		citation := types.Citation{
			EvidenceID:      item.EvidenceID,
			SourceType:      item.SourceType,
			SourceID:        item.SourceID,
			ConversationID:  item.ConversationID,
			ConversationSeq: item.ConversationSeq,
			OccurredAt:      item.OccurredAt,
		}
		if len(item.SourceRefs) > 0 {
			ref := item.SourceRefs[0]
			citation.SourceID = ref.SourceID
			citation.SourceEventID = ref.SourceEventID
			citation.ConversationID = ref.ConversationID
			citation.ConversationSeq = ref.ConversationSeq
			citation.OccurredAt = ref.OccurredAt
		}
		citations = append(citations, citation)
	}
	return citations
}
