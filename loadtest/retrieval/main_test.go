package main

import (
	"path/filepath"
	"testing"
)

func TestParseConfigDefaults(t *testing.T) {
	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}
	if cfg.pgDSN != defaultPGDSN {
		t.Fatalf("unexpected pg dsn %q", cfg.pgDSN)
	}
	if cfg.retrievalTarget != defaultRetrievalTarget {
		t.Fatalf("unexpected retrieval target %q", cfg.retrievalTarget)
	}
	if cfg.resultRoot != defaultResultRoot {
		t.Fatalf("unexpected result root %q", cfg.resultRoot)
	}
	if cfg.tenantID == "" || cfg.conversationID == "" {
		t.Fatalf("expected generated tenant and conversation ids")
	}
}

func TestParseConfigRejectsMissingRetrievalTarget(t *testing.T) {
	if _, err := parseConfig([]string{"--retrieval-target", " "}); err == nil {
		t.Fatalf("expected missing retrieval target to fail")
	}
}

func TestParseConfigVectorBackendDefaults(t *testing.T) {
	cfg, err := parseConfig([]string{"--include-vector-backend"})
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}
	if !cfg.includeVectorBackend {
		t.Fatalf("expected vector backend enabled")
	}
	if cfg.vectorTarget != defaultVectorTarget {
		t.Fatalf("unexpected vector target %q", cfg.vectorTarget)
	}
	if cfg.vectorCollectionType != "MEMORY_EVENT" {
		t.Fatalf("unexpected vector collection type %q", cfg.vectorCollectionType)
	}
	if cfg.vectorVisibilityScope == "" || cfg.vectorPolicyVersion == "" {
		t.Fatalf("expected vector visibility and policy fields")
	}
	if cfg.queryEmbeddingRef == "" || cfg.queryEmbeddingRef[:7] != "sha256:" {
		t.Fatalf("expected low-sensitive query embedding ref, got %q", cfg.queryEmbeddingRef)
	}
}

func TestParseConfigRejectsMissingVectorTargetWhenEnabled(t *testing.T) {
	if _, err := parseConfig([]string{"--include-vector-backend", "--vector-target", " "}); err == nil {
		t.Fatalf("expected missing vector target to fail")
	}
}

func TestPathInside(t *testing.T) {
	root := filepath.Join("E:", "development", "IM")
	inside := filepath.Join(root, "loadtest", "retrieval")
	outside := filepath.Join("H:", "NexusIM", "loadtest-results")
	if !pathInside(inside, root) {
		t.Fatalf("expected %q inside %q", inside, root)
	}
	if pathInside(outside, root) {
		t.Fatalf("did not expect %q inside %q", outside, root)
	}
}
