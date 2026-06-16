package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestMessageRepositoryAuditMessageRetentionProofIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)
	resetMessageCoreTables(t, ctx, pool)

	runID := time.Now().UnixNano()
	tenantID := fmt.Sprintf("tenant-retention-proof-%d", runID)
	_, err := pool.Exec(ctx, `
INSERT INTO message_log (
    tenant_id,
    conversation_id,
    conversation_seq,
    message_id,
    sender_id,
    device_id,
    client_msg_id,
    command_hash,
    message_type,
    payload_json,
    status,
    permission_version,
    classification,
    created_at,
    deleted_at
) VALUES
    ($1, 'conv-1', 7, 'msg-deleted', 'user-1', 'device-1', 'client-1', 'hash-1', 'TEXT', '{"text":"secret current"}'::jsonb, 'DELETED', 1, 'PRIVATE', now() - interval '2 minutes', now() - interval '1 minute'),
    ($1, 'conv-1', 8, 'msg-normal', 'user-1', 'device-1', 'client-2', 'hash-2', 'TEXT', '{"text":"still visible"}'::jsonb, 'NORMAL', 1, 'PRIVATE', now(), NULL);
`, tenantID)
	if err != nil {
		t.Fatalf("seed retention proof message rows: %v", err)
	}
	_, err = pool.Exec(ctx, `
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
    ($1, 'conv-1', 'msg-deleted', 1, 'DELETE', '{"text":"secret before"}'::jsonb, '{"deleted":true}'::jsonb, 'NORMAL', 'DELETED', 'user-1', 'private retention reason', 'trace-delete', now() - interval '1 minute');
`, tenantID)
	if err != nil {
		t.Fatalf("seed retention proof history rows: %v", err)
	}
	_, err = pool.Exec(ctx, `
INSERT INTO conversation_timeline_events (
    tenant_id,
    conversation_id,
    seq,
    event_id,
    event_type,
    event_version,
    message_id,
    actor_id,
    fanout_mode,
    fanout_policy_version,
    permission_version,
    classification,
    mapping_version,
    trace_id,
    payload_json
) VALUES
    ($1, 'conv-1', 7, 'timeline-delete', 'message.deleted.v1', 'v1', 'msg-deleted', 'user-1', 'ALL', 1, 1, 'PRIVATE', 'message.deleted.v1', 'trace-delete', '{"deleted":true}'::jsonb);
`, tenantID)
	if err != nil {
		t.Fatalf("seed retention proof timeline rows: %v", err)
	}
	_, err = pool.Exec(ctx, `
INSERT INTO message_outbox (
    event_id,
    tenant_id,
    conversation_id,
    aggregate_version,
    event_type,
    event_version,
    partition_key,
    mapping_version,
    correlation_id,
    causation_id,
    payload_json,
    trace_id,
    status
) VALUES
    ('outbox-delete', $1, 'conv-1', 7, 'message.deleted.v1', 'v1', 'conv-1', 'message.deleted.v1', 'req-1', 'delete-1', '{"deleted":true}'::jsonb, 'trace-delete', 'PUBLISHED')
`, tenantID)
	if err != nil {
		t.Fatalf("seed retention proof outbox rows: %v", err)
	}

	repository := NewMessageRepository(pool)
	rows, err := repository.AuditMessageRetentionProof(ctx, MessageRetentionProofAuditOptions{
		TenantID:       tenantID,
		ConversationID: "conv-1",
		Status:         "deleted",
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("audit retention proof: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one retention proof row, got %d", len(rows))
	}
	row := rows[0]
	if row.MessageID != "msg-deleted" ||
		row.Status != "DELETED" ||
		row.ConversationSeq != 7 ||
		!row.CurrentPayloadPresent ||
		row.DeletedAt == nil ||
		row.DeleteChangeVersion == nil ||
		*row.DeleteChangeVersion != 1 ||
		row.DeleteChangedBy != "user-1" ||
		!row.DeleteReasonPresent ||
		!row.DeleteBeforePayloadPresent ||
		!row.DeleteAfterPayloadPresent ||
		row.DeleteChangedAt == nil ||
		!row.DeleteTimelineEventPresent ||
		!row.DeleteOutboxEventPresent {
		t.Fatalf("unexpected retention proof row: %+v", row)
	}

	_, err = repository.AuditMessageRetentionProof(ctx, MessageRetentionProofAuditOptions{Status: "PURGED"})
	if err == nil {
		t.Fatalf("expected unsupported status to fail")
	}
}
