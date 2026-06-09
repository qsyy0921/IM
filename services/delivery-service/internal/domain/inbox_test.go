package domain

import (
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

func TestBuildPullResult(t *testing.T) {
	items := []types.InboxItem{
		{ConversationSeq: 11},
		{ConversationSeq: 12},
		{ConversationSeq: 13},
	}
	result := BuildPullResult(items, 2)
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if !result.HasMore {
		t.Fatal("expected has_more")
	}
	if result.NextSeq != 12 {
		t.Fatalf("expected next seq 12, got %d", result.NextSeq)
	}
}

func TestMergeDeliveryCursorTreatsLowerAckAsIdempotent(t *testing.T) {
	next, err := MergeDeliveryCursor(10, 8)
	if err != nil {
		t.Fatalf("merge cursor: %v", err)
	}
	if next != 10 {
		t.Fatalf("expected cursor to stay 10, got %d", next)
	}
}

func TestMergeDeliveryCursorRejectsNonPositiveAck(t *testing.T) {
	_, err := MergeDeliveryCursor(10, 0)
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}
