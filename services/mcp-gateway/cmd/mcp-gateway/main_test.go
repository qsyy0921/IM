package main

import "testing"

func TestValidateMCPGatewayMode(t *testing.T) {
	for _, mode := range []string{"noop", "grpc"} {
		if err := validateMCPGatewayMode(mode); err != nil {
			t.Fatalf("expected mode %s to be valid: %v", mode, err)
		}
	}
	if err := validateMCPGatewayMode("bad"); err == nil {
		t.Fatal("expected invalid mode")
	}
}

func TestValidateMCPGatewayDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	if err := validateMCPGatewayDebugListenerConfig("", false); err != nil {
		t.Fatalf("empty address should be valid: %v", err)
	}
	if err := validateMCPGatewayDebugListenerConfig("127.0.0.1:11924", false); err != nil {
		t.Fatalf("loopback should be valid: %v", err)
	}
	if err := validateMCPGatewayDebugListenerConfig("172.30.80.27:11924", false); err != nil {
		t.Fatalf("private address should be valid: %v", err)
	}
}

func TestValidateMCPGatewayDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	if err := validateMCPGatewayDebugListenerConfig("0.0.0.0:11924", false); err == nil {
		t.Fatal("expected public listener to be rejected")
	}
}

func TestValidateMCPGatewayDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateMCPGatewayDebugListenerConfig("0.0.0.0:11924", true); err != nil {
		t.Fatalf("explicit public listener should be valid: %v", err)
	}
}
