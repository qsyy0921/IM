package main

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	policygrpc "github.com/qsyy0921/IM/services/policy-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/policy-service/internal/app"
	"github.com/qsyy0921/IM/services/policy-service/internal/domain"
	kafkainfra "github.com/qsyy0921/IM/services/policy-service/internal/infrastructure/kafka"
	monitoringinfra "github.com/qsyy0921/IM/services/policy-service/internal/infrastructure/monitoring"
	postgresinfra "github.com/qsyy0921/IM/services/policy-service/internal/infrastructure/postgres"
	contacttrigger "github.com/qsyy0921/IM/services/policy-service/internal/trigger/contact"
	outboxtrigger "github.com/qsyy0921/IM/services/policy-service/internal/trigger/outbox"
	timelinetrigger "github.com/qsyy0921/IM/services/policy-service/internal/trigger/timeline"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run() error {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_POLICY_SERVICE_MODE"))
	switch mode {
	case "", "noop":
		log.Println("policy-service runtime wiring is idle; set NEXUSIM_POLICY_SERVICE_MODE=grpc, contact-consumer, timeline-consumer, outbox-relay, outbox-audit, outbox-repair, outbox-repair-audit, outbox-repair-cleanup, decision-audit-export, tenant-quota-audit, or tenant-quota-set")
		return nil
	case "grpc":
		return runGRPC()
	case "contact-consumer":
		return runContactConsumer()
	case "timeline-consumer":
		return runTimelineConsumer()
	case "outbox-relay":
		return runOutboxRelay()
	case "outbox-audit":
		return runOutboxAudit()
	case "outbox-repair":
		return runOutboxRepair()
	case "outbox-repair-audit":
		return runOutboxRepairAudit()
	case "outbox-repair-cleanup":
		return runOutboxRepairCleanup()
	case "decision-audit-export":
		return runDecisionAuditExport()
	case "tenant-quota-audit":
		return runTenantQuotaAudit()
	case "tenant-quota-set":
		return runTenantQuotaSet()
	default:
		return errors.New("unsupported NEXUSIM_POLICY_SERVICE_MODE")
	}
}

func runOutboxRelay() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := envString("NEXUSIM_PG_DSN", "")
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required for policy outbox relay")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	producer, err := kafkainfra.NewWriterProducer(splitCSV(os.Getenv("NEXUSIM_KAFKA_BROKERS")))
	if err != nil {
		return err
	}
	defer producer.Close()

	relay := outboxtrigger.NewRelay(
		postgresinfra.NewOutboxStore(pool),
		producer,
		outboxtrigger.Config{
			Topic:          envString("NEXUSIM_POLICY_AUDIT_EVENTS_TOPIC", outboxtrigger.TopicPolicyEvents),
			BatchSize:      envInt("NEXUSIM_POLICY_OUTBOX_BATCH_SIZE", 500),
			PollInterval:   envDuration("NEXUSIM_POLICY_OUTBOX_POLL_INTERVAL", time.Second),
			MaxAttempts:    envInt("NEXUSIM_POLICY_OUTBOX_MAX_ATTEMPTS", 5),
			RetryBaseDelay: envDuration("NEXUSIM_POLICY_OUTBOX_RETRY_BASE_DELAY", time.Second),
			ErrorBackoff:   envDuration("NEXUSIM_POLICY_OUTBOX_RELAY_ERROR_BACKOFF", time.Second),
			Logf:           log.Printf,
		},
	)
	debugAddr, err := policyDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr, monitoringinfra.NewHandler(pool, true, nil, nil).WithOutboxRelayStats(relay.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()
	log.Println("policy-service decision audit outbox relay started")
	return relay.Run(ctx)
}

