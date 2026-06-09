BEGIN;

ALTER TABLE member_change_saga
    DROP CONSTRAINT IF EXISTS member_change_saga_change_type_check;

ALTER TABLE member_change_saga
    ADD CONSTRAINT member_change_saga_change_type_check CHECK (
        change_type IN ('JOIN', 'LEAVE', 'ROLE_CHANGED', 'REMOVE', 'OWNER_TRANSFER')
    );

COMMIT;
