package main

import (
	"testing"

	"github.com/qsyy0921/IM/services/policy-service/internal/app"
	kafkainfra "github.com/qsyy0921/IM/services/policy-service/internal/infrastructure/kafka"
	postgresinfra "github.com/qsyy0921/IM/services/policy-service/internal/infrastructure/postgres"
)

func TestPolicyDecisionAuditorFromEnvDefaultsToPostgres(t *testing.T) {
	t.Setenv("NEXUSIM_POLICY_DECISION_AUDIT_SINK", "")

	auditor, closeAuditor, err := policyDecisionAuditorFromEnv(nil, nil)
	defer closeAuditor()

	if err == nil || auditor != nil {
		t.Fatalf("expected missing postgres pool error for default sink")
	}
}

func TestPolicyDecisionAuditorFromEnvRejectsUnsupportedSink(t *testing.T) {
	t.Setenv("NEXUSIM_POLICY_DECISION_AUDIT_SINK", "redis")

	auditor, closeAuditor, err := policyDecisionAuditorFromEnv(nil, nil)
	defer closeAuditor()

	if err == nil || auditor != nil {
		t.Fatalf("expected unsupported sink error")
	}
}

func TestPolicyDecisionAuditorFromEnvBuildsKafkaSink(t *testing.T) {
	t.Setenv("NEXUSIM_POLICY_DECISION_AUDIT_SINK", "kafka")
	t.Setenv("NEXUSIM_KAFKA_BROKERS", "127.0.0.1:9092")
	t.Setenv("NEXUSIM_POLICY_AUDIT_EVENTS_TOPIC", "im.policy.events.test")

	auditor, closeAuditor, err := policyDecisionAuditorFromEnv(nil, nil)
	defer closeAuditor()

	if err != nil {
		t.Fatalf("build kafka auditor: %v", err)
	}
	if _, ok := auditor.(*kafkainfra.DecisionAuditKafka); !ok {
		t.Fatalf("expected kafka auditor, got %T", auditor)
	}
}

func TestPolicyDecisionAuditorFromEnvBuildsAsyncKafkaSink(t *testing.T) {
	t.Setenv("NEXUSIM_POLICY_DECISION_AUDIT_SINK", "kafka_async")
	t.Setenv("NEXUSIM_KAFKA_BROKERS", "127.0.0.1:9092")
	t.Setenv("NEXUSIM_POLICY_AUDIT_EVENTS_TOPIC", "im.policy.events.test")
	t.Setenv("NEXUSIM_POLICY_AUDIT_EVENTS_DLQ_TOPIC", "im.policy.events.test.dlq")
	t.Setenv("NEXUSIM_POLICY_DECISION_AUDIT_ASYNC_QUEUE_SIZE", "16")
	t.Setenv("NEXUSIM_POLICY_DECISION_AUDIT_ASYNC_BATCH_SIZE", "4")
	t.Setenv("NEXUSIM_POLICY_DECISION_AUDIT_ASYNC_FLUSH_INTERVAL", "1ms")

	auditor, closeAuditor, err := policyDecisionAuditorFromEnv(nil, nil)
	defer closeAuditor()

	if err != nil {
		t.Fatalf("build async kafka auditor: %v", err)
	}
	if _, ok := auditor.(*kafkainfra.DecisionAuditKafkaAsync); !ok {
		t.Fatalf("expected async kafka auditor, got %T", auditor)
	}
}

func TestPolicyDecisionAuditorTypesImplementInterface(t *testing.T) {
	var _ app.PolicyDecisionAuditor = (*kafkainfra.DecisionAuditKafka)(nil)
	var _ app.PolicyDecisionAuditor = (*kafkainfra.DecisionAuditKafkaAsync)(nil)
	var _ app.PolicyDecisionAuditor = (*postgresinfra.DecisionAuditOutbox)(nil)
}
