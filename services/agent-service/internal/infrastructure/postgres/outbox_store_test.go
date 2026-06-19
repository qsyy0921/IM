package postgres

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/agent-service/internal/types"
)

func TestOutboxStoreProcessReadyBatchIntegration(t *testing.T) {
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if strings.TrimSpace(dsn) == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, mustReadMigration(t)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE agent_approval_outbox, agent_proposals`); err != nil {
		t.Fatalf("cleanup outbox: %v", err)
	}

	if _, err := pool.Exec(ctx, `
INSERT INTO agent_approval_outbox (
    event_id, tenant_id, proposal_id, approval_id, prepared_audit_id, skill_id,
    tool_name, resource_type, resource_id, risk_level, event_type, event_version,
    mapping_version, partition_key, producer, payload_json
) VALUES (
    'agent-outbox-event-1', 'tenant-agent-outbox-test', 'ap-outbox-1',
    'approval-outbox-1', 'mcp-audit-outbox-1', 'conversation.note.create',
    'conversation.note.create', 'conversation', 'conv-1', 'LOW',
    'agent.proposal.approved.v1', 'v1', 1, 'ap-outbox-1', 'agent-service',
    '{"schema_version":1,"event_type":"agent.proposal.approved.v1","tenant_id":"tenant-agent-outbox-test","proposal_id":"ap-outbox-1","approval_id":"approval-outbox-1","prepared_audit_id":"mcp-audit-outbox-1","skill_id":"conversation.note.create","tool_name":"conversation.note.create","resource_type":"conversation","resource_id":"conv-1","risk_level":"LOW","approved_by_user_id":"approver-1","approved_at_unix_ms":1710000000000}'::jsonb
)`); err != nil {
		t.Fatalf("seed outbox: %v", err)
	}

	store := NewOutboxStore(pool, WithOutboxClock(func() time.Time {
		return time.Unix(1710000001, 0).UTC()
	}))
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		if len(messages) != 1 || messages[0].EventID != "agent-outbox-event-1" {
			t.Fatalf("unexpected messages: %+v", messages)
		}
		return []error{nil}
	})
	if err != nil {
		t.Fatalf("ProcessReadyBatch() error = %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 1 || stats.Retried != 0 || stats.DeadLettered != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	assertAgentApprovalOutboxStatus(t, ctx, pool, "agent-outbox-event-1", types.OutboxStatusPublished, 0)
}

func TestOutboxStoreRetriesAndDeadLettersIntegration(t *testing.T) {
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if strings.TrimSpace(dsn) == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, mustReadMigration(t)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE agent_approval_outbox, agent_proposals`); err != nil {
		t.Fatalf("cleanup outbox: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO agent_approval_outbox (
    event_id, tenant_id, proposal_id, approval_id, prepared_audit_id, skill_id,
    tool_name, resource_type, event_type, event_version, mapping_version,
    partition_key, producer, payload_json, retry_count
) VALUES (
    'agent-outbox-event-dlq', 'tenant-agent-outbox-test', 'ap-outbox-dlq',
    'approval-outbox-dlq', 'mcp-audit-outbox-dlq', 'conversation.note.create',
    'conversation.note.create', 'conversation', 'agent.proposal.approved.v1',
    'v1', 1, 'ap-outbox-dlq', 'agent-service',
    '{"schema_version":1,"event_type":"agent.proposal.approved.v1","tenant_id":"tenant-agent-outbox-test","proposal_id":"ap-outbox-dlq","approval_id":"approval-outbox-dlq","prepared_audit_id":"mcp-audit-outbox-dlq","skill_id":"conversation.note.create","tool_name":"conversation.note.create","resource_type":"conversation","approved_by_user_id":"approver-1","approved_at_unix_ms":1710000000000}'::jsonb,
    1
)`); err != nil {
		t.Fatalf("seed outbox: %v", err)
	}

	store := NewOutboxStore(pool)
	stats, err := store.ProcessReadyBatch(ctx, 10, 2, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		return []error{errors.New("kafka broker unavailable with sensitive detail 127.0.0.1")}
	})
	if err != nil {
		t.Fatalf("ProcessReadyBatch() error = %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 0 || stats.Retried != 0 || stats.DeadLettered != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	assertAgentApprovalOutboxStatus(t, ctx, pool, "agent-outbox-event-dlq", types.OutboxStatusDLQ, 2)
	var lastError string
	if err := pool.QueryRow(ctx, `
SELECT COALESCE(last_error, '')
FROM agent_approval_outbox
WHERE event_id = 'agent-outbox-event-dlq'`).Scan(&lastError); err != nil {
		t.Fatalf("read last_error: %v", err)
	}
	if lastError != "agent approval outbox publish broker unavailable" {
		t.Fatalf("unexpected sanitized last_error %q", lastError)
	}
}

func assertAgentApprovalOutboxStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID string, status string, retryCount int) {
	t.Helper()
	var gotStatus string
	var gotRetryCount int
	if err := pool.QueryRow(ctx, `
SELECT status, retry_count
FROM agent_approval_outbox
WHERE event_id = $1`, eventID).Scan(&gotStatus, &gotRetryCount); err != nil {
		t.Fatalf("read outbox status: %v", err)
	}
	if gotStatus != status || gotRetryCount != retryCount {
		t.Fatalf("unexpected outbox state status=%q retry=%d", gotStatus, gotRetryCount)
	}
}
