package app

import (
	"context"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type MarkPublishedMemberChangesUseCase struct {
	repository MemberChangeProgressRepository
	limit      int
}

func NewMarkPublishedMemberChangesUseCase(
	repository MemberChangeProgressRepository,
	limit int,
) *MarkPublishedMemberChangesUseCase {
	return &MarkPublishedMemberChangesUseCase{
		repository: repository,
		limit:      types.NormalizeMemberChangeProgressLimit(limit),
	}
}

func (uc *MarkPublishedMemberChangesUseCase) Execute(
	ctx context.Context,
) (types.MemberChangePublishProgressStats, error) {
	if uc.repository == nil {
		return types.MemberChangePublishProgressStats{}, types.NewDBWriteFailed("member change progress repository is not configured")
	}
	return uc.repository.MarkPublishedMemberChanges(ctx, uc.limit)
}
