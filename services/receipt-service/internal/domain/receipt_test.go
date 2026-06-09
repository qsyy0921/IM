package domain

import (
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

func TestMergeReadCursorAdvancesWithinVisibleAndReceivedRange(t *testing.T) {
	next, err := MergeReadCursor(10, 12, 20, 15)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if next != 12 {
		t.Fatalf("expected next=12, got %d", next)
	}
}

func TestMergeReadCursorTreatsLowerSeqAsIdempotent(t *testing.T) {
	next, err := MergeReadCursor(10, 9, 9, 9)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if next != 10 {
		t.Fatalf("expected next=10, got %d", next)
	}
}

func TestMergeReadCursorRejectsOutOfRange(t *testing.T) {
	_, err := MergeReadCursor(10, 12, 11, 20)
	if !errors.Is(err, types.ErrReadOutOfVisibleRange) {
		t.Fatalf("expected visible range error, got %v", err)
	}
	_, err = MergeReadCursor(10, 12, 20, 11)
	if !errors.Is(err, types.ErrReadOutOfReceivedRange) {
		t.Fatalf("expected received range error, got %v", err)
	}
}
