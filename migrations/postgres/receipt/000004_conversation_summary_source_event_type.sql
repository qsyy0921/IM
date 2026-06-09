ALTER TABLE user_conversation_summaries
    ADD COLUMN IF NOT EXISTS last_source_event_type TEXT NOT NULL DEFAULT 'message.persisted.v1';

WITH latest_inbox AS (
    SELECT DISTINCT ON (tenant_id, user_id, conversation_id)
        tenant_id,
        user_id,
        conversation_id,
        source_event_type
    FROM receipt_inbox_projection
    ORDER BY tenant_id, user_id, conversation_id, conversation_seq DESC, created_at DESC
)
UPDATE user_conversation_summaries ucs
SET last_source_event_type = latest_inbox.source_event_type,
    updated_at = now()
FROM latest_inbox
WHERE ucs.tenant_id = latest_inbox.tenant_id
  AND ucs.user_id = latest_inbox.user_id
  AND ucs.conversation_id = latest_inbox.conversation_id;