func runOutboxAudit() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := envString("NEXUSIM_PG_DSN", "")
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required for policy outbox audit")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	var outboxID *int64
	outboxIDFilter := strings.TrimSpace(os.Getenv("NEXUSIM_POLICY_OUTBOX_AUDIT_OUTBOX_ID"))
	if outboxIDFilter != "" {
		parsed := envInt64AllowZero("NEXUSIM_POLICY_OUTBOX_AUDIT_OUTBOX_ID", 0)
		outboxID = &parsed
	}
	eventID := envString("NEXUSIM_POLICY_OUTBOX_AUDIT_EVENT_ID", "")
	tenantID := envString("NEXUSIM_POLICY_OUTBOX_AUDIT_TENANT_ID", "")
	aggregateID := envString("NEXUSIM_POLICY_OUTBOX_AUDIT_AGGREGATE_ID", "")
	status := envString("NEXUSIM_POLICY_OUTBOX_AUDIT_STATUS", "")
	eventType := envString("NEXUSIM_POLICY_OUTBOX_AUDIT_EVENT_TYPE", "")
	createdAfter, err := envOptionalRFC3339Time("NEXUSIM_POLICY_OUTBOX_AUDIT_CREATED_AFTER")
	if err != nil {
		return err
	}
	createdBefore, err := envOptionalRFC3339Time("NEXUSIM_POLICY_OUTBOX_AUDIT_CREATED_BEFORE")
	if err != nil {
		return err
	}
	rows, err := postgresinfra.NewOutboxStore(pool).AuditOutbox(ctx, postgresinfra.OutboxAuditOptions{
		OutboxID:      outboxID,
		EventID:       eventID,
		TenantID:      tenantID,
		AggregateID:   aggregateID,
		Status:        status,
		EventType:     eventType,
		CreatedAfter:  createdAfter,
		CreatedBefore: createdBefore,
		Limit:         envInt("NEXUSIM_POLICY_OUTBOX_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("policy-service outbox audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"policy_outbox id=%d event_id=%s tenant_id=%s aggregate_type=%s aggregate_id=%s aggregate_version=%d event_type=%s status=%s retry_count=%d available_at=%s next_retry_at=%s published_at=%s dead_lettered_at=%s last_error=%q",
			row.ID,
			row.EventID,
			row.TenantID,
			row.AggregateType,
			row.AggregateID,
			row.AggregateVersion,
			row.EventType,
			row.Status,
			row.RetryCount,
			row.AvailableAt.Format(time.RFC3339),
			formatOptionalTime(row.NextRetryAt),
			formatOptionalTime(row.PublishedAt),
			formatOptionalTime(row.DeadLetteredAt),
			row.LastError,
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_POLICY_OUTBOX_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeOutboxAuditOutput(outputPath, rows, map[string]string{
			"outbox_id":      outboxIDFilter,
			"event_id":       eventID,
			"tenant_id":      tenantID,
			"aggregate_id":   aggregateID,
			"status":         status,
			"event_type":     eventType,
			"created_after":  formatOptionalTime(createdAfter),
			"created_before": formatOptionalTime(createdBefore),
		}); err != nil {
			return err
		}
	}
	return nil
}

func runOutboxRepair() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := envString("NEXUSIM_PG_DSN", "")
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required for policy outbox repair")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	eventIDs := splitCSV(os.Getenv("NEXUSIM_POLICY_OUTBOX_REPAIR_EVENT_IDS"))
	operator := envString("NEXUSIM_POLICY_OUTBOX_REPAIR_OPERATOR", "local-operator")
	reason, err := policyOperatorReasonFromEnv(
		"NEXUSIM_POLICY_OUTBOX_REPAIR_REASON",
		"NEXUSIM_POLICY_OUTBOX_REPAIR_REASON_FILE",
		"manual policy audit outbox repair",
	)
	if err != nil {
		return err
	}
	stats, err := postgresinfra.NewOutboxStore(pool).RepairDLQEvents(ctx, eventIDs, operator, reason, validatePolicyAuditOutboxMessage)
	if err != nil {
		return err
	}
	log.Printf(
		"policy-service outbox repair completed requested=%d repaired=%d skipped=%d invalid=%d",
		stats.Requested,
		stats.Repaired,
		stats.Skipped,
		stats.Invalid,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_POLICY_OUTBOX_REPAIR_OUTPUT")); outputPath != "" {
		if err := writeOutboxRepairOutput(outputPath, stats, len(eventIDs)); err != nil {
			return err
		}
	}
	if stats.Invalid > 0 {
		return errors.New("policy audit outbox repair skipped invalid events")
	}
	return nil
}

