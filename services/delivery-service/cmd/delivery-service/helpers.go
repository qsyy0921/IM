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
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	grpcapi "github.com/qsyy0921/IM/services/delivery-service/internal/api/grpc"
	monitoringinfra "github.com/qsyy0921/IM/services/delivery-service/internal/infrastructure/monitoring"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type projectionFailureCleanupConfig struct {
	Retention time.Duration
	BatchSize int
	DryRun    bool
}

type outboxRepairCleanupConfig struct {
	Retention time.Duration
	BatchSize int
	DryRun    bool
}

type projectionCheckpointRepairCleanupConfig struct {
	Retention time.Duration
	BatchSize int
	DryRun    bool
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
		DryRun:    envBool("NEXUSIM_DELIVERY_PROJECTION_FAILURE_CLEANUP_DRY_RUN", false),
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
		DryRun:    envBool("NEXUSIM_DELIVERY_OUTBOX_REPAIR_CLEANUP_DRY_RUN", false),
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
		DryRun:    envBool("NEXUSIM_DELIVERY_PROJECTION_REPAIR_CLEANUP_DRY_RUN", false),
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
