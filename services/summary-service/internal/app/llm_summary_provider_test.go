package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/internal/ai/llmboundary"
	"github.com/qsyy0921/IM/services/summary-service/internal/types"
)

func TestGuardedLLMSummaryProviderUsesGroundedCandidate(t *testing.T) {
	now := time.UnixMilli(1710000000000)
	client := &fakeSummaryLLM{candidate: llmboundary.Candidate{
		Text:                "Summary: deployment stays blue-green.",
		CitationEvidenceIDs: []string{"memory:m1"},
		Confidence:          0.81,
	}}
	result, err := NewGuardedLLMSummaryProvider(client, llmboundary.Options{
		TokenBudget:      300,
		MaxEvidenceItems: 1,
		MaxTextRunes:     80,
	}).GenerateSummary(context.Background(), types.SummaryGenerationRequest{
		Focus:        "release recap",
		EvidencePack: summaryEvidencePack(now),
	})
	if err != nil {
		t.Fatalf("GenerateSummary returned error: %v", err)
	}
	if !result.GeneratedByLLM || result.SummaryText != "Summary: deployment stays blue-green." {
		t.Fatalf("expected LLM-generated summary, got %+v", result)
	}
	if result.Citations[0].SourceEventID != "evt-1" {
		t.Fatalf("citation did not use source ref: %+v", result.Citations[0])
	}
	if client.prompt.TokenBudget != 300 || len(client.prompt.Evidence) != 1 {
		t.Fatalf("unexpected prompt boundary: %+v", client.prompt)
	}
}

func TestGuardedLLMSummaryProviderFallsBackOnProviderFailure(t *testing.T) {
	client := &fakeSummaryLLM{err: errors.New("provider unavailable")}
	result, err := NewGuardedLLMSummaryProvider(client, llmboundary.Options{}).GenerateSummary(
		context.Background(),
		types.SummaryGenerationRequest{Focus: "release recap", EvidencePack: summaryEvidencePack(time.Time{})},
	)
	if err != nil {
		t.Fatalf("GenerateSummary returned error: %v", err)
	}
	if result.GeneratedByLLM {
		t.Fatalf("provider failure must fall back to non-LLM extractive summary: %+v", result)
	}
	if !strings.Contains(result.SummaryText, "blue-green") {
		t.Fatalf("fallback did not use evidence text: %q", result.SummaryText)
	}
}

func TestGuardedLLMSummaryProviderDoesNotCallExternalWithSensitiveInput(t *testing.T) {
	client := &fakeSummaryLLM{}
	pack := summaryEvidencePack(time.Time{})
	pack.Items[0].Text = "call 13812345678 for rollout"
	result, err := NewGuardedLLMSummaryProvider(client, llmboundary.Options{}).GenerateSummary(
		context.Background(),
		types.SummaryGenerationRequest{Focus: "release recap", EvidencePack: pack},
	)
	if err != nil {
		t.Fatalf("GenerateSummary returned error: %v", err)
	}
	if client.called {
		t.Fatal("external LLM client must not be called with sensitive EvidencePack text")
	}
	if result.GeneratedByLLM {
		t.Fatalf("sensitive input fallback must not claim LLM generation: %+v", result)
	}
}

func TestGuardedLLMSummaryProviderRejectsUnsafeOutput(t *testing.T) {
	client := &fakeSummaryLLM{candidate: llmboundary.Candidate{
		Text:                "sk-provider-secret-value",
		CitationEvidenceIDs: []string{"memory:m1"},
		Confidence:          0.7,
	}}
	_, err := NewGuardedLLMSummaryProvider(client, llmboundary.Options{}).GenerateSummary(
		context.Background(),
		types.SummaryGenerationRequest{Focus: "release recap", EvidencePack: summaryEvidencePack(time.Time{})},
	)
	if !errors.Is(err, types.ErrSummaryUnavailable) {
		t.Fatalf("expected summary unavailable for unsafe output, got %v", err)
	}
}

func TestGuardedLLMSummaryProviderRejectsMalformedCitation(t *testing.T) {
	client := &fakeSummaryLLM{candidate: llmboundary.Candidate{
		Text:                "Ungrounded summary.",
		CitationEvidenceIDs: []string{"missing"},
		Confidence:          0.7,
	}}
	_, err := NewGuardedLLMSummaryProvider(client, llmboundary.Options{}).GenerateSummary(
		context.Background(),
		types.SummaryGenerationRequest{Focus: "release recap", EvidencePack: summaryEvidencePack(time.Time{})},
	)
	if !errors.Is(err, types.ErrSummaryUnavailable) {
		t.Fatalf("expected summary unavailable for malformed output, got %v", err)
	}
}

type fakeSummaryLLM struct {
	prompt    llmboundary.Prompt
	candidate llmboundary.Candidate
	err       error
	called    bool
}

func (client *fakeSummaryLLM) GenerateCandidate(
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

func summaryEvidencePack(now time.Time) types.EvidencePack {
	return types.EvidencePack{
		PackID:         "ep_1",
		TenantID:       "tenant-1",
		Query:          "release recap",
		ConversationID: "conv-1",
		Items: []types.EvidenceItem{{
			EvidenceID:      "memory:m1",
			SourceType:      types.EvidenceSourceMemoryEvent,
			SourceID:        "m1",
			ConversationID:  "conv-1",
			ConversationSeq: 7,
			Text:            "Alice confirmed the active deployment plan is blue-green.",
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
