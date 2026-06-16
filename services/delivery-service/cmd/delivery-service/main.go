package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	grpcapi "github.com/qsyy0921/IM/services/delivery-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/delivery-service/internal/app"
	kafkainfra "github.com/qsyy0921/IM/services/delivery-service/internal/infrastructure/kafka"
	monitoringinfra "github.com/qsyy0921/IM/services/delivery-service/internal/infrastructure/monitoring"
	postgresinfra "github.com/qsyy0921/IM/services/delivery-service/internal/infrastructure/postgres"
	"github.com/qsyy0921/IM/services/delivery-service/internal/trigger/outbox"
	"github.com/qsyy0921/IM/services/delivery-service/internal/trigger/timeline"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

const deliveryServiceModeHelp = "grpc, timeline-consumer, outbox-relay, outbox-repair, outbox-audit, outbox-repair-audit, outbox-repair-cleanup, projection-checkpoint-repair, projection-checkpoint-repair-audit, projection-checkpoint-repair-cleanup, projection-failure-audit, projection-failure-resolve, or projection-failure-cleanup"

func run() error {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_SERVICE_MODE"))
	switch mode {
	case "", "noop":
		log.Printf("delivery-service runtime wiring is idle; set NEXUSIM_DELIVERY_SERVICE_MODE=%s", deliveryServiceModeHelp)
		return nil
	case "grpc":
		return runGRPCServer()
	case "timeline-consumer":
		return runTimelineConsumer()
	case "outbox-relay":
		return runOutboxRelay()
	case "outbox-repair":
		return runOutboxRepair()
	case "outbox-audit":
		return runOutboxAudit()
	case "outbox-repair-audit":
		return runOutboxRepairAudit()
	case "outbox-repair-cleanup":
		return runOutboxRepairCleanup()
	case "projection-checkpoint-repair":
		return runProjectionCheckpointRepair()
	case "projection-checkpoint-repair-audit":
		return runProjectionCheckpointRepairAudit()
	case "projection-checkpoint-repair-cleanup":
		return runProjectionCheckpointRepairCleanup()
	case "projection-failure-audit":
		return runProjectionFailureAudit()
	case "projection-failure-resolve":
		return runProjectionFailureResolve()
	case "projection-failure-cleanup":
		return runProjectionFailureCleanup()
	default:
		return errors.New("unsupported NEXUSIM_DELIVERY_SERVICE_MODE")
	}
}

func runGRPCServer() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	listenAddr := envString("NEXUSIM_DELIVERY_GRPC_ADDR", "0.0.0.0:10497")
	authMode := envString("NEXUSIM_DELIVERY_AUTH_MODE", "body")
	serverTLSConfig, serverTLSEnabled, err := deliveryGRPCTLSConfigFromEnv()
	if err != nil {
		return err
	}
	if err := validateTrustedMetadataListenerConfig(listenAddr, authMode, serverTLSConfig); err != nil {
		return err
	}
	grpcMetrics := monitoringinfra.NewGRPCMetrics()
	traceConfig, err := deliveryTraceConfigFromEnv()
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
			log.Printf("delivery-service OpenTelemetry trace shutdown failed: %v", err)
		}
	}()
	debugAddr, err := deliveryDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr, monitoringinfra.NewHandler(pool, grpcMetrics).WithTraceStats(traceRuntime.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()
	server, err := newGRPCServerWithConfig(grpcMetrics, authMode, serverTLSConfig, serverTLSEnabled, traceRuntime.UnaryServerInterceptor())
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	repository := postgresinfra.NewRepository(pool)
	grpcapi.Register(
		server,
		grpcapi.NewServer(
			app.NewPullInboxUseCase(repository),
			app.NewAckDeliveryUseCase(repository),
			app.NewHideInboxItemUseCase(repository),
		),
	)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("delivery-service gRPC server started on %s", listenAddr)

	select {
	case err := <-serveErr:
		if errors.Is(err, grpc.ErrServerStopped) {
			return context.Canceled
		}
		return err
	case <-ctx.Done():
		server.GracefulStop()
		err := <-serveErr
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			return err
		}
		return context.Canceled
	}
}

func runTimelineConsumer() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	brokers := splitCSV(os.Getenv("NEXUSIM_KAFKA_BROKERS"))
	topic := envString("NEXUSIM_TIMELINE_TOPIC", "conversation.timeline.events")
	groupID := envString("NEXUSIM_DELIVERY_CONSUMER_GROUP", "nexusim-delivery-service")
	consumer, err := kafkainfra.NewReaderConsumer(kafkainfra.ReaderConfig{
		Brokers: brokers,
		Topic:   topic,
		GroupID: groupID,
	})
	if err != nil {
		return err
	}
	defer consumer.Close()

	repository := postgresinfra.NewRepository(pool)
	worker := timeline.NewWorker(
		consumer,
		app.NewProjectTimelineEventUseCase(repository),
		groupID,
		postgresinfra.NewProjectionFailureStore(pool),
		timeline.Config{
			ErrorBackoff: envDuration("NEXUSIM_DELIVERY_TIMELINE_CONSUMER_ERROR_BACKOFF", time.Second),
			Logf:         log.Printf,
		},
	)
	debugAddr, err := deliveryDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr, monitoringinfra.NewHandler(pool).WithTimelineProjectionWorkerStats(worker.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()
	log.Printf("delivery-service timeline consumer started topic=%s group=%s", topic, groupID)
	return worker.Run(ctx)
}

