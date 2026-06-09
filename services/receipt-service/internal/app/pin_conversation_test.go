package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

func TestPinConversationUseCasePassesCommandToRepository(t *testing.T) {
	repository := &fakeReceiptRepository{}
	useCase := NewPinConversationUseCase(repository)
	_, err := useCase.Execute(context.Background(), types.PinConversationCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID: "conversation-1",
		Pinned:         true,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if repository.pinConversationCommand.ConversationID != "conversation-1" ||
		!repository.pinConversationCommand.Pinned {
		t.Fatalf("unexpected pin command: %+v", repository.pinConversationCommand)
	}
}

func TestPinConversationUseCaseValidatesCommand(t *testing.T) {
	useCase := NewPinConversationUseCase(&fakeReceiptRepository{})
	_, err := useCase.Execute(context.Background(), types.PinConversationCommand{})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}
