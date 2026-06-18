CREATE TABLE IF NOT EXISTS memory_structured_events (
    tenant_id TEXT NOT NULL,
    memory_event_id TEXT NOT NULL,
    scope_type TEXT NOT NULL,
    scope_id TEXT NOT NULL DEFAULT '',
    conversation_id TEXT NOT NULL DEFAULT '',
    topic TEXT NOT NULL DEFAULT '',
    event_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    review_state TEXT NOT NULL DEFAULT 'UNREVIEWED',
    fact_text TEXT NOT NULL DEFAULT '',
    actor_user_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    audience_user_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    valid_from_seq BIGINT,
    valid_to_seq BIGINT,
    valid_from_at TIMESTAMPTZ,
    valid_to_at TIMESTAMPTZ,
    supersedes_event_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    contradicts_event_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    confidence NUMERIC(5,4) NOT NULL DEFAULT 0,
    visibility_version BIGINT NOT NULL DEFAULT 0,
    extraction_version TEXT NOT NULL DEFAULT '',
    source_projection_version BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, memory_event_id),
    CONSTRAINT ck_memory_structured_events_scope CHECK (
        scope_type IN ('CONVERSATION', 'PROJECT', 'PERSONAL', 'TENANT')
    ),
    CONSTRAINT ck_memory_structured_events_type CHECK (
        event_type IN (
            'TASK',
            'DECISION',
            'STATUS',
            'BLOCKER',
            'FILE',
            'PREFERENCE_SIGNAL',
            'ROLE_SIGNAL',
            'PROFILE_SIGNAL'
        )
    ),
    CONSTRAINT ck_memory_structured_events_status CHECK (
        status IN ('PENDING', 'ACTIVE', 'SUPERSEDED', 'REJECTED', 'ARCHIVED', 'DELETED')
    ),
    CONSTRAINT ck_memory_structured_events_review CHECK (
        review_state IN ('UNREVIEWED', 'NEEDS_REVIEW', 'APPROVED', 'REJECTED')
    ),
    CONSTRAINT ck_memory_structured_events_confidence CHECK (
        confidence >= 0 AND confidence <= 1
    ),
    CONSTRAINT ck_memory_structured_events_valid_seq CHECK (
        valid_to_seq IS NULL OR valid_from_seq IS NULL OR valid_to_seq >= valid_from_seq
    ),
    CONSTRAINT ck_memory_structured_events_profile_signal_review CHECK (
        event_type NOT IN ('PREFERENCE_SIGNAL', 'ROLE_SIGNAL', 'PROFILE_SIGNAL')
        OR review_state IN ('NEEDS_REVIEW', 'APPROVED', 'REJECTED')
    )
);

CREATE INDEX IF NOT EXISTS idx_memory_structured_events_scope
    ON memory_structured_events (tenant_id, scope_type, scope_id, status, valid_from_seq);

CREATE INDEX IF NOT EXISTS idx_memory_structured_events_conversation
    ON memory_structured_events (tenant_id, conversation_id, valid_from_seq);

CREATE INDEX IF NOT EXISTS idx_memory_structured_events_text
    ON memory_structured_events
    USING GIN (to_tsvector('simple', fact_text))
    WHERE status = 'ACTIVE';

CREATE TABLE IF NOT EXISTS memory_event_source_refs (
    tenant_id TEXT NOT NULL,
    memory_event_id TEXT NOT NULL,
    source_ref_id TEXT NOT NULL,
    source_type TEXT NOT NULL,
    source_id TEXT NOT NULL,
    source_event_id TEXT NOT NULL,
    conversation_id TEXT NOT NULL DEFAULT '',
    conversation_seq BIGINT,
    occurred_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, memory_event_id, source_ref_id),
    CONSTRAINT fk_memory_event_source_refs_event FOREIGN KEY (tenant_id, memory_event_id)
        REFERENCES memory_structured_events (tenant_id, memory_event_id) ON DELETE CASCADE,
    CONSTRAINT ck_memory_event_source_refs_type CHECK (
        source_type IN ('MESSAGE', 'TIMELINE_EVENT', 'PROFILE_AGGREGATE', 'SYSTEM')
    ),
    CONSTRAINT ck_memory_event_source_refs_seq CHECK (
        conversation_seq IS NULL OR conversation_seq > 0
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_memory_event_source_refs_source
    ON memory_event_source_refs (tenant_id, memory_event_id, source_type, source_id, source_event_id);

CREATE INDEX IF NOT EXISTS idx_memory_event_source_refs_source_lookup
    ON memory_event_source_refs (tenant_id, source_type, source_id, source_event_id);

CREATE TABLE IF NOT EXISTS memory_membership_projection (
    tenant_id TEXT NOT NULL,
    conversation_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '',
    join_seq BIGINT NOT NULL,
    leave_seq BIGINT,
    member_version BIGINT NOT NULL DEFAULT 0,
    permission_version BIGINT NOT NULL DEFAULT 0,
    updated_by_event_id TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id, user_id),
    CONSTRAINT ck_memory_membership_projection_join_seq CHECK (join_seq > 0),
    CONSTRAINT ck_memory_membership_projection_leave_seq CHECK (leave_seq IS NULL OR leave_seq >= join_seq),
    CONSTRAINT ck_memory_membership_projection_status CHECK (
        status IN ('ACTIVE', 'LEFT', 'REMOVED', 'BANNED', '')
    )
);

