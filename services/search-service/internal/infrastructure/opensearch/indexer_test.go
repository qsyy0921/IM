package opensearch

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qsyy0921/IM/services/search-service/internal/types"
)

func TestIndexerEnsuresIndexesAndRefreshes(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.Method+" "+request.URL.Path)
		switch {
		case request.Method == http.MethodPut && request.URL.Path == "/search-index":
			writer.WriteHeader(http.StatusCreated)
			_, _ = writer.Write([]byte(`{"acknowledged":true}`))
		case request.Method == http.MethodPost && request.URL.Path == "/_bulk":
			if got := request.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/x-ndjson") {
				t.Fatalf("bulk content type=%q", got)
			}
			var lines []string
			decoder := json.NewDecoder(request.Body)
			for {
				var item map[string]any
				if err := decoder.Decode(&item); err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					t.Fatalf("decode bulk item: %v", err)
				}
				encoded, _ := json.Marshal(item)
				lines = append(lines, string(encoded))
			}
			if len(lines) != 2 {
				t.Fatalf("bulk lines=%d want 2: %v", len(lines), lines)
			}
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte(`{"errors":false}`))
		case request.Method == http.MethodPost && request.URL.Path == "/search-index/_refresh":
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte(`{"_shards":{"successful":1}}`))
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	indexer, err := NewIndexer(Config{Endpoint: server.URL, Index: "search-index", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("NewIndexer returned error: %v", err)
	}
	ctx := context.Background()
	if err := indexer.EnsureSearchIndex(ctx); err != nil {
		t.Fatalf("EnsureSearchIndex returned error: %v", err)
	}
	if err := indexer.IndexSearchDocuments(ctx, []types.SearchIndexDocument{{
		TenantID:        "tenant-1",
		ConversationID:  "conv-1",
		MessageID:       "message-1",
		ConversationSeq: 7,
		SearchableText:  "project launch decision",
	}}); err != nil {
		t.Fatalf("IndexSearchDocuments returned error: %v", err)
	}
	if err := indexer.RefreshSearchIndex(ctx); err != nil {
		t.Fatalf("RefreshSearchIndex returned error: %v", err)
	}
	want := []string{"PUT /search-index", "POST /_bulk", "POST /search-index/_refresh"}
	if strings.Join(paths, "|") != strings.Join(want, "|") {
		t.Fatalf("paths=%v want %v", paths, want)
	}
}

func TestIndexerFailsClosedOnBulkItemErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`{"errors":true}`))
	}))
	defer server.Close()

	indexer, err := NewIndexer(Config{Endpoint: server.URL, Index: "search-index", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("NewIndexer returned error: %v", err)
	}
	err = indexer.IndexSearchDocuments(context.Background(), []types.SearchIndexDocument{{
		TenantID:        "tenant-1",
		ConversationID:  "conv-1",
		MessageID:       "message-1",
		ConversationSeq: 7,
		SearchableText:  "project launch decision",
	}})
	if !errors.Is(err, types.ErrSearchUnavailable) {
		t.Fatalf("err=%v want ErrSearchUnavailable", err)
	}
}
