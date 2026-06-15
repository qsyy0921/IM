BEGIN;

CREATE TABLE IF NOT EXISTS contact_tenant_request_source_policies (
    tenant_id              TEXT        NOT NULL,
    source_type            TEXT        NOT NULL,
    allow_contact_requests BOOLEAN     NOT NULL DEFAULT true,
    version                BIGINT      NOT NULL,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, source_type),
    CHECK (source_type IN ('DIRECT', 'SEARCH', 'GROUP', 'INVITE_LINK', 'QR_CODE', 'IMPORT')),
    CHECK (version > 0)
);

COMMIT;
