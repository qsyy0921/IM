package main

import "testing"

func TestValidateControlPlaneMode(t *testing.T) {
	for _, mode := range []string{"noop", "grpc"} {
		if err := validateControlPlaneMode(mode); err != nil {
			t.Fatalf("mode %s should be valid: %v", mode, err)
		}
	}
	if err := validateControlPlaneMode("outbox-relay"); err == nil {
		t.Fatal("unsupported mode should fail until implemented")
	}
}

func TestValidateControlPlaneDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	if err := validateControlPlaneDebugListenerConfig("", false); err != nil {
		t.Fatalf("empty debug listener should pass: %v", err)
	}
	if err := validateControlPlaneDebugListenerConfig("127.0.0.1:11930", false); err != nil {
		t.Fatalf("loopback debug listener should pass: %v", err)
	}
	if err := validateControlPlaneDebugListenerConfig("172.30.80.33:11930", false); err != nil {
		t.Fatalf("private debug listener should pass: %v", err)
	}
}

func TestValidateControlPlaneDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	if err := validateControlPlaneDebugListenerConfig("0.0.0.0:11930", false); err == nil {
		t.Fatal("public debug listener should fail without allow flag")
	}
}

func TestValidateControlPlaneDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateControlPlaneDebugListenerConfig("0.0.0.0:11930", true); err != nil {
		t.Fatalf("public debug listener should pass with allow flag: %v", err)
	}
}
