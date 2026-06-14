package timeline

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	conversationtimelinev1 "github.com/qsyy0921/IM/schemas/kafka"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

type fakeConsumer struct {
	message   types.TimelineMessage
	fetchErr  error
	committed bool
}

func (consumer *fakeConsumer) Fetch(context.Context) (types.TimelineMessage, error) {
	return consumer.message, consumer.fetchErr
}

func (consumer *fakeConsumer) Commit(context.Context, types.TimelineMessage) error {
	consumer.committed = true
	return context.Canceled
}

type fakeProjector struct {
	command types.ProjectTimelineEventCommand
	err     error
}

type scriptedConsumer struct {
	message     types.TimelineMessage
	fetchErrs   []error
	commitErrs  []error
	fetchCalls  int
	commitCalls int
	committed   bool
}

type fakeFailureRecorder struct {
	recorded []types.ProjectionFailureRecord
	err      error
}

func (projector *fakeProjector) Execute(
	_ context.Context,
	command types.ProjectTimelineEventCommand,
) (types.ProjectTimelineEventResult, error) {
	projector.command = command
	return types.ProjectTimelineEventResult{ProjectedInboxCount: 1}, projector.err
}

func (recorder *fakeFailureRecorder) RecordFailure(_ context.Context, record types.ProjectionFailureRecord) error {
	recorder.recorded = append(recorder.recorded, record)
	return recorder.err
}

func (consumer *scriptedConsumer) Fetch(context.Context) (types.TimelineMessage, error) {
	var err error
	if consumer.fetchCalls < len(consumer.fetchErrs) {
		err = consumer.fetchErrs[consumer.fetchCalls]
	}
	consumer.fetchCalls++
	if err != nil {
		return types.TimelineMessage{}, err
	}
	return consumer.message, nil
}

func (consumer *scriptedConsumer) Commit(context.Context, types.TimelineMessage) error {
	consumer.committed = true
	var err error
	if consumer.commitCalls < len(consumer.commitErrs) {
		err = consumer.commitErrs[consumer.commitCalls]
	}
	consumer.commitCalls++
	if err != nil {
		return err
	}
	if len(consumer.commitErrs) > 0 {
		return nil
	}
	return context.Canceled
}

func TestWorkerProjectsAndCommitsMessagePersisted(t *testing.T) {
	payload, err := structpb.NewStruct(map[string]any{"text": "hello"})
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	value := mustMarshalTimelineEvent(t, &conversationtimelinev1.ConversationTimelineEvent{
		EventId:          "event-1",
		EventType:        types.TimelineEventMessagePersisted,
		TenantId:         "tenant-1",
		AggregateId:      "conv-1",
		AggregateVersion: 10,
		CorrelationId:    "request-1",
		Metadata: &conversationtimelinev1.TimelineMetadata{
			FanoutMode:        "WRITE_FANOUT",
			PermissionVersion: 7,
		},
		Payload: &conversationtimelinev1.ConversationTimelineEvent_MessagePersisted{
			MessagePersisted: &conversationtimelinev1.MessagePersistedV1{
				MessageId: "msg-1",
				SenderId:  "sender-1",
				Payload:   payload,
			},
		},
	})
	consumer := &fakeConsumer{message: types.TimelineMessage{Topic: "conversation.timeline.events", Partition: 3, Offset: 41, Value: value}}
	projector := &fakeProjector{}
	err = NewWorker(consumer, projector, "delivery-test", nil).Run(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected stop after fake commit, got %v", err)
	}
	if !consumer.committed {
		t.Fatal("expected message commit")
	}
	if projector.command.EventID != "event-1" ||
		projector.command.MessageID != "msg-1" ||
		projector.command.OffsetValue != 42 ||
		projector.command.ConsumerGroup != "delivery-test" {
		t.Fatalf("unexpected command: %+v", projector.command)
	}
}

