package main

import "testing"

func TestValidateModelGatewayMode(t *testing.T) {
	for _, mode := range []string{"noop", "grpc"} {
		if err := validateModelGatewayMode(mode); err != nil {
			t.Fatalf("mode %s: %v", mode, err)
		}
	}
	if err := validateModelGatewayMode("relay"); err == nil {
		t.Fatal("expected unsupported mode to fail")
	}
}

func TestValidateModelGatewayDebugListenerConfig(t *testing.T) {
	if err := validateModelGatewayDebugListenerConfig("127.0.0.1:11932", false); err != nil {
		t.Fatalf("loopback debug listener should be allowed: %v", err)
	}
	if err := validateModelGatewayDebugListenerConfig("172.30.80.35:11932", false); err != nil {
		t.Fatalf("private debug listener should be allowed: %v", err)
	}
	if err := validateModelGatewayDebugListenerConfig("0.0.0.0:11932", false); err == nil {
		t.Fatal("public debug listener should require explicit override")
	}
}
