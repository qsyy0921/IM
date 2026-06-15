package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

type fakeInboxVisibilityRepository struct {
	command types.HideInboxItemCommand
	result  types.HideInboxItemResult
	err     error
}

func (repository *fakeInboxVisibilityRepository) HideInboxItem(
	_ context.Context,
	command types.HideInboxItemCommand,
) (types.HideInboxItemResult, error) {
	repository.command = command
	return repository.result, repository.err
}

func TestHideInboxItemUseCaseValidatesCommand(t *testing.T) {
	repository := &fakeInboxVisibilityRepository{}
	useCase := NewHideInboxItemUseCase(repository)
	_, err := useCase.Execute(context.Background(), types.HideInboxItemCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID: "conv-1",
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if repository.command.ConversationID != "" {
		t.Fatalf("repository should not be called on invalid command: %+v", repository.command)
	}
}

func TestHideInboxItemUseCaseDelegates(t *testing.T) {
	repository := &fakeInboxVisibilityRepository{
		result: types.HideInboxItemResult{
			TenantID:        "tenant-1",
			UserID:          "user-1",
			ConversationID:  "conv-1",
			ConversationSeq: 9,
		},
	}
	useCase := NewHideInboxItemUseCase(repository)
	result, err := useCase.Execute(context.Background(), types.HideInboxItemCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID:  "conv-1",
		ConversationSeq: 9,
		Reason:          "hide locally",
	})
	if err != nil {
		t.Fatalf("hide inbox item: %v", err)
	}
	if result.ConversationSeq != 9 || repository.command.Reason != "hide locally" {
		t.Fatalf("unexpected result=%+v command=%+v", result, repository.command)
	}
}
