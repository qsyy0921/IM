BEGIN;

ALTER TABLE contact_edges
    ADD COLUMN IF NOT EXISTS remark TEXT NOT NULL DEFAULT '';

ALTER TABLE contact_command_idempotency
    ADD COLUMN IF NOT EXISTS result_json JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE contact_command_idempotency
    DROP CONSTRAINT IF EXISTS contact_command_idempotency_command_type_check;

ALTER TABLE contact_command_idempotency
    ADD CONSTRAINT contact_command_idempotency_command_type_check
    CHECK (command_type IN (
        'SEND_CONTACT_REQUEST',
        'RESPOND_CONTACT_REQUEST',
        'CANCEL_CONTACT_REQUEST',
        'DELETE_CONTACT',
        'BLOCK_CONTACT',
        'UNBLOCK_CONTACT',
        'UPDATE_CONTACT_REMARK',
        'UPDATE_CONTACT_GROUP',
        'SET_CONTACT_PRIVACY',
        'SET_TENANT_CONTACT_PRIVACY_DEFAULT',
        'SET_TENANT_CONTACT_REQUEST_SOURCE_POLICY',
        'SET_CONTACT_PRIVACY_EXCEPTION',
        'DELETE_CONTACT_PRIVACY_EXCEPTION'
    ));

COMMIT;
