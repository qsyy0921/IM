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

type SearchIndexDocumentSource interface {
	ListSearchIndexDocuments(
		ctx context.Context,
		command types.RebuildSearchIndexCommand,
		after types.SearchIndexCursor,
		limit int,
	) ([]types.SearchIndexDocument, error)
}

type SearchIndexWriter interface {
	EnsureSearchIndex(ctx context.Context) error
	IndexSearchDocuments(ctx context.Context, documents []types.SearchIndexDocument) error
	RefreshSearchIndex(ctx context.Context) error
}
