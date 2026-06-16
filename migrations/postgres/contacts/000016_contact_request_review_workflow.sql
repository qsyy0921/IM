BEGIN;

ALTER TABLE contact_requests
    DROP CONSTRAINT IF EXISTS contact_requests_status_check;

ALTER TABLE contact_requests
    ADD CONSTRAINT contact_requests_status_check
    CHECK (status IN ('PENDING', 'REVIEW_REQUIRED', 'ACCEPTED', 'DECLINED', 'CANCELED', 'EXPIRED'));

DROP INDEX IF EXISTS uq_contact_requests_pending_pair;

CREATE UNIQUE INDEX IF NOT EXISTS uq_contact_requests_pending_pair
    ON contact_requests (
        tenant_id,
        LEAST(sender_user_id, receiver_user_id),
        GREATEST(sender_user_id, receiver_user_id)
    )
    WHERE status IN ('PENDING', 'REVIEW_REQUIRED');

DROP INDEX IF EXISTS idx_contact_requests_review_required;

CREATE INDEX IF NOT EXISTS idx_contact_requests_review_required
    ON contact_requests (tenant_id, receiver_user_id, created_at DESC, request_id)
    WHERE review_required = true AND status IN ('PENDING', 'REVIEW_REQUIRED');

CREATE TABLE IF NOT EXISTS contact_request_review_audit (
    audit_id            BIGSERIAL   PRIMARY KEY,
    tenant_id           TEXT        NOT NULL,
    request_id          TEXT        NOT NULL,
    previous_status     TEXT        NOT NULL,
    next_status         TEXT        NOT NULL,
    decision            TEXT        NOT NULL,
    operator            TEXT        NOT NULL,
    reason              TEXT        NOT NULL DEFAULT '',
    risk_level          TEXT        NOT NULL,
    review_required     BOOLEAN     NOT NULL,
    reviewed_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (previous_status IN ('PENDING', 'REVIEW_REQUIRED', 'ACCEPTED', 'DECLINED', 'CANCELED', 'EXPIRED')),
    CHECK (next_status IN ('PENDING', 'REVIEW_REQUIRED', 'ACCEPTED', 'DECLINED', 'CANCELED', 'EXPIRED')),
    CHECK (decision IN ('APPROVE', 'DECLINE')),
    CHECK (risk_level IN ('LOW', 'MEDIUM', 'HIGH'))
);

CREATE INDEX IF NOT EXISTS idx_contact_request_review_audit_request
    ON contact_request_review_audit (tenant_id, request_id, reviewed_at DESC);

CREATE INDEX IF NOT EXISTS idx_contact_request_review_audit_operator
    ON contact_request_review_audit (tenant_id, operator, reviewed_at DESC);

COMMIT;
