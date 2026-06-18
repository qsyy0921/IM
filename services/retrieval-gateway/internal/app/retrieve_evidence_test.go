package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/retrieval-gateway/internal/types"
)

func TestRetrieveEvidenceMergesSearchAndMemory(t *testing.T) {
	now := time.Unix(10, 0)
	search := fakeSearchPort{result: types.SearchResult{
		ProjectionVersion: 7,
		Items: []types.SearchMessageEvidence{{
			ConversationID:    "conv-1",
			MessageID:         "msg-1",
			ConversationSeq:   10,
			SourceEventID:     "evt-1",
			SenderID:          "user-a",
			Snippet:           "search snippet",
			OccurredAt:        now,
			VisibilityVersion: 3,
		}},
	}}
	memory := fakeMemoryPort{result: types.MemoryResult{
		ProjectionVersion: 9,
		Items: []types.MemoryEventEvidence{{
			MemoryEventID:     "mem-1",
			ConversationID:    "conv-1",
			Status:            types.MemoryStatusPending,
			ReviewState:       "UNREVIEWED",
			FactText:          "memory fact",
			ActorUserIDs:      []string{"user-a"},
			SourceRefs:        []types.EvidenceSourceRef{{SourceType: "MESSAGE", SourceID: "msg-1", ConversationSeq: 10}},
			ValidFromSeq:      10,
			Confidence:        0.8,
			VisibilityVersion: 4,
			ExtractionVersion: "memory.v1",
		}},
	}}

	result, err := NewRetrieveEvidenceUseCase(&search, &memory).Execute(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got := len(result.Pack.Items); got != 2 {
		t.Fatalf("expected 2 items, got %d", got)
	}
	if result.Pack.SearchProjectionVersion != 7 || result.Pack.MemoryProjectionVersion != 9 {
		t.Fatalf("unexpected projection versions: %+v", result.Pack)
	}
	if result.Pack.Items[0].SourceType != types.EvidenceSourceSearchMessage {
		t.Fatalf("expected search item first, got %+v", result.Pack.Items[0])
	}
	if result.Pack.Items[1].SourceType != types.EvidenceSourceMemoryEvent {
		t.Fatalf("expected memory item second, got %+v", result.Pack.Items[1])
	}
	if result.Pack.RetrievalVersion != types.RetrievalVersion {
		t.Fatalf("unexpected retrieval version %q", result.Pack.RetrievalVersion)
	}
	if result.Pack.Items[0].RerankScore != 1 || result.Pack.Items[0].DedupeReason != types.EvidenceDedupeUniqueSource {
		t.Fatalf("unexpected search item rank metadata: %+v", result.Pack.Items[0])
	}
	if result.Pack.Items[1].RerankScore != 0.8 || result.Pack.Items[1].DedupeReason != types.EvidenceDedupeUniqueSource {
		t.Fatalf("unexpected memory item rank metadata: %+v", result.Pack.Items[1])
	}
	if got := result.Pack.SourceCoverage; len(got) != 2 ||
		got[0].SourceType != types.EvidenceSourceSearchMessage ||
		got[0].CandidateCount != 1 ||
		got[0].ReturnedCount != 1 ||
		got[0].Status != types.EvidenceCoverageReturned ||
		got[1].SourceType != types.EvidenceSourceMemoryEvent ||
		got[1].CandidateCount != 1 ||
		got[1].ReturnedCount != 1 ||
		got[1].Status != types.EvidenceCoverageReturned {
		t.Fatalf("unexpected source coverage: %+v", got)
	}
}

func TestRetrieveEvidenceDefaultsToBothSources(t *testing.T) {
	search := fakeSearchPort{}
	memory := fakeMemoryPort{}
	_, err := NewRetrieveEvidenceUseCase(&search, &memory).Execute(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !search.called || !memory.called {
		t.Fatalf("expected both ports to be called: search=%t memory=%t", search.called, memory.called)
	}
}

func TestRetrieveEvidenceChecksPolicyBeforeSources(t *testing.T) {
	search := fakeSearchPort{}
	memory := fakeMemoryPort{}
	policy := fakePolicyPort{decision: types.RetrievalPolicyDecision{Allowed: true}}
	_, err := NewRetrieveEvidenceUseCase(&search, &memory, WithPolicyPort(&policy)).Execute(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !policy.called {
		t.Fatal("expected policy to be checked")
	}
	if policy.check.ConversationID != "conv-1" {
		t.Fatalf("unexpected policy check: %+v", policy.check)
	}
	if !search.called || !memory.called {
		t.Fatalf("expected sources after policy allow: search=%t memory=%t", search.called, memory.called)
	}
}

func TestRetrieveEvidencePolicyDenySkipsSources(t *testing.T) {
	search := fakeSearchPort{}
	memory := fakeMemoryPort{}
	policy := fakePolicyPort{decision: types.RetrievalPolicyDecision{Allowed: false}}
	_, err := NewRetrieveEvidenceUseCase(&search, &memory, WithPolicyPort(&policy)).Execute(context.Background(), validCommand())
	if !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected permission denied, got %v", err)
	}
	if !policy.called {
		t.Fatal("expected policy to be checked")
	}
	if search.called || memory.called {
		t.Fatalf("sources should not be called after policy deny: search=%t memory=%t", search.called, memory.called)
	}
}

func TestRetrieveEvidencePolicyApprovalRequiredSkipsSources(t *testing.T) {
	search := fakeSearchPort{}
	memory := fakeMemoryPort{}
	policy := fakePolicyPort{decision: types.RetrievalPolicyDecision{Allowed: true, RequiresApproval: true}}
	_, err := NewRetrieveEvidenceUseCase(&search, &memory, WithPolicyPort(&policy)).Execute(context.Background(), validCommand())
	if !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected permission denied, got %v", err)
	}
	if search.called || memory.called {
		t.Fatalf("sources should not be called after approval requirement: search=%t memory=%t", search.called, memory.called)
	}
}

func TestRetrieveEvidencePolicyUnavailableSkipsSources(t *testing.T) {
	search := fakeSearchPort{}
	memory := fakeMemoryPort{}
	policy := fakePolicyPort{err: types.ErrRetrievalUnavailable}
	_, err := NewRetrieveEvidenceUseCase(&search, &memory, WithPolicyPort(&policy)).Execute(context.Background(), validCommand())
	if !errors.Is(err, types.ErrRetrievalUnavailable) {
		t.Fatalf("expected retrieval unavailable, got %v", err)
	}
	if search.called || memory.called {
		t.Fatalf("sources should not be called after policy error: search=%t memory=%t", search.called, memory.called)
	}
}

func TestRetrieveEvidenceHonorsSourceFlags(t *testing.T) {
	command := validCommand()
	command.IncludeSearch = true
	command.IncludeMemory = false
	search := fakeSearchPort{}
	memory := fakeMemoryPort{}
	_, err := NewRetrieveEvidenceUseCase(&search, &memory).Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !search.called || memory.called {
		t.Fatalf("expected only search to be called: search=%t memory=%t", search.called, memory.called)
	}
}

func TestRetrieveEvidenceDedupeAndCoverage(t *testing.T) {
	command := validCommand()
	command.IncludeSearch = false
	command.IncludeMemory = true
	memory := fakeMemoryPort{result: types.MemoryResult{
		Items: []types.MemoryEventEvidence{
			{MemoryEventID: "mem-dup", ConversationID: "conv-1", FactText: "first", Confidence: 0.7},
			{MemoryEventID: "mem-dup", ConversationID: "conv-1", FactText: "duplicate", Confidence: 0.9},
		},
	}}
	result, err := NewRetrieveEvidenceUseCase(&fakeSearchPort{}, &memory).Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got := len(result.Pack.Items); got != 1 {
		t.Fatalf("expected one deduped item, got %d", got)
	}
	if item := result.Pack.Items[0]; item.Text != "first" || item.DedupeReason != types.EvidenceDedupeKeptFirstDuplicateSource {
		t.Fatalf("unexpected deduped item: %+v", item)
	}
	if got := result.Pack.SourceCoverage; len(got) != 2 ||
		got[0].SourceType != types.EvidenceSourceSearchMessage ||
		got[0].Requested ||
		got[0].Status != types.EvidenceCoverageNotRequested ||
		got[1].SourceType != types.EvidenceSourceMemoryEvent ||
		!got[1].Requested ||
		got[1].CandidateCount != 2 ||
		got[1].ReturnedCount != 1 ||
		got[1].DedupedCount != 1 ||
		got[1].Status != types.EvidenceCoverageReturned {
		t.Fatalf("unexpected source coverage: %+v", got)
	}
}

func TestRetrieveEvidenceReranksBeforeTruncating(t *testing.T) {
	command := validCommand()
	command.IncludeSearch = false
	command.IncludeMemory = true
	command.Limit = 1
	memory := fakeMemoryPort{result: types.MemoryResult{
		Items: []types.MemoryEventEvidence{
			{MemoryEventID: "mem-low", ConversationID: "conv-1", FactText: "low", Confidence: 0.1},
			{MemoryEventID: "mem-high", ConversationID: "conv-1", FactText: "high", Confidence: 0.9},
		},
	}}
	result, err := NewRetrieveEvidenceUseCase(&fakeSearchPort{}, &memory).Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got := result.Pack.Items[0].MemoryEventID; got != "mem-high" {
		t.Fatalf("expected highest rerank item, got %q", got)
	}
	if coverage := result.Pack.SourceCoverage[1]; coverage.CandidateCount != 2 || coverage.ReturnedCount != 1 || coverage.Status != types.EvidenceCoverageReturned {
		t.Fatalf("unexpected memory coverage: %+v", coverage)
	}
}

func TestRetrieveEvidencePropagatesPortError(t *testing.T) {
	expected := errors.New("boom")
	_, err := NewRetrieveEvidenceUseCase(&fakeSearchPort{err: expected}, &fakeMemoryPort{}).Execute(context.Background(), validCommand())
	if !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

func TestRetrieveEvidenceRequiresQuery(t *testing.T) {
	command := validCommand()
	command.Query = " "
	_, err := NewRetrieveEvidenceUseCase(&fakeSearchPort{}, &fakeMemoryPort{}).Execute(context.Background(), command)
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func validCommand() types.RetrieveEvidenceCommand {
	return types.RetrieveEvidenceCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		Query:          "project launch",
		ConversationID: "conv-1",
		Limit:          10,
	}
}

type fakeSearchPort struct {
	result types.SearchResult
	err    error
	called bool
}

func (port *fakeSearchPort) SearchMessages(context.Context, types.SearchQuery) (types.SearchResult, error) {
	port.called = true
	return port.result, port.err
}

type fakeMemoryPort struct {
	result types.MemoryResult
	err    error
	called bool
}

func (port *fakeMemoryPort) QueryMemoryEvents(context.Context, types.MemoryQuery) (types.MemoryResult, error) {
	port.called = true
	return port.result, port.err
}

type fakePolicyPort struct {
	decision types.RetrievalPolicyDecision
	err      error
	check    types.RetrievalPolicyCheck
	called   bool
}

func (port *fakePolicyPort) CheckRetrieveEvidence(
	_ context.Context,
	check types.RetrievalPolicyCheck,
) (types.RetrievalPolicyDecision, error) {
	port.called = true
	port.check = check
	return port.decision, port.err
}
