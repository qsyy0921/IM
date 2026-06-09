CREATE TABLE IF NOT EXISTS user_conversation_summaries (
    tenant_id         TEXT        NOT NULL,
    user_id           TEXT        NOT NULL,
    conversation_id   TEXT        NOT NULL,
    last_visible_seq  BIGINT      NOT NULL,
    last_message_id   TEXT        NOT NULL,
    last_sender_id    TEXT        NOT NULL,
    last_read_seq     BIGINT      NOT NULL DEFAULT 0,
    unread_count      BIGINT      NOT NULL DEFAULT 0,
    sort_updated_at   TIMESTAMPTZ NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, conversation_id)
);

CREATE INDEX IF NOT EXISTS idx_user_conversation_summaries_list
    ON user_conversation_summaries (tenant_id, user_id, sort_updated_at DESC, conversation_id);

CREATE TABLE IF NOT EXISTS conversation_summary_checkpoints (
    consumer_group TEXT        NOT NULL,
    topic          TEXT        NOT NULL,
    partition_id   INT         NOT NULL,
    offset_value   BIGINT      NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer_group, topic, partition_id)
);
