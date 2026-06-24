package app

import (
	"context"

	"github.com/qsyy0921/IM/services/retrieval-gateway/internal/types"
)

type graphExpansionNode struct {
	event types.MemoryEventEvidence
	depth int
}

func (usecase RetrieveEvidenceUseCase) expandMemoryGraph(
	ctx context.Context,
	command types.RetrieveEvidenceCommand,
	source types.MemoryEventEvidence,
	queryMemoryIDs map[string]struct{},
	candidates *[]types.EvidenceItem,
	seen map[string]int,
	coverage map[string]*sourceCoverageState,
) error {
	if usecase.graphExpansionDepth <= 0 || len(source.GraphEdges) == 0 {
		return nil
	}
	allowedStatuses := memoryStatusSet(command.EffectiveMemoryStatuses())
	expanded := map[string]struct{}{
		source.MemoryEventID: {},
	}
	queue := []graphExpansionNode{{event: source, depth: 0}}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		if node.depth >= usecase.graphExpansionDepth {
			continue
		}
		for _, edge := range node.event.GraphEdges {
			adjacentID, err := adjacentMemoryEventID(node.event.MemoryEventID, edge)
			if err != nil {
				return err
			}
			if _, ok := queryMemoryIDs[adjacentID]; ok {
				continue
			}

			key := evidenceCandidateKey(types.EvidenceItem{
				SourceType: types.EvidenceSourceMemoryEvent,
				SourceID:   adjacentID,
			})
			if _, ok := seen[key]; ok {
				if state := coverage[types.EvidenceSourceMemoryEvent]; state != nil {
					state.CandidateCount++
					state.DedupedCount++
				}
				(*candidates)[seen[key]].DedupeReason = types.EvidenceDedupeKeptFirstDuplicateSource
				continue
			}

			result, err := usecase.memory.GetMemoryEvent(ctx, types.MemoryEventLookup{
				AuthContext:   command.AuthContext,
				MemoryEventID: adjacentID,
			})
			if err != nil {
				return err
			}
			if result.Item.MemoryEventID == "" {
				return types.NewRetrievalUnavailable("memory graph adjacent event missing")
			}
			if result.Item.MemoryEventID != adjacentID {
				return types.NewRetrievalUnavailable("memory graph adjacent event mismatch")
			}

			if state := coverage[types.EvidenceSourceMemoryEvent]; state != nil {
				state.CandidateCount++
			}
			if !allowedStatuses[result.Item.Status] {
				continue
			}

			adjacent := result.Item
			adjacent.GraphEdges = result.GraphEdges
			appendEvidenceCandidate(candidates, seen, coverage, memoryEventToEvidence(adjacent))
			if _, ok := expanded[adjacentID]; !ok {
				expanded[adjacentID] = struct{}{}
				queue = append(queue, graphExpansionNode{event: adjacent, depth: node.depth + 1})
			}
		}
	}
	return nil
}

func adjacentMemoryEventID(sourceID string, edge types.MemoryGraphEdge) (string, error) {
	switch {
	case sourceID == "":
		return "", types.NewRetrievalUnavailable("memory graph source id is missing")
	case edge.FromMemoryEventID == sourceID && edge.ToMemoryEventID == sourceID:
		return "", types.NewRetrievalUnavailable("memory graph self edge cannot be expanded")
	case edge.FromMemoryEventID == sourceID && edge.ToMemoryEventID != "":
		return edge.ToMemoryEventID, nil
	case edge.ToMemoryEventID == sourceID && edge.FromMemoryEventID != "":
		return edge.FromMemoryEventID, nil
	case edge.FromMemoryEventID == sourceID || edge.ToMemoryEventID == sourceID:
		return "", types.NewRetrievalUnavailable("memory graph adjacent id is missing")
	default:
		return "", types.NewRetrievalUnavailable("memory graph edge does not reference source memory")
	}
}

func memoryStatusSet(statuses []string) map[string]bool {
	out := make(map[string]bool, len(statuses))
	for _, status := range statuses {
		out[status] = true
	}
	return out
}
