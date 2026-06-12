package app

import (
	"context"

	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
)

type SessionRegistry interface {
	Register(context.Context, types.SessionRegistration) (types.SessionRegistrationResult, error)
	Unregister(sessionID string)
	EnqueueNotification(context.Context, types.DeliveryNotification) (types.NotifyDeliveryResult, error)
	EvictDevice(ctx context.Context, tenantID string, userID string, deviceID string, reason string) (types.SessionEvictionResult, error)
	EvictSession(ctx context.Context, tenantID string, userID string, deviceID string, sessionID string, reason string) (types.SessionEvictionResult, error)
}

type DeliveryClient interface {
	AckDelivery(context.Context, types.AckDeliveryCommand) (types.AckDeliveryResult, error)
}
