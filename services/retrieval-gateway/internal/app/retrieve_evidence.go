package app

import (
	"context"
	"fmt"
	"sort"

	"github.com/qsyy0921/IM/services/retrieval-gateway/internal/types"
)

type RetrieveEvidenceUseCase struct {
	search SearchPort
	memory MemoryPort
	policy PolicyPort
}

type RetrieveEvidenceOption func(*RetrieveEvidenceUseCase)

func WithPolicyPort(policy PolicyPort) RetrieveEvidenceOption {
	return func(usecase *RetrieveEvidenceUseCase) {
		usecase.policy = policy
	}
}

func NewRetrieveEvidenceUseCase(search SearchPort, memory MemoryPort, options ...RetrieveEvidenceOption) RetrieveEvidenceUseCase {
	usecase := RetrieveEvidenceUseCase{search: search, memory: memory}
	for _, option := range options {
		if option != nil {
			option(&usecase)
		}
	}
	return usecase
}

func (usecase RetrieveEvidenceUseCase) Execute(
	ctx context.Context,
	command types.RetrieveEvidenceCommand,
) (types.RetrieveEvidenceResult, error) {
	if err := command.Validate(); err != nil {
		return types.RetrieveEvidenceResult{}, err
	}
	if err := usecase.checkPolicy(ctx, command); err != nil {
		return types.RetrieveEvidenceResult{}, err
	}

	limit := command.EffectiveLimit()
	candidates := make([]types.EvidenceItem, 0, limit*2)
	coverage := newSourceCoverage(command)
	seen := map[string]int{}
	var searchProjectionVersion int64
	var memoryProjectionVersion int64
	var searchMaxConversationSeq int64

	if command.ShouldIncludeSearch() {
		if usecase.search == nil {
			return types.RetrieveEvidenceResult{}, types.ErrSearchUnavailable
		}
		result, err := usecase.search.SearchMessages(ctx, types.SearchQuery{
			AuthContext:    command.AuthContext,
			Query:          command.NormalizedQuery(),
			ConversationID: command.ConversationID,
			AfterSeq:       command.AfterSeq,
			Limit:          limit,
		})
		if err != nil {
			return types.RetrieveEvidenceResult{}, err
		}
		searchProjectionVersion = result.ProjectionVersion
		coverage[types.EvidenceSourceSearchMessage].CandidateCount = len(result.Items)
		for _, hit := range result.Items {
			if hit.ConversationSeq > searchMaxConversationSeq {
				searchMaxConversationSeq = hit.ConversationSeq
			}
			item := searchHitToEvidence(hit)
			appendEvidenceCandidate(&candidates, seen, coverage, item)
		}
	}

	if command.ShouldIncludeMemory() {
		if usecase.memory == nil {
			return types.RetrieveEvidenceResult{}, types.ErrMemoryUnavailable
		}
		result, err := usecase.memory.QueryMemoryEvents(ctx, types.MemoryQuery{
			AuthContext:       command.AuthContext,
			Query:             command.NormalizedQuery(),
			ConversationID:    command.ConversationID,
			AfterValidFromSeq: command.AfterSeq,
			AtConversationSeq: effectiveMemoryAtConversationSeq(command, searchMaxConversationSeq),
			Statuses:          command.EffectiveMemoryStatuses(),
			Limit:             limit,
		})
		if err != nil {
			return types.RetrieveEvidenceResult{}, err
		}
		memoryProjectionVersion = result.ProjectionVersion
		coverage[types.EvidenceSourceMemoryEvent].CandidateCount = len(result.Items)
		for _, event := range result.Items {
			item := memoryEventToEvidence(event)
			appendEvidenceCandidate(&candidates, seen, coverage, item)
		}
	}
	items := rerankEvidence(candidates, limit)
	sourceCounts := countEvidenceSources(items)
	sourceCoverage := orderedSourceCoverage(coverage, sourceCounts)

	return types.RetrieveEvidenceResult{
		Pack: types.EvidencePack{
			PackID:                  command.PackID(),
			TenantID:                command.AuthContext.TenantID,
			Query:                   command.NormalizedQuery(),
			ConversationID:          command.ConversationID,
			Items:                   items,
			SourceCounts:            orderedSourceCounts(sourceCounts),
			SearchProjectionVersion: searchProjectionVersion,
			MemoryProjectionVersion: memoryProjectionVersion,
			RetrievalVersion:        types.RetrievalVersion,
			SourceCoverage:          sourceCoverage,
		},
	}, nil
}

func (usecase RetrieveEvidenceUseCase) checkPolicy(ctx context.Context, command types.RetrieveEvidenceCommand) error {
	if usecase.policy == nil {
		return nil
	}
	decision, err := usecase.policy.CheckRetrieveEvidence(ctx, types.RetrievalPolicyCheck{
		AuthContext:    command.AuthContext,
		ConversationID: command.ConversationID,
	})
	if err != nil {
		return err
	}
	if !decision.Allowed || decision.RequiresApproval {
		return types.ErrPermissionDenied
	}
	return nil
}

type sourceCoverageState struct {
	Requested      bool
	CandidateCount int
	DedupedCount   int
}

func newSourceCoverage(command types.RetrieveEvidenceCommand) map[string]*sourceCoverageState {
	return map[string]*sourceCoverageState{
		types.EvidenceSourceSearchMessage: &sourceCoverageState{Requested: command.ShouldIncludeSearch()},
		types.EvidenceSourceMemoryEvent:   &sourceCoverageState{Requested: command.ShouldIncludeMemory()},
	}
}

