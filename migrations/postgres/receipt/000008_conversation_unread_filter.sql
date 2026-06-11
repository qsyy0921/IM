CREATE INDEX IF NOT EXISTS idx_user_conversation_summaries_list_unread
ON user_conversation_summaries (
    tenant_id,
    user_id,
    archived,
    sort_updated_at DESC,
    conversation_id
)
WHERE unread_count > 0;

CREATE INDEX IF NOT EXISTS idx_user_conversation_summaries_list_pinned_unread
ON user_conversation_summaries (
    tenant_id,
    user_id,
    archived,
    pinned DESC,
    sort_updated_at DESC,
    conversation_id
)
WHERE unread_count > 0;
