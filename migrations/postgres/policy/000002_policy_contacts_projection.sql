CREATE TABLE IF NOT EXISTS policy_contact_edges_projection (
    tenant_id           TEXT        NOT NULL,
    owner_user_id       TEXT        NOT NULL,
    contact_user_id     TEXT        NOT NULL,
    status              TEXT        NOT NULL,
    edge_version        BIGINT      NOT NULL,
    updated_by_event_id TEXT        NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, owner_user_id, contact_user_id),
    CHECK (owner_user_id <> contact_user_id),
    CHECK (status IN ('ACTIVE', 'DELETED', 'BLOCKED')),
    CHECK (edge_version > 0),
    CHECK (updated_by_event_id <> '')
);

CREATE INDEX IF NOT EXISTS idx_policy_contact_edges_status
    ON policy_contact_edges_projection (tenant_id, owner_user_id, contact_user_id, status);

CREATE TABLE IF NOT EXISTS policy_kafka_checkpoints (
    consumer_group      TEXT        NOT NULL,
    topic               TEXT        NOT NULL,
    partition_id        INT         NOT NULL,
    offset_value        BIGINT      NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer_group, topic, partition_id),
    CHECK (consumer_group <> ''),
    CHECK (topic <> ''),
    CHECK (partition_id >= 0),
    CHECK (offset_value > 0)
);
