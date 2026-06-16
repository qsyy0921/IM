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

func TestListConversationMembersUseCaseRejectsInvalidRoleFilter(t *testing.T) {
	repository := &fakeListConversationMembersRepository{}
	command := validListConversationMembersCommand()
	command.RoleFilter = "SUPER_ADMIN"
	_, err := NewListConversationMembersUseCase(repository).Execute(context.Background(), command)
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if repository.called {
		t.Fatalf("repository should not be called")
	}
}

func TestListConversationMembersUseCaseRejectsInvalidRoleFilters(t *testing.T) {
	repository := &fakeListConversationMembersRepository{}
	command := validListConversationMembersCommand()
	command.RoleFilters = []types.MemberRole{"SUPER_ADMIN"}
	_, err := NewListConversationMembersUseCase(repository).Execute(context.Background(), command)
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if repository.called {
		t.Fatalf("repository should not be called")
	}
}

func TestListConversationMembersUseCaseRejectsInvalidSort(t *testing.T) {
	repository := &fakeListConversationMembersRepository{}
	command := validListConversationMembersCommand()
	command.Sort = "unknown"
	_, err := NewListConversationMembersUseCase(repository).Execute(context.Background(), command)
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
	command.RoleFilter = types.MemberRoleAdmin
	command.RoleFilters = []types.MemberRole{types.MemberRoleOwner, types.MemberRoleAdmin}
	command.Sort = types.ConversationMemberListSortRoleUserIDAsc

	result, err := NewListConversationMembersUseCase(repository).Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("list conversation members: %v", err)
	}
	if !listConversationMembersCommandsEqual(repository.command, command) {
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
		RoleFilter:     types.MemberRoleAdmin,
	}
}

func listConversationMembersCommandsEqual(left types.ListConversationMembersCommand, right types.ListConversationMembersCommand) bool {
	return left.AuthContext == right.AuthContext &&
		left.ConversationID == right.ConversationID &&
		left.PageSize == right.PageSize &&
		left.PageToken == right.PageToken &&
		left.RoleFilter == right.RoleFilter &&
		left.Sort == right.Sort &&
		memberRolesEqual(left.RoleFilters, right.RoleFilters)
}

func memberRolesEqual(left []types.MemberRole, right []types.MemberRole) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
