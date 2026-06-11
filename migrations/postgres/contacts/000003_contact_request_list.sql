BEGIN;

CREATE INDEX IF NOT EXISTS idx_contact_requests_receiver_status_created
    ON contact_requests (tenant_id, receiver_user_id, status, created_at DESC, request_id);

CREATE INDEX IF NOT EXISTS idx_contact_requests_sender_status_created
    ON contact_requests (tenant_id, sender_user_id, status, created_at DESC, request_id);

COMMIT;
