ALTER TABLE contact_request_review_audit
    ADD COLUMN IF NOT EXISTS source_type TEXT NOT NULL DEFAULT 'DIRECT';

ALTER TABLE contact_request_review_audit
    DROP CONSTRAINT IF EXISTS contact_request_review_audit_source_type_check;

ALTER TABLE contact_request_review_audit
    ADD CONSTRAINT contact_request_review_audit_source_type_check
    CHECK (source_type IN ('DIRECT', 'SEARCH', 'GROUP', 'INVITE_LINK', 'QR_CODE', 'IMPORT'));

CREATE INDEX IF NOT EXISTS idx_contact_request_review_audit_source
    ON contact_request_review_audit (tenant_id, source_type, reviewed_at DESC);
