package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/qsyy0921/IM/services/notification-service/internal/types"
)

type CreateNotificationRequestUseCase struct {
	repository      Repository
	destinationHash DestinationHasher
	requestIDs      RequestIDGenerator
}

func NewCreateNotificationRequestUseCase(
	repository Repository,
	destinationHash DestinationHasher,
	requestIDs RequestIDGenerator,
) *CreateNotificationRequestUseCase {
	return &CreateNotificationRequestUseCase{
		repository:      repository,
		destinationHash: destinationHash,
		requestIDs:      requestIDs,
	}
}

func (useCase *CreateNotificationRequestUseCase) Execute(
	ctx context.Context,
	command types.CreateNotificationRequestCommand,
) (types.NotificationRequest, error) {
	if err := command.Validate(); err != nil {
		return types.NotificationRequest{}, err
	}
	if useCase.repository == nil {
		return types.NotificationRequest{}, types.NewDBWriteFailed("notification repository is not configured")
	}
	if useCase.destinationHash == nil {
		return types.NotificationRequest{}, types.NewDependencyFailed("notification destination hasher is not configured")
	}
	if useCase.requestIDs == nil {
		return types.NotificationRequest{}, types.NewDependencyFailed("notification request id generator is not configured")
	}
	command = command.Normalized()
	destinationHash, err := useCase.destinationHash.HashDestination(command.DestinationRef)
	if err != nil {
		return types.NotificationRequest{}, err
	}
	requestID, err := useCase.requestIDs.NewRequestID()
	if err != nil {
		return types.NotificationRequest{}, err
	}
	commandHash := command.CommandHash(destinationHash)
	return useCase.repository.CreateNotificationRequest(ctx, command, requestID, destinationHash, commandHash)
}

type GetNotificationStatusUseCase struct {
	repository Repository
}

func NewGetNotificationStatusUseCase(repository Repository) *GetNotificationStatusUseCase {
	return &GetNotificationStatusUseCase{repository: repository}
}

func (useCase *GetNotificationStatusUseCase) Execute(
	ctx context.Context,
	command types.GetNotificationStatusCommand,
) (types.NotificationRequest, error) {
	if err := command.Validate(); err != nil {
		return types.NotificationRequest{}, err
	}
	if useCase.repository == nil {
		return types.NotificationRequest{}, types.NewDBReadFailed("notification repository is not configured")
	}
	return useCase.repository.GetNotificationRequest(ctx, command.AuthContext.TenantID, command.RequestID)
}

type CancelNotificationRequestUseCase struct {
	repository Repository
}

func NewCancelNotificationRequestUseCase(repository Repository) *CancelNotificationRequestUseCase {
	return &CancelNotificationRequestUseCase{repository: repository}
}

func (useCase *CancelNotificationRequestUseCase) Execute(
	ctx context.Context,
	command types.CancelNotificationRequestCommand,
) (types.NotificationRequest, error) {
	if err := command.Validate(); err != nil {
		return types.NotificationRequest{}, err
	}
	if useCase.repository == nil {
		return types.NotificationRequest{}, types.NewDBWriteFailed("notification repository is not configured")
	}
	return useCase.repository.CancelNotificationRequest(ctx, command)
}

type RandomRequestIDGenerator struct{}

func NewRandomRequestIDGenerator() RandomRequestIDGenerator {
	return RandomRequestIDGenerator{}
}

func (RandomRequestIDGenerator) NewRequestID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", types.NewDependencyFailed("notification request id generation failed")
	}
	return "notif_" + hex.EncodeToString(raw[:]), nil
}