func runOutboxRelay() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	brokers := splitCSV(os.Getenv("NEXUSIM_KAFKA_BROKERS"))
	producer, err := kafkainfra.NewWriterProducer(brokers)
	if err != nil {
		return err
	}
	defer producer.Close()

	topic := envString("NEXUSIM_DELIVERY_EVENTS_TOPIC", outbox.TopicDeliveryEvents)
	relay := outbox.NewRelay(
		postgresinfra.NewOutboxStore(pool),
		producer,
		outbox.Config{
			Topic:          topic,
			BatchSize:      envInt("NEXUSIM_DELIVERY_OUTBOX_BATCH_SIZE", 500),
			PollInterval:   envDuration("NEXUSIM_DELIVERY_OUTBOX_POLL_INTERVAL", time.Second),
			MaxAttempts:    envInt("NEXUSIM_DELIVERY_OUTBOX_MAX_ATTEMPTS", 5),
			RetryBaseDelay: envDuration("NEXUSIM_DELIVERY_OUTBOX_RETRY_BASE_DELAY", time.Second),
			ErrorBackoff:   envDuration("NEXUSIM_DELIVERY_OUTBOX_RELAY_ERROR_BACKOFF", time.Second),
			Logf:           log.Printf,
		},
	)
	debugAddr, err := deliveryDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr, monitoringinfra.NewHandler(pool).WithOutboxRelayStats(relay.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()
	log.Printf("delivery-service outbox relay started topic=%s", topic)
	return relay.Run(ctx)
}

func runOutboxRepair() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	ids, err := parseInt64CSV(os.Getenv("NEXUSIM_DELIVERY_OUTBOX_REPAIR_IDS"))
	if err != nil {
		return err
	}
	mode := envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_MODE", types.OutboxRepairModeAudit)
	dryRun := envBool("NEXUSIM_DELIVERY_OUTBOX_REPAIR_DRY_RUN", false)
	stats, err := postgresinfra.NewOutboxStore(pool).RepairOutbox(
		ctx,
		types.OutboxRepairOptions{
			OutboxIDs: ids,
			Mode:      mode,
			Operator:  envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_OPERATOR", "manual"),
			Reason:    envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_REASON", "manual delivery outbox repair"),
			DryRun:    dryRun,
		},
	)
	if err != nil {
		return err
	}
	log.Printf(
		"delivery-service outbox repair completed requested=%d audited=%d mutated=%d skipped=%d mode=%s dry_run=%t",
		stats.Requested,
		stats.Audited,
		stats.Mutated,
		stats.Skipped,
		mode,
		dryRun,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_OUTBOX_REPAIR_OUTPUT")); outputPath != "" {
		if err := writeOutboxRepairOutput(outputPath, stats, len(ids), mode, dryRun); err != nil {
			return err
		}
	}
	return nil
}

