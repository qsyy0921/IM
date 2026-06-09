package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

func TestCreateMemberChangeUseCaseValidatesCommand(t *testing.T) {
	repository := &fakeMemberChangeRepository{}
	_, err := NewCreateMemberChangeUseCase(repository).Execute(context.Background(), types.CreateMemberChangeCommand{
		ConversationID: "conv-1",
		TargetUserID:   "target-1",
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if repository.called {
		t.Fatal("repository should not be called for invalid command")
	}
}

func TestCreateMemberChangeUseCaseRejectsReservedConflictPolicy(t *testing.T) {
	repository := &fakeMemberChangeRepository{}
	command := validCreateMemberChangeCommand()
	command.ConflictPolicy = types.MemberChangeConflictPolicyMerge

	_, err := NewCreateMemberChangeUseCase(repository).Execute(context.Background(), command)
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if repository.called {
		t.Fatal("repository should not be called for reserved conflict policy")
	}
}

func TestCreateMemberChangeUseCaseForwardsCommand(t *testing.T) {
	repository := &fakeMemberChangeRepository{
		result: types.MemberChangeResult{
			ChangeID:       "change-1",
			TenantID:       "tenant-1",
			ConversationID: "conv-1",
		},
	}
	command := validCreateMemberChangeCommand()

	result, err := NewCreateMemberChangeUseCase(repository).Execute(context.Background(), command)
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

func TestCreateMemberChangeUseCasePropagatesRepositoryError(t *testing.T) {
	repository := &fakeMemberChangeRepository{err: types.NewMemberConflict("version conflict")}

	_, err := NewCreateMemberChangeUseCase(repository).Execute(context.Background(), validCreateMemberChangeCommand())
	if !errors.Is(err, types.ErrMemberConflict) {
		t.Fatalf("expected member conflict, got %v", err)
	}
}

type fakeMemberChangeRepository struct {
	result  types.MemberChangeResult
	err     error
	called  bool
	command types.CreateMemberChangeCommand
}

func (f *fakeMemberChangeRepository) CreateMemberChange(
	_ context.Context,
	command types.CreateMemberChangeCommand,
) (types.MemberChangeResult, error) {
	f.called = true
	f.command = command
	return f.result, f.err
}

func validCreateMemberChangeCommand() types.CreateMemberChangeCommand {
	return types.CreateMemberChangeCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-1",
			UserID:    "owner-1",
			TraceID:   "trace-1",
			RequestID: "request-1",
		},
		ConversationID:        "conv-1",
		TargetUserID:          "target-1",
		ChangeType:            types.MemberChangeTypeJoin,
		TargetRole:            types.MemberRoleMember,
		ExpectedMemberVersion: 5,
		IdempotencyKey:        "idem-1",
		ConflictPolicy:        types.MemberChangeConflictPolicyReject,
		Reason:                "invite",
	}
}
