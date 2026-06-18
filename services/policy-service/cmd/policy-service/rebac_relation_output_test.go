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

func TestWriteReBACRelationRuleOutputs(t *testing.T) {
	row := postgresinfra.ReBACRelationRuleRow{
		TenantID:          "tenant-a",
		Action:            "SEND",
		RelationType:      "DIRECT_CONTACT_ACTIVE",
		ConversationScope: "DIRECT",
		PermissionVersion: 701,
		Classification:    "DIRECT_CONTACT_REQUIRED",
		Reason:            "internal operator reason",
		Priority:          10,
		Enabled:           true,
		Source:            "operator",
		UpdatedAt:         time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC),
	}

	auditPath := filepath.Join(t.TempDir(), "rebac", "audit.json")
	if err := writeReBACRelationRuleAuditOutput(auditPath, []postgresinfra.ReBACRelationRuleRow{row}); err != nil {
		t.Fatalf("write rebac relation audit output: %v", err)
	}
	rawAudit, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read rebac relation audit output: %v", err)
	}
	var auditOutput rebacRelationRuleOutput
	if err := json.Unmarshal(rawAudit, &auditOutput); err != nil {
		t.Fatalf("decode rebac relation audit output: %v", err)
	}
	if auditOutput.GeneratedAt == "" ||
		len(auditOutput.Rows) != 1 ||
		auditOutput.Rows[0].RelationType != "DIRECT_CONTACT_ACTIVE" ||
		!auditOutput.Rows[0].ReasonPresent ||
		auditOutput.Rows[0].UpdatedAt == "" {
		t.Fatalf("unexpected rebac relation audit output: %+v", auditOutput)
	}
	if string(rawAudit) == "" || strings.Contains(string(rawAudit), "internal operator reason") {
		t.Fatalf("rebac relation audit output leaked raw reason: %s", rawAudit)
	}

	setPath := filepath.Join(t.TempDir(), "rebac", "set.json")
	if err := writeReBACRelationRuleSetOutput(setPath, row); err != nil {
		t.Fatalf("write rebac relation set output: %v", err)
	}
	rawSet, err := os.ReadFile(setPath)
	if err != nil {
		t.Fatalf("read rebac relation set output: %v", err)
	}
	var setOutput rebacRelationRuleSetOutput
	if err := json.Unmarshal(rawSet, &setOutput); err != nil {
		t.Fatalf("decode rebac relation set output: %v", err)
	}
	if setOutput.GeneratedAt == "" ||
		setOutput.Row.TenantID != "tenant-a" ||
		setOutput.Row.Priority != 10 ||
		!setOutput.Row.ReasonPresent {
		t.Fatalf("unexpected rebac relation set output: %+v", setOutput)
	}
	if strings.Contains(string(rawSet), "internal operator reason") {
		t.Fatalf("rebac relation set output leaked raw reason: %s", rawSet)
	}
}
