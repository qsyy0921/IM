package app

import (
	"context"

	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
)

type NotifyDeliveryUseCase struct {
	registry SessionRegistry
}

func NewNotifyDeliveryUseCase(registry SessionRegistry) *NotifyDeliveryUseCase {
	return &NotifyDeliveryUseCase{registry: registry}
}

func (usecase *NotifyDeliveryUseCase) Execute(
	ctx context.Context,
	command types.NotifyDeliveryCommand,
) (types.NotifyDeliveryResult, error) {
	notification := command.Notification
	if err := notification.Validate(); err != nil {
		return types.NotifyDeliveryResult{}, err
	}
	if notification.Kind == types.DeliveryNotificationKindConversationSignal {
		return usecase.registry.EnqueueConversationSignal(ctx, notification)
	}
	return usecase.registry.EnqueueNotification(ctx, notification)
}
