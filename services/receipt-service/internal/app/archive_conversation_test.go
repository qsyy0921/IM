package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

func TestArchiveConversationUseCasePassesCommandToRepository(t *testing.T) {
	repository := &fakeReceiptRepository{}
	useCase := NewArchiveConversationUseCase(repository)
	_, err := useCase.Execute(context.Background(), types.ArchiveConversationCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID: "conversation-1",
		Archived:       true,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if repository.archiveConversationCommand.ConversationID != "conversation-1" ||
		!repository.archiveConversationCommand.Archived {
		t.Fatalf("unexpected archive command: %+v", repository.archiveConversationCommand)
	}
}

func TestArchiveConversationUseCaseValidatesCommand(t *testing.T) {
	useCase := NewArchiveConversationUseCase(&fakeReceiptRepository{})
	_, err := useCase.Execute(context.Background(), types.ArchiveConversationCommand{})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}
