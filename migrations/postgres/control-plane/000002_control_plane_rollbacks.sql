CREATE TABLE IF NOT EXISTS control_config_rollbacks (
    tenant_id TEXT NOT NULL,
    environment TEXT NOT NULL,
    config_kind TEXT NOT NULL,
    bundle_key TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    target_version TEXT NOT NULL,
    command_hash TEXT NOT NULL,
    approval_ref TEXT NOT NULL DEFAULT '',
    operator_ref TEXT NOT NULL DEFAULT '',
    reason_ref TEXT NOT NULL DEFAULT '',
    correlation_id TEXT NOT NULL DEFAULT '',
    causation_id TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, environment, config_kind, bundle_key, idempotency_key),
    CONSTRAINT fk_control_config_rollbacks_target
        FOREIGN KEY (tenant_id, environment, config_kind, bundle_key, target_version)
        REFERENCES control_config_versions (tenant_id, environment, config_kind, bundle_key, version),
    CONSTRAINT ck_control_config_rollbacks_command_hash
        CHECK (command_hash LIKE 'sha256:%')
);

CREATE INDEX IF NOT EXISTS idx_control_config_rollbacks_target
    ON control_config_rollbacks (tenant_id, environment, config_kind, bundle_key, target_version, created_at DESC);
