CREATE INDEX IF NOT EXISTS idx_notification_outbox_ready_relay
    ON notification_outbox (event_version, created_at, event_id)
    WHERE status = 'PENDING';

CREATE INDEX IF NOT EXISTS idx_notification_outbox_request_blocker
    ON notification_outbox (tenant_id, request_id, event_version, created_at, event_id)
    WHERE status IN ('PENDING', 'DLQ');