CREATE INDEX IF NOT EXISTS idx_memory_membership_projection_user
    ON memory_membership_projection (tenant_id, user_id, conversation_id);

CREATE TABLE IF NOT EXISTS memory_graph_edges (
    tenant_id TEXT NOT NULL,
    edge_id TEXT NOT NULL,
    from_memory_event_id TEXT NOT NULL,
    to_memory_event_id TEXT NOT NULL,
    relation_type TEXT NOT NULL,
    confidence NUMERIC(5,4) NOT NULL DEFAULT 0,
    source_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, edge_id),
    CONSTRAINT fk_memory_graph_edges_from_event FOREIGN KEY (tenant_id, from_memory_event_id)
        REFERENCES memory_structured_events (tenant_id, memory_event_id) ON DELETE CASCADE,
    CONSTRAINT fk_memory_graph_edges_to_event FOREIGN KEY (tenant_id, to_memory_event_id)
        REFERENCES memory_structured_events (tenant_id, memory_event_id) ON DELETE CASCADE,
    CONSTRAINT ck_memory_graph_edges_relation CHECK (
        relation_type IN (
            'MENTIONED_BY',
            'ASSIGNED_TO',
            'SUPPORTS',
            'SUPERSEDES',
            'CONTRADICTS',
            'RELATED_TO'
        )
    ),
    CONSTRAINT ck_memory_graph_edges_confidence CHECK (confidence >= 0 AND confidence <= 1),
    CONSTRAINT ck_memory_graph_edges_not_self CHECK (from_memory_event_id <> to_memory_event_id)
);

CREATE INDEX IF NOT EXISTS idx_memory_graph_edges_from
    ON memory_graph_edges (tenant_id, from_memory_event_id, relation_type);

CREATE INDEX IF NOT EXISTS idx_memory_graph_edges_to
    ON memory_graph_edges (tenant_id, to_memory_event_id, relation_type);

CREATE TABLE IF NOT EXISTS memory_profile_aggregates (
    tenant_id TEXT NOT NULL,
    profile_id TEXT NOT NULL,
    subject_user_id TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_key TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'PENDING',
    review_state TEXT NOT NULL DEFAULT 'UNREVIEWED',
    summary_text TEXT NOT NULL DEFAULT '',
    supporting_memory_event_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    confidence NUMERIC(5,4) NOT NULL DEFAULT 0,
    valid_from_at TIMESTAMPTZ,
    valid_to_at TIMESTAMPTZ,
    updated_by_memory_event_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, profile_id),
    CONSTRAINT ck_memory_profile_aggregates_type CHECK (
        aggregate_type IN ('STYLE', 'SKILL', 'ROLE', 'PREFERENCE', 'INTEREST')
    ),
    CONSTRAINT ck_memory_profile_aggregates_status CHECK (
        status IN ('PENDING', 'ACTIVE', 'SUPERSEDED', 'REJECTED', 'ARCHIVED', 'DELETED')
    ),
    CONSTRAINT ck_memory_profile_aggregates_review CHECK (
        review_state IN ('UNREVIEWED', 'NEEDS_REVIEW', 'APPROVED', 'REJECTED')
    ),
    CONSTRAINT ck_memory_profile_aggregates_confidence CHECK (confidence >= 0 AND confidence <= 1)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_memory_profile_aggregates_subject_key
    ON memory_profile_aggregates (tenant_id, subject_user_id, aggregate_type, aggregate_key)
    WHERE status IN ('PENDING', 'ACTIVE');

CREATE TABLE IF NOT EXISTS memory_projection_checkpoints (
    consumer_group TEXT NOT NULL,
    topic TEXT NOT NULL,
    partition_id INTEGER NOT NULL,
    offset_value BIGINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer_group, topic, partition_id),
    CONSTRAINT ck_memory_projection_checkpoints_partition CHECK (partition_id >= 0),
    CONSTRAINT ck_memory_projection_checkpoints_offset CHECK (offset_value >= 0)
);
