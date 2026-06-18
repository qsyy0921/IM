CREATE TABLE IF NOT EXISTS search_message_documents (
    tenant_id TEXT NOT NULL,
    conversation_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    conversation_seq BIGINT NOT NULL,
    source_event_id TEXT NOT NULL,
    searchable_text TEXT NOT NULL DEFAULT '',
    message_type TEXT NOT NULL DEFAULT '',
    sender_id TEXT NOT NULL DEFAULT '',
    tombstone_status TEXT NOT NULL DEFAULT 'NONE',
    change_version BIGINT NOT NULL DEFAULT 1,
    visibility_version BIGINT NOT NULL DEFAULT 0,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id, message_id),
    CONSTRAINT ck_search_message_documents_seq_positive CHECK (conversation_seq > 0),
    CONSTRAINT ck_search_message_documents_tombstone_status CHECK (
        tombstone_status IN ('NONE', 'REVOKED', 'DELETED', 'COMPLIANCE_REDACTED')
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_search_message_documents_event
    ON search_message_documents (tenant_id, source_event_id);

CREATE INDEX IF NOT EXISTS idx_search_message_documents_conversation_seq
    ON search_message_documents (tenant_id, conversation_id, conversation_seq);

CREATE INDEX IF NOT EXISTS idx_search_message_documents_visible_text
    ON search_message_documents
    USING GIN (to_tsvector('simple', searchable_text))
    WHERE tombstone_status = 'NONE';

CREATE TABLE IF NOT EXISTS search_membership_projection (
    tenant_id TEXT NOT NULL,
    conversation_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '',
    join_seq BIGINT NOT NULL,
    leave_seq BIGINT,
    member_version BIGINT NOT NULL DEFAULT 0,
    permission_version BIGINT NOT NULL DEFAULT 0,
    updated_by_event_id TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id, user_id),
    CONSTRAINT ck_search_membership_projection_join_seq_positive CHECK (join_seq > 0),
    CONSTRAINT ck_search_membership_projection_leave_seq CHECK (leave_seq IS NULL OR leave_seq >= join_seq),
    CONSTRAINT ck_search_membership_projection_status CHECK (
        status IN ('ACTIVE', 'LEFT', 'REMOVED', 'BANNED', '')
    )
);

CREATE INDEX IF NOT EXISTS idx_search_membership_projection_user
    ON search_membership_projection (tenant_id, user_id, conversation_id);

CREATE TABLE IF NOT EXISTS search_projection_checkpoints (
    consumer_group TEXT NOT NULL,
    topic TEXT NOT NULL,
    partition_id INTEGER NOT NULL,
    offset_value BIGINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer_group, topic, partition_id),
    CONSTRAINT ck_search_projection_checkpoints_partition CHECK (partition_id >= 0),
    CONSTRAINT ck_search_projection_checkpoints_offset CHECK (offset_value >= 0)
);
