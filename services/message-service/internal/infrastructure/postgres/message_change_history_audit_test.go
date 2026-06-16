package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestMessageRepositoryAuditMessageChangeHistoryIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)
	resetMessageCoreTables(t, ctx, pool)

	runID := time.Now().UnixNano()
	tenantID := fmt.Sprintf("tenant-change-history-%d", runID)
	_, err := pool.Exec(ctx, `
INSERT INTO message_change_history (
    tenant_id,
    conversation_id,
    message_id,
    change_version,
    change_type,
    before_payload_json,
    after_payload_json,
    before_status,
    after_status,
    changed_by,
    reason,
    trace_id,
    changed_at
) VALUES
    ($1, 'conv-1', 'msg-1', 1, 'DELETE', '{"text":"secret before"}'::jsonb, '{"deleted":true}'::jsonb, 'NORMAL', 'DELETED', 'user-1', 'private cleanup reason', 'trace-1', now() - interval '1 minute'),
    ($1, 'conv-1', 'msg-2', 1, 'EDIT', '{"text":"old"}'::jsonb, '{"text":"new"}'::jsonb, 'NORMAL', 'EDITED', 'user-2', NULL, 'trace-2', now() - interval '2 minutes')
`, tenantID)
	if err != nil {
		t.Fatalf("seed message change history: %v", err)
	}

	repository := NewMessageRepository(pool)
	rows, err := repository.AuditMessageChangeHistory(ctx, MessageChangeHistoryAuditOptions{
		TenantID:       tenantID,
		ConversationID: "conv-1",
		MessageID:      "msg-1",
		ChangeType:     "delete",
		ChangedBy:      "user-1",
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("audit message change history: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one audit row, got %d", len(rows))
	}
	row := rows[0]
	if row.ChangeType != "DELETE" ||
		row.BeforeStatus != "NORMAL" ||
		row.AfterStatus != "DELETED" ||
		!row.BeforePayloadPresent ||
		!row.AfterPayloadPresent ||
		!row.ReasonPresent {
		t.Fatalf("unexpected audit row: %+v", row)
	}

	_, err = repository.AuditMessageChangeHistory(ctx, MessageChangeHistoryAuditOptions{ChangeType: "PURGE"})
	if err == nil {
		t.Fatalf("expected unsupported change type to fail")
	}
}
