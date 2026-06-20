package app

import (
	"context"

	"github.com/qsyy0921/IM/services/notification-service/internal/types"
)

type Repository interface {
	CreateNotificationRequest(
		ctx context.Context,
		command types.CreateNotificationRequestCommand,
		requestID string,
		destinationHash string,
		commandHash string,
	) (types.NotificationRequest, error)
	GetNotificationRequest(
		ctx context.Context,
		tenantID types.TenantID,
		requestID string,
	) (types.NotificationRequest, error)
	CancelNotificationRequest(
		ctx context.Context,
		command types.CancelNotificationRequestCommand,
	) (types.NotificationRequest, error)
}

type DestinationHasher interface {
	HashDestination(destinationRef string) (string, error)
}

type RequestIDGenerator interface {
	NewRequestID() (string, error)
}
