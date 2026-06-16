package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
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
	"google.golang.org/grpc/credentials"
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
		log.Println("message-service runtime wiring is idle; set NEXUSIM_MESSAGE_SERVICE_MODE=grpc, outbox-relay, outbox-audit, outbox-repair, outbox-repair-audit, outbox-repair-cleanup, change-history-audit, retention-proof-audit, legal-hold-audit, legal-hold-set, or legal-hold-release")
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
	if value := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_OUTBOX_AUDIT_OUTBOX_ID")); value != "" {
		parsed := envInt64AllowZero("NEXUSIM_MESSAGE_OUTBOX_AUDIT_OUTBOX_ID", 0)
		outboxID = &parsed
	}
	rows, err := postgresinfra.NewOutboxStore(pool).AuditOutbox(ctx, postgresinfra.OutboxAuditOptions{
		OutboxID:       outboxID,
		EventID:        envString("NEXUSIM_MESSAGE_OUTBOX_AUDIT_EVENT_ID", ""),
		TenantID:       envString("NEXUSIM_MESSAGE_OUTBOX_AUDIT_TENANT_ID", ""),
		ConversationID: envString("NEXUSIM_MESSAGE_OUTBOX_AUDIT_CONVERSATION_ID", ""),
		Status:         envString("NEXUSIM_MESSAGE_OUTBOX_AUDIT_STATUS", ""),
		EventType:      envString("NEXUSIM_MESSAGE_OUTBOX_AUDIT_EVENT_TYPE", ""),
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
		if err := writeOutboxAuditOutput(outputPath, rows); err != nil {
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

	rows, err := postgresinfra.NewOutboxStore(pool).AuditOutboxRepairs(ctx, postgresinfra.OutboxRepairAuditOptions{
		EventID:        envString("NEXUSIM_MESSAGE_OUTBOX_REPAIR_AUDIT_EVENT_ID", ""),
		TenantID:       envString("NEXUSIM_MESSAGE_OUTBOX_REPAIR_AUDIT_TENANT_ID", ""),
		ConversationID: envString("NEXUSIM_MESSAGE_OUTBOX_REPAIR_AUDIT_CONVERSATION_ID", ""),
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
		if err := writeOutboxRepairAuditOutput(outputPath, rows); err != nil {
			return err
		}
	}
	return nil
}

type outboxRepairCleanupConfig struct {
	Retention time.Duration
	BatchSize int
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
	})
	if err != nil {
		return err
	}
	log.Printf(
		"message-service outbox repair cleanup completed deleted=%d cutoff=%s retention=%s batch_size=%d",
		stats.Deleted,
		cutoff.Format(time.RFC3339),
		config.Retention,
		config.BatchSize,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_OUTPUT")); outputPath != "" {
		if err := writeOutboxRepairCleanupOutput(outputPath, stats, cutoff, config.Retention, config.BatchSize, map[string]string{
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
			log.Printf("message-service debug server stopped with error: %v", err)
		}
	}()
	log.Printf("message-service debug server started on %s", addr)
	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		<-done
	}, nil
}

func messageDebugAddr() string {
	return envString("NEXUSIM_MESSAGE_DEBUG_ADDR", envString("NEXUSIM_DEBUG_ADDR", ""))
}

func messageDebugAddrFromEnv() (string, error) {
	addr := messageDebugAddr()
	allowPublic, _, err := envOptionalBool("NEXUSIM_MESSAGE_DEBUG_ALLOW_PUBLIC")
	if err != nil {
		return "", err
	}
	return addr, validateMessageDebugListenerConfig(addr, allowPublic)
}

func validateMessageDebugListenerConfig(addr string, allowPublic bool) error {
	if strings.TrimSpace(addr) == "" {
		return nil
	}
	if listenerAddrTrustedWithoutMTLS(addr) {
		return nil
	}
	if allowPublic {
		return nil
	}
	return errors.New("message-service debug listener address is non-private; set NEXUSIM_MESSAGE_DEBUG_ALLOW_PUBLIC=true to allow")
}

func openPGPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if maxConns := envInt("NEXUSIM_PG_MAX_CONNS", 0); maxConns > 0 {
		config.MaxConns = int32(maxConns)
	}
	if minConns := envInt("NEXUSIM_PG_MIN_CONNS", 0); minConns > 0 {
		config.MinConns = int32(minConns)
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

func policyClientTLSConfigFromEnv() (rpcinfra.PolicyClientTLSConfig, error) {
	config := rpcinfra.PolicyClientTLSConfig{
		CAFile:         envString("NEXUSIM_POLICY_SERVICE_TLS_CA_FILE", ""),
		ServerName:     envString("NEXUSIM_POLICY_SERVICE_TLS_SERVER_NAME", ""),
		ClientCertFile: envString("NEXUSIM_POLICY_SERVICE_TLS_CLIENT_CERT_FILE", ""),
		ClientKeyFile:  envString("NEXUSIM_POLICY_SERVICE_TLS_CLIENT_KEY_FILE", ""),
	}
	if !config.Enabled() {
		return config, nil
	}
	if config.CAFile == "" {
		return config, errors.New("NEXUSIM_POLICY_SERVICE_TLS_CA_FILE is required when policy-service TLS is configured")
	}
	if (config.ClientCertFile == "") != (config.ClientKeyFile == "") {
		return config, errors.New("NEXUSIM_POLICY_SERVICE_TLS_CLIENT_CERT_FILE and NEXUSIM_POLICY_SERVICE_TLS_CLIENT_KEY_FILE must be configured together")
	}
	return config, nil
}

func conversationClientTLSConfigFromEnv() (rpcinfra.ConversationClientTLSConfig, error) {
	config := rpcinfra.ConversationClientTLSConfig{
		CAFile:         envString("NEXUSIM_CONVERSATION_SERVICE_TLS_CA_FILE", ""),
		ServerName:     envString("NEXUSIM_CONVERSATION_SERVICE_TLS_SERVER_NAME", ""),
		ClientCertFile: envString("NEXUSIM_CONVERSATION_SERVICE_TLS_CLIENT_CERT_FILE", ""),
		ClientKeyFile:  envString("NEXUSIM_CONVERSATION_SERVICE_TLS_CLIENT_KEY_FILE", ""),
	}
	if !config.Enabled() {
		return config, nil
	}
	if config.CAFile == "" {
		return config, errors.New("NEXUSIM_CONVERSATION_SERVICE_TLS_CA_FILE is required when conversation-service TLS is configured")
	}
	if (config.ClientCertFile == "") != (config.ClientKeyFile == "") {
		return config, errors.New("NEXUSIM_CONVERSATION_SERVICE_TLS_CLIENT_CERT_FILE and NEXUSIM_CONVERSATION_SERVICE_TLS_CLIENT_KEY_FILE must be configured together")
	}
	return config, nil
}

func newGRPCServer() (*grpc.Server, error) {
	authMode := envString("NEXUSIM_MESSAGE_AUTH_MODE", "body")
	tlsConfig, ok, err := messageGRPCTLSConfigFromEnv()
	if err != nil {
		return nil, err
	}
	return newGRPCServerWithConfig(authMode, tlsConfig, ok)
}

func newGRPCServerWithConfig(authMode string, tlsConfig *tls.Config, tlsEnabled bool, traceInterceptors ...grpc.UnaryServerInterceptor) (*grpc.Server, error) {
	interceptors := make([]grpc.UnaryServerInterceptor, 0, 3)
	interceptors = append(interceptors, monitoringinfra.UnaryAccessLogInterceptor(log.Default()))
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
		return nil, errors.New("unsupported NEXUSIM_MESSAGE_AUTH_MODE")
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

func messageTraceConfigFromEnv() (monitoringinfra.TraceConfig, error) {
	enabled, _, err := envOptionalBool("NEXUSIM_MESSAGE_OTEL_TRACES_ENABLED")
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	otlpInsecure, _, err := envOptionalBool("NEXUSIM_MESSAGE_OTEL_TRACES_OTLP_INSECURE")
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	samplingRatio, err := messageTraceSamplingRatioFromEnv()
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	return monitoringinfra.TraceConfig{
		Enabled:       enabled,
		ServiceName:   envString("NEXUSIM_MESSAGE_OTEL_SERVICE_NAME", "message-service"),
		Exporter:      envString("NEXUSIM_MESSAGE_OTEL_TRACES_EXPORTER", "stdout"),
		OTLPEndpoint:  envString("NEXUSIM_MESSAGE_OTEL_TRACES_OTLP_ENDPOINT", ""),
		OTLPInsecure:  otlpInsecure,
		SamplingRatio: samplingRatio,
	}, nil
}

func messageTraceSamplingRatioFromEnv() (float64, error) {
	raw := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_OTEL_TRACES_SAMPLING_RATIO"))
	if raw == "" {
		return 1, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 || value > 1 {
		return 0, errors.New("NEXUSIM_MESSAGE_OTEL_TRACES_SAMPLING_RATIO must be > 0 and <= 1")
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
	return errors.New("message-service uses verified metadata auth on non-private address without gRPC mTLS client certificate")
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

func loadMessageGRPCCredentialsFromEnv() (credentials.TransportCredentials, bool, error) {
	tlsConfig, ok, err := messageGRPCTLSConfigFromEnv()
	if err != nil || !ok {
		return nil, ok, err
	}
	return credentials.NewTLS(tlsConfig), true, nil
}

func messageGRPCTLSConfigFromEnv() (*tls.Config, bool, error) {
	certFile := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_GRPC_TLS_CERT_FILE"))
	keyFile := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_GRPC_TLS_KEY_FILE"))
	clientCAFile := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_GRPC_TLS_CLIENT_CA_FILE"))
	allowedClientDNSNames := envStringSet("NEXUSIM_MESSAGE_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", strings.ToLower)
	allowedClientURIs, err := envURIStringSet("NEXUSIM_MESSAGE_GRPC_TLS_CLIENT_ALLOWED_URIS")
	if err != nil {
		return nil, true, err
	}
	requireClientCert, requireClientCertConfigured, err := envOptionalBool("NEXUSIM_MESSAGE_GRPC_TLS_REQUIRE_CLIENT_CERT")
	if err != nil {
		return nil, true, err
	}
	hasClientAllowlist := len(allowedClientDNSNames) > 0 || len(allowedClientURIs) > 0
	requireClientCert = clientCAFile != "" || hasClientAllowlist || (requireClientCertConfigured && requireClientCert)
	if certFile == "" && keyFile == "" && clientCAFile == "" && !requireClientCert && !hasClientAllowlist {
		return nil, false, nil
	}
	if certFile == "" || keyFile == "" {
		return nil, true, errors.New("NEXUSIM_MESSAGE_GRPC_TLS_CERT_FILE and NEXUSIM_MESSAGE_GRPC_TLS_KEY_FILE must be configured together")
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
			return nil, true, errors.New("NEXUSIM_MESSAGE_GRPC_TLS_CLIENT_CA_FILE is required when client certificates are required")
		}
		pemBytes, err := os.ReadFile(clientCAFile)
		if err != nil {
			return nil, true, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, true, errors.New("NEXUSIM_MESSAGE_GRPC_TLS_CLIENT_CA_FILE does not contain a valid PEM certificate")
		}
		tlsConfig.ClientCAs = pool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		if hasClientAllowlist {
			tlsConfig.VerifyConnection = verifyAllowedMessageGRPCClient(allowedClientDNSNames, allowedClientURIs)
		}
	}
	return tlsConfig, true, nil
}

func verifyAllowedMessageGRPCClient(allowedDNSNames map[string]struct{}, allowedURIs map[string]struct{}) func(tls.ConnectionState) error {
	return func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("message grpc client certificate is required")
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
		return errors.New("message grpc client certificate identity is not allowed")
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

func envInt64(name string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
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

func outboxRepairCleanupConfigFromEnv() (outboxRepairCleanupConfig, error) {
	retention, err := envPositiveDuration("NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_RETENTION", 7*24*time.Hour)
	if err != nil {
		return outboxRepairCleanupConfig{}, err
	}
	batchSize := envInt("NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_BATCH_SIZE", 200)
	if batchSize <= 0 {
		return outboxRepairCleanupConfig{}, errors.New("NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_BATCH_SIZE must be positive")
	}
	return outboxRepairCleanupConfig{
		Retention: retention,
		BatchSize: batchSize,
	}, nil
}

func envFloat(name string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed <= 0 {
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
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
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
		return 0, errors.New(name + " must be a positive duration")
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

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}

func formatOptionalInt(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}
