package app

import (
	"context"

	"github.com/qsyy0921/IM/services/search-service/internal/types"
)

type SearchMessagesRepository interface {
	SearchMessages(ctx context.Context, command types.SearchMessagesCommand, fetchLimit int) ([]types.SearchMessageHit, int64, error)
}

type TimelineProjectionRepository interface {
	ProjectTimelineEvent(ctx context.Context, command types.ProjectTimelineEventCommand) (types.ProjectTimelineEventResult, error)
}
