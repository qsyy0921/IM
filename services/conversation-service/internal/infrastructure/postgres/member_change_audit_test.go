package postgres

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositoryAuditMemberChangesIntegration(t *testing.T) {
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	defer pool.Close()

	resetConversationTables(t, ctx, pool)
	_, err = pool.Exec(ctx, `
INSERT INTO member_change_saga (
    change_id, tenant_id, conversation_id, user_id, change_type, boundary_seq, status,
    idempotency_key, expected_member_version, command_hash, operator_id, conflict_policy,
    retry_count, last_error, next_retry_at, timeline_event_id, outbox_event_id,
    metadata_json, created_at, updated_at
) VALUES
    (
        'change-audit-1', 'tenant-audit', 'conv-audit', 'target-1', 'JOIN', 10, 'OUTBOX_ENQUEUED',
        'idem-audit-1', NULL, 'hash-audit-1', 'operator-1', 'REJECT',
        2, 'duplicate key value violates unique constraint "secret_constraint" user target@example.com',
        now() + interval '5 minutes', 'timeline-audit-1', 'outbox-audit-1',
        '{}'::jsonb, now() - interval '2 minutes', now() - interval '1 minute'
    ),
    (
        'change-audit-2', 'tenant-audit', 'conv-audit', 'target-2', 'LEAVE', 11, 'DONE',
        'idem-audit-2', NULL, 'hash-audit-2', 'operator-2', 'REJECT',
        0, NULL, NULL, 'timeline-audit-2', 'outbox-audit-2',
        '{}'::jsonb, now() - interval '4 minutes', now() - interval '3 minutes'
    )
`)
	if err != nil {
		t.Fatalf("seed member_change_saga: %v", err)
	}

	repository := NewRepository(pool)
	rows, err := repository.AuditMemberChanges(ctx, MemberChangeAuditOptions{
		TenantID:       "tenant-audit",
		ConversationID: "conv-audit",
		Status:         "outbox_enqueued",
		OutboxEventID:  "outbox-audit-1",
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("audit member changes: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one audit row, got %d", len(rows))
	}
	row := rows[0]
	if row.ChangeID != "change-audit-1" || row.TargetUserID != "target-1" || row.OperatorUserID != "operator-1" {
		t.Fatalf("unexpected audit row: %+v", row)
	}
	if row.Status != "OUTBOX_ENQUEUED" || row.BoundarySeq != 10 || row.RetryCount != 2 {
		t.Fatalf("unexpected status fields: %+v", row)
	}
	if row.TimelineEventID != "timeline-audit-1" || row.OutboxEventID != "outbox-audit-1" {
		t.Fatalf("unexpected event ids: %+v", row)
	}
	if row.NextRetryAt == nil {
		t.Fatalf("expected next retry timestamp")
	}
	if row.LastError != "member change processing failed" {
		t.Fatalf("expected sanitized last_error, got %q", row.LastError)
	}
	if strings.Contains(row.LastError, "constraint") || strings.Contains(row.LastError, "target@example.com") {
		t.Fatalf("last_error leaked raw details: %q", row.LastError)
	}

	if _, err := repository.AuditMemberChanges(ctx, MemberChangeAuditOptions{Status: "not-a-status"}); err == nil {
		t.Fatalf("expected unsupported status to fail")
	}
}
