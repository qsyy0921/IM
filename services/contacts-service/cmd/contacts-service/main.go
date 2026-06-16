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

	"github.com/jackc/pgx/v5/pgxpool"
	contactsgrpc "github.com/qsyy0921/IM/services/contacts-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/contacts-service/internal/app"
	kafkainfra "github.com/qsyy0921/IM/services/contacts-service/internal/infrastructure/kafka"
	monitoringinfra "github.com/qsyy0921/IM/services/contacts-service/internal/infrastructure/monitoring"
	postgresinfra "github.com/qsyy0921/IM/services/contacts-service/internal/infrastructure/postgres"
	"github.com/qsyy0921/IM/services/contacts-service/internal/trigger/outbox"
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run() error {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_SERVICE_MODE"))
	switch mode {
	case "", "noop":
		log.Println("contacts-service runtime wiring is idle; set NEXUSIM_CONTACTS_SERVICE_MODE=grpc, outbox-relay, outbox-audit, outbox-repair, outbox-repair-audit, outbox-repair-cleanup, tenant-privacy-default-audit, tenant-privacy-default-set, source-policy-audit, source-policy-set, or contact-request-review")
		return nil
	case "grpc":
		return runGRPC()
	case "outbox-relay":
		return runOutboxRelay()
	case "outbox-audit":
		return runOutboxAudit()
	case "outbox-repair":
		return runOutboxRepair()
	case "outbox-repair-audit":
		return runOutboxRepairAudit()
	case "outbox-repair-cleanup":
		return runOutboxRepairCleanup()
	case "tenant-privacy-default-audit":
		return runTenantPrivacyDefaultAudit()
	case "tenant-privacy-default-set":
		return runTenantPrivacyDefaultSet()
	case "source-policy-audit":
		return runSourcePolicyAudit()
	case "source-policy-set":
		return runSourcePolicySet()
	case "contact-request-review":
		return runContactRequestReview()
	default:
		return errors.New("unsupported NEXUSIM_CONTACTS_SERVICE_MODE")
	}
}

