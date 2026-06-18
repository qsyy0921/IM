package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/rag-service/internal/types"
)

func TestAnswerQuestionBuildsGroundedExtractiveAnswer(t *testing.T) {
	now := time.UnixMilli(1710000000000)
	retrieval := &fakeRetrievalPort{result: types.RetrieveEvidenceResult{
		Pack: types.EvidencePack{
			PackID:         "ep_1",
			TenantID:       "tenant-1",
			Query:          "what changed?",
			ConversationID: "conv-1",
			Items: []types.EvidenceItem{{
				EvidenceID:      "memory:m1",
				SourceType:      types.EvidenceSourceMemoryEvent,
				SourceID:        "m1",
				ConversationID:  "conv-1",
				ConversationSeq: 7,
				Text:            "The active deployment plan is blue-green.",
				Score:           0.9,
				RerankScore:     0.9,
				SourceRefs: []types.EvidenceSourceRef{{
					SourceType:      "MESSAGE",
					SourceID:        "msg-1",
					SourceEventID:   "evt-1",
					ConversationID:  "conv-1",
					ConversationSeq: 7,
					OccurredAt:      now,
				}},
			}},
		},
	}}
	result, err := NewAnswerQuestionUseCase(retrieval).Execute(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != types.AnswerStatusGrounded {
		t.Fatalf("unexpected status %s", result.Status)
	}
	if result.GeneratedByLLM {
		t.Fatal("first-stage rag-service must not claim LLM generation")
	}
	if !strings.Contains(result.AnswerText, "blue-green") {
		t.Fatalf("answer did not include evidence text: %q", result.AnswerText)
	}
	if len(result.Citations) != 1 {
		t.Fatalf("expected 1 citation, got %d", len(result.Citations))
	}
	if result.Citations[0].SourceEventID != "evt-1" || result.Citations[0].ConversationSeq != 7 {
		t.Fatalf("citation lost source ref: %+v", result.Citations[0])
	}
	if retrieval.query.Query != "what changed?" || !retrieval.query.IncludeSearch || !retrieval.query.IncludeMemory {
		t.Fatalf("unexpected retrieval query: %+v", retrieval.query)
	}
	if len(retrieval.query.MemoryStatuses) != 1 || retrieval.query.MemoryStatuses[0] != types.MemoryStatusActive {
		t.Fatalf("expected active memory by default, got %+v", retrieval.query.MemoryStatuses)
	}
}

func TestAnswerQuestionAbstainsWithoutEvidence(t *testing.T) {
	result, err := NewAnswerQuestionUseCase(&fakeRetrievalPort{}).Execute(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != types.AnswerStatusInsufficientEvidence {
		t.Fatalf("unexpected status %s", result.Status)
	}
	if result.Confidence != 0 {
		t.Fatalf("expected zero confidence, got %f", result.Confidence)
	}
	if len(result.Citations) != 0 {
		t.Fatalf("expected no citations, got %d", len(result.Citations))
	}
}

func TestAnswerQuestionPropagatesRetrievalError(t *testing.T) {
	expected := types.ErrRetrievalUnavailable
	_, err := NewAnswerQuestionUseCase(&fakeRetrievalPort{err: expected}).Execute(context.Background(), validCommand())
	if !errors.Is(err, expected) {
		t.Fatalf("expected retrieval error, got %v", err)
	}
}

func TestAnswerQuestionRequiresQuestion(t *testing.T) {
	command := validCommand()
	command.Question = " "
	_, err := NewAnswerQuestionUseCase(&fakeRetrievalPort{}).Execute(context.Background(), command)
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func validCommand() types.AnswerQuestionCommand {
	return types.AnswerQuestionCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		Question:       " what changed? ",
		ConversationID: "conv-1",
		Limit:          5,
	}
}

type fakeRetrievalPort struct {
	query  types.RetrieveEvidenceQuery
	result types.RetrieveEvidenceResult
	err    error
}

func (port *fakeRetrievalPort) RetrieveEvidence(
	_ context.Context,
	query types.RetrieveEvidenceQuery,
) (types.RetrieveEvidenceResult, error) {
	port.query = query
	if port.err != nil {
		return types.RetrieveEvidenceResult{}, port.err
	}
	return port.result, nil
}
