package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	grpcapi "github.com/qsyy0921/IM/services/timeline-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/timeline-service/internal/app"
	postgresinfra "github.com/qsyy0921/IM/services/timeline-service/internal/infrastructure/postgres"
	"github.com/qsyy0921/IM/services/timeline-service/internal/types"
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
	mode := timelineModeFromEnv()
	if err := validateTimelineMode(mode); err != nil {
		return err
	}
	switch mode {
	case "noop":
		return runNoop(ctx)
	case "seq-block-allocator":
		return runSeqBlockAllocator(ctx)
	case "seq-lease-expire":
		return runSeqLeaseExpire(ctx)
	case "gap-marker-create":
		return runGapMarkerCreate(ctx)
	case "gap-marker-close":
		return runGapMarkerClose(ctx)
	case "gap-marker-audit":
		return runGapMarkerAudit(ctx)
	default:
		return fmt.Errorf("unsupported NEXUSIM_TIMELINE_SERVICE_MODE %q", mode)
	}
}

func runSeqBlockAllocator(ctx context.Context) error {
	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	debugAddr, err := timelineDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr)
	if err != nil {
		return err
	}
	defer stopDebug()

	grpcAddr := envString("NEXUSIM_TIMELINE_GRPC_ADDR", "0.0.0.0:10710")
	listener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return err
	}
	server := grpc.NewServer()
	repository := postgresinfra.NewRepository(pool)
	grpcapi.Register(server, grpcapi.NewServer(app.NewAllocateSeqBlockUseCase(
		repository,
		envInt("NEXUSIM_TIMELINE_SEQ_BLOCK_MAX_SIZE", 10000),
		envDuration("NEXUSIM_TIMELINE_SEQ_BLOCK_LEASE_TTL", 30*time.Second),
	)))

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("timeline-service seq block allocator listening on %s", grpcAddr)

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

func runNoop(ctx context.Context) error {
	debugAddr, err := timelineDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr)
	if err != nil {
		return err
	}
	defer stopDebug()

	log.Println("timeline-service noop mode: sequencer and partition roles are contract-only until implementation")
	<-ctx.Done()
	return nil
}

func timelineModeFromEnv() string {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_TIMELINE_SERVICE_MODE"))
	if mode == "" {
		mode = "noop"
	}
	return mode
}

func validateTimelineMode(mode string) error {
	switch mode {
	case "noop", "seq-block-allocator", "seq-lease-expire", "gap-marker-create", "gap-marker-close", "gap-marker-audit":
		return nil
	default:
		return fmt.Errorf("unsupported NEXUSIM_TIMELINE_SERVICE_MODE %q", mode)
	}
}

func runSeqLeaseExpire(ctx context.Context) error {
	repository, closePool, err := timelineRepositoryFromEnv(ctx)
	if err != nil {
		return err
	}
	defer closePool()
	operatorID, err := timelineRepairOperatorID()
	if err != nil {
		return err
	}
	reason, err := timelineRepairReason()
	if err != nil {
		return err
	}
	before, err := envOptionalRFC3339Time("NEXUSIM_TIMELINE_LEASE_EXPIRE_BEFORE")
	if err != nil {
		return err
	}
	if before.IsZero() {
		before = time.Now().UTC()
	}
	command := types.ExpireLeasesCommand{
		Before:     before,
		Limit:      envInt("NEXUSIM_TIMELINE_REPAIR_LIMIT", 100),
		DryRun:     envBool("NEXUSIM_TIMELINE_REPAIR_DRY_RUN", true),
		OperatorID: operatorID,
		Reason:     reason,
	}
	result, err := app.NewExpireSeqBlockLeasesUseCase(repository).Execute(ctx, command)
	if err != nil {
		return err
	}
	log.Printf("timeline-service seq lease expire matched=%d expired=%d dry_run=%t", result.Matched, result.Expired, result.DryRun)
	return writeTimelineRepairOutput(result)
}

func runGapMarkerCreate(ctx context.Context) error {
	repository, closePool, err := timelineRepositoryFromEnv(ctx)
	if err != nil {
		return err
	}
	defer closePool()
	operatorID, err := timelineRepairOperatorID()
	if err != nil {
		return err
	}
	reason, err := timelineRepairReason()
	if err != nil {
		return err
	}
	command := types.GapMarkerCommand{
		TenantID:       envString("NEXUSIM_TIMELINE_GAP_TENANT_ID", ""),
		ConversationID: envString("NEXUSIM_TIMELINE_GAP_CONVERSATION_ID", ""),
		StartSeq:       envInt64("NEXUSIM_TIMELINE_GAP_START_SEQ", 0),
		EndSeq:         envInt64("NEXUSIM_TIMELINE_GAP_END_SEQ", 0),
		SequencerEpoch: envInt64("NEXUSIM_TIMELINE_GAP_EPOCH", 0),
		LeaseID:        envString("NEXUSIM_TIMELINE_GAP_LEASE_ID", ""),
		Reason:         reason,
		OperatorID:     operatorID,
		DryRun:         envBool("NEXUSIM_TIMELINE_REPAIR_DRY_RUN", true),
	}
	marker, err := app.NewCreateGapMarkerUseCase(repository).Execute(ctx, command)
	if err != nil {
		return err
	}
	log.Printf("timeline-service gap marker create marker_id=%s tenant_id=%s conversation_id=%s range=%d-%d dry_run=%t", marker.MarkerID, marker.TenantID, marker.ConversationID, marker.StartSeq, marker.EndSeq, command.DryRun)
	return writeTimelineRepairOutput(marker)
}

