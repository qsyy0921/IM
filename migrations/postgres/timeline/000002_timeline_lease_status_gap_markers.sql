ALTER TABLE timeline_seq_block_leases
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'ACTIVE',
    ADD COLUMN IF NOT EXISTS expired_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS expired_by TEXT,
    ADD COLUMN IF NOT EXISTS expire_reason TEXT;

ALTER TABLE timeline_seq_block_leases
    DROP CONSTRAINT IF EXISTS ck_timeline_seq_block_status;

ALTER TABLE timeline_seq_block_leases
    ADD CONSTRAINT ck_timeline_seq_block_status
    CHECK (status IN ('ACTIVE', 'EXPIRED', 'CANCELLED')) NOT VALID;

CREATE INDEX IF NOT EXISTS idx_timeline_seq_block_leases_active_expiry
    ON timeline_seq_block_leases (expires_at, tenant_id, conversation_id)
    WHERE status = 'ACTIVE';

CREATE TABLE IF NOT EXISTS timeline_seq_gap_markers (
    marker_id           TEXT        PRIMARY KEY,
    tenant_id           TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    start_seq           BIGINT      NOT NULL,
    end_seq             BIGINT      NOT NULL,
    sequencer_epoch     BIGINT      NOT NULL,
    lease_id            TEXT        NOT NULL,
    reason              TEXT        NOT NULL,
    status              TEXT        NOT NULL DEFAULT 'OPEN',
    created_by          TEXT        NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_by           TEXT,
    closed_at           TIMESTAMPTZ,
    close_reason        TEXT,
    CONSTRAINT ck_timeline_gap_marker_range CHECK (start_seq > 0 AND end_seq >= start_seq),
    CONSTRAINT ck_timeline_gap_marker_epoch CHECK (sequencer_epoch > 0),
    CONSTRAINT ck_timeline_gap_marker_status CHECK (status IN ('OPEN', 'CLOSED'))
);

CREATE INDEX IF NOT EXISTS idx_timeline_seq_gap_markers_open_conversation
    ON timeline_seq_gap_markers (tenant_id, conversation_id, start_seq)
    WHERE status = 'OPEN';

CREATE UNIQUE INDEX IF NOT EXISTS uq_timeline_seq_gap_markers_open_range
    ON timeline_seq_gap_markers (tenant_id, conversation_id, start_seq, end_seq)
    WHERE status = 'OPEN';
