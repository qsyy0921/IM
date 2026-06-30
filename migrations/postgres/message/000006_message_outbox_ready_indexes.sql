CREATE INDEX IF NOT EXISTS idx_message_outbox_pending_conversation_version
    ON message_outbox (tenant_id, conversation_id, aggregate_version, id)
    WHERE status = 'PENDING' AND published_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_message_outbox_blocking_conversation_version
    ON message_outbox (tenant_id, conversation_id, aggregate_version, status, id)
    WHERE status IN ('PENDING', 'DLQ') AND published_at IS NULL;
