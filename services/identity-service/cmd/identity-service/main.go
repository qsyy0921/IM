package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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
	identitygrpc "github.com/qsyy0921/IM/services/identity-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/identity-service/internal/app"
	credentialinfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/credential"
	kafkainfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/kafka"
	monitoringinfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/monitoring"
	postgresinfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/postgres"
	tokeninfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/token"
	challengedelivery "github.com/qsyy0921/IM/services/identity-service/internal/trigger/challengedelivery"
	"github.com/qsyy0921/IM/services/identity-service/internal/trigger/outbox"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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
		log.Println("identity-service runtime wiring is idle; set NEXUSIM_IDENTITY_SERVICE_MODE=grpc, outbox-relay, challenge-delivery-worker, challenge-delivery-repair, challenge-delivery-repair-audit, challenge-delivery-repair-cleanup, challenge-request-limit-cleanup, session-mfa-proof-audit, or gateway-token-keyring-rotate")
		return nil
	case "grpc":
		return runGRPC()
	case "outbox-relay":
		return runOutboxRelay()
	case "challenge-delivery-worker":
		return runChallengeDeliveryWorker()
	case "challenge-delivery-repair":
		return runChallengeDeliveryRepair()
	case "challenge-delivery-repair-audit":
		return runChallengeDeliveryRepairAudit()
	case "challenge-delivery-repair-cleanup":
		return runChallengeDeliveryRepairCleanup()
	case "challenge-request-limit-cleanup":
		return runChallengeRequestLimitCleanup()
	case "session-mfa-proof-audit":
		return runSessionMFAProofAudit()
	case "gateway-token-keyring-rotate":
		return runGatewayTokenKeyRingRotate()
	default:
		return errors.New("unsupported NEXUSIM_IDENTITY_SERVICE_MODE")
	}
}