func TestWorkerProjectsAndCommitsMessageRevoked(t *testing.T) {
	value := mustMarshalTimelineEvent(t, &conversationtimelinev1.ConversationTimelineEvent{
		EventId:          "event-revoke-1",
		EventType:        types.TimelineEventMessageRevoked,
		TenantId:         "tenant-1",
		AggregateId:      "conv-1",
		AggregateVersion: 11,
		CorrelationId:    "request-1",
		Metadata: &conversationtimelinev1.TimelineMetadata{
			FanoutMode:        "WRITE_FANOUT",
			PermissionVersion: 7,
		},
		Payload: &conversationtimelinev1.ConversationTimelineEvent_MessageRevoked{
			MessageRevoked: &conversationtimelinev1.MessageRevokedV1{
				MessageId:       "msg-1",
				ConversationSeq: 11,
				ChangeVersion:   1,
				RevokedBy:       "sender-1",
			},
		},
	})
	consumer := &fakeConsumer{message: types.TimelineMessage{Topic: "conversation.timeline.events", Partition: 3, Offset: 42, Value: value}}
	projector := &fakeProjector{}
	err := NewWorker(consumer, projector, "delivery-test", nil).Run(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected stop after fake commit, got %v", err)
	}
	if !consumer.committed {
		t.Fatal("expected message commit")
	}
	if projector.command.EventID != "event-revoke-1" ||
		projector.command.EventType != types.TimelineEventMessageRevoked ||
		projector.command.MessageID != "msg-1" ||
		projector.command.SenderID != "sender-1" ||
		projector.command.OffsetValue != 43 {
		t.Fatalf("unexpected command: %+v", projector.command)
	}
}

func TestWorkerProjectsAndCommitsMessageEdited(t *testing.T) {
	beforePayload, err := structpb.NewStruct(map[string]any{"text": "hello"})
	if err != nil {
		t.Fatalf("before payload: %v", err)
	}
	afterPayload, err := structpb.NewStruct(map[string]any{"text": "hello edited"})
	if err != nil {
		t.Fatalf("after payload: %v", err)
	}
	value := mustMarshalTimelineEvent(t, &conversationtimelinev1.ConversationTimelineEvent{
		EventId:          "event-edit-1",
		EventType:        types.TimelineEventMessageEdited,
		TenantId:         "tenant-1",
		AggregateId:      "conv-1",
		AggregateVersion: 11,
		CorrelationId:    "request-1",
		Metadata: &conversationtimelinev1.TimelineMetadata{
			FanoutMode:        "WRITE_FANOUT",
			PermissionVersion: 7,
		},
		Payload: &conversationtimelinev1.ConversationTimelineEvent_MessageEdited{
			MessageEdited: &conversationtimelinev1.MessageEditedV1{
				MessageId:       "msg-1",
				ConversationSeq: 11,
				ChangeVersion:   1,
				EditedBy:        "sender-1",
				BeforePayload:   beforePayload,
				AfterPayload:    afterPayload,
			},
		},
	})
	consumer := &fakeConsumer{message: types.TimelineMessage{Topic: "conversation.timeline.events", Partition: 3, Offset: 42, Value: value}}
	projector := &fakeProjector{}
	err = NewWorker(consumer, projector, "delivery-test", nil).Run(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected stop after fake commit, got %v", err)
	}
	if !consumer.committed {
		t.Fatal("expected message commit")
	}
	if projector.command.EventID != "event-edit-1" ||
		projector.command.EventType != types.TimelineEventMessageEdited ||
		projector.command.MessageID != "msg-1" ||
		projector.command.SenderID != "sender-1" ||
		projector.command.OffsetValue != 43 {
		t.Fatalf("unexpected command: %+v", projector.command)
	}
}

