package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/qsyy0921/IM/internal/ai/llmboundary"
	"github.com/qsyy0921/IM/services/rag-service/internal/types"
)

type ExternalAnswerLLM interface {
	GenerateCandidate(context.Context, llmboundary.Prompt) (llmboundary.Candidate, error)
}

type GuardedLLMAnswerProvider struct {
	client   ExternalAnswerLLM
	fallback AnswerProvider
	options  llmboundary.Options
}

func NewGuardedLLMAnswerProvider(
	client ExternalAnswerLLM,
	options llmboundary.Options,
) GuardedLLMAnswerProvider {
	return GuardedLLMAnswerProvider{
		client:   client,
		fallback: ExtractiveAnswerProvider{},
		options:  llmboundary.NormalizeOptions(options),
	}
}

func (provider GuardedLLMAnswerProvider) GenerateAnswer(
	ctx context.Context,
	request types.AnswerGenerationRequest,
) (types.AnswerGenerationResult, error) {
	if provider.fallback == nil {
		provider.fallback = ExtractiveAnswerProvider{}
	}
	if provider.client == nil {
		return provider.fallback.GenerateAnswer(ctx, request)
	}
	prompt, err := llmboundary.BuildPrompt(
		"Answer the question using only the provided EvidencePack. Return citations by evidence_id.",
		request.Question,
		answerBoundaryEvidence(request.EvidencePack.Items),
		provider.options,
	)
	if err != nil {
		if isLLMInputFallbackError(err) {
			return provider.fallback.GenerateAnswer(ctx, request)
		}
		return types.AnswerGenerationResult{}, fmt.Errorf("%w: %v", types.ErrRAGUnavailable, err)
	}
	candidate, err := provider.client.GenerateCandidate(ctx, prompt)
	if err != nil {
		return provider.fallback.GenerateAnswer(ctx, request)
	}
	allowedIDs := evidenceIDSet(request.EvidencePack.Items)
	if err := llmboundary.ValidateCandidate(candidate, allowedIDs); err != nil {
		return types.AnswerGenerationResult{}, fmt.Errorf("%w: %v", types.ErrRAGUnavailable, err)
	}
	citations, err := citationsByEvidenceID(request.EvidencePack.Items, candidate.CitationEvidenceIDs)
	if err != nil {
		return types.AnswerGenerationResult{}, fmt.Errorf("%w: %v", types.ErrRAGUnavailable, err)
	}
	return types.AnswerGenerationResult{
		Status:         types.AnswerStatusGrounded,
		AnswerText:     candidate.Text,
		Confidence:     candidate.Confidence,
		Citations:      citations,
		GeneratedByLLM: true,
	}, nil
}

func isLLMInputFallbackError(err error) bool {
	return errors.Is(err, llmboundary.ErrUnsafeInput) ||
		errors.Is(err, llmboundary.ErrMalformedOutput)
}

func answerBoundaryEvidence(items []types.EvidenceItem) []llmboundary.Evidence {
	out := make([]llmboundary.Evidence, 0, len(items))
	for _, item := range items {
		out = append(out, llmboundary.Evidence{
			EvidenceID: item.EvidenceID,
			Text:       item.Text,
		})
	}
	return out
}

func evidenceIDSet(items []types.EvidenceItem) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.EvidenceID != "" {
			out[item.EvidenceID] = struct{}{}
		}
	}
	return out
}

func citationsByEvidenceID(items []types.EvidenceItem, evidenceIDs []string) ([]types.Citation, error) {
	byID := make(map[string]types.EvidenceItem, len(items))
	for _, item := range items {
		byID[item.EvidenceID] = item
	}
	citations := make([]types.Citation, 0, len(evidenceIDs))
	for _, evidenceID := range evidenceIDs {
		item, ok := byID[evidenceID]
		if !ok {
			return nil, fmt.Errorf("unknown evidence id %q", evidenceID)
		}
		citations = append(citations, citationFromEvidenceItem(item))
	}
	return citations, nil
}

func citationFromEvidenceItem(item types.EvidenceItem) types.Citation {
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
	return citation
}
