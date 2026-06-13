package contact

import (
	"testing"

	contacteventsv1 "github.com/qsyy0921/IM/schemas/kafka/contacts/v1"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
	"google.golang.org/protobuf/proto"
)

func TestBuildCommandFromContactAccepted(t *testing.T) {
	command, err := buildCommand("policy-contact-test", types.ContactMessage{
		Topic:     TopicContactEvents,
		Partition: 2,
		Offset:    40,
		Value: mustMarshalContactEvent(t, &contacteventsv1.ContactEvent{
			EventId:          "contact-accepted-1",
			EventType:        types.ContactEventRequestAccepted,
			TenantId:         "tenant-1",
			AggregateVersion: 7,
			Payload: &contacteventsv1.ContactEvent_RequestAccepted{
				RequestAccepted: &contacteventsv1.ContactRequestAcceptedV1{
					TenantId:       "tenant-1",
					RequestId:      "request-1",
					SenderUserId:   "alice",
					ReceiverUserId: "bob",
					Status:         "ACCEPTED",
					EdgeVersion:    5,
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if command.EventID != "contact-accepted-1" ||
		command.EventType != types.ContactEventRequestAccepted ||
		command.SenderUserID != "alice" ||
		command.ReceiverUserID != "bob" ||
		command.EdgeVersion != 5 ||
		command.OffsetValue != 41 {
		t.Fatalf("unexpected command: %+v", command)
	}
}

func TestBuildCommandFromContactBlocked(t *testing.T) {
	command, err := buildCommand("policy-contact-test", types.ContactMessage{
		Topic:  TopicContactEvents,
		Offset: 9,
		Value: mustMarshalContactEvent(t, &contacteventsv1.ContactEvent{
			EventId:   "contact-blocked-1",
			EventType: types.ContactEventEdgeBlocked,
			TenantId:  "tenant-1",
			Payload: &contacteventsv1.ContactEvent_EdgeBlocked{
				EdgeBlocked: &contacteventsv1.ContactEdgeBlockedV1{
					TenantId:      "tenant-1",
					OwnerUserId:   "alice",
					ContactUserId: "bob",
					Status:        types.ContactEdgeStatusBlocked,
					EdgeVersion:   6,
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if command.OwnerUserID != "alice" ||
		command.ContactUserID != "bob" ||
		command.Status != types.ContactEdgeStatusBlocked ||
		command.EdgeVersion != 6 ||
		command.OffsetValue != 10 {
		t.Fatalf("unexpected command: %+v", command)
	}
}

func TestBuildCommandRejectsMalformedContactEvent(t *testing.T) {
	_, err := buildCommand("policy-contact-test", types.ContactMessage{Value: []byte("bad")})
	if err == nil {
		t.Fatal("expected malformed event error")
	}
}

func mustMarshalContactEvent(t *testing.T, event *contacteventsv1.ContactEvent) []byte {
	t.Helper()
	value, err := proto.Marshal(event)
	if err != nil {
		t.Fatalf("marshal contact event: %v", err)
	}
	return value
}
