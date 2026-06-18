package domain

import "github.com/qsyy0921/IM/services/search-service/internal/types"

func BuildSearchMessagesResult(
	items []types.SearchMessageHit,
	limit int,
	projectionVersion int64,
) types.SearchMessagesResult {
	result := types.SearchMessagesResult{ProjectionVersion: projectionVersion}
	if len(items) == 0 {
		return result
	}
	if limit <= 0 || len(items) <= limit {
		result.Items = items
		result.NextCursor = cursorFromLast(items)
		return result
	}
	result.Items = items[:limit]
	result.NextCursor = cursorFromLast(result.Items)
	result.HasMore = true
	return result
}

func cursorFromLast(items []types.SearchMessageHit) string {
	if len(items) == 0 {
		return ""
	}
	return items[len(items)-1].MessageID
}