func runGapMarkerClose(ctx context.Context) error {
	repository, closePool, err := timelineRepositoryFromEnv(ctx)
	if err != nil {
		return err
	}
	defer closePool()
	operatorID, err := timelineRepairOperatorID()
	if err != nil {
		return err
	}
	reason, err := timelineRepairReason()
	if err != nil {
		return err
	}
	command := types.CloseGapMarkerCommand{
		MarkerID:    envString("NEXUSIM_TIMELINE_GAP_MARKER_ID", ""),
		OperatorID:  operatorID,
		CloseReason: reason,
		DryRun:      envBool("NEXUSIM_TIMELINE_REPAIR_DRY_RUN", true),
	}
	marker, err := app.NewCloseGapMarkerUseCase(repository).Execute(ctx, command)
	if err != nil {
		return err
	}
	log.Printf("timeline-service gap marker close marker_id=%s status=%s dry_run=%t", marker.MarkerID, marker.Status, command.DryRun)
	return writeTimelineRepairOutput(marker)
}

func runGapMarkerAudit(ctx context.Context) error {
	repository, closePool, err := timelineRepositoryFromEnv(ctx)
	if err != nil {
		return err
	}
	defer closePool()
	rows, err := repository.AuditGapMarkers(
		ctx,
		envString("NEXUSIM_TIMELINE_GAP_TENANT_ID", ""),
		envString("NEXUSIM_TIMELINE_GAP_CONVERSATION_ID", ""),
		envString("NEXUSIM_TIMELINE_GAP_STATUS", ""),
		envInt("NEXUSIM_TIMELINE_REPAIR_LIMIT", 20),
	)
	if err != nil {
		return err
	}
	log.Printf("timeline-service gap marker audit rows=%d", len(rows))
	return writeTimelineRepairOutput(rows)
}

func timelineRepositoryFromEnv(ctx context.Context) (*postgresinfra.Repository, func(), error) {
	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return nil, nil, errors.New("NEXUSIM_PG_DSN is required")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return nil, nil, err
	}
	return postgresinfra.NewRepository(pool), pool.Close, nil
}

func openPGPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	config.MaxConns = int32(envInt("NEXUSIM_TIMELINE_PG_MAX_CONNS", 16))
	return pgxpool.NewWithConfig(ctx, config)
}

func envString(name, defaultValue string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return defaultValue
}

func timelineRepairOperatorID() (string, error) {
	operatorID := strings.TrimSpace(os.Getenv("NEXUSIM_TIMELINE_REPAIR_OPERATOR_ID"))
	if operatorID == "" {
		return "", errors.New("NEXUSIM_TIMELINE_REPAIR_OPERATOR_ID is required")
	}
	return operatorID, nil
}

func timelineRepairReason() (string, error) {
	path := strings.TrimSpace(os.Getenv("NEXUSIM_TIMELINE_REPAIR_REASON_FILE"))
	if path == "" {
		return "", errors.New("NEXUSIM_TIMELINE_REPAIR_REASON_FILE is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read NEXUSIM_TIMELINE_REPAIR_REASON_FILE: %w", err)
	}
	if len(data) > 64*1024 {
		return "", errors.New("NEXUSIM_TIMELINE_REPAIR_REASON_FILE must be at most 64 KiB")
	}
	reason := strings.TrimSpace(string(data))
	if reason == "" {
		return "", errors.New("NEXUSIM_TIMELINE_REPAIR_REASON_FILE must not be empty")
	}
	return reason, nil
}

func envInt(name string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return defaultValue
	}
	return value
}

func envInt64(name string, defaultValue int64) int64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return defaultValue
	}
	return value
}

func envBool(name string, defaultValue bool) bool {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return defaultValue
	}
	return value
}

func envDuration(name string, defaultValue time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return defaultValue
	}
	return value
}

func envOptionalRFC3339Time(name string) (time.Time, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return time.Time{}, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be RFC3339: %w", name, err)
	}
	return value.UTC(), nil
}

func writeTimelineRepairOutput(payload any) error {
	outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_TIMELINE_REPAIR_OUTPUT"))
	if outputPath == "" {
		return nil
	}
	fullPath, err := filepath.Abs(outputPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(fullPath, append(data, '\n'), 0o644)
}

func timelineDebugAddr() string {
	if addr := strings.TrimSpace(os.Getenv("NEXUSIM_TIMELINE_DEBUG_ADDR")); addr != "" {
		return addr
	}
	return strings.TrimSpace(os.Getenv("NEXUSIM_DEBUG_ADDR"))
}

func timelineDebugAddrFromEnv() (string, error) {
	addr := timelineDebugAddr()
	allowPublic, _, err := envOptionalBool("NEXUSIM_TIMELINE_DEBUG_ALLOW_PUBLIC")
	if err != nil {
		return "", err
	}
	return addr, validateTimelineDebugListenerConfig(addr, allowPublic)
}

func validateTimelineDebugListenerConfig(addr string, allowPublic bool) error {
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
	return errors.New("timeline-service debug listener address is non-private; set NEXUSIM_TIMELINE_DEBUG_ALLOW_PUBLIC=true to allow")
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
			log.Printf("timeline-service debug server stopped: %v", err)
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
		_, _ = writer.Write([]byte("# HELP nexusim_timeline_service_info Static timeline-service info metric.\n"))
		_, _ = writer.Write([]byte("# TYPE nexusim_timeline_service_info gauge\n"))
		_, _ = writer.Write([]byte("nexusim_timeline_service_info 1\n"))
	}
	mux.HandleFunc("/metrics", metricsHandler)
	mux.HandleFunc("/debug/metrics", metricsHandler)
	return mux
}
