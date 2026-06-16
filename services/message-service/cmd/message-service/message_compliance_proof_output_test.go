package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/postgres"
)

func TestWriteMessageComplianceProofAuditOutput(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "message-compliance-proof-audit.json")
	rows := []postgresinfra.MessageComplianceExternalProofAuditRow{{
		TenantID:         "tenant-a",
		ExternalProofRef: "proof://case/a",
		Status:           postgresinfra.MessageComplianceExternalProofStatusVerified,
		Provider:         "legal-system",
		ProofHash:        "sha256:abc123",
		VerifiedBy:       "legal-ops",
		VerifiedAt:       time.Date(2026, 6, 17, 1, 0, 0, 0, time.UTC),
		UpdatedAt:        time.Date(2026, 6, 17, 1, 0, 0, 0, time.UTC),
	}}
	if err := writeMessageComplianceProofAuditOutput(outputPath, rows, map[string]string{
		"tenant_id":      "tenant-a",
		"updated_after":  "2026-06-17T00:00:00Z",
		"updated_before": "2026-06-18T00:00:00Z",
		"provider":       "",
	}); err != nil {
		t.Fatalf("write compliance proof audit output: %v", err)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read compliance proof audit output: %v", err)
	}
	if strings.Contains(string(data), "proof body") || strings.Contains(string(data), "secret") {
		t.Fatalf("proof output leaked forbidden text: %s", string(data))
	}
	var output messageComplianceProofOutput
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("decode compliance proof output: %v", err)
	}
	if len(output.Rows) != 1 ||
		output.Filters["tenant_id"] != "tenant-a" ||
		output.Filters["updated_after"] != "2026-06-17T00:00:00Z" ||
		output.Rows[0].ExternalProofRef != "proof://case/a" ||
		output.Rows[0].ProofHash != "sha256:abc123" ||
		output.Rows[0].Status != postgresinfra.MessageComplianceExternalProofStatusVerified {
		t.Fatalf("unexpected compliance proof output: %+v", output)
	}
	if _, ok := output.Filters["provider"]; ok {
		t.Fatalf("empty provider filter should be omitted: %+v", output.Filters)
	}
}

func TestWriteMessageComplianceProofMutationOutput(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "message-compliance-proof-mutation.json")
	revokedAt := time.Date(2026, 6, 17, 2, 0, 0, 0, time.UTC)
	row := postgresinfra.MessageComplianceExternalProofResult{
		TenantID:         "tenant-a",
		ExternalProofRef: "proof://case/a",
		Status:           postgresinfra.MessageComplianceExternalProofStatusRevoked,
		Provider:         "legal-system",
		ProofHash:        "sha256:abc123",
		VerifiedBy:       "legal-ops",
		VerifiedAt:       time.Date(2026, 6, 17, 1, 0, 0, 0, time.UTC),
		RevokedBy:        "legal-ops",
		RevokedAt:        &revokedAt,
		UpdatedAt:        revokedAt,
	}
	if err := writeMessageComplianceProofMutationOutput(outputPath, row); err != nil {
		t.Fatalf("write compliance proof mutation output: %v", err)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read compliance proof mutation output: %v", err)
	}
	var output messageComplianceProofMutationOutput
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("decode compliance proof mutation output: %v", err)
	}
	if output.Row.Status != postgresinfra.MessageComplianceExternalProofStatusRevoked ||
		output.Row.RevokedBy != "legal-ops" ||
		output.Row.RevokedAt == "" {
		t.Fatalf("unexpected compliance proof mutation output: %+v", output)
	}
}
