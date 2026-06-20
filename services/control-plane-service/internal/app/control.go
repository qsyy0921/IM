package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/qsyy0921/IM/services/control-plane-service/internal/domain"
	"github.com/qsyy0921/IM/services/control-plane-service/internal/types"
)

type PublishConfigVersionUseCase struct {
	repository Repository
	eventIDs   EventIDGenerator
}

func NewPublishConfigVersionUseCase(repository Repository, eventIDs EventIDGenerator) *PublishConfigVersionUseCase {
	return &PublishConfigVersionUseCase{repository: repository, eventIDs: eventIDs}
}

func (useCase *PublishConfigVersionUseCase) Execute(
	ctx context.Context,
	command types.PublishConfigVersionCommand,
) (types.ConfigVersion, error) {
	prepared, err := domain.PrepareConfigVersion(command)
	if err != nil {
		return types.ConfigVersion{}, err
	}
	return useCase.repository.PublishConfigVersion(ctx, prepared, useCase.eventIDs.NewEventID())
}

type GetConfigSnapshotUseCase struct {
	repository Repository
}

func NewGetConfigSnapshotUseCase(repository Repository) *GetConfigSnapshotUseCase {
	return &GetConfigSnapshotUseCase{repository: repository}
}

func (useCase *GetConfigSnapshotUseCase) Execute(
	ctx context.Context,
	command types.GetConfigSnapshotCommand,
) (types.ConfigSnapshot, error) {
	if err := command.Validate(); err != nil {
		return types.ConfigSnapshot{}, err
	}
	return useCase.repository.GetConfigSnapshot(ctx, command.Normalized())
}

type AckAppliedConfigVersionUseCase struct {
	repository Repository
	eventIDs   EventIDGenerator
}

func NewAckAppliedConfigVersionUseCase(repository Repository, eventIDs EventIDGenerator) *AckAppliedConfigVersionUseCase {
	return &AckAppliedConfigVersionUseCase{repository: repository, eventIDs: eventIDs}
}

func (useCase *AckAppliedConfigVersionUseCase) Execute(
	ctx context.Context,
	command types.AckAppliedConfigVersionCommand,
) (types.AppliedConfigVersion, error) {
	if err := command.Validate(); err != nil {
		return types.AppliedConfigVersion{}, err
	}
	return useCase.repository.AckAppliedConfigVersion(ctx, command.Normalized(), useCase.eventIDs.NewEventID())
}

type RandomEventIDGenerator struct{}

func NewRandomEventIDGenerator() RandomEventIDGenerator {
	return RandomEventIDGenerator{}
}

func (RandomEventIDGenerator) NewEventID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "evt_control_fallback"
	}
	return "evt_control_" + hex.EncodeToString(value[:])
}
