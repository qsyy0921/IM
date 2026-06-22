package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/internal/ai/llmboundary"
	"github.com/qsyy0921/IM/services/rag-service/internal/types"
)

func TestGuardedLLMAnswerProviderUsesGroundedCandidate(t *testing.T) {
	now := time.UnixMilli(1710000000000)
	client := &fakeAnswerLLM{candidate: llmboundary.Candidate{
		Text:                "The rollout decision is blue-green.",
		CitationEvidenceIDs: []string{"memory:m1"},
		Confidence:          0.82,
	}}
	result, err := NewGuardedLLMAnswerProvider(client, llmboundary.Options{
		TokenBudget:      256,
		MaxEvidenceItems: 1,
		MaxTextRunes:     80,
	}).GenerateAnswer(context.Background(), types.AnswerGenerationRequest{
		Question:     "what changed?",
		EvidencePack: answerEvidencePack(now),
	})
	if err != nil {
		t.Fatalf("GenerateAnswer returned error: %v", err)
	}
	if !result.GeneratedByLLM || result.AnswerText != "The rollout decision is blue-green." {
		t.Fatalf("expected LLM-generated answer, got %+v", result)
	}
	if result.Citations[0].SourceEventID != "evt-1" {
		t.Fatalf("citation did not use source ref: %+v", result.Citations[0])
	}
	if client.prompt.TokenBudget != 256 || len(client.prompt.Evidence) != 1 {
		t.Fatalf("unexpected prompt boundary: %+v", client.prompt)
	}
}

func TestGuardedLLMAnswerProviderFailsClosedOnProviderFailure(t *testing.T) {
	client := &fakeAnswerLLM{err: errors.New("provider unavailable")}
	_, err := NewGuardedLLMAnswerProvider(client, llmboundary.Options{}).GenerateAnswer(
		context.Background(),
		types.AnswerGenerationRequest{Question: "what changed?", EvidencePack: answerEvidencePack(time.Time{})},
	)
	if !errors.Is(err, types.ErrRAGUnavailable) {
		t.Fatalf("expected rag unavailable for provider failure, got %v", err)
	}
}

func TestGuardedLLMAnswerProviderDoesNotCallExternalWithSensitiveInput(t *testing.T) {
	client := &fakeAnswerLLM{}
	pack := answerEvidencePack(time.Time{})
	pack.Items[0].Text = "contact alice@example.com for launch"
	_, err := NewGuardedLLMAnswerProvider(client, llmboundary.Options{}).GenerateAnswer(
		context.Background(),
		types.AnswerGenerationRequest{Question: "who owns launch?", EvidencePack: pack},
	)
	if !errors.Is(err, types.ErrRAGUnavailable) {
		t.Fatalf("expected rag unavailable for sensitive input, got %v", err)
	}
	if client.called {
		t.Fatal("external LLM client must not be called with sensitive EvidencePack text")
	}
}

func TestGuardedLLMAnswerProviderRejectsUnsafeOutput(t *testing.T) {
	client := &fakeAnswerLLM{candidate: llmboundary.Candidate{
		Text:                "Bearer provider-token-value",
		CitationEvidenceIDs: []string{"memory:m1"},
		Confidence:          0.7,
	}}
	_, err := NewGuardedLLMAnswerProvider(client, llmboundary.Options{}).GenerateAnswer(
		context.Background(),
		types.AnswerGenerationRequest{Question: "what changed?", EvidencePack: answerEvidencePack(time.Time{})},
	)
	if !errors.Is(err, types.ErrRAGUnavailable) {
		t.Fatalf("expected rag unavailable for unsafe output, got %v", err)
	}
}

func TestGuardedLLMAnswerProviderRejectsMalformedCitation(t *testing.T) {
	client := &fakeAnswerLLM{candidate: llmboundary.Candidate{
		Text:                "Ungrounded answer.",
		CitationEvidenceIDs: []string{"missing"},
		Confidence:          0.7,
	}}
	_, err := NewGuardedLLMAnswerProvider(client, llmboundary.Options{}).GenerateAnswer(
		context.Background(),
		types.AnswerGenerationRequest{Question: "what changed?", EvidencePack: answerEvidencePack(time.Time{})},
	)
	if !errors.Is(err, types.ErrRAGUnavailable) {
		t.Fatalf("expected rag unavailable for malformed output, got %v", err)
	}
}

type fakeAnswerLLM struct {
	prompt    llmboundary.Prompt
	candidate llmboundary.Candidate
	err       error
	called    bool
}

func (client *fakeAnswerLLM) GenerateCandidate(
	_ context.Context,
	prompt llmboundary.Prompt,
) (llmboundary.Candidate, error) {
	client.called = true
	client.prompt = prompt
	if client.err != nil {
		return llmboundary.Candidate{}, client.err
	}
	return client.candidate, nil
}

func answerEvidencePack(now time.Time) types.EvidencePack {
	return types.EvidencePack{
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
	}
}