func runGRPC() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	addr := envString("NEXUSIM_CONTACTS_GRPC_ADDR", "0.0.0.0:10500")
	authMode := envString("NEXUSIM_CONTACTS_AUTH_MODE", "body")
	serverTLSConfig, serverTLSEnabled, err := contactsGRPCTLSConfigFromEnv()
	if err != nil {
		return err
	}
	if err := validateTrustedMetadataListenerConfig(addr, authMode, serverTLSConfig); err != nil {
		return err
	}

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	grpcMetrics := monitoringinfra.NewGRPCMetrics()
	traceConfig, err := contactsTraceConfigFromEnv()
	if err != nil {
		return err
	}
	traceRuntime, err := monitoringinfra.NewTraceRuntime(ctx, traceConfig)
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := traceRuntime.Shutdown(shutdownCtx); err != nil {
			log.Printf("contacts-service OpenTelemetry trace shutdown failed: %v", err)
		}
	}()
	debugAddr, err := contactsDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr, monitoringinfra.NewHandler(pool, grpcMetrics).WithTraceStats(traceRuntime.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	repository := postgresinfra.NewRepository(pool)
	server, err := newGRPCServerWithConfig(grpcMetrics, authMode, serverTLSConfig, serverTLSEnabled, traceRuntime.UnaryServerInterceptor())
	if err != nil {
		return err
	}
	contactsgrpc.Register(server, contactsgrpc.NewServer(
		app.NewSendContactRequestUseCase(repository),
		app.NewGetContactPrivacyUseCase(repository),
		app.NewSetContactPrivacyUseCase(repository),
		app.NewSetContactPrivacyExceptionUseCase(repository),
		app.NewListContactPrivacyExceptionsUseCase(repository),
		app.NewDeleteContactPrivacyExceptionUseCase(repository),
		app.NewRespondContactRequestUseCase(repository),
		app.NewCancelContactRequestUseCase(repository),
		app.NewListContactRequestsUseCase(repository),
		app.NewListContactsUseCase(repository),
		app.NewGetContactStateUseCase(repository),
		app.NewDeleteContactUseCase(repository),
		app.NewBlockContactUseCase(repository),
		app.NewUnblockContactUseCase(repository),
		app.NewUpdateContactRemarkUseCase(repository),
		app.NewUpdateContactGroupUseCase(repository),
	))

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("contacts-service grpc listening on %s", addr)

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

func runOutboxRelay() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	brokers := splitCSV(os.Getenv("NEXUSIM_KAFKA_BROKERS"))
	producer, err := kafkainfra.NewWriterProducer(brokers)
	if err != nil {
		return err
	}
	defer producer.Close()

	topic := envString("NEXUSIM_CONTACT_EVENTS_TOPIC", outbox.TopicContactEvents)
	relay := outbox.NewRelay(
		postgresinfra.NewOutboxStore(pool),
		producer,
		outbox.Config{
			Topic:          topic,
			BatchSize:      envInt("NEXUSIM_CONTACTS_OUTBOX_BATCH_SIZE", 500),
			PollInterval:   envDuration("NEXUSIM_CONTACTS_OUTBOX_POLL_INTERVAL", time.Second),
			MaxAttempts:    envInt("NEXUSIM_CONTACTS_OUTBOX_MAX_ATTEMPTS", 5),
			RetryBaseDelay: envDuration("NEXUSIM_CONTACTS_OUTBOX_RETRY_BASE_DELAY", time.Second),
			ErrorBackoff:   envDuration("NEXUSIM_CONTACTS_OUTBOX_RELAY_ERROR_BACKOFF", time.Second),
			Logf:           log.Printf,
		},
	)
	debugAddr, err := contactsDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr, monitoringinfra.NewHandler(pool).WithOutboxRelayStats(relay.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()
	log.Printf("contacts-service outbox relay started topic=%s", topic)
	return relay.Run(ctx)
}

func runOutboxRepair() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	eventIDs := splitCSV(os.Getenv("NEXUSIM_CONTACTS_OUTBOX_REPAIR_EVENT_IDS"))
	reason := envString("NEXUSIM_CONTACTS_OUTBOX_REPAIR_REASON", "manual contacts outbox repair")
	stats, err := postgresinfra.NewOutboxStore(pool).RepairDLQEvents(ctx, eventIDs, reason)
	if err != nil {
		return err
	}
	log.Printf(
		"contacts-service outbox repair completed requested=%d repaired=%d skipped=%d",
		stats.Requested,
		stats.Repaired,
		stats.Skipped,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_OUTBOX_REPAIR_OUTPUT")); outputPath != "" {
		if err := writeOutboxRepairOutput(outputPath, stats, len(eventIDs)); err != nil {
			return err
		}
	}
	return nil
}

