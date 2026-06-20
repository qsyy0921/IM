package app

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"context"

	"github.com/qsyy0921/IM/services/admin-service/internal/domain"
	"github.com/qsyy0921/IM/services/admin-service/internal/types"
)

type CreateAdminOperationResult struct {
	Operation types.AdminOperation
	Replayed  bool
}

type ApproveAdminOperationResult struct {
	Operation types.AdminOperation
	Approval  types.AdminApproval
	Replayed  bool
}

type CreateAdminOperationUseCase struct {
	repository AdminRepository
	ids        IDGenerator
}

func NewCreateAdminOperationUseCase(repository AdminRepository, ids IDGenerator) *CreateAdminOperationUseCase {
	return &CreateAdminOperationUseCase{repository: repository, ids: ids}
}

func (useCase *CreateAdminOperationUseCase) Execute(ctx context.Context, command types.CreateAdminOperationCommand) (CreateAdminOperationResult, error) {
	if useCase.repository == nil || useCase.ids == nil {
		return CreateAdminOperationResult{}, types.NewUnavailable("admin create dependencies are not configured")
	}
	prepared, err := domain.PrepareCreate(command, useCase.ids.NewID("admop"), time.Now().UTC())
	if err != nil {
		return CreateAdminOperationResult{}, err
	}
	operation, replayed, err := useCase.repository.CreateAdminOperation(ctx, prepared)
	if err != nil {
		return CreateAdminOperationResult{}, err
	}
	return CreateAdminOperationResult{Operation: operation, Replayed: replayed}, nil
}

type ApproveAdminOperationUseCase struct {
	repository AdminRepository
	ids        IDGenerator
}

func NewApproveAdminOperationUseCase(repository AdminRepository, ids IDGenerator) *ApproveAdminOperationUseCase {
	return &ApproveAdminOperationUseCase{repository: repository, ids: ids}
}

func (useCase *ApproveAdminOperationUseCase) Execute(ctx context.Context, command types.ApproveAdminOperationCommand) (ApproveAdminOperationResult, error) {
	if useCase.repository == nil || useCase.ids == nil {
		return ApproveAdminOperationResult{}, types.NewUnavailable("admin approval dependencies are not configured")
	}
	prepared, err := domain.PrepareApproval(command, useCase.ids.NewID("admappr"), time.Now().UTC())
	if err != nil {
		return ApproveAdminOperationResult{}, err
	}
	operation, approval, replayed, err := useCase.repository.ApproveAdminOperation(ctx, prepared)
	if err != nil {
		return ApproveAdminOperationResult{}, err
	}
	return ApproveAdminOperationResult{Operation: operation, Approval: approval, Replayed: replayed}, nil
}

type GetAdminOperationUseCase struct {
	repository AdminRepository
}

func NewGetAdminOperationUseCase(repository AdminRepository) *GetAdminOperationUseCase {
	return &GetAdminOperationUseCase{repository: repository}
}

func (useCase *GetAdminOperationUseCase) Execute(ctx context.Context, command types.GetAdminOperationCommand) (types.AdminOperation, []types.AdminApproval, error) {
	if useCase.repository == nil {
		return types.AdminOperation{}, nil, types.NewUnavailable("admin repository is not configured")
	}
	if !command.AuthContext.Valid() || command.OperationID == "" {
		return types.AdminOperation{}, nil, types.NewInvalidArgument("tenant and operation id are required")
	}
	return useCase.repository.GetAdminOperation(ctx, command)
}

type ListAdminOperationsUseCase struct {
	repository AdminRepository
}

func NewListAdminOperationsUseCase(repository AdminRepository) *ListAdminOperationsUseCase {
	return &ListAdminOperationsUseCase{repository: repository}
}

func (useCase *ListAdminOperationsUseCase) Execute(ctx context.Context, command types.ListAdminOperationsCommand) ([]types.AdminOperation, error) {
	if useCase.repository == nil {
		return nil, types.NewUnavailable("admin repository is not configured")
	}
	if !command.AuthContext.Valid() {
		return nil, types.NewInvalidArgument("tenant is required")
	}
	if command.PageSize <= 0 {
		command.PageSize = 50
	}
	if command.PageSize > 100 {
		command.PageSize = 100
	}
	return useCase.repository.ListAdminOperations(ctx, command)
}

type RandomIDGenerator struct{}

func NewRandomIDGenerator() RandomIDGenerator {
	return RandomIDGenerator{}
}

func (RandomIDGenerator) NewID(prefix string) string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return prefix + "_" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return prefix + "_" + hex.EncodeToString(bytes[:])
}
