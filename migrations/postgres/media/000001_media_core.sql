CREATE TABLE IF NOT EXISTS media_assets (
    tenant_id TEXT NOT NULL,
    asset_id TEXT NOT NULL,
    owner_user_id TEXT NOT NULL,
    conversation_id TEXT NOT NULL,
    media_kind TEXT NOT NULL,
    content_type TEXT NOT NULL,
    file_name TEXT NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL,
    sha256 TEXT NOT NULL,
    object_key TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'UPLOAD_PENDING',
    scan_status TEXT NOT NULL DEFAULT 'PENDING',
    thumbnail_status TEXT NOT NULL DEFAULT 'PENDING',
    transcode_status TEXT NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    uploaded_at TIMESTAMPTZ,
    ready_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, asset_id),
    UNIQUE (tenant_id, object_key),
    CONSTRAINT ck_media_assets_kind CHECK (media_kind IN ('IMAGE', 'FILE', 'VOICE', 'VIDEO')),
    CONSTRAINT ck_media_assets_status CHECK (status IN ('UPLOAD_PENDING', 'UPLOADED', 'PROCESSING', 'READY', 'QUARANTINED', 'FAILED', 'DELETED', 'EXPIRED')),
    CONSTRAINT ck_media_assets_scan_status CHECK (scan_status IN ('PENDING', 'PASSED', 'SKIPPED', 'FAILED')),
    CONSTRAINT ck_media_assets_thumbnail_status CHECK (thumbnail_status IN ('PENDING', 'PASSED', 'SKIPPED', 'FAILED')),
    CONSTRAINT ck_media_assets_transcode_status CHECK (transcode_status IN ('PENDING', 'PASSED', 'SKIPPED', 'FAILED')),
    CONSTRAINT ck_media_assets_required_fields CHECK (
        btrim(tenant_id) <> ''
        AND btrim(asset_id) <> ''
        AND btrim(owner_user_id) <> ''
        AND btrim(conversation_id) <> ''
        AND btrim(content_type) <> ''
        AND size_bytes > 0
        AND btrim(sha256) <> ''
        AND btrim(object_key) <> ''
    )
);

CREATE TABLE IF NOT EXISTS media_upload_sessions (
    tenant_id TEXT NOT NULL,
    upload_session_id TEXT NOT NULL,
    asset_id TEXT NOT NULL,
    owner_user_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    command_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    expires_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, upload_session_id),
    UNIQUE (tenant_id, owner_user_id, idempotency_key),
    CONSTRAINT fk_media_upload_sessions_asset
        FOREIGN KEY (tenant_id, asset_id)
        REFERENCES media_assets (tenant_id, asset_id),
    CONSTRAINT ck_media_upload_sessions_status CHECK (status IN ('PENDING', 'COMPLETED', 'EXPIRED')),
    CONSTRAINT ck_media_upload_sessions_required_fields CHECK (
        btrim(tenant_id) <> ''
        AND btrim(upload_session_id) <> ''
        AND btrim(asset_id) <> ''
        AND btrim(owner_user_id) <> ''
        AND btrim(idempotency_key) <> ''
        AND btrim(command_hash) <> ''
    )
);

CREATE TABLE IF NOT EXISTS media_processing_jobs (
    tenant_id TEXT NOT NULL,
    job_id TEXT NOT NULL,
    asset_id TEXT NOT NULL,
    job_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    attempt_count INT NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    dead_lettered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, job_id),
    CONSTRAINT fk_media_processing_jobs_asset
        FOREIGN KEY (tenant_id, asset_id)
        REFERENCES media_assets (tenant_id, asset_id),
    CONSTRAINT ck_media_processing_jobs_type CHECK (job_type IN ('SCAN', 'THUMBNAIL', 'TRANSCODE')),
    CONSTRAINT ck_media_processing_jobs_status CHECK (status IN ('PENDING', 'RUNNING', 'SUCCEEDED', 'FAILED', 'DLQ')),
    CONSTRAINT ck_media_processing_jobs_attempt_count CHECK (attempt_count >= 0)
);

CREATE TABLE IF NOT EXISTS media_outbox (
    event_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    asset_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    event_version INT NOT NULL DEFAULT 1,
    partition_key TEXT NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'PENDING',
    retry_count INT NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    next_retry_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    dead_lettered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, event_id),
    CONSTRAINT fk_media_outbox_asset
        FOREIGN KEY (tenant_id, asset_id)
        REFERENCES media_assets (tenant_id, asset_id),
    CONSTRAINT ck_media_outbox_status CHECK (status IN ('PENDING', 'PUBLISHED', 'DLQ')),
    CONSTRAINT ck_media_outbox_payload_object CHECK (jsonb_typeof(payload_json) = 'object'),
    CONSTRAINT ck_media_outbox_required_fields CHECK (
        btrim(event_id) <> ''
        AND btrim(tenant_id) <> ''
        AND btrim(asset_id) <> ''
        AND btrim(event_type) <> ''
        AND event_version > 0
        AND btrim(partition_key) <> ''
    )
);

CREATE TABLE IF NOT EXISTS media_access_audit (
    tenant_id TEXT NOT NULL,
    audit_id TEXT NOT NULL,
    asset_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    conversation_id TEXT NOT NULL,
    message_id TEXT NOT NULL DEFAULT '',
    variant TEXT NOT NULL,
    decision TEXT NOT NULL,
    decision_source TEXT NOT NULL,
    request_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, audit_id),
    CONSTRAINT ck_media_access_audit_variant CHECK (variant IN ('ORIGINAL', 'THUMBNAIL', 'TRANSCODED')),
    CONSTRAINT ck_media_access_audit_decision CHECK (decision IN ('ALLOW', 'DENY')),
    CONSTRAINT ck_media_access_audit_required_fields CHECK (
        btrim(tenant_id) <> ''
        AND btrim(audit_id) <> ''
        AND btrim(asset_id) <> ''
        AND btrim(user_id) <> ''
        AND btrim(conversation_id) <> ''
        AND btrim(decision_source) <> ''
    )
);

CREATE INDEX IF NOT EXISTS idx_media_assets_tenant_owner
    ON media_assets (tenant_id, owner_user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_media_assets_tenant_conversation
    ON media_assets (tenant_id, conversation_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_media_upload_sessions_asset
    ON media_upload_sessions (tenant_id, asset_id);

CREATE INDEX IF NOT EXISTS idx_media_processing_jobs_ready
    ON media_processing_jobs (status, next_retry_at, created_at)
    WHERE status IN ('PENDING', 'FAILED');

CREATE INDEX IF NOT EXISTS idx_media_outbox_ready
    ON media_outbox (tenant_id, status, available_at, created_at)
    WHERE status = 'PENDING';
