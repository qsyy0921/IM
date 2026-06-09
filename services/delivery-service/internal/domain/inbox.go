package domain

import "github.com/qsyy0921/IM/services/delivery-service/internal/types"

func BuildPullResult(items []types.InboxItem, requestedLimit int) types.PullInboxResult {
	resultItems := items
	hasMore := false
	if requestedLimit > 0 && len(items) > requestedLimit {
		hasMore = true
		resultItems = items[:requestedLimit]
	}
	var nextSeq int64
	if len(resultItems) > 0 {
		nextSeq = resultItems[len(resultItems)-1].ConversationSeq
	}
	return types.PullInboxResult{
		Items:   resultItems,
		NextSeq: nextSeq,
		HasMore: hasMore,
	}
}

func MergeDeliveryCursor(current int64, received int64) (int64, error) {
	if received <= 0 {
		return current, types.NewInvalidArgument("received_seq must be positive")
	}
	if received <= current {
		return current, nil
	}
	return received, nil
}
