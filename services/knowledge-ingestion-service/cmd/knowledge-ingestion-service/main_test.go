package main

import "testing"

func TestValidateKnowledgeIngestionMode(t *testing.T) {
	for _, mode := range []string{"noop", "grpc", "outbox-relay"} {
		if err := validateKnowledgeIngestionMode(mode); err != nil {
			t.Fatalf("mode %s: %v", mode, err)
		}
	}
	if err := validateKnowledgeIngestionMode("parser-worker"); err == nil {
		t.Fatal("expected unsupported mode to fail until worker slice is implemented")
	}
}

func TestValidateKnowledgeIngestionDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	if err := validateKnowledgeIngestionDebugListenerConfig("", false); err != nil {
		t.Fatalf("empty debug listener should be allowed: %v", err)
	}
	if err := validateKnowledgeIngestionDebugListenerConfig("127.0.0.1:11933", false); err != nil {
		t.Fatalf("loopback debug listener should be allowed: %v", err)
	}
	if err := validateKnowledgeIngestionDebugListenerConfig("172.30.80.36:11933", false); err != nil {
		t.Fatalf("private debug listener should be allowed: %v", err)
	}
}

func TestValidateKnowledgeIngestionDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	if err := validateKnowledgeIngestionDebugListenerConfig("0.0.0.0:11933", false); err == nil {
		t.Fatal("public debug listener should require explicit override")
	}
}

func TestValidateKnowledgeIngestionDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateKnowledgeIngestionDebugListenerConfig("0.0.0.0:11933", true); err != nil {
		t.Fatalf("explicit public debug listener opt-in should be allowed: %v", err)
	}
}
