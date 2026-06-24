package app

import (
	"context"

	"github.com/qsyy0921/IM/services/memory-service/internal/types"
)

type MemoryQueryRepository interface {
	QueryMemoryEvents(context.Context, types.QueryMemoryEventsCommand, int) ([]types.StructuredMemoryEvent, int64, error)
	GetMemoryEvent(context.Context, types.GetMemoryEventCommand) (types.StructuredMemoryEvent, []types.MemoryGraphEdge, error)
	ListProfileAggregates(context.Context, types.ListProfileAggregatesCommand, int) ([]types.ProfileAggregate, error)
	RecomputeProfileAggregate(context.Context, types.RecomputeProfileAggregateCommand) (types.ProfileAggregate, int, bool, error)
	SubmitMemoryCandidate(context.Context, types.SubmitMemoryCandidateCommand) (types.StructuredMemoryEvent, error)
	ReviewMemoryCandidate(context.Context, types.ReviewMemoryCandidateCommand) (types.StructuredMemoryEvent, error)
}

type TimelineProjectionRepository interface {
	ProjectTimelineEvent(context.Context, types.ProjectTimelineEventCommand) (types.ProjectTimelineEventResult, error)
}
