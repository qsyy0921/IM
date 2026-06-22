package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

func TestNewConversationCreateRecordWritesBoundaryChangeID(t *testing.T) {
	record, err := NewConversationCreateRecord(types.CreateConversationCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-create",
			UserID:    "user-a",
			RequestID: "request-create",
			TraceID:   "trace-create",
		},
		ConversationID:   "direct-create",
		ConversationType: types.ConversationTypeDirect,
		DirectPeerUserID: types.UserID("user-b"),
		IdempotencyKey:   "create-direct-idem",
	}, []types.EventID{"event-create-1", "event-create-2"}, 1, time.Date(2026, 6, 23, 1, 2, 3, 0, time.UTC))
	if err != nil {
		t.Fatalf("create record: %v", err)
	}
	if len(record.Outbox) != 2 || len(record.Timeline) != 2 {
		t.Fatalf("expected two member boundary events, got outbox=%d timeline=%d", len(record.Outbox), len(record.Timeline))
	}
	for index, event := range record.Outbox {
		var payload map[string]any
		if err := json.Unmarshal(event.PayloadJSON, &payload); err != nil {
			t.Fatalf("decode payload %d: %v", index, err)
		}
		wantChangeID := string(event.EventID)
		if got := payload["change_id"]; got != wantChangeID {
			t.Fatalf("payload %d change_id=%v want %s", index, got, wantChangeID)
		}
	}
}
