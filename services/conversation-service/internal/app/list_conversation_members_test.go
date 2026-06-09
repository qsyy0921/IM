package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

func TestListConversationMembersUseCaseValidatesCommand(t *testing.T) {
	repository := &fakeListConversationMembersRepository{}
	_, err := NewListConversationMembersUseCase(repository).Execute(context.Background(), types.ListConversationMembersCommand{
		AuthContext: types.AuthContext{TenantID: "tenant-1"},
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if repository.called {
		t.Fatalf("repository should not be called")
	}
}

func TestListConversationMembersUseCaseForwardsCommand(t *testing.T) {
	repository := &fakeListConversationMembersRepository{
		result: types.ListConversationMembersResult{
			TenantID:       "tenant-1",
			ConversationID: "conv-1",
			Members: []types.ConversationMember{
				{UserID: "user-1", Role: types.MemberRoleMember, Status: types.MemberStatusActive},
			},
		},
	}
	command := validListConversationMembersCommand()

	result, err := NewListConversationMembersUseCase(repository).Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("list conversation members: %v", err)
	}
	if repository.command != command {
		t.Fatalf("unexpected command: %+v", repository.command)
	}
	if len(result.Members) != 1 || result.Members[0].UserID != "user-1" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestListConversationMembersUseCasePropagatesRepositoryError(t *testing.T) {
	repository := &fakeListConversationMembersRepository{err: types.NewMemberNotActive("left")}

	_, err := NewListConversationMembersUseCase(repository).Execute(context.Background(), validListConversationMembersCommand())
	if !errors.Is(err, types.ErrMemberNotActive) {
		t.Fatalf("expected member not active, got %v", err)
	}
}

type fakeListConversationMembersRepository struct {
	called  bool
	result  types.ListConversationMembersResult
	err     error
	command types.ListConversationMembersCommand
}

func (f *fakeListConversationMembersRepository) ListConversationMembers(
	_ context.Context,
	command types.ListConversationMembersCommand,
) (types.ListConversationMembersResult, error) {
	f.called = true
	f.command = command
	return f.result, f.err
}

func validListConversationMembersCommand() types.ListConversationMembersCommand {
	return types.ListConversationMembersCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
		},
		ConversationID: "conv-1",
		PageSize:       50,
	}
}
