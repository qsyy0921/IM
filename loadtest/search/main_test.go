package main

import "testing"

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
		searchTarget:   "127.0.0.1:10570",
		kafkaBrokers:   []string{"localhost:9092"},
		topic:          "conversation.timeline.events",
		consumerGroup:  "nexusim-search-test",
		pgDSN:          "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
		searchBackend:  "opensearch",
		requestTimeout: 3,
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