func runOutboxAudit() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	var outboxID *int64
	if value := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_OUTBOX_AUDIT_OUTBOX_ID")); value != "" {
		parsed := envInt64AllowZero("NEXUSIM_CONTACTS_OUTBOX_AUDIT_OUTBOX_ID", 0)
		outboxID = &parsed
	}
	rows, err := postgresinfra.NewOutboxStore(pool).AuditOutbox(ctx, postgresinfra.OutboxAuditOptions{
		OutboxID:    outboxID,
		EventID:     envString("NEXUSIM_CONTACTS_OUTBOX_AUDIT_EVENT_ID", ""),
		TenantID:    envString("NEXUSIM_CONTACTS_OUTBOX_AUDIT_TENANT_ID", ""),
		AggregateID: envString("NEXUSIM_CONTACTS_OUTBOX_AUDIT_AGGREGATE_ID", ""),
		Status:      envString("NEXUSIM_CONTACTS_OUTBOX_AUDIT_STATUS", ""),
		EventType:   envString("NEXUSIM_CONTACTS_OUTBOX_AUDIT_EVENT_TYPE", ""),
		Limit:       envInt("NEXUSIM_CONTACTS_OUTBOX_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("contacts-service outbox audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"contacts_outbox id=%d event_id=%s tenant_id=%s aggregate_type=%s aggregate_id=%s aggregate_version=%d event_type=%s status=%s retry_count=%d available_at=%s next_retry_at=%s published_at=%s dead_lettered_at=%s last_error=%q",
			row.ID,
			row.EventID,
			row.TenantID,
			row.AggregateType,
			row.AggregateID,
			row.AggregateVersion,
			row.EventType,
			row.Status,
			row.RetryCount,
			row.AvailableAt.Format(time.RFC3339),
			formatOptionalTime(row.NextRetryAt),
			formatOptionalTime(row.PublishedAt),
			formatOptionalTime(row.DeadLetteredAt),
			row.LastError,
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_OUTBOX_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeOutboxAuditOutput(outputPath, rows); err != nil {
			return err
		}
	}
	return nil
}

func runOutboxRepairAudit() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	rows, err := postgresinfra.NewOutboxStore(pool).AuditOutboxRepairs(ctx, postgresinfra.OutboxRepairAuditOptions{
		EventID:  envString("NEXUSIM_CONTACTS_OUTBOX_REPAIR_AUDIT_EVENT_ID", ""),
		TenantID: envString("NEXUSIM_CONTACTS_OUTBOX_REPAIR_AUDIT_TENANT_ID", ""),
		Limit:    envInt("NEXUSIM_CONTACTS_OUTBOX_REPAIR_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("contacts-service outbox repair audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"contacts_outbox_repair event_id=%s tenant_id=%s previous_status=%s previous_retry_count=%d previous_dead_lettered_at=%s repaired_at=%s reason=%q previous_last_error=%q",
			row.EventID,
			row.TenantID,
			row.PreviousStatus,
			row.PreviousRetryCount,
			formatOptionalTime(row.PreviousDeadLetteredAt),
			row.RepairedAt.Format(time.RFC3339),
			row.Reason,
			row.PreviousLastError,
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_OUTBOX_REPAIR_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeOutboxRepairAuditOutput(outputPath, rows); err != nil {
			return err
		}
	}
	return nil
}

type outboxRepairCleanupConfig struct {
	Retention time.Duration
	BatchSize int
}

func runOutboxRepairCleanup() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	config, err := outboxRepairCleanupConfigFromEnv()
	if err != nil {
		return err
	}
	cutoff := time.Now().UTC().Add(-config.Retention)
	stats, err := postgresinfra.NewOutboxStore(pool).CleanupOutboxRepairs(ctx, postgresinfra.OutboxRepairCleanupOptions{
		EventID:  envString("NEXUSIM_CONTACTS_OUTBOX_REPAIR_CLEANUP_EVENT_ID", ""),
		TenantID: envString("NEXUSIM_CONTACTS_OUTBOX_REPAIR_CLEANUP_TENANT_ID", ""),
		Cutoff:   cutoff,
		Limit:    config.BatchSize,
	})
	if err != nil {
		return err
	}
	log.Printf(
		"contacts-service outbox repair cleanup completed deleted=%d cutoff=%s retention=%s batch_size=%d",
		stats.Deleted,
		cutoff.Format(time.RFC3339),
		config.Retention,
		config.BatchSize,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_OUTBOX_REPAIR_CLEANUP_OUTPUT")); outputPath != "" {
		if err := writeOutboxRepairCleanupOutput(outputPath, stats, cutoff, config.Retention, config.BatchSize, map[string]string{
			"event_id":  envString("NEXUSIM_CONTACTS_OUTBOX_REPAIR_CLEANUP_EVENT_ID", ""),
			"tenant_id": envString("NEXUSIM_CONTACTS_OUTBOX_REPAIR_CLEANUP_TENANT_ID", ""),
		}); err != nil {
			return err
		}
	}
	return nil
}

func runTenantPrivacyDefaultAudit() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tenantID := envString("NEXUSIM_CONTACTS_TENANT_PRIVACY_TENANT_ID", "")
	if tenantID == "" {
		return errors.New("NEXUSIM_CONTACTS_TENANT_PRIVACY_TENANT_ID is required")
	}
	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	result, err := app.NewGetTenantContactPrivacyDefaultUseCase(postgresinfra.NewRepository(pool)).Execute(
		ctx,
		types.GetTenantContactPrivacyDefaultCommand{TenantID: types.TenantID(tenantID)},
	)
	if err != nil {
		return err
	}
	log.Printf(
		"contacts-service tenant privacy default tenant_id=%s allow_contact_requests=%t allow_search_contact_requests=%t allow_profile_visibility=%t profile_visibility_fields=%v version=%d policy_source=%s updated_at_unix_ms=%d",
		result.TenantID,
		result.Settings.AllowContactRequests,
		result.Settings.AllowSearchContactRequests,
		result.Settings.AllowProfileVisibility,
		types.ContactProfileVisibilityFieldsToStrings(result.Settings.ProfileVisibilityFields),
		result.Settings.Version,
		result.Settings.PolicySource,
		result.Settings.UpdatedAtUnixMS,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_TENANT_PRIVACY_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeTenantPrivacyAuditOutput(outputPath, result); err != nil {
			return err
		}
	}
	return nil
}

