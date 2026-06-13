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

	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
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

	authenticator, err := newAuthenticatorFromEnv()
	if err != nil {
		return err
	}
	defer authenticator.Close()
	grpcMetrics := monitoringinfra.NewGRPCMetrics()
	rateLimiter, closeRateLimiter, err := newRateLimiterFromEnv(ctx)
	if err != nil {
		return err
	}
	defer closeRateLimiter()
	stopDebug, err := startDebugServer(ctx, apiGatewayDebugAddr(), monitoringinfra.NewHandler(grpcMetrics).
		WithAuthJWKStats(authenticator.JWKStats).
		WithRateLimitStats(rateLimiter.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()

	conversationConn, err := dialBackend(
		envString("NEXUSIM_API_GATEWAY_CONVERSATION_ADDR", "127.0.0.1:10496"),
		grpcClientTLSConfigFromEnv("NEXUSIM_API_GATEWAY_CONVERSATION_TLS"),
	)
	if err != nil {
		return err
	}
	defer conversationConn.Close()
	messageConn, err := dialBackend(
		envString("NEXUSIM_API_GATEWAY_MESSAGE_ADDR", "127.0.0.1:10495"),
		grpcClientTLSConfigFromEnv("NEXUSIM_API_GATEWAY_MESSAGE_TLS"),
	)
	if err != nil {
		return err
	}
	defer messageConn.Close()
	deliveryConn, err := dialBackend(
		envString("NEXUSIM_API_GATEWAY_DELIVERY_ADDR", "127.0.0.1:10497"),
		grpcClientTLSConfigFromEnv("NEXUSIM_API_GATEWAY_DELIVERY_TLS"),
	)
	if err != nil {
		return err
	}
	defer deliveryConn.Close()
	receiptConn, err := dialBackend(
		envString("NEXUSIM_API_GATEWAY_RECEIPT_ADDR", "127.0.0.1:10499"),
		grpcClientTLSConfigFromEnv("NEXUSIM_API_GATEWAY_RECEIPT_TLS"),
	)
	if err != nil {
		return err
	}
	defer receiptConn.Close()

	gateway := apigrpc.NewServer(apigrpc.Config{
		Authenticator: authenticator,
		Conversation:  conversationv1.NewConversationServiceClient(conversationConn),
		Message:       messagev1.NewMessageServiceClient(messageConn),
		Delivery:      deliveryv1.NewDeliveryServiceClient(deliveryConn),
		Receipt:       receiptv1.NewReceiptServiceClient(receiptConn),
	})
	serverOptions := []grpcgo.ServerOption{grpcgo.ChainUnaryInterceptor(
		grpcMetrics.UnaryServerInterceptor(log.Default()),
		rateLimiter.UnaryServerInterceptor(),
	)}
	if creds, ok, err := loadAPIGatewayGRPCCredentialsFromEnv(); err != nil {
		return err
	} else if ok {
		serverOptions = append(serverOptions, grpcgo.Creds(creds))
	}
	server := grpcgo.NewServer(serverOptions...)
	apigrpc.Register(server, gateway)

	addr := envString("NEXUSIM_API_GATEWAY_GRPC_ADDR", "0.0.0.0:12000")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		server.GracefulStop()
	}()
	log.Printf("api-gateway gRPC listening on %s", addr)
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

func dialBackend(addr string, tlsConfig grpcClientTLSConfig) (*grpcgo.ClientConn, error) {
	transportCredentials := grpcgo.WithTransportCredentials(insecure.NewCredentials())
	if tlsConfig.Enabled() {
		creds, err := grpcClientTLSCredentials(tlsConfig)
		if err != nil {
			return nil, err
		}
		transportCredentials = grpcgo.WithTransportCredentials(creds)
	}
	return grpcgo.NewClient("passthrough:///"+addr, transportCredentials)
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

func loadAPIGatewayGRPCCredentialsFromEnv() (credentials.TransportCredentials, bool, error) {
	tlsConfig, ok, err := apiGatewayGRPCTLSConfigFromEnv()
	if err != nil || !ok {
		return nil, ok, err
	}
	return credentials.NewTLS(tlsConfig), true, nil
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

func newRateLimiterFromEnv(ctx context.Context) (*ratelimitinfra.Limiter, func() error, error) {
	enabled, _, err := envOptionalBool("NEXUSIM_API_GATEWAY_RATE_LIMIT_ENABLED")
	if err != nil {
		return nil, nil, err
	}
	rps := envFloat64("NEXUSIM_API_GATEWAY_RATE_LIMIT_RPS", 0)
	backend := strings.ToLower(strings.TrimSpace(envString("NEXUSIM_API_GATEWAY_RATE_LIMIT_BACKEND", "local")))
	config := ratelimitinfra.Config{
		Enabled:           enabled,
		Backend:           backend,
		RequestsPerSecond: rps,
		Burst:             envInt("NEXUSIM_API_GATEWAY_RATE_LIMIT_BURST", int(rps)),
		MaxKeys:           envInt("NEXUSIM_API_GATEWAY_RATE_LIMIT_MAX_KEYS", 10000),
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
			_ = client.Close()
			return nil, nil, err
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
		return limiter, client.Close, nil
	}
	limiter, err := ratelimitinfra.New(config)
	if err != nil {
		return nil, nil, err
	}
	return limiter, func() error { return nil }, nil
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
