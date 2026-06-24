package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseProviderReadiness(t *testing.T) {
	providers, err := parseProviderReadiness(" pgvector,opensearch-vector,milvus,pgvector ")
	if err != nil {
		t.Fatalf("parseProviderReadiness returned error: %v", err)
	}
	if len(providers) != 3 ||
		providers[0] != providerReadinessPGVector ||
		providers[1] != providerReadinessOpenSearchVector ||
		providers[2] != providerReadinessMilvus {
		t.Fatalf("unexpected providers: %+v", providers)
	}
	if _, err := parseProviderReadiness("unknown-vector"); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported provider error, got %v", err)
	}
}

func TestValidateConfigProviderReadinessRequiresSelectedProviderConfig(t *testing.T) {
	cfg := validTestConfig()
	cfg.phase = "preflight-provider-readiness"
	cfg.providerReadiness = "opensearch-vector"
	cfg.openSearchVectorEndpoint = ""
	cfg.openSearchVectorIndex = "nexusim-vector-items"
	cfg.openSearchVectorField = "embedding_vector"
	cfg.openSearchVectorDimension = 8
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "opensearch-vector-endpoint") {
		t.Fatalf("expected opensearch endpoint error, got %v", err)
	}

	cfg.providerReadiness = "pgvector"
	cfg.openSearchVectorEndpoint = ""
	cfg.pgVectorDSN = ""
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "pgvector-dsn") {
		t.Fatalf("expected pgvector dsn error, got %v", err)
	}

	cfg.providerReadiness = "milvus"
	cfg.pgVectorDSN = ""
	cfg.milvusEndpoint = ""
	cfg.milvusDatabase = "_default"
	cfg.milvusCollection = "nexusim_vector_items"
	cfg.milvusVectorField = "embedding_vector"
	cfg.milvusVectorDimension = 8
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "milvus-endpoint") {
		t.Fatalf("expected milvus endpoint error, got %v", err)
	}
}

func TestPreflightProviderReadinessOpenSearchSuccess(t *testing.T) {
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
	cfg.phase = "preflight-provider-readiness"
	cfg.providerReadiness = "opensearch-vector"
	var result summary
	if err := preflightProviderReadiness(context.Background(), cfg, &result); err != nil {
		t.Fatalf("preflightProviderReadiness returned error: %v", err)
	}
	if len(result.ProviderReadiness) != 1 {
		t.Fatalf("expected one provider readiness entry: %+v", result.ProviderReadiness)
	}
	entry := result.ProviderReadiness[0]
	if entry.Provider != providerReadinessOpenSearchVector || entry.Status != providerReadinessReady || !entry.Available || !entry.Configured {
		t.Fatalf("unexpected readiness entry: %+v", entry)
	}
}

func TestPreflightProviderReadinessOpenSearchFailureStillWritesEntry(t *testing.T) {
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
	cfg.phase = "preflight-provider-readiness"
	cfg.providerReadiness = "opensearch-vector"
	var result summary
	err := preflightProviderReadiness(context.Background(), cfg, &result)
	if err == nil || !strings.Contains(err.Error(), "vector provider readiness failed") {
		t.Fatalf("expected matrix failure, got %v", err)
	}
	if len(result.ProviderReadiness) != 1 {
		t.Fatalf("expected one provider readiness entry: %+v", result.ProviderReadiness)
	}
	entry := result.ProviderReadiness[0]
	if entry.Status != providerReadinessFailed || entry.Error == "" || entry.Available {
		t.Fatalf("unexpected failed readiness entry: %+v", entry)
	}
}

func TestPreflightProviderReadinessMilvusSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMilvusRequest(t, r)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/vectordb/collections/has":
			writeJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"has": true}})
		case r.Method == http.MethodPost && r.URL.Path == "/v2/vectordb/collections/describe":
			writeJSON(t, w, milvusDescribePayload("FloatVector", "8"))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := validMilvusPreflightConfig(server.URL)
	cfg.phase = "preflight-provider-readiness"
	cfg.providerReadiness = "milvus"
	var result summary
	if err := preflightProviderReadiness(context.Background(), cfg, &result); err != nil {
		t.Fatalf("preflightProviderReadiness returned error: %v", err)
	}
	if len(result.ProviderReadiness) != 1 {
		t.Fatalf("expected one provider readiness entry: %+v", result.ProviderReadiness)
	}
	entry := result.ProviderReadiness[0]
	if entry.Provider != providerReadinessMilvus || entry.Status != providerReadinessReady || !entry.Available || !entry.Configured {
		t.Fatalf("unexpected readiness entry: %+v", entry)
	}
}