func runTenantPrivacyDefaultSet() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tenantID := envString("NEXUSIM_CONTACTS_TENANT_PRIVACY_TENANT_ID", "")
	if tenantID == "" {
		return errors.New("NEXUSIM_CONTACTS_TENANT_PRIVACY_TENANT_ID is required")
	}
	allowContactRequests, err := envRequiredBool("NEXUSIM_CONTACTS_TENANT_PRIVACY_ALLOW_CONTACT_REQUESTS")
	if err != nil {
		return err
	}
	allowSearchContactRequests, allowSearchConfigured, err := envOptionalBool("NEXUSIM_CONTACTS_TENANT_PRIVACY_ALLOW_SEARCH_CONTACT_REQUESTS")
	if err != nil {
		return err
	}
	var allowSearchContactRequestsPtr *bool
	if allowSearchConfigured {
		allowSearchContactRequestsPtr = &allowSearchContactRequests
	}
	allowProfileVisibility, allowProfileConfigured, err := envOptionalBool("NEXUSIM_CONTACTS_TENANT_PRIVACY_ALLOW_PROFILE_VISIBILITY")
	if err != nil {
		return err
	}
	var allowProfileVisibilityPtr *bool
	if allowProfileConfigured {
		allowProfileVisibilityPtr = &allowProfileVisibility
	}
	profileVisibilityFields, updateProfileVisibilityFields, err := envOptionalProfileVisibilityFields("NEXUSIM_CONTACTS_TENANT_PRIVACY_PROFILE_VISIBILITY_FIELDS")
	if err != nil {
		return err
	}
	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	result, err := app.NewSetTenantContactPrivacyDefaultUseCase(postgresinfra.NewRepository(pool)).Execute(
		ctx,
		types.SetTenantContactPrivacyDefaultCommand{
			TenantID:                      types.TenantID(tenantID),
			AllowContactRequests:          allowContactRequests,
			AllowSearchContactRequests:    allowSearchContactRequestsPtr,
			AllowProfileVisibility:        allowProfileVisibilityPtr,
			UpdateProfileVisibilityFields: updateProfileVisibilityFields,
			ProfileVisibilityFields:       profileVisibilityFields,
		},
	)
	if err != nil {
		return err
	}
	log.Printf(
		"contacts-service tenant privacy default updated tenant_id=%s allow_contact_requests=%t allow_search_contact_requests=%t allow_profile_visibility=%t profile_visibility_fields=%v version=%d changed=%t updated_at_unix_ms=%d",
		result.TenantID,
		result.Settings.AllowContactRequests,
		result.Settings.AllowSearchContactRequests,
		result.Settings.AllowProfileVisibility,
		types.ContactProfileVisibilityFieldsToStrings(result.Settings.ProfileVisibilityFields),
		result.Settings.Version,
		result.Changed,
		result.Settings.UpdatedAtUnixMS,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_TENANT_PRIVACY_SET_OUTPUT")); outputPath != "" {
		if err := writeTenantPrivacySetOutput(outputPath, result); err != nil {
			return err
		}
	}
	return nil
}

