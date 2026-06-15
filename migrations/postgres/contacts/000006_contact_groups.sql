BEGIN;

ALTER TABLE contact_edges
    ADD COLUMN IF NOT EXISTS group_name TEXT NOT NULL DEFAULT '';

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
        'UPDATE_CONTACT_GROUP'
    ));

CREATE INDEX IF NOT EXISTS idx_contact_edges_owner_group_active
    ON contact_edges (tenant_id, owner_user_id, group_name, contact_user_id)
    WHERE status = 'ACTIVE';

COMMIT;
