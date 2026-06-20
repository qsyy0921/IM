package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/qsyy0921/IM/services/rag-service/internal/types"
)

type AnswerQuestionUseCase struct {
	retrieval RetrievalPort
	provider  AnswerProvider
}

func NewAnswerQuestionUseCase(retrieval RetrievalPort) AnswerQuestionUseCase {
	return NewAnswerQuestionUseCaseWithProvider(retrieval, ExtractiveAnswerProvider{})
}

func NewAnswerQuestionUseCaseWithProvider(retrieval RetrievalPort, provider AnswerProvider) AnswerQuestionUseCase {
	return AnswerQuestionUseCase{retrieval: retrieval, provider: provider}
}

func (usecase AnswerQuestionUseCase) Execute(
	ctx context.Context,
	command types.AnswerQuestionCommand,
) (types.AnswerQuestionResult, error) {
	if err := command.Validate(); err != nil {
		return types.AnswerQuestionResult{}, err
	}
	if usecase.retrieval == nil {
		return types.AnswerQuestionResult{}, types.ErrRetrievalUnavailable
	}
	if usecase.provider == nil {
		return types.AnswerQuestionResult{}, types.ErrRAGUnavailable
	}
	evidence, err := usecase.retrieval.RetrieveEvidence(ctx, types.RetrieveEvidenceQuery{
		AuthContext:       command.AuthContext,
		Query:             command.NormalizedQuestion(),
		ConversationID:    command.ConversationID,
		AfterSeq:          command.AfterSeq,
		AtConversationSeq: command.AtConversationSeq,
		Limit:             command.EffectiveLimit(),
		IncludeSearch:     command.ShouldIncludeSearch(),
		IncludeMemory:     command.ShouldIncludeMemory(),
		MemoryStatuses:    command.EffectiveMemoryStatuses(),
	})
	if err != nil {
		return types.AnswerQuestionResult{}, err
	}
	generation, err := usecase.provider.GenerateAnswer(ctx, types.AnswerGenerationRequest{
		Question:     command.NormalizedQuestion(),
		EvidencePack: evidence.Pack,
	})
	if err != nil {
		return types.AnswerQuestionResult{}, err
	}
	if err := verifyAnswerCitations(evidence.Pack, generation); err != nil {
		return types.AnswerQuestionResult{}, err
	}
	return types.AnswerQuestionResult{
		AnswerID:       command.AnswerID(),
		Status:         generation.Status,
		AnswerText:     generation.AnswerText,
		Confidence:     generation.Confidence,
		Citations:      generation.Citations,
		EvidencePack:   evidence.Pack,
		RAGVersion:     types.RAGVersion,
		GeneratedByLLM: generation.GeneratedByLLM,
	}, nil
}

type ExtractiveAnswerProvider struct{}

func (provider ExtractiveAnswerProvider) GenerateAnswer(
	_ context.Context,
	request types.AnswerGenerationRequest,
) (types.AnswerGenerationResult, error) {
	if len(request.EvidencePack.Items) == 0 {
		return types.AnswerGenerationResult{
			Status:         types.AnswerStatusInsufficientEvidence,
			AnswerText:     "I do not have enough visible evidence to answer this question.",
			Confidence:     0,
			GeneratedByLLM: false,
		}, nil
	}
	return types.AnswerGenerationResult{
		Status:         types.AnswerStatusGrounded,
		AnswerText:     buildExtractiveAnswer(request.EvidencePack.Items),
		Confidence:     answerConfidence(request.EvidencePack.Items),
		Citations:      citationsFromEvidence(request.EvidencePack.Items),
		GeneratedByLLM: false,
	}, nil
}

func buildExtractiveAnswer(items []types.EvidenceItem) string {
	parts := make([]string, 0, types.MaxExtractiveAnswerItems)
	for _, item := range items {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("[%d] %s", len(parts)+1, text))
		if len(parts) >= types.MaxExtractiveAnswerItems {
			break
		}
	}
	if len(parts) == 0 {
		return "I do not have enough visible evidence to answer this question."
	}
	return "Grounded extractive answer: " + strings.Join(parts, " ")
}

func answerConfidence(items []types.EvidenceItem) float64 {
	if len(items) == 0 {
		return 0
	}
	total := 0.0
	for _, item := range items {
		score := item.RerankScore
		if score == 0 {
			score = item.Score
		}
		if score < 0 {
			score = 0
		}
		if score > 1 {
			score = 1
		}
		total += score
	}
	return total / float64(len(items))
}

func citationsFromEvidence(items []types.EvidenceItem) []types.Citation {
	citations := make([]types.Citation, 0, len(items))
	for _, item := range items {
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
