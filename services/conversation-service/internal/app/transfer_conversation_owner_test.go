package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

func TestTransferConversationOwnerUseCaseValidatesCommand(t *testing.T) {
	repository := &fakeOwnerTransferRepository{}

	_, err := NewTransferConversationOwnerUseCase(repository).Execute(context.Background(), types.TransferConversationOwnerCommand{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "owner-1"},
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if repository.called {
		t.Fatal("repository should not be called")
	}
}

func TestTransferConversationOwnerUseCaseForwardsCommand(t *testing.T) {
	repository := &fakeOwnerTransferRepository{
		result: types.TransferConversationOwnerResult{
			ChangeID:            "change-owner-1",
			TenantID:            "tenant-1",
			ConversationID:      "conv-1",
			PreviousOwnerUserID: "owner-1",
			NewOwnerUserID:      "user-2",
			Status:              types.MemberChangeStatusOutboxEnqueued,
			BoundarySeq:         3,
			MemberVersion:       8,
			PermissionVersion:   9,
		},
	}
	command := validTransferConversationOwnerCommand()

	result, err := NewTransferConversationOwnerUseCase(repository).Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("execute transfer owner: %v", err)
	}
	if !repository.called || repository.command != command {
		t.Fatalf("repository did not receive command: called=%v command=%+v", repository.called, repository.command)
	}
	if result.ChangeID != "change-owner-1" || result.NewOwnerUserID != "user-2" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestTransferConversationOwnerUseCasePropagatesRepositoryError(t *testing.T) {
	repository := &fakeOwnerTransferRepository{err: types.NewMemberConflict("version")}

	_, err := NewTransferConversationOwnerUseCase(repository).Execute(context.Background(), validTransferConversationOwnerCommand())
	if !errors.Is(err, types.ErrMemberConflict) {
		t.Fatalf("expected member conflict, got %v", err)
	}
}

type fakeOwnerTransferRepository struct {
	called  bool
	command types.TransferConversationOwnerCommand
	result  types.TransferConversationOwnerResult
	err     error
}

func (f *fakeOwnerTransferRepository) TransferConversationOwner(
	_ context.Context,
	command types.TransferConversationOwnerCommand,
) (types.TransferConversationOwnerResult, error) {
	f.called = true
	f.command = command
	return f.result, f.err
}

func validTransferConversationOwnerCommand() types.TransferConversationOwnerCommand {
	return types.TransferConversationOwnerCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-1",
			UserID:    "owner-1",
			RequestID: "request-1",
			TraceID:   "trace-1",
		},
		ConversationID:        "conv-1",
		NewOwnerUserID:        "user-2",
		ExpectedMemberVersion: 7,
		IdempotencyKey:        "transfer-owner-1",
		Reason:                "handoff",
	}
}