func TestWorkerProjectsAndCommitsMessageDeleted(t *testing.T) {
	value := mustMarshalTimelineEvent(t, &conversationtimelinev1.ConversationTimelineEvent{
		EventId:          "event-delete-1",
		EventType:        types.TimelineEventMessageDeleted,
		TenantId:         "tenant-1",
		AggregateId:      "conv-1",
		AggregateVersion: 11,
		CorrelationId:    "request-1",
		Metadata: &conversationtimelinev1.TimelineMetadata{
			FanoutMode:        "WRITE_FANOUT",
			PermissionVersion: 7,
		},
		Payload: &conversationtimelinev1.ConversationTimelineEvent_MessageDeleted{
			MessageDeleted: &conversationtimelinev1.MessageDeletedV1{
				MessageId:       "msg-1",
				ConversationSeq: 11,
				ChangeVersion:   1,
				DeletedBy:       "sender-1",
				DeleteScope:     conversationtimelinev1.MessageDeleteScope_MESSAGE_DELETE_SCOPE_CONVERSATION_VIEW,
			},
		},
	})
	consumer := &fakeConsumer{message: types.TimelineMessage{Topic: "conversation.timeline.events", Partition: 3, Offset: 42, Value: value}}
	projector := &fakeProjector{}
	err := NewWorker(consumer, projector, "delivery-test", nil).Run(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected stop after fake commit, got %v", err)
	}
	if !consumer.committed {
		t.Fatal("expected message commit")
	}
	if projector.command.EventID != "event-delete-1" ||
		projector.command.EventType != types.TimelineEventMessageDeleted ||
		projector.command.MessageID != "msg-1" ||
		projector.command.SenderID != "sender-1" ||
		projector.command.OffsetValue != 43 {
		t.Fatalf("unexpected command: %+v", projector.command)
	}
}

func TestWorkerProjectsAndCommitsOwnerTransferred(t *testing.T) {
	value := mustMarshalTimelineEvent(t, &conversationtimelinev1.ConversationTimelineEvent{
		EventId:          "owner-transfer-1",
		EventType:        types.TimelineEventConversationMemberOwnerTransferred,
		TenantId:         "tenant-1",
		AggregateId:      "conv-1",
		AggregateVersion: 12,
		CorrelationId:    "request-1",
		Metadata: &conversationtimelinev1.TimelineMetadata{
			FanoutMode:        "WRITE_FANOUT",
			PermissionVersion: 8,
		},
		Payload: &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberOwnerTransferred{
			ConversationMemberOwnerTransferred: &conversationtimelinev1.ConversationMemberOwnerTransferredV1{
				PreviousOwnerUserId:  "owner-1",
				NewOwnerUserId:       "user-2",
				PreviousOwnerNewRole: conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_ADMIN,
				PreviousOwnerStatus:  conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE,
				NewOwnerNewRole:      conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_OWNER,
				NewOwnerStatus:       conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE,
				MemberVersion:        9,
				PermissionVersion:    10,
			},
		},
	})
	consumer := &fakeConsumer{message: types.TimelineMessage{Topic: "conversation.timeline.events", Partition: 3, Offset: 42, Value: value}}
	projector := &fakeProjector{}
	err := NewWorker(consumer, projector, "delivery-test", nil).Run(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected stop after fake commit, got %v", err)
	}
	if !consumer.committed {
		t.Fatal("expected message commit")
	}
	if projector.command.EventID != "owner-transfer-1" ||
		projector.command.EventType != types.TimelineEventConversationMemberOwnerTransferred ||
		projector.command.PreviousOwnerUserID != "owner-1" ||
		projector.command.PreviousOwnerNewRole != "ADMIN" ||
		projector.command.PreviousOwnerStatus != types.DeliveryMemberStatusActive ||
		projector.command.NewOwnerUserID != "user-2" ||
		projector.command.NewOwnerNewRole != "OWNER" ||
		projector.command.NewOwnerStatus != types.DeliveryMemberStatusActive ||
		projector.command.MemberVersion != 9 ||
		projector.command.PermissionVersion != 10 ||
		projector.command.OffsetValue != 43 {
		t.Fatalf("unexpected command: %+v", projector.command)
	}
}