func runSourcePolicyAudit() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tenantID := envString("NEXUSIM_CONTACTS_SOURCE_POLICY_TENANT_ID", "")
	if tenantID == "" {
		return errors.New("NEXUSIM_CONTACTS_SOURCE_POLICY_TENANT_ID is required")
	}
	sourceType, err := envContactRequestSourceType("NEXUSIM_CONTACTS_SOURCE_POLICY_SOURCE_TYPE")
	if err != nil {
		return err
	}
	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	result, err := app.NewGetTenantContactRequestSourcePolicyUseCase(postgresinfra.NewRepository(pool)).Execute(
		ctx,
		types.GetTenantContactRequestSourcePolicyCommand{
			TenantID:   types.TenantID(tenantID),
			SourceType: sourceType,
		},
	)
	if err != nil {
		return err
	}
	log.Printf(
		"contacts-service source policy tenant_id=%s source_type=%s allow_contact_requests=%t risk_level=%s review_required=%t version=%d updated_at_unix_ms=%d",
		result.TenantID,
		result.Policy.SourceType,
		result.Policy.AllowContactRequests,
		result.Policy.RiskLevel,
		result.Policy.ReviewRequired,
		result.Policy.Version,
		result.Policy.UpdatedAtUnixMS,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_SOURCE_POLICY_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeSourcePolicyAuditOutput(outputPath, result); err != nil {
			return err
		}
	}
	return nil
}

func runSourcePolicySet() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tenantID := envString("NEXUSIM_CONTACTS_SOURCE_POLICY_TENANT_ID", "")
	if tenantID == "" {
		return errors.New("NEXUSIM_CONTACTS_SOURCE_POLICY_TENANT_ID is required")
	}
	sourceType, err := envContactRequestSourceType("NEXUSIM_CONTACTS_SOURCE_POLICY_SOURCE_TYPE")
	if err != nil {
		return err
	}
	allowContactRequests, err := envRequiredBool("NEXUSIM_CONTACTS_SOURCE_POLICY_ALLOW_CONTACT_REQUESTS")
	if err != nil {
		return err
	}
	riskLevel, err := envOptionalContactRequestRiskLevel("NEXUSIM_CONTACTS_SOURCE_POLICY_RISK_LEVEL", types.ContactRequestRiskLevelLow)
	if err != nil {
		return err
	}
	reviewRequired, _, err := envOptionalBool("NEXUSIM_CONTACTS_SOURCE_POLICY_REVIEW_REQUIRED")
	if err != nil {
		return err
	}
	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	result, err := app.NewSetTenantContactRequestSourcePolicyUseCase(postgresinfra.NewRepository(pool)).Execute(
		ctx,
		types.SetTenantContactRequestSourcePolicyCommand{
			TenantID:             types.TenantID(tenantID),
			SourceType:           sourceType,
			AllowContactRequests: allowContactRequests,
			RiskLevel:            riskLevel,
			ReviewRequired:       reviewRequired,
		},
	)
	if err != nil {
		return err
	}
	log.Printf(
		"contacts-service source policy updated tenant_id=%s source_type=%s allow_contact_requests=%t risk_level=%s review_required=%t version=%d changed=%t updated_at_unix_ms=%d",
		result.TenantID,
		result.Policy.SourceType,
		result.Policy.AllowContactRequests,
		result.Policy.RiskLevel,
		result.Policy.ReviewRequired,
		result.Policy.Version,
		result.Changed,
		result.Policy.UpdatedAtUnixMS,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_SOURCE_POLICY_SET_OUTPUT")); outputPath != "" {
		if err := writeSourcePolicySetOutput(outputPath, result); err != nil {
			return err
		}
	}
	return nil
}

