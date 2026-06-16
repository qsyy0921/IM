CREATE TABLE IF NOT EXISTS contact_privacy_exceptions (
    tenant_id TEXT NOT NULL,
    owner_user_id TEXT NOT NULL,
    other_user_id TEXT NOT NULL,
    decision TEXT NOT NULL,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, owner_user_id, other_user_id),
    CONSTRAINT ck_contact_privacy_exceptions_decision
        CHECK (decision IN ('ALLOW', 'DENY')),
    CONSTRAINT ck_contact_privacy_exceptions_distinct_users
        CHECK (owner_user_id <> other_user_id)
);

CREATE INDEX IF NOT EXISTS idx_contact_privacy_exceptions_other
    ON contact_privacy_exceptions (tenant_id, other_user_id);

ALTER TABLE contact_command_idempotency
    DROP CONSTRAINT IF EXISTS contact_command_idempotency_command_type_check;

ALTER TABLE contact_command_idempotency
    ADD CONSTRAINT contact_command_idempotency_command_type_check
    CHECK (command_type IN (
        'SEND_CONTACT_REQUEST',
        'RESPOND_CONTACT_REQUEST',
        'DELETE_CONTACT',
        'BLOCK_CONTACT',
        'UNBLOCK_CONTACT',
        'CANCEL_CONTACT_REQUEST',
        'UPDATE_CONTACT_REMARK',
        'UPDATE_CONTACT_GROUP',
        'SET_CONTACT_PRIVACY',
        'SET_TENANT_CONTACT_PRIVACY_DEFAULT',
        'SET_TENANT_CONTACT_REQUEST_SOURCE_POLICY',
        'SET_CONTACT_PRIVACY_EXCEPTION'
    ));
