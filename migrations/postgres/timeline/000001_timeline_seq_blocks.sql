CREATE TABLE IF NOT EXISTS timeline_sequence_state (
    tenant_id           TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    next_seq            BIGINT      NOT NULL DEFAULT 1,
    sequencer_epoch     BIGINT      NOT NULL DEFAULT 1,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id),
    CONSTRAINT ck_timeline_sequence_state_next_seq_positive CHECK (next_seq > 0),
    CONSTRAINT ck_timeline_sequence_state_epoch_positive CHECK (sequencer_epoch > 0)
);

CREATE TABLE IF NOT EXISTS timeline_seq_block_leases (
    lease_id            TEXT        PRIMARY KEY,
    tenant_id           TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    start_seq           BIGINT      NOT NULL,
    end_seq             BIGINT      NOT NULL,
    block_size          INTEGER     NOT NULL,
    sequencer_epoch     BIGINT      NOT NULL,
    requester_id        TEXT        NOT NULL,
    idempotency_key     TEXT        NOT NULL,
    command_hash        TEXT        NOT NULL,
    expires_at          TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_timeline_seq_block_range CHECK (start_seq > 0 AND end_seq >= start_seq),
    CONSTRAINT ck_timeline_seq_block_size CHECK (block_size > 0),
    CONSTRAINT ck_timeline_seq_block_epoch CHECK (sequencer_epoch > 0),
    CONSTRAINT uq_timeline_seq_block_idempotency UNIQUE (tenant_id, conversation_id, requester_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_timeline_seq_block_leases_conversation_created
    ON timeline_seq_block_leases (tenant_id, conversation_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_timeline_seq_block_leases_expiry
    ON timeline_seq_block_leases (expires_at);
