package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

func TestListConversationsUseCasePassesSortToRepository(t *testing.T) {
	repository := &fakeReceiptRepository{}
	useCase := NewListConversationsUseCase(repository)
	_, err := useCase.Execute(context.Background(), types.ListConversationsCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		Limit:      20,
		PageCursor: "cursor-1",
		Sort:       types.ConversationListSortUpdatedAtDesc,
		UnreadOnly: true,
		PinnedOnly: true,
		MutedOnly:  true,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if repository.listCommand.Sort != types.ConversationListSortUpdatedAtDesc ||
		repository.listCommand.Limit != 20 ||
		repository.listCommand.PageCursor != "cursor-1" ||
		!repository.listCommand.UnreadOnly ||
		!repository.listCommand.PinnedOnly ||
		!repository.listCommand.MutedOnly {
		t.Fatalf("unexpected repository command: %+v", repository.listCommand)
	}
}

func TestListConversationsUseCaseRejectsUnsupportedSort(t *testing.T) {
	useCase := NewListConversationsUseCase(&fakeReceiptRepository{})
	_, err := useCase.Execute(context.Background(), types.ListConversationsCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		Sort: "unsupported",
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}
