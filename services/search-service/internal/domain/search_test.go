package domain

import (
	"testing"

	"github.com/qsyy0921/IM/services/search-service/internal/types"
)

func TestBuildSearchMessagesResultTrimsExtraItem(t *testing.T) {
	result := BuildSearchMessagesResult([]types.SearchMessageHit{
		{MessageID: "msg-1"},
		{MessageID: "msg-2"},
		{MessageID: "msg-3"},
	}, 2, 7)
	if !result.HasMore {
		t.Fatalf("expected has_more")
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.NextCursor != "msg-2" {
		t.Fatalf("unexpected next cursor %q", result.NextCursor)
	}
	if result.ProjectionVersion != 7 {
		t.Fatalf("unexpected projection version %d", result.ProjectionVersion)
	}
}
