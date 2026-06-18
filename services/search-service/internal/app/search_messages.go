package app

import (
	"context"

	"github.com/qsyy0921/IM/services/search-service/internal/domain"
	"github.com/qsyy0921/IM/services/search-service/internal/types"
)

type SearchMessagesUseCase struct {
	repository SearchMessagesRepository
}

func NewSearchMessagesUseCase(repository SearchMessagesRepository) *SearchMessagesUseCase {
	return &SearchMessagesUseCase{repository: repository}
}

func (useCase *SearchMessagesUseCase) Execute(
	ctx context.Context,
	command types.SearchMessagesCommand,
) (types.SearchMessagesResult, error) {
	if err := command.Validate(); err != nil {
		return types.SearchMessagesResult{}, err
	}
	limit := command.EffectiveLimit()
	items, projectionVersion, err := useCase.repository.SearchMessages(ctx, command, limit+1)
	if err != nil {
		return types.SearchMessagesResult{}, err
	}
	return domain.BuildSearchMessagesResult(items, limit, projectionVersion), nil
}
