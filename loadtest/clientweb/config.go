package main

import (
	"flag"
	"os"
	"strings"
	"time"

	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
)

type config struct {
	pgDSN          string
	identityTarget string
	gatewayTarget  string
	bffBaseURL     string
	pushURL        string
	resultDir      string

	identityTLS grpctls.Config
	gatewayTLS  grpctls.Config

	tenantID         string
	conversationID   string
	senderUserID     string
	senderPassword   string
	senderDeviceID   string
	receiverUserID   string
	receiverPassword string
	receiverDeviceID string

	gatewayAuthHMACSecret string
	gatewayAuthAudience   string
	gatewayAuthTokenTTL   time.Duration

	requestTimeout time.Duration
	waitTimeout    time.Duration
	pollInterval   time.Duration
	cleanup        bool
}

func parseConfig() config {
	var cfg config
	flag.StringVar(&cfg.pgDSN, "pg-dsn", envString("NEXUSIM_PG_DSN", "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"), "PostgreSQL DSN used for setup and verification")
	flag.StringVar(&cfg.identityTarget, "identity-target", envString("NEXUSIM_CLIENTWEB_IDENTITY_TARGET", "127.0.0.1:10600"), "identity-service gRPC target used only for smoke setup")
	flag.StringVar(&cfg.gatewayTarget, "gateway-target", envString("NEXUSIM_CLIENTWEB_GATEWAY_TARGET", "127.0.0.1:11903"), "api-gateway gRPC target used only for smoke setup")
	flag.StringVar(&cfg.bffBaseURL, "bff-base-url", envString("NEXUSIM_CLIENTWEB_BFF_BASE_URL", "http://127.0.0.1:11905"), "api-gateway HTTP BFF base URL")
	flag.StringVar(&cfg.pushURL, "push-url", envString("NEXUSIM_CLIENTWEB_PUSH_URL", "ws://127.0.0.1:11898/ws"), "push-gateway WebSocket URL")
	flag.StringVar(&cfg.resultDir, "result-dir", envString("NEXUSIM_CLIENTWEB_RESULT_DIR", "."), "directory for client-web-summary.json")

	registerTLSFlags("identity-tls", "NEXUSIM_CLIENTWEB_IDENTITY_TLS", "identity-service setup gRPC", &cfg.identityTLS)
	registerTLSFlags("gateway-tls", "NEXUSIM_CLIENTWEB_GATEWAY_TLS", "api-gateway setup gRPC", &cfg.gatewayTLS)

	flag.StringVar(&cfg.tenantID, "tenant-id", envString("NEXUSIM_CLIENTWEB_TENANT_ID", "tenant-client-web-smoke"), "tenant id")
	flag.StringVar(&cfg.conversationID, "conversation-id", envString("NEXUSIM_CLIENTWEB_CONVERSATION_ID", "conv-client-web-smoke"), "conversation id")
	flag.StringVar(&cfg.senderUserID, "sender-user-id", envString("NEXUSIM_CLIENTWEB_SENDER_USER_ID", "client-web-sender"), "sender user id")
	flag.StringVar(&cfg.senderPassword, "sender-password", envString("NEXUSIM_CLIENTWEB_SENDER_PASSWORD", "ClientWebSenderPassw0rd!"), "sender password")
	flag.StringVar(&cfg.senderDeviceID, "sender-device-id", envString("NEXUSIM_CLIENTWEB_SENDER_DEVICE_ID", "client-web-sender-device"), "sender device id")
	flag.StringVar(&cfg.receiverUserID, "receiver-user-id", envString("NEXUSIM_CLIENTWEB_RECEIVER_USER_ID", "client-web-receiver"), "receiver user id")
	flag.StringVar(&cfg.receiverPassword, "receiver-password", envString("NEXUSIM_CLIENTWEB_RECEIVER_PASSWORD", "ClientWebReceiverPassw0rd!"), "receiver password")
	flag.StringVar(&cfg.receiverDeviceID, "receiver-device-id", envString("NEXUSIM_CLIENTWEB_RECEIVER_DEVICE_ID", "client-web-receiver-device"), "receiver device id")

	flag.StringVar(&cfg.gatewayAuthHMACSecret, "gateway-auth-hmac-secret", os.Getenv("NEXUSIM_CLIENTWEB_GATEWAY_AUTH_HMAC_SECRET"), "HMAC secret shared by api-gateway and push-gateway for setup/auth")
	flag.StringVar(&cfg.gatewayAuthAudience, "gateway-auth-audience", envString("NEXUSIM_CLIENTWEB_GATEWAY_AUTH_AUDIENCE", "api-gateway"), "api-gateway token audience")
	flag.DurationVar(&cfg.gatewayAuthTokenTTL, "gateway-auth-token-ttl", 10*time.Minute, "setup gateway token TTL")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 5*time.Second, "single request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 30*time.Second, "eventual-consistency wait timeout")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 250*time.Millisecond, "poll interval")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "delete existing rows for tenant before setup")
	flag.Parse()
	return cfg
}

func registerTLSFlags(prefix string, envPrefix string, serviceName string, config *grpctls.Config) {
	flag.StringVar(&config.CAFile, prefix+"-ca-file", os.Getenv(envPrefix+"_CA_FILE"), "CA PEM for "+serviceName)
	flag.StringVar(&config.ServerName, prefix+"-server-name", os.Getenv(envPrefix+"_SERVER_NAME"), "override server name for "+serviceName)
	flag.StringVar(&config.ClientCertFile, prefix+"-client-cert-file", os.Getenv(envPrefix+"_CLIENT_CERT_FILE"), "client certificate PEM for "+serviceName)
	flag.StringVar(&config.ClientKeyFile, prefix+"-client-key-file", os.Getenv(envPrefix+"_CLIENT_KEY_FILE"), "client private key PEM for "+serviceName)
}

func envString(name string, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
	}
	return value
}
