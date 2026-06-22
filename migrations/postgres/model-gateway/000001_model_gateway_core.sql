CREATE TABLE IF NOT EXISTS model_invocations (
    tenant_id TEXT NOT NULL,
    invocation_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL DEFAULT '',
    command_hash TEXT NOT NULL,
    caller_service TEXT NOT NULL,
    caller_use_case TEXT NOT NULL,
    request_type TEXT NOT NULL,
    data_class TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    model_id TEXT NOT NULL,
    route_version TEXT NOT NULL DEFAULT 'local-v1',
    prompt_hash TEXT NOT NULL,
    output_hash TEXT NOT NULL DEFAULT '',
    output_schema_version INTEGER NOT NULL DEFAULT 0,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    estimated_cost_microunits BIGINT NOT NULL DEFAULT 0,
    status TEXT NOT NULL,
    failure_class TEXT NOT NULL DEFAULT '',
    provider_latency_ms BIGINT NOT NULL DEFAULT 0,
    timeout_ms BIGINT NOT NULL DEFAULT 0,
    max_output_tokens INTEGER NOT NULL DEFAULT 0,
    prompt_schema_version INTEGER NOT NULL DEFAULT 0,
    correlation_id TEXT NOT NULL DEFAULT '',
    causation_id TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ NULL,
    PRIMARY KEY (tenant_id, invocation_id),
    CONSTRAINT ck_model_invocations_request_type
        CHECK (request_type IN ('TEXT_GENERATION', 'EMBEDDING', 'RERANK', 'CLASSIFICATION', 'EXTRACTION', 'EVAL_JUDGE')),
    CONSTRAINT ck_model_invocations_data_class
        CHECK (data_class IN ('LOW_SENSITIVE', 'BUSINESS_INTERNAL', 'USER_CONTENT', 'SECURITY_SENSITIVE')),
    CONSTRAINT ck_model_invocations_status
        CHECK (status IN ('PENDING', 'SUCCEEDED', 'FAILED')),
    CONSTRAINT ck_model_invocations_token_counts
        CHECK (input_tokens >= 0 AND output_tokens >= 0 AND total_tokens >= 0),
    CONSTRAINT ck_model_invocations_cost
        CHECK (estimated_cost_microunits >= 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_model_invocations_idempotency
    ON model_invocations (tenant_id, caller_service, idempotency_key)
    WHERE idempotency_key <> '';

CREATE INDEX IF NOT EXISTS idx_model_invocations_created
    ON model_invocations (tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS model_budget_windows (
    tenant_id TEXT NOT NULL,
    budget_key TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    model_id TEXT NOT NULL,
    caller_service TEXT NOT NULL,
    caller_use_case TEXT NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    window_end TIMESTAMPTZ NOT NULL,
    request_count BIGINT NOT NULL DEFAULT 0,
    token_count BIGINT NOT NULL DEFAULT 0,
    estimated_cost_microunits BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, budget_key, window_start),
    CONSTRAINT ck_model_budget_windows_counts
        CHECK (request_count >= 0 AND token_count >= 0 AND estimated_cost_microunits >= 0)
);

CREATE TABLE IF NOT EXISTS model_provider_route_snapshots (
    tenant_id TEXT NOT NULL,
    route_version TEXT NOT NULL,
    caller_service TEXT NOT NULL,
    caller_use_case TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    model_id TEXT NOT NULL,
    policy_hash TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, route_version),
    CONSTRAINT ck_model_provider_route_snapshots_status
        CHECK (status IN ('ACTIVE', 'DISABLED'))
);

CREATE TABLE IF NOT EXISTS model_provider_failures (
    tenant_id TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    model_id TEXT NOT NULL,
    failure_class TEXT NOT NULL,
    failure_count BIGINT NOT NULL DEFAULT 0,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, provider_id, model_id, failure_class),
    CONSTRAINT ck_model_provider_failures_count
        CHECK (failure_count >= 0)
);

CREATE TABLE IF NOT EXISTS model_outbox (
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
    CONSTRAINT ck_model_outbox_status
        CHECK (status IN ('PENDING', 'PUBLISHED', 'DLQ'))
);

CREATE INDEX IF NOT EXISTS idx_model_outbox_ready
    ON model_outbox (status, available_at, next_retry_at, partition_key);