func runOutboxAudit() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	var outboxID *int64
	outboxIDFilter := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_OUTBOX_AUDIT_OUTBOX_ID"))
	if outboxIDFilter != "" {
		parsed := envInt64AllowZero("NEXUSIM_DELIVERY_OUTBOX_AUDIT_OUTBOX_ID", 0)
		outboxID = &parsed
	}
	eventID := envString("NEXUSIM_DELIVERY_OUTBOX_AUDIT_EVENT_ID", "")
	tenantID := envString("NEXUSIM_DELIVERY_OUTBOX_AUDIT_TENANT_ID", "")
	conversationID := envString("NEXUSIM_DELIVERY_OUTBOX_AUDIT_CONVERSATION_ID", "")
	status := envString("NEXUSIM_DELIVERY_OUTBOX_AUDIT_STATUS", "")
	eventType := envString("NEXUSIM_DELIVERY_OUTBOX_AUDIT_EVENT_TYPE", "")
	createdAfter, err := envOptionalRFC3339Time("NEXUSIM_DELIVERY_OUTBOX_AUDIT_CREATED_AFTER")
	if err != nil {
		return err
	}
	createdBefore, err := envOptionalRFC3339Time("NEXUSIM_DELIVERY_OUTBOX_AUDIT_CREATED_BEFORE")
	if err != nil {
		return err
	}
	rows, err := postgresinfra.NewOutboxStore(pool).AuditOutbox(ctx, postgresinfra.OutboxAuditOptions{
		OutboxID:       outboxID,
		EventID:        eventID,
		TenantID:       tenantID,
		ConversationID: conversationID,
		Status:         status,
		EventType:      eventType,
		CreatedAfter:   createdAfter,
		CreatedBefore:  createdBefore,
		Limit:          envInt("NEXUSIM_DELIVERY_OUTBOX_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("delivery-service outbox audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"delivery_outbox id=%d event_id=%s tenant_id=%s conversation_id=%s aggregate_version=%d event_type=%s status=%s retry_count=%d available_at=%s published_at=%s dead_lettered_at=%s last_error=%q",
			row.ID,
			row.EventID,
			row.TenantID,
			row.ConversationID,
			row.AggregateVersion,
			row.EventType,
			row.Status,
			row.RetryCount,
			row.AvailableAt.Format(time.RFC3339),
			formatOptionalTime(row.PublishedAt),
			formatOptionalTime(row.DeadLetteredAt),
			row.LastError,
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_OUTBOX_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeOutboxAuditOutput(outputPath, rows, map[string]string{
			"outbox_id":       outboxIDFilter,
			"event_id":        eventID,
			"tenant_id":       tenantID,
			"conversation_id": conversationID,
			"status":          status,
			"event_type":      eventType,
			"created_after":   formatOptionalTime(createdAfter),
			"created_before":  formatOptionalTime(createdBefore),
		}); err != nil {
			return err
		}
	}
	return nil
}

func runOutboxRepairAudit() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	var outboxID *int64
	if value := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_OUTBOX_REPAIR_AUDIT_OUTBOX_ID")); value != "" {
		parsed := envInt64AllowZero("NEXUSIM_DELIVERY_OUTBOX_REPAIR_AUDIT_OUTBOX_ID", 0)
		outboxID = &parsed
	}
	repairedAfter, err := envOptionalRFC3339Time("NEXUSIM_DELIVERY_OUTBOX_REPAIR_AUDIT_REPAIRED_AFTER")
	if err != nil {
		return err
	}
	repairedBefore, err := envOptionalRFC3339Time("NEXUSIM_DELIVERY_OUTBOX_REPAIR_AUDIT_REPAIRED_BEFORE")
	if err != nil {
		return err
	}
	filters := map[string]string{
		"event_id":        envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_AUDIT_EVENT_ID", ""),
		"tenant_id":       envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_AUDIT_TENANT_ID", ""),
		"conversation_id": envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_AUDIT_CONVERSATION_ID", ""),
		"mode":            envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_AUDIT_MODE", ""),
		"outcome":         envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_AUDIT_OUTCOME", ""),
		"repaired_after":  formatOptionalTime(repairedAfter),
		"repaired_before": formatOptionalTime(repairedBefore),
	}
	if outboxID != nil {
		filters["outbox_id"] = strconv.FormatInt(*outboxID, 10)
	}
	rows, err := postgresinfra.NewOutboxStore(pool).AuditOutboxRepairs(ctx, postgresinfra.OutboxRepairAuditOptions{
		OutboxID:       outboxID,
		EventID:        filters["event_id"],
		TenantID:       filters["tenant_id"],
		ConversationID: filters["conversation_id"],
		Mode:           filters["mode"],
		Outcome:        filters["outcome"],
		RepairedAfter:  repairedAfter,
		RepairedBefore: repairedBefore,
		Limit:          envInt("NEXUSIM_DELIVERY_OUTBOX_REPAIR_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("delivery-service outbox repair audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"delivery_outbox_repair outbox_id=%d event_id=%s tenant_id=%s conversation_id=%s aggregate_version=%d mode=%s outcome=%s skip_reason=%s dry_run=%t before_status=%s before_retry=%d after_status=%s after_retry=%d operator=%s created_at=%s reason=%q",
			row.OutboxID,
			row.EventID,
			row.TenantID,
			row.ConversationID,
			row.AggregateVersion,
			row.Mode,
			row.Outcome,
			row.SkipReason,
			row.DryRun,
			row.BeforeStatus,
			row.BeforeRetryCount,
			row.AfterStatus,
			row.AfterRetryCount,
			row.Operator,
			row.CreatedAt.Format(time.RFC3339),
			row.Reason,
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_OUTBOX_REPAIR_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeOutboxRepairAuditOutput(outputPath, rows, filters); err != nil {
			return err
		}
	}
	return nil
}

func runOutboxRepairCleanup() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	config, err := outboxRepairCleanupConfigFromEnv()
	if err != nil {
		return err
	}
	cutoff := time.Now().UTC().Add(-config.Retention)
	outboxID := outboxRepairCleanupOutboxID()
	stats, err := postgresinfra.NewOutboxStore(pool).CleanupOutboxRepairs(ctx, postgresinfra.OutboxRepairCleanupOptions{
		OutboxID:       outboxID,
		EventID:        envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_CLEANUP_EVENT_ID", ""),
		TenantID:       envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_CLEANUP_TENANT_ID", ""),
		ConversationID: envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_CLEANUP_CONVERSATION_ID", ""),
		Mode:           envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_CLEANUP_MODE", ""),
		Outcome:        envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_CLEANUP_OUTCOME", ""),
		Cutoff:         cutoff,
		Limit:          config.BatchSize,
	})
	if err != nil {
		return err
	}
	log.Printf(
		"delivery-service outbox repair cleanup completed deleted=%d cutoff=%s retention=%s batch_size=%d",
		stats.Deleted,
		cutoff.Format(time.RFC3339),
		config.Retention,
		config.BatchSize,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_OUTBOX_REPAIR_CLEANUP_OUTPUT")); outputPath != "" {
		filters := map[string]string{
			"event_id":        envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_CLEANUP_EVENT_ID", ""),
			"tenant_id":       envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_CLEANUP_TENANT_ID", ""),
			"conversation_id": envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_CLEANUP_CONVERSATION_ID", ""),
			"mode":            envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_CLEANUP_MODE", ""),
			"outcome":         envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_CLEANUP_OUTCOME", ""),
		}
		if outboxID != nil {
			filters["outbox_id"] = strconv.FormatInt(*outboxID, 10)
		}
		if err := writeOutboxRepairCleanupOutput(outputPath, stats, cutoff, config.Retention, config.BatchSize, filters); err != nil {
			return err
		}
	}
	return nil
}

