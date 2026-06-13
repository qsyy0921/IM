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
	policygrpc "github.com/qsyy0921/IM/services/policy-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/policy-service/internal/app"
	"github.com/qsyy0921/IM/services/policy-service/internal/domain"
	kafkainfra "github.com/qsyy0921/IM/services/policy-service/internal/infrastructure/kafka"
	monitoringinfra "github.com/qsyy0921/IM/services/policy-service/internal/infrastructure/monitoring"
	postgresinfra "github.com/qsyy0921/IM/services/policy-service/internal/infrastructure/postgres"
	contacttrigger "github.com/qsyy0921/IM/services/policy-service/internal/trigger/contact"
	outboxtrigger "github.com/qsyy0921/IM/services/policy-service/internal/trigger/outbox"
	timelinetrigger "github.com/qsyy0921/IM/services/policy-service/internal/trigger/timeline"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
	"google.golang.org/grpc"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run() error {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_POLICY_SERVICE_MODE"))
	switch mode {
	case "", "noop":
		log.Println("policy-service runtime wiring is idle; set NEXUSIM_POLICY_SERVICE_MODE=grpc, contact-consumer, timeline-consumer, outbox-relay, or outbox-repair")
		return nil
	case "grpc":
		return runGRPC()
	case "contact-consumer":
		return runContactConsumer()
	case "timeline-consumer":
		return runTimelineConsumer()
	case "outbox-relay":
		return runOutboxRelay()
	case "outbox-repair":
		return runOutboxRepair()
	default:
		return errors.New("unsupported NEXUSIM_POLICY_SERVICE_MODE")
	}
}

func runOutboxRelay() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := envString("NEXUSIM_PG_DSN", "")
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required for policy outbox relay")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	producer, err := kafkainfra.NewWriterProducer(splitCSV(os.Getenv("NEXUSIM_KAFKA_BROKERS")))
	if err != nil {
		return err
	}
	defer producer.Close()

	relay := outboxtrigger.NewRelay(
		postgresinfra.NewOutboxStore(pool),
		producer,
		outboxtrigger.Config{
			Topic:          envString("NEXUSIM_POLICY_AUDIT_EVENTS_TOPIC", outboxtrigger.TopicPolicyEvents),
			BatchSize:      envInt("NEXUSIM_POLICY_OUTBOX_BATCH_SIZE", 500),
			PollInterval:   envDuration("NEXUSIM_POLICY_OUTBOX_POLL_INTERVAL", time.Second),
			MaxAttempts:    envInt("NEXUSIM_POLICY_OUTBOX_MAX_ATTEMPTS", 5),
			RetryBaseDelay: envDuration("NEXUSIM_POLICY_OUTBOX_RETRY_BASE_DELAY", time.Second),
		},
	)
	log.Println("policy-service decision audit outbox relay started")
	return relay.Run(ctx)
}

func runOutboxRepair() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := envString("NEXUSIM_PG_DSN", "")
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required for policy outbox repair")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	eventIDs := splitCSV(os.Getenv("NEXUSIM_POLICY_OUTBOX_REPAIR_EVENT_IDS"))
	operator := envString("NEXUSIM_POLICY_OUTBOX_REPAIR_OPERATOR", "local-operator")
	reason := envString("NEXUSIM_POLICY_OUTBOX_REPAIR_REASON", "manual policy audit outbox repair")
	stats, err := postgresinfra.NewOutboxStore(pool).RepairDLQEvents(ctx, eventIDs, operator, reason, validatePolicyAuditOutboxMessage)
	if err != nil {
		return err
	}
	log.Printf(
		"policy-service outbox repair completed requested=%d repaired=%d skipped=%d invalid=%d",
		stats.Requested,
		stats.Repaired,
		stats.Skipped,
		stats.Invalid,
	)
	if stats.Invalid > 0 {
		return errors.New("policy audit outbox repair skipped invalid events")
	}
	return nil
}

func validatePolicyAuditOutboxMessage(message types.OutboxMessage) error {
	_, err := outboxtrigger.BuildPolicyEvent(message)
	return err
}

func runContactConsumer() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := envString("NEXUSIM_PG_DSN", "")
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required for policy contact consumer")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	consumer, err := kafkainfra.NewReaderConsumer(kafkainfra.ReaderConfig{
		Brokers: splitCSV(os.Getenv("NEXUSIM_KAFKA_BROKERS")),
		Topic:   envString("NEXUSIM_CONTACT_EVENTS_TOPIC", contacttrigger.TopicContactEvents),
		GroupID: envString("NEXUSIM_POLICY_CONTACT_CONSUMER_GROUP", "nexusim-policy-contacts"),
	})
	if err != nil {
		return err
	}
	defer consumer.Close()

	worker := contacttrigger.NewWorker(
		consumer,
		app.NewProjectContactEventUseCase(postgresinfra.NewProjectionRepository(pool)),
		envString("NEXUSIM_POLICY_CONTACT_CONSUMER_GROUP", "nexusim-policy-contacts"),
	)
	log.Println("policy-service contact projection consumer started")
	return worker.Run(ctx)
}

