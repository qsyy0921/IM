package main

import "testing"

func TestValidateNotificationServiceMode(t *testing.T) {
	for _, mode := range []string{"noop", "grpc", "delivery-worker", "outbox-relay"} {
		if err := validateNotificationServiceMode(mode); err != nil {
			t.Fatalf("mode %s should be valid: %v", mode, err)
		}
	}
	if err := validateNotificationServiceMode("provider-worker"); err == nil {
		t.Fatal("unsupported mode should fail")
	}
}

func TestValidateNotificationDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	if err := validateNotificationDebugListenerConfig("", false); err != nil {
		t.Fatalf("empty debug listener should pass: %v", err)
	}
	if err := validateNotificationDebugListenerConfig("127.0.0.1:11928", false); err != nil {
		t.Fatalf("loopback debug listener should pass: %v", err)
	}
	if err := validateNotificationDebugListenerConfig("172.30.80.31:11928", false); err != nil {
		t.Fatalf("private debug listener should pass: %v", err)
	}
}

func TestValidateNotificationDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	if err := validateNotificationDebugListenerConfig("0.0.0.0:11928", false); err == nil {
		t.Fatal("public debug listener should fail without allow flag")
	}
}

func TestValidateNotificationDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateNotificationDebugListenerConfig("0.0.0.0:11928", true); err != nil {
		t.Fatalf("public debug listener should pass with allow flag: %v", err)
	}
}
