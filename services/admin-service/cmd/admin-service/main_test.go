package main

import "testing"

func TestValidateAdminMode(t *testing.T) {
	for _, mode := range []string{"noop", "grpc"} {
		if err := validateAdminMode(mode); err != nil {
			t.Fatalf("mode %s: %v", mode, err)
		}
	}
	if err := validateAdminMode("operation-worker"); err == nil {
		t.Fatal("expected unsupported mode to fail until worker slice is implemented")
	}
}

func TestValidateAdminDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	if err := validateAdminDebugListenerConfig("", false); err != nil {
		t.Fatalf("empty debug listener should be allowed: %v", err)
	}
	if err := validateAdminDebugListenerConfig("127.0.0.1:11936", false); err != nil {
		t.Fatalf("loopback debug listener should be allowed: %v", err)
	}
	if err := validateAdminDebugListenerConfig("172.30.80.39:11936", false); err != nil {
		t.Fatalf("private debug listener should be allowed: %v", err)
	}
}

func TestValidateAdminDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	if err := validateAdminDebugListenerConfig("0.0.0.0:11936", false); err == nil {
		t.Fatal("public debug listener should require explicit override")
	}
}

func TestValidateAdminDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateAdminDebugListenerConfig("0.0.0.0:11936", true); err != nil {
		t.Fatalf("explicit public debug listener opt-in should be allowed: %v", err)
	}
}
