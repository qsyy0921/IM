package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
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

	contactsv1 "github.com/qsyy0921/IM/api/proto/nexusim/contacts/v1"
	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	identityv1 "github.com/qsyy0921/IM/api/proto/nexusim/identity/v1"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	receiptv1 "github.com/qsyy0921/IM/api/proto/nexusim/receipt/v1"
	gatewayauth "github.com/qsyy0921/IM/internal/gatewayauth"
	apigrpc "github.com/qsyy0921/IM/services/api-gateway/internal/api/grpc"
	monitoringinfra "github.com/qsyy0921/IM/services/api-gateway/internal/infrastructure/monitoring"
	ratelimitinfra "github.com/qsyy0921/IM/services/api-gateway/internal/infrastructure/ratelimit"
	"github.com/redis/go-redis/v9"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run() error {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_MODE"))
	switch mode {
	case "", "noop":
		log.Println("api-gateway runtime wiring is idle; set NEXUSIM_API_GATEWAY_MODE=grpc")
		return nil
	case "grpc":
		return runGRPC()
	default:
		return errors.New("unsupported NEXUSIM_API_GATEWAY_MODE")
	}
}

func runGRPC() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	listenAddr := envString("NEXUSIM_API_GATEWAY_GRPC_ADDR", "0.0.0.0:12000")
	serverTLSConfig, serverTLSEnabled, err := apiGatewayGRPCTLSConfigFromEnv()
	if err != nil {
		return err
	}
	if err := validateAPIGatewayAuthListenerConfig(listenAddr, envString("NEXUSIM_API_GATEWAY_AUTH_MODE", "hmac"), serverTLSEnabled); err != nil {
		return err
	}

	authenticator, err := newAuthenticatorFromEnv()
	if err != nil {
		return err
	}
	defer authenticator.Close()
	grpcMetrics := monitoringinfra.NewGRPCMetrics()
	traceConfig, err := apiGatewayTraceConfigFromEnv()
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
			log.Printf("api-gateway OpenTelemetry trace shutdown failed: %v", err)
		}
	}()
	rateLimiter, closeRateLimiter, err := newRateLimiterFromEnv(ctx, authenticator)
	if err != nil {
		return err
	}
	defer closeRateLimiter()
	legacyDescriptorConfig, err := apiGatewayLegacyDescriptorConfigFromEnv(time.Now)
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, apiGatewayDebugAddr(), monitoringinfra.NewHandler(grpcMetrics).
		WithAuthJWKStats(authenticator.JWKStats).
		WithRateLimitStats(rateLimiter.Snapshot).
		WithRuntimeStats(func() monitoringinfra.RuntimeSnapshot {
			return monitoringinfra.RuntimeSnapshot{
				RegisterLegacyDescriptors:       legacyDescriptorConfig.Register,
				LegacyDescriptorsAllowedUntilMS: legacyDescriptorConfig.AllowedUntilUnixMS,
			}
		}).
		WithTraceStats(traceRuntime.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()

	conversationAddr := envString("NEXUSIM_API_GATEWAY_CONVERSATION_ADDR", "127.0.0.1:10496")
	conversationTLS := grpcClientTLSConfigFromEnv("NEXUSIM_API_GATEWAY_CONVERSATION_TLS")
	messageAddr := envString("NEXUSIM_API_GATEWAY_MESSAGE_ADDR", "127.0.0.1:10495")
	messageTLS := grpcClientTLSConfigFromEnv("NEXUSIM_API_GATEWAY_MESSAGE_TLS")
	deliveryAddr := envString("NEXUSIM_API_GATEWAY_DELIVERY_ADDR", "127.0.0.1:10497")
	deliveryTLS := grpcClientTLSConfigFromEnv("NEXUSIM_API_GATEWAY_DELIVERY_TLS")
	receiptAddr := envString("NEXUSIM_API_GATEWAY_RECEIPT_ADDR", "127.0.0.1:10499")
	receiptTLS := grpcClientTLSConfigFromEnv("NEXUSIM_API_GATEWAY_RECEIPT_TLS")
	contactsAddr := envString("NEXUSIM_API_GATEWAY_CONTACTS_ADDR", "127.0.0.1:10500")
	contactsTLS := grpcClientTLSConfigFromEnv("NEXUSIM_API_GATEWAY_CONTACTS_TLS")
	identityAddr := envString("NEXUSIM_API_GATEWAY_IDENTITY_ADDR", "127.0.0.1:10501")
	identityTLS := grpcClientTLSConfigFromEnv("NEXUSIM_API_GATEWAY_IDENTITY_TLS")

	if err := validateTrustedMetadataBackendConfig("conversation-service", conversationAddr, envString("NEXUSIM_CONVERSATION_AUTH_MODE", "body"), conversationTLS); err != nil {
		return err
	}
	if err := validateTrustedMetadataBackendConfig("message-service", messageAddr, envString("NEXUSIM_MESSAGE_AUTH_MODE", "body"), messageTLS); err != nil {
		return err
	}
	if err := validateTrustedMetadataBackendConfig("delivery-service", deliveryAddr, envString("NEXUSIM_DELIVERY_AUTH_MODE", "body"), deliveryTLS); err != nil {
		return err
	}
	if err := validateTrustedMetadataBackendConfig("receipt-service", receiptAddr, envString("NEXUSIM_RECEIPT_AUTH_MODE", "body"), receiptTLS); err != nil {
		return err
	}
	if err := validateTrustedMetadataBackendConfig("contacts-service", contactsAddr, envString("NEXUSIM_CONTACTS_AUTH_MODE", "body"), contactsTLS); err != nil {
		return err
	}
	if err := validateTrustedMetadataBackendConfig("identity-service", identityAddr, envString("NEXUSIM_IDENTITY_ADMIN_AUTH_MODE", "body"), identityTLS); err != nil {
		return err
	}

	conversationConn, err := dialBackend(
		conversationAddr,
		conversationTLS,
		traceRuntime.UnaryClientInterceptor(),
	)
	if err != nil {
		return err
	}
	defer conversationConn.Close()
	messageConn, err := dialBackend(
		messageAddr,
		messageTLS,
		traceRuntime.UnaryClientInterceptor(),
	)
	if err != nil {
		return err
	}
	defer messageConn.Close()
	deliveryConn, err := dialBackend(
		deliveryAddr,
		deliveryTLS,
		traceRuntime.UnaryClientInterceptor(),
	)
	if err != nil {
		return err
	}
	defer deliveryConn.Close()
	receiptConn, err := dialBackend(
		receiptAddr,
		receiptTLS,
		traceRuntime.UnaryClientInterceptor(),
	)
	if err != nil {
		return err
	}
	defer receiptConn.Close()
	contactsConn, err := dialBackend(
		contactsAddr,
		contactsTLS,
		traceRuntime.UnaryClientInterceptor(),
	)
	if err != nil {
		return err
	}
	defer contactsConn.Close()
	identityConn, err := dialBackend(
		identityAddr,
		identityTLS,
		traceRuntime.UnaryClientInterceptor(),
	)
	if err != nil {
		return err
	}
	defer identityConn.Close()

	gateway := apigrpc.NewServer(apigrpc.Config{
		Authenticator: authenticator,
		Contacts:      contactsv1.NewContactsServiceClient(contactsConn),
		Conversation:  conversationv1.NewConversationServiceClient(conversationConn),
		Identity:      identityv1.NewIdentityServiceClient(identityConn),
		Message:       messagev1.NewMessageServiceClient(messageConn),
		Delivery:      deliveryv1.NewDeliveryServiceClient(deliveryConn),
		Receipt:       receiptv1.NewReceiptServiceClient(receiptConn),
	})
	serverOptions := []grpcgo.ServerOption{grpcgo.ChainUnaryInterceptor(
		grpcMetrics.UnaryServerInterceptor(log.Default()),
		traceRuntime.UnaryServerInterceptor(),
		rateLimiter.UnaryServerInterceptor(),
	)}
	if serverTLSEnabled {
		serverOptions = append(serverOptions, grpcgo.Creds(credentials.NewTLS(serverTLSConfig)))
	}
	server := grpcgo.NewServer(serverOptions...)
	apigrpc.RegisterWithConfig(server, gateway, apigrpc.RegisterConfig{
		RegisterLegacyDescriptors: legacyDescriptorConfig.Register,
	})

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		server.GracefulStop()
	}()
	log.Printf("api-gateway gRPC listening on %s", listenAddr)
	if err := server.Serve(listener); err != nil && !errors.Is(err, grpcgo.ErrServerStopped) {
		return err
	}
	return ctx.Err()
}

