CREATE INDEX IF NOT EXISTS idx_device_received_cursors_user_conversation_seq
    ON device_received_cursors (tenant_id, user_id, conversation_id, last_received_seq);
