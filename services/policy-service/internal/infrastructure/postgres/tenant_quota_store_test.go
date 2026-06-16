package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

func TestTenantQuotaStoreSetAndAuditIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	store := NewTenantQuotaStore(pool)

	row, err := store.SetTenantQuota(ctx, TenantQuotaSetOptions{
		TenantID:          "tenant-policy",
		Action:            "send",
		MaxDecisions:      10,
		WindowSeconds:     300,
		PermissionVersion: 501,
		Classification:    "TENANT_SEND_QUOTA",
		Reason:            "quota exceeded raw reason should not be exported",
		Enabled:           true,
		Source:            "operator",
	})
	if err != nil {
		t.Fatalf("set tenant quota: %v", err)
	}
	if row.TenantID != "tenant-policy" ||
		row.Action != "SEND" ||
		row.MaxDecisions != 10 ||
		row.WindowSeconds != 300 ||
		row.PermissionVersion != 501 ||
		row.Classification != "TENANT_SEND_QUOTA" ||
		row.Reason == "" ||
		!row.Enabled ||
		row.Source != "operator" {
		t.Fatalf("unexpected tenant quota row: %+v", row)
	}

	updated, err := store.SetTenantQuota(ctx, TenantQuotaSetOptions{
		TenantID:          "tenant-policy",
		Action:            "SEND",
		MaxDecisions:      20,
		WindowSeconds:     600,
		PermissionVersion: 502,
		Classification:    "TENANT_SEND_QUOTA_V2",
		Enabled:           false,
		Source:            "operator-update",
	})
	if err != nil {
		t.Fatalf("update tenant quota: %v", err)
	}
	if updated.MaxDecisions != 20 || updated.WindowSeconds != 600 || updated.PermissionVersion != 502 || updated.Classification != "TENANT_SEND_QUOTA_V2" || updated.Enabled || updated.Source != "operator-update" {
		t.Fatalf("unexpected updated tenant quota row: %+v", updated)
	}

	enabled := false
	rows, err := store.AuditTenantQuotas(ctx, TenantQuotaAuditOptions{
		TenantID: "tenant-policy",
		Action:   "send",
		Enabled:  &enabled,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("audit tenant quotas: %v", err)
	}
	if len(rows) != 1 || rows[0].Classification != "TENANT_SEND_QUOTA_V2" || rows[0].Enabled {
		t.Fatalf("unexpected audited tenant quotas: %+v", rows)
	}
}

func TestTenantQuotaStoreRejectsInvalidOptions(t *testing.T) {
	store := NewTenantQuotaStore(openTestPool(t))
	ctx := context.Background()
	if _, err := store.SetTenantQuota(ctx, TenantQuotaSetOptions{}); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid set options, got %v", err)
	}
	if _, err := store.AuditTenantQuotas(ctx, TenantQuotaAuditOptions{Action: "FORWARD"}); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid audit action, got %v", err)
	}
}
