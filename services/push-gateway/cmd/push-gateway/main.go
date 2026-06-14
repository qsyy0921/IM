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

	wsapi "github.com/qsyy0921/IM/services/push-gateway/internal/api/websocket"
	"github.com/qsyy0921/IM/services/push-gateway/internal/app"
	authinfra "github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/auth"
	kafkainfra "github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/kafka"
	"github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/memory"
	monitoringinfra "github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/monitoring"
	redisroute "github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/redisroute"
	revocationinfra "github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/revocation"
	rpcinfra "github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/rpc"
	"github.com/qsyy0921/IM/services/push-gateway/internal/trigger/delivery"
	identitytrigger "github.com/qsyy0921/IM/services/push-gateway/internal/trigger/identity"
	"github.com/redis/go-redis/v9"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run() error {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_PUSH_GATEWAY_MODE"))
	switch mode {
	case "", "noop":
		log.Println("push-gateway runtime wiring is idle; set NEXUSIM_PUSH_GATEWAY_MODE=ws|delivery-consumer|identity-consumer|all")
		return nil
	case "ws":
		return runRuntime(true, false, false)
	case "delivery-consumer":
		return runRuntime(false, true, false)
	case "identity-consumer":
		return runRuntime(false, false, true)
	case "all":
		return runRuntime(true, true, true)
	default:
		return errors.New("unsupported NEXUSIM_PUSH_GATEWAY_MODE")
	}
}