func TestWorkerDoesNotCommitWhenProjectionFails(t *testing.T) {
	value := mustMarshalTimelineEvent(t, &conversationtimelinev1.ConversationTimelineEvent{
		EventId:          "event-1",
		EventType:        types.TimelineEventConversationMemberJoined,
		TenantId:         "tenant-1",
		AggregateId:      "conv-1",
		AggregateVersion: 10,
		Payload: &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberJoined{
			ConversationMemberJoined: &conversationtimelinev1.ConversationMemberJoinedV1{
				TargetUserId:      "user-1",
				NewRole:           conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_MEMBER,
				NewStatus:         conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE,
				MemberVersion:     2,
				PermissionVersion: 3,
			},
		},
	})
	consumer := &fakeConsumer{message: types.TimelineMessage{Topic: "conversation.timeline.events", Value: value}}
	projector := &fakeProjector{err: types.NewDBWriteFailed("boom")}
	recorder := &fakeFailureRecorder{}
	err := NewWorker(consumer, projector, "delivery-test", recorder).Run(context.Background())
	if !errors.Is(err, types.ErrDBWriteFailed) {
		t.Fatalf("expected projection error, got %v", err)
	}
	if consumer.committed {
		t.Fatal("did not expect commit")
	}
	if len(recorder.recorded) != 1 {
		t.Fatalf("expected one recorded failure, got %d", len(recorder.recorded))
	}
	if recorder.recorded[0].FailureClass != types.ProjectionFailureClassDBWrite ||
		recorder.recorded[0].EventID != "event-1" ||
		recorder.recorded[0].OffsetValue != 0 ||
		recorder.recorded[0].LastError != "delivery projection write failed" {
		t.Fatalf("unexpected recorded failure: %+v", recorder.recorded[0])
	}
}

func TestBestEffortProjectionFailureRecordUsesStableLastError(t *testing.T) {
	value := mustMarshalTimelineEvent(t, &conversationtimelinev1.ConversationTimelineEvent{
		EventId:          "event-sensitive",
		EventType:        types.TimelineEventConversationMemberJoined,
		TenantId:         "tenant-1",
		AggregateId:      "conv-1",
		AggregateVersion: 10,
		Payload: &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberJoined{
			ConversationMemberJoined: &conversationtimelinev1.ConversationMemberJoinedV1{
				TargetUserId:      "user-1",
				NewRole:           conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_MEMBER,
				NewStatus:         conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE,
				MemberVersion:     2,
				PermissionVersion: 3,
			},
		},
	})
	record := bestEffortProjectionFailureRecord(
		"delivery-test",
		types.TimelineMessage{Topic: "conversation.timeline.events", Partition: 1, Offset: 42, Value: value},
		types.NewDBWriteFailed("pq duplicate key user=user1@example.com token=secret-token internal-table"),
	)
	if record.FailureClass != types.ProjectionFailureClassDBWrite || record.LastError != "delivery projection write failed" {
		t.Fatalf("unexpected projection failure record: %+v", record)
	}
	for _, forbidden := range []string{"user1@example.com", "secret-token", "internal-table", "duplicate key"} {
		if strings.Contains(record.LastError, forbidden) {
			t.Fatalf("projection failure record leaked %q: %q", forbidden, record.LastError)
		}
	}
}

func TestWorkerRejectsMalformedEvent(t *testing.T) {
	consumer := &fakeConsumer{message: types.TimelineMessage{Value: []byte("bad")}}
	projector := &fakeProjector{}
	recorder := &fakeFailureRecorder{}
	err := NewWorker(consumer, projector, "delivery-test", recorder).Run(context.Background())
	if err == nil {
		t.Fatal("expected malformed event error")
	}
	if consumer.committed {
		t.Fatal("did not expect commit")
	}
	if len(recorder.recorded) != 1 {
		t.Fatalf("expected one recorded failure, got %d", len(recorder.recorded))
	}
	if recorder.recorded[0].FailureClass != types.ProjectionFailureClassDecode ||
		recorder.recorded[0].EventID != "" ||
		recorder.recorded[0].LastError != "delivery projection decode failed" {
		t.Fatalf("unexpected malformed recorded failure: %+v", recorder.recorded[0])
	}
}

