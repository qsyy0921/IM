package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/qsyy0921/IM/services/presence-service/internal/domain"
	"github.com/qsyy0921/IM/services/presence-service/internal/types"
)

type UpdatePresenceUseCase struct {
	repository Repository
	eventIDs   EventIDGenerator
}

func NewUpdatePresenceUseCase(repository Repository, eventIDs EventIDGenerator) *UpdatePresenceUseCase {
	return &UpdatePresenceUseCase{repository: repository, eventIDs: eventIDs}
}

func (useCase *UpdatePresenceUseCase) Execute(
	ctx context.Context,
	command types.UpdatePresenceCommand,
) (types.PresenceState, error) {
	prepared, err := domain.PreparePresenceUpdate(command, time.Now())
	if err != nil {
		return types.PresenceState{}, err
	}
	eventID, err := useCase.eventIDs.NewEventID()
	if err != nil {
		return types.PresenceState{}, err
	}
	return useCase.repository.UpdatePresence(ctx, prepared, eventID)
}

type GetPresenceUseCase struct {
	repository Repository
}

func NewGetPresenceUseCase(repository Repository) *GetPresenceUseCase {
	return &GetPresenceUseCase{repository: repository}
}

func (useCase *GetPresenceUseCase) Execute(
	ctx context.Context,
	command types.GetPresenceCommand,
) ([]types.PresenceState, error) {
	if err := command.Validate(); err != nil {
		return nil, err
	}
	normalized := command.Normalized()
	states, err := useCase.repository.GetPresenceStates(ctx, normalized)
	if err != nil {
		return nil, err
	}
	return domain.ApplyVisibility(normalized, states), nil
}

type UpdateTypingUseCase struct {
	repository Repository
	eventIDs   EventIDGenerator
}

func NewUpdateTypingUseCase(repository Repository, eventIDs EventIDGenerator) *UpdateTypingUseCase {
	return &UpdateTypingUseCase{repository: repository, eventIDs: eventIDs}
}

func (useCase *UpdateTypingUseCase) Execute(
	ctx context.Context,
	command types.UpdateTypingCommand,
) (types.TypingIndicator, error) {
	prepared, err := domain.PrepareTypingUpdate(command, time.Now())
	if err != nil {
		return types.TypingIndicator{}, err
	}
	eventID, err := useCase.eventIDs.NewEventID()
	if err != nil {
		return types.TypingIndicator{}, err
	}
	return useCase.repository.UpdateTyping(ctx, prepared, eventID)
}

type RandomEventIDGenerator struct{}

func NewRandomEventIDGenerator() RandomEventIDGenerator {
	return RandomEventIDGenerator{}
}

func (RandomEventIDGenerator) NewEventID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", types.NewFailedPrecondition("presence event id generation failed")
	}
	return "evt_presence_" + hex.EncodeToString(value[:]), nil
}
