package main

import "testing"

func TestValidateVectorIndexMode(t *testing.T) {
	for _, mode := range []string{"noop", "grpc"} {
		if err := validateVectorIndexMode(mode); err != nil {
			t.Fatalf("mode %s: %v", mode, err)
		}
	}
	if err := validateVectorIndexMode("backend-worker"); err == nil {
		t.Fatal("expected unsupported mode to fail until worker slice is implemented")
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
