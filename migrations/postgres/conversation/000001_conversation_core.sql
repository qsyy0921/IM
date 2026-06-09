BEGIN;

CREATE TABLE IF NOT EXISTS conversations (
    tenant_id              TEXT        NOT NULL,
    conversation_id        TEXT        NOT NULL,
    conversation_type      TEXT        NOT NULL,
    status                 TEXT        NOT NULL,
    conversation_mode      TEXT        NOT NULL,
    fanout_mode            TEXT        NOT NULL,
    fanout_policy_version  BIGINT      NOT NULL DEFAULT 1,
    member_version         BIGINT      NOT NULL DEFAULT 1,
    permission_version     BIGINT      NOT NULL DEFAULT 1,
    current_seq_shard      TEXT        NOT NULL DEFAULT 'local',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id),
    CHECK (conversation_type IN ('DIRECT', 'GROUP')),
    CHECK (status IN ('ACTIVE', 'ARCHIVED', 'DELETED')),
    CHECK (conversation_mode IN ('LOCAL_ROW_LOCK', 'SEQUENCER_BLOCK')),
    CHECK (fanout_mode IN ('WRITE_FANOUT', 'HYBRID_FANOUT', 'READ_FANOUT', 'BROADCAST_SIGNAL'))
);

CREATE TABLE IF NOT EXISTS conversation_members (
    tenant_id           TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    user_id             TEXT        NOT NULL,
    role                TEXT        NOT NULL,
    status              TEXT        NOT NULL,
    join_seq            BIGINT,
    leave_seq           BIGINT,
    member_version      BIGINT      NOT NULL,
    permission_version  BIGINT      NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id, user_id),
    CHECK (role IN ('OWNER', 'ADMIN', 'MEMBER')),
    CHECK (status IN ('ACTIVE', 'LEFT', 'BANNED')),
    FOREIGN KEY (tenant_id, conversation_id)
        REFERENCES conversations (tenant_id, conversation_id)
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_conversation_members_user
    ON conversation_members (tenant_id, user_id, status);

CREATE TABLE IF NOT EXISTS member_change_saga (
    change_id                TEXT        PRIMARY KEY,
    tenant_id                TEXT        NOT NULL,
    conversation_id          TEXT        NOT NULL,
    user_id                  TEXT        NOT NULL,
    change_type              TEXT        NOT NULL,
    boundary_seq             BIGINT,
    status                   TEXT        NOT NULL,
    idempotency_key          TEXT        NOT NULL,
    expected_member_version  BIGINT,
    command_hash             TEXT        NOT NULL,
    operator_id              TEXT        NOT NULL,
    conflict_policy          TEXT        NOT NULL,
    retry_count              INT         NOT NULL DEFAULT 0,
    last_error               TEXT,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, conversation_id, idempotency_key),
    CHECK (change_type IN ('JOIN', 'LEAVE', 'ROLE_CHANGED', 'REMOVE')),
    CHECK (status IN ('PENDING_BOUNDARY', 'BOUNDARY_ALLOCATED', 'MEMBER_UPDATED', 'EVENT_PUBLISHED', 'DONE', 'FAILED_COMPENSATED')),
    CHECK (conflict_policy IN ('REJECT', 'MERGE', 'COMPENSATE'))
);

CREATE INDEX IF NOT EXISTS idx_member_change_saga_conversation_status
    ON member_change_saga (tenant_id, conversation_id, status, created_at);

COMMIT;
