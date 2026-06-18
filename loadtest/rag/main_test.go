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
	if cfg.ragTarget != defaultRAGTarget {
		t.Fatalf("unexpected rag target %q", cfg.ragTarget)
	}
	if cfg.resultRoot != defaultResultRoot {
		t.Fatalf("unexpected result root %q", cfg.resultRoot)
	}
	if cfg.tenantID == "" || cfg.conversationID == "" {
		t.Fatalf("expected generated tenant and conversation ids")
	}
}

func TestParseConfigRejectsMissingRAGTarget(t *testing.T) {
	if _, err := parseConfig([]string{"--rag-target", " "}); err == nil {
		t.Fatalf("expected missing rag target to fail")
	}
}

func TestPathInside(t *testing.T) {
	root := filepath.Join("E:", "development", "IM")
	inside := filepath.Join(root, "loadtest", "rag")
	outside := filepath.Join("H:", "NexusIM", "loadtest-results")
	if !pathInside(inside, root) {
		t.Fatalf("expected %q inside %q", inside, root)
	}
	if pathInside(outside, root) {
		t.Fatalf("did not expect %q inside %q", outside, root)
	}
}