func runTimelineConsumer() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := envString("NEXUSIM_PG_DSN", "")
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required for policy timeline consumer")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	groupID := envString("NEXUSIM_POLICY_TIMELINE_CONSUMER_GROUP", "nexusim-policy-timeline")
	consumer, err := kafkainfra.NewReaderConsumer(kafkainfra.ReaderConfig{
		Brokers: splitCSV(os.Getenv("NEXUSIM_KAFKA_BROKERS")),
		Topic:   envString("NEXUSIM_CONVERSATION_TIMELINE_TOPIC", timelinetrigger.TopicConversationTimelineEvents),
		GroupID: groupID,
	})
	if err != nil {
		return err
	}
	defer consumer.Close()

	worker := timelinetrigger.NewWorker(
		consumer,
		app.NewProjectConversationMemberEventUseCase(postgresinfra.NewProjectionRepository(pool)),
		groupID,
	)
	log.Println("policy-service conversation timeline projection consumer started")
	return worker.Run(ctx)
}

func runGRPC() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := envString("NEXUSIM_POLICY_GRPC_ADDR", "0.0.0.0:10800")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	policy := domain.StaticMessagePolicy{
		Allowed:           envBool("NEXUSIM_POLICY_MESSAGE_ALLOWED", true),
		PermissionVersion: envInt64("NEXUSIM_POLICY_PERMISSION_VERSION", 1),
		Classification:    envString("NEXUSIM_POLICY_CLASSIFICATION", "INTERNAL"),
		Reason:            envString("NEXUSIM_POLICY_DENY_REASON", ""),
	}
	var evaluator app.MessagePolicyEvaluator = policy
	var useCaseOptions []app.CheckMessageActionOption
	var pool *pgxpool.Pool
	rulesEnabled := envBool("NEXUSIM_POLICY_RULES_ENABLED", false)
	if rulesEnabled {
		dsn := envString("NEXUSIM_PG_DSN", "")
		if dsn == "" {
			return errors.New("NEXUSIM_PG_DSN is required when NEXUSIM_POLICY_RULES_ENABLED=true")
		}
		var err error
		pool, err = openPGPool(ctx, dsn)
		if err != nil {
			return err
		}
		defer pool.Close()
		postgresEvaluator := postgresinfra.NewMessagePolicyEvaluator(pool, policy)
		evaluator = postgresEvaluator
		useCaseOptions = append(useCaseOptions, app.WithPolicyDecisionAuditor(postgresinfra.NewDecisionAuditOutbox(pool)))
		useCaseOptions = append(useCaseOptions, app.WithMessageOwnershipOverrideChecker(postgresEvaluator))
		log.Println("policy-service message action rule store enabled")
		log.Println("policy-service decision audit outbox enabled")
	}
	grpcMetrics := monitoringinfra.NewGRPCMetrics()
	decisionMetrics := monitoringinfra.NewDecisionMetrics()
	useCaseOptions = append(useCaseOptions, app.WithPolicyDecisionObserver(decisionMetrics))
	stopDebug, err := startDebugServer(ctx, policyDebugAddr(), monitoringinfra.NewHandler(pool, rulesEnabled, grpcMetrics, decisionMetrics))
	if err != nil {
		return err
	}
	defer stopDebug()

	server := grpc.NewServer(grpc.UnaryInterceptor(grpcMetrics.UnaryServerInterceptor(log.Default())))
	policygrpc.Register(server, policygrpc.NewServer(app.NewCheckMessageActionUseCase(evaluator, useCaseOptions...)))
	go func() {
		<-ctx.Done()
		server.GracefulStop()
	}()
	log.Printf("policy-service grpc listening on %s", addr)
	if err := server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return err
	}
	return ctx.Err()
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
			log.Printf("policy-service debug server stopped with error: %v", err)
		}
	}()
	log.Printf("policy-service debug server started on %s", addr)
	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		<-done
	}, nil
}

func openPGPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if maxConns := envInt("NEXUSIM_POLICY_PG_MAX_CONNS", 0); maxConns > 0 {
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

func envBool(name string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
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

func policyDebugAddr() string {
	return envString("NEXUSIM_POLICY_DEBUG_ADDR", envString("NEXUSIM_DEBUG_ADDR", ""))
}

func envInt64(name string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}
