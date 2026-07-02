-- The policy outbox relay runs unordered in the local loadtest runtime, but the
-- ordered path still only needs rows that can block publishing.
DROP INDEX IF EXISTS idx_policy_decision_audit_outbox_partition_order;

CREATE INDEX IF NOT EXISTS idx_policy_decision_audit_outbox_partition_order_ready
    ON policy_decision_audit_outbox (tenant_id, partition_key, aggregate_version)
    WHERE status IN ('PENDING', 'DLQ');
