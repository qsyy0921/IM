BEGIN;

CREATE TABLE IF NOT EXISTS contact_tenant_privacy_defaults (
    tenant_id              TEXT        PRIMARY KEY,
    allow_contact_requests BOOLEAN     NOT NULL DEFAULT true,
    version                BIGINT      NOT NULL,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (version > 0)
);

COMMIT;
