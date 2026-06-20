package main

import (
	"encoding/json"
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

func TestParseFlagsBuildsCreateDefaults(t *testing.T) {
	cfg := parseFlags([]string{
		"-mode", "create",
		"-operation-type", "CONFIG_PUBLISH",
		"-target-ref-hash", "sha256:quota-target",
		"-payload-schema-version", "admin.config_publish.v1",
		"-operation-payload-json", `{"environment":"local","config_kind":"quota","bundle_key":"api-gateway.default","version":"v1","schema_version":"control-plane.quota.v1","payload_json":"{}"}`,
		"-operator-ref", "operator:alice",
	})
	if cfg.mode != "create" {
		t.Fatalf("mode = %q", cfg.mode)
	}
	if cfg.riskLevel != "MEDIUM" {
		t.Fatalf("risk = %q", cfg.riskLevel)
	}
	if cfg.idempotencyKey != "create:CONFIG_PUBLISH:sha256:quota-target:operator:alice" {
		t.Fatalf("idempotency key = %q", cfg.idempotencyKey)
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestValidateRejectsInvalidCreatePayloadJSON(t *testing.T) {
	cfg := config{
		mode:             "create",
		target:           "127.0.0.1:10770",
		tenantID:         "tenant",
		operatorRef:      "operator:alice",
		operatorRole:     "ADMIN",
		operationType:    "CONFIG_PUBLISH",
		targetRefHash:    "sha256:quota-target",
		riskLevel:        "MEDIUM",
		payloadSchema:    "admin.config_publish.v1",
		operationPayload: "{not-json",
		idempotencyKey:   "idem",
		requestTimeout:   1,
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestParseFlagsBuildsConfigPublishSmokeDefaults(t *testing.T) {
	cfg := parseFlags([]string{
		"-mode", "config-publish-smoke",
		"-tenant-id", "tenant-admin-smoke",
		"-run-name", "admin smoke:one",
	})
	if cfg.mode != "config-publish-smoke" {
		t.Fatalf("mode = %q", cfg.mode)
	}
	if cfg.controlPlaneTarget == "" || cfg.pgDSN == "" || cfg.resultRoot == "" {
		t.Fatalf("missing smoke defaults: %+v", cfg)
	}
	if cfg.runName != "admin smoke:one" {
		t.Fatalf("run name = %q", cfg.runName)
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestParseFlagsBuildsConfigRollbackSmokeDefaults(t *testing.T) {
	cfg := parseFlags([]string{
		"-mode", "config-rollback-smoke",
		"-tenant-id", "tenant-admin-smoke",
		"-run-name", "admin rollback smoke",
	})
	if cfg.mode != "config-rollback-smoke" {
		t.Fatalf("mode = %q", cfg.mode)
	}
	if cfg.runName != "admin rollback smoke" {
		t.Fatalf("run name = %q", cfg.runName)
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestParseFlagsBuildsTenantQuotaSmokeDefaults(t *testing.T) {
	cfg := parseFlags([]string{
		"-mode", "tenant-quota-smoke",
		"-tenant-id", "tenant-admin-smoke",
		"-run-name", "admin tenant quota smoke",
	})
	if cfg.mode != "tenant-quota-smoke" {
		t.Fatalf("mode = %q", cfg.mode)
	}
	if cfg.runName != "admin tenant quota smoke" {
		t.Fatalf("run name = %q", cfg.runName)
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestConfigPublishOperationPayloadDoesNotExposeSecretFields(t *testing.T) {
	payload := configPublishOperationPayload("quota-v1")
	var decoded map[string]any
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("payload json: %v", err)
	}
	forbidden := []string{"secret", "token", "password", "private_key", "dsn"}
	for key := range decoded {
		for _, marker := range forbidden {
			if key == marker {
				t.Fatalf("unexpected sensitive key %q", key)
			}
		}
	}
	if decoded["version"] != "quota-v1" {
		t.Fatalf("version = %v", decoded["version"])
	}
	if _, ok := decoded["payload_json"].(string); !ok {
		t.Fatalf("payload_json is not a string: %#v", decoded["payload_json"])
	}
}

func TestConfigRollbackOperationPayloadUsesLowSensitiveTargetVersion(t *testing.T) {
	payload := configRollbackOperationPayload("quota-v1")
	var decoded map[string]any
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("payload json: %v", err)
	}
	if decoded["target_version"] != "quota-v1" {
		t.Fatalf("target_version = %v", decoded["target_version"])
	}
	if _, ok := decoded["payload_json"]; ok {
		t.Fatal("rollback payload should not include payload_json")
	}
}

func TestTenantQuotaOperationPayloadUsesLowSensitiveFields(t *testing.T) {
	payload := tenantQuotaOperationPayload("quota-v1")
	var decoded map[string]any
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("payload json: %v", err)
	}
	if decoded["config_version"] != "quota-v1" ||
		decoded["tenant_ref"] != "tenant-free" ||
		decoded["quota_rps"].(float64) != 20 ||
		decoded["quota_burst"].(float64) != 40 ||
		decoded["effective_at_unix_ms"].(float64) <= 0 {
		t.Fatalf("unexpected payload: %+v", decoded)
	}
	for _, forbidden := range []string{"payload_json", "secret", "token", "password"} {
		if _, ok := decoded[forbidden]; ok {
			t.Fatalf("tenant quota payload leaked %q", forbidden)
		}
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
