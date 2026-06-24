package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateConfigRequiresMilvusVectorConfig(t *testing.T) {
	cfg := validTestConfig()
	cfg.phase = "preflight-milvus-vector"
	cfg.milvusEndpoint = ""
	cfg.milvusDatabase = "_default"
	cfg.milvusCollection = "nexusim_vector_items"
	cfg.milvusVectorField = "embedding_vector"
	cfg.milvusVectorDimension = 8
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "milvus-endpoint") {
		t.Fatalf("expected endpoint validation error, got %v", err)
	}

	cfg.milvusEndpoint = "http://root:Milvus@127.0.0.1:19530"
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "must not include credentials") {
		t.Fatalf("expected credentials validation error, got %v", err)
	}

	cfg.milvusEndpoint = "http://127.0.0.1:19530"
	cfg.milvusCollection = "nexusim-vector-items"
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "unsafe milvus collection") {
		t.Fatalf("expected unsafe collection validation error, got %v", err)
	}

	cfg.milvusCollection = "nexusim_vector_items"
	cfg.milvusVectorField = "embedding.vector"
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "unsafe milvus vector field") {
		t.Fatalf("expected unsafe field validation error, got %v", err)
	}

	cfg.milvusVectorField = "embedding_vector"
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("expected valid Milvus vector preflight config: %v", err)
	}
}

func TestPreflightMilvusVectorSuccess(t *testing.T) {
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
	cfg.milvusToken = "root:Milvus"
	var result summary
	if err := preflightMilvusVector(context.Background(), cfg, &result); err != nil {
		t.Fatalf("preflightMilvusVector failed: %v", err)
	}
	if !result.MilvusAvailable || !result.MilvusCollectionExists || !result.MilvusSchemaVerified {
		t.Fatalf("expected available/collection/schema true: %+v", result)
	}
	if result.MilvusVectorFieldType != "FloatVector" || result.MilvusVectorDimension != 8 {
		t.Fatalf("unexpected vector field summary: %+v", result)
	}
}

func TestPreflightMilvusVectorFailsOnMissingCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMilvusRequest(t, r)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/vectordb/collections/has":
			writeJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"has": false}})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := validMilvusPreflightConfig(server.URL)
	var result summary
	err := preflightMilvusVector(context.Background(), cfg, &result)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected missing collection error, got %v", err)
	}
	if !result.MilvusAvailable || result.MilvusCollectionExists {
		t.Fatalf("expected endpoint available and collection missing: %+v", result)
	}
}

func TestPreflightMilvusVectorFailsOnSchemaDrift(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMilvusRequest(t, r)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/vectordb/collections/has":
			writeJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"has": true}})
		case r.Method == http.MethodPost && r.URL.Path == "/v2/vectordb/collections/describe":
			writeJSON(t, w, milvusDescribePayload("FloatVector", "16"))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := validMilvusPreflightConfig(server.URL)
	var result summary
	err := preflightMilvusVector(context.Background(), cfg, &result)
	if err == nil || !strings.Contains(err.Error(), "dimension=16, expected 8") {
		t.Fatalf("expected schema drift error, got %v", err)
	}
	if result.MilvusSchemaVerified {
		t.Fatalf("schema drift must not verify: %+v", result)
	}
}

func validMilvusPreflightConfig(endpoint string) config {
	cfg := validTestConfig()
	cfg.phase = "preflight-milvus-vector"
	cfg.milvusEndpoint = endpoint
	cfg.milvusDatabase = "_default"
	cfg.milvusCollection = "nexusim_vector_items"
	cfg.milvusVectorField = "embedding_vector"
	cfg.milvusVectorDimension = 8
	cfg.requestTimeout = 0
	return cfg
}

func assertMilvusRequest(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("expected json content type, got %q", r.Header.Get("Content-Type"))
	}
	if strings.Contains(r.URL.RawQuery, "token") {
		t.Fatalf("milvus request must not carry token in query: %s", r.URL.String())
	}
}

func milvusDescribePayload(fieldType string, dimension string) map[string]any {
	return map[string]any{
		"code": 0,
		"data": map[string]any{
			"fields": []map[string]any{
				{
					"name": "id",
					"type": "VarChar",
				},
				{
					"name": "embedding_vector",
					"type": fieldType,
					"params": []map[string]any{
						{"key": "dim", "value": dimension},
					},
				},
			},
		},
	}
}
