package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

func TestGetSendContextUseCaseValidatesCommand(t *testing.T) {
	repository := &fakeConversationRepository{}
	_, err := NewGetSendContextUseCase(repository).Execute(context.Background(), types.GetSendContextCommand{
		ConversationID: "conv-1",
		UserID:         "user-1",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if repository.called {
		t.Fatal("repository should not be called for invalid command")
	}
}

func TestGetSendContextUseCaseForwardsCommand(t *testing.T) {
	repository := &fakeConversationRepository{
		result: types.ConversationSendContext{
			TenantID:       "tenant-1",
			ConversationID: "conv-1",
		},
	}
	command := types.GetSendContextCommand{
		TenantID:       "tenant-1",
		ConversationID: "conv-1",
		UserID:         "user-1",
		TraceID:        "trace-1",
	}

	result, err := NewGetSendContextUseCase(repository).Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !repository.called {
		t.Fatal("repository was not called")
	}
	if repository.command != command {
		t.Fatalf("unexpected command: %+v", repository.command)
	}
	if result.TenantID != "tenant-1" || result.ConversationID != "conv-1" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestGetSendContextUseCasePropagatesRepositoryError(t *testing.T) {
	repository := &fakeConversationRepository{err: types.NewConversationNotFound("missing")}

	_, err := NewGetSendContextUseCase(repository).Execute(context.Background(), types.GetSendContextCommand{
		TenantID:       "tenant-1",
		ConversationID: "conv-1",
		UserID:         "user-1",
	})
	if !errors.Is(err, types.ErrConversationNotFound) {
		t.Fatalf("expected conversation not found, got %v", err)
	}
}

type fakeConversationRepository struct {
	result  types.ConversationSendContext
	err     error
	called  bool
	command types.GetSendContextCommand
}

func (f *fakeConversationRepository) GetSendContext(
	_ context.Context,
	command types.GetSendContextCommand,
) (types.ConversationSendContext, error) {
	f.called = true
	f.command = command
	return f.result, f.err
}
