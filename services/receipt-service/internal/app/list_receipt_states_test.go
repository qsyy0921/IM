package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

func TestListReceiptStatesUseCasePassesBatchToRepository(t *testing.T) {
	repository := &fakeReceiptRepository{}
	access := &fakeReceiptAccess{
		viewAccess: types.ReceiptAccessContext{
			TenantID:       "tenant-1",
			ConversationID: "conversation-1",
		},
	}
	useCase := NewListReceiptStatesUseCase(repository, access)
	result, err := useCase.Execute(context.Background(), types.ListReceiptStatesCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID: "conversation-1",
		Items: []types.ReceiptStateQuery{
			{MessageID: "message-1"},
			{ConversationSeq: 2},
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(repository.getReceiptStateCalls) != 0 {
		t.Fatalf("expected no per-item repository calls, got %+v", repository.getReceiptStateCalls)
	}
	if repository.listReceiptStatesCommand.AccessContext.TenantID != "tenant-1" ||
		len(repository.listReceiptStatesCommand.Items) != 2 ||
		repository.listReceiptStatesCommand.Items[0].MessageID != "message-1" ||
		repository.listReceiptStatesCommand.Items[1].ConversationSeq != 2 {
		t.Fatalf("unexpected batch repository command: %+v", repository.listReceiptStatesCommand)
	}
	if access.viewCalls != 1 {
		t.Fatalf("expected one access check, got %d", access.viewCalls)
	}
	if len(result.Items) != 2 ||
		result.Items[0].MessageID != "message-1" ||
		result.Items[1].ConversationSeq != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestListReceiptStatesUseCaseRejectsEmptyItems(t *testing.T) {
	useCase := NewListReceiptStatesUseCase(&fakeReceiptRepository{}, nil)
	_, err := useCase.Execute(context.Background(), types.ListReceiptStatesCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID: "conversation-1",
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestListReceiptStatesUseCaseRejectsInvalidReceivedDeviceDetailLimit(t *testing.T) {
	useCase := NewListReceiptStatesUseCase(&fakeReceiptRepository{}, nil)
	base := types.ListReceiptStatesCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID: "conversation-1",
		Items:          []types.ReceiptStateQuery{{ConversationSeq: 1}},
	}
	tests := []struct {
		name    string
		command types.ListReceiptStatesCommand
	}{
		{
			name: "limit without include",
			command: func() types.ListReceiptStatesCommand {
				command := base
				command.ReceivedDeviceLimitHint = 1
				return command
			}(),
		},
		{
			name: "limit over max",
			command: func() types.ListReceiptStatesCommand {
				command := base
				command.IncludeReceivedDevices = true
				command.ReceivedDeviceLimitHint = types.MaxReceivedDeviceDetailLimit + 1
				return command
			}(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := useCase.Execute(context.Background(), test.command)
			if !errors.Is(err, types.ErrInvalidArgument) {
				t.Fatalf("expected invalid argument, got %v", err)
			}
		})
	}
}

type fakeReceiptAccess struct {
	viewCalls  int
	viewAccess types.ReceiptAccessContext
}

func (access *fakeReceiptAccess) CanMarkRead(context.Context, types.AuthContext, types.ConversationID) (types.ReceiptAccessContext, error) {
	return types.ReceiptAccessContext{}, nil
}

func (access *fakeReceiptAccess) CanViewReceiptState(context.Context, types.AuthContext, types.ConversationID) (types.ReceiptAccessContext, error) {
	access.viewCalls++
	return access.viewAccess, nil
}
