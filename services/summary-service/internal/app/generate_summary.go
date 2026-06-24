package app

import (
	"context"
	"strings"

	"github.com/qsyy0921/IM/services/summary-service/internal/types"
)

type GenerateConversationSummaryUseCase struct {
	retrieval RetrievalPort
	provider  SummaryProvider
}

func NewGenerateConversationSummaryUseCase(retrieval RetrievalPort) GenerateConversationSummaryUseCase {
	return NewGenerateConversationSummaryUseCaseWithProvider(retrieval, ExtractiveSummaryProvider{})
}

func NewGenerateConversationSummaryUseCaseWithProvider(
	retrieval RetrievalPort,
	provider SummaryProvider,
) GenerateConversationSummaryUseCase {
	return GenerateConversationSummaryUseCase{retrieval: retrieval, provider: provider}
}

func (usecase GenerateConversationSummaryUseCase) Execute(
	ctx context.Context,
	command types.GenerateConversationSummaryCommand,
) (types.GenerateConversationSummaryResult, error) {
	if err := command.Validate(); err != nil {
		return types.GenerateConversationSummaryResult{}, err
	}
	if usecase.retrieval == nil {
		return types.GenerateConversationSummaryResult{}, types.ErrRetrievalUnavailable
	}
	if usecase.provider == nil {
		return types.GenerateConversationSummaryResult{}, types.ErrSummaryUnavailable
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
		return types.GenerateConversationSummaryResult{}, err
	}
	generationPack, err := groundableEvidencePack(evidence.Pack)
	if err != nil {
		return types.GenerateConversationSummaryResult{}, err
	}
	generation, err := usecase.provider.GenerateSummary(ctx, types.SummaryGenerationRequest{
		Focus:        command.NormalizedFocus(),
		EvidencePack: generationPack,
	})
	if err != nil {
		return types.GenerateConversationSummaryResult{}, err
	}
	if err := verifySummaryCitations(evidence.Pack, generation); err != nil {
		return types.GenerateConversationSummaryResult{}, err
	}
	return types.GenerateConversationSummaryResult{
		SummaryID:      command.SummaryID(),
		Status:         generation.Status,
		SummaryText:    generation.SummaryText,
		Confidence:     generation.Confidence,
		Citations:      generation.Citations,
		EvidencePack:   evidence.Pack,
		SummaryVersion: types.SummaryVersion,
		GeneratedByLLM: generation.GeneratedByLLM,
	}, nil
}

type ExtractiveSummaryProvider struct{}

func (provider ExtractiveSummaryProvider) GenerateSummary(
	_ context.Context,
	request types.SummaryGenerationRequest,
) (types.SummaryGenerationResult, error) {
	pack, err := groundableEvidencePack(request.EvidencePack)
	if err != nil {
		return types.SummaryGenerationResult{}, err
	}
	if len(pack.Items) == 0 {
		return types.SummaryGenerationResult{
			Status:         types.SummaryStatusInsufficientEvidence,
			SummaryText:    "I do not have enough visible evidence to summarize this conversation.",
			Confidence:     0,
			GeneratedByLLM: false,
		}, nil
	}
	return types.SummaryGenerationResult{
		Status:         types.SummaryStatusGrounded,
		SummaryText:    buildExtractiveSummary(pack.Items),
		Confidence:     summaryConfidence(pack.Items),
		Citations:      citationsFromEvidence(pack.Items),
		GeneratedByLLM: false,
	}, nil
}

func buildExtractiveSummary(items []types.EvidenceItem) string {
	limit := len(items)
	if limit > types.MaxExtractiveSummaryItems {
		limit = types.MaxExtractiveSummaryItems
	}
	lines := make([]string, 0, limit+1)
	lines = append(lines, "Summary based on visible evidence:")
	for i := 0; i < limit; i++ {
		text := strings.TrimSpace(items[i].Text)
		if text == "" {
			continue
		}
		lines = append(lines, "- "+compactText(text, 180))
	}
	if len(lines) == 1 {
		return "Summary based on visible evidence."
	}
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

func summaryConfidence(items []types.EvidenceItem) float64 {
	if len(items) == 0 {
		return 0
	}
	count := len(items)
	if count > types.MaxExtractiveSummaryItems {
		count = types.MaxExtractiveSummaryItems
	}
	score := 0.65 + float64(count)*0.05
	if score > 0.9 {
		return 0.9
	}
	return score
}

func citationsFromEvidence(items []types.EvidenceItem) []types.Citation {
	limit := len(items)
	if limit > types.MaxExtractiveSummaryItems {
		limit = types.MaxExtractiveSummaryItems
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