func runOutboxRepairAudit() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := envString("NEXUSIM_PG_DSN", "")
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required for policy outbox repair audit")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	repairedAfter, err := envOptionalRFC3339Time("NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_REPAIRED_AFTER")
	if err != nil {
		return err
	}
	repairedBefore, err := envOptionalRFC3339Time("NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_REPAIRED_BEFORE")
	if err != nil {
		return err
	}
	filters := map[string]string{
		"event_id":        envString("NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_EVENT_ID", ""),
		"tenant_id":       envString("NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_TENANT_ID", ""),
		"operator":        envString("NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_OPERATOR", ""),
		"outcome":         envString("NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_OUTCOME", ""),
		"repaired_after":  formatOptionalFilterTime(repairedAfter),
		"repaired_before": formatOptionalFilterTime(repairedBefore),
	}
	rows, err := postgresinfra.NewOutboxStore(pool).AuditOutboxRepairs(ctx, postgresinfra.OutboxRepairAuditOptions{
		EventID:        filters["event_id"],
		TenantID:       filters["tenant_id"],
		Operator:       filters["operator"],
		Outcome:        filters["outcome"],
		RepairedAfter:  repairedAfter,
		RepairedBefore: repairedBefore,
		Limit:          envInt("NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("policy-service outbox repair audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"policy_outbox_repair event_id=%s tenant_id=%s operator=%s outcome=%s skip_reason=%s previous_status=%s previous_retry_count=%d previous_dead_lettered_at=%s repaired_at=%s reason=%q previous_last_error=%q",
			row.EventID,
			row.TenantID,
			row.Operator,
			row.Outcome,
			row.SkipReason,
			row.PreviousStatus,
			row.PreviousRetryCount,
			formatOptionalTime(row.PreviousDeadLetteredAt),
			row.RepairedAt.Format(time.RFC3339),
			row.Reason,
			row.PreviousLastError,
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeOutboxRepairAuditOutput(outputPath, rows, filters); err != nil {
			return err
		}
	}
	return nil
}

func runOutboxRepairCleanup() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := envString("NEXUSIM_PG_DSN", "")
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required for policy outbox repair cleanup")
	}
	config, err := outboxRepairCleanupConfigFromEnv()
	if err != nil {
		return err
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	cutoff := time.Now().UTC().Add(-config.Retention)
	stats, err := postgresinfra.NewOutboxStore(pool).CleanupOutboxRepairs(ctx, postgresinfra.OutboxRepairCleanupOptions{
		EventID:  envString("NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_EVENT_ID", ""),
		TenantID: envString("NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_TENANT_ID", ""),
		Operator: envString("NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_OPERATOR", ""),
		Outcome:  envString("NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_OUTCOME", ""),
		Cutoff:   cutoff,
		Limit:    config.BatchSize,
		DryRun:   config.DryRun,
	})
	if err != nil {
		return err
	}
	log.Printf(
		"policy-service outbox repair cleanup completed deleted=%d retention=%s batch_size=%d dry_run=%t",
		stats.Deleted,
		config.Retention,
		config.BatchSize,
		config.DryRun,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_OUTPUT")); outputPath != "" {
		if err := writeOutboxRepairCleanupOutput(outputPath, stats, cutoff, config.Retention, config.BatchSize, config.DryRun, map[string]string{
			"event_id":  envString("NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_EVENT_ID", ""),
			"tenant_id": envString("NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_TENANT_ID", ""),
			"operator":  envString("NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_OPERATOR", ""),
			"outcome":   envString("NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_OUTCOME", ""),
		}); err != nil {
			return err
		}
	}
	return nil
}

