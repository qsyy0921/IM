package main

import (
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/policy-service/internal/app"
	kafkainfra "github.com/qsyy0921/IM/services/policy-service/internal/infrastructure/kafka"
	postgresinfra "github.com/qsyy0921/IM/services/policy-service/internal/infrastructure/postgres"
	outboxtrigger "github.com/qsyy0921/IM/services/policy-service/internal/trigger/outbox"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type policyDecisionAuditStageObserver interface {
	RecordPolicyDecisionStage(action types.MessageAction, stage string, failed bool, latencyMS int64)
}

func policyDecisionAuditorFromEnv(
	auditPool *pgxpool.Pool,
	observer policyDecisionAuditStageObserver,
) (app.PolicyDecisionAuditor, func(), error) {
	sink := strings.ToLower(strings.TrimSpace(os.Getenv("NEXUSIM_POLICY_DECISION_AUDIT_SINK")))
	if sink == "" {
		sink = "postgres"
	}
	switch sink {
	case "postgres", "pg", "outbox":
		if auditPool == nil {
			return nil, func() {}, errors.New("policy decision audit postgres pool is required")
		}
		log.Println("policy-service decision audit outbox enabled with isolated postgres pool")
		return postgresinfra.NewDecisionAuditOutbox(
			auditPool,
			postgresinfra.WithDecisionAuditStageObserver(observer),
		), func() {}, nil
	case "kafka":
		producer, err := kafkainfra.NewWriterProducerWithConfig(splitCSV(os.Getenv("NEXUSIM_KAFKA_BROKERS")), kafkainfra.WriterProducerConfig{
			BatchSize:    envInt("NEXUSIM_POLICY_DECISION_AUDIT_KAFKA_BATCH_SIZE", 1),
			BatchTimeout: envDuration("NEXUSIM_POLICY_DECISION_AUDIT_KAFKA_BATCH_TIMEOUT", time.Millisecond),
		})
		if err != nil {
			return nil, func() {}, err
		}
		topic := envString("NEXUSIM_POLICY_AUDIT_EVENTS_TOPIC", outboxtrigger.TopicPolicyEvents)
		log.Printf("policy-service decision audit kafka sink enabled topic=%s", topic)
		return kafkainfra.NewDecisionAuditKafka(
				producer,
				kafkainfra.WithDecisionAuditKafkaTopic(topic),
				kafkainfra.WithDecisionAuditKafkaStageObserver(observer),
			), func() {
				if err := producer.Close(); err != nil {
					log.Printf("policy-service decision audit kafka producer close failed: %v", err)
				}
			}, nil
	case "kafka_async", "async_kafka", "kafka-async":
		asyncBatchSize := envInt("NEXUSIM_POLICY_DECISION_AUDIT_ASYNC_BATCH_SIZE", 100)
		asyncFlushInterval := envDuration("NEXUSIM_POLICY_DECISION_AUDIT_ASYNC_FLUSH_INTERVAL", 10*time.Millisecond)
		producer, err := kafkainfra.NewWriterProducerWithConfig(splitCSV(os.Getenv("NEXUSIM_KAFKA_BROKERS")), kafkainfra.WriterProducerConfig{
			BatchSize:    envInt("NEXUSIM_POLICY_DECISION_AUDIT_KAFKA_WRITER_BATCH_SIZE", asyncBatchSize),
			BatchTimeout: envDuration("NEXUSIM_POLICY_DECISION_AUDIT_KAFKA_WRITER_BATCH_TIMEOUT", asyncFlushInterval),
		})
		if err != nil {
			return nil, func() {}, err
		}
		topic := envString("NEXUSIM_POLICY_AUDIT_EVENTS_TOPIC", outboxtrigger.TopicPolicyEvents)
		dlqTopic := envString("NEXUSIM_POLICY_AUDIT_EVENTS_DLQ_TOPIC", topic+".dlq")
		auditor := kafkainfra.NewDecisionAuditKafkaAsync(
			producer,
			kafkainfra.WithDecisionAuditKafkaAsyncTopic(topic),
			kafkainfra.WithDecisionAuditKafkaAsyncDLQTopic(dlqTopic),
			kafkainfra.WithDecisionAuditKafkaAsyncQueueSize(envInt("NEXUSIM_POLICY_DECISION_AUDIT_ASYNC_QUEUE_SIZE", 8192)),
			kafkainfra.WithDecisionAuditKafkaAsyncWorkers(envInt("NEXUSIM_POLICY_DECISION_AUDIT_ASYNC_WORKERS", 1)),
			kafkainfra.WithDecisionAuditKafkaAsyncBatchSize(asyncBatchSize),
			kafkainfra.WithDecisionAuditKafkaAsyncFlushInterval(asyncFlushInterval),
			kafkainfra.WithDecisionAuditKafkaAsyncRetry(
				envInt("NEXUSIM_POLICY_DECISION_AUDIT_ASYNC_MAX_ATTEMPTS", 5),
				envDuration("NEXUSIM_POLICY_DECISION_AUDIT_ASYNC_RETRY_BASE_DELAY", 50*time.Millisecond),
				envDuration("NEXUSIM_POLICY_DECISION_AUDIT_ASYNC_RETRY_MAX_DELAY", time.Second),
			),
			kafkainfra.WithDecisionAuditKafkaAsyncCloseTimeout(envDuration("NEXUSIM_POLICY_DECISION_AUDIT_ASYNC_CLOSE_TIMEOUT", 5*time.Second)),
			kafkainfra.WithDecisionAuditKafkaAsyncStageObserver(observer),
			kafkainfra.WithDecisionAuditKafkaAsyncLogf(log.Printf),
		)
		log.Printf("policy-service decision audit async kafka sink enabled topic=%s dlq_topic=%s batch_size=%d", topic, dlqTopic, asyncBatchSize)
		return auditor, func() {
			if err := auditor.Close(); err != nil {
				log.Printf("policy-service decision audit async kafka close failed: %v", err)
			}
			if err := producer.Close(); err != nil {
				log.Printf("policy-service decision audit kafka producer close failed: %v", err)
			}
		}, nil
	default:
		return nil, func() {}, errors.New("unsupported NEXUSIM_POLICY_DECISION_AUDIT_SINK")
	}
}
