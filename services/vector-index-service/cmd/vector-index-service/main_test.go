package main

import "testing"

func TestValidateVectorIndexMode(t *testing.T) {
	for _, mode := range []string{"noop", "grpc", "outbox-relay", "rebuild-worker", "embedding-worker", "embedding-producer", "chunk-consumer"} {
		if err := validateVectorIndexMode(mode); err != nil {
			t.Fatalf("mode %s: %v", mode, err)
		}
	}
	if err := validateVectorIndexMode("backend-worker"); err == nil {
		t.Fatal("expected unsupported mode to fail until worker slice is implemented")
	}
}

func TestEmbeddingTaskSourceModeFromEnv(t *testing.T) {
	t.Setenv("NEXUSIM_VECTOR_EMBEDDING_SOURCE", "")
	t.Setenv("NEXUSIM_KNOWLEDGE_INGESTION_GRPC_ADDR", "")
	if got := embeddingTaskSourceModeFromEnv(); got != "file" {
		t.Fatalf("expected file fallback, got %s", got)
	}
	t.Setenv("NEXUSIM_KNOWLEDGE_INGESTION_GRPC_ADDR", "127.0.0.1:10740")
	if got := embeddingTaskSourceModeFromEnv(); got != "knowledge" {
		t.Fatalf("expected knowledge auto source, got %s", got)
	}
	t.Setenv("NEXUSIM_VECTOR_EMBEDDING_SOURCE", "file")
	if got := embeddingTaskSourceModeFromEnv(); got != "file" {
		t.Fatalf("explicit source should win, got %s", got)
	}
	t.Setenv("NEXUSIM_VECTOR_EMBEDDING_SOURCE", "postgres")
	if got := embeddingTaskSourceModeFromEnv(); got != "postgres" {
		t.Fatalf("explicit postgres source should win, got %s", got)
	}
}

func TestEmbeddingProducerSourceModeFromEnv(t *testing.T) {
	t.Setenv("NEXUSIM_VECTOR_EMBEDDING_PRODUCER_SOURCE", "")
	t.Setenv("NEXUSIM_VECTOR_EMBEDDING_SOURCE", "")
	t.Setenv("NEXUSIM_KNOWLEDGE_INGESTION_GRPC_ADDR", "")
	if got := embeddingProducerSourceModeFromEnv(); got != "file" {
		t.Fatalf("expected file fallback, got %s", got)
	}
	t.Setenv("NEXUSIM_KNOWLEDGE_INGESTION_GRPC_ADDR", "127.0.0.1:10740")
	if got := embeddingProducerSourceModeFromEnv(); got != "knowledge" {
		t.Fatalf("expected knowledge auto source, got %s", got)
	}
	t.Setenv("NEXUSIM_VECTOR_EMBEDDING_SOURCE", "postgres")
	if got := embeddingProducerSourceModeFromEnv(); got != "knowledge" {
		t.Fatalf("worker postgres source should not make producer self-loop, got %s", got)
	}
	t.Setenv("NEXUSIM_VECTOR_EMBEDDING_PRODUCER_SOURCE", "file")
	if got := embeddingProducerSourceModeFromEnv(); got != "file" {
		t.Fatalf("explicit producer source should win, got %s", got)
	}
}

func TestVectorProviderBackendModeFromEnv(t *testing.T) {
	t.Setenv("NEXUSIM_VECTOR_PROVIDER_BACKEND", "")
	if got := vectorProviderBackendModeFromEnv(); got != "" {
		t.Fatalf("expected empty backend mode, got %s", got)
	}
	t.Setenv("NEXUSIM_VECTOR_PROVIDER_BACKEND", "PGVECTOR")
	if got := vectorProviderBackendModeFromEnv(); got != "pgvector" {
		t.Fatalf("expected normalized pgvector mode, got %s", got)
	}
	t.Setenv("NEXUSIM_VECTOR_PROVIDER_BACKEND", "POSTGRES-TEST")
	if got := vectorProviderBackendModeFromEnv(); got != "postgres-test" {
		t.Fatalf("expected normalized postgres-test mode, got %s", got)
	}
}

func TestValidateVectorIndexDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	if err := validateVectorIndexDebugListenerConfig("", false); err != nil {
		t.Fatalf("empty debug listener should be allowed: %v", err)
	}
	if err := validateVectorIndexDebugListenerConfig("127.0.0.1:11935", false); err != nil {
		t.Fatalf("loopback debug listener should be allowed: %v", err)
	}
	if err := validateVectorIndexDebugListenerConfig("172.30.80.38:11935", false); err != nil {
		t.Fatalf("private debug listener should be allowed: %v", err)
	}
}

func TestValidateVectorIndexDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	if err := validateVectorIndexDebugListenerConfig("0.0.0.0:11935", false); err == nil {
		t.Fatal("public debug listener should require explicit override")
	}
}

func TestValidateVectorIndexDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateVectorIndexDebugListenerConfig("0.0.0.0:11935", true); err != nil {
		t.Fatalf("explicit public debug listener opt-in should be allowed: %v", err)
	}
}
