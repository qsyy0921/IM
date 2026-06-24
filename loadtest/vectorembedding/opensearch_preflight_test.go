package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateConfigRequiresOpenSearchVectorConfig(t *testing.T) {
	cfg := validTestConfig()
	cfg.phase = "preflight-opensearch-vector"
	cfg.openSearchVectorEndpoint = ""
	cfg.openSearchVectorIndex = "nexusim-vector-items"
	cfg.openSearchVectorField = "embedding_vector"
	cfg.openSearchVectorDimension = 8
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "opensearch-vector-endpoint") {
		t.Fatalf("expected endpoint validation error, got %v", err)
	}

	cfg.openSearchVectorEndpoint = "http://127.0.0.1:9200"
	cfg.openSearchVectorIndex = "NexusIMVector"
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "unsafe opensearch vector index") {
		t.Fatalf("expected unsafe index validation error, got %v", err)
	}

	cfg.openSearchVectorIndex = "nexusim-vector-items"
	cfg.openSearchVectorField = "embedding-vector"
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "unsafe opensearch vector field") {
		t.Fatalf("expected unsafe field validation error, got %v", err)
	}

	cfg.openSearchVectorField = "embedding_vector"
	cfg.openSearchVectorEndpoint = "http://user:pass@127.0.0.1:9200"
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "must not include credentials") {
		t.Fatalf("expected credentials validation error, got %v", err)
	}

	cfg.openSearchVectorEndpoint = "http://127.0.0.1:9200"
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("expected valid OpenSearch vector preflight config: %v", err)
	}
}

func TestPreflightOpenSearchVectorSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/":
			writeJSON(t, w, map[string]any{"cluster_name": "nexusim-test"})
		case r.Method == http.MethodHead && r.URL.Path == "/nexusim-vector-items":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/nexusim-vector-items/_mapping":
			writeJSON(t, w, map[string]any{
				"nexusim-vector-items": map[string]any{
					"mappings": map[string]any{
						"properties": map[string]any{
							"embedding_vector": map[string]any{
								"type":      "knn_vector",
								"dimension": 8,
							},
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := validOpenSearchPreflightConfig(server.URL)
	var result summary
	if err := preflightOpenSearchVector(context.Background(), cfg, &result); err != nil {
		t.Fatalf("preflightOpenSearchVector failed: %v", err)
	}
	if !result.OpenSearchVectorAvailable || !result.OpenSearchVectorIndexExists || !result.OpenSearchVectorMappingVerified {
		t.Fatalf("expected available/index/mapping true: %+v", result)
	}
	if result.OpenSearchVectorFieldType != "knn_vector" || result.OpenSearchVectorDimension != 8 {
		t.Fatalf("unexpected vector field summary: %+v", result)
	}
}

func TestPreflightOpenSearchVectorFailsOnMissingIndex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/":
			writeJSON(t, w, map[string]any{"cluster_name": "nexusim-test"})
		case r.Method == http.MethodHead && r.URL.Path == "/nexusim-vector-items":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := validOpenSearchPreflightConfig(server.URL)
	var result summary
	err := preflightOpenSearchVector(context.Background(), cfg, &result)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected missing index error, got %v", err)
	}
	if !result.OpenSearchVectorAvailable || result.OpenSearchVectorIndexExists {
		t.Fatalf("expected endpoint available and index missing: %+v", result)
	}
}

func TestPreflightOpenSearchVectorFailsOnMappingDrift(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/":
			writeJSON(t, w, map[string]any{"cluster_name": "nexusim-test"})
		case r.Method == http.MethodHead && r.URL.Path == "/nexusim-vector-items":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/nexusim-vector-items/_mapping":
			writeJSON(t, w, map[string]any{
				"nexusim-vector-items": map[string]any{
					"mappings": map[string]any{
						"properties": map[string]any{
							"embedding_vector": map[string]any{
								"type":      "float",
								"dimension": 8,
							},
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := validOpenSearchPreflightConfig(server.URL)
	var result summary
	err := preflightOpenSearchVector(context.Background(), cfg, &result)
	if err == nil || !strings.Contains(err.Error(), "expected knn_vector") {
		t.Fatalf("expected mapping drift error, got %v", err)
	}
	if result.OpenSearchVectorMappingVerified {
		t.Fatalf("mapping drift must not verify: %+v", result)
	}
}

func validOpenSearchPreflightConfig(endpoint string) config {
	cfg := validTestConfig()
	cfg.phase = "preflight-opensearch-vector"
	cfg.openSearchVectorEndpoint = endpoint
	cfg.openSearchVectorIndex = "nexusim-vector-items"
	cfg.openSearchVectorField = "embedding_vector"
	cfg.openSearchVectorDimension = 8
	cfg.requestTimeout = 0
	return cfg
}

func writeJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("write json: %v", err)
	}
}
