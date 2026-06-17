package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func waitMembership(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	deadline := time.Now().Add(cfg.waitTimeout)
	for time.Now().Before(deadline) {
		var count int
		err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM delivery_membership_projection
WHERE tenant_id = $1
  AND conversation_id = $2
  AND user_id = $3
  AND status = 'ACTIVE'
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID).Scan(&count)
		if err == nil && count > 0 {
			return nil
		}
		time.Sleep(cfg.pollInterval)
	}
	return errors.New("delivery membership projection timeout")
}

func waitReceiptReceived(ctx context.Context, pool *pgxpool.Pool, cfg config, seq int64) error {
	deadline := time.Now().Add(cfg.waitTimeout)
	for time.Now().Before(deadline) {
		var receivedSeq int64
		err := pool.QueryRow(ctx, `
SELECT COALESCE(MAX(CASE WHEN received_at IS NULL THEN 0 ELSE conversation_seq END), 0)
FROM message_receipt_states
WHERE tenant_id = $1
  AND conversation_id = $2
  AND conversation_seq = $3
  AND user_id = $4
`, cfg.tenantID, cfg.conversationID, seq, cfg.receiverUserID).Scan(&receivedSeq)
		if err == nil && receivedSeq >= seq {
			return nil
		}
		time.Sleep(cfg.pollInterval)
	}
	return errors.New("receipt received projection timeout")
}

func waitReceiptOutboxPublished(ctx context.Context, pool *pgxpool.Pool, cfg config, wantPublished int64) error {
	deadline := time.Now().Add(cfg.waitTimeout)
	for time.Now().Before(deadline) {
		var total int64
		var published int64
		var dlq int64
		err := pool.QueryRow(ctx, `
SELECT
    COUNT(*),
    COUNT(*) FILTER (WHERE status = 'PUBLISHED'),
    COUNT(*) FILTER (WHERE status = 'DLQ')
FROM receipt_outbox
WHERE tenant_id = $1
  AND conversation_id = $2
`, cfg.tenantID, cfg.conversationID).Scan(&total, &published, &dlq)
		if err != nil {
			return fmt.Errorf("query receipt outbox publish state: %w", err)
		}
		if dlq > 0 {
			return fmt.Errorf("receipt outbox reached DLQ: dlq=%d", dlq)
		}
		if total >= wantPublished && published >= wantPublished {
			return nil
		}
		time.Sleep(cfg.pollInterval)
	}
	return fmt.Errorf("receipt outbox publish timeout: want_published=%d", wantPublished)
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	statements := []string{
		`DELETE FROM conversation_summary_checkpoints WHERE consumer_group = $1`,
		`DELETE FROM user_conversation_summaries WHERE tenant_id = $1`,
		`DELETE FROM receipt_outbox WHERE tenant_id = $1`,
		`DELETE FROM receipt_kafka_checkpoints WHERE consumer_group = $1`,
		`DELETE FROM message_receipt_states WHERE tenant_id = $1`,
		`DELETE FROM user_read_cursors WHERE tenant_id = $1`,
		`DELETE FROM user_received_cursors WHERE tenant_id = $1`,
		`DELETE FROM device_received_cursors WHERE tenant_id = $1`,
		`DELETE FROM receipt_inbox_projection WHERE tenant_id = $1`,
		`DELETE FROM delivery_outbox WHERE tenant_id = $1`,
		`DELETE FROM device_delivery_cursors WHERE tenant_id = $1`,
		`DELETE FROM user_inbox WHERE tenant_id = $1`,
		`DELETE FROM delivery_membership_projection WHERE tenant_id = $1`,
		`DELETE FROM delivery_kafka_checkpoints WHERE consumer_group = $1`,
		`DELETE FROM message_outbox WHERE tenant_id = $1`,
		`DELETE FROM conversation_timeline_events WHERE tenant_id = $1`,
		`DELETE FROM message_log WHERE tenant_id = $1`,
		`DELETE FROM conversation_seq WHERE tenant_id = $1`,
		`DELETE FROM member_change_saga WHERE tenant_id = $1`,
		`DELETE FROM conversation_members WHERE tenant_id = $1`,
		`DELETE FROM conversations WHERE tenant_id = $1`,
	}
	for _, statement := range statements {
		arg := any(cfg.tenantID)
		if strings.Contains(statement, "receipt_kafka_checkpoints") || strings.Contains(statement, "conversation_summary_checkpoints") {
			arg = cfg.receiptGroup
		}
		if strings.Contains(statement, "delivery_kafka_checkpoints") {
			arg = cfg.deliveryGroup
		}
		if _, err := pool.Exec(ctx, statement, arg); err != nil {
			return fmt.Errorf("cleanup tenant: %w", err)
		}
	}
	return nil
}

