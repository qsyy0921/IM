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

func run() error {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_SERVICE_MODE"))
	switch mode {
	case "", "noop":
		log.Println("delivery-service runtime wiring is idle; set NEXUSIM_DELIVERY_SERVICE_MODE=grpc")
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
	grpcMetrics := monitoringinfra.NewGRPCMetrics()
	stopDebug, err := startDebugServer(ctx, deliveryDebugAddr(), monitoringinfra.NewHandler(pool, grpcMetrics))
	if err != nil {
		return err
	}
	defer stopDebug()
	server, err := newGRPCServer(grpcMetrics)
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
	stopDebug, err := startDebugServer(ctx, deliveryDebugAddr(), monitoringinfra.NewHandler(pool))
	if err != nil {
		return err
	}
	defer stopDebug()

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
	)
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
	stopDebug, err := startDebugServer(ctx, deliveryDebugAddr(), monitoringinfra.NewHandler(pool))
	if err != nil {
		return err
	}
	defer stopDebug()

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
		},
	)
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
	if value := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_OUTBOX_AUDIT_OUTBOX_ID")); value != "" {
		parsed := envInt64AllowZero("NEXUSIM_DELIVERY_OUTBOX_AUDIT_OUTBOX_ID", 0)
		outboxID = &parsed
	}
	rows, err := postgresinfra.NewOutboxStore(pool).AuditOutbox(ctx, postgresinfra.OutboxAuditOptions{
		OutboxID:       outboxID,
		EventID:        envString("NEXUSIM_DELIVERY_OUTBOX_AUDIT_EVENT_ID", ""),
		TenantID:       envString("NEXUSIM_DELIVERY_OUTBOX_AUDIT_TENANT_ID", ""),
		ConversationID: envString("NEXUSIM_DELIVERY_OUTBOX_AUDIT_CONVERSATION_ID", ""),
		Status:         envString("NEXUSIM_DELIVERY_OUTBOX_AUDIT_STATUS", ""),
		EventType:      envString("NEXUSIM_DELIVERY_OUTBOX_AUDIT_EVENT_TYPE", ""),
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
	rows, err := postgresinfra.NewOutboxStore(pool).AuditOutboxRepairs(ctx, postgresinfra.OutboxRepairAuditOptions{
		OutboxID:       outboxID,
		EventID:        envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_AUDIT_EVENT_ID", ""),
		TenantID:       envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_AUDIT_TENANT_ID", ""),
		ConversationID: envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_AUDIT_CONVERSATION_ID", ""),
		Mode:           envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_AUDIT_MODE", ""),
		Outcome:        envString("NEXUSIM_DELIVERY_OUTBOX_REPAIR_AUDIT_OUTCOME", ""),
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
	stats, err := postgresinfra.NewOutboxStore(pool).CleanupOutboxRepairs(ctx, postgresinfra.OutboxRepairCleanupOptions{
		OutboxID:       outboxRepairCleanupOutboxID(),
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
	includeResolved := envBool("NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_INCLUDE_RESOLVED", false)
	rows, err := postgresinfra.NewProjectionFailureStore(pool).AuditFailures(ctx, postgresinfra.ProjectionFailureAuditOptions{
		ConsumerGroup:  envString("NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_CONSUMER_GROUP", ""),
		Topic:          envString("NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_TOPIC", "conversation.timeline.events"),
		PartitionID:    partitionID,
		OffsetValue:    offsetValue,
		EventID:        envString("NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_EVENT_ID", ""),
		EventType:      envString("NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_EVENT_TYPE", ""),
		FailureClass:   envString("NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_FAILURE_CLASS", ""),
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
	if !includeResolved && len(rows) > 0 {
		return fmt.Errorf("delivery projection failure audit found %d unresolved rows", len(rows))
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
	stats, err := postgresinfra.NewProjectionFailureStore(pool).CleanupResolvedFailures(ctx, postgresinfra.ProjectionFailureCleanupOptions{
		ConsumerGroup: envString("NEXUSIM_DELIVERY_PROJECTION_FAILURE_CLEANUP_CONSUMER_GROUP", ""),
		Topic:         envString("NEXUSIM_DELIVERY_PROJECTION_FAILURE_CLEANUP_TOPIC", "conversation.timeline.events"),
		PartitionID:   projectionFailureCleanupPartitionID(),
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
	stats, err := postgresinfra.NewProjectionRepairStore(pool).CleanupCheckpointRepairs(ctx, postgresinfra.ProjectionRepairCleanupOptions{
		ConsumerGroup: envString("NEXUSIM_DELIVERY_PROJECTION_REPAIR_CLEANUP_CONSUMER_GROUP", ""),
		Topic:         envString("NEXUSIM_DELIVERY_PROJECTION_REPAIR_CLEANUP_TOPIC", "conversation.timeline.events"),
		PartitionID:   projectionCheckpointRepairCleanupPartitionID(),
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
	return value.Format(time.RFC3339)
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
	interceptors := make([]grpc.UnaryServerInterceptor, 0, 2)
	if grpcMetrics != nil {
		interceptors = append(interceptors, grpcMetrics.UnaryServerInterceptor(log.Default()))
	} else {
		interceptors = append(interceptors, monitoringinfra.UnaryAccessLogInterceptor(log.Default()))
	}
	switch strings.ToLower(envString("NEXUSIM_DELIVERY_AUTH_MODE", "body")) {
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
	if creds, ok, err := loadDeliveryGRPCCredentialsFromEnv(); err != nil {
		return nil, err
	} else if ok {
		serverOptions = append(serverOptions, grpc.Creds(creds))
	}
	return grpc.NewServer(serverOptions...), nil
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
