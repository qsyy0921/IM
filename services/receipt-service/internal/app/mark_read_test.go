package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

func TestMarkReadUseCaseValidatesCommand(t *testing.T) {
	useCase := NewMarkReadUseCase(&fakeReceiptRepository{}, nil)
	_, err := useCase.Execute(context.Background(), types.MarkReadCommand{})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestMarkReadUseCasePassesCommandToRepository(t *testing.T) {
	repository := &fakeReceiptRepository{}
	useCase := NewMarkReadUseCase(repository, nil)
	result, err := useCase.Execute(context.Background(), types.MarkReadCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID: "conversation-1",
		ReadSeq:        11,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if repository.markReadCommand.ReadSeq != 11 {
		t.Fatalf("expected repository command read_seq=11, got %d", repository.markReadCommand.ReadSeq)
	}
	if result.LastReadSeq != 11 {
		t.Fatalf("expected last_read_seq=11, got %d", result.LastReadSeq)
	}
}

type fakeReceiptRepository struct {
	markReadCommand types.MarkReadCommand
}

func (repository fakeReceiptRepository) GetReceiptState(context.Context, types.GetReceiptStateCommand) (types.GetReceiptStateResult, error) {
	return types.GetReceiptStateResult{}, nil
}

func (repository fakeReceiptRepository) ListConversations(context.Context, types.ListConversationsCommand) (types.ListConversationsResult, error) {
	return types.ListConversationsResult{}, nil
}

func (repository *fakeReceiptRepository) MarkRead(_ context.Context, command types.MarkReadCommand) (types.MarkReadResult, error) {
	repository.markReadCommand = command
	return types.MarkReadResult{
		TenantID:       command.AuthContext.TenantID,
		UserID:         command.AuthContext.UserID,
		ConversationID: command.ConversationID,
		LastReadSeq:    command.ReadSeq,
	}, nil
}
