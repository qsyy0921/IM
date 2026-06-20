package main

import "testing"

func TestValidatePresenceMode(t *testing.T) {
	for _, mode := range []string{"noop", "grpc"} {
		if err := validatePresenceMode(mode); err != nil {
			t.Fatalf("mode %s should be valid: %v", mode, err)
		}
	}
	if err := validatePresenceMode("outbox-relay"); err == nil {
		t.Fatal("unsupported mode should fail until implemented")
	}
}

func TestValidatePresenceDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	if err := validatePresenceDebugListenerConfig("", false); err != nil {
		t.Fatalf("empty debug listener should pass: %v", err)
	}
	if err := validatePresenceDebugListenerConfig("127.0.0.1:11931", false); err != nil {
		t.Fatalf("loopback debug listener should pass: %v", err)
	}
	if err := validatePresenceDebugListenerConfig("172.30.80.34:11931", false); err != nil {
		t.Fatalf("private debug listener should pass: %v", err)
	}
}

func TestValidatePresenceDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	if err := validatePresenceDebugListenerConfig("0.0.0.0:11931", false); err == nil {
		t.Fatal("public debug listener should fail without allow flag")
	}
}

func TestValidatePresenceDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validatePresenceDebugListenerConfig("0.0.0.0:11931", true); err != nil {
		t.Fatalf("public debug listener should pass with allow flag: %v", err)
	}
}
