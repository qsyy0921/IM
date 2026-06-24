package app

import (
	"context"
	"fmt"

	"github.com/qsyy0921/IM/internal/ai/llmboundary"
	"github.com/qsyy0921/IM/services/summary-service/internal/types"
)

type ExternalSummaryLLM interface {
	GenerateCandidate(context.Context, llmboundary.Prompt) (llmboundary.Candidate, error)
}

type GuardedLLMSummaryProvider struct {
	client  ExternalSummaryLLM
	options llmboundary.Options
}

func NewGuardedLLMSummaryProvider(
	client ExternalSummaryLLM,
	options llmboundary.Options,
) GuardedLLMSummaryProvider {
	return GuardedLLMSummaryProvider{
		client:  client,
		options: llmboundary.NormalizeOptions(options),
	}
}

func (provider GuardedLLMSummaryProvider) GenerateSummary(
	ctx context.Context,
	request types.SummaryGenerationRequest,
) (types.SummaryGenerationResult, error) {
	if provider.client == nil {
		return types.SummaryGenerationResult{}, types.ErrSummaryUnavailable
	}
	pack, err := groundableEvidencePack(request.EvidencePack)
	if err != nil {
		return types.SummaryGenerationResult{}, fmt.Errorf("%w: %v", types.ErrSummaryUnavailable, err)
	}
	prompt, err := llmboundary.BuildPrompt(
		"Summarize the conversation using only the provided EvidencePack. Return citations by evidence_id.",
		request.Focus,
		summaryBoundaryEvidence(pack.Items),
		provider.options,
	)
	if err != nil {
		return types.SummaryGenerationResult{}, fmt.Errorf("%w: %v", types.ErrSummaryUnavailable, err)
	}
	candidate, err := provider.client.GenerateCandidate(ctx, prompt)
	if err != nil {
		return types.SummaryGenerationResult{}, fmt.Errorf("%w: model provider failed", types.ErrSummaryUnavailable)
	}
	allowedIDs := summaryEvidenceIDSet(pack.Items)
	if err := llmboundary.ValidateCandidate(candidate, allowedIDs); err != nil {
		return types.SummaryGenerationResult{}, fmt.Errorf("%w: %v", types.ErrSummaryUnavailable, err)
	}
	citations, err := summaryCitationsByEvidenceID(pack.Items, candidate.CitationEvidenceIDs)
	if err != nil {
		return types.SummaryGenerationResult{}, fmt.Errorf("%w: %v", types.ErrSummaryUnavailable, err)
	}
	return types.SummaryGenerationResult{
		Status:         types.SummaryStatusGrounded,
		SummaryText:    candidate.Text,
		Confidence:     candidate.Confidence,
		Citations:      citations,
		GeneratedByLLM: true,
	}, nil
}

func summaryBoundaryEvidence(items []types.EvidenceItem) []llmboundary.Evidence {
	out := make([]llmboundary.Evidence, 0, len(items))
	for _, item := range items {
		out = append(out, llmboundary.Evidence{
			EvidenceID: item.EvidenceID,
			Text:       item.Text,
		})
	}
	return out
}

func summaryEvidenceIDSet(items []types.EvidenceItem) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.EvidenceID != "" {
			out[item.EvidenceID] = struct{}{}
		}
	}
	return out
}

func summaryCitationsByEvidenceID(items []types.EvidenceItem, evidenceIDs []string) ([]types.Citation, error) {
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
		citation, err := summaryCitationFromEvidenceItem(item)
		if err != nil {
			return nil, err
		}
		citations = append(citations, citation)
	}
	return citations, nil
}

func summaryCitationFromEvidenceItem(item types.EvidenceItem) (types.Citation, error) {
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
