CREATE INDEX IF NOT EXISTS idx_delivery_outbox_pending_ready_expr
    ON delivery_outbox ((COALESCE(next_retry_at, available_at)), id)
    WHERE status = 'PENDING' AND published_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_delivery_outbox_blocking_aggregate
    ON delivery_outbox (tenant_id, conversation_id, aggregate_version)
    WHERE status IN ('PENDING', 'DLQ');

CREATE INDEX IF NOT EXISTS idx_delivery_outbox_pending_conversation_version
    ON delivery_outbox (tenant_id, conversation_id, aggregate_version, id)
    WHERE status = 'PENDING' AND published_at IS NULL;
