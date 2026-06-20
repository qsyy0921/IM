package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/summary-service/internal/types"
)

func TestGenerateConversationSummaryBuildsGroundedExtractiveSummary(t *testing.T) {
	now := time.UnixMilli(1710000000000)
	retrieval := &fakeRetrievalPort{result: types.RetrieveEvidenceResult{
		Pack: types.EvidencePack{
			Items: []types.EvidenceItem{{
				EvidenceID:      "search:msg-1",
				SourceType:      types.EvidenceSourceSearchMessage,
				SourceID:        "msg-1",
				ConversationID:  "conv-1",
				ConversationSeq: 7,
				Text:            "Alice confirmed the rollout plan.",
				OccurredAt:      now,
			}},
		},
	}}
	result, err := NewGenerateConversationSummaryUseCase(retrieval).Execute(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != types.SummaryStatusGrounded {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if !strings.Contains(result.SummaryText, "Alice confirmed") {
		t.Fatalf("summary did not use evidence text: %q", result.SummaryText)
	}
	if len(result.Citations) != 1 || result.Citations[0].EvidenceID != "search:msg-1" {
		t.Fatalf("unexpected citations: %#v", result.Citations)
	}
	if result.GeneratedByLLM {
		t.Fatal("first-stage summary should not claim LLM generation")
	}
	if retrieval.query.Query != "release recap" {
		t.Fatalf("unexpected retrieval query: %q", retrieval.query.Query)
	}
	if retrieval.query.AtConversationSeq != 13 {
		t.Fatalf("expected at_conversation_seq to be forwarded, got %d", retrieval.query.AtConversationSeq)
	}
}

func TestGenerateConversationSummaryAbstainsWithoutEvidence(t *testing.T) {
	result, err := NewGenerateConversationSummaryUseCase(&fakeRetrievalPort{}).Execute(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != types.SummaryStatusInsufficientEvidence {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if len(result.Citations) != 0 {
		t.Fatalf("abstention should not include citations: %#v", result.Citations)
	}
}

func TestGenerateConversationSummaryUsesProviderBoundary(t *testing.T) {
	now := time.UnixMilli(1710000000000)
	retrieval := &fakeRetrievalPort{result: types.RetrieveEvidenceResult{
		Pack: types.EvidencePack{
			Items: []types.EvidenceItem{{
				EvidenceID:      "memory:event-1",
				SourceType:      types.EvidenceSourceMemoryEvent,
				SourceID:        "mem-1",
				ConversationID:  "conv-1",
				ConversationSeq: 9,
				Text:            "memory text",
				SourceRefs: []types.EvidenceSourceRef{{
					SourceID:        "msg-9",
					SourceEventID:   "evt-9",
					ConversationID:  "conv-1",
					ConversationSeq: 9,
					OccurredAt:      now,
				}},
			}},
		},
	}}
	provider := fakeSummaryProvider{result: types.SummaryGenerationResult{
		Status:      types.SummaryStatusGrounded,
		SummaryText: "provider summary",
		Confidence:  0.82,
		Citations: []types.Citation{{
			EvidenceID:      "memory:event-1",
			SourceType:      types.EvidenceSourceMemoryEvent,
			SourceID:        "msg-9",
			SourceEventID:   "evt-9",
			ConversationID:  "conv-1",
			ConversationSeq: 9,
			OccurredAt:      now,
		}},
	}}
	result, err := NewGenerateConversationSummaryUseCaseWithProvider(retrieval, &provider).Execute(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.SummaryText != "provider summary" {
		t.Fatalf("provider summary was not used: %q", result.SummaryText)
	}
	if !provider.called {
		t.Fatal("provider was not called")
	}
	if provider.request.Focus != "release recap" {
		t.Fatalf("unexpected provider focus: %q", provider.request.Focus)
	}
}

func TestGenerateConversationSummaryRejectsUngroundedCitation(t *testing.T) {
	retrieval := &fakeRetrievalPort{result: types.RetrieveEvidenceResult{
		Pack: types.EvidencePack{
			Items: []types.EvidenceItem{{
				EvidenceID:      "search:msg-1",
				SourceType:      types.EvidenceSourceSearchMessage,
				SourceID:        "msg-1",
				ConversationID:  "conv-1",
				ConversationSeq: 3,
				Text:            "source text",
			}},
		},
	}}
	provider := fakeSummaryProvider{result: types.SummaryGenerationResult{
		Status:      types.SummaryStatusGrounded,
		SummaryText: "ungrounded summary",
		Confidence:  0.8,
		Citations: []types.Citation{{
			EvidenceID:      "search:msg-1",
			SourceType:      types.EvidenceSourceSearchMessage,
			SourceID:        "different-message",
			ConversationID:  "conv-1",
			ConversationSeq: 3,
		}},
	}}
	_, err := NewGenerateConversationSummaryUseCaseWithProvider(retrieval, &provider).Execute(context.Background(), validCommand())
	if !errors.Is(err, types.ErrCitationVerification) {
		t.Fatalf("expected citation verification error, got %v", err)
	}
}

func validCommand() types.GenerateConversationSummaryCommand {
	return types.GenerateConversationSummaryCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID:    "conv-1",
		Focus:             "release recap",
		AtConversationSeq: 13,
		Limit:             3,
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

type fakeSummaryProvider struct {
	request types.SummaryGenerationRequest
	result  types.SummaryGenerationResult
	err     error
	called  bool
}

func (provider *fakeSummaryProvider) GenerateSummary(
	_ context.Context,
	request types.SummaryGenerationRequest,
) (types.SummaryGenerationResult, error) {
	provider.called = true
	provider.request = request
	if provider.err != nil {
		return types.SummaryGenerationResult{}, provider.err
	}
	return provider.result, nil
}