func effectiveMemoryAtConversationSeq(command types.RetrieveEvidenceCommand, searchMaxConversationSeq int64) int64 {
	if command.AtConversationSeq > 0 {
		return command.AtConversationSeq
	}
	return searchMaxConversationSeq
}

func appendEvidenceCandidate(
	items *[]types.EvidenceItem,
	seen map[string]int,
	coverage map[string]*sourceCoverageState,
	item types.EvidenceItem,
) {
	key := item.SourceType + ":" + item.SourceID
	if index, ok := seen[key]; ok {
		if state := coverage[item.SourceType]; state != nil {
			state.DedupedCount++
		}
		(*items)[index].DedupeReason = types.EvidenceDedupeKeptFirstDuplicateSource
		return
	}
	item.RerankScore = evidenceRerankScore(item)
	item.DedupeReason = types.EvidenceDedupeUniqueSource
	seen[key] = len(*items)
	*items = append(*items, item)
}

func rerankEvidence(items []types.EvidenceItem, limit int) []types.EvidenceItem {
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].RerankScore > items[j].RerankScore
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func evidenceRerankScore(item types.EvidenceItem) float64 {
	switch item.SourceType {
	case types.EvidenceSourceSearchMessage:
		return clampScore(item.Score)
	case types.EvidenceSourceMemoryEvent:
		return clampScore(item.Score)
	default:
		return 0
	}
}

func clampScore(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func countEvidenceSources(items []types.EvidenceItem) map[string]int {
	counts := map[string]int{}
	for _, item := range items {
		counts[item.SourceType]++
	}
	return counts
}

func searchHitToEvidence(hit types.SearchMessageEvidence) types.EvidenceItem {
	sourceID := hit.MessageID
	if sourceID == "" {
		sourceID = hit.SourceEventID
	}
	return types.EvidenceItem{
		EvidenceID:        "search:" + sourceID,
		SourceType:        types.EvidenceSourceSearchMessage,
		SourceID:          sourceID,
		ConversationID:    hit.ConversationID,
		ConversationSeq:   hit.ConversationSeq,
		Text:              hit.Snippet,
		Score:             1,
		SpeakerUserID:     hit.SenderID,
		MessageID:         hit.MessageID,
		OccurredAt:        hit.OccurredAt,
		VisibilityVersion: hit.VisibilityVersion,
		SourceRefs: []types.EvidenceSourceRef{{
			SourceType:      "MESSAGE",
			SourceID:        hit.MessageID,
			SourceEventID:   hit.SourceEventID,
			ConversationID:  hit.ConversationID,
			ConversationSeq: hit.ConversationSeq,
			OccurredAt:      hit.OccurredAt,
		}},
	}
}

func memoryEventToEvidence(event types.MemoryEventEvidence) types.EvidenceItem {
	seq := event.ValidFromSeq
	if seq == 0 && len(event.SourceRefs) > 0 {
		seq = event.SourceRefs[0].ConversationSeq
	}
	return types.EvidenceItem{
		EvidenceID:        "memory:" + event.MemoryEventID,
		SourceType:        types.EvidenceSourceMemoryEvent,
		SourceID:          event.MemoryEventID,
		ConversationID:    event.ConversationID,
		ConversationSeq:   seq,
		Text:              event.FactText,
		Score:             event.Confidence,
		MemoryEventID:     event.MemoryEventID,
		OccurredAt:        event.ValidFromAt,
		ValidFromSeq:      event.ValidFromSeq,
		ValidToSeq:        event.ValidToSeq,
		VisibilityVersion: event.VisibilityVersion,
		SourceRefs:        event.SourceRefs,
		ActorUserIDs:      event.ActorUserIDs,
		AudienceUserIDs:   event.AudienceUserIDs,
		TemporalStatus:    event.Status,
		ReviewState:       event.ReviewState,
		ExtractionVersion: event.ExtractionVersion,
	}
}

func orderedSourceCounts(counts map[string]int) []types.EvidenceSourceCount {
	out := make([]types.EvidenceSourceCount, 0, 2)
	for _, sourceType := range []string{types.EvidenceSourceSearchMessage, types.EvidenceSourceMemoryEvent} {
		if count := counts[sourceType]; count > 0 {
			out = append(out, types.EvidenceSourceCount{SourceType: sourceType, Count: count})
		}
	}
	return out
}

func orderedSourceCoverage(
	coverage map[string]*sourceCoverageState,
	sourceCounts map[string]int,
) []types.EvidenceSourceCoverage {
	out := make([]types.EvidenceSourceCoverage, 0, 2)
	for _, sourceType := range []string{types.EvidenceSourceSearchMessage, types.EvidenceSourceMemoryEvent} {
		state := coverage[sourceType]
		if state == nil {
			continue
		}
		returnedCount := sourceCounts[sourceType]
		out = append(out, types.EvidenceSourceCoverage{
			SourceType:     sourceType,
			Requested:      state.Requested,
			CandidateCount: state.CandidateCount,
			ReturnedCount:  returnedCount,
			DedupedCount:   state.DedupedCount,
			Status:         sourceCoverageStatus(state, returnedCount),
		})
	}
	return out
}

func sourceCoverageStatus(state *sourceCoverageState, returnedCount int) string {
	switch {
	case state == nil || !state.Requested:
		return types.EvidenceCoverageNotRequested
	case returnedCount > 0:
		return types.EvidenceCoverageReturned
	case state.CandidateCount == 0:
		return types.EvidenceCoverageEmpty
	default:
		return types.EvidenceCoverageFiltered
	}
}

func evidenceDebugString(items []types.EvidenceItem) string {
	return fmt.Sprintf("%d evidence items", len(items))
}
