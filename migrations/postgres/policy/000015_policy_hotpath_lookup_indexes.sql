CREATE INDEX IF NOT EXISTS idx_policy_decision_audit_outbox_quota_allowed_recent
    ON policy_decision_audit_outbox (tenant_id, action, created_at DESC)
    WHERE allowed = true;
