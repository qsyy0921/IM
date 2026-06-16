package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

func TestSetConversationDraftUseCasePassesCommand(t *testing.T) {
	repository := &fakeReceiptRepository{}
	useCase := NewSetConversationDraftUseCase(repository)
	_, err := useCase.Execute(context.Background(), types.SetConversationDraftCommand{
		AuthContext:    testAuthContext(),
		ConversationID: "conv-1",
		DraftText:      "hello draft",
	})
	if err != nil {
		t.Fatalf("set conversation draft: %v", err)
	}
	if repository.setConversationDraftCommand.DraftText != "hello draft" ||
		repository.setConversationDraftCommand.ConversationID != "conv-1" {
		t.Fatalf("unexpected set draft command: %+v", repository.setConversationDraftCommand)
	}
}

func TestSetConversationDraftUseCaseRejectsInvalidDraft(t *testing.T) {
	useCase := NewSetConversationDraftUseCase(&fakeReceiptRepository{})
	_, err := useCase.Execute(context.Background(), types.SetConversationDraftCommand{
		AuthContext:    testAuthContext(),
		ConversationID: "conv-1",
		DraftText:      strings.Repeat("x", types.MaxConversationDraftSize+1),
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func testAuthContext() types.AuthContext {
	return types.AuthContext{
		TenantID: "tenant-1",
		UserID:   "user-1",
		DeviceID: "device-1",
	}
}
