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
	if cfg.tenantID == "" {
		t.Fatalf("expected generated tenant id")
	}
}

func TestParseConfigRejectsMissingRetrievalTarget(t *testing.T) {
	if _, err := parseConfig([]string{"--retrieval-target", " "}); err == nil {
		t.Fatalf("expected missing retrieval target to fail")
	}
}

func TestPathInside(t *testing.T) {
	root := filepath.Join("E:", "development", "IM")
	inside := filepath.Join(root, "loadtest", "retrievalnegative")
	outside := filepath.Join("H:", "NexusIM", "loadtest-results")
	if !pathInside(inside, root) {
		t.Fatalf("expected %q inside %q", inside, root)
	}
	if pathInside(outside, root) {
		t.Fatalf("did not expect %q inside %q", outside, root)
	}
}
