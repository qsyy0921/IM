package main

import (
	"testing"

	adminv1 "github.com/qsyy0921/IM/api/proto/nexusim/admin/v1"
)

func TestSplitCSVTrimsDeduplicatesAndSkipsEmpty(t *testing.T) {
	got := splitCSV(" evidence:one, evidence:one, ,evidence:two ")
	want := []string{"evidence:one", "evidence:two"}
	if len(got) != len(want) {
		t.Fatalf("unexpected length got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected item %d got=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestParseFlagsBuildsApprovalDefaults(t *testing.T) {
	cfg := parseFlags([]string{
		"-mode", "approve",
		"-operation-id", "admop_1",
		"-approver-ref", "operator:bob",
	})
	if cfg.mode != "approve" {
		t.Fatalf("mode = %q", cfg.mode)
	}
	if cfg.decision != "APPROVE" {
		t.Fatalf("decision = %q", cfg.decision)
	}
	if cfg.idempotencyKey != "approve:admop_1:operator:bob" {
		t.Fatalf("idempotency key = %q", cfg.idempotencyKey)
	}
	if cfg.requestID != "admin-operator-approve" || cfg.traceID != cfg.requestID {
		t.Fatalf("unexpected request/trace ids: %+v", cfg)
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestValidateRejectsMissingOperationForApproval(t *testing.T) {
	cfg := config{
		mode:           "approve",
		target:         "127.0.0.1:10770",
		tenantID:       "tenant",
		approverRef:    "operator:bob",
		idempotencyKey: "idem",
		requestTimeout: 1,
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestSummarizeOperationOmitsPayloadBody(t *testing.T) {
	summary := summarizeOperation(&adminv1.AdminOperation{
		OperationId:          "admop_1",
		OperationType:        "CONFIG_PUBLISH",
		TargetRefHash:        "sha256:target",
		RiskLevel:            "CRITICAL",
		PayloadSchemaVersion: "admin.config_publish.v1",
		PayloadHash:          "sha256:payload",
		Status:               "APPROVED",
	})
	if summary == nil {
		t.Fatal("summary is nil")
	}
	if summary.PayloadHash != "sha256:payload" || summary.PayloadSchemaVersion == "" {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}
