CREATE TABLE IF NOT EXISTS policy_revision_state (
    tenant_id TEXT NOT NULL,
    scope_type TEXT NOT NULL,
    scope_id TEXT NOT NULL,
    action TEXT NOT NULL DEFAULT '',
    revision BIGINT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, scope_type, scope_id, action),
    CHECK (tenant_id <> ''),
    CHECK (scope_type <> ''),
    CHECK (scope_id <> ''),
    CHECK (action IN ('', 'SEND', 'EDIT', 'REVOKE', 'DELETE')),
    CHECK (revision > 0)
);

CREATE INDEX IF NOT EXISTS idx_policy_revision_state_tenant_updated
    ON policy_revision_state (tenant_id, updated_at DESC);

CREATE OR REPLACE FUNCTION policy_bump_revision(
    p_tenant_id TEXT,
    p_scope_type TEXT,
    p_scope_id TEXT,
    p_action TEXT
) RETURNS VOID AS $$
DECLARE
    v_action TEXT;
BEGIN
    v_action := COALESCE(p_action, '');
    IF COALESCE(p_tenant_id, '') = '' OR COALESCE(p_scope_type, '') = '' OR COALESCE(p_scope_id, '') = '' THEN
        RETURN;
    END IF;

    INSERT INTO policy_revision_state (
        tenant_id,
        scope_type,
        scope_id,
        action,
        revision,
        updated_at
    ) VALUES (
        p_tenant_id,
        p_scope_type,
        p_scope_id,
        v_action,
        1,
        now()
    )
    ON CONFLICT (tenant_id, scope_type, scope_id, action) DO UPDATE
    SET revision = policy_revision_state.revision + 1,
        updated_at = now();
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION policy_revision_bump_exact_message_rule() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM policy_bump_revision(OLD.tenant_id, 'exact_message_action', OLD.user_id || ':' || OLD.conversation_id, OLD.action);
        RETURN OLD;
    END IF;

    IF TG_OP = 'UPDATE' AND (
        OLD.tenant_id <> NEW.tenant_id OR
        OLD.user_id <> NEW.user_id OR
        OLD.conversation_id <> NEW.conversation_id OR
        OLD.action <> NEW.action
    ) THEN
        PERFORM policy_bump_revision(OLD.tenant_id, 'exact_message_action', OLD.user_id || ':' || OLD.conversation_id, OLD.action);
    END IF;

    PERFORM policy_bump_revision(NEW.tenant_id, 'exact_message_action', NEW.user_id || ':' || NEW.conversation_id, NEW.action);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_policy_revision_exact_message_rule ON policy_message_action_rules;
CREATE TRIGGER trg_policy_revision_exact_message_rule
AFTER INSERT OR UPDATE OR DELETE ON policy_message_action_rules
FOR EACH ROW EXECUTE FUNCTION policy_revision_bump_exact_message_rule();

CREATE OR REPLACE FUNCTION policy_revision_bump_tenant_action() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM policy_bump_revision(OLD.tenant_id, 'tenant_action', '*', OLD.action);
        RETURN OLD;
    END IF;

    IF TG_OP = 'UPDATE' AND (
        OLD.tenant_id <> NEW.tenant_id OR
        OLD.action <> NEW.action
    ) THEN
        PERFORM policy_bump_revision(OLD.tenant_id, 'tenant_action', '*', OLD.action);
    END IF;

    PERFORM policy_bump_revision(NEW.tenant_id, 'tenant_action', '*', NEW.action);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_policy_revision_tenant_message_rule ON policy_tenant_message_action_rules;
CREATE TRIGGER trg_policy_revision_tenant_message_rule
AFTER INSERT OR UPDATE OR DELETE ON policy_tenant_message_action_rules
FOR EACH ROW EXECUTE FUNCTION policy_revision_bump_tenant_action();

DROP TRIGGER IF EXISTS trg_policy_revision_conversation_role_rule ON policy_conversation_role_action_rules;
CREATE TRIGGER trg_policy_revision_conversation_role_rule
AFTER INSERT OR UPDATE OR DELETE ON policy_conversation_role_action_rules
FOR EACH ROW EXECUTE FUNCTION policy_revision_bump_tenant_action();

DROP TRIGGER IF EXISTS trg_policy_revision_tenant_quota ON policy_tenant_message_action_quotas;
CREATE TRIGGER trg_policy_revision_tenant_quota
AFTER INSERT OR UPDATE OR DELETE ON policy_tenant_message_action_quotas
FOR EACH ROW EXECUTE FUNCTION policy_revision_bump_tenant_action();

DROP TRIGGER IF EXISTS trg_policy_revision_rebac_relation_rule ON policy_rebac_message_action_rules;
CREATE TRIGGER trg_policy_revision_rebac_relation_rule
AFTER INSERT OR UPDATE OR DELETE ON policy_rebac_message_action_rules
FOR EACH ROW EXECUTE FUNCTION policy_revision_bump_tenant_action();

DROP TRIGGER IF EXISTS trg_policy_revision_ownership_override_rule ON policy_message_ownership_override_rules;
CREATE TRIGGER trg_policy_revision_ownership_override_rule
AFTER INSERT OR UPDATE OR DELETE ON policy_message_ownership_override_rules
FOR EACH ROW EXECUTE FUNCTION policy_revision_bump_tenant_action();