func runRuntime(enableWS bool, enableDeliveryConsumer bool, enableIdentityConsumer bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	localRegistry := memory.NewRegistryWithConfig(memory.Config{
		ResumeBufferTTL: envDuration("NEXUSIM_PUSH_RESUME_BUFFER_TTL", 10*time.Minute),
	})
	registry := app.SessionRegistry(localRegistry)
	var revocationStore revocationinfra.Store = revocationinfra.NewMemoryStore()
	errs := make(chan error, 6)
	var closers []func() error

	var redisClient redis.UniversalClient
	var redisRegistry *redisroute.Registry
	var redisSubscriber *redisroute.Subscriber
	var authenticator *authinfra.Authenticator
	routeBackend := envString("NEXUSIM_PUSH_ROUTE_BACKEND", "memory")
	if routeBackend == "redis" {
		gatewayID := envString("NEXUSIM_PUSH_GATEWAY_ID", defaultGatewayID())
		var err error
		redisClient, err = newRedisUniversalClient(loadRedisClientConfigFromEnv())
		if err != nil {
			return err
		}
		if err := redisClient.Ping(ctx).Err(); err != nil {
			return err
		}
		routeConfig := redisroute.Config{
			GatewayID:             gatewayID,
			KeyPrefix:             envString("NEXUSIM_PUSH_REDIS_KEY_PREFIX", "nexusim:push"),
			RouteTTL:              envDuration("NEXUSIM_PUSH_ROUTE_TTL", 90*time.Second),
			ResumeTTL:             envDuration("NEXUSIM_PUSH_RESUME_BUFFER_TTL", 10*time.Minute),
			RenewFailureThreshold: envInt("NEXUSIM_PUSH_ROUTE_RENEW_FAILURES_BEFORE_EVICT", 3),
		}
		redisRegistry = redisroute.NewRegistry(localRegistry, redisClient, routeConfig)
		revocationStore = revocationinfra.NewRedisStore(redisClient, routeConfig.KeyPrefix)
		redisRegistry.StartCleanupLoop(ctx, envDurationAllowZero("NEXUSIM_PUSH_ROUTE_CLEANUP_INTERVAL", 30*time.Second))
		registry = redisRegistry
		closers = append(closers, redisClient.Close)
		if enableWS {
			redisSubscriber = redisroute.NewSubscriber(localRegistry, redisClient, redisroute.SubscriberConfig{
				GatewayID:    routeConfig.GatewayID,
				KeyPrefix:    routeConfig.KeyPrefix,
				ErrorBackoff: envDuration("NEXUSIM_PUSH_REDIS_SUBSCRIBER_ERROR_BACKOFF", 200*time.Millisecond),
				Logf:         log.Printf,
			})
			go func() {
				log.Printf("push-gateway redis route subscriber started for gateway_id=%s", gatewayID)
				errs <- redisSubscriber.Run(ctx)
			}()
		}
	} else if routeBackend != "memory" {
		return errors.New("unsupported NEXUSIM_PUSH_ROUTE_BACKEND")
	}

	monitoringHandler := monitoringinfra.NewHandler().
		WithMemoryMetrics(localRegistry.Metrics).
		WithRedisRegistryMetrics(func() redisroute.Metrics {
			return redisRouteRegistryMetrics(redisRegistry)
		}).
		WithRedisSubscriberMetrics(func() redisroute.Metrics {
			return redisRouteSubscriberMetrics(redisSubscriber)
		}).
		WithAuthJWKStats(func() *authinfra.JWKStats {
			return authenticatorJWKStats(authenticator)
		})
	if redisSubscriber != nil {
		monitoringHandler.WithRedisSubscriberWorkerStats(redisSubscriber.Snapshot)
	}

	var wsAddr string
	if enableWS {
		deliveryAddr := envString("NEXUSIM_DELIVERY_GRPC_ADDR", "127.0.0.1:10497")
		deliveryTLS, err := deliveryClientTLSConfigFromEnv()
		if err != nil {
			return err
		}
		deliveryClient, closeDelivery, err := rpcinfra.DialDeliveryClientWithConfig(ctx, rpcinfra.DeliveryClientDialConfig{
			Addr:    deliveryAddr,
			Timeout: envDuration("NEXUSIM_DELIVERY_GRPC_TIMEOUT", 500*time.Millisecond),
			TLS:     deliveryTLS,
		})
		if err != nil {
			return err
		}
		closers = append(closers, closeDelivery)
		jwksJSON, err := loadPushAuthJWKSetJSON()
		if err != nil {
			return err
		}
		authenticator, err = authinfra.NewAuthenticator(authinfra.Config{
			Mode:               authinfra.Mode(envString("NEXUSIM_PUSH_AUTH_MODE", "mock")),
			Secret:             os.Getenv("NEXUSIM_PUSH_AUTH_HMAC_SECRET"),
			PreviousSecrets:    splitCSV(os.Getenv("NEXUSIM_PUSH_AUTH_HMAC_PREVIOUS_SECRETS")),
			JWKSetJSON:         jwksJSON,
			JWKSetURL:          os.Getenv("NEXUSIM_PUSH_AUTH_JWKS_URL"),
			JWKRefreshInterval: envDuration("NEXUSIM_PUSH_AUTH_JWKS_REFRESH_INTERVAL", 5*time.Minute),
			TrustedIssuers:     splitCSV(os.Getenv("NEXUSIM_PUSH_AUTH_TRUSTED_ISSUERS")),
			Revocation:         revocationStore,
		})
		if err != nil {
			return err
		}
		defer authenticator.Close()
		server := wsapi.NewServer(
			app.NewConnectSessionUseCase(registry),
			app.NewDisconnectSessionUseCase(registry),
			app.NewHandleClientFrameUseCase(deliveryClient),
			wsapi.Config{
				QueueSize:         envInt("NEXUSIM_PUSH_SESSION_QUEUE_SIZE", 256),
				HeartbeatInterval: envDuration("NEXUSIM_PUSH_HEARTBEAT_INTERVAL", 30*time.Second),
				WriteTimeout:      envDuration("NEXUSIM_PUSH_WRITE_TIMEOUT", 2*time.Second),
				WriteDelay:        envDuration("NEXUSIM_PUSH_TEST_WRITE_DELAY", 0),
				Authenticator:     authenticator,
			},
		)
		mux := http.NewServeMux()
		mux.Handle("/healthz", monitoringHandler)
		mux.Handle("/readyz", monitoringHandler)
		mux.Handle("/debug/metrics", monitoringHandler)
		mux.Handle("/", server)
		wsAddr = envString("NEXUSIM_PUSH_WS_ADDR", "0.0.0.0:10496")
		if err := validatePushAuthListenerConfig(wsAddr, envString("NEXUSIM_PUSH_AUTH_MODE", "mock")); err != nil {
			return err
		}
		wsTLSConfig, _, err := pushWSTLSConfigFromEnv()
		if err != nil {
			return err
		}
		startHTTPServer(ctx, errs, "websocket", wsAddr, mux, wsTLSConfig)
	}

	if debugAddr := envString("NEXUSIM_PUSH_DEBUG_ADDR", ""); debugAddr != "" && debugAddr != wsAddr {
		mux := http.NewServeMux()
		mux.Handle("/healthz", monitoringHandler)
		mux.Handle("/readyz", monitoringHandler)
		mux.Handle("/debug/metrics", monitoringHandler)
		startHTTPServer(ctx, errs, "debug metrics", debugAddr, mux, nil)
	}

	if enableDeliveryConsumer {
		topic := envString("NEXUSIM_DELIVERY_EVENTS_TOPIC", delivery.TopicDeliveryEvents)
		if topic != delivery.TopicDeliveryEvents {
			return errors.New("push-gateway may only consume im.delivery.events")
		}
		consumer, err := kafkainfra.NewReaderConsumer(kafkainfra.ReaderConfig{
			Brokers: splitCSV(os.Getenv("NEXUSIM_KAFKA_BROKERS")),
			Topic:   topic,
			GroupID: envString("NEXUSIM_PUSH_CONSUMER_GROUP", "nexusim-push-gateway"),
		})
		if err != nil {
			return err
		}
		closers = append(closers, consumer.Close)
		worker := delivery.NewWorker(consumer, app.NewNotifyDeliveryUseCase(registry), delivery.Config{
			ErrorBackoff: envDuration("NEXUSIM_PUSH_DELIVERY_CONSUMER_ERROR_BACKOFF", time.Second),
			Logf:         log.Printf,
		})
		monitoringHandler.WithDeliveryConsumerStats(worker.Snapshot)
		go func() {
			log.Printf("push-gateway delivery consumer started")
			errs <- worker.Run(ctx)
		}()
	}
	if enableIdentityConsumer {
		topic := envString("NEXUSIM_IDENTITY_EVENTS_TOPIC", identitytrigger.TopicIdentityEvents)
		if topic != identitytrigger.TopicIdentityEvents {
			return errors.New("push-gateway may only consume im.identity.events")
		}
		consumer, err := kafkainfra.NewReaderConsumer(kafkainfra.ReaderConfig{
			Brokers: splitCSV(os.Getenv("NEXUSIM_KAFKA_BROKERS")),
			Topic:   topic,
			GroupID: envString("NEXUSIM_PUSH_IDENTITY_CONSUMER_GROUP", "nexusim-push-gateway-identity"),
		})
		if err != nil {
			return err
		}
		closers = append(closers, consumer.Close)
		worker := identitytrigger.NewWorker(consumer, revocationinfra.NewRecorder(revocationStore, registry), identitytrigger.Config{
			ErrorBackoff: envDuration("NEXUSIM_PUSH_IDENTITY_CONSUMER_ERROR_BACKOFF", time.Second),
			Logf:         log.Printf,
		})
		monitoringHandler.WithIdentityConsumerStats(worker.Snapshot)
		go func() {
			log.Printf("push-gateway identity consumer started")
			errs <- worker.Run(ctx)
		}()
	}

	var err error
	select {
	case err = <-errs:
	case <-ctx.Done():
		err = context.Canceled
	}
	stop()
	for _, closeFn := range closers {
		if closeErr := closeFn(); closeErr != nil && err == nil {
			err = closeErr
		}
	}
	return err
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

func loadRedisClientConfigFromEnv() redisClientConfig {
	return redisClientConfig{
		Mode:               envString("NEXUSIM_PUSH_REDIS_MODE", "single"),
		Addr:               envString("NEXUSIM_PUSH_REDIS_ADDR", "127.0.0.1:6379"),
		SentinelAddrs:      splitCSV(os.Getenv("NEXUSIM_PUSH_REDIS_SENTINEL_ADDRS")),
		SentinelMasterName: envString("NEXUSIM_PUSH_REDIS_SENTINEL_MASTER_NAME", ""),
		Username:           os.Getenv("NEXUSIM_PUSH_REDIS_USERNAME"),
		Password:           os.Getenv("NEXUSIM_PUSH_REDIS_PASSWORD"),
		DB:                 envIntAllowZero("NEXUSIM_PUSH_REDIS_DB", 0),
		SentinelUsername:   os.Getenv("NEXUSIM_PUSH_REDIS_SENTINEL_USERNAME"),
		SentinelPassword:   os.Getenv("NEXUSIM_PUSH_REDIS_SENTINEL_PASSWORD"),
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
			return nil, errors.New("NEXUSIM_PUSH_REDIS_SENTINEL_MASTER_NAME is required when NEXUSIM_PUSH_REDIS_MODE=sentinel")
		}
		if len(config.SentinelAddrs) == 0 {
			return nil, errors.New("NEXUSIM_PUSH_REDIS_SENTINEL_ADDRS is required when NEXUSIM_PUSH_REDIS_MODE=sentinel")
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
		return nil, errors.New("unsupported NEXUSIM_PUSH_REDIS_MODE=" + config.Mode)
	}
}

func startHTTPServer(ctx context.Context, errs chan<- error, name string, addr string, handler http.Handler, tlsConfig *tls.Config) {
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		TLSConfig:         tlsConfig,
	}
	go func() {
		log.Printf("push-gateway %s started on %s", name, httpServer.Addr)
		var err error
		if tlsConfig != nil {
			err = httpServer.ListenAndServeTLS("", "")
		} else {
			err = httpServer.ListenAndServe()
		}
		if errors.Is(err, http.ErrServerClosed) {
			err = context.Canceled
		}
		errs <- err
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
}

func redisRouteRegistryMetrics(registry *redisroute.Registry) redisroute.Metrics {
	if registry == nil {
		return redisroute.Metrics{}
	}
	return registry.Metrics()
}

func redisRouteSubscriberMetrics(subscriber *redisroute.Subscriber) redisroute.Metrics {
	if subscriber == nil {
		return redisroute.Metrics{}
	}
	return subscriber.Metrics()
}

func authenticatorJWKStats(authenticator *authinfra.Authenticator) *authinfra.JWKStats {
	stats := authenticator.JWKStats()
	if !stats.RemoteURLConfigured && stats.CachedKeyCount == 0 && stats.RefreshFailures == 0 {
		return nil
	}
	return &stats
}

func pushWSTLSConfigFromEnv() (*tls.Config, bool, error) {
	certFile := strings.TrimSpace(os.Getenv("NEXUSIM_PUSH_WS_TLS_CERT_FILE"))
	keyFile := strings.TrimSpace(os.Getenv("NEXUSIM_PUSH_WS_TLS_KEY_FILE"))
	clientCAFile := strings.TrimSpace(os.Getenv("NEXUSIM_PUSH_WS_TLS_CLIENT_CA_FILE"))
	allowedClientDNSNames := envStringSet("NEXUSIM_PUSH_WS_TLS_CLIENT_ALLOWED_DNS_NAMES", strings.ToLower)
	allowedClientURIs, err := envURIStringSet("NEXUSIM_PUSH_WS_TLS_CLIENT_ALLOWED_URIS")
	if err != nil {
		return nil, true, err
	}
	requireClientCert, requireClientCertConfigured, err := envOptionalBool("NEXUSIM_PUSH_WS_TLS_REQUIRE_CLIENT_CERT")
	if err != nil {
		return nil, true, err
	}
	hasClientAllowlist := len(allowedClientDNSNames) > 0 || len(allowedClientURIs) > 0
	requireClientCert = clientCAFile != "" || hasClientAllowlist || (requireClientCertConfigured && requireClientCert)
	if certFile == "" && keyFile == "" && clientCAFile == "" && !requireClientCert && !hasClientAllowlist {
		return nil, false, nil
	}
	if certFile == "" || keyFile == "" {
		return nil, true, errors.New("NEXUSIM_PUSH_WS_TLS_CERT_FILE and NEXUSIM_PUSH_WS_TLS_KEY_FILE must be configured together")
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
			return nil, true, errors.New("NEXUSIM_PUSH_WS_TLS_CLIENT_CA_FILE is required when client certificates are required")
		}
		pemBytes, err := os.ReadFile(clientCAFile)
		if err != nil {
			return nil, true, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, true, errors.New("NEXUSIM_PUSH_WS_TLS_CLIENT_CA_FILE does not contain a valid PEM certificate")
		}
		tlsConfig.ClientCAs = pool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		if hasClientAllowlist {
			tlsConfig.VerifyConnection = verifyAllowedPushWSClient(allowedClientDNSNames, allowedClientURIs)
		}
	}
	return tlsConfig, true, nil
}

