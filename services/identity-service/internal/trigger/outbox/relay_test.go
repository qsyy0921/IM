package outbox

import (
	"testing"
	"time"

	identityeventsv1 "github.com/qsyy0921/IM/schemas/kafka/identity/v1"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestBuildIdentityEventDeviceRevoked(t *testing.T) {
	event, err := BuildIdentityEvent(baseMessage(types.IdentityEventDeviceRevoked, []byte(`{
        "tenant_id":"tenant-1",
        "user_id":"user-1",
        "device_id":"device-1",
        "status":"REVOKED",
        "revoked_by":"admin-1",
        "reason":"lost",
        "revoked_at":"2026-06-12T01:02:03Z"
    }`)))
	if err != nil {
		t.Fatalf("build event: %v", err)
	}
	payload, ok := event.Payload.(*identityeventsv1.IdentityEvent_DeviceRevoked)
	if !ok {
		t.Fatalf("expected device revoked payload, got %T", event.Payload)
	}
	if payload.DeviceRevoked.DeviceId != "device-1" || payload.DeviceRevoked.RevokedBy != "admin-1" {
		t.Fatalf("unexpected payload: %+v", payload.DeviceRevoked)
	}
}

func TestBuildIdentityEventSessionRevoked(t *testing.T) {
	event, err := BuildIdentityEvent(baseMessage(types.IdentityEventSessionRevoked, []byte(`{
        "tenant_id":"tenant-1",
        "user_id":"user-1",
        "device_id":"device-1",
        "session_id":"session-1",
        "status":"REVOKED",
        "revoked_by":"admin-1",
        "reason":"manual",
        "revoked_at":"2026-06-12T01:02:03Z"
    }`)))
	if err != nil {
		t.Fatalf("build event: %v", err)
	}
	payload, ok := event.Payload.(*identityeventsv1.IdentityEvent_SessionRevoked)
	if !ok {
		t.Fatalf("expected session revoked payload, got %T", event.Payload)
	}
	if payload.SessionRevoked.SessionId != "session-1" || payload.SessionRevoked.DeviceId != "device-1" {
		t.Fatalf("unexpected payload: %+v", payload.SessionRevoked)
	}
}

func TestBuildIdentityEventRejectsUnsupportedType(t *testing.T) {
	_, err := BuildIdentityEvent(baseMessage("identity.unknown.v1", []byte(`{}`)))
	if err == nil {
		t.Fatal("expected unsupported event error")
	}
}

func baseMessage(eventType string, payload []byte) types.OutboxMessage {
	return types.OutboxMessage{
		EventID:          "event-1",
		EventType:        eventType,
		EventVersion:     "v1",
		TenantID:         "tenant-1",
		AggregateType:    "identity_device",
		AggregateID:      "user-1:device-1",
		AggregateVersion: 1,
		PartitionKey:     "tenant-1:user-1:device-1",
		MappingVersion:   1,
		Producer:         "identity-service",
		PayloadJSON:      payload,
		OccurredAt:       time.Date(2026, 6, 12, 1, 2, 3, 0, time.UTC),
	}
}
