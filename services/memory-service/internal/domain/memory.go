package domain

import (
	"fmt"

	"github.com/qsyy0921/IM/services/memory-service/internal/types"
)

func BuildQueryMemoryEventsResult(items []types.StructuredMemoryEvent, projectionVersion int64, limit int) types.QueryMemoryEventsResult {
	result := types.QueryMemoryEventsResult{
		Items:             items,
		ProjectionVersion: projectionVersion,
	}
	if limit > 0 && len(items) == limit {
		last := items[len(items)-1]
		if last.ValidFromSeq > 0 {
			result.NextCursor = fmt.Sprintf("%d", last.ValidFromSeq)
		} else {
			result.NextCursor = last.MemoryEventID
		}
	}
	return result
}

func BuildListProfileAggregatesResult(items []types.ProfileAggregate, limit int) types.ListProfileAggregatesResult {
	result := types.ListProfileAggregatesResult{Items: items}
	if limit > 0 && len(items) == limit {
		result.NextCursor = items[len(items)-1].ProfileID
	}
	return result
}
