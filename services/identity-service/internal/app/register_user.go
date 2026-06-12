package app

import (
	"context"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/domain"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type RegisterUserUseCase struct {
	repository Repository
	passwords  PasswordHasher
	now        func() time.Time
}

func NewRegisterUserUseCase(repository Repository, passwords PasswordHasher) *RegisterUserUseCase {
	return &RegisterUserUseCase{
		repository: repository,
		passwords:  passwords,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

func (uc *RegisterUserUseCase) Execute(ctx context.Context, command types.RegisterUserCommand) (types.RegisterUserResult, error) {
	if err := domain.ValidateRegisterUser(command); err != nil {
		return types.RegisterUserResult{}, err
	}
	if uc.repository == nil {
		return types.RegisterUserResult{}, types.NewDBWriteFailed("identity repository is not configured")
	}
	if uc.passwords == nil {
		return types.RegisterUserResult{}, types.NewInvalidCredentials("password hasher is not configured")
	}
	passwordHash, err := uc.passwords.HashPassword(command.Password)
	if err != nil {
		return types.RegisterUserResult{}, err
	}
	return uc.repository.RegisterUser(ctx, command, passwordHash, uc.now())
}
