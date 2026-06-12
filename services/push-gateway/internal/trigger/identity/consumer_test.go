package identity

import (
	"context"
	"errors"
	"testing"

	identityeventsv1 "github.com/qsyy0921/IM/schemas/kafka/identity/v1"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
	"google.golang.org/protobuf/proto"
)

func TestWorkerRecordsDeviceRevocationAndCommits(t *testing.T) {
	event := &identityeventsv1.IdentityEvent{
		EventId:      "identity-event-1",
		EventType:    EventIdentityDeviceRevoked,
		EventVersion: "1.0.0",
		TenantId:     "tenant-1",
		AggregateId:  "user-1:device-1",
		PartitionKey: "tenant-1:user-1:device-1",
		Payload: &identityeventsv1.IdentityEvent_DeviceRevoked{
			DeviceRevoked: &identityeventsv1.IdentityDeviceRevokedV1{
				TenantId: "tenant-1",
				UserId:   "user-1",
				DeviceId: "device-1",
				Status:   "REVOKED",
			},
		},
	}
	value, _ := proto.Marshal(event)
	consumer := &fakeConsumer{messages: []types.DeliveryEventMessage{{Topic: TopicIdentityEvents, Offset: 11, Value: value}}}
	recorder := &recordingRecorder{}
	worker := NewWorker(consumer, recorder)

	if err := worker.Run(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("run: %v", err)
	}
	if recorder.deviceRevokes != 1 || recorder.tenantID != "tenant-1" || recorder.userID != "user-1" || recorder.deviceID != "device-1" {
		t.Fatalf("unexpected recorder: %+v", recorder)
	}
	if consumer.commits != 1 {
		t.Fatalf("expected commit, got %d", consumer.commits)
	}
}

func TestWorkerRecordsSessionRevocationAndCommits(t *testing.T) {
	event := &identityeventsv1.IdentityEvent{
		EventId:      "identity-event-1",
		EventType:    EventIdentitySessionRevoked,
		EventVersion: "1.0.0",
		TenantId:     "tenant-1",
		AggregateId:  "user-1:device-1:session-1",
		PartitionKey: "tenant-1:user-1:device-1",
		Payload: &identityeventsv1.IdentityEvent_SessionRevoked{
			SessionRevoked: &identityeventsv1.IdentitySessionRevokedV1{
				TenantId:  "tenant-1",
				UserId:    "user-1",
				DeviceId:  "device-1",
				SessionId: "session-1",
				Status:    "REVOKED",
			},
		},
	}
	value, _ := proto.Marshal(event)
	consumer := &fakeConsumer{messages: []types.DeliveryEventMessage{{Value: value}}}
	recorder := &recordingRecorder{}
	worker := NewWorker(consumer, recorder)

	if err := worker.Run(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("run: %v", err)
	}
	if recorder.sessionRevokes != 1 || recorder.sessionID != "session-1" {
		t.Fatalf("unexpected recorder: %+v", recorder)
	}
	if consumer.commits != 1 {
		t.Fatalf("expected commit, got %d", consumer.commits)
	}
}

func TestWorkerFailClosedForUnsupportedIdentityEvent(t *testing.T) {
	value, _ := proto.Marshal(&identityeventsv1.IdentityEvent{
		EventId:      "identity-event-1",
		EventType:    "identity.unknown.v1",
		EventVersion: "1.0.0",
		TenantId:     "tenant-1",
		AggregateId:  "user-1:device-1",
		PartitionKey: "tenant-1:user-1:device-1",
	})
	consumer := &fakeConsumer{messages: []types.DeliveryEventMessage{{Value: value}}}
	worker := NewWorker(consumer, &recordingRecorder{})

	if err := worker.Run(context.Background()); !errors.Is(err, types.ErrUnsupportedIdentityEvent) {
		t.Fatalf("expected unsupported identity event, got %v", err)
	}
	if consumer.commits != 0 {
		t.Fatalf("unsupported event must not be committed")
	}
}

func TestWorkerFailClosedForEventTypeMismatch(t *testing.T) {
	event := &identityeventsv1.IdentityEvent{
		EventId:      "identity-event-1",
		EventType:    EventIdentitySessionRevoked,
		EventVersion: "1.0.0",
		TenantId:     "tenant-1",
		AggregateId:  "user-1:device-1",
		PartitionKey: "tenant-1:user-1:device-1",
		Payload: &identityeventsv1.IdentityEvent_DeviceRevoked{
			DeviceRevoked: &identityeventsv1.IdentityDeviceRevokedV1{
				TenantId: "tenant-1",
				UserId:   "user-1",
				DeviceId: "device-1",
			},
		},
	}
	value, _ := proto.Marshal(event)
	consumer := &fakeConsumer{messages: []types.DeliveryEventMessage{{Value: value}}}
	worker := NewWorker(consumer, &recordingRecorder{})

	if err := worker.Run(context.Background()); err == nil {
		t.Fatalf("expected fail-closed error")
	}
	if consumer.commits != 0 {
		t.Fatalf("mismatched event must not be committed")
	}
}

type fakeConsumer struct {
	messages []types.DeliveryEventMessage
	commits  int
}

func (consumer *fakeConsumer) Fetch(ctx context.Context) (types.DeliveryEventMessage, error) {
	if len(consumer.messages) == 0 {
		return types.DeliveryEventMessage{}, context.Canceled
	}
	message := consumer.messages[0]
	consumer.messages = consumer.messages[1:]
	return message, nil
}

func (consumer *fakeConsumer) Commit(ctx context.Context, message types.DeliveryEventMessage) error {
	consumer.commits++
	return nil
}

type recordingRecorder struct {
	deviceRevokes  int
	sessionRevokes int
	tenantID       string
	userID         string
	deviceID       string
	sessionID      string
}

func (recorder *recordingRecorder) RevokeDevice(ctx context.Context, tenantID string, userID string, deviceID string) error {
	recorder.deviceRevokes++
	recorder.tenantID = tenantID
	recorder.userID = userID
	recorder.deviceID = deviceID
	return nil
}

func (recorder *recordingRecorder) RevokeSession(ctx context.Context, tenantID string, userID string, deviceID string, sessionID string) error {
	recorder.sessionRevokes++
	recorder.tenantID = tenantID
	recorder.userID = userID
	recorder.deviceID = deviceID
	recorder.sessionID = sessionID
	return nil
}