func runProjectionCheckpointRepair() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	mode := envString("NEXUSIM_DELIVERY_PROJECTION_REPAIR_MODE", types.ProjectionCheckpointRepairModeAudit)
	dryRun := envBool("NEXUSIM_DELIVERY_PROJECTION_REPAIR_DRY_RUN", false)
	targetOffset := envInt64AllowZero("NEXUSIM_DELIVERY_PROJECTION_REPAIR_TARGET_OFFSET", 0)
	failureOffset := envInt64AllowZero("NEXUSIM_DELIVERY_PROJECTION_REPAIR_FAILURE_OFFSET", 0)
	stats, err := postgresinfra.NewProjectionRepairStore(pool).RepairCheckpoint(
		ctx,
		types.ProjectionCheckpointRepairOptions{
			ConsumerGroup: envString("NEXUSIM_DELIVERY_PROJECTION_REPAIR_CONSUMER_GROUP", ""),
			Topic:         envString("NEXUSIM_DELIVERY_PROJECTION_REPAIR_TOPIC", "conversation.timeline.events"),
			PartitionID:   int32(envIntAllowZero("NEXUSIM_DELIVERY_PROJECTION_REPAIR_PARTITION_ID", 0)),
			TargetOffset:  targetOffset,
			FailureOffset: failureOffset,
			Mode:          mode,
			Operator:      envString("NEXUSIM_DELIVERY_PROJECTION_REPAIR_OPERATOR", "manual"),
			Reason:        envString("NEXUSIM_DELIVERY_PROJECTION_REPAIR_REASON", "manual delivery projection checkpoint repair"),
			DryRun:        dryRun,
		},
	)
	if err != nil {
		return err
	}
	log.Printf(
		"delivery-service projection checkpoint repair completed requested=%d audited=%d mutated=%d skipped=%d mode=%s target_offset=%d failure_offset=%d dry_run=%t",
		stats.Requested,
		stats.Audited,
		stats.Mutated,
		stats.Skipped,
		mode,
		targetOffset,
		failureOffset,
		dryRun,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_PROJECTION_REPAIR_OUTPUT")); outputPath != "" {
		if err := writeProjectionRepairOutput(outputPath, stats, mode, targetOffset, failureOffset, dryRun); err != nil {
			return err
		}
	}
	return nil
}