func runContactRequestReview() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tenantID := envString("NEXUSIM_CONTACTS_REQUEST_REVIEW_TENANT_ID", "")
	if tenantID == "" {
		return errors.New("NEXUSIM_CONTACTS_REQUEST_REVIEW_TENANT_ID is required")
	}
	requestID := envString("NEXUSIM_CONTACTS_REQUEST_REVIEW_REQUEST_ID", "")
	if requestID == "" {
		return errors.New("NEXUSIM_CONTACTS_REQUEST_REVIEW_REQUEST_ID is required")
	}
	decision, err := envContactRequestReviewDecision("NEXUSIM_CONTACTS_REQUEST_REVIEW_DECISION")
	if err != nil {
		return err
	}
	operator := envString("NEXUSIM_CONTACTS_REQUEST_REVIEW_OPERATOR", "manual")
	reason := envString("NEXUSIM_CONTACTS_REQUEST_REVIEW_REASON", "")
	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	result, err := app.NewReviewContactRequestUseCase(postgresinfra.NewRepository(pool)).Execute(
		ctx,
		types.ReviewContactRequestCommand{
			TenantID:  types.TenantID(tenantID),
			RequestID: requestID,
			Decision:  decision,
			Operator:  operator,
			Reason:    reason,
		},
	)
	if err != nil {
		return err
	}
	log.Printf(
		"contacts-service contact request review completed tenant_id=%s request_id=%s decision=%s previous_status=%s next_status=%s sender_user_id=%s receiver_user_id=%s risk_level=%s review_required=%t operator=%s reason_present=%t",
		result.TenantID,
		result.RequestID,
		result.Decision,
		result.PreviousStatus,
		result.Status,
		result.SenderUserID,
		result.ReceiverUserID,
		result.RiskLevel,
		result.ReviewRequired,
		operator,
		reason != "",
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_REQUEST_REVIEW_OUTPUT")); outputPath != "" {
		if err := writeContactRequestReviewOutput(outputPath, result, operator, reason != ""); err != nil {
			return err
		}
	}
	return nil
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
			log.Printf("contacts-service debug server stopped with error: %v", err)
		}
	}()
	log.Printf("contacts-service debug server started on %s", addr)
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
	if maxConns := envInt("NEXUSIM_CONTACTS_PG_MAX_CONNS", 0); maxConns > 0 {
		config.MaxConns = int32(maxConns)
	}
	return pgxpool.NewWithConfig(ctx, config)
}

func contactsDebugAddr() string {
	return envString("NEXUSIM_CONTACTS_DEBUG_ADDR", envString("NEXUSIM_DEBUG_ADDR", ""))
}

func contactsDebugAddrFromEnv() (string, error) {
	addr := contactsDebugAddr()
	allowPublic, _, err := envOptionalBool("NEXUSIM_CONTACTS_DEBUG_ALLOW_PUBLIC")
	if err != nil {
		return "", err
	}
	return addr, validateContactsDebugListenerConfig(addr, allowPublic)
}

func validateContactsDebugListenerConfig(addr string, allowPublic bool) error {
	if strings.TrimSpace(addr) == "" {
		return nil
	}
	if listenerAddrTrustedWithoutMTLS(addr) {
		return nil
	}
	if allowPublic {
		return nil
	}
	return errors.New("contacts-service debug listener address is non-private; set NEXUSIM_CONTACTS_DEBUG_ALLOW_PUBLIC=true to allow")
}

func newGRPCServer(grpcMetrics *monitoringinfra.GRPCMetrics) (*grpc.Server, error) {
	authMode := envString("NEXUSIM_CONTACTS_AUTH_MODE", "body")
	tlsConfig, tlsEnabled, err := contactsGRPCTLSConfigFromEnv()
	if err != nil {
		return nil, err
	}
	return newGRPCServerWithConfig(grpcMetrics, authMode, tlsConfig, tlsEnabled)
}

