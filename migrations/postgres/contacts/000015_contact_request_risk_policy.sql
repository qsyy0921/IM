BEGIN;

ALTER TABLE contact_tenant_request_source_policies
    ADD COLUMN IF NOT EXISTS risk_level TEXT NOT NULL DEFAULT 'LOW',
    ADD COLUMN IF NOT EXISTS review_required BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE contact_tenant_request_source_policies
    DROP CONSTRAINT IF EXISTS contact_tenant_request_source_policies_risk_level_check;

ALTER TABLE contact_tenant_request_source_policies
    ADD CONSTRAINT contact_tenant_request_source_policies_risk_level_check
    CHECK (risk_level IN ('LOW', 'MEDIUM', 'HIGH'));

ALTER TABLE contact_requests
    ADD COLUMN IF NOT EXISTS risk_level TEXT NOT NULL DEFAULT 'LOW',
    ADD COLUMN IF NOT EXISTS review_required BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE contact_requests
    DROP CONSTRAINT IF EXISTS contact_requests_risk_level_check;

ALTER TABLE contact_requests
    ADD CONSTRAINT contact_requests_risk_level_check
    CHECK (risk_level IN ('LOW', 'MEDIUM', 'HIGH'));

CREATE INDEX IF NOT EXISTS idx_contact_requests_review_required
    ON contact_requests (tenant_id, receiver_user_id, created_at DESC, request_id)
    WHERE review_required = true AND status = 'PENDING';

COMMIT;
