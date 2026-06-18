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

	retrievalgrpc "github.com/qsyy0921/IM/services/retrieval-gateway/internal/api/grpc"
	"github.com/qsyy0921/IM/services/retrieval-gateway/internal/app"
	rpcinfra "github.com/qsyy0921/IM/services/retrieval-gateway/internal/infrastructure/rpc"
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
	mode := retrievalGatewayModeFromEnv()
	if err := validateRetrievalGatewayMode(mode); err != nil {
		return err
	}
	switch mode {
	case "noop":
		return runNoop(ctx)
	case "grpc":
		return runGRPC(ctx)
	default:
		return fmt.Errorf("unsupported NEXUSIM_RETRIEVAL_GATEWAY_MODE %q", mode)
	}
}

func runNoop(ctx context.Context) error {
	debugAddr, err := retrievalDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr)
	if err != nil {
		return err
	}
	defer stopDebug()

	log.Println("retrieval-gateway noop mode: set NEXUSIM_RETRIEVAL_GATEWAY_MODE=grpc to start runtime role")
	<-ctx.Done()
	return nil
}

func runGRPC(ctx context.Context) error {
	debugAddr, err := retrievalDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr)
	if err != nil {
		return err
	}
	defer stopDebug()

	timeout := envDuration("NEXUSIM_RETRIEVAL_DEPENDENCY_TIMEOUT", 500*time.Millisecond)
	searchClient, closeSearch, err := rpcinfra.DialSearchClient(ctx, envString("NEXUSIM_SEARCH_GRPC_ADDR", "127.0.0.1:10570"), timeout)
	if err != nil {
		return err
	}
	defer closeSearch()
	memoryClient, closeMemory, err := rpcinfra.DialMemoryClient(ctx, envString("NEXUSIM_MEMORY_GRPC_ADDR", "127.0.0.1:10580"), timeout)
	if err != nil {
		return err
	}
	defer closeMemory()

	addr := envString("NEXUSIM_RETRIEVAL_GRPC_ADDR", "127.0.0.1:10590")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	server := grpc.NewServer()
	retrievalgrpc.Register(server, retrievalgrpc.NewServer(app.NewRetrieveEvidenceUseCase(searchClient, memoryClient)))

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("retrieval-gateway grpc listening on %s", addr)

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

func retrievalGatewayModeFromEnv() string {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_RETRIEVAL_GATEWAY_MODE"))
	if mode == "" {
		mode = "noop"
	}
	return mode
}

func validateRetrievalGatewayMode(mode string) error {
	switch mode {
	case "noop", "grpc":
		return nil
	default:
		return fmt.Errorf("unsupported NEXUSIM_RETRIEVAL_GATEWAY_MODE %q", mode)
	}
}

func retrievalDebugAddr() string {
	if addr := strings.TrimSpace(os.Getenv("NEXUSIM_RETRIEVAL_DEBUG_ADDR")); addr != "" {
		return addr
	}
	return strings.TrimSpace(os.Getenv("NEXUSIM_DEBUG_ADDR"))
}

func retrievalDebugAddrFromEnv() (string, error) {
	addr := retrievalDebugAddr()
	allowPublic, _, err := envOptionalBool("NEXUSIM_RETRIEVAL_DEBUG_ALLOW_PUBLIC")
	if err != nil {
		return "", err
	}
	return addr, validateRetrievalDebugListenerConfig(addr, allowPublic)
}

func validateRetrievalDebugListenerConfig(addr string, allowPublic bool) error {
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
	return errors.New("retrieval-gateway debug listener address is non-private; set NEXUSIM_RETRIEVAL_DEBUG_ALLOW_PUBLIC=true to allow")
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
			log.Printf("retrieval-gateway debug server stopped: %v", err)
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
		_, _ = writer.Write([]byte("nexusim_retrieval_gateway_info 1\n"))
	}
	mux.HandleFunc("/debug/metrics", metricsHandler)
	mux.HandleFunc("/metrics", metricsHandler)
	return mux
}
