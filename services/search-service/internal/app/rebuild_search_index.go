package app

import (
	"context"

	"github.com/qsyy0921/IM/services/search-service/internal/types"
)

type RebuildSearchIndexUseCase struct {
	source SearchIndexDocumentSource
	writer SearchIndexWriter
}

func NewRebuildSearchIndexUseCase(source SearchIndexDocumentSource, writer SearchIndexWriter) *RebuildSearchIndexUseCase {
	return &RebuildSearchIndexUseCase{source: source, writer: writer}
}

func (useCase *RebuildSearchIndexUseCase) Execute(ctx context.Context, command types.RebuildSearchIndexCommand) (types.RebuildSearchIndexResult, error) {
	if err := command.Validate(); err != nil {
		return types.RebuildSearchIndexResult{}, err
	}
	if useCase == nil || useCase.source == nil {
		return types.RebuildSearchIndexResult{}, types.NewDBReadFailed("search index source is not configured")
	}
	if command.Execute && useCase.writer == nil {
		return types.RebuildSearchIndexResult{}, types.NewSearchUnavailable("search index writer is not configured")
	}

	result := types.RebuildSearchIndexResult{
		TenantID:       command.TenantID,
		ConversationID: command.ConversationID,
		DryRun:         !command.Execute,
	}
	if command.Execute {
		if err := useCase.writer.EnsureSearchIndex(ctx); err != nil {
			return types.RebuildSearchIndexResult{}, err
		}
	}

	batchSize := command.EffectiveBatchSize()
	cursor := types.SearchIndexCursor{}
	for {
		limit := batchSize
		if command.MaxDocuments > 0 {
			remaining := command.MaxDocuments - result.Scanned
			if remaining <= 0 {
				break
			}
			if remaining < limit {
				limit = remaining
			}
		}
		documents, err := useCase.source.ListSearchIndexDocuments(ctx, command, cursor, limit)
		if err != nil {
			return types.RebuildSearchIndexResult{}, err
		}
		if len(documents) == 0 {
			break
		}
		result.Scanned += len(documents)
		result.Batches++
		last := documents[len(documents)-1]
		cursor = types.SearchIndexCursor{
			ConversationID: last.ConversationID,
			MessageID:      last.MessageID,
		}
		if command.Execute {
			if err := useCase.writer.IndexSearchDocuments(ctx, documents); err != nil {
				return types.RebuildSearchIndexResult{}, err
			}
			result.Indexed += len(documents)
		}
		if len(documents) < limit {
			break
		}
	}
	if command.Execute {
		if err := useCase.writer.RefreshSearchIndex(ctx); err != nil {
			return types.RebuildSearchIndexResult{}, err
		}
	}
	return result, nil
}
