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
	grpcapi "github.com/qsyy0921/IM/services/receipt-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/receipt-service/internal/app"
	accessinfra "github.com/qsyy0921/IM/services/receipt-service/internal/infrastructure/access"
	kafkainfra "github.com/qsyy0921/IM/services/receipt-service/internal/infrastructure/kafka"
	monitoringinfra "github.com/qsyy0921/IM/services/receipt-service/internal/infrastructure/monitoring"
	postgresinfra "github.com/qsyy0921/IM/services/receipt-service/internal/infrastructure/postgres"
	"github.com/qsyy0921/IM/services/receipt-service/internal/trigger/delivery"
	"github.com/qsyy0921/IM/services/receipt-service/internal/trigger/outbox"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run() error {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_RECEIPT_SERVICE_MODE"))
	switch mode {
	case "", "noop":
		log.Println("receipt-service runtime wiring is idle; set NEXUSIM_RECEIPT_SERVICE_MODE=grpc, delivery-consumer, outbox-relay, outbox-audit, outbox-repair, or outbox-repair-audit")
		return nil
	case "grpc":
		return runGRPCServer()
	case "delivery-consumer":
		return runDeliveryConsumer()
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
		return errors.New("unsupported NEXUSIM_RECEIPT_SERVICE_MODE")
	}
}