func runDecisionAuditExport() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := envString("NEXUSIM_PG_DSN", "")
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required for policy decision audit export")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	allowed, allowedConfigured, err := envOptionalBool("NEXUSIM_POLICY_DECISION_AUDIT_EXPORT_ALLOWED")
	if err != nil {
		return err
	}
	var allowedFilter *bool
	if allowedConfigured {
		allowedFilter = &allowed
	}
	createdAfter, err := envOptionalRFC3339Time("NEXUSIM_POLICY_DECISION_AUDIT_EXPORT_CREATED_AFTER")
	if err != nil {
		return err
	}
	createdBefore, err := envOptionalRFC3339Time("NEXUSIM_POLICY_DECISION_AUDIT_EXPORT_CREATED_BEFORE")
	if err != nil {
		return err
	}
	filters := map[string]string{
		"event_id":        envString("NEXUSIM_POLICY_DECISION_AUDIT_EXPORT_EVENT_ID", ""),
		"tenant_id":       envString("NEXUSIM_POLICY_DECISION_AUDIT_EXPORT_TENANT_ID", ""),
		"action":          envString("NEXUSIM_POLICY_DECISION_AUDIT_EXPORT_ACTION", ""),
		"allowed":         envString("NEXUSIM_POLICY_DECISION_AUDIT_EXPORT_ALLOWED", ""),
		"classification":  envString("NEXUSIM_POLICY_DECISION_AUDIT_EXPORT_CLASSIFICATION", ""),
		"reason_code":     envString("NEXUSIM_POLICY_DECISION_AUDIT_EXPORT_REASON_CODE", ""),
		"decision_source": envString("NEXUSIM_POLICY_DECISION_AUDIT_EXPORT_DECISION_SOURCE", ""),
		"status":          envString("NEXUSIM_POLICY_DECISION_AUDIT_EXPORT_STATUS", ""),
		"created_after":   formatOptionalFilterTime(createdAfter),
		"created_before":  formatOptionalFilterTime(createdBefore),
	}
	rows, err := postgresinfra.NewOutboxStore(pool).ExportDecisionAudit(ctx, postgresinfra.DecisionAuditExportOptions{
		EventID:        filters["event_id"],
		TenantID:       filters["tenant_id"],
		Action:         filters["action"],
		Allowed:        allowedFilter,
		Classification: filters["classification"],
		ReasonCode:     filters["reason_code"],
		DecisionSource: filters["decision_source"],
		Status:         filters["status"],
		CreatedAfter:   createdAfter,
		CreatedBefore:  createdBefore,
		Limit:          envInt("NEXUSIM_POLICY_DECISION_AUDIT_EXPORT_LIMIT", 100),
	})
	if err != nil {
		return err
	}
	log.Printf("policy-service decision audit export completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"policy_decision_audit event_id=%s tenant_id=%s action=%s allowed=%t classification=%s reason_code=%s decision_source=%s status=%s permission_version=%d created_at=%s",
			row.EventID,
			row.TenantID,
			row.Action,
			row.Allowed,
			row.Classification,
			row.ReasonCode,
			row.DecisionSource,
			row.Status,
			row.PermissionVersion,
			row.CreatedAt.Format(time.RFC3339),
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_POLICY_DECISION_AUDIT_EXPORT_OUTPUT")); outputPath != "" {
		if err := writeDecisionAuditExportOutput(outputPath, rows, filters); err != nil {
			return err
		}
	}
	return nil
}

func runTenantQuotaAudit() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := envString("NEXUSIM_PG_DSN", "")
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required for policy tenant quota audit")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	enabled, enabledConfigured, err := envOptionalBool("NEXUSIM_POLICY_TENANT_QUOTA_AUDIT_ENABLED")
	if err != nil {
		return err
	}
	var enabledFilter *bool
	if enabledConfigured {
		enabledFilter = &enabled
	}
	rows, err := postgresinfra.NewTenantQuotaStore(pool).AuditTenantQuotas(ctx, postgresinfra.TenantQuotaAuditOptions{
		TenantID: envString("NEXUSIM_POLICY_TENANT_QUOTA_AUDIT_TENANT_ID", ""),
		Action:   envString("NEXUSIM_POLICY_TENANT_QUOTA_AUDIT_ACTION", ""),
		Enabled:  enabledFilter,
		Limit:    envInt("NEXUSIM_POLICY_TENANT_QUOTA_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("policy-service tenant quota audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"policy_tenant_quota tenant_id=%s action=%s max_decisions=%d window_seconds=%d permission_version=%d classification=%s enabled=%t source=%s updated_at=%s",
			row.TenantID,
			row.Action,
			row.MaxDecisions,
			row.WindowSeconds,
			row.PermissionVersion,
			row.Classification,
			row.Enabled,
			row.Source,
			row.UpdatedAt.Format(time.RFC3339),
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_POLICY_TENANT_QUOTA_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeTenantQuotaAuditOutput(outputPath, rows); err != nil {
			return err
		}
	}
	return nil
}

func runTenantQuotaSet() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := envString("NEXUSIM_PG_DSN", "")
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required for policy tenant quota set")
	}
	maxDecisions, err := envPositiveInt("NEXUSIM_POLICY_TENANT_QUOTA_SET_MAX_DECISIONS", 0)
	if err != nil {
		return err
	}
	window, err := envPositiveDuration("NEXUSIM_POLICY_TENANT_QUOTA_SET_WINDOW", 0)
	if err != nil {
		return err
	}
	permissionVersion, err := envPositiveInt64("NEXUSIM_POLICY_TENANT_QUOTA_SET_PERMISSION_VERSION", 0)
	if err != nil {
		return err
	}
	enabled, enabledConfigured, err := envOptionalBool("NEXUSIM_POLICY_TENANT_QUOTA_SET_ENABLED")
	if err != nil {
		return err
	}
	if !enabledConfigured {
		enabled = true
	}
	reason, err := policyOperatorReasonFromEnv(
		"NEXUSIM_POLICY_TENANT_QUOTA_SET_REASON",
		"NEXUSIM_POLICY_TENANT_QUOTA_SET_REASON_FILE",
		"",
	)
	if err != nil {
		return err
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	row, err := postgresinfra.NewTenantQuotaStore(pool).SetTenantQuota(ctx, postgresinfra.TenantQuotaSetOptions{
		TenantID:          envString("NEXUSIM_POLICY_TENANT_QUOTA_SET_TENANT_ID", ""),
		Action:            envString("NEXUSIM_POLICY_TENANT_QUOTA_SET_ACTION", ""),
		MaxDecisions:      maxDecisions,
		WindowSeconds:     int(window.Seconds()),
		PermissionVersion: permissionVersion,
		Classification:    envString("NEXUSIM_POLICY_TENANT_QUOTA_SET_CLASSIFICATION", ""),
		Reason:            reason,
		Enabled:           enabled,
		Source:            envString("NEXUSIM_POLICY_TENANT_QUOTA_SET_SOURCE", "manual"),
	})
	if err != nil {
		return err
	}
	log.Printf(
		"policy-service tenant quota set tenant_id=%s action=%s max_decisions=%d window_seconds=%d permission_version=%d classification=%s enabled=%t source=%s",
		row.TenantID,
		row.Action,
		row.MaxDecisions,
		row.WindowSeconds,
		row.PermissionVersion,
		row.Classification,
		row.Enabled,
		row.Source,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_POLICY_TENANT_QUOTA_SET_OUTPUT")); outputPath != "" {
		if err := writeTenantQuotaSetOutput(outputPath, row); err != nil {
			return err
		}
	}
	return nil
}

