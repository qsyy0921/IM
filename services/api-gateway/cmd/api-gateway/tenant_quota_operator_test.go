package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAPIGatewayTenantQuotaOutputs(t *testing.T) {
	row := apiGatewayTenantQuotaRow{
		TenantID:          "tenant-a",
		RequestsPerSecond: 12.5,
		Burst:             16,
		Enabled:           true,
		Source:            "operator",
		UpdatedAt:         time.Date(2026, 6, 18, 8, 0, 0, 0, time.UTC),
	}
	auditPath := filepath.Join(t.TempDir(), "quota", "audit.json")
	if err := writeAPIGatewayTenantQuotaAuditOutput(auditPath, []apiGatewayTenantQuotaRow{row}); err != nil {
		t.Fatalf("write api-gateway tenant quota audit output: %v", err)
	}
	rawAudit, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read api-gateway tenant quota audit output: %v", err)
	}
	var auditOutput apiGatewayTenantQuotaAuditOutput
	if err := json.Unmarshal(rawAudit, &auditOutput); err != nil {
		t.Fatalf("decode api-gateway tenant quota audit output: %v", err)
	}
	if auditOutput.GeneratedAt == "" || len(auditOutput.Rows) != 1 ||
		auditOutput.Rows[0].TenantID != "tenant-a" ||
		auditOutput.Rows[0].RequestsPerSecond != 12.5 ||
		auditOutput.Rows[0].UpdatedAt == "" {
		t.Fatalf("unexpected api-gateway tenant quota audit output: %+v", auditOutput)
	}

	setPath := filepath.Join(t.TempDir(), "quota", "set.json")
	if err := writeAPIGatewayTenantQuotaSetOutput(setPath, row, true); err != nil {
		t.Fatalf("write api-gateway tenant quota set output: %v", err)
	}
	rawSet, err := os.ReadFile(setPath)
	if err != nil {
		t.Fatalf("read api-gateway tenant quota set output: %v", err)
	}
	var setOutput apiGatewayTenantQuotaSetOutput
	if err := json.Unmarshal(rawSet, &setOutput); err != nil {
		t.Fatalf("decode api-gateway tenant quota set output: %v", err)
	}
	if setOutput.GeneratedAt == "" || !setOutput.DryRun || setOutput.Row.Burst != 16 {
		t.Fatalf("unexpected api-gateway tenant quota set output: %+v", setOutput)
	}
	for _, leaked := range []string{"password", "secret", "bearer"} {
		if strings.Contains(strings.ToLower(string(rawSet)), leaked) || strings.Contains(strings.ToLower(string(rawAudit)), leaked) {
			t.Fatalf("api-gateway tenant quota output leaked sensitive marker %q", leaked)
		}
	}
}

func TestAPIGatewayTenantQuotaValidation(t *testing.T) {
	if err := validateAPIGatewayTenantID("tenant a"); err == nil {
		t.Fatalf("expected tenant id with whitespace to fail")
	}
	if err := validateAPIGatewayTenantQuotaSetOptions(apiGatewayTenantQuotaSetOptions{
		TenantID:          "tenant-a",
		RequestsPerSecond: 1,
		Burst:             2,
		Source:            "operator",
	}); err != nil {
		t.Fatalf("expected valid quota set options: %v", err)
	}
	if err := validateAPIGatewayTenantQuotaSetOptions(apiGatewayTenantQuotaSetOptions{
		TenantID:          "tenant-a",
		RequestsPerSecond: 0,
		Burst:             2,
		Source:            "operator",
	}); err == nil {
		t.Fatalf("expected non-positive rps to fail")
	}
}

func TestAPIGatewayTenantQuotaSetAndAuditIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openAPIGatewayTestPool(t)
	resetAPIGatewayTenantPlanTables(t, ctx, pool)

	row, err := setAPIGatewayTenantQuota(ctx, pool, apiGatewayTenantQuotaSetOptions{
		TenantID:          "tenant-operator-a",
		RequestsPerSecond: 11.5,
		Burst:             12,
		Enabled:           true,
		Source:            "operator",
	})
	if err != nil {
		t.Fatalf("set api-gateway tenant quota: %v", err)
	}
	if row.TenantID != "tenant-operator-a" || row.RequestsPerSecond != 11.5 || row.Burst != 12 || !row.Enabled || row.Source != "operator" {
		t.Fatalf("unexpected tenant quota set row: %+v", row)
	}

	rows, err := auditAPIGatewayTenantQuotas(ctx, pool, apiGatewayTenantQuotaAuditOptions{
		TenantID: "tenant-operator-a",
		Limit:    20,
	})
	if err != nil {
		t.Fatalf("audit api-gateway tenant quotas: %v", err)
	}
	if len(rows) != 1 || rows[0].TenantID != "tenant-operator-a" || rows[0].Burst != 12 {
		t.Fatalf("unexpected tenant quota audit rows: %+v", rows)
	}

	snapshot, err := tenantRateLimitPlansFromDBPool(ctx, pool, time.Hour, true, true)
	if err != nil {
		t.Fatalf("load api-gateway tenant plans from DB after operator set: %v", err)
	}
	if plan := snapshot.Plans["tenant-operator-a"]; plan.RequestsPerSecond != 11.5 || plan.Burst != 12 {
		t.Fatalf("operator-set row was not visible to DB tenant plan source: %+v", snapshot.Plans)
	}

	if _, err := setAPIGatewayTenantQuota(ctx, pool, apiGatewayTenantQuotaSetOptions{
		TenantID:          "tenant-operator-a",
		RequestsPerSecond: 3,
		Burst:             4,
		Enabled:           false,
		Source:            "operator-update",
	}); err != nil {
		t.Fatalf("update api-gateway tenant quota: %v", err)
	}
	rows, err = auditAPIGatewayTenantQuotas(ctx, pool, apiGatewayTenantQuotaAuditOptions{
		TenantID: "tenant-operator-a",
		Limit:    20,
	})
	if err != nil {
		t.Fatalf("audit updated api-gateway tenant quota: %v", err)
	}
	if len(rows) != 1 || rows[0].Enabled || rows[0].Source != "operator-update" || rows[0].RequestsPerSecond != 3 {
		t.Fatalf("unexpected updated tenant quota audit rows: %+v", rows)
	}
}