func runProjectionFailureAudit() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	var partitionID *int32
	if value := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_PARTITION_ID")); value != "" {
		parsed := int32(envIntAllowZero("NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_PARTITION_ID", 0))
		partitionID = &parsed
	}
	var offsetValue *int64
	if value := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_OFFSET_VALUE")); value != "" {
		parsed := envInt64AllowZero("NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_OFFSET_VALUE", 0)
		offsetValue = &parsed
	}
	lastSeenAfter, err := envOptionalRFC3339Time("NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_LAST_SEEN_AFTER")
	if err != nil {
		return err
	}
	lastSeenBefore, err := envOptionalRFC3339Time("NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_LAST_SEEN_BEFORE")
	if err != nil {
		return err
	}
	includeResolved := envBool("NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_INCLUDE_RESOLVED", false)
	consumerGroup := envString("NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_CONSUMER_GROUP", "")
	topic := envString("NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_TOPIC", "conversation.timeline.events")
	eventID := envString("NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_EVENT_ID", "")
	eventType := envString("NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_EVENT_TYPE", "")
	failureClass := envString("NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_FAILURE_CLASS", "")
	rows, err := postgresinfra.NewProjectionFailureStore(pool).AuditFailures(ctx, postgresinfra.ProjectionFailureAuditOptions{
		ConsumerGroup:  consumerGroup,
		Topic:          topic,
		PartitionID:    partitionID,
		OffsetValue:    offsetValue,
		EventID:        eventID,
		EventType:      eventType,
		FailureClass:   failureClass,
		LastSeenAfter:  lastSeenAfter,
		LastSeenBefore: lastSeenBefore,
		UnresolvedOnly: !includeResolved,
		Limit:          envInt("NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf(
		"delivery-service projection failure audit completed rows=%d include_resolved=%t",
		len(rows),
		includeResolved,
	)
	for _, row := range rows {
		resolved := row.ResolvedAt != nil
		log.Printf(
			"projection_failure consumer_group=%s topic=%s partition_id=%d offset=%d event_id=%s event_type=%s class=%s failure_count=%d resolved=%t last_seen=%s error=%q",
			row.ConsumerGroup,
			row.Topic,
			row.PartitionID,
			row.OffsetValue,
			row.EventID,
			row.EventType,
			row.FailureClass,
			row.FailureCount,
			resolved,
			row.LastSeenAt.Format(time.RFC3339),
			row.LastError,
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_OUTPUT")); outputPath != "" {
		filters := map[string]string{
			"consumer_group":   consumerGroup,
			"topic":            topic,
			"event_id":         eventID,
			"event_type":       eventType,
			"failure_class":    failureClass,
			"last_seen_after":  formatOptionalTime(lastSeenAfter),
			"last_seen_before": formatOptionalTime(lastSeenBefore),
		}
		if partitionID != nil {
			filters["partition_id"] = strconv.FormatInt(int64(*partitionID), 10)
		}
		if offsetValue != nil {
			filters["offset_value"] = strconv.FormatInt(*offsetValue, 10)
		}
		if includeResolved {
			filters["include_resolved"] = strconv.FormatBool(includeResolved)
		}
		if err := writeProjectionFailureAuditOutput(outputPath, rows, includeResolved, filters); err != nil {
			return err
		}
	}
	if !includeResolved && len(rows) > 0 {
		return fmt.Errorf("delivery projection failure audit found %d unresolved rows", len(rows))
	}
	return nil
}

func runProjectionFailureResolve() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	offsetValue, err := envRequiredInt64AllowZero("NEXUSIM_DELIVERY_PROJECTION_FAILURE_RESOLVE_OFFSET_VALUE")
	if err != nil {
		return err
	}
	options := types.ProjectionFailureResolveOptions{
		ConsumerGroup: envString("NEXUSIM_DELIVERY_PROJECTION_FAILURE_RESOLVE_CONSUMER_GROUP", ""),
		Topic:         envString("NEXUSIM_DELIVERY_PROJECTION_FAILURE_RESOLVE_TOPIC", "conversation.timeline.events"),
		PartitionID:   int32(envIntAllowZero("NEXUSIM_DELIVERY_PROJECTION_FAILURE_RESOLVE_PARTITION_ID", 0)),
		OffsetValue:   offsetValue,
		Operator:      envString("NEXUSIM_DELIVERY_PROJECTION_FAILURE_RESOLVE_OPERATOR", "manual"),
		Reason:        envString("NEXUSIM_DELIVERY_PROJECTION_FAILURE_RESOLVE_REASON", "manual delivery projection failure resolution"),
		DryRun:        envBool("NEXUSIM_DELIVERY_PROJECTION_FAILURE_RESOLVE_DRY_RUN", false),
	}
	stats, err := postgresinfra.NewProjectionFailureStore(pool).ResolveFailure(ctx, options)
	if err != nil {
		return err
	}
	log.Printf(
		"delivery-service projection failure resolve completed requested=%d audited=%d resolved=%d consumer_group=%s topic=%s partition_id=%d offset=%d dry_run=%t reason_present=%t",
		stats.Requested,
		stats.Audited,
		stats.Resolved,
		options.ConsumerGroup,
		options.Topic,
		options.PartitionID,
		options.OffsetValue,
		options.DryRun,
		strings.TrimSpace(options.Reason) != "",
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_PROJECTION_FAILURE_RESOLVE_OUTPUT")); outputPath != "" {
		if err := writeProjectionFailureResolveOutput(outputPath, stats, options); err != nil {
			return err
		}
	}
	return nil
}

func runProjectionCheckpointRepairAudit() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	var partitionID *int32
	if value := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_PROJECTION_REPAIR_AUDIT_PARTITION_ID")); value != "" {
		parsed := int32(envIntAllowZero("NEXUSIM_DELIVERY_PROJECTION_REPAIR_AUDIT_PARTITION_ID", 0))
		partitionID = &parsed
	}
	rows, err := postgresinfra.NewProjectionRepairStore(pool).AuditCheckpointRepairs(ctx, postgresinfra.ProjectionRepairAuditOptions{
		ConsumerGroup: envString("NEXUSIM_DELIVERY_PROJECTION_REPAIR_AUDIT_CONSUMER_GROUP", ""),
		Topic:         envString("NEXUSIM_DELIVERY_PROJECTION_REPAIR_AUDIT_TOPIC", "conversation.timeline.events"),
		PartitionID:   partitionID,
		Mode:          envString("NEXUSIM_DELIVERY_PROJECTION_REPAIR_AUDIT_MODE", ""),
		Outcome:       envString("NEXUSIM_DELIVERY_PROJECTION_REPAIR_AUDIT_OUTCOME", ""),
		Limit:         envInt("NEXUSIM_DELIVERY_PROJECTION_REPAIR_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("delivery-service projection checkpoint repair audit completed rows=%d", len(rows))
	for _, row := range rows {
		failureOffset := int64(-1)
		if row.FailureOffset != nil {
			failureOffset = *row.FailureOffset
		}
		log.Printf(
			"projection_checkpoint_repair consumer_group=%s topic=%s partition_id=%d mode=%s outcome=%s skip_reason=%s dry_run=%t before_offset=%d after_offset=%d failure_offset=%d failure_event_id=%s failure_class=%s operator=%s created_at=%s reason=%q",
			row.ConsumerGroup,
			row.Topic,
			row.PartitionID,
			row.Mode,
			row.Outcome,
			row.SkipReason,
			row.DryRun,
			row.BeforeOffset,
			row.AfterOffset,
			failureOffset,
			row.FailureEvent,
			row.FailureClass,
			row.Operator,
			row.CreatedAt.Format(time.RFC3339),
			row.Reason,
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_PROJECTION_REPAIR_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeProjectionRepairAuditOutput(outputPath, rows); err != nil {
			return err
		}
	}
	return nil
}

func runProjectionFailureCleanup() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	config, err := projectionFailureCleanupConfigFromEnv()
	if err != nil {
		return err
	}
	cutoff := time.Now().UTC().Add(-config.Retention)
	partitionID := projectionFailureCleanupPartitionID()
	stats, err := postgresinfra.NewProjectionFailureStore(pool).CleanupResolvedFailures(ctx, postgresinfra.ProjectionFailureCleanupOptions{
		ConsumerGroup: envString("NEXUSIM_DELIVERY_PROJECTION_FAILURE_CLEANUP_CONSUMER_GROUP", ""),
		Topic:         envString("NEXUSIM_DELIVERY_PROJECTION_FAILURE_CLEANUP_TOPIC", "conversation.timeline.events"),
		PartitionID:   partitionID,
		FailureClass:  envString("NEXUSIM_DELIVERY_PROJECTION_FAILURE_CLEANUP_FAILURE_CLASS", ""),
		Cutoff:        cutoff,
		Limit:         config.BatchSize,
	})
	if err != nil {
		return err
	}
	log.Printf(
		"delivery-service projection failure cleanup completed deleted=%d cutoff=%s retention=%s batch_size=%d",
		stats.Deleted,
		cutoff.Format(time.RFC3339),
		config.Retention,
		config.BatchSize,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_PROJECTION_FAILURE_CLEANUP_OUTPUT")); outputPath != "" {
		filters := map[string]string{
			"consumer_group": envString("NEXUSIM_DELIVERY_PROJECTION_FAILURE_CLEANUP_CONSUMER_GROUP", ""),
			"topic":          envString("NEXUSIM_DELIVERY_PROJECTION_FAILURE_CLEANUP_TOPIC", "conversation.timeline.events"),
			"failure_class":  envString("NEXUSIM_DELIVERY_PROJECTION_FAILURE_CLEANUP_FAILURE_CLASS", ""),
		}
		if partitionID != nil {
			filters["partition_id"] = strconv.FormatInt(int64(*partitionID), 10)
		}
		if err := writeOperatorCleanupOutput(outputPath, stats.Deleted, cutoff, config.Retention, config.BatchSize, filters); err != nil {
			return err
		}
	}
	return nil
}

func runProjectionCheckpointRepairCleanup() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	config, err := projectionCheckpointRepairCleanupConfigFromEnv()
	if err != nil {
		return err
	}
	cutoff := time.Now().UTC().Add(-config.Retention)
	partitionID := projectionCheckpointRepairCleanupPartitionID()
	stats, err := postgresinfra.NewProjectionRepairStore(pool).CleanupCheckpointRepairs(ctx, postgresinfra.ProjectionRepairCleanupOptions{
		ConsumerGroup: envString("NEXUSIM_DELIVERY_PROJECTION_REPAIR_CLEANUP_CONSUMER_GROUP", ""),
		Topic:         envString("NEXUSIM_DELIVERY_PROJECTION_REPAIR_CLEANUP_TOPIC", "conversation.timeline.events"),
		PartitionID:   partitionID,
		Mode:          envString("NEXUSIM_DELIVERY_PROJECTION_REPAIR_CLEANUP_MODE", ""),
		Outcome:       envString("NEXUSIM_DELIVERY_PROJECTION_REPAIR_CLEANUP_OUTCOME", ""),
		Cutoff:        cutoff,
		Limit:         config.BatchSize,
	})
	if err != nil {
		return err
	}
	log.Printf(
		"delivery-service projection checkpoint repair cleanup completed deleted=%d cutoff=%s retention=%s batch_size=%d",
		stats.Deleted,
		cutoff.Format(time.RFC3339),
		config.Retention,
		config.BatchSize,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_PROJECTION_REPAIR_CLEANUP_OUTPUT")); outputPath != "" {
		filters := map[string]string{
			"consumer_group": envString("NEXUSIM_DELIVERY_PROJECTION_REPAIR_CLEANUP_CONSUMER_GROUP", ""),
			"topic":          envString("NEXUSIM_DELIVERY_PROJECTION_REPAIR_CLEANUP_TOPIC", "conversation.timeline.events"),
			"mode":           envString("NEXUSIM_DELIVERY_PROJECTION_REPAIR_CLEANUP_MODE", ""),
			"outcome":        envString("NEXUSIM_DELIVERY_PROJECTION_REPAIR_CLEANUP_OUTCOME", ""),
		}
		if partitionID != nil {
			filters["partition_id"] = strconv.FormatInt(int64(*partitionID), 10)
		}
		if err := writeOperatorCleanupOutput(outputPath, stats.Deleted, cutoff, config.Retention, config.BatchSize, filters); err != nil {
			return err
		}
	}
	return nil
}

type projectionFailureCleanupConfig struct {
	Retention time.Duration
	BatchSize int
}

type outboxRepairCleanupConfig struct {
	Retention time.Duration
	BatchSize int
}

type projectionCheckpointRepairCleanupConfig struct {
	Retention time.Duration
	BatchSize int
}

func projectionFailureCleanupConfigFromEnv() (projectionFailureCleanupConfig, error) {
	retention, err := envPositiveDuration("NEXUSIM_DELIVERY_PROJECTION_FAILURE_RETENTION", 7*24*time.Hour)
	if err != nil {
		return projectionFailureCleanupConfig{}, err
	}
	batchSize, err := envPositiveInt("NEXUSIM_DELIVERY_PROJECTION_FAILURE_CLEANUP_BATCH_SIZE", 5000)
	if err != nil {
		return projectionFailureCleanupConfig{}, err
	}
	return projectionFailureCleanupConfig{
		Retention: retention,
		BatchSize: batchSize,
	}, nil
}

func outboxRepairCleanupConfigFromEnv() (outboxRepairCleanupConfig, error) {
	retention, err := envPositiveDuration("NEXUSIM_DELIVERY_OUTBOX_REPAIR_RETENTION", 7*24*time.Hour)
	if err != nil {
		return outboxRepairCleanupConfig{}, err
	}
	batchSize, err := envPositiveInt("NEXUSIM_DELIVERY_OUTBOX_REPAIR_CLEANUP_BATCH_SIZE", 5000)
	if err != nil {
		return outboxRepairCleanupConfig{}, err
	}
	return outboxRepairCleanupConfig{
		Retention: retention,
		BatchSize: batchSize,
	}, nil
}

func projectionCheckpointRepairCleanupConfigFromEnv() (projectionCheckpointRepairCleanupConfig, error) {
	retention, err := envPositiveDuration("NEXUSIM_DELIVERY_PROJECTION_REPAIR_RETENTION", 7*24*time.Hour)
	if err != nil {
		return projectionCheckpointRepairCleanupConfig{}, err
	}
	batchSize, err := envPositiveInt("NEXUSIM_DELIVERY_PROJECTION_REPAIR_CLEANUP_BATCH_SIZE", 5000)
	if err != nil {
		return projectionCheckpointRepairCleanupConfig{}, err
	}
	return projectionCheckpointRepairCleanupConfig{
		Retention: retention,
		BatchSize: batchSize,
	}, nil
}

func projectionFailureCleanupPartitionID() *int32 {
	value := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_PROJECTION_FAILURE_CLEANUP_PARTITION_ID"))
	if value == "" {
		return nil
	}
	parsed := int32(envIntAllowZero("NEXUSIM_DELIVERY_PROJECTION_FAILURE_CLEANUP_PARTITION_ID", 0))
	return &parsed
}

func outboxRepairCleanupOutboxID() *int64 {
	value := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_OUTBOX_REPAIR_CLEANUP_OUTBOX_ID"))
	if value == "" {
		return nil
	}
	parsed := envInt64AllowZero("NEXUSIM_DELIVERY_OUTBOX_REPAIR_CLEANUP_OUTBOX_ID", 0)
	return &parsed
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func envOptionalRFC3339Time(name string) (*time.Time, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, fmt.Errorf("%s must be RFC3339: %w", name, err)
	}
	utc := parsed.UTC()
	return &utc, nil
}

func projectionCheckpointRepairCleanupPartitionID() *int32 {
	value := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_PROJECTION_REPAIR_CLEANUP_PARTITION_ID"))
	if value == "" {
		return nil
	}
	parsed := int32(envIntAllowZero("NEXUSIM_DELIVERY_PROJECTION_REPAIR_CLEANUP_PARTITION_ID", 0))
	return &parsed
}

func openPGPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if maxConns := envInt("NEXUSIM_DELIVERY_PG_MAX_CONNS", 0); maxConns > 0 {
		config.MaxConns = int32(maxConns)
	}
	return pgxpool.NewWithConfig(ctx, config)
}

