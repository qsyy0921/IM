CREATE TABLE IF NOT EXISTS notification_requests (
    tenant_id TEXT NOT NULL,
    request_id TEXT NOT NULL,
    requester_service TEXT NOT NULL,
    requester_user_id TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL,
    command_hash TEXT NOT NULL,
    channel TEXT NOT NULL,
    recipient_ref TEXT NOT NULL,
    destination_hash TEXT NOT NULL,
    destination_masked TEXT NOT NULL DEFAULT '',
    template_key TEXT NOT NULL,
    template_version TEXT NOT NULL,
    locale TEXT NOT NULL DEFAULT '',
    priority TEXT NOT NULL DEFAULT 'NORMAL',
    template_variables_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    secret_payload_ciphertext BYTEA NOT NULL DEFAULT ''::bytea,
    secret_payload_key_version TEXT NOT NULL DEFAULT '',
    secret_payload_expires_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'ACCEPTED',
    attempt_count INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    last_failure_class TEXT NOT NULL DEFAULT '',
    last_public_error TEXT NOT NULL DEFAULT '',
    correlation_id TEXT NOT NULL DEFAULT '',
    causation_id TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at TIMESTAMPTZ,
    dead_lettered_at TIMESTAMPTZ,
    canceled_at TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, request_id),
    UNIQUE (tenant_id, requester_service, idempotency_key),
    CONSTRAINT ck_notification_requests_channel CHECK (channel IN ('EMAIL', 'SMS', 'APNS', 'FCM', 'SYSTEM')),
    CONSTRAINT ck_notification_requests_priority CHECK (priority IN ('LOW', 'NORMAL', 'HIGH')),
    CONSTRAINT ck_notification_requests_status CHECK (status IN ('ACCEPTED', 'SCHEDULED', 'SENDING', 'RETRY_WAIT', 'DELIVERED', 'DLQ', 'CANCELED')),
    CONSTRAINT ck_notification_requests_attempt_count CHECK (attempt_count >= 0),
    CONSTRAINT ck_notification_requests_template_variables_object CHECK (jsonb_typeof(template_variables_json) = 'object'),
    CONSTRAINT ck_notification_requests_required_fields CHECK (
        btrim(tenant_id) <> ''
        AND btrim(request_id) <> ''
        AND btrim(requester_service) <> ''
        AND btrim(idempotency_key) <> ''
        AND btrim(command_hash) <> ''
        AND btrim(recipient_ref) <> ''
        AND btrim(destination_hash) <> ''
        AND btrim(template_key) <> ''
        AND btrim(template_version) <> ''
    )
);

CREATE TABLE IF NOT EXISTS notification_templates (
    tenant_id TEXT NOT NULL,
    template_key TEXT NOT NULL,
    template_version TEXT NOT NULL,
    channel TEXT NOT NULL,
    locale TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'DRAFT',
    subject_template TEXT NOT NULL DEFAULT '',
    body_template_ref TEXT NOT NULL,
    checksum TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deprecated_at TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, template_key, template_version, channel, locale),
    CONSTRAINT ck_notification_templates_channel CHECK (channel IN ('EMAIL', 'SMS', 'APNS', 'FCM', 'SYSTEM')),
    CONSTRAINT ck_notification_templates_status CHECK (status IN ('DRAFT', 'PUBLISHED', 'DEPRECATED')),
    CONSTRAINT ck_notification_templates_required_fields CHECK (
        btrim(tenant_id) <> ''
        AND btrim(template_key) <> ''
        AND btrim(template_version) <> ''
        AND btrim(body_template_ref) <> ''
        AND btrim(checksum) <> ''
    )
);