func newGRPCServerWithConfig(grpcMetrics *monitoringinfra.GRPCMetrics, authMode string, tlsConfig *tls.Config, tlsEnabled bool, traceInterceptors ...grpc.UnaryServerInterceptor) (*grpc.Server, error) {
	interceptors := make([]grpc.UnaryServerInterceptor, 0, 3)
	if grpcMetrics != nil {
		interceptors = append(interceptors, grpcMetrics.UnaryServerInterceptor(log.Default()))
	}
	for _, interceptor := range traceInterceptors {
		if interceptor != nil {
			interceptors = append(interceptors, interceptor)
		}
	}
	switch strings.ToLower(strings.TrimSpace(authMode)) {
	case "body", "request", "legacy":
	case "metadata", "verified-metadata":
		interceptors = append(interceptors, contactsgrpc.VerifiedAuthUnaryInterceptor(true))
	default:
		return nil, errors.New("unsupported NEXUSIM_CONTACTS_AUTH_MODE")
	}
	serverOptions := make([]grpc.ServerOption, 0, 2)
	if len(interceptors) > 0 {
		serverOptions = append(serverOptions, grpc.ChainUnaryInterceptor(interceptors...))
	}
	if tlsEnabled {
		serverOptions = append(serverOptions, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}
	return grpc.NewServer(serverOptions...), nil
}

func contactsTraceConfigFromEnv() (monitoringinfra.TraceConfig, error) {
	enabled, _, err := envOptionalBool("NEXUSIM_CONTACTS_OTEL_TRACES_ENABLED")
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	otlpInsecure, _, err := envOptionalBool("NEXUSIM_CONTACTS_OTEL_TRACES_OTLP_INSECURE")
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	samplingRatio, err := contactsTraceSamplingRatioFromEnv()
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	return monitoringinfra.TraceConfig{
		Enabled:       enabled,
		ServiceName:   envString("NEXUSIM_CONTACTS_OTEL_SERVICE_NAME", "contacts-service"),
		Exporter:      envString("NEXUSIM_CONTACTS_OTEL_TRACES_EXPORTER", "stdout"),
		OTLPEndpoint:  envString("NEXUSIM_CONTACTS_OTEL_TRACES_OTLP_ENDPOINT", ""),
		OTLPInsecure:  otlpInsecure,
		SamplingRatio: samplingRatio,
	}, nil
}

func contactsTraceSamplingRatioFromEnv() (float64, error) {
	raw := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_OTEL_TRACES_SAMPLING_RATIO"))
	if raw == "" {
		return 1, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 || value > 1 {
		return 0, errors.New("NEXUSIM_CONTACTS_OTEL_TRACES_SAMPLING_RATIO must be > 0 and <= 1")
	}
	return value, nil
}

func validateTrustedMetadataListenerConfig(listenAddr string, authMode string, tlsConfig *tls.Config) error {
	if !usesTrustedMetadataAuth(authMode) {
		return nil
	}
	if listenerAddrTrustedWithoutMTLS(listenAddr) {
		return nil
	}
	if tlsConfig != nil && tlsConfig.ClientAuth == tls.RequireAndVerifyClientCert {
		return nil
	}
	return errors.New("contacts-service uses verified metadata auth on non-private address without gRPC mTLS client certificate")
}

func usesTrustedMetadataAuth(authMode string) bool {
	switch strings.ToLower(strings.TrimSpace(authMode)) {
	case "metadata", "verified-metadata":
		return true
	default:
		return false
	}
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

func loadContactsGRPCCredentialsFromEnv() (credentials.TransportCredentials, bool, error) {
	tlsConfig, ok, err := contactsGRPCTLSConfigFromEnv()
	if err != nil || !ok {
		return nil, ok, err
	}
	return credentials.NewTLS(tlsConfig), true, nil
}

func contactsGRPCTLSConfigFromEnv() (*tls.Config, bool, error) {
	certFile := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_GRPC_TLS_CERT_FILE"))
	keyFile := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_GRPC_TLS_KEY_FILE"))
	clientCAFile := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_CA_FILE"))
	allowedClientDNSNames := envStringSet("NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", strings.ToLower)
	allowedClientURIs, err := envURIStringSet("NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_ALLOWED_URIS")
	if err != nil {
		return nil, true, err
	}
	requireClientCert, requireClientCertConfigured, err := envOptionalBool("NEXUSIM_CONTACTS_GRPC_TLS_REQUIRE_CLIENT_CERT")
	if err != nil {
		return nil, true, err
	}
	hasClientAllowlist := len(allowedClientDNSNames) > 0 || len(allowedClientURIs) > 0
	requireClientCert = clientCAFile != "" || hasClientAllowlist || (requireClientCertConfigured && requireClientCert)
	if certFile == "" && keyFile == "" && clientCAFile == "" && !requireClientCert && !hasClientAllowlist {
		return nil, false, nil
	}
	if certFile == "" || keyFile == "" {
		return nil, true, errors.New("NEXUSIM_CONTACTS_GRPC_TLS_CERT_FILE and NEXUSIM_CONTACTS_GRPC_TLS_KEY_FILE must be configured together")
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
			return nil, true, errors.New("NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_CA_FILE is required when client certificates are required")
		}
		pemBytes, err := os.ReadFile(clientCAFile)
		if err != nil {
			return nil, true, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, true, errors.New("NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_CA_FILE does not contain a valid PEM certificate")
		}
		tlsConfig.ClientCAs = pool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		if hasClientAllowlist {
			tlsConfig.VerifyConnection = verifyAllowedContactsGRPCClient(allowedClientDNSNames, allowedClientURIs)
		}
	}
	return tlsConfig, true, nil
}

func verifyAllowedContactsGRPCClient(allowedDNSNames map[string]struct{}, allowedURIs map[string]struct{}) func(tls.ConnectionState) error {
	return func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("contacts grpc client certificate is required")
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
		return errors.New("contacts grpc client certificate identity is not allowed")
	}
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

func envRequiredBool(name string) (bool, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return false, errors.New(name + " is required")
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, errors.New(name + " must be true or false")
	}
	return parsed, nil
}

func envContactRequestSourceType(name string) (types.ContactRequestSourceType, error) {
	value := strings.ToUpper(strings.TrimSpace(os.Getenv(name)))
	if value == "" {
		return "", errors.New(name + " is required")
	}
	sourceType := types.NormalizeContactRequestSourceType(types.ContactRequestSourceType(value))
	if sourceType == "" {
		return "", errors.New(name + " is invalid")
	}
	return sourceType, nil
}

func envOptionalContactRequestRiskLevel(name string, fallback types.ContactRequestRiskLevel) (types.ContactRequestRiskLevel, error) {
	value := strings.ToUpper(strings.TrimSpace(os.Getenv(name)))
	if value == "" {
		return fallback, nil
	}
	riskLevel := types.NormalizeContactRequestRiskLevel(types.ContactRequestRiskLevel(value))
	if riskLevel == "" {
		return "", errors.New(name + " is invalid")
	}
	return riskLevel, nil
}

func envContactRequestReviewDecision(name string) (types.ContactRequestReviewDecision, error) {
	value := strings.ToUpper(strings.TrimSpace(os.Getenv(name)))
	if value == "" {
		return "", errors.New(name + " is required")
	}
	decision := types.NormalizeContactRequestReviewDecision(types.ContactRequestReviewDecision(value))
	if decision == "" {
		return "", errors.New(name + " is invalid")
	}
	return decision, nil
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

func envPositiveDuration(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, errors.New(name + " must be a positive duration")
	}
	return parsed, nil
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

func envOptionalProfileVisibilityFields(name string) ([]types.ContactProfileVisibilityField, bool, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return nil, false, nil
	}
	rawFields := splitCSV(value)
	fields := make([]types.ContactProfileVisibilityField, 0, len(rawFields))
	for _, field := range rawFields {
		fields = append(fields, types.ContactProfileVisibilityField(field))
	}
	normalized, err := types.NormalizeContactProfileVisibilityFields(fields)
	if err != nil {
		return nil, true, errors.New(name + " contains an unsupported profile visibility field")
	}
	return normalized, true, nil
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

func envInt64AllowZero(name string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}

func envPositiveInt(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, errors.New(name + " must be a positive integer")
	}
	return parsed, nil
}

func outboxRepairCleanupConfigFromEnv() (outboxRepairCleanupConfig, error) {
	retention, err := envPositiveDuration("NEXUSIM_CONTACTS_OUTBOX_REPAIR_RETENTION", 7*24*time.Hour)
	if err != nil {
		return outboxRepairCleanupConfig{}, err
	}
	batchSize, err := envPositiveInt("NEXUSIM_CONTACTS_OUTBOX_REPAIR_CLEANUP_BATCH_SIZE", 5000)
	if err != nil {
		return outboxRepairCleanupConfig{}, err
	}
	return outboxRepairCleanupConfig{
		Retention: retention,
		BatchSize: batchSize,
	}, nil
}
