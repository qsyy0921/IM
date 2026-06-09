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
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	grpcapi "github.com/qsyy0921/IM/services/conversation-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/conversation-service/internal/app"
	postgresinfra "github.com/qsyy0921/IM/services/conversation-service/internal/infrastructure/postgres"
	"github.com/qsyy0921/IM/services/conversation-service/internal/trigger/memberchange"
	"google.golang.org/grpc"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run() error {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_CONVERSATION_SERVICE_MODE"))
	switch mode {
	case "", "noop":
		log.Println("conversation-service runtime wiring is idle; set NEXUSIM_CONVERSATION_SERVICE_MODE=grpc or member-change-worker")
		return nil
	case "grpc":
		return runGRPCServer()
	case "member-change-worker":
		return runMemberChangeWorker()
	default:
		return errors.New("unsupported NEXUSIM_CONVERSATION_SERVICE_MODE")
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

	listenAddr := envString("NEXUSIM_CONVERSATION_GRPC_ADDR", "0.0.0.0:10496")
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	server := grpc.NewServer()
	repository := postgresinfra.NewRepository(pool)
	grpcapi.Register(
		server,
		grpcapi.NewServer(
			app.NewGetSendContextUseCase(repository),
			grpcapi.WithCreateMemberChange(app.NewCreateMemberChangeUseCase(repository)),
			grpcapi.WithGetMemberChange(app.NewGetMemberChangeUseCase(repository)),
		),
	)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("conversation-service gRPC server started on %s", listenAddr)

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

func runMemberChangeWorker() error {
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

	repository := postgresinfra.NewRepository(pool)
	useCase := app.NewMarkPublishedMemberChangesUseCase(
		repository,
		envInt("NEXUSIM_MEMBER_CHANGE_PROGRESS_BATCH_SIZE", 100),
	)
	pollInterval := envDuration("NEXUSIM_MEMBER_CHANGE_PROGRESS_POLL_INTERVAL", time.Second)
	worker := memberchange.NewProgressWorker(
		useCase,
		memberchange.ProgressConfig{PollInterval: pollInterval},
	)
	log.Printf(
		"conversation-service member change progress worker started batch_size=%d poll_interval=%s",
		envInt("NEXUSIM_MEMBER_CHANGE_PROGRESS_BATCH_SIZE", 100),
		pollInterval,
	)
	return worker.Run(ctx)
}

func openPGPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if maxConns := envInt("NEXUSIM_CONVERSATION_PG_MAX_CONNS", 0); maxConns > 0 {
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
