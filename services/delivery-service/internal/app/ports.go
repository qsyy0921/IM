package app

import (
	"context"

	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

type InboxRepository interface {
	PullInbox(ctx context.Context, command types.PullInboxCommand, fetchLimit int) ([]types.InboxItem, error)
}

type DeliveryCursorRepository interface {
	AckDelivery(ctx context.Context, command types.AckDeliveryCommand) (types.AckDeliveryResult, error)
}

type TimelineProjectionRepository interface {
	ProjectTimelineEvent(ctx context.Context, command types.ProjectTimelineEventCommand) (types.ProjectTimelineEventResult, error)
}
