package delivery

import (
	"testing"

	deliveryeventsv1 "github.com/qsyy0921/IM/schemas/kafka/delivery/v1"
	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
	"google.golang.org/protobuf/proto"
)

func TestBuildCommandFromInboxItemCreated(t *testing.T) {
	value, err := proto.Marshal(&deliveryeventsv1.DeliveryEvent{
		EventId:          "delivery-event-1",
		EventType:        types.DeliveryEventInboxItemCreated,
		TenantId:         "tenant-1",
		AggregateId:      "conversation-1",
		AggregateVersion: 12,
		Payload: &deliveryeventsv1.DeliveryEvent_InboxItemCreated{
			InboxItemCreated: &deliveryeventsv1.DeliveryInboxItemCreatedV1{
				TenantId:        "tenant-1",
				UserId:          "receiver-1",
				ConversationId:  "conversation-1",
				ConversationSeq: 12,
				SourceEventId:   "timeline-event-1",
				MessageId:       "message-1",
				SenderId:        "sender-1",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	command, err := buildCommand("receipt-test", types.DeliveryMessage{
		Topic:     "im.delivery.events",
		Partition: 3,
		Offset:    9,
		Value:     value,
	})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if command.UserID != "receiver-1" ||
		command.SenderID != "sender-1" ||
		command.MessageID != "message-1" ||
		command.OffsetValue != 10 {
		t.Fatalf("unexpected command: %+v", command)
	}
}

func TestBuildCommandFromAckRecorded(t *testing.T) {
	value, err := proto.Marshal(&deliveryeventsv1.DeliveryEvent{
		EventId:     "delivery-ack-1",
		EventType:   types.DeliveryEventAckRecorded,
		TenantId:    "tenant-1",
		AggregateId: "conversation-1",
		Payload: &deliveryeventsv1.DeliveryEvent_AckRecorded{
			AckRecorded: &deliveryeventsv1.DeliveryAckRecordedV1{
				TenantId:        "tenant-1",
				UserId:          "receiver-1",
				DeviceId:        "device-1",
				ConversationId:  "conversation-1",
				LastReceivedSeq: 12,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	command, err := buildCommand("receipt-test", types.DeliveryMessage{Topic: "im.delivery.events", Value: value})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if command.DeviceID != "device-1" || command.LastReceivedSeq != 12 {
		t.Fatalf("unexpected command: %+v", command)
	}
}