func runGRPC() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	addr := envString("NEXUSIM_IDENTITY_GRPC_ADDR", "0.0.0.0:10600")
	authMode := envString("NEXUSIM_IDENTITY_ADMIN_AUTH_MODE", "body")
	serverTLSConfig, serverTLSEnabled, err := identityGRPCTLSConfigFromEnv()
	if err != nil {
		return err
	}
	if err := validateTrustedMetadataListenerConfig(addr, authMode, serverTLSConfig); err != nil {
		return err
	}
	if err := validateIdentityProductionKeyGuardFromEnv(identityRuntimeKeyGuardScope{
		GatewayToken:           true,
		MFA:                    true,
		MFARecovery:            true,
		ChallengeRequestLimit:  true,
		ChallengeDeliveryToken: challengeDeliveryMode() == "outbox",
	}); err != nil {
		return err
	}

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	signer, err := newGatewayTokenSigner()
	if err != nil {
		return err
	}
	refreshTokens := tokeninfra.NewRefreshTokenCodec()
	challengeTokens := tokeninfra.NewChallengeTokenCodec()
	passwords := credentialinfra.NewPBKDF2Hasher(envInt("NEXUSIM_IDENTITY_PASSWORD_PBKDF2_ITERATIONS", 0))
	dummyPasswordHash, err := passwords.HashPassword("nexusim dummy login password")
	if err != nil {
		return err
	}
	mfaManager, err := newMFASecretManager()
	if err != nil {
		return err
	}
	mfaRecoveryCodes, err := newMFARecoveryCodeManager()
	if err != nil {
		return err
	}
	challengeNotifier, challengeDeliveryMode, err := newChallengeNotifier()
	if err != nil {
		return err
	}
	var challengeDeliveryTokens app.ChallengeDeliveryTokenCodec
	if challengeDeliveryMode == "outbox" {
		challengeDeliveryTokens, err = newChallengeDeliveryTokenManager()
		if err != nil {
			return err
		}
	}
	grpcMetrics := monitoringinfra.NewGRPCMetrics()
	traceConfig, err := identityTraceConfigFromEnv()
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
			log.Printf("identity-service OpenTelemetry trace shutdown failed: %v", err)
		}
	}()
	challengeDeliveryMetrics := monitoringinfra.NewChallengeDeliveryMetrics(challengeDeliveryMode)
	challengeNotifier = monitoringinfra.NewInstrumentedChallengeNotifier(challengeNotifier, challengeDeliveryMetrics)
	challengeOptions := app.ChallengeOptions{
		ReturnDevToken:            envBool("NEXUSIM_IDENTITY_DEV_RETURN_CHALLENGE_TOKEN", false),
		Notifier:                  challengeNotifier,
		DeliveryOutbox:            challengeDeliveryMode == "outbox",
		DeliveryTokenCodec:        challengeDeliveryTokens,
		RequestLimitTargetKeySeed: envString("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_SECRET", ""),
	}
	jwkSet, err := gatewayTokenJWKSetWithAdditionalKeys(signer.JWKSet())
	if err != nil {
		return err
	}
	oidcDiscovery, err := identityOIDCDiscoveryFromEnv(signer, jwkSet)
	if err != nil {
		return err
	}
	debugAddr, err := identityDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr, monitoringinfra.NewHandler(pool, grpcMetrics).
		WithJWKSet(jwkSet).
		WithOIDCDiscovery(oidcDiscovery).
		WithChallengeDeliveryMetrics(challengeDeliveryMetrics).
		WithTraceStats(traceRuntime.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	repository := postgresinfra.NewRepository(
		pool,
		postgresinfra.WithChallengeRequestLimit(
			envInt("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_MAX_PER_WINDOW", postgresinfra.DefaultChallengeRequestMaxPerWindow),
			envDuration("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_WINDOW", postgresinfra.DefaultChallengeRequestWindow),
		),
		postgresinfra.WithChallengeRequestLockDuration(
			envDuration("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LOCK_DURATION", postgresinfra.DefaultChallengeRequestLockDuration),
		),
	)
	server, err := newGRPCServerWithConfig(grpcMetrics, authMode, serverTLSConfig, serverTLSEnabled, traceRuntime.UnaryServerInterceptor())
	if err != nil {
		return err
	}
	mfaRiskPolicy := identityMFARiskPolicyFromEnv()
	mfaRecoveryRiskPolicy := identityMFARecoveryRiskPolicyFromEnv()
	identitygrpc.Register(server, identitygrpc.NewServer(
		app.NewRegisterUserUseCase(repository, passwords),
		app.NewLoginUseCase(
			repository,
			signer,
			passwords,
			refreshTokens,
			app.WithLoginRiskPolicy(app.LoginRiskPolicy{
				MaxFailedAttempts: envInt("NEXUSIM_IDENTITY_LOGIN_MAX_FAILED_ATTEMPTS", app.DefaultLoginMaxFailedAttempts),
				FailureWindow:     envDuration("NEXUSIM_IDENTITY_LOGIN_FAILURE_WINDOW", app.DefaultLoginFailureWindow),
				LockDuration:      envDuration("NEXUSIM_IDENTITY_LOGIN_LOCK_DURATION", app.DefaultLoginLockDuration),
			}),
			app.WithLoginMFARiskPolicy(mfaRiskPolicy),
			app.WithLoginMFARecoveryRiskPolicy(mfaRecoveryRiskPolicy),
			app.WithLoginDummyPasswordHash(dummyPasswordHash),
			app.WithLoginMFASecretManager(mfaManager),
			app.WithLoginMFARecoveryCodeManager(mfaRecoveryCodes),
		),
		app.NewRefreshGatewayTokenUseCase(
			repository,
			signer,
			refreshTokens,
			app.WithRefreshMFARiskPolicy(mfaRiskPolicy),
			app.WithRefreshMFARecoveryRiskPolicy(mfaRecoveryRiskPolicy),
			app.WithRefreshMFASecretManager(mfaManager),
			app.WithRefreshMFARecoveryCodeManager(mfaRecoveryCodes),
		),
		app.NewRequestVerificationChallengeUseCase(repository, challengeTokens, passwords, app.ChallengeOptions{
			ReturnDevToken:     challengeOptions.ReturnDevToken,
			Notifier:           challengeOptions.Notifier,
			DeliveryOutbox:     challengeOptions.DeliveryOutbox,
			DeliveryTokenCodec: challengeOptions.DeliveryTokenCodec,
		}),
		app.NewConfirmVerificationChallengeUseCase(repository, challengeTokens),
		app.NewRequestPasswordResetUseCase(repository, challengeTokens, challengeOptions),
		app.NewConfirmPasswordResetUseCase(repository, challengeTokens, passwords),
		app.NewBeginMFAEnrollmentUseCase(repository, passwords, mfaManager),
		app.NewConfirmMFAEnrollmentUseCase(repository, mfaManager, mfaRecoveryCodes),
		app.NewDisableMFAFactorUseCase(repository, passwords),
		app.NewRegenerateMFARecoveryCodesUseCase(repository, passwords, mfaManager, mfaRecoveryCodes),
		app.NewRevokeMFARecoveryCodesUseCase(repository, passwords),
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

	topic := envString("NEXUSIM_IDENTITY_EVENTS_TOPIC", outbox.TopicIdentityEvents)
	relay := outbox.NewRelay(
		postgresinfra.NewOutboxStore(pool),
		producer,
		outbox.Config{
			Topic:          topic,
			BatchSize:      envInt("NEXUSIM_IDENTITY_OUTBOX_BATCH_SIZE", 500),
			PollInterval:   envDuration("NEXUSIM_IDENTITY_OUTBOX_POLL_INTERVAL", time.Second),
			MaxAttempts:    envInt("NEXUSIM_IDENTITY_OUTBOX_MAX_ATTEMPTS", 5),
			RetryBaseDelay: envDuration("NEXUSIM_IDENTITY_OUTBOX_RETRY_BASE_DELAY", time.Second),
			ErrorBackoff:   envDuration("NEXUSIM_IDENTITY_OUTBOX_RELAY_ERROR_BACKOFF", time.Second),
			Logf:           log.Printf,
		},
	)
	debugAddr, err := identityDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr, monitoringinfra.NewHandler(pool).WithOutboxRelayStats(relay.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()
	log.Printf("identity-service outbox relay started topic=%s", topic)
	return relay.Run(ctx)
}

func runChallengeDeliveryWorker() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := validateIdentityProductionKeyGuardFromEnv(identityRuntimeKeyGuardScope{
		ChallengeDeliveryToken: true,
	}); err != nil {
		return err
	}

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	notifier, providerMode, err := newChallengeDeliveryWorkerNotifier()
	if err != nil {
		return err
	}
	challengeDeliveryMetrics := monitoringinfra.NewChallengeDeliveryMetrics("outbox-" + providerMode)

	tokenManager, err := newChallengeDeliveryTokenManager()
	if err != nil {
		return err
	}
	instrumentedNotifier := monitoringinfra.NewInstrumentedChallengeNotifier(notifier, challengeDeliveryMetrics)
	worker := challengedelivery.NewWorker(
		postgresinfra.NewChallengeDeliveryStore(pool),
		instrumentedNotifier,
		tokenManager,
		challengedelivery.Config{
			BatchSize:      envInt("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_BATCH_SIZE", 100),
			PollInterval:   envDuration("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_POLL_INTERVAL", time.Second),
			MaxAttempts:    envInt("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_MAX_ATTEMPTS", 5),
			RetryBaseDelay: envDuration("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_RETRY_BASE_DELAY", time.Second),
			ErrorBackoff:   envDuration("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_ERROR_BACKOFF", time.Second),
			Logf:           log.Printf,
		},
	)
	debugAddr, err := identityDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr, monitoringinfra.NewHandler(pool).
		WithChallengeDeliveryMetrics(challengeDeliveryMetrics).
		WithChallengeDeliveryWorkerStats(worker.Snapshot))
	if err != nil {
		return err
	}
	defer stopDebug()
	log.Printf("identity-service challenge delivery worker started provider=%s", providerMode)
	return worker.Run(ctx)
}

func runChallengeDeliveryRepair() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	ids, err := parseInt64CSV(os.Getenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_IDS"))
	if err != nil {
		return err
	}
	reason, err := identityOperatorReasonFromEnv(
		"NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_REASON",
		"NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_REASON_FILE",
		"manual identity challenge delivery repair",
	)
	if err != nil {
		return err
	}
	stats, err := postgresinfra.NewChallengeDeliveryStore(pool).RepairDeliveries(
		ctx,
		types.ChallengeDeliveryRepairOptions{
			DeliveryIDs: ids,
			Mode:        envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_MODE", types.ChallengeDeliveryRepairModeAudit),
			Operator:    envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_OPERATOR", "manual"),
			Reason:      reason,
			DryRun:      envBool("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_DRY_RUN", false),
		})
	if err != nil {
		return err
	}
	mode := envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_MODE", types.ChallengeDeliveryRepairModeAudit)
	dryRun := envBool("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_DRY_RUN", false)
	log.Printf(
		"identity-service challenge delivery repair completed requested=%d audited=%d mutated=%d skipped=%d mode=%s dry_run=%t",
		stats.Requested,
		stats.Audited,
		stats.Mutated,
		stats.Skipped,
		mode,
		dryRun,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_OUTPUT")); outputPath != "" {
		if err := writeChallengeDeliveryRepairOutput(outputPath, stats, len(ids), mode, dryRun); err != nil {
			return err
		}
	}
	return nil
}

func runChallengeDeliveryRepairAudit() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	deliveryID, err := optionalPositiveInt64Env("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_DELIVERY_ID")
	if err != nil {
		return err
	}
	repairedAfter, err := optionalRFC3339TimeEnv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_REPAIRED_AFTER")
	if err != nil {
		return err
	}
	repairedBefore, err := optionalRFC3339TimeEnv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_REPAIRED_BEFORE")
	if err != nil {
		return err
	}
	filters := map[string]string{
		"tenant_id":              envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_TENANT_ID", ""),
		"user_id":                envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_USER_ID", ""),
		"challenge_id":           envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_CHALLENGE_ID", ""),
		"mode":                   envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_MODE", ""),
		"outcome":                envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_OUTCOME", ""),
		"previous_failure_class": envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_PREVIOUS_FAILURE_CLASS", ""),
		"new_failure_class":      envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_NEW_FAILURE_CLASS", ""),
		"repaired_after":         formatOptionalTime(repairedAfter),
		"repaired_before":        formatOptionalTime(repairedBefore),
	}
	if deliveryID != nil {
		filters["delivery_id"] = strconv.FormatInt(*deliveryID, 10)
	}
	rows, err := postgresinfra.NewChallengeDeliveryStore(pool).AuditDeliveryRepairs(ctx, postgresinfra.ChallengeDeliveryRepairAuditOptions{
		DeliveryID:           deliveryID,
		TenantID:             filters["tenant_id"],
		UserID:               filters["user_id"],
		ChallengeID:          filters["challenge_id"],
		Mode:                 filters["mode"],
		Outcome:              filters["outcome"],
		PreviousFailureClass: filters["previous_failure_class"],
		NewFailureClass:      filters["new_failure_class"],
		RepairedAfter:        repairedAfter,
		RepairedBefore:       repairedBefore,
		Limit:                envInt("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("identity-service challenge delivery repair audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"identity_challenge_delivery_repair delivery_id=%d tenant_id=%s user_id=%s challenge_id=%s mode=%s outcome=%s skip_reason=%s dry_run=%t previous_delivery_status=%s previous_challenge_status=%s previous_challenge_delivery_status=%s previous_retry_count=%d previous_failure_class=%s new_delivery_status=%s new_challenge_status=%s new_challenge_delivery_status=%s new_failure_class=%s operator=%s repaired_at=%s reason=%q previous_dead_lettered_at=%s",
			row.DeliveryID,
			row.TenantID,
			row.UserID,
			row.ChallengeID,
			row.Mode,
			row.Outcome,
			row.SkipReason,
			row.DryRun,
			row.PreviousDeliveryStatus,
			row.PreviousChallengeStatus,
			row.PreviousChallengeDeliveryStatus,
			row.PreviousRetryCount,
			row.PreviousFailureClass,
			row.NewDeliveryStatus,
			row.NewChallengeStatus,
			row.NewChallengeDeliveryStatus,
			row.NewFailureClass,
			row.Operator,
			row.RepairedAt.Format(time.RFC3339),
			row.Reason,
			formatOptionalTime(row.PreviousDeadLetteredAt),
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeChallengeDeliveryRepairAuditOutput(outputPath, rows, filters); err != nil {
			return err
		}
	}
	return nil
}

func runChallengeDeliveryRepairCleanup() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	config, err := challengeDeliveryRepairCleanupConfigFromEnv()
	if err != nil {
		return err
	}
	deliveryID, err := optionalPositiveInt64Env("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_DELIVERY_ID")
	if err != nil {
		return err
	}
	cutoff := time.Now().UTC().Add(-config.Retention)
	stats, err := postgresinfra.NewChallengeDeliveryStore(pool).CleanupDeliveryRepairs(ctx, postgresinfra.ChallengeDeliveryRepairCleanupOptions{
		DeliveryID:           deliveryID,
		TenantID:             envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_TENANT_ID", ""),
		UserID:               envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_USER_ID", ""),
		ChallengeID:          envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_CHALLENGE_ID", ""),
		Mode:                 envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_MODE", ""),
		Outcome:              envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_OUTCOME", ""),
		PreviousFailureClass: envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_PREVIOUS_FAILURE_CLASS", ""),
		NewFailureClass:      envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_NEW_FAILURE_CLASS", ""),
		Cutoff:               cutoff,
		Limit:                config.BatchSize,
		DryRun:               config.DryRun,
	})
	if err != nil {
		return err
	}
	log.Printf(
		"identity-service challenge delivery repair cleanup completed deleted=%d cutoff=%s retention=%s batch_size=%d dry_run=%t",
		stats.Deleted,
		cutoff.Format(time.RFC3339),
		config.Retention,
		config.BatchSize,
		config.DryRun,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_OUTPUT")); outputPath != "" {
		filters := map[string]string{
			"tenant_id":              envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_TENANT_ID", ""),
			"user_id":                envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_USER_ID", ""),
			"challenge_id":           envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_CHALLENGE_ID", ""),
			"mode":                   envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_MODE", ""),
			"outcome":                envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_OUTCOME", ""),
			"previous_failure_class": envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_PREVIOUS_FAILURE_CLASS", ""),
			"new_failure_class":      envString("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_NEW_FAILURE_CLASS", ""),
		}
		if deliveryID != nil {
			filters["delivery_id"] = strconv.FormatInt(*deliveryID, 10)
		}
		if err := writeOperatorCleanupOutput(outputPath, stats.Deleted, cutoff, config.Retention, config.BatchSize, config.DryRun, filters); err != nil {
			return err
		}
	}
	return nil
}

func runChallengeRequestLimitCleanup() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	config, err := challengeRequestLimitCleanupConfigFromEnv()
	if err != nil {
		return err
	}
	cutoff := time.Now().UTC().Add(-config.Retention)
	deleted, err := postgresinfra.NewRepository(pool).CleanupChallengeRequestLimits(ctx, cutoff, config.BatchSize, config.DryRun)
	if err != nil {
		return err
	}
	log.Printf(
		"identity-service challenge request limit cleanup completed deleted=%d cutoff=%s retention=%s batch_size=%d dry_run=%t",
		deleted,
		cutoff.Format(time.RFC3339),
		config.Retention,
		config.BatchSize,
		config.DryRun,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_CLEANUP_OUTPUT")); outputPath != "" {
		if err := writeOperatorCleanupOutput(outputPath, deleted, cutoff, config.Retention, config.BatchSize, config.DryRun, nil); err != nil {
			return err
		}
	}
	return nil
}

