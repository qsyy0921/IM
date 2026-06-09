package main

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	grpcapi "github.com/qsyy0921/IM/services/delivery-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/delivery-service/internal/app"
	postgresinfra "github.com/qsyy0921/IM/services/delivery-service/internal/infrastructure/postgres"
	"google.golang.org/grpc"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run() error {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_SERVICE_MODE"))
	switch mode {
	case "", "noop":
		log.Println("delivery-service runtime wiring is idle; set NEXUSIM_DELIVERY_SERVICE_MODE=grpc")
		return nil
	case "grpc":
		return runGRPCServer()
	default:
		return errors.New("unsupported NEXUSIM_DELIVERY_SERVICE_MODE")
	}
}

func runGRPCServer() error {
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

	listenAddr := envString("NEXUSIM_DELIVERY_GRPC_ADDR", "0.0.0.0:10497")
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	server := grpc.NewServer()
	repository := postgresinfra.NewRepository(pool)
	grpcapi.Register(
		server,
		grpcapi.NewServer(
			app.NewPullInboxUseCase(repository),
			app.NewAckDeliveryUseCase(repository),
		),
	)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("delivery-service gRPC server started on %s", listenAddr)

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
