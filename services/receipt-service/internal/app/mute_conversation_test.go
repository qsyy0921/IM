package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

func TestMuteConversationUseCasePassesCommandToRepository(t *testing.T) {
	repository := &fakeReceiptRepository{}
	useCase := NewMuteConversationUseCase(repository)
	_, err := useCase.Execute(context.Background(), types.MuteConversationCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID: "conversation-1",
		Muted:          true,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if repository.muteConversationCommand.ConversationID != "conversation-1" ||
		!repository.muteConversationCommand.Muted {
		t.Fatalf("unexpected mute command: %+v", repository.muteConversationCommand)
	}
}

func TestMuteConversationUseCaseValidatesCommand(t *testing.T) {
	useCase := NewMuteConversationUseCase(&fakeReceiptRepository{})
	_, err := useCase.Execute(context.Background(), types.MuteConversationCommand{})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}
