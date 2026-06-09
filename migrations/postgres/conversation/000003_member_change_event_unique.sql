BEGIN;

CREATE UNIQUE INDEX IF NOT EXISTS uq_member_change_saga_outbox_event
    ON member_change_saga (tenant_id, outbox_event_id)
    WHERE outbox_event_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_member_change_saga_timeline_event
    ON member_change_saga (tenant_id, timeline_event_id)
    WHERE timeline_event_id IS NOT NULL;

COMMIT;
