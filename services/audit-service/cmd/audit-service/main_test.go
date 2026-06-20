package main

import "testing"

func TestValidateAuditServiceMode(t *testing.T) {
	for _, mode := range []string{"noop", "grpc"} {
		if err := validateAuditServiceMode(mode); err != nil {
			t.Fatalf("mode %s should be valid: %v", mode, err)
		}
	}
	if err := validateAuditServiceMode("outbox-relay"); err == nil {
		t.Fatal("unsupported mode should fail")
	}
}

func TestValidateAuditDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	if err := validateAuditDebugListenerConfig("", false); err != nil {
		t.Fatalf("empty debug listener should pass: %v", err)
	}
	if err := validateAuditDebugListenerConfig("127.0.0.1:11929", false); err != nil {
		t.Fatalf("loopback debug listener should pass: %v", err)
	}
	if err := validateAuditDebugListenerConfig("172.30.80.32:11929", false); err != nil {
		t.Fatalf("private debug listener should pass: %v", err)
	}
}

func TestValidateAuditDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	if err := validateAuditDebugListenerConfig("0.0.0.0:11929", false); err == nil {
		t.Fatal("public debug listener should fail without allow flag")
	}
}

func TestValidateAuditDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateAuditDebugListenerConfig("0.0.0.0:11929", true); err != nil {
		t.Fatalf("public debug listener should pass with allow flag: %v", err)
	}
}
