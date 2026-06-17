package main

import (
	"flag"
	"os"
	"strings"
	"time"

	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
)

type config struct {
	target                string
	tls                   grpctls.Config
	resultDir             string
	pgDSN                 string
	kafkaBrokers          []string
	contactTopic          string
	requestTimeout        time.Duration
	waitTimeout           time.Duration
	pollInterval          time.Duration
	tenantID              string
	senderUserID          string
	receiverUserID        string
	senderDeviceID        string
	receiverDeviceID      string
	scenario              string
	cleanup               bool
	verifiedMetadata      bool
	gatewayFacade         bool
	gatewayAuthMode       string
	gatewayAuthHMACSecret string
	gatewayAuthAudience   string
	gatewayAuthTokenTTL   time.Duration
}

func parseConfig() config {
	var cfg config
	var brokers string
	flag.StringVar(&cfg.target, "target", "127.0.0.1:10500", "contacts-service gRPC target")
	flag.StringVar(&cfg.tls.CAFile, "contacts-tls-ca-file", "", "CA PEM for contacts-service gRPC TLS")
	flag.StringVar(&cfg.tls.ServerName, "contacts-tls-server-name", "", "server name for contacts-service gRPC TLS")
	flag.StringVar(&cfg.tls.ClientCertFile, "contacts-tls-client-cert-file", "", "client certificate PEM for contacts-service mTLS")
	flag.StringVar(&cfg.tls.ClientKeyFile, "contacts-tls-client-key-file", "", "client private key PEM for contacts-service mTLS")
	flag.StringVar(&cfg.resultDir, "result-dir", "H:\\NexusIM\\loadtest-results\\contacts-smoke", "result directory")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", "", "PostgreSQL DSN")
	flag.StringVar(&brokers, "kafka-brokers", "localhost:9092", "comma-separated Kafka brokers")
	flag.StringVar(&cfg.contactTopic, "contact-topic", "im.contact.events", "contacts Kafka topic")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 5*time.Second, "per-request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 10*time.Second, "wait timeout for outbox/Kafka")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 200*time.Millisecond, "poll interval")
	flag.StringVar(&cfg.tenantID, "tenant-id", "tenant-contacts-smoke", "tenant id")
	flag.StringVar(&cfg.senderUserID, "sender-user-id", "contact-sender", "sender user id")
	flag.StringVar(&cfg.receiverUserID, "receiver-user-id", "contact-receiver", "receiver user id")
	flag.StringVar(&cfg.senderDeviceID, "sender-device-id", "sender-device-1", "sender device id")
	flag.StringVar(&cfg.receiverDeviceID, "receiver-device-id", "receiver-device-1", "receiver device id")
	flag.StringVar(&cfg.scenario, "scenario", "accept", "scenario: accept, decline, cancel, delete, block, unblock, remark, or readd")
	flag.BoolVar(&cfg.cleanup, "cleanup", false, "delete tenant contacts rows before running")
	flag.BoolVar(&cfg.verifiedMetadata, "verified-auth-metadata", envBool("NEXUSIM_CONTACTS_LOADTEST_VERIFIED_AUTH_METADATA", false), "send gateway verified identity through contacts-service gRPC metadata")
	flag.BoolVar(&cfg.gatewayFacade, "gateway-facade", envBool("NEXUSIM_CONTACTS_LOADTEST_GATEWAY_FACADE", false), "use nexusim.gateway.v1.GatewayService for contacts user-facing RPCs")
	flag.StringVar(&cfg.gatewayAuthMode, "gateway-auth-mode", os.Getenv("NEXUSIM_CONTACTS_LOADTEST_GATEWAY_AUTH_MODE"), "api-gateway auth mode for contacts facade calls: mock or hmac")
	flag.StringVar(&cfg.gatewayAuthHMACSecret, "gateway-auth-hmac-secret", os.Getenv("NEXUSIM_CONTACTS_LOADTEST_GATEWAY_AUTH_HMAC_SECRET"), "HMAC secret used to sign api-gateway contacts token when --gateway-auth-mode=hmac")
	flag.StringVar(&cfg.gatewayAuthAudience, "gateway-auth-audience", envString("NEXUSIM_CONTACTS_LOADTEST_GATEWAY_AUTH_AUDIENCE", "api-gateway"), "audience claim used for generated api-gateway contacts token")
	flag.DurationVar(&cfg.gatewayAuthTokenTTL, "gateway-auth-token-ttl", 10*time.Minute, "TTL for generated HMAC api-gateway contacts token")
	flag.Parse()
	cfg.kafkaBrokers = splitCSV(brokers)
	cfg.scenario = strings.ToLower(strings.TrimSpace(cfg.scenario))
	if cfg.scenario == "" {
		cfg.scenario = "accept"
	}
	if cfg.pollInterval <= 0 {
		cfg.pollInterval = 200 * time.Millisecond
	}
	if cfg.requestTimeout <= 0 {
		cfg.requestTimeout = 5 * time.Second
	}
	if cfg.waitTimeout <= 0 {
		cfg.waitTimeout = 10 * time.Second
	}
	return cfg
}
func envBool(name string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "":
		return fallback
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func envString(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}
