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
}

func (repository *fakeTimelineProjectionRepository) ProjectTimelineEvent(
	_ context.Context,
	command types.ProjectTimelineEventCommand,
) (types.ProjectTimelineEventResult, error) {
	repository.command = command
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

func TestProjectTimelineEventUseCaseValidation(t *testing.T) {
	useCase := NewProjectTimelineEventUseCase(&fakeTimelineProjectionRepository{})
	_, err := useCase.Execute(context.Background(), types.ProjectTimelineEventCommand{})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}