func startDebugServer(ctx context.Context, addr string, handler http.Handler) (func(), error) {
	if strings.TrimSpace(addr) == "" {
		return func() {}, nil
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("api-gateway debug server stopped with error: %v", err)
		}
	}()
	log.Printf("api-gateway debug server started on %s", addr)
	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		select {
		case <-done:
		case <-ctx.Done():
		}
	}, nil
}

func apiGatewayDebugAddr() string {
	return envString("NEXUSIM_API_GATEWAY_DEBUG_ADDR", envString("NEXUSIM_DEBUG_ADDR", ""))
}

type apiGatewayLegacyDescriptorConfig struct {
	Register           bool
	AllowedUntilUnixMS int64
}

func apiGatewayRegisterLegacyDescriptors() (bool, error) {
	config, err := apiGatewayLegacyDescriptorConfigFromEnv(time.Now)
	if err != nil {
		return false, err
	}
	return config.Register, nil
}

func apiGatewayLegacyDescriptorConfigFromEnv(now func() time.Time) (apiGatewayLegacyDescriptorConfig, error) {
	value, configured, err := envOptionalBool("NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS")
	if err != nil {
		return apiGatewayLegacyDescriptorConfig{}, err
	}
	allowedUntilMS, err := apiGatewayLegacyDescriptorsAllowedUntilFromEnv()
	if err != nil {
		return apiGatewayLegacyDescriptorConfig{}, err
	}
	config := apiGatewayLegacyDescriptorConfig{
		Register:           configured && value,
		AllowedUntilUnixMS: allowedUntilMS,
	}
	if !configured {
		return config, nil
	}
	if config.Register && allowedUntilMS > 0 {
		if now == nil {
			now = time.Now
		}
		if !time.UnixMilli(allowedUntilMS).After(now()) {
			return apiGatewayLegacyDescriptorConfig{}, errors.New("api-gateway legacy descriptor opt-in has expired")
		}
	}
	return config, nil
}

