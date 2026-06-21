ALTER TABLE workflow_requests
    DROP CONSTRAINT IF EXISTS ck_workflow_requests_status;

ALTER TABLE workflow_requests
    ADD CONSTRAINT ck_workflow_requests_status
        CHECK (status IN (
            'WAITING_DECISION',
            'APPROVED',
            'REJECTED',
            'CANCELED',
            'COMPENSATION_PENDING'
        ));
