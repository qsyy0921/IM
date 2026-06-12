package identity

import (
	"context"
	"errors"

	identityeventsv1 "github.com/qsyy0921/IM/schemas/kafka/identity/v1"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
	"google.golang.org/protobuf/proto"
)

const (
	TopicIdentityEvents         = "im.identity.events"
	EventIdentityDeviceRevoked  = "identity.device.revoked.v1"
	EventIdentitySessionRevoked = "identity.session.revoked.v1"
)

type Consumer interface {
	Fetch(context.Context) (types.DeliveryEventMessage, error)
	Commit(context.Context, types.DeliveryEventMessage) error
}

type Recorder interface {
	RevokeDevice(ctx context.Context, tenantID string, userID string, deviceID string) error
	RevokeSession(ctx context.Context, tenantID string, userID string, deviceID string, sessionID string) error
}

type Worker struct {
	consumer Consumer
	recorder Recorder
}

func NewWorker(consumer Consumer, recorder Recorder) *Worker {
	return &Worker{consumer: consumer, recorder: recorder}
}

func (worker *Worker) Run(ctx context.Context) error {
	for {
		message, err := worker.consumer.Fetch(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			return err
		}
		if err := worker.apply(ctx, message); err != nil {
			return err
		}
		if err := worker.consumer.Commit(ctx, message); err != nil {
			return err
		}
	}
}

func (worker *Worker) apply(ctx context.Context, message types.DeliveryEventMessage) error {
	var event identityeventsv1.IdentityEvent
	if err := proto.Unmarshal(message.Value, &event); err != nil {
		return err
	}
	if event.GetEventId() == "" ||
		event.GetEventType() == "" ||
		event.GetEventVersion() == "" ||
		event.GetTenantId() == "" ||
		event.GetAggregateId() == "" ||
		event.GetPartitionKey() == "" {
		return types.NewInvalidFrame("identity event envelope is incomplete")
	}
	switch payload := event.GetPayload().(type) {
	case *identityeventsv1.IdentityEvent_DeviceRevoked:
		if event.GetEventType() != EventIdentityDeviceRevoked {
			return types.NewInvalidFrame("identity event type mismatch")
		}
		revoked := payload.DeviceRevoked
		if revoked == nil ||
			revoked.GetTenantId() == "" ||
			revoked.GetUserId() == "" ||
			revoked.GetDeviceId() == "" {
			return types.NewInvalidFrame("identity device revoked payload is incomplete")
		}
		if event.GetTenantId() != revoked.GetTenantId() {
			return types.NewInvalidFrame("identity event envelope mismatch")
		}
		return worker.recorder.RevokeDevice(ctx, revoked.GetTenantId(), revoked.GetUserId(), revoked.GetDeviceId())
	case *identityeventsv1.IdentityEvent_SessionRevoked:
		if event.GetEventType() != EventIdentitySessionRevoked {
			return types.NewInvalidFrame("identity event type mismatch")
		}
		revoked := payload.SessionRevoked
		if revoked == nil ||
			revoked.GetTenantId() == "" ||
			revoked.GetUserId() == "" ||
			revoked.GetDeviceId() == "" ||
			revoked.GetSessionId() == "" {
			return types.NewInvalidFrame("identity session revoked payload is incomplete")
		}
		if event.GetTenantId() != revoked.GetTenantId() {
			return types.NewInvalidFrame("identity event envelope mismatch")
		}
		return worker.recorder.RevokeSession(ctx, revoked.GetTenantId(), revoked.GetUserId(), revoked.GetDeviceId(), revoked.GetSessionId())
	default:
		return types.ErrUnsupportedIdentityEvent
	}
}
