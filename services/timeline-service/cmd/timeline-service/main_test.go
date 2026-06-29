package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateTimelineMode(t *testing.T) {
	validModes := []string{
		"noop",
		"seq-block-allocator",
		"seq-lease-expire",
		"gap-marker-create",
		"gap-marker-close",
		"gap-marker-audit",
	}
	for _, mode := range validModes {
		if err := validateTimelineMode(mode); err != nil {
			t.Fatalf("%s mode should be valid: %v", mode, err)
		}
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

func TestTimelineRepairOperatorIDRequiresExplicitEnv(t *testing.T) {
	t.Setenv("NEXUSIM_TIMELINE_REPAIR_OPERATOR_ID", "")
	if _, err := timelineRepairOperatorID(); err == nil {
		t.Fatal("timeline repair operator id should be required")
	}
	t.Setenv("NEXUSIM_TIMELINE_REPAIR_OPERATOR_ID", "operator-a")
	operatorID, err := timelineRepairOperatorID()
	if err != nil {
		t.Fatalf("timeline repair operator id: %v", err)
	}
	if operatorID != "operator-a" {
		t.Fatalf("operator id = %q", operatorID)
	}
}

func TestTimelineRepairReasonRequiresExplicitFile(t *testing.T) {
	t.Setenv("NEXUSIM_TIMELINE_REPAIR_REASON_FILE", "")
	if _, err := timelineRepairReason(); err == nil {
		t.Fatal("timeline repair reason file should be required")
	}
	path := filepath.Join(t.TempDir(), "reason.txt")
	if err := os.WriteFile(path, []byte("repair reason\n"), 0o644); err != nil {
		t.Fatalf("write reason: %v", err)
	}
	t.Setenv("NEXUSIM_TIMELINE_REPAIR_REASON_FILE", path)
	reason, err := timelineRepairReason()
	if err != nil {
		t.Fatalf("timeline repair reason: %v", err)
	}
	if reason != "repair reason" {
		t.Fatalf("reason = %q", reason)
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