func validatePolicyAuditOutboxMessage(message types.OutboxMessage) error {
	_, err := outboxtrigger.BuildPolicyEvent(message)
	return err
}

func runContactConsumer() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := envString("NEXUSIM_PG_DSN", "")
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required for policy contact consumer")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	consumer, err := kafkainfra.NewReaderConsumer(kafkainfra.ReaderConfig{
		Brokers: splitCSV(os.Getenv("NEXUSIM_KAFKA_BROKERS")),
		Topic:   envString("NEXUSIM_CONTACT_EVENTS_TOPIC", contacttrigger.TopicContactEvents),
		GroupID: envString("NEXUSIM_POLICY_CONTACT_CONSUMER_GROUP", "nexusim-policy-contacts"),
	})
	if err != nil {
		return err
	}
	defer consumer.Close()

	worker := contacttrigger.NewWorker(
		consumer,
		app.NewProjectContactEventUseCase(postgresinfra.NewProjectionRepository(pool)),
		envString("NEXUSIM_POLICY_CONTACT_CONSUMER_GROUP", "nexusim-policy-contacts"),
		contacttrigger.Config{
			ErrorBackoff: envDuration("NEXUSIM_POLICY_CONTACT_CONSUMER_ERROR_BACKOFF", time.Second),
			Logf:         log.Printf,
		},
	)
	debugAddr, err := policyDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr, monitoringinfra.NewHandler(pool, true, nil, nil).WithContactProjectionWorkerStats(worker.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()
	log.Println("policy-service contact projection consumer started")
	return worker.Run(ctx)
}

func runTimelineConsumer() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := envString("NEXUSIM_PG_DSN", "")
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required for policy timeline consumer")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	groupID := envString("NEXUSIM_POLICY_TIMELINE_CONSUMER_GROUP", "nexusim-policy-timeline")
	consumer, err := kafkainfra.NewReaderConsumer(kafkainfra.ReaderConfig{
		Brokers: splitCSV(os.Getenv("NEXUSIM_KAFKA_BROKERS")),
		Topic:   envString("NEXUSIM_CONVERSATION_TIMELINE_TOPIC", timelinetrigger.TopicConversationTimelineEvents),
		GroupID: groupID,
	})
	if err != nil {
		return err
	}
	defer consumer.Close()

	worker := timelinetrigger.NewWorker(
		consumer,
		app.NewProjectConversationMemberEventUseCase(postgresinfra.NewProjectionRepository(pool)),
		groupID,
		timelinetrigger.Config{
			ErrorBackoff: envDuration("NEXUSIM_POLICY_TIMELINE_CONSUMER_ERROR_BACKOFF", time.Second),
			Logf:         log.Printf,
		},
	)
	debugAddr, err := policyDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr, monitoringinfra.NewHandler(pool, true, nil, nil).WithTimelineProjectionWorkerStats(worker.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()
	log.Println("policy-service conversation timeline projection consumer started")
	return worker.Run(ctx)
}

