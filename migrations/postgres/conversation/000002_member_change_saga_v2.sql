BEGIN;

ALTER TABLE member_change_saga
    ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS dead_lettered_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS timeline_event_id TEXT,
    ADD COLUMN IF NOT EXISTS outbox_event_id TEXT,
    ADD COLUMN IF NOT EXISTS metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE member_change_saga
    DROP CONSTRAINT IF EXISTS member_change_saga_status_check;
ALTER TABLE member_change_saga
    DROP CONSTRAINT IF EXISTS member_change_saga_retry_count_check;
ALTER TABLE member_change_saga
    DROP CONSTRAINT IF EXISTS member_change_saga_metadata_json_check;

ALTER TABLE member_change_saga
    ADD CONSTRAINT member_change_saga_status_check CHECK (
        status IN (
            'PENDING_BOUNDARY',
            'BOUNDARY_ALLOCATED',
            'MEMBER_UPDATED',
            'OUTBOX_ENQUEUED',
            'EVENT_PUBLISHED',
            'DONE',
            'FAILED_COMPENSATED'
        )
    ),
    ADD CONSTRAINT member_change_saga_retry_count_check CHECK (retry_count >= 0),
    ADD CONSTRAINT member_change_saga_metadata_json_check CHECK (jsonb_typeof(metadata_json) = 'object');

CREATE INDEX IF NOT EXISTS idx_member_change_saga_next_retry
    ON member_change_saga (tenant_id, status, next_retry_at)
    WHERE status IN ('PENDING_BOUNDARY', 'BOUNDARY_ALLOCATED', 'MEMBER_UPDATED', 'OUTBOX_ENQUEUED');

CREATE INDEX IF NOT EXISTS idx_member_change_saga_outbox_event
    ON member_change_saga (tenant_id, outbox_event_id)
    WHERE outbox_event_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_member_change_saga_dead_letter
    ON member_change_saga (tenant_id, dead_lettered_at)
    WHERE dead_lettered_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_conversation_members_conversation_status
    ON conversation_members (tenant_id, conversation_id, status);

COMMIT;