func runSessionMFAProofAudit() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	stats, err := postgresinfra.NewRepository(pool).AuditSessionMFAProofConstraints(ctx)
	if err != nil {
		return err
	}
	log.Printf(
		"identity-service session mfa proof audit completed invalid_total=%d unknown_method=%d empty_method_with_proof=%d totp_missing_proof=%d recovery_invalid_proof=%d",
		stats.InvalidTotal,
		stats.UnknownMethod,
		stats.EmptyMethodWithProof,
		stats.TOTPMissingProof,
		stats.RecoveryInvalidProof,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_SESSION_MFA_PROOF_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeSessionMFAProofAuditOutput(outputPath, stats); err != nil {
			return err
		}
	}
	if stats.InvalidTotal > 0 {
		return fmt.Errorf("identity session mfa proof audit found %d invalid rows", stats.InvalidTotal)
	}
	return nil
}

type challengeRequestLimitCleanupConfig struct {
	Retention time.Duration
	BatchSize int
	DryRun    bool
}

type challengeDeliveryRepairCleanupConfig struct {
	Retention time.Duration
	BatchSize int
	DryRun    bool
}

func challengeDeliveryRepairCleanupConfigFromEnv() (challengeDeliveryRepairCleanupConfig, error) {
	retention, err := envPositiveDuration("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_RETENTION", 7*24*time.Hour)
	if err != nil {
		return challengeDeliveryRepairCleanupConfig{}, err
	}
	batchSize, err := envPositiveInt("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_BATCH_SIZE", 5000)
	if err != nil {
		return challengeDeliveryRepairCleanupConfig{}, err
	}
	return challengeDeliveryRepairCleanupConfig{
		Retention: retention,
		BatchSize: batchSize,
		DryRun:    envBool("NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_DRY_RUN", false),
	}, nil
}