func runGRPCServer() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	grpcMetrics := monitoringinfra.NewGRPCMetrics()
	traceConfig, err := receiptTraceConfigFromEnv()
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
			log.Printf("receipt-service OpenTelemetry trace shutdown failed: %v", err)
		}
	}()
	debugAddr, err := receiptDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr, monitoringinfra.NewHandler(pool, grpcMetrics).WithTraceStats(traceRuntime.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()

	listenAddr := envString("NEXUSIM_RECEIPT_GRPC_ADDR", "0.0.0.0:10499")
	authMode := envString("NEXUSIM_RECEIPT_AUTH_MODE", "body")
	serverTLSConfig, serverTLSEnabled, err := receiptGRPCTLSConfigFromEnv()
	if err != nil {
		return err
	}
	if err := validateTrustedMetadataListenerConfig(listenAddr, authMode, serverTLSConfig); err != nil {
		return err
	}
	server, err := newGRPCServerWithConfig(grpcMetrics, authMode, serverTLSConfig, serverTLSEnabled, traceRuntime.UnaryServerInterceptor())
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	repository := postgresinfra.NewRepository(pool)
	access := accessinfra.NewStaticAllowAccess()
	grpcapi.Register(
		server,
		grpcapi.NewServer(
			app.NewMarkReadUseCase(repository, access),
			app.NewGetReceiptStateUseCase(repository, access),
			app.NewListReceiptStatesUseCase(repository, access),
			app.NewListConversationsUseCase(repository),
			app.NewArchiveConversationUseCase(repository),
			app.NewPinConversationUseCase(repository),
			app.NewMuteConversationUseCase(repository),
		),
	)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("receipt-service gRPC server started on %s", listenAddr)

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

func runDeliveryConsumer() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	brokers := splitCSV(os.Getenv("NEXUSIM_KAFKA_BROKERS"))
	topic := envString("NEXUSIM_DELIVERY_EVENTS_TOPIC", "im.delivery.events")
	groupID := envString("NEXUSIM_RECEIPT_CONSUMER_GROUP", "nexusim-receipt-service")
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
	worker := delivery.NewWorker(
		consumer,
		app.NewProjectDeliveryEventUseCase(repository),
		groupID,
		delivery.Config{
			ErrorBackoff: envDuration("NEXUSIM_RECEIPT_DELIVERY_CONSUMER_ERROR_BACKOFF", time.Second),
			Logf:         log.Printf,
		},
	)
	debugAddr, err := receiptDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr, monitoringinfra.NewHandler(pool).WithDeliveryProjectionWorkerStats(worker.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()
	log.Printf("receipt-service delivery consumer started topic=%s group=%s", topic, groupID)
	return worker.Run(ctx)
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

	topic := envString("NEXUSIM_RECEIPT_EVENTS_TOPIC", outbox.TopicReceiptEvents)
	relay := outbox.NewRelay(
		postgresinfra.NewOutboxStore(pool),
		producer,
		outbox.Config{
			Topic:          topic,
			BatchSize:      envInt("NEXUSIM_RECEIPT_OUTBOX_BATCH_SIZE", 500),
			PollInterval:   envDuration("NEXUSIM_RECEIPT_OUTBOX_POLL_INTERVAL", time.Second),
			MaxAttempts:    envInt("NEXUSIM_RECEIPT_OUTBOX_MAX_ATTEMPTS", 5),
			RetryBaseDelay: envDuration("NEXUSIM_RECEIPT_OUTBOX_RETRY_BASE_DELAY", time.Second),
			ErrorBackoff:   envDuration("NEXUSIM_RECEIPT_OUTBOX_RELAY_ERROR_BACKOFF", time.Second),
			Logf:           log.Printf,
		},
	)
	debugAddr, err := receiptDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr, monitoringinfra.NewHandler(pool).WithOutboxRelayStats(relay.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()
	log.Printf("receipt-service outbox relay started topic=%s", topic)
	return relay.Run(ctx)
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
	if value := strings.TrimSpace(os.Getenv("NEXUSIM_RECEIPT_OUTBOX_AUDIT_OUTBOX_ID")); value != "" {
		parsed := envInt64AllowZero("NEXUSIM_RECEIPT_OUTBOX_AUDIT_OUTBOX_ID", 0)
		outboxID = &parsed
	}
	rows, err := postgresinfra.NewOutboxStore(pool).AuditOutbox(ctx, postgresinfra.OutboxAuditOptions{
		OutboxID:       outboxID,
		EventID:        envString("NEXUSIM_RECEIPT_OUTBOX_AUDIT_EVENT_ID", ""),
		TenantID:       envString("NEXUSIM_RECEIPT_OUTBOX_AUDIT_TENANT_ID", ""),
		ConversationID: envString("NEXUSIM_RECEIPT_OUTBOX_AUDIT_CONVERSATION_ID", ""),
		Status:         envString("NEXUSIM_RECEIPT_OUTBOX_AUDIT_STATUS", ""),
		EventType:      envString("NEXUSIM_RECEIPT_OUTBOX_AUDIT_EVENT_TYPE", ""),
		Limit:          envInt("NEXUSIM_RECEIPT_OUTBOX_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("receipt-service outbox audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"receipt_outbox id=%d event_id=%s tenant_id=%s conversation_id=%s aggregate_version=%d event_type=%s status=%s retry_count=%d available_at=%s published_at=%s dead_lettered_at=%s last_error=%q",
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

func runOutboxRepair() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	eventIDs := splitCSV(os.Getenv("NEXUSIM_RECEIPT_OUTBOX_REPAIR_EVENT_IDS"))
	reason := envString("NEXUSIM_RECEIPT_OUTBOX_REPAIR_REASON", "manual receipt outbox repair")
	stats, err := postgresinfra.NewOutboxStore(pool).RepairDLQEvents(ctx, eventIDs, reason)
	if err != nil {
		return err
	}
	log.Printf(
		"receipt-service outbox repair completed requested=%d repaired=%d skipped=%d",
		stats.Requested,
		stats.Repaired,
		stats.Skipped,
	)
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

	rows, err := postgresinfra.NewOutboxStore(pool).AuditOutboxRepairs(ctx, postgresinfra.OutboxRepairAuditOptions{
		EventID:  envString("NEXUSIM_RECEIPT_OUTBOX_REPAIR_AUDIT_EVENT_ID", ""),
		TenantID: envString("NEXUSIM_RECEIPT_OUTBOX_REPAIR_AUDIT_TENANT_ID", ""),
		Limit:    envInt("NEXUSIM_RECEIPT_OUTBOX_REPAIR_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("receipt-service outbox repair audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"receipt_outbox_repair event_id=%s tenant_id=%s previous_status=%s previous_retry_count=%d previous_dead_lettered_at=%s repaired_at=%s reason=%q previous_last_error=%q",
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
	return nil
}

type outboxRepairCleanupConfig struct {
	Retention time.Duration
	BatchSize int
}

func runOutboxRepairCleanup() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
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
		EventID:  envString("NEXUSIM_RECEIPT_OUTBOX_REPAIR_CLEANUP_EVENT_ID", ""),
		TenantID: envString("NEXUSIM_RECEIPT_OUTBOX_REPAIR_CLEANUP_TENANT_ID", ""),
		Cutoff:   cutoff,
		Limit:    config.BatchSize,
	})
	if err != nil {
		return err
	}
	log.Printf(
		"receipt-service outbox repair cleanup completed deleted=%d cutoff=%s retention=%s batch_size=%d",
		stats.Deleted,
		cutoff.Format(time.RFC3339),
		config.Retention,
		config.BatchSize,
	)
	return nil
}

func openPGPool(ctx context.Context) (*pgxpool.Pool, error) {
	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return nil, errors.New("NEXUSIM_PG_DSN is required")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if maxConns := envInt("NEXUSIM_RECEIPT_PG_MAX_CONNS", 0); maxConns > 0 {
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

func newGRPCServer(grpcMetrics ...*monitoringinfra.GRPCMetrics) (*grpc.Server, error) {
	authMode := envString("NEXUSIM_RECEIPT_AUTH_MODE", "body")
	tlsConfig, ok, err := receiptGRPCTLSConfigFromEnv()
	if err != nil {
		return nil, err
	}
	var metrics *monitoringinfra.GRPCMetrics
	if len(grpcMetrics) > 0 {
		metrics = grpcMetrics[0]
	}
	return newGRPCServerWithConfig(metrics, authMode, tlsConfig, ok)
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
		return nil, errors.New("unsupported NEXUSIM_RECEIPT_AUTH_MODE")
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

func receiptTraceConfigFromEnv() (monitoringinfra.TraceConfig, error) {
	enabled, _, err := envOptionalBool("NEXUSIM_RECEIPT_OTEL_TRACES_ENABLED")
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	otlpInsecure, _, err := envOptionalBool("NEXUSIM_RECEIPT_OTEL_TRACES_OTLP_INSECURE")
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	samplingRatio, err := receiptTraceSamplingRatioFromEnv()
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	return monitoringinfra.TraceConfig{
		Enabled:       enabled,
		ServiceName:   envString("NEXUSIM_RECEIPT_OTEL_SERVICE_NAME", "receipt-service"),
		Exporter:      envString("NEXUSIM_RECEIPT_OTEL_TRACES_EXPORTER", "stdout"),
		OTLPEndpoint:  envString("NEXUSIM_RECEIPT_OTEL_TRACES_OTLP_ENDPOINT", ""),
		OTLPInsecure:  otlpInsecure,
		SamplingRatio: samplingRatio,
	}, nil
}

func receiptTraceSamplingRatioFromEnv() (float64, error) {
	raw := strings.TrimSpace(os.Getenv("NEXUSIM_RECEIPT_OTEL_TRACES_SAMPLING_RATIO"))
	if raw == "" {
		return 1, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 || value > 1 {
		return 0, errors.New("NEXUSIM_RECEIPT_OTEL_TRACES_SAMPLING_RATIO must be > 0 and <= 1")
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
	return errors.New("receipt-service uses verified metadata auth on non-private address without gRPC mTLS client certificate")
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

func loadReceiptGRPCCredentialsFromEnv() (credentials.TransportCredentials, bool, error) {
	tlsConfig, ok, err := receiptGRPCTLSConfigFromEnv()
	if err != nil || !ok {
		return nil, ok, err
	}
	return credentials.NewTLS(tlsConfig), true, nil
}

func receiptGRPCTLSConfigFromEnv() (*tls.Config, bool, error) {
	certFile := strings.TrimSpace(os.Getenv("NEXUSIM_RECEIPT_GRPC_TLS_CERT_FILE"))
	keyFile := strings.TrimSpace(os.Getenv("NEXUSIM_RECEIPT_GRPC_TLS_KEY_FILE"))
	clientCAFile := strings.TrimSpace(os.Getenv("NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_CA_FILE"))
	allowedClientDNSNames := envStringSet("NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", strings.ToLower)
	allowedClientURIs, err := envURIStringSet("NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_ALLOWED_URIS")
	if err != nil {
		return nil, true, err
	}
	requireClientCert, requireClientCertConfigured, err := envOptionalBool("NEXUSIM_RECEIPT_GRPC_TLS_REQUIRE_CLIENT_CERT")
	if err != nil {
		return nil, true, err
	}
	hasClientAllowlist := len(allowedClientDNSNames) > 0 || len(allowedClientURIs) > 0
	requireClientCert = clientCAFile != "" || hasClientAllowlist || (requireClientCertConfigured && requireClientCert)
	if certFile == "" && keyFile == "" && clientCAFile == "" && !requireClientCert && !hasClientAllowlist {
		return nil, false, nil
	}
	if certFile == "" || keyFile == "" {
		return nil, true, errors.New("NEXUSIM_RECEIPT_GRPC_TLS_CERT_FILE and NEXUSIM_RECEIPT_GRPC_TLS_KEY_FILE must be configured together")
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
			return nil, true, errors.New("NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_CA_FILE is required when client certificates are required")
		}
		pemBytes, err := os.ReadFile(clientCAFile)
		if err != nil {
			return nil, true, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, true, errors.New("NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_CA_FILE does not contain a valid PEM certificate")
		}
		tlsConfig.ClientCAs = pool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		if hasClientAllowlist {
			tlsConfig.VerifyConnection = verifyAllowedReceiptGRPCClient(allowedClientDNSNames, allowedClientURIs)
		}
	}
	return tlsConfig, true, nil
}

func verifyAllowedReceiptGRPCClient(allowedDNSNames map[string]struct{}, allowedURIs map[string]struct{}) func(tls.ConnectionState) error {
	return func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("receipt grpc client certificate is required")
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
		return errors.New("receipt grpc client certificate identity is not allowed")
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

func startDebugServer(ctx context.Context, addr string, handler http.Handler) (func(), error) {
	if strings.TrimSpace(addr) == "" || handler == nil {
		return func() {}, nil
	}
	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	select {
	case err := <-errCh:
		return nil, err
	case <-time.After(50 * time.Millisecond):
	case <-ctx.Done():
		_ = server.Shutdown(context.Background())
		return func() {}, ctx.Err()
	}
	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()
	return func() {
		_ = server.Shutdown(context.Background())
	}, nil
}

func receiptDebugAddr() string {
	return envString("NEXUSIM_RECEIPT_DEBUG_ADDR", envString("NEXUSIM_DEBUG_ADDR", ""))
}

func receiptDebugAddrFromEnv() (string, error) {
	addr := receiptDebugAddr()
	allowPublic, _, err := envOptionalBool("NEXUSIM_RECEIPT_DEBUG_ALLOW_PUBLIC")
	if err != nil {
		return "", err
	}
	return addr, validateReceiptDebugListenerConfig(addr, allowPublic)
}

func validateReceiptDebugListenerConfig(addr string, allowPublic bool) error {
	if strings.TrimSpace(addr) == "" {
		return nil
	}
	if listenerAddrTrustedWithoutMTLS(addr) {
		return nil
	}
	if allowPublic {
		return nil
	}
	return errors.New("receipt-service debug listener address is non-private; set NEXUSIM_RECEIPT_DEBUG_ALLOW_PUBLIC=true to allow")
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
	retention, err := envPositiveDuration("NEXUSIM_RECEIPT_OUTBOX_REPAIR_RETENTION", 7*24*time.Hour)
	if err != nil {
		return outboxRepairCleanupConfig{}, err
	}
	batchSize, err := envPositiveInt("NEXUSIM_RECEIPT_OUTBOX_REPAIR_CLEANUP_BATCH_SIZE", 5000)
	if err != nil {
		return outboxRepairCleanupConfig{}, err
	}
	return outboxRepairCleanupConfig{
		Retention: retention,
		BatchSize: batchSize,
	}, nil
}
