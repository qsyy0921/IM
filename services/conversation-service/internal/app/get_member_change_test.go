package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

func TestGetMemberChangeUseCaseValidatesCommand(t *testing.T) {
	repository := &fakeGetMemberChangeRepository{}
	_, err := NewGetMemberChangeUseCase(repository).Execute(context.Background(), types.GetMemberChangeCommand{
		ConversationID: "conv-1",
		ChangeID:       "change-1",
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if repository.called {
		t.Fatal("repository should not be called for invalid command")
	}
}

func TestGetMemberChangeUseCaseForwardsCommand(t *testing.T) {
	repository := &fakeGetMemberChangeRepository{
		result: types.MemberChangeDetail{
			ChangeID:       "change-1",
			TenantID:       "tenant-1",
			ConversationID: "conv-1",
		},
	}
	command := validGetMemberChangeCommand()

	result, err := NewGetMemberChangeUseCase(repository).Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !repository.called {
		t.Fatal("repository was not called")
	}
	if repository.command != command {
		t.Fatalf("unexpected command: %+v", repository.command)
	}
	if result.ChangeID != "change-1" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestGetMemberChangeUseCasePropagatesRepositoryError(t *testing.T) {
	repository := &fakeGetMemberChangeRepository{err: types.NewMemberChangeNotFound("missing")}

	_, err := NewGetMemberChangeUseCase(repository).Execute(context.Background(), validGetMemberChangeCommand())
	if !errors.Is(err, types.ErrMemberChangeNotFound) {
		t.Fatalf("expected member change not found, got %v", err)
	}
}

type fakeGetMemberChangeRepository struct {
	result  types.MemberChangeDetail
	err     error
	called  bool
	command types.GetMemberChangeCommand
}

func (f *fakeGetMemberChangeRepository) GetMemberChange(
	_ context.Context,
	command types.GetMemberChangeCommand,
) (types.MemberChangeDetail, error) {
	f.called = true
	f.command = command
	return f.result, f.err
}

func validGetMemberChangeCommand() types.GetMemberChangeCommand {
	return types.GetMemberChangeCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-1",
			UserID:    "owner-1",
			TraceID:   "trace-1",
			RequestID: "request-1",
		},
		ConversationID: "conv-1",
		ChangeID:       "change-1",
	}
}