func envString(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func newGRPCServer(grpcMetrics *monitoringinfra.GRPCMetrics) (*grpc.Server, error) {
	authMode := envString("NEXUSIM_DELIVERY_AUTH_MODE", "body")
	tlsConfig, ok, err := deliveryGRPCTLSConfigFromEnv()
	if err != nil {
		return nil, err
	}
	return newGRPCServerWithConfig(grpcMetrics, authMode, tlsConfig, ok)
}

func newGRPCServerWithConfig(grpcMetrics *monitoringinfra.GRPCMetrics, authMode string, tlsConfig *tls.Config, tlsEnabled bool, traceInterceptors ...grpc.UnaryServerInterceptor) (*grpc.Server, error) {
	interceptors := make([]grpc.UnaryServerInterceptor, 0, 3)
	if grpcMetrics != nil {
		interceptors = append(interceptors, grpcMetrics.UnaryServerInterceptor(log.Default()))
	} else {
		interceptors = append(interceptors, monitoringinfra.UnaryAccessLogInterceptor(log.Default()))
	}
	for _, interceptor := range traceInterceptors {
		if interceptor != nil {
			interceptors = append(interceptors, interceptor)
		}
	}
	switch strings.ToLower(strings.TrimSpace(authMode)) {
	case "body", "request", "legacy":
	case "metadata", "verified-metadata":
		interceptors = append(interceptors, grpcapi.VerifiedAuthUnaryInterceptor(true))
	default:
		return nil, errors.New("unsupported NEXUSIM_DELIVERY_AUTH_MODE")
	}
	serverOptions := make([]grpc.ServerOption, 0, 2)
	if len(interceptors) > 0 {
		serverOptions = append(serverOptions, grpc.ChainUnaryInterceptor(interceptors...))
	}
	if tlsEnabled {
		serverOptions = append(serverOptions, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}
	return grpc.NewServer(serverOptions...), nil
}

func deliveryTraceConfigFromEnv() (monitoringinfra.TraceConfig, error) {
	enabled, _, err := envOptionalBool("NEXUSIM_DELIVERY_OTEL_TRACES_ENABLED")
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	otlpInsecure, _, err := envOptionalBool("NEXUSIM_DELIVERY_OTEL_TRACES_OTLP_INSECURE")
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	samplingRatio, err := deliveryTraceSamplingRatioFromEnv()
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	return monitoringinfra.TraceConfig{
		Enabled:       enabled,
		ServiceName:   envString("NEXUSIM_DELIVERY_OTEL_SERVICE_NAME", "delivery-service"),
		Exporter:      envString("NEXUSIM_DELIVERY_OTEL_TRACES_EXPORTER", "stdout"),
		OTLPEndpoint:  envString("NEXUSIM_DELIVERY_OTEL_TRACES_OTLP_ENDPOINT", ""),
		OTLPInsecure:  otlpInsecure,
		SamplingRatio: samplingRatio,
	}, nil
}

func deliveryTraceSamplingRatioFromEnv() (float64, error) {
	raw := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_OTEL_TRACES_SAMPLING_RATIO"))
	if raw == "" {
		return 1, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 || value > 1 {
		return 0, errors.New("NEXUSIM_DELIVERY_OTEL_TRACES_SAMPLING_RATIO must be > 0 and <= 1")
	}
	return value, nil
}

func validateTrustedMetadataListenerConfig(listenAddr string, authMode string, tlsConfig *tls.Config) error {
	if !usesTrustedMetadataAuth(authMode) {
		return nil
	}
	if listenerAddrTrustedWithoutMTLS(listenAddr) {
		return nil
	}
	if tlsConfig != nil && tlsConfig.ClientAuth == tls.RequireAndVerifyClientCert {
		return nil
	}
	return errors.New("delivery-service uses verified metadata auth on non-private address without gRPC mTLS client certificate")
}

func usesTrustedMetadataAuth(authMode string) bool {
	switch strings.ToLower(strings.TrimSpace(authMode)) {
	case "metadata", "verified-metadata":
		return true
	default:
		return false
	}
}

func listenerAddrTrustedWithoutMTLS(addr string) bool {
	host := strings.TrimSpace(addr)
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		host = splitHost
	}
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}

func startDebugServer(ctx context.Context, addr string, handler http.Handler) (func(), error) {
	if strings.TrimSpace(addr) == "" {
		return func() {}, nil
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: handler}
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	go func() {
		defer close(done)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("delivery-service debug server stopped with error: %v", err)
		}
	}()
	log.Printf("delivery-service debug server started on %s", addr)
	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		<-done
	}, nil
}

