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
	grpcapi "github.com/qsyy0921/IM/services/conversation-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/conversation-service/internal/app"
	"github.com/qsyy0921/IM/services/conversation-service/internal/domain"
	monitoringinfra "github.com/qsyy0921/IM/services/conversation-service/internal/infrastructure/monitoring"
	postgresinfra "github.com/qsyy0921/IM/services/conversation-service/internal/infrastructure/postgres"
	"github.com/qsyy0921/IM/services/conversation-service/internal/trigger/memberchange"
	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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
		log.Println("conversation-service runtime wiring is idle; set NEXUSIM_CONVERSATION_SERVICE_MODE=grpc, member-change-worker, member-change-audit, member-window-audit, member-window-repair, or member-window-repair-audit")
		return nil
	case "grpc":
		return runGRPCServer()
	case "member-change-worker":
		return runMemberChangeWorker()
	case "member-change-audit":
		return runMemberChangeAudit()
	case "member-window-audit":
		return runMemberWindowAudit()
	case "member-window-repair":
		return runMemberWindowRepair()
	case "member-window-repair-audit":
		return runMemberWindowRepairAudit()
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
	listenAddr := envString("NEXUSIM_CONVERSATION_GRPC_ADDR", "0.0.0.0:10496")
	authMode := envString("NEXUSIM_CONVERSATION_AUTH_MODE", "body")
	serverTLSConfig, serverTLSEnabled, err := conversationGRPCTLSConfigFromEnv()
	if err != nil {
		return err
	}
	if err := validateTrustedMetadataListenerConfig(listenAddr, authMode, serverTLSConfig); err != nil {
		return err
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	scaleThresholds, err := conversationScaleThresholdsFromEnv()
	if err != nil {
		return err
	}
	grpcMetrics := monitoringinfra.NewGRPCMetrics()
	traceConfig, err := conversationTraceConfigFromEnv()
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
			log.Printf("conversation-service OpenTelemetry trace shutdown failed: %v", err)
		}
	}()
	debugAddr, err := conversationDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr, monitoringinfra.NewHandler(pool, grpcMetrics).WithTraceStats(traceRuntime.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	server, err := newGRPCServerWithConfig(grpcMetrics, authMode, serverTLSConfig, serverTLSEnabled, traceRuntime.UnaryServerInterceptor())
	if err != nil {
		return err
	}
	repository := postgresinfra.NewRepository(pool, postgresinfra.WithScaleThresholds(scaleThresholds))
	grpcapi.Register(
		server,
		grpcapi.NewServer(
			app.NewGetSendContextUseCase(repository),
			grpcapi.WithCreateConversation(app.NewCreateConversationUseCase(repository)),
			grpcapi.WithCreateMemberChange(app.NewCreateMemberChangeUseCase(repository)),
			grpcapi.WithTransferConversationOwner(app.NewTransferConversationOwnerUseCase(repository)),
			grpcapi.WithGetMemberChange(app.NewGetMemberChangeUseCase(repository)),
			grpcapi.WithListConversationMembers(app.NewListConversationMembersUseCase(repository)),
			grpcapi.WithGetConversationProfile(app.NewGetConversationProfileUseCase(repository)),
			grpcapi.WithUpdateConversationProfile(app.NewUpdateConversationProfileUseCase(repository)),
			grpcapi.WithCreateConversationNote(app.NewCreateConversationNoteUseCase(repository)),
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

func newGRPCServer(grpcMetrics ...*monitoringinfra.GRPCMetrics) (*grpc.Server, error) {
	authMode := envString("NEXUSIM_CONVERSATION_AUTH_MODE", "body")
	tlsConfig, tlsEnabled, err := conversationGRPCTLSConfigFromEnv()
	if err != nil {
		return nil, err
	}
	var metrics *monitoringinfra.GRPCMetrics
	if len(grpcMetrics) > 0 {
		metrics = grpcMetrics[0]
	}
	return newGRPCServerWithConfig(metrics, authMode, tlsConfig, tlsEnabled)
}

func newGRPCServerWithConfig(grpcMetrics *monitoringinfra.GRPCMetrics, authMode string, tlsConfig *tls.Config, tlsEnabled bool, traceInterceptors ...grpc.UnaryServerInterceptor) (*grpc.Server, error) {
	interceptors := make([]grpc.UnaryServerInterceptor, 0, 3)
	metrics := monitoringinfra.NewGRPCMetrics()
	if grpcMetrics != nil {
		metrics = grpcMetrics
	}
	interceptors = append(interceptors, metrics.UnaryServerInterceptor(log.Default()))
	for _, interceptor := range traceInterceptors {
		if interceptor != nil {
			interceptors = append(interceptors, interceptor)
		}
	}
	switch strings.ToLower(strings.TrimSpace(authMode)) {
	case "body", "request", "legacy":
	case "metadata", "verified-metadata":
		interceptors = append(interceptors, grpcapi.VerifiedAuthUnaryInterceptor(true))
	default:
		return nil, errors.New("unsupported NEXUSIM_CONVERSATION_AUTH_MODE")
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

func conversationTraceConfigFromEnv() (monitoringinfra.TraceConfig, error) {
	enabled, _, err := envOptionalBool("NEXUSIM_CONVERSATION_OTEL_TRACES_ENABLED")
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	otlpInsecure, _, err := envOptionalBool("NEXUSIM_CONVERSATION_OTEL_TRACES_OTLP_INSECURE")
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	samplingRatio, err := conversationTraceSamplingRatioFromEnv()
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	return monitoringinfra.TraceConfig{
		Enabled:       enabled,
		ServiceName:   envString("NEXUSIM_CONVERSATION_OTEL_SERVICE_NAME", "conversation-service"),
		Exporter:      envString("NEXUSIM_CONVERSATION_OTEL_TRACES_EXPORTER", "stdout"),
		OTLPEndpoint:  envString("NEXUSIM_CONVERSATION_OTEL_TRACES_OTLP_ENDPOINT", ""),
		OTLPInsecure:  otlpInsecure,
		SamplingRatio: samplingRatio,
	}, nil
}

func conversationTraceSamplingRatioFromEnv() (float64, error) {
	raw := strings.TrimSpace(os.Getenv("NEXUSIM_CONVERSATION_OTEL_TRACES_SAMPLING_RATIO"))
	if raw == "" {
		return 1, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 || value > 1 {
		return 0, errors.New("NEXUSIM_CONVERSATION_OTEL_TRACES_SAMPLING_RATIO must be > 0 and <= 1")
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
	return errors.New("conversation-service uses verified metadata auth on non-private address without gRPC mTLS client certificate")
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
	batchSize := types.NormalizeMemberChangeProgressLimit(
		envInt("NEXUSIM_MEMBER_CHANGE_PROGRESS_BATCH_SIZE", types.DefaultMemberChangeProgressLimit),
	)
	useCase := app.NewMarkPublishedMemberChangesUseCase(
		repository,
		batchSize,
	)
	pollInterval := envDuration("NEXUSIM_MEMBER_CHANGE_PROGRESS_POLL_INTERVAL", time.Second)
	errorBackoff := envDuration("NEXUSIM_MEMBER_CHANGE_PROGRESS_ERROR_BACKOFF", pollInterval)
	worker := memberchange.NewProgressWorker(
		useCase,
		memberchange.ProgressConfig{
			PollInterval: pollInterval,
			ErrorBackoff: errorBackoff,
			Logf:         log.Printf,
		},
	)
	debugAddr, err := conversationDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr, monitoringinfra.NewHandler(pool).WithMemberChangeWorkerStats(worker.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()
	log.Printf(
		"conversation-service member change progress worker started batch_size=%d poll_interval=%s error_backoff=%s",
		batchSize,
		pollInterval,
		errorBackoff,
	)
	return worker.Run(ctx)
}

func runMemberChangeAudit() error {
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

	updatedAfter, err := envOptionalRFC3339Time("NEXUSIM_CONVERSATION_MEMBER_CHANGE_AUDIT_UPDATED_AFTER")
	if err != nil {
		return err
	}
	updatedBefore, err := envOptionalRFC3339Time("NEXUSIM_CONVERSATION_MEMBER_CHANGE_AUDIT_UPDATED_BEFORE")
	if err != nil {
		return err
	}
	filters := map[string]string{
		"change_id":        envString("NEXUSIM_CONVERSATION_MEMBER_CHANGE_AUDIT_CHANGE_ID", ""),
		"tenant_id":        envString("NEXUSIM_CONVERSATION_MEMBER_CHANGE_AUDIT_TENANT_ID", ""),
		"conversation_id":  envString("NEXUSIM_CONVERSATION_MEMBER_CHANGE_AUDIT_CONVERSATION_ID", ""),
		"target_user_id":   envString("NEXUSIM_CONVERSATION_MEMBER_CHANGE_AUDIT_TARGET_USER_ID", ""),
		"operator_user_id": envString("NEXUSIM_CONVERSATION_MEMBER_CHANGE_AUDIT_OPERATOR_USER_ID", ""),
		"change_type":      envString("NEXUSIM_CONVERSATION_MEMBER_CHANGE_AUDIT_CHANGE_TYPE", ""),
		"status":           envString("NEXUSIM_CONVERSATION_MEMBER_CHANGE_AUDIT_STATUS", ""),
		"outbox_event_id":  envString("NEXUSIM_CONVERSATION_MEMBER_CHANGE_AUDIT_OUTBOX_EVENT_ID", ""),
		"updated_after":    formatAuditFilterTime(updatedAfter),
		"updated_before":   formatAuditFilterTime(updatedBefore),
	}
	rows, err := postgresinfra.NewRepository(pool).AuditMemberChanges(ctx, postgresinfra.MemberChangeAuditOptions{
		ChangeID:       filters["change_id"],
		TenantID:       filters["tenant_id"],
		ConversationID: filters["conversation_id"],
		TargetUserID:   filters["target_user_id"],
		OperatorUserID: filters["operator_user_id"],
		ChangeType:     filters["change_type"],
		Status:         filters["status"],
		OutboxEventID:  filters["outbox_event_id"],
		UpdatedAfter:   updatedAfter,
		UpdatedBefore:  updatedBefore,
		Limit:          envInt("NEXUSIM_CONVERSATION_MEMBER_CHANGE_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("conversation-service member change audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"member_change_saga change_id=%s tenant_id=%s conversation_id=%s target_user_id=%s operator_user_id=%s change_type=%s status=%s boundary_seq=%d timeline_event_id=%s outbox_event_id=%s retry_count=%d next_retry_at=%s dead_lettered_at=%s completed_at=%s last_error=%q",
			row.ChangeID,
			row.TenantID,
			row.ConversationID,
			row.TargetUserID,
			row.OperatorUserID,
			row.ChangeType,
			row.Status,
			row.BoundarySeq,
			row.TimelineEventID,
			row.OutboxEventID,
			row.RetryCount,
			formatOptionalTime(row.NextRetryAt),
			formatOptionalTime(row.DeadLetteredAt),
			formatOptionalTime(row.CompletedAt),
			row.LastError,
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_CONVERSATION_MEMBER_CHANGE_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeMemberChangeAuditOutput(outputPath, rows, filters); err != nil {
			return err
		}
	}
	return nil
}

func runMemberWindowAudit() error {
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

	updatedAfter, err := envOptionalRFC3339Time("NEXUSIM_CONVERSATION_MEMBER_WINDOW_AUDIT_UPDATED_AFTER")
	if err != nil {
		return err
	}
	updatedBefore, err := envOptionalRFC3339Time("NEXUSIM_CONVERSATION_MEMBER_WINDOW_AUDIT_UPDATED_BEFORE")
	if err != nil {
		return err
	}
	filters := map[string]string{
		"tenant_id":       envString("NEXUSIM_CONVERSATION_MEMBER_WINDOW_AUDIT_TENANT_ID", ""),
		"conversation_id": envString("NEXUSIM_CONVERSATION_MEMBER_WINDOW_AUDIT_CONVERSATION_ID", ""),
		"user_id":         envString("NEXUSIM_CONVERSATION_MEMBER_WINDOW_AUDIT_USER_ID", ""),
		"role":            envString("NEXUSIM_CONVERSATION_MEMBER_WINDOW_AUDIT_ROLE", ""),
		"status":          envString("NEXUSIM_CONVERSATION_MEMBER_WINDOW_AUDIT_STATUS", ""),
		"issue_class":     envString("NEXUSIM_CONVERSATION_MEMBER_WINDOW_AUDIT_ISSUE_CLASS", ""),
		"updated_after":   formatAuditFilterTime(updatedAfter),
		"updated_before":  formatAuditFilterTime(updatedBefore),
	}
	rows, err := postgresinfra.NewRepository(pool).AuditMemberWindows(ctx, postgresinfra.MemberWindowAuditOptions{
		TenantID:       filters["tenant_id"],
		ConversationID: filters["conversation_id"],
		UserID:         filters["user_id"],
		Role:           filters["role"],
		Status:         filters["status"],
		IssueClass:     filters["issue_class"],
		UpdatedAfter:   updatedAfter,
		UpdatedBefore:  updatedBefore,
		Limit:          envInt("NEXUSIM_CONVERSATION_MEMBER_WINDOW_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("conversation-service member window audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"conversation_member_window tenant_id=%s conversation_id=%s user_id=%s role=%s status=%s join_seq=%s leave_seq=%s member_version=%d permission_version=%d conversation_member_version=%d conversation_permission_version=%d conversation_status=%s issue_class=%s updated_at=%s",
			row.TenantID,
			row.ConversationID,
			row.UserID,
			row.Role,
			row.Status,
			formatOptionalInt64(row.JoinSeq, row.HasJoinSeq),
			formatOptionalInt64(row.LeaveSeq, row.HasLeaveSeq),
			row.MemberVersion,
			row.PermissionVersion,
			row.ConversationMemberVersion,
			row.ConversationPermissionVersion,
			row.ConversationStatus,
			row.IssueClass,
			row.UpdatedAt.UTC().Format(time.RFC3339Nano),
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_CONVERSATION_MEMBER_WINDOW_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeMemberWindowAuditOutput(outputPath, rows, filters); err != nil {
			return err
		}
	}
	return nil
}

func runMemberWindowRepair() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required")
	}
	dryRun := true
	if value, present, err := envOptionalBool("NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_DRY_RUN"); err != nil {
		return err
	} else if present {
		dryRun = value
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	reason, err := conversationOperatorReasonFromEnv(
		"NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_REASON",
		"NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_REASON_FILE",
		"manual member window repair",
	)
	if err != nil {
		return err
	}

	options := postgresinfra.MemberWindowRepairOptions{
		TenantID:       envString("NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_TENANT_ID", ""),
		ConversationID: envString("NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_CONVERSATION_ID", ""),
		UserID:         envString("NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_USER_ID", ""),
		IssueClass:     envString("NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_ISSUE_CLASS", ""),
		OperatorID:     envString("NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_OPERATOR_ID", "manual"),
		Reason:         reason,
		DryRun:         dryRun,
		Limit:          envInt("NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_LIMIT", 20),
	}
	stats, err := postgresinfra.NewRepository(pool).RepairMemberWindows(ctx, options)
	if err != nil {
		return err
	}
	issueClass := strings.TrimSpace(options.IssueClass)
	if issueClass == "" {
		issueClass = postgresinfra.MemberWindowIssueActiveWithLeaveSeq
	}
	log.Printf(
		"conversation-service member window repair completed requested=%d repaired=%d skipped=%d dry_run=%t issue_class=%s",
		stats.Requested,
		stats.Repaired,
		stats.Skipped,
		stats.DryRun,
		issueClass,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_OUTPUT")); outputPath != "" {
		if err := writeMemberWindowRepairOutput(outputPath, stats, options); err != nil {
			return err
		}
	}
	return nil
}

func runMemberWindowRepairAudit() error {
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

	repairedAfter, err := envOptionalRFC3339Time("NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_AUDIT_REPAIRED_AFTER")
	if err != nil {
		return err
	}
	repairedBefore, err := envOptionalRFC3339Time("NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_AUDIT_REPAIRED_BEFORE")
	if err != nil {
		return err
	}
	filters := map[string]string{
		"tenant_id":       envString("NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_AUDIT_TENANT_ID", ""),
		"conversation_id": envString("NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_AUDIT_CONVERSATION_ID", ""),
		"user_id":         envString("NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_AUDIT_USER_ID", ""),
		"issue_class":     envString("NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_AUDIT_ISSUE_CLASS", ""),
		"outcome":         envString("NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_AUDIT_OUTCOME", ""),
		"repaired_after":  formatAuditFilterTime(repairedAfter),
		"repaired_before": formatAuditFilterTime(repairedBefore),
	}
	rows, err := postgresinfra.NewRepository(pool).AuditMemberWindowRepairs(ctx, postgresinfra.MemberWindowRepairAuditOptions{
		TenantID:       filters["tenant_id"],
		ConversationID: filters["conversation_id"],
		UserID:         filters["user_id"],
		IssueClass:     filters["issue_class"],
		Outcome:        filters["outcome"],
		RepairedAfter:  repairedAfter,
		RepairedBefore: repairedBefore,
		Limit:          envInt("NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("conversation-service member window repair audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"conversation_member_window_repair id=%d tenant_id=%s conversation_id=%s user_id=%s issue_class=%s action=%s outcome=%s previous_join_seq=%s previous_leave_seq=%s new_leave_seq=%s operator_id=%s dry_run=%t repaired_at=%s",
			row.ID,
			row.TenantID,
			row.ConversationID,
			row.UserID,
			row.IssueClass,
			row.RepairAction,
			row.RepairOutcome,
			formatOptionalInt64(row.PreviousJoinSeq, row.HasJoinSeq),
			formatOptionalInt64(row.PreviousLeaveSeq, row.HasLeaveSeq),
			formatOptionalInt64(row.NewLeaveSeq, row.HasNewLeaveSeq),
			row.OperatorID,
			row.DryRun,
			row.RepairedAt.UTC().Format(time.RFC3339Nano),
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeMemberWindowRepairAuditOutput(outputPath, rows, filters); err != nil {
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
			log.Printf("conversation-service debug server stopped with error: %v", err)
		}
	}()
	log.Printf("conversation-service debug server started on %s", addr)
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
	if maxConns := envInt("NEXUSIM_CONVERSATION_PG_MAX_CONNS", 0); maxConns > 0 {
		config.MaxConns = int32(maxConns)
	}
	return pgxpool.NewWithConfig(ctx, config)
}

func envString(name string, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
	}
	return value
}

func conversationDebugAddr() string {
	return envString("NEXUSIM_CONVERSATION_DEBUG_ADDR", envString("NEXUSIM_DEBUG_ADDR", ""))
}

func conversationDebugAddrFromEnv() (string, error) {
	addr := conversationDebugAddr()
	allowPublic, _, err := envOptionalBool("NEXUSIM_CONVERSATION_DEBUG_ALLOW_PUBLIC")
	if err != nil {
		return "", err
	}
	return addr, validateConversationDebugListenerConfig(addr, allowPublic)
}

func validateConversationDebugListenerConfig(addr string, allowPublic bool) error {
	if strings.TrimSpace(addr) == "" {
		return nil
	}
	if listenerAddrTrustedWithoutMTLS(addr) {
		return nil
	}
	if allowPublic {
		return nil
	}
	return errors.New("conversation-service debug listener address is non-private; set NEXUSIM_CONVERSATION_DEBUG_ALLOW_PUBLIC=true to allow")
}

func loadConversationGRPCCredentialsFromEnv() (credentials.TransportCredentials, bool, error) {
	tlsConfig, ok, err := conversationGRPCTLSConfigFromEnv()
	if err != nil || !ok {
		return nil, ok, err
	}
	return credentials.NewTLS(tlsConfig), true, nil
}

func conversationGRPCTLSConfigFromEnv() (*tls.Config, bool, error) {
	certFile := strings.TrimSpace(os.Getenv("NEXUSIM_CONVERSATION_GRPC_TLS_CERT_FILE"))
	keyFile := strings.TrimSpace(os.Getenv("NEXUSIM_CONVERSATION_GRPC_TLS_KEY_FILE"))
	clientCAFile := strings.TrimSpace(os.Getenv("NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_CA_FILE"))
	allowedClientDNSNames := envStringSet("NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", strings.ToLower)
	allowedClientURIs, err := envURIStringSet("NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_ALLOWED_URIS")
	if err != nil {
		return nil, true, err
	}
	requireClientCert, requireClientCertConfigured, err := envOptionalBool("NEXUSIM_CONVERSATION_GRPC_TLS_REQUIRE_CLIENT_CERT")
	if err != nil {
		return nil, true, err
	}
	hasClientAllowlist := len(allowedClientDNSNames) > 0 || len(allowedClientURIs) > 0
	requireClientCert = clientCAFile != "" || hasClientAllowlist || (requireClientCertConfigured && requireClientCert)
	if certFile == "" && keyFile == "" && clientCAFile == "" && !requireClientCert && !hasClientAllowlist {
		return nil, false, nil
	}
	if certFile == "" || keyFile == "" {
		return nil, true, errors.New("NEXUSIM_CONVERSATION_GRPC_TLS_CERT_FILE and NEXUSIM_CONVERSATION_GRPC_TLS_KEY_FILE must be configured together")
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
			return nil, true, errors.New("NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_CA_FILE is required when client certificates are required")
		}
		pemBytes, err := os.ReadFile(clientCAFile)
		if err != nil {
			return nil, true, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, true, errors.New("NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_CA_FILE does not contain a valid PEM certificate")
		}
		tlsConfig.ClientCAs = pool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		if hasClientAllowlist {
			tlsConfig.VerifyConnection = verifyAllowedConversationGRPCClient(allowedClientDNSNames, allowedClientURIs)
		}
	}
	return tlsConfig, true, nil
}

func verifyAllowedConversationGRPCClient(allowedDNSNames map[string]struct{}, allowedURIs map[string]struct{}) func(tls.ConnectionState) error {
	return func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("conversation grpc client certificate is required")
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
		return errors.New("conversation grpc client certificate identity is not allowed")
	}
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

func conversationScaleThresholdsFromEnv() (domain.ConversationScaleThresholds, error) {
	thresholds := domain.DefaultConversationScaleThresholds()
	var err error
	thresholds.SmallGroupMaxActiveMembers, err = envOptionalPositiveInt64(
		"NEXUSIM_CONVERSATION_SCALE_SMALL_MAX",
		thresholds.SmallGroupMaxActiveMembers,
	)
	if err != nil {
		return domain.ConversationScaleThresholds{}, err
	}
	thresholds.MediumGroupMaxActiveMembers, err = envOptionalPositiveInt64(
		"NEXUSIM_CONVERSATION_SCALE_MEDIUM_MAX",
		thresholds.MediumGroupMaxActiveMembers,
	)
	if err != nil {
		return domain.ConversationScaleThresholds{}, err
	}
	thresholds.LargeGroupMaxActiveMembers, err = envOptionalPositiveInt64(
		"NEXUSIM_CONVERSATION_SCALE_LARGE_MAX",
		thresholds.LargeGroupMaxActiveMembers,
	)
	if err != nil {
		return domain.ConversationScaleThresholds{}, err
	}
	if err := thresholds.Validate(); err != nil {
		return domain.ConversationScaleThresholds{}, errors.New("conversation scale thresholds must satisfy small < medium < large")
	}
	return thresholds, nil
}

func envOptionalPositiveInt64(name string, defaultValue int64) (int64, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, errors.New(name + " must be a positive integer")
	}
	return parsed, nil
}

func envInt(name string, defaultValue int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return defaultValue
	}
	return parsed
}

func envOptionalRFC3339Time(name string) (*time.Time, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, errors.New(name + " must be RFC3339")
	}
	utc := parsed.UTC()
	return &utc, nil
}

func formatAuditFilterTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}

func formatOptionalInt64(value int64, ok bool) string {
	if !ok {
		return ""
	}
	return strconv.FormatInt(value, 10)
}

func envDuration(name string, defaultValue time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return defaultValue
	}
	return parsed
}
