package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

func TestReBACRelationRuleStoreSetAndAuditIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	store := NewReBACRelationRuleStore(pool)

	row, err := store.SetReBACRelationRule(ctx, ReBACRelationRuleSetOptions{
		TenantID:          "tenant-policy",
		Action:            "send",
		RelationType:      "direct_contact_active",
		ConversationScope: "direct",
		PermissionVersion: 701,
		Classification:    "DIRECT_CONTACT_REQUIRED",
		Reason:            "raw operator reason should not be exported",
		Priority:          10,
		Enabled:           true,
		Source:            "operator",
	})
	if err != nil {
		t.Fatalf("set rebac relation rule: %v", err)
	}
	if row.TenantID != "tenant-policy" ||
		row.Action != "SEND" ||
		row.RelationType != string(types.ReBACRelationDirectContactActive) ||
		row.ConversationScope != string(types.ReBACConversationScopeDirect) ||
		row.PermissionVersion != 701 ||
		row.Classification != "DIRECT_CONTACT_REQUIRED" ||
		row.Reason == "" ||
		row.Priority != 10 ||
		!row.Enabled ||
		row.Source != "operator" {
		t.Fatalf("unexpected rebac relation row: %+v", row)
	}

	updated, err := store.SetReBACRelationRule(ctx, ReBACRelationRuleSetOptions{
		TenantID:          "tenant-policy",
		Action:            "SEND",
		RelationType:      string(types.ReBACRelationDirectContactActive),
		ConversationScope: string(types.ReBACConversationScopeDirect),
		PermissionVersion: 702,
		Classification:    "DIRECT_CONTACT_REQUIRED_V2",
		Priority:          20,
		Enabled:           false,
		Source:            "operator-update",
	})
	if err != nil {
		t.Fatalf("update rebac relation rule: %v", err)
	}
	if updated.PermissionVersion != 702 ||
		updated.Classification != "DIRECT_CONTACT_REQUIRED_V2" ||
		updated.Priority != 20 ||
		updated.Enabled ||
		updated.Source != "operator-update" {
		t.Fatalf("unexpected updated rebac relation row: %+v", updated)
	}

	enabled := false
	rows, err := store.AuditReBACRelationRules(ctx, ReBACRelationRuleAuditOptions{
		TenantID:          "tenant-policy",
		Action:            "send",
		RelationType:      "DIRECT_CONTACT_ACTIVE",
		ConversationScope: "direct",
		Enabled:           &enabled,
		Limit:             10,
	})
	if err != nil {
		t.Fatalf("audit rebac relation rules: %v", err)
	}
	if len(rows) != 1 || rows[0].Classification != "DIRECT_CONTACT_REQUIRED_V2" || rows[0].Enabled {
		t.Fatalf("unexpected audited rebac relation rules: %+v", rows)
	}
}

func TestReBACRelationRuleStoreRejectsInvalidOptions(t *testing.T) {
	store := NewReBACRelationRuleStore(openTestPool(t))
	ctx := context.Background()
	if _, err := store.SetReBACRelationRule(ctx, ReBACRelationRuleSetOptions{}); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid set options, got %v", err)
	}
	if _, err := store.AuditReBACRelationRules(ctx, ReBACRelationRuleAuditOptions{Action: "FORWARD"}); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid audit action, got %v", err)
	}
	if _, err := store.AuditReBACRelationRules(ctx, ReBACRelationRuleAuditOptions{RelationType: "FRIEND"}); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid audit relation type, got %v", err)
	}
	if _, err := store.AuditReBACRelationRules(ctx, ReBACRelationRuleAuditOptions{ConversationScope: "THREAD"}); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid audit conversation scope, got %v", err)
	}
}