func verifyAllowedPushWSClient(allowedDNSNames map[string]struct{}, allowedURIs map[string]struct{}) func(tls.ConnectionState) error {
	return func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("push websocket client certificate is required")
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
		return errors.New("push websocket client certificate identity is not allowed")
	}
}

func deliveryClientTLSConfigFromEnv() (rpcinfra.DeliveryClientTLSConfig, error) {
	config := rpcinfra.DeliveryClientTLSConfig{
		CAFile:         envString("NEXUSIM_DELIVERY_SERVICE_TLS_CA_FILE", ""),
		ServerName:     envString("NEXUSIM_DELIVERY_SERVICE_TLS_SERVER_NAME", ""),
		ClientCertFile: envString("NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_CERT_FILE", ""),
		ClientKeyFile:  envString("NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_KEY_FILE", ""),
	}
	if !config.Enabled() {
		return config, nil
	}
	if strings.TrimSpace(config.CAFile) == "" {
		return config, errors.New("NEXUSIM_DELIVERY_SERVICE_TLS_CA_FILE is required when delivery-service TLS is configured")
	}
	if (strings.TrimSpace(config.ClientCertFile) == "") != (strings.TrimSpace(config.ClientKeyFile) == "") {
		return config, errors.New("NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_CERT_FILE and NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_KEY_FILE must be configured together")
	}
	return config, nil
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

func envDurationAllowZero(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	if value == "0" {
		return 0
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func defaultGatewayID() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "gateway"
	}
	return hostname + "-" + strconv.Itoa(os.Getpid())
}

func loadPushAuthJWKSetJSON() (string, error) {
	if value := strings.TrimSpace(os.Getenv("NEXUSIM_PUSH_AUTH_JWKS_JSON")); value != "" {
		return value, nil
	}
	path := strings.TrimSpace(os.Getenv("NEXUSIM_PUSH_AUTH_JWKS_FILE"))
	if path == "" {
		return "", nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func validatePushAuthListenerConfig(listenAddr string, authMode string) error {
	if !usesMockPushAuth(authMode) {
		return nil
	}
	if listenerAddrTrustedWithoutMTLS(listenAddr) {
		return nil
	}
	return errors.New("push-gateway uses mock auth on non-private websocket address")
}

func usesMockPushAuth(authMode string) bool {
	return strings.EqualFold(strings.TrimSpace(authMode), "mock")
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
