package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func openPool(ctx context.Context, cfg config) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, cfg.PGDSN)
	if err != nil {
		return nil, fmt.Errorf("open postgres pool: %w", err)
	}
	return pool, nil
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	statements := []string{
		`DELETE FROM delivery_outbox WHERE tenant_id = $1`,
		`DELETE FROM device_delivery_cursors WHERE tenant_id = $1`,
		`DELETE FROM delivery_membership_projection WHERE tenant_id = $1`,
		`DELETE FROM user_inbox WHERE tenant_id = $1`,
		`DELETE FROM message_command_idempotency WHERE tenant_id = $1`,
		`DELETE FROM message_change_history WHERE tenant_id = $1`,
		`DELETE FROM timeline_gap_markers WHERE tenant_id = $1`,
		`DELETE FROM seq_allocation_journal WHERE tenant_id = $1`,
		`DELETE FROM message_outbox WHERE tenant_id = $1`,
		`DELETE FROM conversation_timeline_events WHERE tenant_id = $1`,
		`DELETE FROM message_log WHERE tenant_id = $1`,
		`DELETE FROM conversation_seq WHERE tenant_id = $1`,
		`DELETE FROM member_change_saga WHERE tenant_id = $1`,
		`DELETE FROM conversation_members WHERE tenant_id = $1`,
		`DELETE FROM conversations WHERE tenant_id = $1`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement, tenantID); err != nil {
			return fmt.Errorf("cleanup tenant %s: %w", tenantID, err)
		}
	}
	return nil
}

func waitForCount(
	ctx context.Context,
	pool *pgxpool.Pool,
	timeout time.Duration,
	interval time.Duration,
	query string,
	expected int64,
	args ...any,
) (int64, error) {
	deadline := time.Now().Add(timeout)
	for {
		var count int64
		if err := pool.QueryRow(ctx, query, args...).Scan(&count); err != nil {
			return 0, err
		}
		if count >= expected {
			return count, nil
		}
		if time.Now().After(deadline) {
			return count, fmt.Errorf("timed out waiting for count >= %d, last=%d", expected, count)
		}
		select {
		case <-ctx.Done():
			return count, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func waitForZero(
	ctx context.Context,
	pool *pgxpool.Pool,
	timeout time.Duration,
	interval time.Duration,
	query string,
	args ...any,
) (int64, error) {
	deadline := time.Now().Add(timeout)
	for {
		var count int64
		if err := pool.QueryRow(ctx, query, args...).Scan(&count); err != nil {
			return 0, err
		}
		if count == 0 {
			return count, nil
		}
		if time.Now().After(deadline) {
			return count, fmt.Errorf("timed out waiting for count = 0, last=%d", count)
		}
		select {
		case <-ctx.Done():
			return count, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func readConversationFanoutMode(ctx context.Context, pool *pgxpool.Pool, cfg config) (string, error) {
	var fanoutMode string
	err := pool.QueryRow(ctx, `
SELECT fanout_mode
FROM conversations
WHERE tenant_id = $1 AND conversation_id = $2
`, cfg.TenantID, cfg.ConversationID).Scan(&fanoutMode)
	if err != nil {
		return "", fmt.Errorf("query conversation fanout mode: %w", err)
	}
	return fanoutMode, nil
}

func readPostgresStats(ctx context.Context, pool *pgxpool.Pool, cfg config) (postgresStats, error) {
	stats := postgresStats{}
	if err := pool.QueryRow(ctx, `
SELECT conversation_mode, fanout_mode, fanout_policy_version
FROM conversations
WHERE tenant_id = $1 AND conversation_id = $2
`, cfg.TenantID, cfg.ConversationID).Scan(&stats.ConversationMode, &stats.FanoutMode, &stats.FanoutPolicyVersion); err != nil {
		return postgresStats{}, fmt.Errorf("query conversation policy: %w", err)
	}
	queries := []struct {
		name   string
		target *int64
		sql    string
	}{
		{"conversation members", &stats.ConversationMemberCount, `SELECT COUNT(*) FROM conversation_members WHERE tenant_id = $1 AND conversation_id = $2 AND status = 'ACTIVE'`},
		{"delivery membership", &stats.DeliveryMembershipActiveCount, `SELECT COUNT(*) FROM delivery_membership_projection WHERE tenant_id = $1 AND conversation_id = $2 AND status = 'ACTIVE'`},
		{"message log", &stats.MessageLogCount, `SELECT COUNT(*) FROM message_log WHERE tenant_id = $1 AND conversation_id = $2`},
		{"delivery timeline", &stats.DeliveryTimelineRows, `SELECT COUNT(*) FROM delivery_timeline_items WHERE tenant_id = $1 AND conversation_id = $2`},
		{"user inbox", &stats.UserInboxRows, `SELECT COUNT(*) FROM user_inbox WHERE tenant_id = $1 AND conversation_id = $2`},
		{"delivery outbox", &stats.DeliveryOutboxRows, `SELECT COUNT(*) FROM delivery_outbox WHERE tenant_id = $1 AND conversation_id = $2`},
		{"message outbox pending", &stats.MessageOutboxPending, `SELECT COUNT(*) FROM message_outbox WHERE tenant_id = $1 AND conversation_id = $2 AND status = 'PENDING'`},
		{"message outbox dlq", &stats.MessageOutboxDLQ, `SELECT COUNT(*) FROM message_outbox WHERE tenant_id = $1 AND conversation_id = $2 AND status = 'DLQ'`},
		{"delivery outbox pending", &stats.DeliveryOutboxPending, `SELECT COUNT(*) FROM delivery_outbox WHERE tenant_id = $1 AND conversation_id = $2 AND status = 'PENDING'`},
		{"delivery outbox dlq", &stats.DeliveryOutboxDLQ, `SELECT COUNT(*) FROM delivery_outbox WHERE tenant_id = $1 AND conversation_id = $2 AND status = 'DLQ'`},
	}
	for _, query := range queries {
		if err := pool.QueryRow(ctx, query.sql, cfg.TenantID, cfg.ConversationID).Scan(query.target); err != nil {
			return postgresStats{}, fmt.Errorf("query %s: %w", query.name, err)
		}
	}
	return stats, nil
}
