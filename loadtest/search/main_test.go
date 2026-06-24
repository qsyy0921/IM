package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSplitCSV(t *testing.T) {
	got := splitCSV(" a, b ,, a ")
	want := []string{"a", "b", "a"}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d: %#v", len(got), len(want), got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("item %d=%q want %q", index, got[index], want[index])
		}
	}
}

func TestSanitizeRunName(t *testing.T) {
	got := sanitizeRunName("search smoke: 2026/06/19")
	if got != "search-smoke--2026-06-19" {
		t.Fatalf("sanitize=%q", got)
	}
}

func TestPathInside(t *testing.T) {
	if !pathInside(`E:\development\IM\docs`, `E:\development\IM`) {
		t.Fatalf("expected docs under repo")
	}
	if pathInside(`H:\NexusIM\loadtest-results\search`, `E:\development\IM`) {
		t.Fatalf("H drive output must not be treated as repo-local")
	}
}

func TestValidateConfigRequiresOpenSearchEndpointAndIndex(t *testing.T) {
	cfg := config{
		phase:          "smoke",
		searchTarget:   "127.0.0.1:10570",
		kafkaBrokers:   []string{"localhost:9092"},
		topic:          "conversation.timeline.events",
		consumerGroup:  "nexusim-search-test",
		pgDSN:          "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
		searchBackend:  "opensearch",
		requestTimeout: 3 * time.Second,
	}
	if err := validateConfig(cfg); err == nil {
		t.Fatalf("expected missing opensearch endpoint/index to fail")
	}
	cfg.openSearchEndpoint = "http://127.0.0.1:9200"
	cfg.openSearchIndex = "nexusim-search-test"
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("expected opensearch config to pass: %v", err)
	}
}

func TestValidateConfigRejectsUnsafeOpenSearchEndpoint(t *testing.T) {
	cfg := validSearchConfigForTest()
	cfg.searchBackend = "opensearch"
	cfg.openSearchEndpoint = "http://user:pass@127.0.0.1:9200?token=secret"
	cfg.openSearchIndex = "nexusim-search-test"
	if err := validateConfig(cfg); err == nil {
		t.Fatalf("expected endpoint credentials/query to fail")
	}
}

func TestValidateConfigPreflightOpenSearchRequiresOpenSearchBackend(t *testing.T) {
	cfg := validSearchConfigForTest()
	cfg.phase = "preflight-opensearch"
	cfg.searchBackend = "postgres"
	if err := validateConfig(cfg); err == nil {
		t.Fatalf("expected preflight-opensearch with postgres backend to fail")
	}
	cfg.searchBackend = "opensearch"
	cfg.openSearchEndpoint = "http://127.0.0.1:9200"
	cfg.openSearchIndex = "nexusim-search-test"
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("expected preflight-opensearch config to pass: %v", err)
	}
}

func TestPreflightOpenSearchCreatesIndexAndMarksReady(t *testing.T) {
	var rootChecked bool
	var indexCreated bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/":
			rootChecked = true
			writer.WriteHeader(http.StatusOK)
		case request.Method == http.MethodPut && request.URL.Path == "/nexusim-search-test":
			indexCreated = true
			writer.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected opensearch request %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	cfg := validSearchConfigForTest()
	cfg.phase = "preflight-opensearch"
	cfg.searchBackend = "opensearch"
	cfg.openSearchEndpoint = server.URL
	cfg.openSearchIndex = "nexusim-search-test"
	cfg.openSearchHTTPClient = server.Client()
	result := summary{}
	if err := preflightOpenSearch(context.Background(), cfg, &result); err != nil {
		t.Fatalf("preflight failed: %v", err)
	}
	if !rootChecked || !indexCreated || !result.OpenSearchReady {
		t.Fatalf("root=%v index=%v ready=%v", rootChecked, indexCreated, result.OpenSearchReady)
	}
}

func TestPreflightOpenSearchFailsClosedOnEndpointError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cfg := validSearchConfigForTest()
	cfg.phase = "preflight-opensearch"
	cfg.searchBackend = "opensearch"
	cfg.openSearchEndpoint = server.URL
	cfg.openSearchIndex = "nexusim-search-test"
	cfg.openSearchHTTPClient = server.Client()
	result := summary{}
	if err := preflightOpenSearch(context.Background(), cfg, &result); err == nil {
		t.Fatalf("expected preflight to fail on non-2xx endpoint")
	}
	if result.OpenSearchReady {
		t.Fatalf("ready must stay false on failed preflight")
	}
}

func TestOpenSearchURL(t *testing.T) {
	cfg := config{
		openSearchEndpoint: "http://127.0.0.1:9200/root",
		openSearchIndex:    "nexusim-search-test",
	}
	got := openSearchURL(cfg, "/"+cfg.openSearchIndex+"/_refresh")
	want := "http://127.0.0.1:9200/root/nexusim-search-test/_refresh"
	if got != want {
		t.Fatalf("url=%q want %q", got, want)
	}
}

func validSearchConfigForTest() config {
	return config{
		phase:          "smoke",
		searchTarget:   "127.0.0.1:10570",
		kafkaBrokers:   []string{"localhost:9092"},
		topic:          "conversation.timeline.events",
		consumerGroup:  "nexusim-search-test",
		pgDSN:          "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
		searchBackend:  "postgres",
		requestTimeout: 3 * time.Second,
	}
}
