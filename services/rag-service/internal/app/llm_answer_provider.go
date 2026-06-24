package app

import (
	"context"
	"fmt"

	"github.com/qsyy0921/IM/internal/ai/llmboundary"
	"github.com/qsyy0921/IM/services/rag-service/internal/types"
)

type ExternalAnswerLLM interface {
	GenerateCandidate(context.Context, llmboundary.Prompt) (llmboundary.Candidate, error)
}

type GuardedLLMAnswerProvider struct {
	client  ExternalAnswerLLM
	options llmboundary.Options
}

func NewGuardedLLMAnswerProvider(
	client ExternalAnswerLLM,
	options llmboundary.Options,
) GuardedLLMAnswerProvider {
	return GuardedLLMAnswerProvider{
		client:  client,
		options: llmboundary.NormalizeOptions(options),
	}
}

func (provider GuardedLLMAnswerProvider) GenerateAnswer(
	ctx context.Context,
	request types.AnswerGenerationRequest,
) (types.AnswerGenerationResult, error) {
	if provider.client == nil {
		return types.AnswerGenerationResult{}, types.ErrRAGUnavailable
	}
	pack, err := groundableEvidencePack(request.EvidencePack)
	if err != nil {
		return types.AnswerGenerationResult{}, fmt.Errorf("%w: %v", types.ErrRAGUnavailable, err)
	}
	prompt, err := llmboundary.BuildPrompt(
		"Answer the question using only the provided EvidencePack. Return citations by evidence_id.",
		request.Question,
		answerBoundaryEvidence(pack.Items),
		provider.options,
	)
	if err != nil {
		return types.AnswerGenerationResult{}, fmt.Errorf("%w: %v", types.ErrRAGUnavailable, err)
	}
	candidate, err := provider.client.GenerateCandidate(ctx, prompt)
	if err != nil {
		return types.AnswerGenerationResult{}, fmt.Errorf("%w: model provider failed", types.ErrRAGUnavailable)
	}
	allowedIDs := evidenceIDSet(pack.Items)
	if err := llmboundary.ValidateCandidate(candidate, allowedIDs); err != nil {
		return types.AnswerGenerationResult{}, fmt.Errorf("%w: %v", types.ErrRAGUnavailable, err)
	}
	citations, err := citationsByEvidenceID(pack.Items, candidate.CitationEvidenceIDs)
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
		citation, err := citationFromEvidenceItem(item)
		if err != nil {
			return nil, err
		}
		citations = append(citations, citation)
	}
	return citations, nil
}

func citationFromEvidenceItem(item types.EvidenceItem) (types.Citation, error) {
	if len(item.SourceRefs) == 0 {
		return types.Citation{}, fmt.Errorf("evidence %q has no source_ref", item.EvidenceID)
	}
	ref := item.SourceRefs[0]
	return types.Citation{
		EvidenceID:      item.EvidenceID,
		SourceType:      item.SourceType,
		SourceID:        ref.SourceID,
		SourceEventID:   ref.SourceEventID,
		ConversationID:  ref.ConversationID,
		ConversationSeq: ref.ConversationSeq,
		OccurredAt:      ref.OccurredAt,
	}, nil
}
