package app

import (
	"context"
	"fmt"

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
	items := make([]types.EvidenceItem, 0, limit)
	sourceCounts := map[string]int{}
	seen := map[string]struct{}{}
	var searchProjectionVersion int64
	var memoryProjectionVersion int64

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
		for _, hit := range result.Items {
			item := searchHitToEvidence(hit)
			if appendEvidence(&items, seen, item, limit) {
				sourceCounts[item.SourceType]++
			}
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
			Statuses:          command.EffectiveMemoryStatuses(),
			Limit:             limit,
		})
		if err != nil {
			return types.RetrieveEvidenceResult{}, err
		}
		memoryProjectionVersion = result.ProjectionVersion
		for _, event := range result.Items {
			item := memoryEventToEvidence(event)
			if appendEvidence(&items, seen, item, limit) {
				sourceCounts[item.SourceType]++
			}
		}
	}

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

func appendEvidence(items *[]types.EvidenceItem, seen map[string]struct{}, item types.EvidenceItem, limit int) bool {
	if len(*items) >= limit {
		return false
	}
	key := item.SourceType + ":" + item.SourceID
	if _, ok := seen[key]; ok {
		return false
	}
	seen[key] = struct{}{}
	*items = append(*items, item)
	return true
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

func evidenceDebugString(items []types.EvidenceItem) string {
	return fmt.Sprintf("%d evidence items", len(items))
}
