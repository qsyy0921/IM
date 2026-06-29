package app

import (
	"context"

	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
)

type NotifyConversationSignalUseCase struct {
	registry SessionRegistry
}

func NewNotifyConversationSignalUseCase(registry SessionRegistry) *NotifyConversationSignalUseCase {
	return &NotifyConversationSignalUseCase{registry: registry}
}

func (usecase *NotifyConversationSignalUseCase) Execute(
	ctx context.Context,
	command types.NotifyDeliveryCommand,
) (types.NotifyDeliveryResult, error) {
	notification := command.Notification
	notification.Kind = types.DeliveryNotificationKindConversationSignal
	if err := notification.Validate(); err != nil {
		return types.NotifyDeliveryResult{}, err
	}
	return usecase.registry.EnqueueConversationSignal(ctx, notification)
}
