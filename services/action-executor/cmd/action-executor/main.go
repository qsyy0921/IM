package main

import (
	"context"
	"errors"
	"fmt"
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
	actiongrpc "github.com/qsyy0921/IM/services/action-executor/internal/api/grpc"
	"github.com/qsyy0921/IM/services/action-executor/internal/app"
	postgresinfra "github.com/qsyy0921/IM/services/action-executor/internal/infrastructure/postgres"
	rpcinfra "github.com/qsyy0921/IM/services/action-executor/internal/infrastructure/rpc"
	"google.golang.org/grpc"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	mode := actionExecutorModeFromEnv()
	if err := validateActionExecutorMode(mode); err != nil {
		return err
	}
	switch mode {
	case "noop":
		return runNoop(ctx)
	case "grpc":
		return runGRPC(ctx)
	default:
		return fmt.Errorf("unsupported NEXUSIM_ACTION_EXECUTOR_MODE %q", mode)
	}
}

func runNoop(ctx context.Context) error {
	debugAddr, err := actionExecutorDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr)
	if err != nil {
		return err
	}
	defer stopDebug()

	log.Println("action-executor noop mode: set NEXUSIM_ACTION_EXECUTOR_MODE=grpc to start runtime role")
	<-ctx.Done()
	return nil
}

func runGRPC(ctx context.Context) error {
	debugAddr, err := actionExecutorDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr)
	if err != nil {
		return err
	}
	defer stopDebug()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	timeout := envDuration("NEXUSIM_ACTION_EXECUTOR_DEPENDENCY_TIMEOUT", 500*time.Millisecond)
	skillClient, closeSkill, err := rpcinfra.DialSkillRegistryClient(
		ctx,
		envString("NEXUSIM_SKILL_REGISTRY_GRPC_ADDR", "127.0.0.1:10640"),
		timeout,
	)
	if err != nil {
		return err
	}
	defer closeSkill()
	policyClient, closePolicy, err := rpcinfra.DialPolicyClient(
		ctx,
		envString("NEXUSIM_POLICY_GRPC_ADDR", "127.0.0.1:10800"),
		timeout,
	)
	if err != nil {
		return err
	}
	defer closePolicy()

	addr := envString("NEXUSIM_ACTION_EXECUTOR_GRPC_ADDR", "127.0.0.1:10660")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	repository := postgresinfra.NewRepository(pool)
	server := grpc.NewServer()
	actiongrpc.Register(server, actiongrpc.NewServer(app.NewExecuteApprovedActionUseCase(skillClient, policyClient, repository)))

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("action-executor grpc listening on %s", addr)

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

func actionExecutorModeFromEnv() string {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_ACTION_EXECUTOR_MODE"))
	if mode == "" {
		mode = "noop"
	}
	return mode
}

func validateActionExecutorMode(mode string) error {
	switch mode {
	case "noop", "grpc":
		return nil
	default:
		return fmt.Errorf("unsupported NEXUSIM_ACTION_EXECUTOR_MODE %q", mode)
	}
}

func actionExecutorDebugAddr() string {
	if addr := strings.TrimSpace(os.Getenv("NEXUSIM_ACTION_EXECUTOR_DEBUG_ADDR")); addr != "" {
		return addr
	}
	return strings.TrimSpace(os.Getenv("NEXUSIM_DEBUG_ADDR"))
}

func actionExecutorDebugAddrFromEnv() (string, error) {
	addr := actionExecutorDebugAddr()
	allowPublic, _, err := envOptionalBool("NEXUSIM_ACTION_EXECUTOR_DEBUG_ALLOW_PUBLIC")
	if err != nil {
		return "", err
	}
	return addr, validateActionExecutorDebugListenerConfig(addr, allowPublic)
}

func validateActionExecutorDebugListenerConfig(addr string, allowPublic bool) error {
	if strings.TrimSpace(addr) == "" {
		return nil
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
		return nil
	}
	if allowPublic {
		return nil
	}
	return errors.New("action-executor debug listener address is non-private; set NEXUSIM_ACTION_EXECUTOR_DEBUG_ALLOW_PUBLIC=true to allow")
}

func envOptionalBool(name string) (bool, bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return false, false, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, true, err
	}
	return value, true, nil
}

func envString(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
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
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return pgxpool.NewWithConfig(connectCtx, config)
}

func startDebugServer(ctx context.Context, addr string) (func(), error) {
	if strings.TrimSpace(addr) == "" {
		return func() {}, nil
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	server := &http.Server{
		Handler:           newDebugHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("action-executor debug server stopped: %v", err)
		}
	}()
	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}, nil
}

func newDebugHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/readyz", func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("ok\n"))
	})
	metricsHandler := func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = writer.Write([]byte("nexusim_action_executor_service_info 1\n"))
	}
	mux.HandleFunc("/debug/metrics", metricsHandler)
	mux.HandleFunc("/metrics", metricsHandler)
	return mux
}
