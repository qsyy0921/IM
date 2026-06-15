package domain

import (
	"testing"

	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
)

func TestDeliveryNotifyBuildsHideFrameForHiddenInboxItem(t *testing.T) {
	frame := DeliveryNotify(types.DeliveryNotification{
		Kind:            types.DeliveryNotificationKindInboxItemHidden,
		EventID:         "delivery-event-hide-1",
		TenantID:        "tenant-1",
		UserID:          "user-1",
		ConversationID:  "conversation-1",
		ConversationSeq: 7,
		SourceEventID:   "hide-request-1",
		SourceEventType: "delivery.inbox_item.hidden.v1",
		MessageID:       "message-1",
		CorrelationID:   "corr-1",
	})

	if frame.Op != types.OpDeliveryHide ||
		frame.EventID != "delivery-event-hide-1" ||
		frame.ConversationSeq != 7 ||
		frame.MessageID != "message-1" ||
		!frame.PullRequired {
		t.Fatalf("unexpected hide frame: %+v", frame)
	}
}
