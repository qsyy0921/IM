CREATE TABLE IF NOT EXISTS ai_eval_runs (
    tenant_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    suite_id TEXT NOT NULL,
    stage TEXT NOT NULL DEFAULT '',
    adapter TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'PENDING',
    case_count INTEGER NOT NULL DEFAULT 0,
    passed_count INTEGER NOT NULL DEFAULT 0,
    failed_count INTEGER NOT NULL DEFAULT 0,
    skipped_count INTEGER NOT NULL DEFAULT 0,
    summary_ref TEXT NOT NULL DEFAULT '',
    report_ref TEXT NOT NULL DEFAULT '',
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, run_id),
    CONSTRAINT ck_ai_eval_runs_status CHECK (status IN ('PENDING', 'RUNNING', 'PASSED', 'FAILED')),
    CONSTRAINT ck_ai_eval_runs_required_fields CHECK (
        btrim(tenant_id) <> ''
        AND btrim(run_id) <> ''
        AND btrim(suite_id) <> ''
    ),
    CONSTRAINT ck_ai_eval_runs_counts CHECK (
        case_count >= 0
        AND passed_count >= 0
        AND failed_count >= 0
        AND skipped_count >= 0
        AND passed_count + failed_count + skipped_count <= case_count
    ),
    CONSTRAINT ck_ai_eval_runs_metadata_object CHECK (jsonb_typeof(metadata_json) = 'object')
);

CREATE INDEX IF NOT EXISTS idx_ai_eval_runs_tenant_suite
    ON ai_eval_runs (tenant_id, suite_id, run_id);

CREATE INDEX IF NOT EXISTS idx_ai_eval_runs_tenant_status
    ON ai_eval_runs (tenant_id, status, run_id);

CREATE INDEX IF NOT EXISTS idx_ai_eval_runs_updated_at
    ON ai_eval_runs (tenant_id, updated_at DESC);
