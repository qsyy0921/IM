ALTER TABLE workflow_requests
    DROP CONSTRAINT IF EXISTS ck_workflow_requests_type;

ALTER TABLE workflow_requests
    ADD CONSTRAINT ck_workflow_requests_type
        CHECK (workflow_type IN ('ACTION_APPROVAL', 'REPAIR_APPROVAL', 'ADMIN_OPERATION'));
