CREATE TABLE IF NOT EXISTS skill_registry_definitions (
    tenant_id TEXT NOT NULL,
    skill_id TEXT NOT NULL,
    display_name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    version TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE',
    tool_name TEXT NOT NULL,
    allowed_actions_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    input_schema_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    output_schema_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    permission_scope TEXT NOT NULL DEFAULT '',
    risk_level TEXT NOT NULL,
    requires_approval BOOLEAN NOT NULL DEFAULT TRUE,
    audit_event_type TEXT NOT NULL DEFAULT '',
    owner_service TEXT NOT NULL DEFAULT '',
    tags_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, skill_id),
    UNIQUE (tenant_id, display_name, version),
    CONSTRAINT ck_skill_registry_status CHECK (status IN ('ACTIVE', 'DISABLED')),
    CONSTRAINT ck_skill_registry_required_fields CHECK (
        btrim(tenant_id) <> ''
        AND btrim(skill_id) <> ''
        AND btrim(display_name) <> ''
        AND btrim(version) <> ''
        AND btrim(tool_name) <> ''
        AND btrim(risk_level) <> ''
    ),
    CONSTRAINT ck_skill_registry_allowed_actions_array CHECK (jsonb_typeof(allowed_actions_json) = 'array'),
    CONSTRAINT ck_skill_registry_input_schema_object CHECK (jsonb_typeof(input_schema_json) = 'object'),
    CONSTRAINT ck_skill_registry_output_schema_object CHECK (jsonb_typeof(output_schema_json) = 'object'),
    CONSTRAINT ck_skill_registry_tags_array CHECK (jsonb_typeof(tags_json) = 'array'),
    CONSTRAINT ck_skill_registry_metadata_object CHECK (jsonb_typeof(metadata_json) = 'object')
);

CREATE INDEX IF NOT EXISTS idx_skill_registry_tenant_status
    ON skill_registry_definitions (tenant_id, status, skill_id);

CREATE INDEX IF NOT EXISTS idx_skill_registry_tenant_tool
    ON skill_registry_definitions (tenant_id, tool_name, skill_id);

CREATE INDEX IF NOT EXISTS idx_skill_registry_tenant_owner
    ON skill_registry_definitions (tenant_id, owner_service, skill_id)
    WHERE owner_service <> '';
