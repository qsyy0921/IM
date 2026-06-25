package main

import "testing"

func TestValidateWorkflowMode(t *testing.T) {
	for _, mode := range []string{
		"noop",
		"grpc",
		"timer-worker",
		"compensation-worker",
		"compensation-executor",
		"compensation-instruction-import",
		"external-callback-delivery-import",
		"external-callback-delivery-worker",
	} {
		if err := validateWorkflowMode(mode); err != nil {
			t.Fatalf("mode %s: %v", mode, err)
		}
	}
	if err := validateWorkflowMode("bad-mode"); err == nil {
		t.Fatal("expected unsupported mode to fail")
	}
}

func TestValidateWorkflowDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	if err := validateWorkflowDebugListenerConfig("", false); err != nil {
		t.Fatalf("empty debug listener should be allowed: %v", err)
	}
	if err := validateWorkflowDebugListenerConfig("127.0.0.1:11934", false); err != nil {
		t.Fatalf("loopback debug listener should be allowed: %v", err)
	}
	if err := validateWorkflowDebugListenerConfig("172.30.80.37:11934", false); err != nil {
		t.Fatalf("private debug listener should be allowed: %v", err)
	}
}

func TestValidateWorkflowDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	if err := validateWorkflowDebugListenerConfig("0.0.0.0:11934", false); err == nil {
		t.Fatal("public debug listener should require explicit override")
	}
}

func TestValidateWorkflowDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateWorkflowDebugListenerConfig("0.0.0.0:11934", true); err != nil {
		t.Fatalf("explicit public debug listener opt-in should be allowed: %v", err)
	}
}
