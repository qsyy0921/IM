package domain

import "github.com/qsyy0921/IM/services/receipt-service/internal/types"

func MergeReadCursor(current int64, requested int64, maxVisible int64, maxReceived int64) (int64, error) {
	if requested <= 0 {
		return current, types.NewInvalidArgument("read_seq must be positive")
	}
	if requested <= current {
		return current, nil
	}
	if maxVisible <= 0 || requested > maxVisible {
		return current, types.NewReadOutOfVisibleRange("read_seq exceeds visible range")
	}
	if maxReceived <= 0 || requested > maxReceived {
		return current, types.NewReadOutOfReceivedRange("read_seq exceeds received range")
	}
	return requested, nil
}
