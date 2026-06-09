package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

func TestListReceiptStatesUseCasePassesItemsToRepository(t *testing.T) {
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
	if len(repository.getReceiptStateCalls) != 2 ||
		repository.getReceiptStateCalls[0].MessageID != "message-1" ||
		repository.getReceiptStateCalls[1].ConversationSeq != 2 ||
		repository.getReceiptStateCalls[0].AccessContext.TenantID != "tenant-1" {
		t.Fatalf("unexpected repository calls: %+v", repository.getReceiptStateCalls)
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
