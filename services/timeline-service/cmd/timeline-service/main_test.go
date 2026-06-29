package main

import (
	"context"
	"testing"
)

func TestValidateTimelineMode(t *testing.T) {
	if err := validateTimelineMode("noop"); err != nil {
		t.Fatalf("noop mode should be valid: %v", err)
	}
	if err := validateTimelineMode("seq-block-allocator"); err != nil {
		t.Fatalf("seq-block-allocator mode should be valid: %v", err)
	}
}

func TestRunNoopReturnsWhenContextCanceled(t *testing.T) {
	t.Setenv("NEXUSIM_TIMELINE_SERVICE_MODE", "noop")
	t.Setenv("NEXUSIM_TIMELINE_DEBUG_ADDR", "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := run(ctx); err != nil {
		t.Fatalf("run noop: %v", err)
	}
}

func TestRunSeqBlockAllocatorRequiresDSN(t *testing.T) {
	t.Setenv("NEXUSIM_TIMELINE_SERVICE_MODE", "seq-block-allocator")
	t.Setenv("NEXUSIM_PG_DSN", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := run(ctx); err == nil {
		t.Fatal("seq-block-allocator should require NEXUSIM_PG_DSN")
	}
}

func TestValidateTimelineDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	if err := validateTimelineDebugListenerConfig("", false); err != nil {
		t.Fatalf("empty debug addr should be valid: %v", err)
	}
	if err := validateTimelineDebugListenerConfig("127.0.0.1:11937", false); err != nil {
		t.Fatalf("loopback debug addr should be valid: %v", err)
	}
	if err := validateTimelineDebugListenerConfig("172.30.80.40:11937", false); err != nil {
		t.Fatalf("private debug addr should be valid: %v", err)
	}
}

func TestValidateTimelineDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	if err := validateTimelineDebugListenerConfig("0.0.0.0:11937", false); err == nil {
		t.Fatal("public debug addr should require explicit allow flag")
	}
}

func TestValidateTimelineDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateTimelineDebugListenerConfig("0.0.0.0:11937", true); err != nil {
		t.Fatalf("explicit public debug addr opt-in should be valid: %v", err)
	}
}
