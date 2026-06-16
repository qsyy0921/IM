package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

func TestWriteTenantPrivacyAuditOutput(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "nested", "tenant-privacy-audit.json")
	result := types.GetTenantContactPrivacyDefaultResult{
		TenantID: "tenant-a",
		Settings: types.ContactPrivacySettings{
			AllowContactRequests:       true,
			AllowSearchContactRequests: false,
			Version:                    12,
			PolicySource:               types.ContactPrivacyPolicySourceTenantDefault,
			UpdatedAtUnixMS:            1800000000000,
		},
	}

	if err := writeTenantPrivacyAuditOutput(outputPath, result); err != nil {
		t.Fatalf("writeTenantPrivacyAuditOutput() error = %v", err)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var output tenantPrivacyAuditOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if output.GeneratedAt == "" ||
		output.TenantID != "tenant-a" ||
		!output.AllowContactRequests ||
		output.AllowSearchContactRequests ||
		output.Version != 12 ||
		output.PolicySource != "TENANT_DEFAULT" ||
		output.UpdatedAtUnixMS != 1800000000000 {
		t.Fatalf("unexpected tenant privacy audit output: %+v", output)
	}
}

func TestWriteSourcePolicyAuditOutput(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "nested", "source-policy-audit.json")
	result := types.GetTenantContactRequestSourcePolicyResult{
		TenantID: "tenant-a",
		Policy: types.ContactRequestSourcePolicy{
			SourceType:           types.ContactRequestSourceTypeSearch,
			AllowContactRequests: false,
			Version:              7,
			UpdatedAtUnixMS:      1800000001000,
		},
	}

	if err := writeSourcePolicyAuditOutput(outputPath, result); err != nil {
		t.Fatalf("writeSourcePolicyAuditOutput() error = %v", err)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var output sourcePolicyAuditOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if output.GeneratedAt == "" ||
		output.TenantID != "tenant-a" ||
		output.SourceType != "SEARCH" ||
		output.AllowContactRequests ||
		output.Version != 7 ||
		output.UpdatedAtUnixMS != 1800000001000 {
		t.Fatalf("unexpected source policy audit output: %+v", output)
	}
}
