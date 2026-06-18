CREATE TABLE IF NOT EXISTS api_gateway_rate_limit_tenant_plans (
    tenant_id            TEXT             PRIMARY KEY,
    requests_per_second  DOUBLE PRECISION NOT NULL,
    burst                INTEGER          NOT NULL,
    enabled              BOOLEAN          NOT NULL DEFAULT true,
    source               TEXT             NOT NULL DEFAULT 'manual',
    updated_at           TIMESTAMPTZ      NOT NULL DEFAULT now(),
    CHECK (tenant_id <> ''),
    CHECK (requests_per_second > 0),
    CHECK (burst > 0),
    CHECK (source <> '')
);

CREATE INDEX IF NOT EXISTS idx_api_gateway_rate_limit_tenant_plans_enabled
    ON api_gateway_rate_limit_tenant_plans (enabled, updated_at DESC);