CREATE OR REPLACE FUNCTION policy_revision_bump_user_restriction() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM policy_bump_revision(OLD.tenant_id, 'user_action', OLD.user_id, OLD.action);
        RETURN OLD;
    END IF;

    IF TG_OP = 'UPDATE' AND (
        OLD.tenant_id <> NEW.tenant_id OR
        OLD.user_id <> NEW.user_id OR
        OLD.action <> NEW.action
    ) THEN
        PERFORM policy_bump_revision(OLD.tenant_id, 'user_action', OLD.user_id, OLD.action);
    END IF;

    PERFORM policy_bump_revision(NEW.tenant_id, 'user_action', NEW.user_id, NEW.action);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_policy_revision_user_restriction ON policy_user_message_action_restrictions;
CREATE TRIGGER trg_policy_revision_user_restriction
AFTER INSERT OR UPDATE OR DELETE ON policy_user_message_action_restrictions
FOR EACH ROW EXECUTE FUNCTION policy_revision_bump_user_restriction();

CREATE OR REPLACE FUNCTION policy_revision_bump_conversation_member_projection() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM policy_bump_revision(OLD.tenant_id, 'conversation_member', OLD.conversation_id || ':' || OLD.user_id, '');
        RETURN OLD;
    END IF;

    IF TG_OP = 'UPDATE' AND (
        OLD.tenant_id <> NEW.tenant_id OR
        OLD.conversation_id <> NEW.conversation_id OR
        OLD.user_id <> NEW.user_id
    ) THEN
        PERFORM policy_bump_revision(OLD.tenant_id, 'conversation_member', OLD.conversation_id || ':' || OLD.user_id, '');
    END IF;

    PERFORM policy_bump_revision(NEW.tenant_id, 'conversation_member', NEW.conversation_id || ':' || NEW.user_id, '');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_policy_revision_conversation_member_projection ON policy_conversation_members_projection;
CREATE TRIGGER trg_policy_revision_conversation_member_projection
AFTER INSERT OR UPDATE OR DELETE ON policy_conversation_members_projection
FOR EACH ROW EXECUTE FUNCTION policy_revision_bump_conversation_member_projection();

CREATE OR REPLACE FUNCTION policy_revision_bump_contact_edge_projection() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM policy_bump_revision(OLD.tenant_id, 'contact_edge', OLD.owner_user_id || ':' || OLD.contact_user_id, 'SEND');
        RETURN OLD;
    END IF;

    IF TG_OP = 'UPDATE' AND (
        OLD.tenant_id <> NEW.tenant_id OR
        OLD.owner_user_id <> NEW.owner_user_id OR
        OLD.contact_user_id <> NEW.contact_user_id
    ) THEN
        PERFORM policy_bump_revision(OLD.tenant_id, 'contact_edge', OLD.owner_user_id || ':' || OLD.contact_user_id, 'SEND');
    END IF;

    PERFORM policy_bump_revision(NEW.tenant_id, 'contact_edge', NEW.owner_user_id || ':' || NEW.contact_user_id, 'SEND');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_policy_revision_contact_edge_projection ON policy_contact_edges_projection;
CREATE TRIGGER trg_policy_revision_contact_edge_projection
AFTER INSERT OR UPDATE OR DELETE ON policy_contact_edges_projection
FOR EACH ROW EXECUTE FUNCTION policy_revision_bump_contact_edge_projection();

INSERT INTO policy_revision_state (tenant_id, scope_type, scope_id, action, revision, updated_at)
SELECT DISTINCT tenant_id, 'exact_message_action', user_id || ':' || conversation_id, action, 1, now()
FROM policy_message_action_rules
ON CONFLICT DO NOTHING;

INSERT INTO policy_revision_state (tenant_id, scope_type, scope_id, action, revision, updated_at)
SELECT DISTINCT tenant_id, 'tenant_action', '*', action, 1, now()
FROM policy_tenant_message_action_rules
ON CONFLICT DO NOTHING;

INSERT INTO policy_revision_state (tenant_id, scope_type, scope_id, action, revision, updated_at)
SELECT DISTINCT tenant_id, 'tenant_action', '*', action, 1, now()
FROM policy_conversation_role_action_rules
ON CONFLICT DO NOTHING;

INSERT INTO policy_revision_state (tenant_id, scope_type, scope_id, action, revision, updated_at)
SELECT DISTINCT tenant_id, 'tenant_action', '*', action, 1, now()
FROM policy_tenant_message_action_quotas
ON CONFLICT DO NOTHING;

INSERT INTO policy_revision_state (tenant_id, scope_type, scope_id, action, revision, updated_at)
SELECT DISTINCT tenant_id, 'tenant_action', '*', action, 1, now()
FROM policy_rebac_message_action_rules
ON CONFLICT DO NOTHING;

INSERT INTO policy_revision_state (tenant_id, scope_type, scope_id, action, revision, updated_at)
SELECT DISTINCT tenant_id, 'tenant_action', '*', action, 1, now()
FROM policy_message_ownership_override_rules
ON CONFLICT DO NOTHING;

INSERT INTO policy_revision_state (tenant_id, scope_type, scope_id, action, revision, updated_at)
SELECT DISTINCT tenant_id, 'user_action', user_id, action, 1, now()
FROM policy_user_message_action_restrictions
ON CONFLICT DO NOTHING;

INSERT INTO policy_revision_state (tenant_id, scope_type, scope_id, action, revision, updated_at)
SELECT DISTINCT tenant_id, 'conversation_member', conversation_id || ':' || user_id, '', 1, now()
FROM policy_conversation_members_projection
ON CONFLICT DO NOTHING;

INSERT INTO policy_revision_state (tenant_id, scope_type, scope_id, action, revision, updated_at)
SELECT DISTINCT tenant_id, 'contact_edge', owner_user_id || ':' || contact_user_id, 'SEND', 1, now()
FROM policy_contact_edges_projection
ON CONFLICT DO NOTHING;
