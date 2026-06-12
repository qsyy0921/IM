package app

import (
	"context"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/domain"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type RevokeSessionUseCase struct {
	repository Repository
	now        func() time.Time
}

func NewRevokeSessionUseCase(repository Repository) *RevokeSessionUseCase {
	return &RevokeSessionUseCase{repository: repository, now: func() time.Time { return time.Now().UTC() }}
}

func (uc *RevokeSessionUseCase) Execute(ctx context.Context, command types.RevokeSessionCommand) (types.RevokeSessionResult, error) {
	if err := command.AdminContext.Validate(); err != nil {
		return types.RevokeSessionResult{}, err
	}
	if err := domain.ValidateRevokeTarget(command.UserID, command.DeviceID); err != nil {
		return types.RevokeSessionResult{}, err
	}
	if err := domain.ValidateSessionID(command.SessionID); err != nil {
		return types.RevokeSessionResult{}, err
	}
	if uc.repository == nil {
		return types.RevokeSessionResult{}, types.NewDBWriteFailed("identity repository is not configured")
	}
	return uc.repository.RevokeSession(ctx, command, uc.now())
}
