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
		log.Println("policy-service runtime wiring is idle; set NEXUSIM_POLICY_SERVICE_MODE=grpc, contact-consumer, timeline-consumer, outbox-relay, outbox-audit, outbox-repair, outbox-repair-audit, or outbox-repair-cleanup")
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
	default:
		return errors.New("unsupported NEXUSIM_POLICY_SERVICE_MODE")
	}
}

type outboxRepairCleanupConfig struct {
	Retention time.Duration
	BatchSize int
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
	if value := strings.TrimSpace(os.Getenv("NEXUSIM_POLICY_OUTBOX_AUDIT_OUTBOX_ID")); value != "" {
		parsed := envInt64AllowZero("NEXUSIM_POLICY_OUTBOX_AUDIT_OUTBOX_ID", 0)
		outboxID = &parsed
	}
	rows, err := postgresinfra.NewOutboxStore(pool).AuditOutbox(ctx, postgresinfra.OutboxAuditOptions{
		OutboxID:    outboxID,
		EventID:     envString("NEXUSIM_POLICY_OUTBOX_AUDIT_EVENT_ID", ""),
		TenantID:    envString("NEXUSIM_POLICY_OUTBOX_AUDIT_TENANT_ID", ""),
		AggregateID: envString("NEXUSIM_POLICY_OUTBOX_AUDIT_AGGREGATE_ID", ""),
		Status:      envString("NEXUSIM_POLICY_OUTBOX_AUDIT_STATUS", ""),
		EventType:   envString("NEXUSIM_POLICY_OUTBOX_AUDIT_EVENT_TYPE", ""),
		Limit:       envInt("NEXUSIM_POLICY_OUTBOX_AUDIT_LIMIT", 20),
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
	reason := envString("NEXUSIM_POLICY_OUTBOX_REPAIR_REASON", "manual policy audit outbox repair")
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

	rows, err := postgresinfra.NewOutboxStore(pool).AuditOutboxRepairs(ctx, postgresinfra.OutboxRepairAuditOptions{
		EventID:  envString("NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_EVENT_ID", ""),
		TenantID: envString("NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_TENANT_ID", ""),
		Operator: envString("NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_OPERATOR", ""),
		Outcome:  envString("NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_OUTCOME", ""),
		Limit:    envInt("NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_LIMIT", 20),
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
		if err := writeOutboxRepairAuditOutput(outputPath, rows); err != nil {
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

	stats, err := postgresinfra.NewOutboxStore(pool).CleanupOutboxRepairs(ctx, postgresinfra.OutboxRepairCleanupOptions{
		EventID:  envString("NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_EVENT_ID", ""),
		TenantID: envString("NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_TENANT_ID", ""),
		Operator: envString("NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_OPERATOR", ""),
		Outcome:  envString("NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_OUTCOME", ""),
		Cutoff:   time.Now().UTC().Add(-config.Retention),
		Limit:    config.BatchSize,
	})
	if err != nil {
		return err
	}
	log.Printf(
		"policy-service outbox repair cleanup completed deleted=%d retention=%s batch_size=%d",
		stats.Deleted,
		config.Retention,
		config.BatchSize,
	)
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

	policy := domain.StaticMessagePolicy{
		Allowed:           envBool("NEXUSIM_POLICY_MESSAGE_ALLOWED", true),
		PermissionVersion: envInt64("NEXUSIM_POLICY_PERMISSION_VERSION", 1),
		Classification:    envString("NEXUSIM_POLICY_CLASSIFICATION", "INTERNAL"),
		Reason:            envString("NEXUSIM_POLICY_DENY_REASON", ""),
	}
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
			log.Printf("policy-service debug server stopped with error: %v", err)
		}
	}()
	log.Printf("policy-service debug server started on %s", addr)
	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		<-done
	}, nil
}

func openPGPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if maxConns := envInt("NEXUSIM_POLICY_PG_MAX_CONNS", 0); maxConns > 0 {
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

func policyDebugAddr() string {
	return envString("NEXUSIM_POLICY_DEBUG_ADDR", envString("NEXUSIM_DEBUG_ADDR", ""))
}

func policyDebugAddrFromEnv() (string, error) {
	addr := policyDebugAddr()
	allowPublic, _, err := envOptionalBool("NEXUSIM_POLICY_DEBUG_ALLOW_PUBLIC")
	if err != nil {
		return "", err
	}
	return addr, validatePolicyDebugListenerConfig(addr, allowPublic)
}

func validatePolicyDebugListenerConfig(addr string, allowPublic bool) error {
	if strings.TrimSpace(addr) == "" {
		return nil
	}
	if listenerAddrTrustedWithoutMTLS(addr) {
		return nil
	}
	if allowPublic {
		return nil
	}
	return errors.New("policy-service debug listener address is non-private; set NEXUSIM_POLICY_DEBUG_ALLOW_PUBLIC=true to allow")
}

func policyTraceConfigFromEnv() (monitoringinfra.TraceConfig, error) {
	enabled, _, err := envOptionalBool("NEXUSIM_POLICY_OTEL_TRACES_ENABLED")
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	otlpInsecure, _, err := envOptionalBool("NEXUSIM_POLICY_OTEL_TRACES_OTLP_INSECURE")
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	samplingRatio, err := policyTraceSamplingRatioFromEnv()
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	return monitoringinfra.TraceConfig{
		Enabled:       enabled,
		ServiceName:   envString("NEXUSIM_POLICY_OTEL_SERVICE_NAME", "policy-service"),
		Exporter:      envString("NEXUSIM_POLICY_OTEL_TRACES_EXPORTER", "stdout"),
		OTLPEndpoint:  envString("NEXUSIM_POLICY_OTEL_TRACES_OTLP_ENDPOINT", ""),
		OTLPInsecure:  otlpInsecure,
		SamplingRatio: samplingRatio,
	}, nil
}

func policyTraceSamplingRatioFromEnv() (float64, error) {
	raw := strings.TrimSpace(os.Getenv("NEXUSIM_POLICY_OTEL_TRACES_SAMPLING_RATIO"))
	if raw == "" {
		return 1, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 || value > 1 {
		return 0, errors.New("NEXUSIM_POLICY_OTEL_TRACES_SAMPLING_RATIO must be > 0 and <= 1")
	}
	return value, nil
}

func validatePolicyListenerConfig(listenAddr string, tlsEnabled bool) error {
	if listenerAddrTrustedWithoutMTLS(listenAddr) {
		return nil
	}
	if tlsEnabled {
		return nil
	}
	return errors.New("policy-service uses non-private gRPC listener address without TLS")
}

func loadPolicyGRPCCredentialsFromEnv() (credentials.TransportCredentials, bool, error) {
	tlsConfig, ok, err := policyGRPCTLSConfigFromEnv()
	if err != nil || !ok {
		return nil, ok, err
	}
	return credentials.NewTLS(tlsConfig), true, nil
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

func policyGRPCTLSConfigFromEnv() (*tls.Config, bool, error) {
	certFile := strings.TrimSpace(os.Getenv("NEXUSIM_POLICY_GRPC_TLS_CERT_FILE"))
	keyFile := strings.TrimSpace(os.Getenv("NEXUSIM_POLICY_GRPC_TLS_KEY_FILE"))
	clientCAFile := strings.TrimSpace(os.Getenv("NEXUSIM_POLICY_GRPC_TLS_CLIENT_CA_FILE"))
	allowedClientDNSNames := envStringSet("NEXUSIM_POLICY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", strings.ToLower)
	allowedClientURIs, err := envURIStringSet("NEXUSIM_POLICY_GRPC_TLS_CLIENT_ALLOWED_URIS")
	if err != nil {
		return nil, true, err
	}
	requireClientCert, requireClientCertConfigured, err := envOptionalBool("NEXUSIM_POLICY_GRPC_TLS_REQUIRE_CLIENT_CERT")
	if err != nil {
		return nil, true, err
	}
	hasClientAllowlist := len(allowedClientDNSNames) > 0 || len(allowedClientURIs) > 0
	requireClientCert = clientCAFile != "" || hasClientAllowlist || (requireClientCertConfigured && requireClientCert)
	if certFile == "" && keyFile == "" && clientCAFile == "" && !requireClientCert && !hasClientAllowlist {
		return nil, false, nil
	}
	if certFile == "" || keyFile == "" {
		return nil, true, errors.New("NEXUSIM_POLICY_GRPC_TLS_CERT_FILE and NEXUSIM_POLICY_GRPC_TLS_KEY_FILE must be configured together")
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
			return nil, true, errors.New("NEXUSIM_POLICY_GRPC_TLS_CLIENT_CA_FILE is required when client certificates are required")
		}
		pemBytes, err := os.ReadFile(clientCAFile)
		if err != nil {
			return nil, true, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, true, errors.New("NEXUSIM_POLICY_GRPC_TLS_CLIENT_CA_FILE does not contain a valid PEM certificate")
		}
		tlsConfig.ClientCAs = pool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		if hasClientAllowlist {
			tlsConfig.VerifyConnection = verifyAllowedPolicyGRPCClient(allowedClientDNSNames, allowedClientURIs)
		}
	}
	return tlsConfig, true, nil
}

func verifyAllowedPolicyGRPCClient(allowedDNSNames map[string]struct{}, allowedURIs map[string]struct{}) func(tls.ConnectionState) error {
	return func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("policy grpc client certificate is required")
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
		return errors.New("policy grpc client certificate identity is not allowed")
	}
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

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

func envPositiveInt(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, errors.New(name + " must be a positive integer")
	}
	return parsed, nil
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}

func outboxRepairCleanupConfigFromEnv() (outboxRepairCleanupConfig, error) {
	retention, err := envPositiveDuration("NEXUSIM_POLICY_OUTBOX_REPAIR_RETENTION", 7*24*time.Hour)
	if err != nil {
		return outboxRepairCleanupConfig{}, err
	}
	batchSize, err := envPositiveInt("NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_BATCH_SIZE", 5000)
	if err != nil {
		return outboxRepairCleanupConfig{}, err
	}
	return outboxRepairCleanupConfig{
		Retention: retention,
		BatchSize: batchSize,
	}, nil
}