func apiGatewayLegacyDescriptorsAllowedUntilFromEnv() (int64, error) {
	raw := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_LEGACY_DESCRIPTORS_ALLOWED_UNTIL"))
	if raw == "" {
		return 0, nil
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.UnixMilli(), nil
	}
	if parsed, err := time.Parse("2006-01-02", raw); err == nil {
		return parsed.UnixMilli(), nil
	}
	return 0, errors.New("NEXUSIM_API_GATEWAY_LEGACY_DESCRIPTORS_ALLOWED_UNTIL must be RFC3339 or YYYY-MM-DD")
}

func apiGatewayTraceConfigFromEnv() (monitoringinfra.TraceConfig, error) {
	enabled, _, err := envOptionalBool("NEXUSIM_API_GATEWAY_OTEL_TRACES_ENABLED")
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	otlpInsecure, _, err := envOptionalBool("NEXUSIM_API_GATEWAY_OTEL_TRACES_OTLP_INSECURE")
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	samplingRatio, err := apiGatewayTraceSamplingRatioFromEnv()
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	return monitoringinfra.TraceConfig{
		Enabled:       enabled,
		ServiceName:   envString("NEXUSIM_API_GATEWAY_OTEL_SERVICE_NAME", "api-gateway"),
		Exporter:      envString("NEXUSIM_API_GATEWAY_OTEL_TRACES_EXPORTER", "stdout"),
		OTLPEndpoint:  envString("NEXUSIM_API_GATEWAY_OTEL_TRACES_OTLP_ENDPOINT", ""),
		OTLPInsecure:  otlpInsecure,
		SamplingRatio: samplingRatio,
	}, nil
}

