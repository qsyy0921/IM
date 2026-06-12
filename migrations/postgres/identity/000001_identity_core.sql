CREATE TABLE IF NOT EXISTS identity_users (
    tenant_id text NOT NULL,
    user_id text NOT NULL,
    status text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id)
);

CREATE TABLE IF NOT EXISTS identity_devices (
    tenant_id text NOT NULL,
    user_id text NOT NULL,
    device_id text NOT NULL,
    status text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    revoked_by text NOT NULL DEFAULT '',
    revoke_reason text NOT NULL DEFAULT '',
    PRIMARY KEY (tenant_id, user_id, device_id)
);

CREATE INDEX IF NOT EXISTS idx_identity_devices_user
    ON identity_devices (tenant_id, user_id, status, updated_at DESC);

CREATE TABLE IF NOT EXISTS identity_sessions (
    tenant_id text NOT NULL,
    user_id text NOT NULL,
    device_id text NOT NULL,
    session_id text NOT NULL,
    status text NOT NULL,
    audience text NOT NULL,
    issued_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    revoked_by text NOT NULL DEFAULT '',
    revoke_reason text NOT NULL DEFAULT '',
    trace_id text NOT NULL DEFAULT '',
    request_id text NOT NULL DEFAULT '',
    PRIMARY KEY (tenant_id, user_id, device_id, session_id)
);

CREATE INDEX IF NOT EXISTS idx_identity_sessions_device
    ON identity_sessions (tenant_id, user_id, device_id, status, expires_at DESC);
