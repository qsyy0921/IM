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

	contactsgrpc "github.com/qsyy0921/IM/services/contacts-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/contacts-service/internal/app"
	kafkainfra "github.com/qsyy0921/IM/services/contacts-service/internal/infrastructure/kafka"
	monitoringinfra "github.com/qsyy0921/IM/services/contacts-service/internal/infrastructure/monitoring"
	postgresinfra "github.com/qsyy0921/IM/services/contacts-service/internal/infrastructure/postgres"
	"github.com/qsyy0921/IM/services/contacts-service/internal/trigger/outbox"
	"google.golang.org/grpc"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run() error {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_SERVICE_MODE"))
	switch mode {
	case "", "noop":
		log.Println("contacts-service runtime wiring is idle; set NEXUSIM_CONTACTS_SERVICE_MODE=grpc, outbox-relay, outbox-audit, outbox-repair, outbox-repair-audit, outbox-repair-cleanup, tenant-privacy-default-audit, tenant-privacy-default-set, source-policy-audit, source-policy-set, contact-request-review, or contact-request-review-audit")
		return nil
	case "grpc":
		return runGRPC()
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
	case "tenant-privacy-default-audit":
		return runTenantPrivacyDefaultAudit()
	case "tenant-privacy-default-set":
		return runTenantPrivacyDefaultSet()
	case "source-policy-audit":
		return runSourcePolicyAudit()
	case "source-policy-set":
		return runSourcePolicySet()
	case "contact-request-review":
		return runContactRequestReview()
	case "contact-request-review-audit":
		return runContactRequestReviewAudit()
	default:
		return errors.New("unsupported NEXUSIM_CONTACTS_SERVICE_MODE")
	}
}

func runGRPC() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	addr := envString("NEXUSIM_CONTACTS_GRPC_ADDR", "0.0.0.0:10500")
	authMode := envString("NEXUSIM_CONTACTS_AUTH_MODE", "body")
	serverTLSConfig, serverTLSEnabled, err := contactsGRPCTLSConfigFromEnv()
	if err != nil {
		return err
	}
	if err := validateTrustedMetadataListenerConfig(addr, authMode, serverTLSConfig); err != nil {
		return err
	}

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	grpcMetrics := monitoringinfra.NewGRPCMetrics()
	traceConfig, err := contactsTraceConfigFromEnv()
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
			log.Printf("contacts-service OpenTelemetry trace shutdown failed: %v", err)
		}
	}()
	debugAddr, err := contactsDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr, monitoringinfra.NewHandler(pool, grpcMetrics).WithTraceStats(traceRuntime.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	repository := postgresinfra.NewRepository(pool)
	server, err := newGRPCServerWithConfig(grpcMetrics, authMode, serverTLSConfig, serverTLSEnabled, traceRuntime.UnaryServerInterceptor())
	if err != nil {
		return err
	}
	contactsgrpc.Register(server, contactsgrpc.NewServer(
		app.NewSendContactRequestUseCase(repository),
		app.NewGetContactPrivacyUseCase(repository),
		app.NewSetContactPrivacyUseCase(repository),
		app.NewSetContactPrivacyExceptionUseCase(repository),
		app.NewListContactPrivacyExceptionsUseCase(repository),
		app.NewDeleteContactPrivacyExceptionUseCase(repository),
		app.NewRespondContactRequestUseCase(repository),
		app.NewCancelContactRequestUseCase(repository),
		app.NewListContactRequestsUseCase(repository),
		app.NewListContactsUseCase(repository),
		app.NewGetContactStateUseCase(repository),
		app.NewDeleteContactUseCase(repository),
		app.NewBlockContactUseCase(repository),
		app.NewUnblockContactUseCase(repository),
		app.NewUpdateContactRemarkUseCase(repository),
		app.NewUpdateContactGroupUseCase(repository),
	))

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("contacts-service grpc listening on %s", addr)

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

func runOutboxRelay() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
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

	topic := envString("NEXUSIM_CONTACT_EVENTS_TOPIC", outbox.TopicContactEvents)
	relay := outbox.NewRelay(
		postgresinfra.NewOutboxStore(pool),
		producer,
		outbox.Config{
			Topic:          topic,
			BatchSize:      envInt("NEXUSIM_CONTACTS_OUTBOX_BATCH_SIZE", 500),
			PollInterval:   envDuration("NEXUSIM_CONTACTS_OUTBOX_POLL_INTERVAL", time.Second),
			MaxAttempts:    envInt("NEXUSIM_CONTACTS_OUTBOX_MAX_ATTEMPTS", 5),
			RetryBaseDelay: envDuration("NEXUSIM_CONTACTS_OUTBOX_RETRY_BASE_DELAY", time.Second),
			ErrorBackoff:   envDuration("NEXUSIM_CONTACTS_OUTBOX_RELAY_ERROR_BACKOFF", time.Second),
			Logf:           log.Printf,
		},
	)
	debugAddr, err := contactsDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr, monitoringinfra.NewHandler(pool).WithOutboxRelayStats(relay.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()
	log.Printf("contacts-service outbox relay started topic=%s", topic)
	return relay.Run(ctx)
}

func runOutboxRepair() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	eventIDs := splitCSV(os.Getenv("NEXUSIM_CONTACTS_OUTBOX_REPAIR_EVENT_IDS"))
	reason := envString("NEXUSIM_CONTACTS_OUTBOX_REPAIR_REASON", "manual contacts outbox repair")
	stats, err := postgresinfra.NewOutboxStore(pool).RepairDLQEvents(ctx, eventIDs, reason)
	if err != nil {
		return err
	}
	log.Printf(
		"contacts-service outbox repair completed requested=%d repaired=%d skipped=%d",
		stats.Requested,
		stats.Repaired,
		stats.Skipped,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_OUTBOX_REPAIR_OUTPUT")); outputPath != "" {
		if err := writeOutboxRepairOutput(outputPath, stats, len(eventIDs)); err != nil {
			return err
		}
	}
	return nil
}

