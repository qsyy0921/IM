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
	if cfg.agentTarget != defaultAgentTarget {
		t.Fatalf("unexpected agent target %q", cfg.agentTarget)
	}
	if cfg.actionTarget != defaultActionTarget {
		t.Fatalf("unexpected action executor target %q", cfg.actionTarget)
	}
	if cfg.resultRoot != defaultResultRoot {
		t.Fatalf("unexpected result root %q", cfg.resultRoot)
	}
	if cfg.tenantID == "" || cfg.conversationID == "" {
		t.Fatalf("expected generated tenant and conversation ids")
	}
	if cfg.resourceID != cfg.conversationID {
		t.Fatalf("expected default resource id to follow conversation id")
	}
}

func TestParseConfigRejectsMissingAgentTarget(t *testing.T) {
	if _, err := parseConfig([]string{"--agent-target", " "}); err == nil {
		t.Fatalf("expected missing agent target to fail")
	}
}

func TestParseConfigRejectsMissingActionExecutorTarget(t *testing.T) {
	if _, err := parseConfig([]string{"--action-executor-target", " "}); err == nil {
		t.Fatalf("expected missing action executor target to fail")
	}
}

func TestPathInside(t *testing.T) {
	root := filepath.Join("E:", "development", "IM")
	inside := filepath.Join(root, "loadtest", "agent")
	outside := filepath.Join("H:", "NexusIM", "loadtest-results")
	if !pathInside(inside, root) {
		t.Fatalf("expected %q inside %q", inside, root)
	}
	if pathInside(outside, root) {
		t.Fatalf("did not expect %q inside %q", outside, root)
	}
}