func apiGatewayTraceSamplingRatioFromEnv() (float64, error) {
	raw := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_OTEL_TRACES_SAMPLING_RATIO"))
	if raw == "" {
		return 1, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 || value > 1 {
		return 0, errors.New("NEXUSIM_API_GATEWAY_OTEL_TRACES_SAMPLING_RATIO must be > 0 and <= 1")
	}
	return value, nil
}

func dialBackend(addr string, tlsConfig grpcClientTLSConfig, unaryInterceptors ...grpcgo.UnaryClientInterceptor) (*grpcgo.ClientConn, error) {
	options := []grpcgo.DialOption{grpcgo.WithTransportCredentials(insecure.NewCredentials())}
	if tlsConfig.Enabled() {
		creds, err := grpcClientTLSCredentials(tlsConfig)
		if err != nil {
			return nil, err
		}
		options[0] = grpcgo.WithTransportCredentials(creds)
	}
	var chain []grpcgo.UnaryClientInterceptor
	for _, interceptor := range unaryInterceptors {
		if interceptor != nil {
			chain = append(chain, interceptor)
		}
	}
	if len(chain) > 0 {
		options = append(options, grpcgo.WithChainUnaryInterceptor(chain...))
	}
	return grpcgo.NewClient("passthrough:///"+addr, options...)
}

func validateTrustedMetadataBackendConfig(serviceName string, addr string, authMode string, tlsConfig grpcClientTLSConfig) error {
	if !usesTrustedMetadataAuth(authMode) {
		return nil
	}
	if backendAddrTrustedWithoutMTLS(addr) {
		return nil
	}
	if tlsConfig.ClientCertConfigured() {
		return nil
	}
	return errors.New(serviceName + " uses verified metadata auth on non-private address without gateway mTLS client certificate")
}

func validateAPIGatewayAuthListenerConfig(listenAddr string, authMode string, tlsEnabled bool) error {
	if usesMockGatewayAuth(authMode) {
		if backendAddrTrustedWithoutMTLS(listenAddr) {
			return nil
		}
		return errors.New("api-gateway uses mock auth on non-private listener address")
	}
	if backendAddrTrustedWithoutMTLS(listenAddr) {
		return nil
	}
	if !usesSignedGatewayAuth(authMode) {
		return nil
	}
	if tlsEnabled {
		return nil
	}
	return errors.New("api-gateway uses signed auth on non-private listener address without gRPC TLS")
}

func usesMockGatewayAuth(authMode string) bool {
	return strings.EqualFold(strings.TrimSpace(authMode), string(gatewayauth.ModeMock))
}

func usesSignedGatewayAuth(authMode string) bool {
	mode := strings.TrimSpace(strings.ToLower(authMode))
	return mode == string(gatewayauth.ModeHMAC) || mode == string(gatewayauth.ModeJWT)
}

func usesTrustedMetadataAuth(authMode string) bool {
	switch strings.ToLower(strings.TrimSpace(authMode)) {
	case "metadata", "verified-metadata":
		return true
	default:
		return false
	}
}

func backendAddrTrustedWithoutMTLS(addr string) bool {
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

func newAuthenticatorFromEnv() (*gatewayauth.Authenticator, error) {
	jwksJSON, err := loadJWKSetJSON()
	if err != nil {
		return nil, err
	}
	return gatewayauth.NewAuthenticator(gatewayauth.Config{
		Mode:               gatewayauth.Mode(envString("NEXUSIM_API_GATEWAY_AUTH_MODE", "hmac")),
		Secret:             os.Getenv("NEXUSIM_API_GATEWAY_AUTH_HMAC_SECRET"),
		PreviousSecrets:    splitCSV(os.Getenv("NEXUSIM_API_GATEWAY_AUTH_HMAC_PREVIOUS_SECRETS")),
		JWKSetJSON:         jwksJSON,
		JWKSetURL:          os.Getenv("NEXUSIM_API_GATEWAY_AUTH_JWKS_URL"),
		JWKRefreshInterval: envDuration("NEXUSIM_API_GATEWAY_AUTH_JWKS_REFRESH_INTERVAL", 5*time.Minute),
		TrustedIssuers:     splitCSV(os.Getenv("NEXUSIM_API_GATEWAY_AUTH_TRUSTED_ISSUERS")),
		Audience:           envString("NEXUSIM_API_GATEWAY_AUTH_AUDIENCE", "api-gateway"),
	})
}

func loadJWKSetJSON() (string, error) {
	if value := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_AUTH_JWKS_JSON")); value != "" {
		return value, nil
	}
	path := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_AUTH_JWKS_FILE"))
	if path == "" {
		return "", nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func envString(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envFloat64(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func splitCSV(value string) []string {
	var result []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func apiGatewayGRPCTLSConfigFromEnv() (*tls.Config, bool, error) {
	certFile := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_GRPC_TLS_CERT_FILE"))
	keyFile := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_GRPC_TLS_KEY_FILE"))
	clientCAFile := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_GRPC_TLS_CLIENT_CA_FILE"))
	allowedClientDNSNames := envStringSet("NEXUSIM_API_GATEWAY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", strings.ToLower)
	allowedClientURIs, err := envURIStringSet("NEXUSIM_API_GATEWAY_GRPC_TLS_CLIENT_ALLOWED_URIS")
	if err != nil {
		return nil, true, err
	}
	requireClientCert, requireClientCertConfigured, err := envOptionalBool("NEXUSIM_API_GATEWAY_GRPC_TLS_REQUIRE_CLIENT_CERT")
	if err != nil {
		return nil, true, err
	}
	hasClientAllowlist := len(allowedClientDNSNames) > 0 || len(allowedClientURIs) > 0
	requireClientCert = clientCAFile != "" || hasClientAllowlist || (requireClientCertConfigured && requireClientCert)
	if certFile == "" && keyFile == "" && clientCAFile == "" && !requireClientCert && !hasClientAllowlist {
		return nil, false, nil
	}
	if certFile == "" || keyFile == "" {
		return nil, true, errors.New("NEXUSIM_API_GATEWAY_GRPC_TLS_CERT_FILE and NEXUSIM_API_GATEWAY_GRPC_TLS_KEY_FILE must be configured together")
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
			return nil, true, errors.New("NEXUSIM_API_GATEWAY_GRPC_TLS_CLIENT_CA_FILE is required when client certificates are required")
		}
		pemBytes, err := os.ReadFile(clientCAFile)
		if err != nil {
			return nil, true, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, true, errors.New("NEXUSIM_API_GATEWAY_GRPC_TLS_CLIENT_CA_FILE does not contain a valid PEM certificate")
		}
		tlsConfig.ClientCAs = pool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		if hasClientAllowlist {
			tlsConfig.VerifyConnection = verifyAllowedAPIGatewayGRPCClient(allowedClientDNSNames, allowedClientURIs)
		}
	}
	return tlsConfig, true, nil
}

func verifyAllowedAPIGatewayGRPCClient(allowedDNSNames map[string]struct{}, allowedURIs map[string]struct{}) func(tls.ConnectionState) error {
	return func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("api-gateway grpc client certificate is required")
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
		return errors.New("api-gateway grpc client certificate identity is not allowed")
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

func newRateLimiterFromEnv(ctx context.Context, authenticator *gatewayauth.Authenticator) (*ratelimitinfra.Limiter, func() error, error) {
	enabled, _, err := envOptionalBool("NEXUSIM_API_GATEWAY_RATE_LIMIT_ENABLED")
	if err != nil {
		return nil, nil, err
	}
	rps := envFloat64("NEXUSIM_API_GATEWAY_RATE_LIMIT_RPS", 0)
	backend := strings.ToLower(strings.TrimSpace(envString("NEXUSIM_API_GATEWAY_RATE_LIMIT_BACKEND", "local")))
	scope := strings.ToLower(strings.TrimSpace(envString("NEXUSIM_API_GATEWAY_RATE_LIMIT_SCOPE", "token")))
	tenantPlanSnapshot, err := tenantRateLimitPlansFromEnv(ctx)
	if err != nil {
		return nil, nil, err
	}
	tenantPlanReloadInterval, err := tenantPlanReloadIntervalFromEnv()
	if err != nil {
		return nil, nil, err
	}
	tenantPlanMaxAge, err := tenantPlanMaxAgeFromEnv()
	if err != nil {
		return nil, nil, err
	}
	if tenantPlanReloadInterval > 0 && tenantPlanSnapshot.Source != "file" && tenantPlanSnapshot.Source != "url" {
		return nil, nil, errors.New("api-gateway tenant plan reload requires NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE=file or url")
	}
	config := ratelimitinfra.Config{
		Enabled:                     enabled,
		Backend:                     backend,
		KeyScope:                    scope,
		RequestsPerSecond:           rps,
		Burst:                       envInt("NEXUSIM_API_GATEWAY_RATE_LIMIT_BURST", int(rps)),
		TenantPlans:                 tenantPlanSnapshot.Plans,
		TenantPlanSource:            tenantPlanSnapshot.Source,
		TenantPlanVersion:           tenantPlanSnapshot.Version,
		TenantPlanGeneratedAtUnixMS: tenantPlanSnapshot.GeneratedAtUnixMS,
		TenantPlanChecksumPresent:   tenantPlanSnapshot.ChecksumPresent,
		MaxKeys:                     envInt("NEXUSIM_API_GATEWAY_RATE_LIMIT_MAX_KEYS", 10000),
		IdentityFunc:                rateLimitIdentityFunc(authenticator),
	}
	if enabled && backend == "redis" {
		failOpen := true
		if value, configured, err := envOptionalBool("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_FAIL_OPEN"); err != nil {
			return nil, nil, err
		} else if configured {
			failOpen = value
		}
		client, err := newRedisUniversalClient(loadRateLimitRedisClientConfigFromEnv())
		if err != nil {
			return nil, nil, err
		}
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := client.Ping(pingCtx).Err(); err != nil {
			if !failOpen {
				_ = client.Close()
				return nil, nil, err
			}
			log.Printf("api-gateway redis rate limiter ping failed; continuing because fail-open is enabled: %v", err)
		}
		config.RedisClient = client
		config.RedisKeyPrefix = envString("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_KEY_PREFIX", "nexusim:api-gateway")
		config.RedisWindow = envDuration("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_WINDOW", time.Second)
		config.RedisFailOpen = failOpen
		limiter, err := ratelimitinfra.New(config)
		if err != nil {
			_ = client.Close()
			return nil, nil, err
		}
		closeFn := func() error { return client.Close() }
		if tenantPlanReloadInterval > 0 {
			stopReloader, err := startTenantPlanReloader(ctx, limiter, tenantPlanSnapshot.Source, tenantPlanReloadLocationFromEnv(tenantPlanSnapshot.Source), tenantPlanMaxAge, tenantPlanReloadInterval)
			if err != nil {
				_ = closeFn()
				return nil, nil, err
			}
			closeFn = combineCloseFuncs(stopReloader, closeFn)
		}
		return limiter, closeFn, nil
	}
	limiter, err := ratelimitinfra.New(config)
	if err != nil {
		return nil, nil, err
	}
	closeFn := func() error { return nil }
	if tenantPlanReloadInterval > 0 {
		stopReloader, err := startTenantPlanReloader(ctx, limiter, tenantPlanSnapshot.Source, tenantPlanReloadLocationFromEnv(tenantPlanSnapshot.Source), tenantPlanMaxAge, tenantPlanReloadInterval)
		if err != nil {
			return nil, nil, err
		}
		closeFn = stopReloader
	}
	return limiter, closeFn, nil
}

type tenantRateLimitPlanSnapshot struct {
	Plans             map[string]ratelimitinfra.Plan
	Source            string
	Version           string
	GeneratedAtUnixMS int64
	ChecksumPresent   bool
}

type tenantRateLimitPlanPayload struct {
	RequestsPerSecond float64 `json:"requests_per_second"`
	RPS               float64 `json:"rps"`
	Burst             int     `json:"burst"`
}

type tenantRateLimitPlanSnapshotPayload struct {
	Version           string                                `json:"version"`
	GeneratedAtUnixMS int64                                 `json:"generated_at_unix_ms"`
	Checksum          string                                `json:"checksum"`
	Plans             map[string]tenantRateLimitPlanPayload `json:"plans"`
}

const (
	tenantPlanSnapshotMaxBytes      = 1 << 20
	tenantPlanSnapshotVersionPrefix = "quota-v1"
)

func tenantRateLimitPlansFromEnv(ctx context.Context) (tenantRateLimitPlanSnapshot, error) {
	source := strings.ToLower(strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE")))
	raw := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_JSON"))
	path := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_FILE"))
	endpoint := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL"))
	maxAge, err := tenantPlanMaxAgeFromEnv()
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	if source == "" || source == "auto" {
		switch {
		case raw != "":
			source = "inline"
		case path != "":
			source = "file"
		case endpoint != "":
			source = "url"
		default:
			source = "none"
		}
	}
	switch source {
	case "none":
		if raw != "" || path != "" || endpoint != "" {
			return tenantRateLimitPlanSnapshot{}, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE=none cannot be used with tenant plan JSON, file or URL")
		}
		return tenantRateLimitPlanSnapshot{Source: source}, nil
	case "inline", "json":
		if raw == "" {
			return tenantRateLimitPlanSnapshot{}, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_JSON is required when tenant plan source is inline")
		}
		source = "inline"
	case "file":
		if path == "" {
			return tenantRateLimitPlanSnapshot{}, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_FILE is required when tenant plan source is file")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return tenantRateLimitPlanSnapshot{}, err
		}
		raw = string(data)
	case "url", "http", "https", "config-url", "config_url":
		if endpoint == "" {
			return tenantRateLimitPlanSnapshot{}, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL is required when tenant plan source is url")
		}
		source = "url"
		snapshot, err := tenantRateLimitPlansFromURL(ctx, endpoint, maxAge)
		if err != nil {
			return tenantRateLimitPlanSnapshot{}, err
		}
		snapshot.Source = source
		return snapshot, nil
	case "db", "database", "config", "config-center", "config_center":
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan source " + source + " is not supported yet; use inline, file or url")
	default:
		return tenantRateLimitPlanSnapshot{}, errors.New("unsupported api-gateway tenant plan source")
	}
	snapshot, err := parseTenantRateLimitPlanSnapshot(raw)
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	snapshot.Source = source
	if err := validateTenantPlanMaxAge(snapshot, maxAge); err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	return snapshot, nil
}

func tenantRateLimitPlansFromURL(ctx context.Context, endpoint string, maxAge time.Duration) (tenantRateLimitPlanSnapshot, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return tenantRateLimitPlanSnapshot{}, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL is required when tenant plan source is url")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan URL source requires http or https")
	}
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan URL source returned non-200 status")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, tenantPlanSnapshotMaxBytes+1))
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	if len(data) > tenantPlanSnapshotMaxBytes {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan URL source response is too large")
	}
	snapshot, err := parseTenantRateLimitPlanSnapshot(string(data))
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	if snapshot.Version == "" {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan URL source requires a versioned snapshot")
	}
	if err := validateTenantPlanMaxAge(snapshot, maxAge); err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	snapshot.Source = "url"
	return snapshot, nil
}

func parseTenantRateLimitPlans(raw string) (map[string]ratelimitinfra.Plan, error) {
	snapshot, err := parseTenantRateLimitPlanSnapshot(raw)
	if err != nil {
		return nil, err
	}
	return snapshot.Plans, nil
}

func parseTenantRateLimitPlanSnapshot(raw string) (tenantRateLimitPlanSnapshot, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return tenantRateLimitPlanSnapshot{}, nil
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	if _, hasVersion := probe["version"]; hasVersion || probe["checksum"] != nil || probe["generated_at_unix_ms"] != nil {
		return parseVersionedTenantRateLimitPlanSnapshot(raw)
	}
	plans, err := parseTenantRateLimitPlanPayloads(raw)
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	return tenantRateLimitPlanSnapshot{Plans: plans}, nil
}

func parseVersionedTenantRateLimitPlanSnapshot(raw string) (tenantRateLimitPlanSnapshot, error) {
	var payload tenantRateLimitPlanSnapshotPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	payload.Version = strings.TrimSpace(payload.Version)
	if payload.Version == "" {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan snapshot version is required")
	}
	if payload.Version != tenantPlanSnapshotVersionPrefix && !strings.HasPrefix(payload.Version, tenantPlanSnapshotVersionPrefix+".") {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan snapshot version is not supported")
	}
	if payload.GeneratedAtUnixMS <= 0 {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan snapshot generated_at_unix_ms must be greater than 0")
	}
	if payload.Plans == nil {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan snapshot plans is required")
	}
	plans := tenantRateLimitPlansFromPayload(payload.Plans)
	checksumPresent, err := validateTenantPlanSnapshotChecksum(payload.Checksum, plans)
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	return tenantRateLimitPlanSnapshot{
		Plans:             plans,
		Version:           payload.Version,
		GeneratedAtUnixMS: payload.GeneratedAtUnixMS,
		ChecksumPresent:   checksumPresent,
	}, nil
}

func parseTenantRateLimitPlanPayloads(raw string) (map[string]ratelimitinfra.Plan, error) {
	var payload map[string]tenantRateLimitPlanPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}
	return tenantRateLimitPlansFromPayload(payload), nil
}

func tenantRateLimitPlansFromPayload(payload map[string]tenantRateLimitPlanPayload) map[string]ratelimitinfra.Plan {
	plans := make(map[string]ratelimitinfra.Plan, len(payload))
	for tenantID, item := range payload {
		rps := item.RequestsPerSecond
		if rps <= 0 {
			rps = item.RPS
		}
		plans[tenantID] = ratelimitinfra.Plan{RequestsPerSecond: rps, Burst: item.Burst}
	}
	return plans
}

func validateTenantPlanSnapshotChecksum(expected string, plans map[string]ratelimitinfra.Plan) (bool, error) {
	expected = strings.TrimSpace(strings.ToLower(expected))
	if expected == "" {
		return false, nil
	}
	if !strings.HasPrefix(expected, "sha256:") {
		return true, errors.New("api-gateway tenant plan snapshot checksum must use sha256:<hex>")
	}
	actual, err := tenantPlanSnapshotChecksum(plans)
	if err != nil {
		return true, err
	}
	if subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) != 1 {
		return true, errors.New("api-gateway tenant plan snapshot checksum mismatch")
	}
	return true, nil
}

func tenantPlanSnapshotChecksum(plans map[string]ratelimitinfra.Plan) (string, error) {
	type normalizedPlan struct {
		RequestsPerSecond float64 `json:"requests_per_second"`
		Burst             int     `json:"burst,omitempty"`
	}
	normalized := make(map[string]normalizedPlan, len(plans))
	for tenantID, plan := range plans {
		normalized[tenantID] = normalizedPlan{RequestsPerSecond: plan.RequestsPerSecond, Burst: plan.Burst}
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func tenantPlanReloadIntervalFromEnv() (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_RELOAD_INTERVAL"))
	if raw == "" || raw == "0" {
		return 0, nil
	}
	interval, err := time.ParseDuration(raw)
	if err != nil || interval <= 0 {
		return 0, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_RELOAD_INTERVAL must be a positive duration")
	}
	return interval, nil
}

func tenantPlanMaxAgeFromEnv() (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_MAX_AGE"))
	if raw == "" || raw == "0" {
		return 0, nil
	}
	maxAge, err := time.ParseDuration(raw)
	if err != nil || maxAge <= 0 {
		return 0, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_MAX_AGE must be a positive duration")
	}
	return maxAge, nil
}

func validateTenantPlanMaxAge(snapshot tenantRateLimitPlanSnapshot, maxAge time.Duration) error {
	if maxAge <= 0 || snapshot.GeneratedAtUnixMS <= 0 {
		return nil
	}
	generatedAt := time.UnixMilli(snapshot.GeneratedAtUnixMS)
	if time.Since(generatedAt) > maxAge {
		return errors.New("api-gateway tenant plan snapshot is stale")
	}
	return nil
}

func tenantPlanReloadLocationFromEnv(source string) string {
	switch source {
	case "file":
		return strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_FILE"))
	case "url":
		return strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL"))
	default:
		return ""
	}
}

func startTenantPlanReloader(ctx context.Context, limiter *ratelimitinfra.Limiter, source string, location string, maxAge time.Duration, interval time.Duration) (func() error, error) {
	source = strings.TrimSpace(source)
	location = strings.TrimSpace(location)
	if location == "" {
		switch source {
		case "file":
			return nil, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_FILE is required when tenant plan reload is enabled")
		case "url":
			return nil, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL is required when tenant plan reload is enabled")
		default:
			return nil, errors.New("api-gateway tenant plan reload requires file or url source")
		}
	}
	if interval <= 0 {
		return func() error { return nil }, nil
	}
	reloadCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-reloadCtx.Done():
				return
			case <-ticker.C:
				snapshot, err := tenantRateLimitPlansFromSource(reloadCtx, source, location, maxAge)
				if err != nil {
					limiter.RecordTenantPlanReloadError()
					log.Printf("api-gateway tenant rate limit plan reload failed: %v", err)
					continue
				}
				if err := limiter.UpdateTenantPlanSnapshot(snapshot.Plans, snapshot.Version, snapshot.GeneratedAtUnixMS, snapshot.ChecksumPresent); err != nil {
					log.Printf("api-gateway tenant rate limit plan reload rejected: %v", err)
				}
			}
		}
	}()
	return func() error {
		cancel()
		<-done
		return nil
	}, nil
}

func tenantRateLimitPlansFromSource(ctx context.Context, source string, location string, maxAge time.Duration) (tenantRateLimitPlanSnapshot, error) {
	switch source {
	case "file":
		snapshot, err := tenantRateLimitPlansFromFile(location)
		if err != nil {
			return tenantRateLimitPlanSnapshot{}, err
		}
		if err := validateTenantPlanMaxAge(snapshot, maxAge); err != nil {
			return tenantRateLimitPlanSnapshot{}, err
		}
		return snapshot, nil
	case "url":
		return tenantRateLimitPlansFromURL(ctx, location, maxAge)
	default:
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan reload requires file or url source")
	}
}

func tenantRateLimitPlansFromFile(path string) (tenantRateLimitPlanSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	snapshot, err := parseTenantRateLimitPlanSnapshot(string(data))
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	snapshot.Source = "file"
	return snapshot, nil
}

func combineCloseFuncs(functions ...func() error) func() error {
	return func() error {
		var combined error
		for _, fn := range functions {
			if fn == nil {
				continue
			}
			if err := fn(); err != nil {
				combined = errors.Join(combined, err)
			}
		}
		return combined
	}
}

func rateLimitIdentityFunc(authenticator *gatewayauth.Authenticator) ratelimitinfra.IdentityFunc {
	return func(ctx context.Context) (ratelimitinfra.Identity, error) {
		if authenticator == nil {
			return ratelimitinfra.Identity{}, errors.New("api-gateway authenticator is not configured")
		}
		auth, err := authenticator.Authenticate(rateLimitAuthRequestFromMetadata(ctx))
		if err != nil {
			return ratelimitinfra.Identity{}, err
		}
		return ratelimitinfra.Identity{TenantID: auth.TenantID, UserID: auth.UserID}, nil
	}
}

func rateLimitAuthRequestFromMetadata(ctx context.Context) *http.Request {
	query := url.Values{}
	if value := firstIncomingMetadata(ctx, "x-nexusim-gateway-token"); value != "" {
		query.Set("token", value)
	}
	if value := firstIncomingMetadata(ctx, "x-nexusim-tenant-id"); value != "" {
		query.Set("tenant_id", value)
	}
	if value := firstIncomingMetadata(ctx, "x-nexusim-user-id"); value != "" {
		query.Set("user_id", value)
	}
	if value := firstIncomingMetadata(ctx, "x-nexusim-device-id"); value != "" {
		query.Set("device_id", value)
	}
	if value := firstIncomingMetadata(ctx, "x-nexusim-trace-id"); value != "" {
		query.Set("trace_id", value)
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://api-gateway/rate-limit-auth?"+query.Encode(), nil)
	if authorization := firstIncomingMetadata(ctx, "authorization"); authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	return request
}

func firstIncomingMetadata(ctx context.Context, key string) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

type grpcClientTLSConfig struct {
	EnvPrefix      string
	CAFile         string
	ServerName     string
	ClientCertFile string
	ClientKeyFile  string
}

type redisClientConfig struct {
	Mode               string
	Addr               string
	SentinelAddrs      []string
	SentinelMasterName string
	Username           string
	Password           string
	DB                 int
	SentinelUsername   string
	SentinelPassword   string
}

func loadRateLimitRedisClientConfigFromEnv() redisClientConfig {
	return redisClientConfig{
		Mode:               envString("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_MODE", "single"),
		Addr:               envString("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_ADDR", "127.0.0.1:6379"),
		SentinelAddrs:      splitCSV(os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_SENTINEL_ADDRS")),
		SentinelMasterName: envString("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_SENTINEL_MASTER_NAME", ""),
		Username:           os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_USERNAME"),
		Password:           os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_PASSWORD"),
		DB:                 envInt("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_DB", 0),
		SentinelUsername:   os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_SENTINEL_USERNAME"),
		SentinelPassword:   os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_SENTINEL_PASSWORD"),
	}
}

func newRedisUniversalClient(config redisClientConfig) (redis.UniversalClient, error) {
	switch strings.ToLower(strings.TrimSpace(config.Mode)) {
	case "", "single":
		return redis.NewClient(&redis.Options{
			Addr:     config.Addr,
			Username: config.Username,
			Password: config.Password,
			DB:       config.DB,
		}), nil
	case "sentinel":
		if strings.TrimSpace(config.SentinelMasterName) == "" {
			return nil, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_SENTINEL_MASTER_NAME is required when redis sentinel mode is enabled")
		}
		if len(config.SentinelAddrs) == 0 {
			return nil, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_SENTINEL_ADDRS is required when redis sentinel mode is enabled")
		}
		return redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:       config.SentinelMasterName,
			SentinelAddrs:    config.SentinelAddrs,
			Username:         config.Username,
			Password:         config.Password,
			DB:               config.DB,
			SentinelUsername: config.SentinelUsername,
			SentinelPassword: config.SentinelPassword,
		}), nil
	default:
		return nil, errors.New("unsupported NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_MODE=" + config.Mode)
	}
}

func grpcClientTLSConfigFromEnv(envPrefix string) grpcClientTLSConfig {
	envPrefix = strings.TrimSpace(envPrefix)
	return grpcClientTLSConfig{
		EnvPrefix:      envPrefix,
		CAFile:         envString(envPrefix+"_CA_FILE", ""),
		ServerName:     envString(envPrefix+"_SERVER_NAME", ""),
		ClientCertFile: envString(envPrefix+"_CLIENT_CERT_FILE", ""),
		ClientKeyFile:  envString(envPrefix+"_CLIENT_KEY_FILE", ""),
	}
}

func (config grpcClientTLSConfig) Enabled() bool {
	return strings.TrimSpace(config.CAFile) != "" ||
		strings.TrimSpace(config.ServerName) != "" ||
		strings.TrimSpace(config.ClientCertFile) != "" ||
		strings.TrimSpace(config.ClientKeyFile) != ""
}

func (config grpcClientTLSConfig) ClientCertConfigured() bool {
	return strings.TrimSpace(config.ClientCertFile) != "" && strings.TrimSpace(config.ClientKeyFile) != ""
}

func grpcClientTLSCredentials(config grpcClientTLSConfig) (credentials.TransportCredentials, error) {
	caFile := strings.TrimSpace(config.CAFile)
	if caFile == "" {
		return nil, errors.New(config.EnvPrefix + "_CA_FILE is required when service TLS is configured")
	}
	clientCertFile := strings.TrimSpace(config.ClientCertFile)
	clientKeyFile := strings.TrimSpace(config.ClientKeyFile)
	if (clientCertFile == "") != (clientKeyFile == "") {
		return nil, errors.New(config.EnvPrefix + "_CLIENT_CERT_FILE and " + config.EnvPrefix + "_CLIENT_KEY_FILE must be configured together")
	}
	pemBytes, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pemBytes) {
		return nil, errors.New(config.EnvPrefix + "_CA_FILE does not contain a valid PEM certificate")
	}
	tlsConfig := &tls.Config{
		RootCAs:    roots,
		ServerName: strings.TrimSpace(config.ServerName),
		MinVersion: tls.VersionTLS12,
	}
	if clientCertFile != "" {
		cert, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	return credentials.NewTLS(tlsConfig), nil
}