func challengeRequestLimitCleanupConfigFromEnv() (challengeRequestLimitCleanupConfig, error) {
	retention, err := envPositiveDuration("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_RETENTION", 24*time.Hour)
	if err != nil {
		return challengeRequestLimitCleanupConfig{}, err
	}
	batchSize, err := envPositiveInt("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_CLEANUP_BATCH_SIZE", 5000)
	if err != nil {
		return challengeRequestLimitCleanupConfig{}, err
	}
	config := challengeRequestLimitCleanupConfig{
		Retention: retention,
		BatchSize: batchSize,
		DryRun:    envBool("NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_CLEANUP_DRY_RUN", false),
	}
	return config, nil
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

func identityDebugAddrFromEnv() (string, error) {
	addr := identityDebugAddr()
	allowPublic, _, err := envOptionalBool("NEXUSIM_IDENTITY_DEBUG_ALLOW_PUBLIC")
	if err != nil {
		return "", err
	}
	return addr, validateIdentityDebugListenerConfig(addr, allowPublic)
}

func validateIdentityDebugListenerConfig(addr string, allowPublic bool) error {
	if strings.TrimSpace(addr) == "" {
		return nil
	}
	if listenerAddrTrustedWithoutMTLS(addr) {
		return nil
	}
	if allowPublic {
		return nil
	}
	return errors.New("identity-service debug listener address is non-private; set NEXUSIM_IDENTITY_DEBUG_ALLOW_PUBLIC=true to allow")
}

func newGRPCServer(grpcMetrics *monitoringinfra.GRPCMetrics) (*grpc.Server, error) {
	authMode := envString("NEXUSIM_IDENTITY_ADMIN_AUTH_MODE", "body")
	tlsConfig, tlsEnabled, err := identityGRPCTLSConfigFromEnv()
	if err != nil {
		return nil, err
	}
	return newGRPCServerWithConfig(grpcMetrics, authMode, tlsConfig, tlsEnabled)
}

func newGRPCServerWithConfig(grpcMetrics *monitoringinfra.GRPCMetrics, authMode string, tlsConfig *tls.Config, tlsEnabled bool, traceInterceptors ...grpc.UnaryServerInterceptor) (*grpc.Server, error) {
	options := make([]grpc.ServerOption, 0, 2)
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
		interceptors = append(interceptors, identitygrpc.VerifiedAdminUnaryInterceptor(true))
	default:
		return nil, errors.New("unsupported NEXUSIM_IDENTITY_ADMIN_AUTH_MODE")
	}
	if len(interceptors) > 0 {
		options = append(options, grpc.ChainUnaryInterceptor(interceptors...))
	}
	if tlsEnabled {
		options = append(options, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}
	return grpc.NewServer(options...), nil
}

func identityTraceConfigFromEnv() (monitoringinfra.TraceConfig, error) {
	enabled, _, err := envOptionalBool("NEXUSIM_IDENTITY_OTEL_TRACES_ENABLED")
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	otlpInsecure, _, err := envOptionalBool("NEXUSIM_IDENTITY_OTEL_TRACES_OTLP_INSECURE")
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	samplingRatio, err := identityTraceSamplingRatioFromEnv()
	if err != nil {
		return monitoringinfra.TraceConfig{}, err
	}
	return monitoringinfra.TraceConfig{
		Enabled:       enabled,
		ServiceName:   envString("NEXUSIM_IDENTITY_OTEL_SERVICE_NAME", "identity-service"),
		Exporter:      envString("NEXUSIM_IDENTITY_OTEL_TRACES_EXPORTER", "stdout"),
		OTLPEndpoint:  envString("NEXUSIM_IDENTITY_OTEL_TRACES_OTLP_ENDPOINT", ""),
		OTLPInsecure:  otlpInsecure,
		SamplingRatio: samplingRatio,
	}, nil
}

func identityTraceSamplingRatioFromEnv() (float64, error) {
	raw := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_OTEL_TRACES_SAMPLING_RATIO"))
	if raw == "" {
		return 1, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 || value > 1 {
		return 0, errors.New("NEXUSIM_IDENTITY_OTEL_TRACES_SAMPLING_RATIO must be > 0 and <= 1")
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
	return errors.New("identity-service uses verified metadata auth on non-private address without gRPC mTLS client certificate")
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

func loadIdentityGRPCCredentialsFromEnv() (credentials.TransportCredentials, bool, error) {
	tlsConfig, ok, err := identityGRPCTLSConfigFromEnv()
	if err != nil || !ok {
		return nil, ok, err
	}
	return credentials.NewTLS(tlsConfig), true, nil
}

func identityGRPCTLSConfigFromEnv() (*tls.Config, bool, error) {
	certFile := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GRPC_TLS_CERT_FILE"))
	keyFile := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GRPC_TLS_KEY_FILE"))
	clientCAFile := strings.TrimSpace(os.Getenv("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_CA_FILE"))
	allowedClientDNSNames := envStringSet("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES", strings.ToLower)
	allowedClientURIs, err := envURIStringSet("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_ALLOWED_URIS")
	if err != nil {
		return nil, true, err
	}
	requireClientCert, requireClientCertConfigured, err := envOptionalBool("NEXUSIM_IDENTITY_GRPC_TLS_REQUIRE_CLIENT_CERT")
	if err != nil {
		return nil, true, err
	}
	hasClientAllowlist := len(allowedClientDNSNames) > 0 || len(allowedClientURIs) > 0
	requireClientCert = clientCAFile != "" || hasClientAllowlist || (requireClientCertConfigured && requireClientCert)
	if certFile == "" && keyFile == "" && clientCAFile == "" && !requireClientCert && !hasClientAllowlist {
		return nil, false, nil
	}
	if certFile == "" || keyFile == "" {
		return nil, true, errors.New("NEXUSIM_IDENTITY_GRPC_TLS_CERT_FILE and NEXUSIM_IDENTITY_GRPC_TLS_KEY_FILE must be configured together")
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
			return nil, true, errors.New("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_CA_FILE is required when client certificates are required")
		}
		pemBytes, err := os.ReadFile(clientCAFile)
		if err != nil {
			return nil, true, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, true, errors.New("NEXUSIM_IDENTITY_GRPC_TLS_CLIENT_CA_FILE does not contain a valid PEM certificate")
		}
		tlsConfig.ClientCAs = pool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		if hasClientAllowlist {
			tlsConfig.VerifyConnection = verifyAllowedIdentityGRPCClient(allowedClientDNSNames, allowedClientURIs)
		}
	}
	return tlsConfig, true, nil
}

func verifyAllowedIdentityGRPCClient(allowedDNSNames map[string]struct{}, allowedURIs map[string]struct{}) func(tls.ConnectionState) error {
	return func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("identity grpc client certificate is required")
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
		return errors.New("identity grpc client certificate identity is not allowed")
	}
}
