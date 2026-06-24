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
	var createRequest map[string]any
	var bulkDocuments []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.Method+" "+request.URL.Path)
		switch {
		case request.Method == http.MethodPut && request.URL.Path == "/search-index":
			if err := json.NewDecoder(request.Body).Decode(&createRequest); err != nil {
				t.Fatalf("decode create index request: %v", err)
			}
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
				if source, ok := item["tenant_id"]; ok {
					if source == "" {
						t.Fatalf("empty tenant_id in bulk item")
					}
					bulkDocuments = append(bulkDocuments, item)
				}
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
		TenantID:          "tenant-1",
		ConversationID:    "conv-1",
		MessageID:         "message-1",
		ConversationSeq:   7,
		SourceEventID:     "event-1",
		SearchableText:    "project launch decision",
		VisibilityVersion: 11,
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
	assertCreateIndexContract(t, createRequest)
	if len(bulkDocuments) != 1 {
		t.Fatalf("bulk documents=%d want 1", len(bulkDocuments))
	}
	if bulkDocuments[0]["source_event_id"] != "event-1" || bulkDocuments[0]["visibility_version"].(float64) != 11 {
		t.Fatalf("bulk document missing source/version fields: %+v", bulkDocuments[0])
	}
}

func TestIndexerVerifiesExistingIndexMapping(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.Method+" "+request.URL.Path)
		switch {
		case request.Method == http.MethodPut && request.URL.Path == "/search-index":
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = writer.Write([]byte(`{"error":{"type":"resource_already_exists_exception"}}`))
		case request.Method == http.MethodGet && request.URL.Path == "/search-index/_mapping":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(validMappingResponse(searchIndexMappingVersion)))
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	indexer, err := NewIndexer(Config{Endpoint: server.URL, Index: "search-index", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("NewIndexer returned error: %v", err)
	}
	if err := indexer.EnsureSearchIndex(context.Background()); err != nil {
		t.Fatalf("EnsureSearchIndex returned error: %v", err)
	}
	want := []string{"PUT /search-index", "GET /search-index/_mapping"}
	if strings.Join(paths, "|") != strings.Join(want, "|") {
		t.Fatalf("paths=%v want %v", paths, want)
	}
}

func TestIndexerFailsClosedOnExistingIndexMappingMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPut && request.URL.Path == "/search-index":
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = writer.Write([]byte(`{"error":{"type":"resource_already_exists_exception"}}`))
		case request.Method == http.MethodGet && request.URL.Path == "/search-index/_mapping":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(validMappingResponse("old-version")))
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	indexer, err := NewIndexer(Config{Endpoint: server.URL, Index: "search-index", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("NewIndexer returned error: %v", err)
	}
	err = indexer.EnsureSearchIndex(context.Background())
	if !errors.Is(err, types.ErrSearchUnavailable) {
		t.Fatalf("err=%v want ErrSearchUnavailable", err)
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
		TenantID:          "tenant-1",
		ConversationID:    "conv-1",
		MessageID:         "message-1",
		ConversationSeq:   7,
		SourceEventID:     "event-1",
		SearchableText:    "project launch decision",
		VisibilityVersion: 11,
	}})
	if !errors.Is(err, types.ErrSearchUnavailable) {
		t.Fatalf("err=%v want ErrSearchUnavailable", err)
	}
}

func assertCreateIndexContract(t *testing.T, request map[string]any) {
	t.Helper()
	mappings, ok := request["mappings"].(map[string]any)
	if !ok {
		t.Fatalf("create request missing mappings: %+v", request)
	}
	if mappings["dynamic"] != "strict" {
		t.Fatalf("dynamic=%v want strict", mappings["dynamic"])
	}
	meta, ok := mappings["_meta"].(map[string]any)
	if !ok || meta["nexusim_mapping_version"] != searchIndexMappingVersion || meta["owner"] != "search-service" {
		t.Fatalf("unexpected mapping meta: %+v", mappings["_meta"])
	}
	properties, ok := mappings["properties"].(map[string]any)
	if !ok {
		t.Fatalf("missing properties: %+v", mappings)
	}
	for field, expectedType := range requiredSearchIndexFieldTypes {
		property, ok := properties[field].(map[string]any)
		if !ok || property["type"] != expectedType {
			t.Fatalf("field %s mapping=%+v want %s", field, properties[field], expectedType)
		}
	}
}

func validMappingResponse(version string) string {
	return `{
		"search-index": {
			"mappings": {
				"dynamic": "strict",
				"_meta": {
					"nexusim_mapping_version": "` + version + `",
					"owner": "search-service",
					"source_projection": "search_message_documents"
				},
				"properties": {
					"tenant_id": {"type": "keyword"},
					"conversation_id": {"type": "keyword"},
					"message_id": {"type": "keyword"},
					"conversation_seq": {"type": "long"},
					"source_event_id": {"type": "keyword"},
					"searchable_text": {"type": "text"},
					"visibility_version": {"type": "long"}
				}
			}
		}
	}`
}