func deliveryDebugAddr() string {
	return envString("NEXUSIM_DELIVERY_DEBUG_ADDR", envString("NEXUSIM_DEBUG_ADDR", ""))
}

func deliveryDebugAddrFromEnv() (string, error) {
	addr := deliveryDebugAddr()
	allowPublic, _, err := envOptionalBool("NEXUSIM_DELIVERY_DEBUG_ALLOW_PUBLIC")
	if err != nil {
		return "", err
	}
	return addr, validateDeliveryDebugListenerConfig(addr, allowPublic)
}

func validateDeliveryDebugListenerConfig(addr string, allowPublic bool) error {
	if strings.TrimSpace(addr) == "" {
		return nil
	}
	if listenerAddrTrustedWithoutMTLS(addr) {
		return nil
	}
	if allowPublic {
		return nil
	}
	return errors.New("delivery-service debug listener address is non-private; set NEXUSIM_DELIVERY_DEBUG_ALLOW_PUBLIC=true to allow")
}

func loadDeliveryGRPCCredentialsFromEnv() (credentials.TransportCredentials, bool, error) {
	tlsConfig, ok, err := deliveryGRPCTLSConfigFromEnv()
	if err != nil || !ok {
		return nil, ok, err
	}
	return credentials.NewTLS(tlsConfig), true, nil
}

