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
			Status:            types.MemoryStatusActive,
			ReviewState:       "APPROVED",
			FactText:          "memory fact",
			ActorUserIDs:      []string{"user-a"},
			SourceRefs:        []types.EvidenceSourceRef{{SourceType: "MESSAGE", SourceID: "msg-1", ConversationSeq: 10}},
			ValidFromSeq:      10,
			Confidence:        0.8,
			VisibilityVersion: 4,
			ExtractionVersion: "memory.v1",
		}},
	}, details: map[string]types.MemoryEventLookupResult{
		"mem-1": {
			Item: types.MemoryEventEvidence{
				MemoryEventID:     "mem-1",
				ConversationID:    "conv-1",
				Status:            types.MemoryStatusActive,
				ReviewState:       "APPROVED",
				FactText:          "memory fact",
				ActorUserIDs:      []string{"user-a"},
				SourceRefs:        []types.EvidenceSourceRef{{SourceType: "MESSAGE", SourceID: "msg-1", ConversationSeq: 10}},
				ValidFromSeq:      10,
				Confidence:        0.8,
				VisibilityVersion: 4,
				ExtractionVersion: "memory.v1",
			},
			GraphEdges: []types.MemoryGraphEdge{{
				EdgeID:            "edge-1",
				FromMemoryEventID: "mem-1",
				ToMemoryEventID:   "mem-2",
				RelationType:      "SUPPORTS",
				Confidence:        0.7,
				SourceRefs:        []types.EvidenceSourceRef{{SourceType: "MESSAGE", SourceID: "msg-1", ConversationSeq: 10}},
			}},
		},
		"mem-2": {
			Item: types.MemoryEventEvidence{
				MemoryEventID:     "mem-2",
				ConversationID:    "conv-1",
				Status:            types.MemoryStatusActive,
				ReviewState:       "APPROVED",
				FactText:          "linked memory fact",
				SourceRefs:        []types.EvidenceSourceRef{{SourceType: "MESSAGE", SourceID: "msg-2", ConversationSeq: 11}},
				ValidFromSeq:      11,
				Confidence:        0.66,
				VisibilityVersion: 4,
				ExtractionVersion: "memory.v1",
			},
		},
	}}

	result, err := NewRetrieveEvidenceUseCase(&search, &memory).Execute(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got := len(result.Pack.Items); got != 3 {
		t.Fatalf("expected 3 items, got %d", got)
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
	if result.Pack.Items[0].RerankScore <= 1 || result.Pack.Items[0].DedupeReason != types.EvidenceDedupeUniqueSource {
		t.Fatalf("unexpected search item rank metadata: %+v", result.Pack.Items[0])
	}
	if result.Pack.Items[1].RerankScore <= result.Pack.Items[1].Score ||
		result.Pack.Items[1].RerankScore < 0.9 ||
		result.Pack.Items[1].DedupeReason != types.EvidenceDedupeUniqueSource {
		t.Fatalf("unexpected memory item rank metadata: %+v", result.Pack.Items[1])
	}
	if got := result.Pack.Items[1].MemoryGraphEdges; len(got) != 1 || got[0].RelationType != "SUPPORTS" || len(got[0].SourceRefs) != 1 {
		t.Fatalf("expected memory graph edge to be carried in evidence item: %+v", got)
	}
	if result.Pack.Items[2].SourceType != types.EvidenceSourceMemoryEvent ||
		result.Pack.Items[2].MemoryEventID != "mem-2" ||
		result.Pack.Items[2].Text != "linked memory fact" {
		t.Fatalf("expected one-hop graph adjacent memory evidence, got %+v", result.Pack.Items[2])
	}
	if got := memory.query.AtConversationSeq; got != 10 {
		t.Fatalf("expected memory current query at search seq 10, got %d", got)
	}
	if got := memory.query.Statuses; len(got) != 1 || got[0] != types.MemoryStatusActive {
		t.Fatalf("expected active memory status by default, got %+v", got)
	}
	if got := result.Pack.SourceCoverage; len(got) != 4 ||
		got[0].SourceType != types.EvidenceSourceSearchMessage ||
		got[0].CandidateCount != 1 ||
		got[0].ReturnedCount != 1 ||
		got[0].Status != types.EvidenceCoverageReturned ||
		got[1].SourceType != types.EvidenceSourceVectorItem ||
		got[1].Requested ||
		got[1].Status != types.EvidenceCoverageNotRequested ||
		got[2].SourceType != types.EvidenceSourceMemoryEvent ||
		got[2].CandidateCount != 2 ||
		got[2].ReturnedCount != 2 ||
		got[2].Status != types.EvidenceCoverageReturned ||
		got[3].SourceType != types.EvidenceSourceProfileAggregate ||
		got[3].CandidateCount != 0 ||
		got[3].ReturnedCount != 0 ||
		got[3].Status != types.EvidenceCoverageEmpty {
		t.Fatalf("unexpected source coverage: %+v", got)
	}
}

func TestRetrieveEvidenceGraphExpansionDepthZeroDisablesAdjacentMemory(t *testing.T) {
	command := validCommand()
	command.IncludeSearch = false
	command.IncludeMemory = true
	memory := fakeMemoryPort{
		result: types.MemoryResult{
			Items: []types.MemoryEventEvidence{{MemoryEventID: "mem-1", ConversationID: "conv-1", FactText: "root", Status: types.MemoryStatusActive, Confidence: 0.8}},
		},
		details: map[string]types.MemoryEventLookupResult{
			"mem-1": {
				Item: types.MemoryEventEvidence{MemoryEventID: "mem-1", ConversationID: "conv-1", FactText: "root", Status: types.MemoryStatusActive, Confidence: 0.8},
				GraphEdges: []types.MemoryGraphEdge{{
					EdgeID:            "edge-1",
					FromMemoryEventID: "mem-1",
					ToMemoryEventID:   "mem-2",
					RelationType:      "SUPPORTS",
				}},
			},
			"mem-2": {
				Item: types.MemoryEventEvidence{MemoryEventID: "mem-2", ConversationID: "conv-1", FactText: "adjacent", Status: types.MemoryStatusActive, Confidence: 0.7},
			},
		},
	}

	result, err := NewRetrieveEvidenceUseCase(
		&fakeSearchPort{},
		&memory,
		WithGraphExpansionDepth(0),
	).Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got := len(result.Pack.Items); got != 1 {
		t.Fatalf("expected only root memory when graph depth is zero, got %d items: %+v", got, result.Pack.Items)
	}
	if got := result.Pack.Items[0].MemoryEventID; got != "mem-1" {
		t.Fatalf("expected root memory, got %q", got)
	}
	if result.Pack.RetrievalVersion != types.RetrievalVersionForGraphDepth(0) {
		t.Fatalf("unexpected retrieval version %q", result.Pack.RetrievalVersion)
	}
	if coverage := result.Pack.SourceCoverage[2]; coverage.CandidateCount != 1 || coverage.ReturnedCount != 1 {
		t.Fatalf("unexpected memory coverage with graph depth zero: %+v", coverage)
	}
}

func TestRetrieveEvidenceGraphExpansionDepthTwoIncludesSecondHop(t *testing.T) {
	command := validCommand()
	command.IncludeSearch = false
	command.IncludeMemory = true
	memory := fakeMemoryPort{
		result: types.MemoryResult{
			Items: []types.MemoryEventEvidence{{MemoryEventID: "mem-1", ConversationID: "conv-1", FactText: "root", Status: types.MemoryStatusActive, Confidence: 0.8}},
		},
		details: map[string]types.MemoryEventLookupResult{
			"mem-1": {
				Item: types.MemoryEventEvidence{MemoryEventID: "mem-1", ConversationID: "conv-1", FactText: "root", Status: types.MemoryStatusActive, Confidence: 0.8},
				GraphEdges: []types.MemoryGraphEdge{{
					EdgeID:            "edge-1",
					FromMemoryEventID: "mem-1",
					ToMemoryEventID:   "mem-2",
					RelationType:      "SUPPORTS",
				}},
			},
			"mem-2": {
				Item: types.MemoryEventEvidence{MemoryEventID: "mem-2", ConversationID: "conv-1", FactText: "first hop", Status: types.MemoryStatusActive, Confidence: 0.7},
				GraphEdges: []types.MemoryGraphEdge{{
					EdgeID:            "edge-2",
					FromMemoryEventID: "mem-2",
					ToMemoryEventID:   "mem-3",
					RelationType:      "SUPPORTS",
				}},
			},
			"mem-3": {
				Item: types.MemoryEventEvidence{MemoryEventID: "mem-3", ConversationID: "conv-1", FactText: "second hop", Status: types.MemoryStatusActive, Confidence: 0.6},
			},
		},
	}

	result, err := NewRetrieveEvidenceUseCase(
		&fakeSearchPort{},
		&memory,
		WithGraphExpansionDepth(2),
	).Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Pack.RetrievalVersion != types.RetrievalVersionForGraphDepth(2) {
		t.Fatalf("unexpected retrieval version %q", result.Pack.RetrievalVersion)
	}
	ids := map[string]bool{}
	for _, item := range result.Pack.Items {
		ids[item.MemoryEventID] = true
	}
	for _, id := range []string{"mem-1", "mem-2", "mem-3"} {
		if !ids[id] {
			t.Fatalf("expected graph-expanded memory %s in EvidencePack, got %+v", id, result.Pack.Items)
		}
	}
	if coverage := result.Pack.SourceCoverage[2]; coverage.CandidateCount != 3 || coverage.ReturnedCount != 3 {
		t.Fatalf("unexpected depth-two memory coverage: %+v", coverage)
	}
}

func TestRetrieveEvidenceInvalidGraphExpansionDepthFailsClosed(t *testing.T) {
	_, err := NewRetrieveEvidenceUseCase(
		&fakeSearchPort{},
		&fakeMemoryPort{},
		WithGraphExpansionDepth(types.MaxGraphExpansionDepth+1),
	).Execute(context.Background(), validCommand())
	if !errors.Is(err, types.ErrRetrievalUnavailable) {
		t.Fatalf("expected retrieval unavailable from invalid graph expansion depth, got %v", err)
	}
}

func TestRetrieveEvidenceUsesExplicitMemoryAtConversationSeq(t *testing.T) {
	command := validCommand()
	command.IncludeSearch = false
	command.IncludeMemory = true
	command.AtConversationSeq = 42
	memory := fakeMemoryPort{}
	_, err := NewRetrieveEvidenceUseCase(&fakeSearchPort{}, &memory).Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got := memory.query.AtConversationSeq; got != 42 {
		t.Fatalf("expected explicit memory current seq 42, got %d", got)
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

func TestRetrieveEvidenceIncludesExplicitVectorSource(t *testing.T) {
	command := validVectorCommand()
	vector := fakeVectorPort{result: types.VectorResult{
		Items: []types.VectorItemEvidence{{
			VectorItemRef:     "vitem_1",
			SourceRefHash:     "sha256:source-ref",
			SourceService:     "memory-service",
			CollectionType:    types.VectorCollectionMemoryEvent,
			Score:             0.72,
			VisibilityVersion: 11,
			TombstoneStatus:   "NONE",
		}},
	}}
	search := fakeSearchPort{}
	memory := fakeMemoryPort{}
	result, err := NewRetrieveEvidenceUseCase(&search, &memory, WithVectorPort(&vector)).Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if search.called || memory.called || !vector.called {
		t.Fatalf("expected only vector to be called: search=%t memory=%t vector=%t", search.called, memory.called, vector.called)
	}
	if vector.query.QueryEmbeddingRef != "sha256:query-embedding" ||
		vector.query.VisibilityScope != "tenant:tenant-1:user:user-1" ||
		vector.query.PolicyVersion != "policy-v1" ||
		vector.query.TopK != command.EffectiveLimit() ||
		vector.query.RetrievalRequestID != command.PackID() ||
		len(vector.query.CollectionTypes) != 1 ||
		vector.query.CollectionTypes[0] != types.VectorCollectionMemoryEvent {
		t.Fatalf("unexpected vector query: %+v", vector.query)
	}
	if got := len(result.Pack.Items); got != 1 {
		t.Fatalf("expected one vector item, got %d", got)
	}
	item := result.Pack.Items[0]
	if item.SourceType != types.EvidenceSourceVectorItem ||
		item.VectorItemRef != "vitem_1" ||
		item.VectorSourceRefHash != "sha256:source-ref" ||
		item.VectorSourceService != "memory-service" ||
		item.VectorCollectionType != types.VectorCollectionMemoryEvent ||
		item.VectorTombstoneStatus != "NONE" ||
		item.RerankScore <= item.Score ||
		len(item.SourceRefs) != 1 ||
		item.SourceRefs[0].SourceID != "sha256:source-ref" {
		t.Fatalf("unexpected vector evidence item: %+v", item)
	}
	if got := result.Pack.SourceCoverage; len(got) != 4 ||
		got[0].SourceType != types.EvidenceSourceSearchMessage ||
		got[0].Status != types.EvidenceCoverageNotRequested ||
		got[1].SourceType != types.EvidenceSourceVectorItem ||
		!got[1].Requested ||
		got[1].CandidateCount != 1 ||
		got[1].ReturnedCount != 1 ||
		got[1].Status != types.EvidenceCoverageReturned ||
		got[2].SourceType != types.EvidenceSourceMemoryEvent ||
		got[2].Status != types.EvidenceCoverageNotRequested ||
		got[3].SourceType != types.EvidenceSourceProfileAggregate ||
		got[3].Status != types.EvidenceCoverageNotRequested {
		t.Fatalf("unexpected vector source coverage: %+v", got)
	}
}

func TestRetrieveEvidenceFailsClosedWhenVectorPortMissing(t *testing.T) {
	_, err := NewRetrieveEvidenceUseCase(&fakeSearchPort{}, &fakeMemoryPort{}).Execute(context.Background(), validVectorCommand())
	if !errors.Is(err, types.ErrVectorUnavailable) {
		t.Fatalf("expected vector unavailable, got %v", err)
	}
}

func TestRetrieveEvidenceFailsClosedWhenVectorSearchFails(t *testing.T) {
	vector := fakeVectorPort{err: types.ErrVectorUnavailable}
	_, err := NewRetrieveEvidenceUseCase(&fakeSearchPort{}, &fakeMemoryPort{}, WithVectorPort(&vector)).Execute(context.Background(), validVectorCommand())
	if !errors.Is(err, types.ErrVectorUnavailable) {
		t.Fatalf("expected vector unavailable, got %v", err)
	}
}

func TestRetrieveEvidenceRejectsVectorWithoutEmbeddingRef(t *testing.T) {
	command := validVectorCommand()
	command.QueryEmbeddingRef = ""
	_, err := NewRetrieveEvidenceUseCase(&fakeSearchPort{}, &fakeMemoryPort{}, WithVectorPort(&fakeVectorPort{})).Execute(context.Background(), command)
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestRetrieveEvidenceRejectsVectorWithoutCollectionTypes(t *testing.T) {
	command := validVectorCommand()
	command.VectorCollections = nil
	_, err := NewRetrieveEvidenceUseCase(&fakeSearchPort{}, &fakeMemoryPort{}, WithVectorPort(&fakeVectorPort{})).Execute(context.Background(), command)
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestRetrieveEvidenceFailsClosedOnMalformedVectorResult(t *testing.T) {
	vector := fakeVectorPort{result: types.VectorResult{
		Items: []types.VectorItemEvidence{{VectorItemRef: "vitem_1"}},
	}}
	_, err := NewRetrieveEvidenceUseCase(&fakeSearchPort{}, &fakeMemoryPort{}, WithVectorPort(&vector)).Execute(context.Background(), validVectorCommand())
	if !errors.Is(err, types.ErrRetrievalUnavailable) {
		t.Fatalf("expected retrieval unavailable, got %v", err)
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
	if got := result.Pack.SourceCoverage; len(got) != 4 ||
		got[0].SourceType != types.EvidenceSourceSearchMessage ||
		got[0].Requested ||
		got[0].Status != types.EvidenceCoverageNotRequested ||
		got[1].SourceType != types.EvidenceSourceVectorItem ||
		got[1].Requested ||
		got[1].Status != types.EvidenceCoverageNotRequested ||
		got[2].SourceType != types.EvidenceSourceMemoryEvent ||
		!got[2].Requested ||
		got[2].CandidateCount != 2 ||
		got[2].ReturnedCount != 1 ||
		got[2].DedupedCount != 1 ||
		got[2].Status != types.EvidenceCoverageReturned ||
		got[3].SourceType != types.EvidenceSourceProfileAggregate ||
		!got[3].Requested ||
		got[3].Status != types.EvidenceCoverageEmpty {
		t.Fatalf("unexpected source coverage: %+v", got)
	}
}

func TestRetrieveEvidenceCarriesProfileAggregateEvidence(t *testing.T) {
	command := validCommand()
	command.IncludeSearch = false
	command.IncludeMemory = true
	now := time.Unix(20, 0)
	memory := fakeMemoryPort{
		profileResult: types.ProfileAggregateResult{
			Items: []types.ProfileAggregateEvidence{{
				ProfileID:                "profile-1",
				SubjectUserID:            "user-1",
				AggregateType:            "SKILL",
				AggregateKey:             "phoenix-launch",
				Status:                   types.MemoryStatusActive,
				ReviewState:              "APPROVED",
				SummaryText:              "user-1 is reliable for phoenix launch coordination",
				SupportingMemoryEventIDs: []string{"mem-1", "mem-2"},
				Confidence:               0.91,
				ValidFromAt:              now.Add(-time.Hour),
				UpdatedAt:                now,
			}},
		},
	}

	result, err := NewRetrieveEvidenceUseCase(&fakeSearchPort{}, &memory).Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !memory.profileCalled {
		t.Fatal("expected profile aggregate query")
	}
	if memory.profileQuery.SubjectUserID != command.AuthContext.UserID {
		t.Fatalf("expected profile subject to be current user, got %+v", memory.profileQuery)
	}
	if got := len(result.Pack.Items); got != 1 {
		t.Fatalf("expected one profile item, got %d", got)
	}
	item := result.Pack.Items[0]
	if item.SourceType != types.EvidenceSourceProfileAggregate ||
		item.ProfileID != "profile-1" ||
		item.ProfileSubjectUserID != "user-1" ||
		item.ProfileAggregateType != "SKILL" ||
		item.ProfileAggregateKey != "phoenix-launch" ||
		len(item.SupportingMemoryEventIDs) != 2 ||
		item.Text == "" {
		t.Fatalf("profile evidence was not preserved: %+v", item)
	}
	if got := result.Pack.SourceCoverage; len(got) != 4 ||
		got[3].SourceType != types.EvidenceSourceProfileAggregate ||
		got[3].CandidateCount != 1 ||
		got[3].ReturnedCount != 1 ||
		got[3].Status != types.EvidenceCoverageReturned {
		t.Fatalf("unexpected profile source coverage: %+v", got)
	}
}

func TestRetrieveEvidenceFailsClosedWhenProfileLookupFails(t *testing.T) {
	command := validCommand()
	command.IncludeSearch = false
	command.IncludeMemory = true
	memory := fakeMemoryPort{profileErr: types.ErrMemoryUnavailable}
	_, err := NewRetrieveEvidenceUseCase(&fakeSearchPort{}, &memory).Execute(context.Background(), command)
	if !errors.Is(err, types.ErrMemoryUnavailable) {
		t.Fatalf("expected memory unavailable from profile lookup, got %v", err)
	}
}

func TestRetrieveEvidenceFailsClosedWhenMemoryGraphLookupFails(t *testing.T) {
	command := validCommand()
	command.IncludeSearch = false
	command.IncludeMemory = true
	memory := fakeMemoryPort{
		result: types.MemoryResult{
			Items: []types.MemoryEventEvidence{{MemoryEventID: "mem-1", ConversationID: "conv-1", FactText: "memory", Confidence: 0.7}},
		},
		getErr: types.ErrMemoryUnavailable,
	}
	_, err := NewRetrieveEvidenceUseCase(&fakeSearchPort{}, &memory).Execute(context.Background(), command)
	if !errors.Is(err, types.ErrMemoryUnavailable) {
		t.Fatalf("expected memory unavailable from graph lookup, got %v", err)
	}
}

func TestRetrieveEvidenceFailsClosedWhenMemoryGraphExpansionLookupFails(t *testing.T) {
	command := validCommand()
	command.IncludeSearch = false
	command.IncludeMemory = true
	memory := fakeMemoryPort{
		result: types.MemoryResult{
			Items: []types.MemoryEventEvidence{{MemoryEventID: "mem-1", ConversationID: "conv-1", FactText: "memory", Status: types.MemoryStatusActive, Confidence: 0.7}},
		},
		details: map[string]types.MemoryEventLookupResult{
			"mem-1": {
				Item: types.MemoryEventEvidence{MemoryEventID: "mem-1", ConversationID: "conv-1", FactText: "memory", Status: types.MemoryStatusActive, Confidence: 0.7},
				GraphEdges: []types.MemoryGraphEdge{{
					EdgeID:            "edge-1",
					FromMemoryEventID: "mem-1",
					ToMemoryEventID:   "mem-2",
					RelationType:      "SUPPORTS",
				}},
			},
		},
		getErrors: map[string]error{"mem-2": types.ErrMemoryUnavailable},
	}
	_, err := NewRetrieveEvidenceUseCase(&fakeSearchPort{}, &memory).Execute(context.Background(), command)
	if !errors.Is(err, types.ErrMemoryUnavailable) {
		t.Fatalf("expected memory unavailable from adjacent graph lookup, got %v", err)
	}
}

func TestRetrieveEvidenceFiltersGraphExpansionByMemoryStatus(t *testing.T) {
	command := validCommand()
	command.IncludeSearch = false
	command.IncludeMemory = true
	memory := fakeMemoryPort{
		result: types.MemoryResult{
			Items: []types.MemoryEventEvidence{{MemoryEventID: "mem-current", ConversationID: "conv-1", FactText: "current", Status: types.MemoryStatusActive, Confidence: 0.8}},
		},
		details: map[string]types.MemoryEventLookupResult{
			"mem-current": {
				Item: types.MemoryEventEvidence{MemoryEventID: "mem-current", ConversationID: "conv-1", FactText: "current", Status: types.MemoryStatusActive, Confidence: 0.8},
				GraphEdges: []types.MemoryGraphEdge{{
					EdgeID:            "edge-1",
					FromMemoryEventID: "mem-current",
					ToMemoryEventID:   "mem-old",
					RelationType:      "SUPERSEDES",
				}},
			},
			"mem-old": {
				Item: types.MemoryEventEvidence{MemoryEventID: "mem-old", ConversationID: "conv-1", FactText: "old", Status: types.MemoryStatusSuperseded, Confidence: 0.9},
			},
		},
	}
	result, err := NewRetrieveEvidenceUseCase(&fakeSearchPort{}, &memory).Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got := len(result.Pack.Items); got != 1 {
		t.Fatalf("expected superseded adjacent memory to be filtered from default active retrieval, got %d items: %+v", got, result.Pack.Items)
	}
	if result.Pack.Items[0].MemoryEventID != "mem-current" {
		t.Fatalf("unexpected retained memory item: %+v", result.Pack.Items[0])
	}
	if coverage := result.Pack.SourceCoverage[2]; coverage.CandidateCount != 2 || coverage.ReturnedCount != 1 {
		t.Fatalf("expected filtered adjacent memory to be counted but not returned, got %+v", coverage)
	}
}

func TestRetrieveEvidenceFailsClosedWhenGraphEdgeDoesNotReferenceSource(t *testing.T) {
	command := validCommand()
	command.IncludeSearch = false
	command.IncludeMemory = true
	memory := fakeMemoryPort{
		result: types.MemoryResult{
			Items: []types.MemoryEventEvidence{{MemoryEventID: "mem-1", ConversationID: "conv-1", FactText: "memory", Status: types.MemoryStatusActive, Confidence: 0.7}},
		},
		details: map[string]types.MemoryEventLookupResult{
			"mem-1": {
				Item: types.MemoryEventEvidence{MemoryEventID: "mem-1", ConversationID: "conv-1", FactText: "memory", Status: types.MemoryStatusActive, Confidence: 0.7},
				GraphEdges: []types.MemoryGraphEdge{{
					EdgeID:            "edge-bad",
					FromMemoryEventID: "mem-x",
					ToMemoryEventID:   "mem-y",
					RelationType:      "SUPPORTS",
				}},
			},
		},
	}
	_, err := NewRetrieveEvidenceUseCase(&fakeSearchPort{}, &memory).Execute(context.Background(), command)
	if !errors.Is(err, types.ErrRetrievalUnavailable) {
		t.Fatalf("expected retrieval unavailable from malformed graph edge, got %v", err)
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
	if coverage := result.Pack.SourceCoverage[2]; coverage.CandidateCount != 2 || coverage.ReturnedCount != 1 || coverage.Status != types.EvidenceCoverageReturned {
		t.Fatalf("unexpected memory coverage: %+v", coverage)
	}
}

func TestRetrieveEvidenceRerankRewardsSourceChainBeforeTruncating(t *testing.T) {
	command := validCommand()
	command.Limit = 1
	search := fakeSearchPort{result: types.SearchResult{
		Items: []types.SearchMessageEvidence{{
			ConversationID:  "conv-1",
			MessageID:       "msg-search",
			ConversationSeq: 10,
			SourceEventID:   "evt-search",
			SenderID:        "user-a",
			Snippet:         "single search hit",
		}},
	}}
	memory := fakeMemoryPort{
		result: types.MemoryResult{Items: []types.MemoryEventEvidence{{
			MemoryEventID:   "mem-chain",
			ConversationID:  "conv-1",
			FactText:        "multi-source memory chain",
			ActorUserIDs:    []string{"user-a", "user-b"},
			AudienceUserIDs: []string{"user-1"},
			SourceRefs: []types.EvidenceSourceRef{
				{SourceType: "MESSAGE", SourceID: "msg-a", SourceEventID: "evt-a", ConversationID: "conv-1", ConversationSeq: 11},
				{SourceType: "MESSAGE", SourceID: "msg-b", SourceEventID: "evt-b", ConversationID: "conv-cross", ConversationSeq: 12},
			},
			Confidence: 0.74,
		}}},
	}
	result, err := NewRetrieveEvidenceUseCase(&search, &memory).Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got := len(result.Pack.Items); got != 1 {
		t.Fatalf("expected one item after truncation, got %d", got)
	}
	item := result.Pack.Items[0]
	if item.SourceType != types.EvidenceSourceMemoryEvent || item.MemoryEventID != "mem-chain" {
		t.Fatalf("expected multi-source memory chain to outrank single search hit, got %+v", item)
	}
	if item.RerankScore <= 1 {
		t.Fatalf("expected source-chain rerank score above single search baseline, got %+v", item)
	}
	if coverage := result.Pack.SourceCoverage; coverage[0].Status != types.EvidenceCoverageFiltered ||
		coverage[2].Status != types.EvidenceCoverageReturned {
		t.Fatalf("unexpected coverage after source-chain rerank truncation: %+v", coverage)
	}
}

func TestRankFusionRewardsMultiLaneEvidence(t *testing.T) {
	items := []types.EvidenceItem{
		{
			SourceType:     types.EvidenceSourceSearchMessage,
			SourceID:       "msg-search",
			ConversationID: "conv-1",
			Score:          1,
		},
		{
			SourceType: types.EvidenceSourceVectorItem,
			SourceID:   "vitem-1",
			Score:      0.88,
		},
		{
			SourceType:      types.EvidenceSourceMemoryEvent,
			SourceID:        "mem-chain",
			ConversationID:  "conv-1",
			Score:           0.74,
			ActorUserIDs:    []string{"user-a", "user-b"},
			AudienceUserIDs: []string{"user-1"},
			SourceRefs: []types.EvidenceSourceRef{
				{SourceType: "MESSAGE", SourceID: "msg-a", ConversationID: "conv-1", ConversationSeq: 11},
				{SourceType: "MESSAGE", SourceID: "msg-b", ConversationID: "conv-cross", ConversationSeq: 12},
			},
			MemoryGraphEdges: []types.MemoryGraphEdge{{
				EdgeID:       "edge-1",
				RelationType: "SUPPORTS",
				SourceRefs: []types.EvidenceSourceRef{
					{SourceType: "MESSAGE", SourceID: "msg-a", ConversationID: "conv-1", ConversationSeq: 11},
					{SourceType: "MESSAGE", SourceID: "msg-b", ConversationID: "conv-cross", ConversationSeq: 12},
				},
			}},
		},
		{
			SourceType:               types.EvidenceSourceProfileAggregate,
			SourceID:                 "profile-1",
			Score:                    0.82,
			ProfileSubjectUserID:     "user-1",
			SupportingMemoryEventIDs: []string{"mem-a", "mem-b"},
		},
	}

	scores := rankFusionScores(items)
	searchScore := scores[evidenceCandidateKey(items[0])]
	vectorScore := scores[evidenceCandidateKey(items[1])]
	memoryScore := scores[evidenceCandidateKey(items[2])]
	profileScore := scores[evidenceCandidateKey(items[3])]
	if searchScore <= 0 {
		t.Fatalf("expected lexical search lane score, got %f", searchScore)
	}
	if vectorScore <= 0 {
		t.Fatalf("expected vector lane score, got %f", vectorScore)
	}
	if memoryScore <= searchScore {
		t.Fatalf("expected memory evidence with source-chain, graph, and actor lanes to outrank single-lane search fusion: search=%f memory=%f", searchScore, memoryScore)
	}
	if profileScore <= searchScore {
		t.Fatalf("expected profile evidence with support lanes to outrank single-lane search fusion: search=%f profile=%f", searchScore, profileScore)
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

func validVectorCommand() types.RetrieveEvidenceCommand {
	command := validCommand()
	command.IncludeSearch = false
	command.IncludeMemory = false
	command.IncludeVector = true
	command.QueryEmbeddingRef = "sha256:query-embedding"
	command.VectorCollections = []string{types.VectorCollectionMemoryEvent}
	command.VectorVisibilityScope = "tenant:tenant-1:user:user-1"
	command.VectorPolicyVersion = "policy-v1"
	command.VectorMinScore = 0.2
	return command
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
	result        types.MemoryResult
	err           error
	getErr        error
	getErrors     map[string]error
	profileResult types.ProfileAggregateResult
	profileErr    error
	called        bool
	profileCalled bool
	query         types.MemoryQuery
	profileQuery  types.ProfileAggregateQuery
	details       map[string]types.MemoryEventLookupResult
}

type fakeVectorPort struct {
	result types.VectorResult
	err    error
	called bool
	query  types.VectorQuery
}

func (port *fakeVectorPort) SearchVectors(_ context.Context, query types.VectorQuery) (types.VectorResult, error) {
	port.called = true
	port.query = query
	return port.result, port.err
}

func (port *fakeMemoryPort) QueryMemoryEvents(_ context.Context, query types.MemoryQuery) (types.MemoryResult, error) {
	port.called = true
	port.query = query
	return port.result, port.err
}

func (port *fakeMemoryPort) GetMemoryEvent(_ context.Context, lookup types.MemoryEventLookup) (types.MemoryEventLookupResult, error) {
	if port.getErrors != nil {
		if err, ok := port.getErrors[lookup.MemoryEventID]; ok {
			return types.MemoryEventLookupResult{}, err
		}
	}
	if port.getErr != nil {
		return types.MemoryEventLookupResult{}, port.getErr
	}
	if port.details != nil {
		if result, ok := port.details[lookup.MemoryEventID]; ok {
			return result, nil
		}
	}
	for _, item := range port.result.Items {
		if item.MemoryEventID == lookup.MemoryEventID {
			return types.MemoryEventLookupResult{Item: item, GraphEdges: item.GraphEdges}, nil
		}
	}
	return types.MemoryEventLookupResult{Item: types.MemoryEventEvidence{MemoryEventID: lookup.MemoryEventID}}, nil
}

func (port *fakeMemoryPort) ListProfileAggregates(_ context.Context, query types.ProfileAggregateQuery) (types.ProfileAggregateResult, error) {
	port.profileCalled = true
	port.profileQuery = query
	return port.profileResult, port.profileErr
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