func seedConversation(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	_, err := pool.Exec(ctx, `
INSERT INTO conversations (
    tenant_id, conversation_id, conversation_type, status, conversation_mode,
    fanout_mode, fanout_policy_version, member_version, permission_version, current_seq_shard
) VALUES ($1, $2, 'GROUP', 'ACTIVE', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 1, 1, 1, 'local')
`, cfg.tenantID, cfg.conversationID)
	if err != nil {
		return fmt.Errorf("seed conversation: %w", err)
	}
	_, err = pool.Exec(ctx, `
INSERT INTO conversation_members (
    tenant_id, conversation_id, user_id, role, status, member_version, permission_version
) VALUES ($1, $2, $3, 'OWNER', 'ACTIVE', 1, 1)
`, cfg.tenantID, cfg.conversationID, cfg.ownerUserID)
	if err != nil {
		return fmt.Errorf("seed owner member: %w", err)
	}
	return nil
}

func fillPostgresStats(ctx context.Context, pool *pgxpool.Pool, cfg config, result *summary) error {
	assign := func(target *int64, query string, args ...any) error {
		var value int64
		if err := pool.QueryRow(ctx, query, args...).Scan(&value); err != nil {
			return err
		}
		*target = value
		return nil
	}
	if err := assign(&result.ReceiptProjection.InboxProjectionCount, `
SELECT COUNT(*) FROM receipt_inbox_projection
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID); err != nil {
		return fmt.Errorf("query receipt projection count: %w", err)
	}
	if err := pool.QueryRow(ctx, `
SELECT COALESCE(MIN(conversation_seq), 0), COALESCE(MAX(conversation_seq), 0)
FROM receipt_inbox_projection
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID).Scan(
		&result.ReceiptProjection.InboxProjectionMinSeq,
		&result.ReceiptProjection.InboxProjectionMaxSeq,
	); err != nil {
		return fmt.Errorf("query receipt projection min/max: %w", err)
	}
	if err := assign(&result.ReceiptProjection.DeviceReceivedCursorSeq, `
SELECT COALESCE(MAX(last_received_seq), 0)
FROM device_received_cursors
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3 AND device_id = $4
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID, cfg.receiverDeviceID); err != nil {
		return fmt.Errorf("query device received cursor: %w", err)
	}
	if err := assign(&result.ReceiptProjection.UserReceivedCursorSeq, `
SELECT COALESCE(MAX(last_received_seq), 0)
FROM user_received_cursors
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID); err != nil {
		return fmt.Errorf("query user received cursor: %w", err)
	}
	if err := assign(&result.ReceiptProjection.UserReadCursorSeq, `
SELECT COALESCE(MAX(last_read_seq), 0)
FROM user_read_cursors
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID); err != nil {
		return fmt.Errorf("query user read cursor: %w", err)
	}
	if err := assign(&result.ReceiptProjection.MessageReceiptStateCount, `
SELECT COUNT(*) FROM message_receipt_states
WHERE tenant_id = $1 AND conversation_id = $2
`, cfg.tenantID, cfg.conversationID); err != nil {
		return fmt.Errorf("query message receipt state count: %w", err)
	}
	if err := pool.QueryRow(ctx, `
SELECT
    COALESCE(MAX(CASE WHEN received_at IS NULL THEN 0 ELSE conversation_seq END), 0),
    COALESCE(MAX(CASE WHEN read_at IS NULL THEN 0 ELSE conversation_seq END), 0)
FROM message_receipt_states
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID).Scan(
		&result.ReceiptProjection.ReceiverReceivedSeq,
		&result.ReceiptProjection.ReceiverReadSeq,
	); err != nil {
		return fmt.Errorf("query receiver receipt state: %w", err)
	}
	if cfg.receiptGroup != "" {
		if err := assign(&result.ReceiptProjection.ReceiptCheckpointOffset, `
SELECT COALESCE(MAX(offset_value), 0)
FROM receipt_kafka_checkpoints
WHERE consumer_group = $1 AND topic = 'im.delivery.events'
`, cfg.receiptGroup); err != nil {
			return fmt.Errorf("query receipt checkpoint: %w", err)
		}
	}
	if cfg.deliveryGroup != "" {
		if err := assign(&result.ReceiptProjection.DeliveryCheckpointOffset, `
SELECT COALESCE(MAX(offset_value), 0)
FROM delivery_kafka_checkpoints
WHERE consumer_group = $1
`, cfg.deliveryGroup); err != nil {
			return fmt.Errorf("query delivery checkpoint: %w", err)
		}
	}
	if err := fillReceiptOutboxStats(ctx, pool, cfg, &result.ReceiptOutbox); err != nil {
		return err
	}
	if err := fillDeliveryOutboxStats(ctx, pool, cfg, &result.DeliveryOutbox); err != nil {
		return err
	}
	return nil
}

