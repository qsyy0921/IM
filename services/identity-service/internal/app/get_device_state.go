package app

import (
	"context"

	"github.com/qsyy0921/IM/services/identity-service/internal/domain"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type GetDeviceStateUseCase struct {
	repository Repository
}

func NewGetDeviceStateUseCase(repository Repository) *GetDeviceStateUseCase {
	return &GetDeviceStateUseCase{repository: repository}
}

func (uc *GetDeviceStateUseCase) Execute(ctx context.Context, command types.GetDeviceStateCommand) (types.GetDeviceStateResult, error) {
	if err := command.AdminContext.Validate(); err != nil {
		return types.GetDeviceStateResult{}, err
	}
	if err := domain.ValidateRevokeTarget(command.UserID, command.DeviceID); err != nil {
		return types.GetDeviceStateResult{}, err
	}
	if uc.repository == nil {
		return types.GetDeviceStateResult{}, types.NewDBReadFailed("identity repository is not configured")
	}
	return uc.repository.GetDeviceState(ctx, command)
}
