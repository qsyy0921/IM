package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

type fakeTimelineProjectionRepository struct {
	command types.ProjectTimelineEventCommand
	result  types.ProjectTimelineEventResult
	called  bool
}

func (repository *fakeTimelineProjectionRepository) ProjectTimelineEvent(
	_ context.Context,
	command types.ProjectTimelineEventCommand,
) (types.ProjectTimelineEventResult, error) {
	repository.command = command
	repository.called = true
	return repository.result, nil
}

func TestProjectTimelineEventUseCase(t *testing.T) {
	repository := &fakeTimelineProjectionRepository{
		result: types.ProjectTimelineEventResult{ProjectedInboxCount: 2},
	}
	useCase := NewProjectTimelineEventUseCase(repository)
	result, err := useCase.Execute(context.Background(), types.ProjectTimelineEventCommand{
		TenantID:        "tenant-1",
		EventID:         "event-1",
		EventType:       types.TimelineEventMessagePersisted,
		ConversationID:  "conv-1",
		ConversationSeq: 10,
		MessageID:       "msg-1",
		FanoutMode:      types.DeliveryFanoutModeWriteFanout,
	})
	if err != nil {
		t.Fatalf("project timeline event: %v", err)
	}
	if result.ProjectedInboxCount != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if repository.command.EventID != "event-1" {
		t.Fatalf("command was not passed to repository: %+v", repository.command)
	}
}

func TestProjectTimelineEventUseCasePassesHybridFanoutToRepository(t *testing.T) {
	repository := &fakeTimelineProjectionRepository{
		result: types.ProjectTimelineEventResult{ProjectedInboxCount: 3},
	}
	useCase := NewProjectTimelineEventUseCase(repository)
	result, err := useCase.Execute(context.Background(), types.ProjectTimelineEventCommand{
		TenantID:        "tenant-1",
		EventID:         "event-1",
		EventType:       types.TimelineEventMessagePersisted,
		ConversationID:  "conv-1",
		ConversationSeq: 10,
		MessageID:       "msg-1",
		FanoutMode:      types.DeliveryFanoutModeHybridFanout,
	})
	if err != nil {
		t.Fatalf("project timeline event: %v", err)
	}
	if !repository.called || repository.command.FanoutMode != types.DeliveryFanoutModeHybridFanout {
		t.Fatalf("repository call not recorded: called=%t command=%+v", repository.called, repository.command)
	}
	if result.ProjectedInboxCount != 3 {
		t.Fatalf("result=%+v", result)
	}
}

func TestProjectTimelineEventUseCaseValidation(t *testing.T) {
	useCase := NewProjectTimelineEventUseCase(&fakeTimelineProjectionRepository{})
	_, err := useCase.Execute(context.Background(), types.ProjectTimelineEventCommand{})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}
