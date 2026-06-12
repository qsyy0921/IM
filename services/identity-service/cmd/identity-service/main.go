package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	identitygrpc "github.com/qsyy0921/IM/services/identity-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/identity-service/internal/app"
	credentialinfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/credential"
	kafkainfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/kafka"
	monitoringinfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/monitoring"
	postgresinfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/postgres"
	tokeninfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/token"
	"github.com/qsyy0921/IM/services/identity-service/internal/trigger/outbox"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
	"google.golang.org/grpc"
)

type gatewayTokenSigner interface {
	SignGatewayToken(types.TokenClaims) (string, error)
	JWKSet() tokeninfra.JWKSet
}

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run() error {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_SERVICE_MODE"))
	switch mode {
	case "", "noop":
		log.Println("identity-service runtime wiring is idle; set NEXUSIM_IDENTITY_SERVICE_MODE=grpc or outbox-relay")
		return nil
	case "grpc":
		return runGRPC()
	case "outbox-relay":
		return runOutboxRelay()
	default:
		return errors.New("unsupported NEXUSIM_IDENTITY_SERVICE_MODE")
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
	signer, err := newGatewayTokenSigner()
	if err != nil {
		return err
	}
	refreshTokens := tokeninfra.NewRefreshTokenCodec()
	passwords := credentialinfra.NewPBKDF2Hasher(envInt("NEXUSIM_IDENTITY_PASSWORD_PBKDF2_ITERATIONS", 0))
	grpcMetrics := monitoringinfra.NewGRPCMetrics()
	jwkSet, err := gatewayTokenJWKSetWithAdditionalKeys(signer.JWKSet())
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, identityDebugAddr(), monitoringinfra.NewHandler(pool, grpcMetrics).WithJWKSet(jwkSet))
	if err != nil {
		return err
	}
	defer stopDebug()

	addr := envString("NEXUSIM_IDENTITY_GRPC_ADDR", "0.0.0.0:10600")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	repository := postgresinfra.NewRepository(pool)
	server, err := newGRPCServer(grpcMetrics)
	if err != nil {
		return err
	}
	identitygrpc.Register(server, identitygrpc.NewServer(
		app.NewRegisterUserUseCase(repository, passwords),
		app.NewLoginUseCase(
			repository,
			signer,
			passwords,
			refreshTokens,
			app.WithLoginRiskPolicy(app.LoginRiskPolicy{
				MaxFailedAttempts: envInt("NEXUSIM_IDENTITY_LOGIN_MAX_FAILED_ATTEMPTS", app.DefaultLoginMaxFailedAttempts),
				FailureWindow:     envDuration("NEXUSIM_IDENTITY_LOGIN_FAILURE_WINDOW", app.DefaultLoginFailureWindow),
				LockDuration:      envDuration("NEXUSIM_IDENTITY_LOGIN_LOCK_DURATION", app.DefaultLoginLockDuration),
			}),
		),
		app.NewRefreshGatewayTokenUseCase(repository, signer, refreshTokens),
		app.NewIssueGatewayTokenUseCase(repository, signer),
		app.NewRevokeDeviceUseCase(repository),
		app.NewRevokeSessionUseCase(repository),
		app.NewGetDeviceStateUseCase(repository),
	))

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("identity-service grpc listening on %s", addr)

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

func newGatewayTokenSigner() (gatewayTokenSigner, error) {
	secret := envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_SECRET", envString("NEXUSIM_PUSH_AUTH_HMAC_SECRET", ""))
	switch strings.ToLower(envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_FORMAT", "legacy")) {
	case "legacy", "hmac", "custom":
		return tokeninfra.NewHMACSigner(secret)
	case "jwt", "jwt-hs256", "hs256":
		return tokeninfra.NewJWTSigner(
			secret,
			envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_KEY_ID", ""),
			envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ISSUER", ""),
		)
	case "jwt-rs256", "rs256":
		privateKeyPEM, err := loadRSAPrivateKeyPEM()
		if err != nil {
			return nil, err
		}
		return tokeninfra.NewRS256SignerFromPEM(
			privateKeyPEM,
			envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_KEY_ID", ""),
			envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ISSUER", ""),
		)
	default:
		return nil, errors.New("unsupported NEXUSIM_IDENTITY_GATEWAY_TOKEN_FORMAT")
	}
}

func loadRSAPrivateKeyPEM() (string, error) {
	if pemValue := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RSA_PRIVATE_KEY_PEM")); pemValue != "" {
		return pemValue, nil
	}
	path := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RSA_PRIVATE_KEY_FILE"))
	if path == "" {
		return "", errors.New("NEXUSIM_IDENTITY_GATEWAY_TOKEN_RSA_PRIVATE_KEY_PEM or NEXUSIM_IDENTITY_GATEWAY_TOKEN_RSA_PRIVATE_KEY_FILE is required for RS256")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func gatewayTokenJWKSetWithAdditionalKeys(base tokeninfra.JWKSet) (tokeninfra.JWKSet, error) {
	additional, err := loadAdditionalGatewayTokenJWKSet()
	if err != nil {
		return tokeninfra.JWKSet{}, err
	}
	if len(additional.Keys) == 0 {
		return base, nil
	}
	result := tokeninfra.JWKSet{Keys: make([]tokeninfra.JWK, 0, len(base.Keys)+len(additional.Keys))}
	seen := make(map[string]struct{}, len(base.Keys)+len(additional.Keys))
	appendKey := func(key tokeninfra.JWK) {
		keyID := strings.TrimSpace(key.KeyID)
		if keyID != "" {
			if _, ok := seen[keyID]; ok {
				return
			}
			seen[keyID] = struct{}{}
		}
		result.Keys = append(result.Keys, key)
	}
	for _, key := range base.Keys {
		appendKey(key)
	}
	for _, key := range additional.Keys {
		appendKey(key)
	}
	return result, nil
}

func loadAdditionalGatewayTokenJWKSet() (tokeninfra.JWKSet, error) {
	raw := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_JSON"))
	if raw == "" {
		path := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_FILE"))
		if path == "" {
			return tokeninfra.JWKSet{}, nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return tokeninfra.JWKSet{}, err
		}
		raw = strings.TrimSpace(string(content))
	}
	if raw == "" {
		return tokeninfra.JWKSet{}, nil
	}
	var set tokeninfra.JWKSet
	if err := json.Unmarshal([]byte(raw), &set); err != nil {
		return tokeninfra.JWKSet{}, err
	}
	return set, nil
}

func runOutboxRelay() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	stopDebug, err := startDebugServer(ctx, identityDebugAddr(), monitoringinfra.NewHandler(pool))
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

	topic := envString("NEXUSIM_IDENTITY_EVENTS_TOPIC", outbox.TopicIdentityEvents)
	relay := outbox.NewRelay(
		postgresinfra.NewOutboxStore(pool),
		producer,
		outbox.Config{
			Topic:          topic,
			BatchSize:      envInt("NEXUSIM_IDENTITY_OUTBOX_BATCH_SIZE", 500),
			PollInterval:   envDuration("NEXUSIM_IDENTITY_OUTBOX_POLL_INTERVAL", time.Second),
			MaxAttempts:    envInt("NEXUSIM_IDENTITY_OUTBOX_MAX_ATTEMPTS", 5),
			RetryBaseDelay: envDuration("NEXUSIM_IDENTITY_OUTBOX_RETRY_BASE_DELAY", time.Second),
		},
	)
	log.Printf("identity-service outbox relay started topic=%s", topic)
	return relay.Run(ctx)
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
			log.Printf("identity-service debug server stopped with error: %v", err)
		}
	}()
	log.Printf("identity-service debug server started on %s", addr)
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
	if maxConns := envInt("NEXUSIM_IDENTITY_PG_MAX_CONNS", 0); maxConns > 0 {
		config.MaxConns = int32(maxConns)
	}
	return pgxpool.NewWithConfig(ctx, config)
}

func identityDebugAddr() string {
	return envString("NEXUSIM_IDENTITY_DEBUG_ADDR", envString("NEXUSIM_DEBUG_ADDR", ""))
}

func newGRPCServer(grpcMetrics *monitoringinfra.GRPCMetrics) (*grpc.Server, error) {
	interceptors := make([]grpc.UnaryServerInterceptor, 0, 2)
	if grpcMetrics != nil {
		interceptors = append(interceptors, grpcMetrics.UnaryServerInterceptor(log.Default()))
	}
	switch strings.ToLower(envString("NEXUSIM_IDENTITY_ADMIN_AUTH_MODE", "body")) {
	case "body", "request", "legacy":
	case "metadata", "verified-metadata":
		interceptors = append(interceptors, identitygrpc.VerifiedAdminUnaryInterceptor(true))
	default:
		return nil, errors.New("unsupported NEXUSIM_IDENTITY_ADMIN_AUTH_MODE")
	}
	if len(interceptors) == 0 {
		return grpc.NewServer(), nil
	}
	return grpc.NewServer(grpc.ChainUnaryInterceptor(interceptors...)), nil
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
