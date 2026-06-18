package app

import (
	"context"
	"testing"

	"github.com/qsyy0921/IM/services/search-service/internal/types"
)

type fakeSearchMessagesRepository struct {
	command           types.SearchMessagesCommand
	fetchLimit        int
	items             []types.SearchMessageHit
	projectionVersion int64
}

func (repository *fakeSearchMessagesRepository) SearchMessages(
	_ context.Context,
	command types.SearchMessagesCommand,
	fetchLimit int,
) ([]types.SearchMessageHit, int64, error) {
	repository.command = command
	repository.fetchLimit = fetchLimit
	return repository.items, repository.projectionVersion, nil
}

func TestSearchMessagesUseCaseFetchesOneExtraItem(t *testing.T) {
	repository := &fakeSearchMessagesRepository{
		items: []types.SearchMessageHit{
			{MessageID: "msg-1"},
			{MessageID: "msg-2"},
			{MessageID: "msg-3"},
		},
		projectionVersion: 9,
	}
	useCase := NewSearchMessagesUseCase(repository)
	result, err := useCase.Execute(context.Background(), types.SearchMessagesCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		Query:          "hello",
		ConversationID: "conv-1",
		Limit:          2,
	})
	if err != nil {
		t.Fatalf("search messages: %v", err)
	}
	if repository.fetchLimit != 3 {
		t.Fatalf("expected fetch limit 3, got %d", repository.fetchLimit)
	}
	if repository.command.NormalizedQuery() != "hello" {
		t.Fatalf("unexpected query %q", repository.command.Query)
	}
	if !result.HasMore || len(result.Items) != 2 || result.NextCursor != "msg-2" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.ProjectionVersion != 9 {
		t.Fatalf("unexpected projection version %d", result.ProjectionVersion)
	}
}
