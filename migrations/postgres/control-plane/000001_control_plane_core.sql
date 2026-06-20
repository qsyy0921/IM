CREATE TABLE IF NOT EXISTS control_config_bundles (
    tenant_id TEXT NOT NULL,
    environment TEXT NOT NULL,
    config_kind TEXT NOT NULL,
    bundle_key TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, environment, config_kind, bundle_key),
    CONSTRAINT ck_control_config_bundles_status
        CHECK (status IN ('ACTIVE', 'DISABLED'))
);

CREATE TABLE IF NOT EXISTS control_config_versions (
    tenant_id TEXT NOT NULL,
    environment TEXT NOT NULL,
    config_kind TEXT NOT NULL,
    bundle_key TEXT NOT NULL,
    version TEXT NOT NULL,
    schema_version TEXT NOT NULL,
    payload_json JSONB NOT NULL,
    payload_checksum TEXT NOT NULL,
    command_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE',
    effective_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NULL,
    published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    rolled_back_at TIMESTAMPTZ NULL,
    approval_ref TEXT NOT NULL DEFAULT '',
    operator_ref TEXT NOT NULL DEFAULT '',
    reason_ref TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    causation_id TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, environment, config_kind, bundle_key, version),
    CONSTRAINT fk_control_config_versions_bundle
        FOREIGN KEY (tenant_id, environment, config_kind, bundle_key)
        REFERENCES control_config_bundles (tenant_id, environment, config_kind, bundle_key),
    CONSTRAINT ck_control_config_versions_status
        CHECK (status IN ('PUBLISHED', 'ACTIVE', 'ROLLED_BACK', 'EXPIRED')),
    CONSTRAINT ck_control_config_versions_checksum
        CHECK (payload_checksum LIKE 'sha256:%'),
    CONSTRAINT ck_control_config_versions_expires
        CHECK (expires_at IS NULL OR expires_at > effective_at)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_control_config_versions_idempotency
    ON control_config_versions (tenant_id, environment, config_kind, bundle_key, idempotency_key);

CREATE TABLE IF NOT EXISTS control_rollout_rules (
    tenant_id TEXT NOT NULL,
    environment TEXT NOT NULL,
    config_kind TEXT NOT NULL,
    bundle_key TEXT NOT NULL,
    version TEXT NOT NULL,
    rule_id TEXT NOT NULL,
    ring TEXT NOT NULL DEFAULT '',
    percentage INTEGER NOT NULL DEFAULT 100,
    tenant_allowlist_hash TEXT NOT NULL DEFAULT '',
    service_version_constraint TEXT NOT NULL DEFAULT '',
    starts_at TIMESTAMPTZ NULL,
    ends_at TIMESTAMPTZ NULL,
    PRIMARY KEY (tenant_id, environment, config_kind, bundle_key, version, rule_id),
    CONSTRAINT fk_control_rollout_rules_version
        FOREIGN KEY (tenant_id, environment, config_kind, bundle_key, version)
        REFERENCES control_config_versions (tenant_id, environment, config_kind, bundle_key, version),
    CONSTRAINT ck_control_rollout_rules_percentage
        CHECK (percentage >= 0 AND percentage <= 100)
);

CREATE TABLE IF NOT EXISTS control_applied_acks (
    tenant_id TEXT NOT NULL,
    environment TEXT NOT NULL,
    service_name TEXT NOT NULL,
    instance_ref TEXT NOT NULL,
    config_kind TEXT NOT NULL,
    bundle_key TEXT NOT NULL,
    version TEXT NOT NULL,
    service_version TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'IN_SYNC',
    last_error_class TEXT NOT NULL DEFAULT '',
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, environment, service_name, instance_ref, config_kind, bundle_key),
    CONSTRAINT ck_control_applied_acks_status
        CHECK (status IN ('IN_SYNC', 'STALE_VERSION', 'MISSING_ACK', 'APPLY_FAILED', 'UNKNOWN_INSTANCE'))
);

CREATE TABLE IF NOT EXISTS control_outbox (
    event_id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    event_version INTEGER NOT NULL DEFAULT 1,
    partition_key TEXT NOT NULL,
    payload_json JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    retry_count INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    next_retry_at TIMESTAMPTZ NULL,
    published_at TIMESTAMPTZ NULL,
    dead_lettered_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_control_outbox_status
        CHECK (status IN ('PENDING', 'PUBLISHED', 'DLQ'))
);

CREATE INDEX IF NOT EXISTS idx_control_config_versions_current
    ON control_config_versions (tenant_id, environment, config_kind, bundle_key, status, effective_at DESC);

CREATE INDEX IF NOT EXISTS idx_control_outbox_ready
    ON control_outbox (status, available_at, next_retry_at, partition_key);