func runOutboxAudit() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	var outboxID *int64
	outboxIDFilter := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_OUTBOX_AUDIT_OUTBOX_ID"))
	if outboxIDFilter != "" {
		parsed := envInt64AllowZero("NEXUSIM_CONTACTS_OUTBOX_AUDIT_OUTBOX_ID", 0)
		outboxID = &parsed
	}
	eventID := envString("NEXUSIM_CONTACTS_OUTBOX_AUDIT_EVENT_ID", "")
	tenantID := envString("NEXUSIM_CONTACTS_OUTBOX_AUDIT_TENANT_ID", "")
	aggregateID := envString("NEXUSIM_CONTACTS_OUTBOX_AUDIT_AGGREGATE_ID", "")
	status := envString("NEXUSIM_CONTACTS_OUTBOX_AUDIT_STATUS", "")
	eventType := envString("NEXUSIM_CONTACTS_OUTBOX_AUDIT_EVENT_TYPE", "")
	createdAfter, err := envOptionalRFC3339Time("NEXUSIM_CONTACTS_OUTBOX_AUDIT_CREATED_AFTER")
	if err != nil {
		return err
	}
	createdBefore, err := envOptionalRFC3339Time("NEXUSIM_CONTACTS_OUTBOX_AUDIT_CREATED_BEFORE")
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
		Limit:         envInt("NEXUSIM_CONTACTS_OUTBOX_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("contacts-service outbox audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"contacts_outbox id=%d event_id=%s tenant_id=%s aggregate_type=%s aggregate_id=%s aggregate_version=%d event_type=%s status=%s retry_count=%d available_at=%s next_retry_at=%s published_at=%s dead_lettered_at=%s last_error=%q",
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
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_OUTBOX_AUDIT_OUTPUT")); outputPath != "" {
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

func runOutboxRepairAudit() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	repairedAfter, err := envOptionalRFC3339Time("NEXUSIM_CONTACTS_OUTBOX_REPAIR_AUDIT_REPAIRED_AFTER")
	if err != nil {
		return err
	}
	repairedBefore, err := envOptionalRFC3339Time("NEXUSIM_CONTACTS_OUTBOX_REPAIR_AUDIT_REPAIRED_BEFORE")
	if err != nil {
		return err
	}
	filters := map[string]string{
		"event_id":        envString("NEXUSIM_CONTACTS_OUTBOX_REPAIR_AUDIT_EVENT_ID", ""),
		"tenant_id":       envString("NEXUSIM_CONTACTS_OUTBOX_REPAIR_AUDIT_TENANT_ID", ""),
		"repaired_after":  formatOptionalTime(repairedAfter),
		"repaired_before": formatOptionalTime(repairedBefore),
	}
	rows, err := postgresinfra.NewOutboxStore(pool).AuditOutboxRepairs(ctx, postgresinfra.OutboxRepairAuditOptions{
		EventID:        filters["event_id"],
		TenantID:       filters["tenant_id"],
		RepairedAfter:  repairedAfter,
		RepairedBefore: repairedBefore,
		Limit:          envInt("NEXUSIM_CONTACTS_OUTBOX_REPAIR_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("contacts-service outbox repair audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"contacts_outbox_repair event_id=%s tenant_id=%s previous_status=%s previous_retry_count=%d previous_dead_lettered_at=%s repaired_at=%s reason=%q previous_last_error=%q",
			row.EventID,
			row.TenantID,
			row.PreviousStatus,
			row.PreviousRetryCount,
			formatOptionalTime(row.PreviousDeadLetteredAt),
			row.RepairedAt.Format(time.RFC3339),
			row.Reason,
			row.PreviousLastError,
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_OUTBOX_REPAIR_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeOutboxRepairAuditOutput(outputPath, rows, filters); err != nil {
			return err
		}
	}
	return nil
}
