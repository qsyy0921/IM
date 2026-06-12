package app

import (
	"context"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/domain"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type RevokeDeviceUseCase struct {
	repository Repository
	now        func() time.Time
}

func NewRevokeDeviceUseCase(repository Repository) *RevokeDeviceUseCase {
	return &RevokeDeviceUseCase{repository: repository, now: func() time.Time { return time.Now().UTC() }}
}

func (uc *RevokeDeviceUseCase) Execute(ctx context.Context, command types.RevokeDeviceCommand) (types.RevokeDeviceResult, error) {
	if err := command.AdminContext.Validate(); err != nil {
		return types.RevokeDeviceResult{}, err
	}
	if err := domain.ValidateRevokeTarget(command.UserID, command.DeviceID); err != nil {
		return types.RevokeDeviceResult{}, err
	}
	if uc.repository == nil {
		return types.RevokeDeviceResult{}, types.NewDBWriteFailed("identity repository is not configured")
	}
	return uc.repository.RevokeDevice(ctx, command, uc.now())
}
