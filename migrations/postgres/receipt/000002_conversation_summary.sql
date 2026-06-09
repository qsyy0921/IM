ALTER TABLE receipt_inbox_projection
    ADD COLUMN IF NOT EXISTS source_event_type TEXT NOT NULL DEFAULT 'message.persisted.v1';

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

WITH latest_inbox AS (
    SELECT DISTINCT ON (tenant_id, user_id, conversation_id)
        tenant_id,
        user_id,
        conversation_id,
        conversation_seq,
        message_id,
        sender_id,
        created_at
    FROM receipt_inbox_projection
    ORDER BY tenant_id, user_id, conversation_id, conversation_seq DESC, created_at DESC
),
summary_counts AS (
    SELECT
        rip.tenant_id,
        rip.user_id,
        rip.conversation_id,
        COALESCE(urc.last_read_seq, 0) AS last_read_seq,
        COUNT(*) FILTER (
            WHERE rip.source_event_type = 'message.persisted.v1'
              AND rip.conversation_seq > COALESCE(urc.last_read_seq, 0)
        ) AS unread_count
    FROM receipt_inbox_projection rip
    LEFT JOIN user_read_cursors urc
      ON urc.tenant_id = rip.tenant_id
     AND urc.user_id = rip.user_id
     AND urc.conversation_id = rip.conversation_id
    GROUP BY rip.tenant_id, rip.user_id, rip.conversation_id, urc.last_read_seq
)
INSERT INTO user_conversation_summaries (
    tenant_id,
    user_id,
    conversation_id,
    last_visible_seq,
    last_message_id,
    last_sender_id,
    last_read_seq,
    unread_count,
    sort_updated_at,
    updated_at
)
SELECT
    latest_inbox.tenant_id,
    latest_inbox.user_id,
    latest_inbox.conversation_id,
    latest_inbox.conversation_seq,
    latest_inbox.message_id,
    latest_inbox.sender_id,
    summary_counts.last_read_seq,
    summary_counts.unread_count,
    latest_inbox.created_at,
    now()
FROM latest_inbox
JOIN summary_counts
  ON summary_counts.tenant_id = latest_inbox.tenant_id
 AND summary_counts.user_id = latest_inbox.user_id
 AND summary_counts.conversation_id = latest_inbox.conversation_id
ON CONFLICT (tenant_id, user_id, conversation_id) DO NOTHING;
