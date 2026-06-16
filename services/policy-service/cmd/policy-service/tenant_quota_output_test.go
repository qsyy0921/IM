package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/policy-service/internal/infrastructure/postgres"
)

func TestWriteTenantQuotaOutputs(t *testing.T) {
	row := postgresinfra.TenantQuotaRow{
		TenantID:          "tenant-a",
		Action:            "SEND",
		MaxDecisions:      10,
		WindowSeconds:     300,
		PermissionVersion: 501,
		Classification:    "TENANT_SEND_QUOTA",
		Reason:            "internal operator reason",
		Enabled:           true,
		Source:            "operator",
		UpdatedAt:         time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC),
	}
	auditPath := filepath.Join(t.TempDir(), "quota", "audit.json")
	if err := writeTenantQuotaAuditOutput(auditPath, []postgresinfra.TenantQuotaRow{row}); err != nil {
		t.Fatalf("write tenant quota audit output: %v", err)
	}
	rawAudit, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read tenant quota audit output: %v", err)
	}
	var auditOutput tenantQuotaOutput
	if err := json.Unmarshal(rawAudit, &auditOutput); err != nil {
		t.Fatalf("decode tenant quota audit output: %v", err)
	}
	if auditOutput.GeneratedAt == "" || len(auditOutput.Rows) != 1 || !auditOutput.Rows[0].ReasonPresent || auditOutput.Rows[0].UpdatedAt == "" {
		t.Fatalf("unexpected tenant quota audit output: %+v", auditOutput)
	}
	if string(rawAudit) == "" || strings.Contains(string(rawAudit), "internal operator reason") {
		t.Fatalf("tenant quota audit output leaked raw reason: %s", rawAudit)
	}

	setPath := filepath.Join(t.TempDir(), "quota", "set.json")
	if err := writeTenantQuotaSetOutput(setPath, row); err != nil {
		t.Fatalf("write tenant quota set output: %v", err)
	}
	rawSet, err := os.ReadFile(setPath)
	if err != nil {
		t.Fatalf("read tenant quota set output: %v", err)
	}
	var setOutput tenantQuotaSetOutput
	if err := json.Unmarshal(rawSet, &setOutput); err != nil {
		t.Fatalf("decode tenant quota set output: %v", err)
	}
	if setOutput.GeneratedAt == "" || setOutput.Row.TenantID != "tenant-a" || !setOutput.Row.ReasonPresent {
		t.Fatalf("unexpected tenant quota set output: %+v", setOutput)
	}
	if strings.Contains(string(rawSet), "internal operator reason") {
		t.Fatalf("tenant quota set output leaked raw reason: %s", rawSet)
	}
}