CREATE TABLE IF NOT EXISTS notification_delivery_attempts (
    tenant_id TEXT NOT NULL,
    attempt_id TEXT NOT NULL,
    request_id TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    provider_message_id_hash TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    failure_class TEXT NOT NULL DEFAULT '',
    public_error TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    retry_after TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, attempt_id),
    CONSTRAINT fk_notification_delivery_attempts_request
        FOREIGN KEY (tenant_id, request_id)
        REFERENCES notification_requests (tenant_id, request_id),
    CONSTRAINT ck_notification_delivery_attempts_status CHECK (status IN ('SENDING', 'SUCCEEDED', 'FAILED')),
    CONSTRAINT ck_notification_delivery_attempts_required_fields CHECK (
        btrim(tenant_id) <> ''
        AND btrim(attempt_id) <> ''
        AND btrim(request_id) <> ''
        AND btrim(provider_id) <> ''
    )
);

CREATE TABLE IF NOT EXISTS notification_provider_routes (
    tenant_id TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    channel TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE',
    priority INT NOT NULL DEFAULT 100,
    config_ref TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, provider_id, channel),
    CONSTRAINT ck_notification_provider_routes_channel CHECK (channel IN ('EMAIL', 'SMS', 'APNS', 'FCM', 'SYSTEM')),
    CONSTRAINT ck_notification_provider_routes_status CHECK (status IN ('ACTIVE', 'DISABLED')),
    CONSTRAINT ck_notification_provider_routes_priority CHECK (priority >= 0),
    CONSTRAINT ck_notification_provider_routes_required_fields CHECK (
        btrim(tenant_id) <> ''
        AND btrim(provider_id) <> ''
        AND btrim(config_ref) <> ''
    )
);

CREATE TABLE IF NOT EXISTS notification_suppressions (
    tenant_id TEXT NOT NULL,
    suppression_id TEXT NOT NULL,
    channel TEXT NOT NULL,
    recipient_ref TEXT NOT NULL,
    destination_hash TEXT NOT NULL,
    reason TEXT NOT NULL,
    source TEXT NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, suppression_id),
    CONSTRAINT ck_notification_suppressions_channel CHECK (channel IN ('EMAIL', 'SMS', 'APNS', 'FCM', 'SYSTEM')),
    CONSTRAINT ck_notification_suppressions_required_fields CHECK (
        btrim(tenant_id) <> ''
        AND btrim(suppression_id) <> ''
        AND btrim(recipient_ref) <> ''
        AND btrim(destination_hash) <> ''
        AND btrim(reason) <> ''
        AND btrim(source) <> ''
    )
);

CREATE TABLE IF NOT EXISTS notification_outbox (
    event_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    request_id TEXT NOT NULL,
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
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, event_id),
    CONSTRAINT fk_notification_outbox_request
        FOREIGN KEY (tenant_id, request_id)
        REFERENCES notification_requests (tenant_id, request_id),
    CONSTRAINT ck_notification_outbox_status CHECK (status IN ('PENDING', 'PUBLISHED', 'DLQ')),
    CONSTRAINT ck_notification_outbox_retry_count CHECK (retry_count >= 0),
    CONSTRAINT ck_notification_outbox_payload_object CHECK (jsonb_typeof(payload_json) = 'object'),
    CONSTRAINT ck_notification_outbox_required_fields CHECK (
        btrim(event_id) <> ''
        AND btrim(tenant_id) <> ''
        AND btrim(request_id) <> ''
        AND btrim(event_type) <> ''
        AND event_version > 0
        AND btrim(partition_key) <> ''
    )
);

CREATE INDEX IF NOT EXISTS idx_notification_requests_status_ready
    ON notification_requests (tenant_id, status, next_attempt_at, created_at)
    WHERE status IN ('ACCEPTED', 'SCHEDULED', 'RETRY_WAIT');

CREATE INDEX IF NOT EXISTS idx_notification_requests_recipient
    ON notification_requests (tenant_id, recipient_ref, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_notification_outbox_ready
    ON notification_outbox (tenant_id, status, available_at, created_at)
    WHERE status = 'PENDING';

CREATE INDEX IF NOT EXISTS idx_notification_suppressions_active
    ON notification_suppressions (tenant_id, channel, destination_hash, starts_at, expires_at);
