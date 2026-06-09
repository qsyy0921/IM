package app

import (
	"context"
	"testing"

	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

type fakeInboxRepository struct {
	command    types.PullInboxCommand
	fetchLimit int
	items      []types.InboxItem
}

func (repository *fakeInboxRepository) PullInbox(
	_ context.Context,
	command types.PullInboxCommand,
	fetchLimit int,
) ([]types.InboxItem, error) {
	repository.command = command
	repository.fetchLimit = fetchLimit
	return repository.items, nil
}

func TestPullInboxUseCaseFetchesOneExtraItem(t *testing.T) {
	repository := &fakeInboxRepository{
		items: []types.InboxItem{
			{ConversationSeq: 1},
			{ConversationSeq: 2},
			{ConversationSeq: 3},
		},
	}
	useCase := NewPullInboxUseCase(repository)
	result, err := useCase.Execute(context.Background(), types.PullInboxCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID: "conv-1",
		AfterSeq:       0,
		Limit:          2,
	})
	if err != nil {
		t.Fatalf("pull inbox: %v", err)
	}
	if repository.fetchLimit != 3 {
		t.Fatalf("expected fetch limit 3, got %d", repository.fetchLimit)
	}
	if !result.HasMore || result.NextSeq != 2 || len(result.Items) != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
}
