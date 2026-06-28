package main

import (
	"context"
	"testing"
)

func TestValidateTimelineMode(t *testing.T) {
	if err := validateTimelineMode("noop"); err != nil {
		t.Fatalf("noop mode should be valid: %v", err)
	}
	if err := validateTimelineMode("seq-block-allocator"); err == nil {
		t.Fatal("planned sequencer roles must not be accepted before implementation")
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

func TestValidateTimelineDebugListenerConfig(t *testing.T) {
	if err := validateTimelineDebugListenerConfig("127.0.0.1:11937", false); err != nil {
		t.Fatalf("loopback debug addr should be valid: %v", err)
	}
	if err := validateTimelineDebugListenerConfig("0.0.0.0:11937", false); err == nil {
		t.Fatal("public debug addr should require explicit allow flag")
	}
}
