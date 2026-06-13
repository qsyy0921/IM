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
	contactsgrpc "github.com/qsyy0921/IM/services/contacts-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/contacts-service/internal/app"
	kafkainfra "github.com/qsyy0921/IM/services/contacts-service/internal/infrastructure/kafka"
	monitoringinfra "github.com/qsyy0921/IM/services/contacts-service/internal/infrastructure/monitoring"
	postgresinfra "github.com/qsyy0921/IM/services/contacts-service/internal/infrastructure/postgres"
	"github.com/qsyy0921/IM/services/contacts-service/internal/trigger/outbox"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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
		log.Println("contacts-service runtime wiring is idle; set NEXUSIM_CONTACTS_SERVICE_MODE=grpc, outbox-relay, outbox-repair, or outbox-repair-audit")
		return nil
	case "grpc":
		return runGRPC()
	case "outbox-relay":
		return runOutboxRelay()
	case "outbox-repair":
		return runOutboxRepair()
	case "outbox-repair-audit":
		return runOutboxRepairAudit()
	default:
		return errors.New("unsupported NEXUSIM_CONTACTS_SERVICE_MODE")
	}
}

func runGRPC() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	grpcMetrics := monitoringinfra.NewGRPCMetrics()
	stopDebug, err := startDebugServer(ctx, contactsDebugAddr(), monitoringinfra.NewHandler(pool, grpcMetrics))
	if err != nil {
		return err
	}
	defer stopDebug()

	addr := envString("NEXUSIM_CONTACTS_GRPC_ADDR", "0.0.0.0:10500")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	repository := postgresinfra.NewRepository(pool)
	server, err := newGRPCServer(grpcMetrics)
	if err != nil {
		return err
	}
	contactsgrpc.Register(server, contactsgrpc.NewServer(
		app.NewSendContactRequestUseCase(repository),
		app.NewRespondContactRequestUseCase(repository),
		app.NewCancelContactRequestUseCase(repository),
		app.NewListContactRequestsUseCase(repository),
		app.NewListContactsUseCase(repository),
		app.NewGetContactStateUseCase(repository),
		app.NewDeleteContactUseCase(repository),
		app.NewBlockContactUseCase(repository),
		app.NewUnblockContactUseCase(repository),
		app.NewUpdateContactRemarkUseCase(repository),
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
	stopDebug, err := startDebugServer(ctx, contactsDebugAddr(), monitoringinfra.NewHandler(pool))
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
		},
	)
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
		EventID:  envString("NEXUSIM_CONTACTS_OUTBOX_REPAIR_AUDIT_EVENT_ID", ""),
		TenantID: envString("NEXUSIM_CONTACTS_OUTBOX_REPAIR_AUDIT_TENANT_ID", ""),
		Limit:    envInt("NEXUSIM_CONTACTS_OUTBOX_REPAIR_AUDIT_LIMIT", 20),
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
			log.Printf("contacts-service debug server stopped with error: %v", err)
		}
	}()
	log.Printf("contacts-service debug server started on %s", addr)
	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		<-done
	}, nil
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
	if maxConns := envInt("NEXUSIM_CONTACTS_PG_MAX_CONNS", 0); maxConns > 0 {
		config.MaxConns = int32(maxConns)
	}
	return pgxpool.NewWithConfig(ctx, config)
}

func contactsDebugAddr() string {
	return envString("NEXUSIM_CONTACTS_DEBUG_ADDR", envString("NEXUSIM_DEBUG_ADDR", ""))
}

func newGRPCServer(grpcMetrics *monitoringinfra.GRPCMetrics) (*grpc.Server, error) {
	interceptors := make([]grpc.UnaryServerInterceptor, 0, 2)
	if grpcMetrics != nil {
		interceptors = append(interceptors, grpcMetrics.UnaryServerInterceptor(log.Default()))
	}
	switch strings.ToLower(envString("NEXUSIM_CONTACTS_AUTH_MODE", "body")) {
	case "body", "request", "legacy":
	case "metadata", "verified-metadata":
		interceptors = append(interceptors, contactsgrpc.VerifiedAuthUnaryInterceptor(true))
	default:
		return nil, errors.New("unsupported NEXUSIM_CONTACTS_AUTH_MODE")
	}
	serverOptions := make([]grpc.ServerOption, 0, 2)
	if len(interceptors) > 0 {
		serverOptions = append(serverOptions, grpc.ChainUnaryInterceptor(interceptors...))
	}
	if creds, ok, err := loadContactsGRPCCredentialsFromEnv(); err != nil {
		return nil, err
	} else if ok {
		serverOptions = append(serverOptions, grpc.Creds(creds))
	}
	return grpc.NewServer(serverOptions...), nil
}

func loadContactsGRPCCredentialsFromEnv() (credentials.TransportCredentials, bool, error) {
	tlsConfig, ok, err := contactsGRPCTLSConfigFromEnv()
	if err != nil || !ok {
		return nil, ok, err
	}
	return credentials.NewTLS(tlsConfig), true, nil
}

func contactsGRPCTLSConfigFromEnv() (*tls.Config, bool, error) {
	certFile := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_GRPC_TLS_CERT_FILE"))
	keyFile := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_GRPC_TLS_KEY_FILE"))
	clientCAFile := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_CA_FILE"))
	allowedClientDNSNames := envStringSet("NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", strings.ToLower)
	allowedClientURIs, err := envURIStringSet("NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_ALLOWED_URIS")
	if err != nil {
		return nil, true, err
	}
	requireClientCert, requireClientCertConfigured, err := envOptionalBool("NEXUSIM_CONTACTS_GRPC_TLS_REQUIRE_CLIENT_CERT")
	if err != nil {
		return nil, true, err
	}
	hasClientAllowlist := len(allowedClientDNSNames) > 0 || len(allowedClientURIs) > 0
	requireClientCert = clientCAFile != "" || hasClientAllowlist || (requireClientCertConfigured && requireClientCert)
	if certFile == "" && keyFile == "" && clientCAFile == "" && !requireClientCert && !hasClientAllowlist {
		return nil, false, nil
	}
	if certFile == "" || keyFile == "" {
		return nil, true, errors.New("NEXUSIM_CONTACTS_GRPC_TLS_CERT_FILE and NEXUSIM_CONTACTS_GRPC_TLS_KEY_FILE must be configured together")
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
			return nil, true, errors.New("NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_CA_FILE is required when client certificates are required")
		}
		pemBytes, err := os.ReadFile(clientCAFile)
		if err != nil {
			return nil, true, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, true, errors.New("NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_CA_FILE does not contain a valid PEM certificate")
		}
		tlsConfig.ClientCAs = pool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		if hasClientAllowlist {
			tlsConfig.VerifyConnection = verifyAllowedContactsGRPCClient(allowedClientDNSNames, allowedClientURIs)
		}
	}
	return tlsConfig, true, nil
}

func verifyAllowedContactsGRPCClient(allowedDNSNames map[string]struct{}, allowedURIs map[string]struct{}) func(tls.ConnectionState) error {
	return func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("contacts grpc client certificate is required")
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
		return errors.New("contacts grpc client certificate identity is not allowed")
	}
}

func envString(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
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