func fillReceiptOutboxStats(ctx context.Context, pool *pgxpool.Pool, cfg config, stats *receiptOutboxStats) error {
	if err := pool.QueryRow(ctx, `
SELECT
    COUNT(*),
    COUNT(*) FILTER (WHERE status = 'PENDING'),
    COUNT(*) FILTER (WHERE status = 'PUBLISHED'),
    COUNT(*) FILTER (WHERE status = 'DLQ')
FROM receipt_outbox
WHERE tenant_id = $1 AND conversation_id = $2
`, cfg.tenantID, cfg.conversationID).Scan(&stats.Total, &stats.Pending, &stats.Published, &stats.DLQ); err != nil {
		return fmt.Errorf("query receipt outbox stats: %w", err)
	}
	rows, err := pool.Query(ctx, `
SELECT event_type, COUNT(*)
FROM receipt_outbox
WHERE tenant_id = $1 AND conversation_id = $2
GROUP BY event_type
ORDER BY event_type
`, cfg.tenantID, cfg.conversationID)
	if err != nil {
		return fmt.Errorf("query receipt outbox by type: %w", err)
	}
	defer rows.Close()
	stats.ByEventType = map[string]int64{}
	for rows.Next() {
		var eventType string
		var count int64
		if err := rows.Scan(&eventType, &count); err != nil {
			return fmt.Errorf("scan receipt outbox by type: %w", err)
		}
		stats.ByEventType[eventType] = count
	}
	return rows.Err()
}

func fillDeliveryOutboxStats(ctx context.Context, pool *pgxpool.Pool, cfg config, stats *outboxStats) error {
	if err := pool.QueryRow(ctx, `
SELECT
    COUNT(*),
    COUNT(*) FILTER (WHERE status = 'PENDING'),
    COUNT(*) FILTER (WHERE status = 'PUBLISHED'),
    COUNT(*) FILTER (WHERE status = 'DLQ')
FROM delivery_outbox
WHERE tenant_id = $1 AND conversation_id = $2
`, cfg.tenantID, cfg.conversationID).Scan(&stats.Total, &stats.Pending, &stats.Published, &stats.DLQ); err != nil {
		return fmt.Errorf("query delivery outbox stats: %w", err)
	}
	return nil
}
