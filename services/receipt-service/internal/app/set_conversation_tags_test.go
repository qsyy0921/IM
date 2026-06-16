package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

func TestSetConversationTagsUseCaseNormalizesAndPassesCommand(t *testing.T) {
	repository := &fakeReceiptRepository{}
	useCase := NewSetConversationTagsUseCase(repository)
	_, err := useCase.Execute(context.Background(), types.SetConversationTagsCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID: "conversation-1",
		Tags:           []string{"work", "important", "work"},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if repository.setConversationTagsCommand.ConversationID != "conversation-1" ||
		len(repository.setConversationTagsCommand.Tags) != 2 ||
		repository.setConversationTagsCommand.Tags[0] != "work" ||
		repository.setConversationTagsCommand.Tags[1] != "important" {
		t.Fatalf("unexpected set tags command: %+v", repository.setConversationTagsCommand)
	}
}

func TestSetConversationTagsUseCaseRejectsInvalidTags(t *testing.T) {
	useCase := NewSetConversationTagsUseCase(&fakeReceiptRepository{})
	_, err := useCase.Execute(context.Background(), types.SetConversationTagsCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID: "conversation-1",
		Tags:           []string{"bad tag"},
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}