func runGRPC() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := envString("NEXUSIM_POLICY_GRPC_ADDR", "0.0.0.0:10800")
	serverTLSConfig, serverTLSEnabled, err := policyGRPCTLSConfigFromEnv()
	if err != nil {
		return err
	}
	if err := validatePolicyListenerConfig(addr, serverTLSEnabled); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	policy := staticMessagePolicyFromEnv()
	var evaluator app.MessagePolicyEvaluator = policy
	var useCaseOptions []app.CheckMessageActionOption
	var pool *pgxpool.Pool
	rulesEnabled := envBool("NEXUSIM_POLICY_RULES_ENABLED", false)
	if rulesEnabled {
		dsn := envString("NEXUSIM_PG_DSN", "")
		if dsn == "" {
			return errors.New("NEXUSIM_PG_DSN is required when NEXUSIM_POLICY_RULES_ENABLED=true")
		}
		var err error
		pool, err = openPGPool(ctx, dsn)
		if err != nil {
			return err
		}
		defer pool.Close()
		postgresEvaluator := postgresinfra.NewMessagePolicyEvaluator(pool, policy)
		evaluator = postgresEvaluator
		useCaseOptions = append(useCaseOptions, app.WithPolicyDecisionAuditor(postgresinfra.NewDecisionAuditOutbox(pool)))
		useCaseOptions = append(useCaseOptions, app.WithMessageOwnershipOverrideChecker(postgresEvaluator))
		log.Println("policy-service message action rule store enabled")
		log.Println("policy-service decision audit outbox enabled")
	}
	moderator, moderationEnabled, err := policyContentModeratorFromEnv()
	if err != nil {
		return err
	}
	if moderationEnabled {
		useCaseOptions = append(useCaseOptions, app.WithMessageContentModerator(moderator))
		log.Println("policy-service message content moderation enabled")
	}
	grpcMetrics := monitoringinfra.NewGRPCMetrics()
	traceConfig, err := policyTraceConfigFromEnv()
	if err != nil {
		return err
	}
	traceRuntime, err := monitoringinfra.NewTraceRuntime(ctx, traceConfig)
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := traceRuntime.Shutdown(shutdownCtx); err != nil {
			log.Printf("policy-service OpenTelemetry trace shutdown failed: %v", err)
		}
	}()
	decisionMetrics := monitoringinfra.NewDecisionMetrics()
	useCaseOptions = append(useCaseOptions, app.WithPolicyDecisionObserver(decisionMetrics))
	debugAddr, err := policyDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr, monitoringinfra.NewHandler(pool, rulesEnabled, grpcMetrics, decisionMetrics).WithTraceStats(traceRuntime.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()

	serverOptions := []grpc.ServerOption{grpc.ChainUnaryInterceptor(
		grpcMetrics.UnaryServerInterceptor(log.Default()),
		traceRuntime.UnaryServerInterceptor(),
	)}
	if serverTLSEnabled {
		serverOptions = append(serverOptions, grpc.Creds(credentials.NewTLS(serverTLSConfig)))
	}
	server := grpc.NewServer(serverOptions...)
	policygrpc.Register(server, policygrpc.NewServer(app.NewCheckMessageActionUseCase(evaluator, useCaseOptions...)))
	go func() {
		<-ctx.Done()
		server.GracefulStop()
	}()
	log.Printf("policy-service grpc listening on %s", addr)
	if err := server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return err
	}
	return ctx.Err()
}

func staticMessagePolicyFromEnv() domain.StaticMessagePolicy {
	permissionVersion := int64(0)
	if strings.TrimSpace(os.Getenv("NEXUSIM_POLICY_PERMISSION_VERSION")) != "" {
		permissionVersion = envInt64("NEXUSIM_POLICY_PERMISSION_VERSION", 0)
	}
	return domain.StaticMessagePolicy{
		Allowed:           envBool("NEXUSIM_POLICY_MESSAGE_ALLOWED", true),
		PermissionVersion: permissionVersion,
		Classification:    envString("NEXUSIM_POLICY_CLASSIFICATION", "INTERNAL"),
		Reason:            envString("NEXUSIM_POLICY_DENY_REASON", ""),
	}
}
