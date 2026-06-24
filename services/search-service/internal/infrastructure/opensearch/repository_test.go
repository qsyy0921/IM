package opensearch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/search-service/internal/types"
)

func TestRepositorySearchMessagesUsesOpenSearchCandidatesThenHydrator(t *testing.T) {
	ctx := context.Background()
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", request.Method)
		}
		if request.URL.Path != "/nexusim-search/_search" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		if request.URL.Query().Get("allow_partial_search_results") != "false" {
			t.Fatalf("expected partial search results to be disabled")
		}
		if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"hits": {
				"hits": [
					{"_source": {"conversation_id": "conv-2", "message_id": "msg-2"}},
					{"_source": {"conversation_id": "conv-1", "message_id": "msg-1"}}
				]
			}
		}`))
	}))
	defer server.Close()

	hydrator := &fakeHydrator{
		items: []types.SearchMessageHit{{
			ConversationID:  "conv-2",
			MessageID:       "msg-2",
			ConversationSeq: 9,
			Snippet:         "project launch",
		}},
		projectionVersion: 17,
	}
	repository, err := NewRepository(Config{
		Endpoint: server.URL,
		Index:    "nexusim-search",
		Timeout:  time.Second,
	}, hydrator)
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}

	items, projectionVersion, err := repository.SearchMessages(ctx, types.SearchMessagesCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		Query:          "project launch",
		ConversationID: "conv-2",
		AfterSeq:       3,
		Limit:          10,
	}, 11)
	if err != nil {
		t.Fatalf("search messages: %v", err)
	}
	if projectionVersion != 17 || len(items) != 1 || items[0].MessageID != "msg-2" {
		t.Fatalf("unexpected search result items=%+v projection=%d", items, projectionVersion)
	}
	if len(hydrator.candidates) != 2 {
		t.Fatalf("expected two candidates, got %+v", hydrator.candidates)
	}
	if hydrator.candidates[0].ConversationID != "conv-2" || hydrator.candidates[0].MessageID != "msg-2" {
		t.Fatalf("candidate order should follow opensearch rank: %+v", hydrator.candidates)
	}
	if hydrator.command.AuthContext.UserID != "user-1" || hydrator.fetchLimit != 11 {
		t.Fatalf("hydrator command/fetch limit not propagated: command=%+v limit=%d", hydrator.command, hydrator.fetchLimit)
	}

	assertOpenSearchRequest(t, requestBody)
}

func TestRepositorySearchMessagesFailsClosedOnOpenSearchError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "provider body should not leak", http.StatusInternalServerError)
	}))
	defer server.Close()
	repository, err := NewRepository(Config{
		Endpoint: server.URL,
		Index:    "nexusim-search",
		Timeout:  time.Second,
	}, &fakeHydrator{})
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}

	_, _, err = repository.SearchMessages(context.Background(), validSearchCommand(), 10)
	if !errors.Is(err, types.ErrSearchUnavailable) {
		t.Fatalf("expected search unavailable, got %v", err)
	}
}

func TestRepositorySearchMessagesFailsClosedOnMalformedHit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"hits":{"hits":[{"_source":{"conversation_id":"conv-1"}}]}}`))
	}))
	defer server.Close()
	repository, err := NewRepository(Config{
		Endpoint: server.URL,
		Index:    "nexusim-search",
		Timeout:  time.Second,
	}, &fakeHydrator{})
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}

	_, _, err = repository.SearchMessages(context.Background(), validSearchCommand(), 10)
	if !errors.Is(err, types.ErrSearchUnavailable) {
		t.Fatalf("expected search unavailable, got %v", err)
	}
}

func TestNewRepositoryValidatesEndpointAndIndex(t *testing.T) {
	if _, err := NewRepository(Config{Endpoint: "https://search.example.com", Index: "bad/index"}, &fakeHydrator{}); err == nil {
		t.Fatalf("expected bad index to fail")
	}
	if _, err := NewRepository(Config{Endpoint: "ftp://search.example.com", Index: "nexusim"}, &fakeHydrator{}); err == nil {
		t.Fatalf("expected bad endpoint scheme to fail")
	}
	if _, err := NewRepository(Config{Endpoint: "https://user:pass@search.example.com", Index: "nexusim"}, &fakeHydrator{}); err == nil {
		t.Fatalf("expected endpoint credentials to fail")
	}
	if _, err := NewRepository(Config{Endpoint: "https://search.example.com", Index: "nexusim", Username: "user"}, &fakeHydrator{}); err == nil {
		t.Fatalf("expected username without password to fail")
	}
}

func assertOpenSearchRequest(t *testing.T, request map[string]any) {
	t.Helper()
	source, ok := request["_source"].([]any)
	if !ok || len(source) != 2 || source[0] != "conversation_id" || source[1] != "message_id" {
		t.Fatalf("unexpected source fields: %+v", request["_source"])
	}
	query := request["query"].(map[string]any)
	boolQuery := query["bool"].(map[string]any)
	must := boolQuery["must"].([]any)
	match := must[0].(map[string]any)["match"].(map[string]any)
	searchable := match["searchable_text"].(map[string]any)
	if searchable["query"] != "project launch" || searchable["operator"] != "and" {
		t.Fatalf("unexpected match query: %+v", searchable)
	}
	filters := boolQuery["filter"].([]any)
	if len(filters) != 3 {
		t.Fatalf("expected tenant/range/conversation filters, got %+v", filters)
	}
}

func validSearchCommand() types.SearchMessagesCommand {
	return types.SearchMessagesCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		Query: "project launch",
		Limit: 10,
	}
}

type fakeHydrator struct {
	candidates        []types.SearchMessageCandidate
	command           types.SearchMessagesCommand
	fetchLimit        int
	items             []types.SearchMessageHit
	projectionVersion int64
}

func (hydrator *fakeHydrator) SearchMessagesByCandidates(
	_ context.Context,
	command types.SearchMessagesCommand,
	candidates []types.SearchMessageCandidate,
	fetchLimit int,
) ([]types.SearchMessageHit, int64, error) {
	hydrator.command = command
	hydrator.candidates = append([]types.SearchMessageCandidate(nil), candidates...)
	hydrator.fetchLimit = fetchLimit
	return hydrator.items, hydrator.projectionVersion, nil
}
