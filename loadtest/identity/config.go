package main

import (
	"flag"
	"strings"
	"time"

	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
)

type config struct {
	mode               string
	target             string
	gatewayFacade      bool
	tls                grpctls.Config
	resultDir          string
	pgDSN              string
	webhookListen      string
	webhookFile        string
	webhookBearerToken string
	requestTimeout     time.Duration
	waitTimeout        time.Duration
	pollInterval       time.Duration
	duration           time.Duration
	vus                int
	tenantID           string
	userID             string
	deviceID           string
	audience           string
	password           string
	newPassword        string
	destination        string
	cleanup            bool
}

func parseConfig() config {
	var cfg config
	flag.StringVar(&cfg.mode, "mode", "client", "mode: client or webhook")
	flag.StringVar(&cfg.target, "target", "127.0.0.1:10600", "identity-service or api-gateway gRPC target")
	flag.BoolVar(&cfg.gatewayFacade, "gateway-facade", false, "call identity RPCs through nexusim.gateway.v1.GatewayService facade")
	flag.StringVar(&cfg.tls.CAFile, "identity-tls-ca-file", "", "CA PEM for target gRPC TLS")
	flag.StringVar(&cfg.tls.ServerName, "identity-tls-server-name", "", "server name for target gRPC TLS")
	flag.StringVar(&cfg.tls.ClientCertFile, "identity-tls-client-cert-file", "", "client certificate PEM for target mTLS")
	flag.StringVar(&cfg.tls.ClientKeyFile, "identity-tls-client-key-file", "", "client private key PEM for target mTLS")
	flag.StringVar(&cfg.resultDir, "result-dir", "H:\\NexusIM\\loadtest-results\\identity-challenge-delivery-outbox-smoke", "result directory")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", "", "PostgreSQL DSN")
	flag.StringVar(&cfg.webhookListen, "webhook-listen", "127.0.0.1:0", "webhook listen address for webhook mode")
	flag.StringVar(&cfg.webhookFile, "webhook-file", "", "path where webhook mode writes the last received challenge")
	flag.StringVar(&cfg.webhookBearerToken, "webhook-bearer-token", "", "expected webhook bearer token")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 5*time.Second, "per-request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 20*time.Second, "wait timeout")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 200*time.Millisecond, "poll interval")
	flag.DurationVar(&cfg.duration, "duration", 0, "capacity mode duration; 0 runs the single smoke scenario")
	flag.IntVar(&cfg.vus, "vus", 1, "capacity mode virtual users")
	flag.StringVar(&cfg.tenantID, "tenant-id", "tenant-identity-smoke", "tenant id")
	flag.StringVar(&cfg.userID, "user-id", "identity-user", "user id")
	flag.StringVar(&cfg.deviceID, "device-id", "identity-device", "device id for Login and RefreshGatewayToken")
	flag.StringVar(&cfg.audience, "audience", "api-gateway", "gateway token audience for Login and RefreshGatewayToken")
	flag.StringVar(&cfg.password, "password", "IdentitySmokePassw0rd!", "user password")
	flag.StringVar(&cfg.newPassword, "new-password", "IdentitySmokeResetPassw0rd!", "new password used by ConfirmPasswordReset")
	flag.StringVar(&cfg.destination, "destination", "identity-user@example.com", "verification destination")
	flag.BoolVar(&cfg.cleanup, "cleanup", false, "delete identity rows for this tenant before running")
	flag.Parse()
	cfg.mode = strings.ToLower(strings.TrimSpace(cfg.mode))
	if cfg.vus <= 0 {
		cfg.vus = 1
	}
	if cfg.duration > 0 && cfg.waitTimeout < 30*time.Second {
		cfg.waitTimeout = 30 * time.Second
	}
	return cfg
}
