BEGIN;

CREATE TABLE IF NOT EXISTS conversation_notes (
    tenant_id             TEXT        NOT NULL,
    conversation_id       TEXT        NOT NULL,
    note_id               TEXT        NOT NULL,
    author_user_id        TEXT        NOT NULL,
    body                  TEXT        NOT NULL,
    source_tool_name      TEXT        NOT NULL DEFAULT '',
    source_proposal_id    TEXT        NOT NULL DEFAULT '',
    source_approval_id    TEXT        NOT NULL DEFAULT '',
    source_execution_id   TEXT        NOT NULL DEFAULT '',
    idempotency_key       TEXT        NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id, note_id),
    UNIQUE (tenant_id, conversation_id, author_user_id, idempotency_key),
    FOREIGN KEY (tenant_id, conversation_id)
        REFERENCES conversations (tenant_id, conversation_id)
        ON DELETE CASCADE,
    CHECK (length(body) > 0 AND length(body) <= 4096),
    CHECK (length(source_tool_name) <= 160),
    CHECK (length(source_proposal_id) <= 160),
    CHECK (length(source_approval_id) <= 160),
    CHECK (length(source_execution_id) <= 160),
    CHECK (length(idempotency_key) > 0 AND length(idempotency_key) <= 160)
);

CREATE INDEX IF NOT EXISTS idx_conversation_notes_conversation_created
    ON conversation_notes (tenant_id, conversation_id, created_at DESC, note_id DESC);

COMMIT;
