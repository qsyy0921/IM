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

	grpcapi "github.com/qsyy0921/IM/services/message-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/message-service/internal/app"
	admissioninfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/admission"
	kafkainfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/kafka"
	metricsinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/metrics"
	monitoringinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/monitoring"
	postgresinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/postgres"
	rpcinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/rpc"
	"github.com/qsyy0921/IM/services/message-service/internal/trigger/outbox"
	"google.golang.org/grpc"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run() error {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_SERVICE_MODE"))
	switch mode {
	case "", "noop":
		log.Println("message-service runtime wiring is idle; set NEXUSIM_MESSAGE_SERVICE_MODE=grpc, outbox-relay, outbox-audit, outbox-repair, outbox-repair-audit, outbox-repair-cleanup, change-history-audit, retention-proof-audit, legal-hold-audit, legal-hold-set, legal-hold-release, compliance-proof-audit, compliance-proof-register, compliance-proof-revoke, compliance-approval-audit, compliance-approval-create, or compliance-approval-cancel")
		return nil
	case "grpc":
		return runGRPCServer()
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
	case "change-history-audit":
		return runMessageChangeHistoryAudit()
	case "retention-proof-audit":
		return runMessageRetentionProofAudit()
	case "legal-hold-audit":
		return runMessageLegalHoldAudit()
	case "legal-hold-set":
		return runMessageLegalHoldSet()
	case "legal-hold-release":
		return runMessageLegalHoldRelease()
	case "compliance-proof-audit":
		return runMessageComplianceProofAudit()
	case "compliance-proof-register":
		return runMessageComplianceProofRegister()
	case "compliance-proof-revoke":
		return runMessageComplianceProofRevoke()
	case "compliance-approval-audit":
		return runMessageComplianceApprovalAudit()
	case "compliance-approval-create":
		return runMessageComplianceApprovalCreate()
	case "compliance-approval-cancel":
		return runMessageComplianceApprovalCancel()
	default:
		return errors.New("unsupported NEXUSIM_MESSAGE_SERVICE_MODE")
	}
}