func mustMarshalTimelineEvent(t *testing.T, event *conversationtimelinev1.ConversationTimelineEvent) []byte {
	t.Helper()
	value, err := proto.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return value
}

func TestWorkerRetriesTransientFetchErrorAndExposesSnapshot(t *testing.T) {
	consumer := &scriptedConsumer{
		message:    types.TimelineMessage{Topic: "conversation.timeline.events", Partition: 1, Offset: 10, Value: mustMarshalTimelineEvent(t, &conversationtimelinev1.ConversationTimelineEvent{EventId: "event-1", EventType: types.TimelineEventConversationMemberBoundaryCancelled, TenantId: "tenant-1", AggregateId: "conv-1", AggregateVersion: 10, Payload: &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberBoundaryCancelled{ConversationMemberBoundaryCancelled: &conversationtimelinev1.ConversationMemberBoundaryCancelledV1{TargetUserId: "user-1", NewRole: conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_MEMBER, NewStatus: conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE, MemberVersion: 2, PermissionVersion: 3}}})},
		fetchErrs:  []error{fmt.Errorf("temporary fetch failure"), nil, context.Canceled},
		commitErrs: []error{nil},
	}
	projector := &fakeProjector{}
	worker := NewWorker(consumer, projector, "delivery-test", nil, Config{ErrorBackoff: time.Millisecond})

	err := worker.Run(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected worker stop after successful retry, got %v", err)
	}
	if consumer.fetchCalls != 3 {
		t.Fatalf("expected 3 fetch calls, got %d", consumer.fetchCalls)
	}
	if !consumer.committed {
		t.Fatal("expected commit after retry")
	}
	snapshot := worker.Snapshot()
	if snapshot.TotalErrors != 1 || snapshot.ConsecutiveErrors != 0 {
		t.Fatalf("unexpected retry snapshot: %+v", snapshot)
	}
	if snapshot.LastErrorAtMS == 0 || snapshot.LastSuccessAtMS == 0 || snapshot.LastCommitAtMS == 0 {
		t.Fatalf("expected error/success timestamps, got %+v", snapshot)
	}
	if snapshot.LastErrorBackoffMS != time.Millisecond.Milliseconds() {
		t.Fatalf("expected backoff %d, got %+v", time.Millisecond.Milliseconds(), snapshot)
	}
}

func TestWorkerRetriesTransientCommitError(t *testing.T) {
	consumer := &scriptedConsumer{
		message:    types.TimelineMessage{Topic: "conversation.timeline.events", Partition: 1, Offset: 10, Value: mustMarshalTimelineEvent(t, &conversationtimelinev1.ConversationTimelineEvent{EventId: "event-1", EventType: types.TimelineEventConversationMemberBoundaryCancelled, TenantId: "tenant-1", AggregateId: "conv-1", AggregateVersion: 10, Payload: &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberBoundaryCancelled{ConversationMemberBoundaryCancelled: &conversationtimelinev1.ConversationMemberBoundaryCancelledV1{TargetUserId: "user-1", NewRole: conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_MEMBER, NewStatus: conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE, MemberVersion: 2, PermissionVersion: 3}}})},
		fetchErrs:  []error{nil, nil, context.Canceled},
		commitErrs: []error{fmt.Errorf("temporary commit failure"), nil},
	}
	projector := &fakeProjector{}
	worker := NewWorker(consumer, projector, "delivery-test", nil, Config{ErrorBackoff: time.Millisecond})

	err := worker.Run(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected worker stop after successful retry, got %v", err)
	}
	if consumer.commitCalls != 2 {
		t.Fatalf("expected 2 commit calls, got %d", consumer.commitCalls)
	}
	if consumer.fetchCalls != 3 {
		t.Fatalf("expected retry plus terminal cancel fetch, got %d fetch calls", consumer.fetchCalls)
	}
	if projector.command.EventID != "event-1" {
		t.Fatalf("expected successful projection after retry, got %+v", projector.command)
	}
}
