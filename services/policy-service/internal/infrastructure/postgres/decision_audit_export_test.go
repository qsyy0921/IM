package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

func TestOutboxStoreExportDecisionAuditIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	deniedID, _ := insertPolicyAuditOutboxRow(t, ctx, pool, "policy-audit-export-denied", "tenant-policy:conversation-export-denied", types.OutboxStatusPending, 0)
	allowedID, _ := insertPolicyAuditOutboxRow(t, ctx, pool, "policy-audit-export-allowed", "tenant-policy:conversation-export-allowed", types.OutboxStatusPending, 0)
	publishedAt := time.Date(2026, 6, 17, 9, 30, 0, 0, time.UTC)
	createdAt := time.Date(2026, 6, 17, 9, 20, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
UPDATE policy_decision_audit_outbox
SET allowed = false,
    classification = 'CONTENT_PROVIDER_DENIED',
    reason_code = 'CONTENT_PROVIDER_DENIED',
    status = 'PUBLISHED',
    created_at = $3,
    published_at = $2,
    last_error = 'provider body user=user1@example.com token=secret-token'
WHERE id = $1
`, deniedID, publishedAt, createdAt); err != nil {
		t.Fatalf("mark denied export row: %v", err)
	}
	if _, err := pool.Exec(ctx, `
UPDATE policy_decision_audit_outbox
SET created_at = $2
WHERE id = $1
`, allowedID, createdAt.Add(-2*time.Hour)); err != nil {
		t.Fatalf("mark allowed export row created_at: %v", err)
	}

	allowed := false
	createdAfter := createdAt.Add(-time.Minute)
	createdBefore := createdAt.Add(time.Minute)
	rows, err := NewOutboxStore(pool).ExportDecisionAudit(ctx, DecisionAuditExportOptions{
		TenantID:       "tenant-policy",
		Action:         "send",
		Allowed:        &allowed,
		Classification: "CONTENT_PROVIDER_DENIED",
		ReasonCode:     "CONTENT_PROVIDER_DENIED",
		Status:         "published",
		CreatedAfter:   &createdAfter,
		CreatedBefore:  &createdBefore,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("export decision audit: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one exported decision row, got %d", len(rows))
	}
	row := rows[0]
	if row.EventID != "policy-audit-export-denied" ||
		row.TenantID != "tenant-policy" ||
		row.Action != "SEND" ||
		row.Allowed ||
		row.Classification != "CONTENT_PROVIDER_DENIED" ||
		row.ReasonCode != "CONTENT_PROVIDER_DENIED" ||
		row.Status != types.OutboxStatusPublished ||
		row.PublishedAt == nil ||
		!row.MessageIDPresent ||
		!row.DirectPeerContextPresent ||
		row.ActorUserKey == "" ||
		row.ConversationKey == "" ||
		row.PartitionKey == "" {
		t.Fatalf("unexpected exported decision row: %+v", row)
	}
	serialized := row.EventID + row.ActorUserKey + row.DeviceKey + row.ConversationKey + row.MessageKey + row.DirectPeerKey + row.TraceID + row.CorrelationID
	for _, forbidden := range []string{"user1@example.com", "secret-token", "provider body"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("exported decision row leaked sensitive value %q: %+v", forbidden, row)
		}
	}
}

func TestOutboxStoreExportDecisionAuditRejectsInvalidFilters(t *testing.T) {
	store := NewOutboxStore(openTestPool(t))
	if _, err := store.ExportDecisionAudit(context.Background(), DecisionAuditExportOptions{Action: "FORWARD"}); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid action error, got %v", err)
	}
	if _, err := store.ExportDecisionAudit(context.Background(), DecisionAuditExportOptions{Status: "BROKEN"}); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid status error, got %v", err)
	}
	createdAfter := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	createdBefore := createdAfter
	if _, err := store.ExportDecisionAudit(context.Background(), DecisionAuditExportOptions{CreatedAfter: &createdAfter, CreatedBefore: &createdBefore}); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid time window error, got %v", err)
	}
}
