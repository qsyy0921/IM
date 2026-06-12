package main

import (
	"context"
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
	monitoringinfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/monitoring"
	postgresinfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/postgres"
	tokeninfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/token"
	"google.golang.org/grpc"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run() error {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_SERVICE_MODE"))
	switch mode {
	case "", "noop":
		log.Println("identity-service runtime wiring is idle; set NEXUSIM_IDENTITY_SERVICE_MODE=grpc")
		return nil
	case "grpc":
		return runGRPC()
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
	grpcMetrics := monitoringinfra.NewGRPCMetrics()
	stopDebug, err := startDebugServer(ctx, identityDebugAddr(), monitoringinfra.NewHandler(pool, grpcMetrics))
	if err != nil {
		return err
	}
	defer stopDebug()

	signer, err := tokeninfra.NewHMACSigner(envString("NEXUSIM_IDENTITY_GATEWAY_TOKEN_SECRET", envString("NEXUSIM_PUSH_AUTH_HMAC_SECRET", "")))
	if err != nil {
		return err
	}

	addr := envString("NEXUSIM_IDENTITY_GRPC_ADDR", "0.0.0.0:10600")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	repository := postgresinfra.NewRepository(pool)
	server := grpc.NewServer(grpc.UnaryInterceptor(grpcMetrics.UnaryServerInterceptor(log.Default())))
	identitygrpc.Register(server, identitygrpc.NewServer(
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
