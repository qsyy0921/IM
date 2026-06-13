package contact

import (
	"context"
	"errors"
	"testing"
	"time"

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

func TestWorkerRunRetriesAfterProjectorError(t *testing.T) {
	t.Parallel()

	consumer := &fakeConsumer{
		fetches: []fakeFetch{
			{message: contactMessageForTest(t, "contact-retry-1")},
			{message: contactMessageForTest(t, "contact-retry-1")},
			{err: context.Canceled},
		},
	}
	projector := &fakeProjector{errs: []error{errors.New("projection failed"), nil}}
	worker := NewWorker(
		consumer,
		projector,
		"policy-contact-test",
		Config{ErrorBackoff: time.Millisecond},
	)

	err := worker.Run(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if consumer.commitCount != 1 {
		t.Fatalf("expected one committed message, got %d", consumer.commitCount)
	}
	if projector.executeCount != 2 {
		t.Fatalf("expected projector retries, got %d", projector.executeCount)
	}
	snapshot := worker.Snapshot()
	if snapshot.TotalErrors != 1 || snapshot.ConsecutiveErrors != 0 {
		t.Fatalf("unexpected snapshot after retry: %+v", snapshot)
	}
	if snapshot.LastErrorAtMS == 0 || snapshot.LastSuccessAtMS == 0 || snapshot.LastCommitAtMS == 0 {
		t.Fatalf("expected worker timestamps, got %+v", snapshot)
	}
	if snapshot.LastErrorBackoffMS != time.Millisecond.Milliseconds() {
		t.Fatalf("unexpected error backoff: %+v", snapshot)
	}
}

func TestWorkerRunFailsFastWhenConsumerMissing(t *testing.T) {
	t.Parallel()

	worker := NewWorker(nil, &fakeProjector{}, "policy-contact-test")
	err := worker.Run(context.Background())
	if err == nil || err.Error() != "policy contact consumer is not configured" {
		t.Fatalf("unexpected error: %v", err)
	}
}

type fakeConsumer struct {
	fetches     []fakeFetch
	fetchIndex  int
	commitCount int
}

type fakeFetch struct {
	message types.ContactMessage
	err     error
}

func (consumer *fakeConsumer) Fetch(context.Context) (types.ContactMessage, error) {
	if consumer.fetchIndex >= len(consumer.fetches) {
		return types.ContactMessage{}, context.Canceled
	}
	result := consumer.fetches[consumer.fetchIndex]
	consumer.fetchIndex++
	return result.message, result.err
}

func (consumer *fakeConsumer) Commit(context.Context, types.ContactMessage) error {
	consumer.commitCount++
	return nil
}

type fakeProjector struct {
	errs         []error
	executeCount int
}

func (projector *fakeProjector) Execute(context.Context, types.ProjectContactEventCommand) (types.ProjectContactEventResult, error) {
	projector.executeCount++
	if projector.executeCount <= len(projector.errs) {
		return types.ProjectContactEventResult{}, projector.errs[projector.executeCount-1]
	}
	return types.ProjectContactEventResult{}, nil
}

func contactMessageForTest(t *testing.T, eventID string) types.ContactMessage {
	t.Helper()
	return types.ContactMessage{
		Topic:     TopicContactEvents,
		Partition: 1,
		Offset:    8,
		Value: mustMarshalContactEvent(t, &contacteventsv1.ContactEvent{
			EventId:   eventID,
			EventType: types.ContactEventRequestCreated,
			TenantId:  "tenant-1",
			Payload: &contacteventsv1.ContactEvent_RequestCreated{
				RequestCreated: &contacteventsv1.ContactRequestCreatedV1{
					TenantId:       "tenant-1",
					RequestId:      "request-1",
					SenderUserId:   "alice",
					ReceiverUserId: "bob",
					Status:         "PENDING",
				},
			},
		}),
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
