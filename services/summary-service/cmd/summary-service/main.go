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

	"github.com/qsyy0921/IM/internal/ai/llmboundary"
	"github.com/qsyy0921/IM/internal/ai/pythonworker"
	summarygrpc "github.com/qsyy0921/IM/services/summary-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/summary-service/internal/app"
	rpcinfra "github.com/qsyy0921/IM/services/summary-service/internal/infrastructure/rpc"
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
	mode := summaryServiceModeFromEnv()
	if err := validateSummaryServiceMode(mode); err != nil {
		return err
	}
	switch mode {
	case "noop":
		return runNoop(ctx)
	case "grpc":
		return runGRPC(ctx)
	default:
		return fmt.Errorf("unsupported NEXUSIM_SUMMARY_SERVICE_MODE %q", mode)
	}
}

func runNoop(ctx context.Context) error {
	debugAddr, err := summaryDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr)
	if err != nil {
		return err
	}
	defer stopDebug()

	log.Println("summary-service noop mode: set NEXUSIM_SUMMARY_SERVICE_MODE=grpc to start runtime role")
	<-ctx.Done()
	return nil
}

func runGRPC(ctx context.Context) error {
	debugAddr, err := summaryDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr)
	if err != nil {
		return err
	}
	defer stopDebug()

	timeout := envDuration("NEXUSIM_SUMMARY_DEPENDENCY_TIMEOUT", 500*time.Millisecond)
	retrievalClient, closeRetrieval, err := rpcinfra.DialRetrievalClient(ctx, envString("NEXUSIM_RETRIEVAL_GRPC_ADDR", "127.0.0.1:10590"), timeout)
	if err != nil {
		return err
	}
	defer closeRetrieval()
	provider, err := summaryProviderFromEnv()
	if err != nil {
		return err
	}

	addr := envString("NEXUSIM_SUMMARY_GRPC_ADDR", "127.0.0.1:10620")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	server := grpc.NewServer()
	summarygrpc.Register(server, summarygrpc.NewServer(app.NewGenerateConversationSummaryUseCaseWithProvider(retrievalClient, provider)))

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("summary-service grpc listening on %s", addr)

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

func summaryServiceModeFromEnv() string {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_SUMMARY_SERVICE_MODE"))
	if mode == "" {
		mode = "noop"
	}
	return mode
}

func validateSummaryServiceMode(mode string) error {
	switch mode {
	case "noop", "grpc":
		return nil
	default:
		return fmt.Errorf("unsupported NEXUSIM_SUMMARY_SERVICE_MODE %q", mode)
	}
}

func summaryDebugAddr() string {
	if addr := strings.TrimSpace(os.Getenv("NEXUSIM_SUMMARY_DEBUG_ADDR")); addr != "" {
		return addr
	}
	return strings.TrimSpace(os.Getenv("NEXUSIM_DEBUG_ADDR"))
}

func summaryDebugAddrFromEnv() (string, error) {
	addr := summaryDebugAddr()
	allowPublic, _, err := envOptionalBool("NEXUSIM_SUMMARY_DEBUG_ALLOW_PUBLIC")
	if err != nil {
		return "", err
	}
	return addr, validateSummaryDebugListenerConfig(addr, allowPublic)
}

func validateSummaryDebugListenerConfig(addr string, allowPublic bool) error {
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
	return errors.New("summary-service debug listener address is non-private; set NEXUSIM_SUMMARY_DEBUG_ALLOW_PUBLIC=true to allow")
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

func envString(name string, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
	}
	return value
}

func envDuration(name string, defaultValue time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func envInt(name string, defaultValue int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func summaryProviderFromEnv() (app.SummaryProvider, error) {
	mode := strings.ToLower(envString("NEXUSIM_SUMMARY_PROVIDER_MODE", "extractive"))
	switch mode {
	case "extractive":
		return app.ExtractiveSummaryProvider{}, nil
	case "external-http":
		client, err := llmboundary.NewHTTPClient(llmboundary.HTTPClientOptions{
			Endpoint:         envString("NEXUSIM_SUMMARY_LLM_ENDPOINT", ""),
			BearerToken:      envString("NEXUSIM_SUMMARY_LLM_BEARER_TOKEN", ""),
			Timeout:          envDuration("NEXUSIM_SUMMARY_LLM_TIMEOUT", 2*time.Second),
			MaxResponseBytes: int64(envInt("NEXUSIM_SUMMARY_LLM_MAX_RESPONSE_BYTES", int(llmboundary.DefaultMaxResponseBytes))),
		})
		if err != nil {
			return nil, err
		}
		return app.NewGuardedLLMSummaryProvider(client, llmboundary.Options{
			TokenBudget:      envInt("NEXUSIM_SUMMARY_LLM_TOKEN_BUDGET", llmboundary.DefaultTokenBudget),
			MaxEvidenceItems: envInt("NEXUSIM_SUMMARY_LLM_MAX_EVIDENCE_ITEMS", llmboundary.DefaultMaxEvidenceItems),
			MaxTextRunes:     envInt("NEXUSIM_SUMMARY_LLM_MAX_TEXT_RUNES", llmboundary.DefaultMaxTextRunes),
		}), nil
	case "python-worker":
		runner, err := summaryPythonWorkerRunnerFromEnv()
		if err != nil {
			return nil, err
		}
		return app.NewPythonWorkerSummaryProvider(app.ExtractiveSummaryProvider{}, runner), nil
	default:
		return nil, fmt.Errorf("unsupported NEXUSIM_SUMMARY_PROVIDER_MODE %q", mode)
	}
}

func summaryPythonWorkerRunnerFromEnv() (pythonworker.Runner, error) {
	return pythonworker.NewRunner(pythonworker.RunnerOptions{
		Python:         envString("NEXUSIM_SUMMARY_PYTHON_BIN", "python"),
		ScriptPath:     envString("NEXUSIM_SUMMARY_PYTHON_WORKER_SCRIPT", "ai/python/scripts/run_candidate_worker.py"),
		WorkDir:        envString("NEXUSIM_SUMMARY_PYTHON_WORKER_WORKDIR", "."),
		Timeout:        envDuration("NEXUSIM_SUMMARY_PYTHON_WORKER_TIMEOUT", 5*time.Second),
		MaxOutputBytes: int64(envInt("NEXUSIM_SUMMARY_PYTHON_WORKER_MAX_OUTPUT_BYTES", int(pythonworker.DefaultMaxOutputBytes))),
	})
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
			log.Printf("summary-service debug server stopped: %v", err)
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
		_, _ = writer.Write([]byte("nexusim_summary_service_info 1\n"))
	}
	mux.HandleFunc("/debug/metrics", metricsHandler)
	mux.HandleFunc("/metrics", metricsHandler)
	return mux
}