func runGRPCServer() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required")
	}
	listenAddr := envString("NEXUSIM_GRPC_ADDR", "0.0.0.0:10495")
	authMode := envString("NEXUSIM_MESSAGE_AUTH_MODE", "body")
	serverTLSConfig, serverTLSEnabled, err := messageGRPCTLSConfigFromEnv()
	if err != nil {
		return err
	}
	if err := validateTrustedMetadataListenerConfig(listenAddr, authMode, serverTLSConfig); err != nil {
		return err
	}

	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	metrics := metricsinfra.NewCollector()
	traceConfig, err := messageTraceConfigFromEnv()
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
			log.Printf("message-service OpenTelemetry trace shutdown failed: %v", err)
		}
	}()
	debugAddr, err := messageDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr, metricsinfra.NewHandler(metrics, pool).WithTraceStats(traceRuntime.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()

	staticPolicy := rpcinfra.NewStaticPolicy()
	staticPolicy.Allowed = envBool("NEXUSIM_MOCK_POLICY_ALLOWED", staticPolicy.Allowed)
	staticPolicy.PermissionVersion = envInt64("NEXUSIM_MOCK_PERMISSION_VERSION", staticPolicy.PermissionVersion)
	staticPolicy.Classification = envString("NEXUSIM_MOCK_CLASSIFICATION", staticPolicy.Classification)

	var policy app.PolicyCheckPort = staticPolicy
	if policyAddr := envString("NEXUSIM_POLICY_SERVICE_ADDR", ""); policyAddr != "" {
		policyTLS, err := policyClientTLSConfigFromEnv()
		if err != nil {
			return err
		}
		client, closeClient, err := rpcinfra.DialPolicyClientWithConfig(ctx, rpcinfra.PolicyClientDialConfig{
			Addr:    policyAddr,
			Timeout: envDuration("NEXUSIM_POLICY_RPC_TIMEOUT", 30*time.Millisecond),
			TLS:     policyTLS,
		})
		if err != nil {
			return err
		}
		defer func() {
			if err := closeClient(); err != nil {
				log.Printf("close policy-service client: %v", err)
			}
		}()
		policy = client
		log.Printf("message-service using policy-service at %s", policyAddr)
	}

	var conversation app.ConversationQueryPort
	if conversationAddr := envString("NEXUSIM_CONVERSATION_SERVICE_ADDR", ""); conversationAddr != "" {
		conversationTLS, err := conversationClientTLSConfigFromEnv()
		if err != nil {
			return err
		}
		client, closeClient, err := rpcinfra.DialConversationClientWithConfig(ctx, rpcinfra.ConversationClientDialConfig{
			Addr:    conversationAddr,
			Timeout: envDuration("NEXUSIM_CONVERSATION_RPC_TIMEOUT", 30*time.Millisecond),
			TLS:     conversationTLS,
		})
		if err != nil {
			return err
		}
		defer func() {
			if err := closeClient(); err != nil {
				log.Printf("close conversation-service client: %v", err)
			}
		}()
		conversation = client
		log.Printf("message-service using conversation-service at %s", conversationAddr)
	} else {
		staticConversation := rpcinfra.NewStaticConversation()
		staticConversation.MemberVersion = envInt64("NEXUSIM_MOCK_MEMBER_VERSION", staticConversation.MemberVersion)
		staticConversation.PermissionVersion = staticPolicy.PermissionVersion
		staticConversation.FanoutPolicyVersion = envInt64("NEXUSIM_MOCK_FANOUT_POLICY_VERSION", staticConversation.FanoutPolicyVersion)
		conversation = staticConversation
	}

	repositoryOptions := []postgresinfra.MessageRepositoryOption{postgresinfra.WithMetrics(metrics)}
	if envBool("NEXUSIM_PG_BACKPRESSURE_ENABLED", false) {
		repositoryOptions = append(repositoryOptions, postgresinfra.WithBackpressure(postgresinfra.BackpressureConfig{
			Enabled:           true,
			MinAvailableConns: int32(envInt("NEXUSIM_PG_BACKPRESSURE_MIN_AVAILABLE_CONNS", 0)),
		}))
	}

	useCaseOptions := make([]app.SendMessageUseCaseOption, 0, 1)
	if envBool("NEXUSIM_ADAPTIVE_LIMIT_ENABLED", false) {
		config := admissioninfra.Config{
			Enabled:                       true,
			MinAvailableConns:             int32(envInt("NEXUSIM_ADAPTIVE_MIN_AVAILABLE_CONNS", 0)),
			ReleaseAvailableConns:         int32(envInt("NEXUSIM_ADAPTIVE_RELEASE_AVAILABLE_CONNS", 0)),
			MaxPoolAcquireP95:             envDuration("NEXUSIM_ADAPTIVE_MAX_POOL_ACQUIRE_P95", 0),
			MaxInFlight:                   envInt64("NEXUSIM_ADAPTIVE_MAX_IN_FLIGHT", 0),
			MaxOutboxPending:              envInt64("NEXUSIM_ADAPTIVE_MAX_OUTBOX_PENDING", 0),
			ReleaseOutboxPending:          envInt64("NEXUSIM_ADAPTIVE_RELEASE_OUTBOX_PENDING", 0),
			MaxRelayProcessReadyActiveP95: envDuration("NEXUSIM_ADAPTIVE_MAX_RELAY_ACTIVE_P95", 0),
			MinOutboxFetchedPerCall:       envFloat("NEXUSIM_ADAPTIVE_MIN_OUTBOX_FETCHED_PER_CALL", 0),
			MinKafkaPublishRecordsPerCall: envFloat("NEXUSIM_ADAPTIVE_MIN_KAFKA_RECORDS_PER_CALL", 0),
			MinMetricSamples:              int64(envInt("NEXUSIM_ADAPTIVE_MIN_METRIC_SAMPLES", 20)),
			SampleInterval:                envDuration("NEXUSIM_ADAPTIVE_SAMPLE_INTERVAL", time.Second),
			RelayMetricsURL:               envString("NEXUSIM_ADAPTIVE_RELAY_METRICS_URL", ""),
			HTTPTimeout:                   envDuration("NEXUSIM_ADAPTIVE_HTTP_TIMEOUT", time.Second),
			RetryBaseDelay:                envDuration("NEXUSIM_ADAPTIVE_RETRY_BASE_DELAY", 500*time.Millisecond),
			RetryMaxDelay:                 envDuration("NEXUSIM_ADAPTIVE_RETRY_MAX_DELAY", 2*time.Second),
		}
		admission := admissioninfra.NewController(
			config,
			admissioninfra.NewPGXPoolStatsProvider(pool),
			metrics,
			admissioninfra.NewPostgresOutboxBacklogProvider(pool),
		)
		admission.Start(ctx)
		useCaseOptions = append(useCaseOptions, app.WithAdmission(admission))
		log.Printf(
			"message-service adaptive limit enabled min_available=%d max_pool_acquire_p95=%s max_in_flight=%d max_outbox_pending=%d relay_metrics_url=%s",
			config.MinAvailableConns,
			config.MaxPoolAcquireP95,
			config.MaxInFlight,
			config.MaxOutboxPending,
			config.RelayMetricsURL,
		)
	}

	messageRepository := postgresinfra.NewMessageRepository(pool, repositoryOptions...)
	sendUseCase := app.NewSendMessageUseCase(
		policy,
		conversation,
		rpcinfra.NoopSequencer{},
		messageRepository,
		useCaseOptions...,
	)
	editUseCase := app.NewEditMessageUseCase(policy, conversation, messageRepository)
	revokeUseCase := app.NewRevokeMessageUseCase(policy, conversation, messageRepository)
	deleteUseCase := app.NewDeleteMessageUseCase(policy, conversation, messageRepository)

	server, err := newGRPCServerWithConfig(authMode, serverTLSConfig, serverTLSEnabled, traceRuntime.UnaryServerInterceptor())
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	grpcapi.Register(server, grpcapi.NewServer(
		sendUseCase,
		grpcapi.WithMetrics(metrics),
		grpcapi.WithEditMessage(editUseCase),
		grpcapi.WithRevokeMessage(revokeUseCase),
		grpcapi.WithDeleteMessage(deleteUseCase),
	))

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("message-service gRPC server started on %s", listenAddr)

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

	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required")
	}
	brokers := splitCSV(os.Getenv("NEXUSIM_KAFKA_BROKERS"))
	if len(brokers) == 0 {
		return errors.New("NEXUSIM_KAFKA_BROKERS is required")
	}

	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	metrics := metricsinfra.NewCollector()

	producer, err := kafkainfra.NewWriterProducer(brokers)
	if err != nil {
		return err
	}
	defer func() {
		if err := producer.Close(); err != nil {
			log.Printf("close kafka producer: %v", err)
		}
	}()

	pollInterval := envDuration("NEXUSIM_OUTBOX_POLL_INTERVAL", time.Second)
	publishBatchEnabled := envBool("NEXUSIM_OUTBOX_PUBLISH_BATCH_ENABLED", true)
	relay := outbox.NewRelay(
		postgresinfra.NewOutboxStore(pool, postgresinfra.WithOutboxMetrics(metrics)),
		producer,
		outbox.Config{
			Topic:               envString("NEXUSIM_KAFKA_TOPIC", outbox.TopicConversationTimelineEvents),
			BatchSize:           envInt("NEXUSIM_OUTBOX_BATCH_SIZE", 500),
			WorkerCount:         envInt("NEXUSIM_OUTBOX_WORKERS", 1),
			DisablePublishBatch: !publishBatchEnabled,
			PollInterval:        pollInterval,
			FailureBackoff:      envDuration("NEXUSIM_OUTBOX_FAILURE_BACKOFF", pollInterval),
			ErrorBackoff:        envDuration("NEXUSIM_OUTBOX_RELAY_ERROR_BACKOFF", time.Second),
			MaxAttempts:         envInt("NEXUSIM_OUTBOX_MAX_ATTEMPTS", 5),
			RetryBaseDelay:      envDuration("NEXUSIM_OUTBOX_RETRY_BASE_DELAY", time.Second),
			Metrics:             metrics,
			Logf:                log.Printf,
		},
	)
	debugAddr, err := messageDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr, metricsinfra.NewHandler(metrics, pool).WithOutboxRelayStats(relay.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()
	log.Printf(
		"message-service outbox relay started workers=%d batch_size=%d publish_batch_enabled=%t poll_interval=%s failure_backoff=%s",
		envInt("NEXUSIM_OUTBOX_WORKERS", 1),
		envInt("NEXUSIM_OUTBOX_BATCH_SIZE", 500),
		publishBatchEnabled,
		pollInterval,
		envDuration("NEXUSIM_OUTBOX_FAILURE_BACKOFF", pollInterval),
	)
	return relay.Run(ctx)
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
	outboxIDFilter := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_OUTBOX_AUDIT_OUTBOX_ID"))
	if outboxIDFilter != "" {
		parsed := envInt64AllowZero("NEXUSIM_MESSAGE_OUTBOX_AUDIT_OUTBOX_ID", 0)
		outboxID = &parsed
	}
	eventID := envString("NEXUSIM_MESSAGE_OUTBOX_AUDIT_EVENT_ID", "")
	tenantID := envString("NEXUSIM_MESSAGE_OUTBOX_AUDIT_TENANT_ID", "")
	conversationID := envString("NEXUSIM_MESSAGE_OUTBOX_AUDIT_CONVERSATION_ID", "")
	status := envString("NEXUSIM_MESSAGE_OUTBOX_AUDIT_STATUS", "")
	eventType := envString("NEXUSIM_MESSAGE_OUTBOX_AUDIT_EVENT_TYPE", "")
	createdAfter, err := envOptionalRFC3339Time("NEXUSIM_MESSAGE_OUTBOX_AUDIT_CREATED_AFTER")
	if err != nil {
		return err
	}
	createdBefore, err := envOptionalRFC3339Time("NEXUSIM_MESSAGE_OUTBOX_AUDIT_CREATED_BEFORE")
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
		Limit:          envInt("NEXUSIM_MESSAGE_OUTBOX_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("message-service outbox audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"message_outbox id=%d event_id=%s tenant_id=%s conversation_id=%s aggregate_version=%d event_type=%s status=%s retry_count=%d available_at=%s published_at=%s dead_lettered_at=%s last_error=%q",
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
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_OUTBOX_AUDIT_OUTPUT")); outputPath != "" {
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

	eventIDs := splitCSV(os.Getenv("NEXUSIM_MESSAGE_OUTBOX_REPAIR_EVENT_IDS"))
	reason := envString("NEXUSIM_MESSAGE_OUTBOX_REPAIR_REASON", "manual message outbox repair")
	stats, err := postgresinfra.NewOutboxStore(pool).RepairDLQEvents(ctx, eventIDs, reason)
	if err != nil {
		return err
	}
	log.Printf(
		"message-service outbox repair completed requested=%d repaired=%d skipped=%d",
		stats.Requested,
		stats.Repaired,
		stats.Skipped,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_OUTBOX_REPAIR_OUTPUT")); outputPath != "" {
		if err := writeOutboxRepairOutput(outputPath, stats, len(eventIDs)); err != nil {
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

	repairedAfter, err := envOptionalRFC3339Time("NEXUSIM_MESSAGE_OUTBOX_REPAIR_AUDIT_REPAIRED_AFTER")
	if err != nil {
		return err
	}
	repairedBefore, err := envOptionalRFC3339Time("NEXUSIM_MESSAGE_OUTBOX_REPAIR_AUDIT_REPAIRED_BEFORE")
	if err != nil {
		return err
	}
	filters := map[string]string{
		"event_id":        envString("NEXUSIM_MESSAGE_OUTBOX_REPAIR_AUDIT_EVENT_ID", ""),
		"tenant_id":       envString("NEXUSIM_MESSAGE_OUTBOX_REPAIR_AUDIT_TENANT_ID", ""),
		"conversation_id": envString("NEXUSIM_MESSAGE_OUTBOX_REPAIR_AUDIT_CONVERSATION_ID", ""),
		"repaired_after":  formatOptionalFilterTime(repairedAfter),
		"repaired_before": formatOptionalFilterTime(repairedBefore),
	}
	rows, err := postgresinfra.NewOutboxStore(pool).AuditOutboxRepairs(ctx, postgresinfra.OutboxRepairAuditOptions{
		EventID:        filters["event_id"],
		TenantID:       filters["tenant_id"],
		ConversationID: filters["conversation_id"],
		RepairedAfter:  repairedAfter,
		RepairedBefore: repairedBefore,
		Limit:          envInt("NEXUSIM_MESSAGE_OUTBOX_REPAIR_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("message-service outbox repair audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"message_outbox_repair event_id=%s tenant_id=%s conversation_id=%s previous_status=%s previous_retry_count=%d previous_dead_lettered_at=%s repaired_at=%s reason=%q previous_last_error=%q",
			row.EventID,
			row.TenantID,
			row.ConversationID,
			row.PreviousStatus,
			row.PreviousRetryCount,
			formatOptionalTime(row.PreviousDeadLetteredAt),
			row.RepairedAt.Format(time.RFC3339),
			row.Reason,
			row.PreviousLastError,
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_OUTBOX_REPAIR_AUDIT_OUTPUT")); outputPath != "" {
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
	stats, err := postgresinfra.NewOutboxStore(pool).CleanupOutboxRepairs(ctx, postgresinfra.OutboxRepairCleanupOptions{
		EventID:        envString("NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_EVENT_ID", ""),
		TenantID:       envString("NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_TENANT_ID", ""),
		ConversationID: envString("NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_CONVERSATION_ID", ""),
		Cutoff:         cutoff,
		Limit:          config.BatchSize,
		DryRun:         config.DryRun,
	})
	if err != nil {
		return err
	}
	log.Printf(
		"message-service outbox repair cleanup completed deleted=%d cutoff=%s retention=%s batch_size=%d dry_run=%t",
		stats.Deleted,
		cutoff.Format(time.RFC3339),
		config.Retention,
		config.BatchSize,
		config.DryRun,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_OUTPUT")); outputPath != "" {
		if err := writeOutboxRepairCleanupOutput(outputPath, stats, cutoff, config.Retention, config.BatchSize, config.DryRun, map[string]string{
			"event_id":        envString("NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_EVENT_ID", ""),
			"tenant_id":       envString("NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_TENANT_ID", ""),
			"conversation_id": envString("NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_CONVERSATION_ID", ""),
		}); err != nil {
			return err
		}
	}
	return nil
}

func runMessageChangeHistoryAudit() error {
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

	rows, err := postgresinfra.NewMessageRepository(pool).AuditMessageChangeHistory(ctx, postgresinfra.MessageChangeHistoryAuditOptions{
		TenantID:       envString("NEXUSIM_MESSAGE_CHANGE_HISTORY_AUDIT_TENANT_ID", ""),
		ConversationID: envString("NEXUSIM_MESSAGE_CHANGE_HISTORY_AUDIT_CONVERSATION_ID", ""),
		MessageID:      envString("NEXUSIM_MESSAGE_CHANGE_HISTORY_AUDIT_MESSAGE_ID", ""),
		ChangeType:     envString("NEXUSIM_MESSAGE_CHANGE_HISTORY_AUDIT_CHANGE_TYPE", ""),
		ChangedBy:      envString("NEXUSIM_MESSAGE_CHANGE_HISTORY_AUDIT_CHANGED_BY", ""),
		Limit:          envInt("NEXUSIM_MESSAGE_CHANGE_HISTORY_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("message-service change history audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"message_change_history tenant_id=%s conversation_id=%s message_id=%s change_version=%d change_type=%s before_status=%s after_status=%s changed_by=%s before_payload_present=%t after_payload_present=%t reason_present=%t trace_id=%s changed_at=%s",
			row.TenantID,
			row.ConversationID,
			row.MessageID,
			row.ChangeVersion,
			row.ChangeType,
			row.BeforeStatus,
			row.AfterStatus,
			row.ChangedBy,
			row.BeforePayloadPresent,
			row.AfterPayloadPresent,
			row.ReasonPresent,
			row.TraceID,
			row.ChangedAt.Format(time.RFC3339),
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_CHANGE_HISTORY_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeMessageChangeHistoryAuditOutput(outputPath, rows); err != nil {
			return err
		}
	}
	return nil
}

func runMessageRetentionProofAudit() error {
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

	rows, err := postgresinfra.NewMessageRepository(pool).AuditMessageRetentionProof(ctx, postgresinfra.MessageRetentionProofAuditOptions{
		TenantID:       envString("NEXUSIM_MESSAGE_RETENTION_PROOF_AUDIT_TENANT_ID", ""),
		ConversationID: envString("NEXUSIM_MESSAGE_RETENTION_PROOF_AUDIT_CONVERSATION_ID", ""),
		MessageID:      envString("NEXUSIM_MESSAGE_RETENTION_PROOF_AUDIT_MESSAGE_ID", ""),
		Status:         envString("NEXUSIM_MESSAGE_RETENTION_PROOF_AUDIT_STATUS", "DELETED"),
		Limit:          envInt("NEXUSIM_MESSAGE_RETENTION_PROOF_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("message-service retention proof audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"message_retention_proof tenant_id=%s conversation_id=%s message_id=%s conversation_seq=%d message_type=%s status=%s current_payload_present=%t deleted_at=%s delete_change_version=%s delete_changed_by=%s delete_reason_present=%t delete_timeline_event_present=%t delete_outbox_event_present=%t",
			row.TenantID,
			row.ConversationID,
			row.MessageID,
			row.ConversationSeq,
			row.MessageType,
			row.Status,
			row.CurrentPayloadPresent,
			formatOptionalTime(row.DeletedAt),
			formatOptionalInt(row.DeleteChangeVersion),
			row.DeleteChangedBy,
			row.DeleteReasonPresent,
			row.DeleteTimelineEventPresent,
			row.DeleteOutboxEventPresent,
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_RETENTION_PROOF_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeMessageRetentionProofAuditOutput(outputPath, rows); err != nil {
			return err
		}
	}
	return nil
}
