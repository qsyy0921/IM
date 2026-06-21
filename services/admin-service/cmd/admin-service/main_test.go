package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateAdminMode(t *testing.T) {
	for _, mode := range []string{"noop", "grpc", "operation-worker", "outbox-relay", "compensation-request"} {
		if err := validateAdminMode(mode); err != nil {
			t.Fatalf("mode %s: %v", mode, err)
		}
	}
	if err := validateAdminMode("future-worker"); err == nil {
		t.Fatal("expected unsupported mode to fail")
	}
}

func TestValidateAdminDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	if err := validateAdminDebugListenerConfig("", false); err != nil {
		t.Fatalf("empty debug listener should be allowed: %v", err)
	}
	if err := validateAdminDebugListenerConfig("127.0.0.1:11936", false); err != nil {
		t.Fatalf("loopback debug listener should be allowed: %v", err)
	}
	if err := validateAdminDebugListenerConfig("172.30.80.39:11936", false); err != nil {
		t.Fatalf("private debug listener should be allowed: %v", err)
	}
}

func TestValidateAdminDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	if err := validateAdminDebugListenerConfig("0.0.0.0:11936", false); err == nil {
		t.Fatal("public debug listener should require explicit override")
	}
}

func TestValidateAdminDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateAdminDebugListenerConfig("0.0.0.0:11936", true); err != nil {
		t.Fatalf("explicit public debug listener opt-in should be allowed: %v", err)
	}
}

func TestAdminCompensationHelpers(t *testing.T) {
	t.Setenv("NEXUSIM_ADMIN_COMPENSATION_DRY_RUN", "")
	value, err := envBoolDefault("NEXUSIM_ADMIN_COMPENSATION_DRY_RUN", true)
	if err != nil || !value {
		t.Fatalf("default dry-run = %v err=%v", value, err)
	}
	t.Setenv("NEXUSIM_ADMIN_COMPENSATION_DRY_RUN", "false")
	value, err = envBoolDefault("NEXUSIM_ADMIN_COMPENSATION_DRY_RUN", true)
	if err != nil || value {
		t.Fatalf("explicit dry-run = %v err=%v", value, err)
	}
	reasonPath := filepath.Join(t.TempDir(), "reason.txt")
	if err := os.WriteFile(reasonPath, []byte("compensate failed admin operation"), 0o644); err != nil {
		t.Fatalf("write reason: %v", err)
	}
	sha, err := adminReasonFileSHA256(reasonPath)
	if err != nil {
		t.Fatalf("reason sha: %v", err)
	}
	if len(sha) != 64 {
		t.Fatalf("unexpected sha length: %s", sha)
	}
	outputPath := filepath.Join(t.TempDir(), "summary.json")
	if err := writeAdminCompensationSummary(adminCompensationSummary{
		SchemaVersion: adminCompensationSummarySchemaVersion,
		Service:       "admin-service",
		Mode:          "compensation-request",
		TenantID:      "tenant",
		OperationID:   "admop",
		GeneratedAt:   testNow(),
	}, outputPath); err != nil {
		t.Fatalf("write summary: %v", err)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("summary missing: %v", err)
	}
}

func testNow() time.Time {
	return time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
}