func deliveryGRPCTLSConfigFromEnv() (*tls.Config, bool, error) {
	certFile := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_GRPC_TLS_CERT_FILE"))
	keyFile := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_GRPC_TLS_KEY_FILE"))
	clientCAFile := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_CA_FILE"))
	allowedClientDNSNames := envStringSet("NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", strings.ToLower)
	allowedClientURIs, err := envURIStringSet("NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_ALLOWED_URIS")
	if err != nil {
		return nil, true, err
	}
	requireClientCert, requireClientCertConfigured, err := envOptionalBool("NEXUSIM_DELIVERY_GRPC_TLS_REQUIRE_CLIENT_CERT")
	if err != nil {
		return nil, true, err
	}
	hasClientAllowlist := len(allowedClientDNSNames) > 0 || len(allowedClientURIs) > 0
	requireClientCert = clientCAFile != "" || hasClientAllowlist || (requireClientCertConfigured && requireClientCert)
	if certFile == "" && keyFile == "" && clientCAFile == "" && !requireClientCert && !hasClientAllowlist {
		return nil, false, nil
	}
	if certFile == "" || keyFile == "" {
		return nil, true, errors.New("NEXUSIM_DELIVERY_GRPC_TLS_CERT_FILE and NEXUSIM_DELIVERY_GRPC_TLS_KEY_FILE must be configured together")
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, true, err
	}
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	if requireClientCert {
		if clientCAFile == "" {
			return nil, true, errors.New("NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_CA_FILE is required when client certificates are required")
		}
		pemBytes, err := os.ReadFile(clientCAFile)
		if err != nil {
			return nil, true, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, true, errors.New("NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_CA_FILE does not contain a valid PEM certificate")
		}
		tlsConfig.ClientCAs = pool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		if hasClientAllowlist {
			tlsConfig.VerifyConnection = verifyAllowedDeliveryGRPCClient(allowedClientDNSNames, allowedClientURIs)
		}
	}
	return tlsConfig, true, nil
}

func verifyAllowedDeliveryGRPCClient(allowedDNSNames map[string]struct{}, allowedURIs map[string]struct{}) func(tls.ConnectionState) error {
	return func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("delivery grpc client certificate is required")
		}
		cert := state.PeerCertificates[0]
		for _, dnsName := range cert.DNSNames {
			if _, ok := allowedDNSNames[strings.ToLower(strings.TrimSpace(dnsName))]; ok {
				return nil
			}
		}
		for _, uri := range cert.URIs {
			if uri == nil {
				continue
			}
			if _, ok := allowedURIs[uri.String()]; ok {
				return nil
			}
		}
		return errors.New("delivery grpc client certificate identity is not allowed")
	}
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envIntAllowZero(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func envInt64AllowZero(name string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func envRequiredInt64AllowZero(name string) (int64, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return 0, errors.New(name + " is required")
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, errors.New(name + " must be a non-negative integer")
	}
	return parsed, nil
}

func envStringSet(name string, normalize func(string) string) map[string]struct{} {
	values := make(map[string]struct{})
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return values
	}
	for _, item := range strings.Split(raw, ",") {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if normalize != nil {
			value = normalize(value)
		}
		values[value] = struct{}{}
	}
	return values
}

func envURIStringSet(name string) (map[string]struct{}, error) {
	values := make(map[string]struct{})
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return values, nil
	}
	for _, item := range strings.Split(raw, ",") {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return nil, errors.New(name + " contains an invalid URI")
		}
		values[parsed.String()] = struct{}{}
	}
	return values, nil
}

func envOptionalBool(name string) (bool, bool, error) {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if value == "" {
		return false, false, nil
	}
	switch value {
	case "1", "true", "yes", "y", "on":
		return true, true, nil
	case "0", "false", "no", "n", "off":
		return false, true, nil
	default:
		return false, true, errors.New(name + " must be a boolean")
	}
}

func envBool(name string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envPositiveDuration(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return parsed, nil
}

func envPositiveInt(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func parseInt64CSV(value string) ([]int64, error) {
	parts := strings.Split(value, ",")
	result := make([]int64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parsed, err := strconv.ParseInt(part, 10, 64)
		if err != nil || parsed <= 0 {
			return nil, errors.New("NEXUSIM_DELIVERY_OUTBOX_REPAIR_IDS must contain positive integer ids")
		}
		result = append(result, parsed)
	}
	return result, nil
}
