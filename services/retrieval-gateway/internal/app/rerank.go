package app

import (
	"sort"
	"strings"

	"github.com/qsyy0921/IM/services/retrieval-gateway/internal/types"
)

const reciprocalRankFusionK = 60.0

func rerankEvidence(items []types.EvidenceItem, limit int) []types.EvidenceItem {
	fusionScores := rankFusionScores(items)
	for index := range items {
		items[index].RerankScore = evidenceRerankScore(items[index], fusionScores[evidenceCandidateKey(items[index])])
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].RerankScore > items[j].RerankScore
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func evidenceRerankScore(item types.EvidenceItem, rankFusionBonus float64) float64 {
	return clampScore(item.Score) + sourceChainRerankBonus(item) + rankFusionBonus
}

func rankFusionScores(items []types.EvidenceItem) map[string]float64 {
	lanes := buildRankFusionLanes(items)
	scores := make(map[string]float64, len(items))
	for _, lane := range lanes {
		sort.SliceStable(lane, func(i, j int) bool {
			return laneRankingScore(items[lane[i]]) > laneRankingScore(items[lane[j]])
		})
		for rank, index := range lane {
			key := evidenceCandidateKey(items[index])
			scores[key] += reciprocalRankScore(rank + 1)
		}
	}
	return scores
}

func buildRankFusionLanes(items []types.EvidenceItem) [][]int {
	lanesByName := map[string][]int{}
	for index, item := range items {
		for _, lane := range evidenceRankLanes(item) {
			lanesByName[lane] = append(lanesByName[lane], index)
		}
	}
	ordered := make([][]int, 0, len(lanesByName))
	for _, name := range []string{
		"lexical_search",
		"vector_item",
		"memory_event",
		"profile_aggregate",
		"source_chain",
		"memory_graph",
		"actor_attribution",
		"profile_support",
	} {
		if lane := lanesByName[name]; len(lane) > 0 {
			ordered = append(ordered, lane)
		}
	}
	return ordered
}

func evidenceRankLanes(item types.EvidenceItem) []string {
	lanes := make([]string, 0, 4)
	switch item.SourceType {
	case types.EvidenceSourceSearchMessage:
		lanes = append(lanes, "lexical_search")
	case types.EvidenceSourceVectorItem:
		lanes = append(lanes, "vector_item")
	case types.EvidenceSourceMemoryEvent:
		lanes = append(lanes, "memory_event")
		if len(item.SourceRefs) > 1 || hasCrossConversationSourceRef(item.ConversationID, item.SourceRefs) {
			lanes = append(lanes, "source_chain")
		}
		if len(item.MemoryGraphEdges) > 0 {
			lanes = append(lanes, "memory_graph")
		}
		if len(item.ActorUserIDs) > 1 || len(item.AudienceUserIDs) > 0 {
			lanes = append(lanes, "actor_attribution")
		}
	case types.EvidenceSourceProfileAggregate:
		lanes = append(lanes, "profile_aggregate")
		if len(item.SupportingMemoryEventIDs) > 1 {
			lanes = append(lanes, "profile_support")
			lanes = append(lanes, "source_chain")
		}
	}
	return lanes
}

func laneRankingScore(item types.EvidenceItem) float64 {
	return clampScore(item.Score) + sourceChainRerankBonus(item)
}

func reciprocalRankScore(rank int) float64 {
	if rank <= 0 {
		return 0
	}
	return 1 / (float64(rank) + reciprocalRankFusionK)
}

func evidenceCandidateKey(item types.EvidenceItem) string {
	return item.SourceType + ":" + item.SourceID
}

func sourceChainRerankBonus(item types.EvidenceItem) float64 {
	switch item.SourceType {
	case types.EvidenceSourceMemoryEvent:
		return memorySourceChainRerankBonus(item)
	case types.EvidenceSourceProfileAggregate:
		return profileSourceChainRerankBonus(item)
	default:
		return 0
	}
}

func memorySourceChainRerankBonus(item types.EvidenceItem) float64 {
	var bonus float64
	if len(item.SourceRefs) > 0 {
		bonus += 0.02
	}
	if len(item.SourceRefs) > 1 {
		bonus += 0.08
	}
	if hasCrossConversationSourceRef(item.ConversationID, item.SourceRefs) {
		bonus += 0.08
	}
	if len(item.ActorUserIDs) > 1 {
		bonus += 0.06
	}
	if len(item.AudienceUserIDs) > 0 {
		bonus += 0.04
	}
	if len(item.MemoryGraphEdges) > 0 {
		bonus += 0.10
	}
	if memoryGraphSourceRefCount(item.MemoryGraphEdges) > 1 {
		bonus += 0.04
	}
	return bonus
}

func profileSourceChainRerankBonus(item types.EvidenceItem) float64 {
	var bonus float64
	if len(item.SupportingMemoryEventIDs) > 1 {
		bonus += 0.08
	}
	if item.ProfileSubjectUserID != "" {
		bonus += 0.02
	}
	return bonus
}

func hasCrossConversationSourceRef(conversationID types.ConversationID, refs []types.EvidenceSourceRef) bool {
	for _, ref := range refs {
		if ref.ConversationID != "" && !strings.EqualFold(string(ref.ConversationID), string(conversationID)) {
			return true
		}
	}
	return false
}

func memoryGraphSourceRefCount(edges []types.MemoryGraphEdge) int {
	count := 0
	for _, edge := range edges {
		count += len(edge.SourceRefs)
	}
	return count
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
