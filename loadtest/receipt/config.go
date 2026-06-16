package main

import (
	"flag"
	"os"
	"time"

	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
)

type config struct {
	conversationTarget   string
	messageTarget        string
	deliveryTarget       string
	receiptTarget        string
	conversationTLS      grpctls.Config
	messageTLS           grpctls.Config
	deliveryTLS          grpctls.Config
	receiptTLS           grpctls.Config
	resultDir            string
	pgDSN                string
	requestTimeout       time.Duration
	waitTimeout          time.Duration
	pollInterval         time.Duration
	tenantID             string
	conversationID       string
	ownerUserID          string
	receiverUserID       string
	receiverDeviceID     string
	deliveryGroup        string
	receiptGroup         string
	kafkaBrokers         []string
	receiptEventsTopic   string
	receiptEventsGroup   string
	verifiedAuthMetadata bool
	cleanup              bool
}

func parseConfig() config {
	var cfg config
	flag.StringVar(&cfg.conversationTarget, "conversation-target", "127.0.0.1:10496", "conversation-service gRPC target")
	flag.StringVar(&cfg.messageTarget, "message-target", "127.0.0.1:10495", "message-service gRPC target")
	flag.StringVar(&cfg.deliveryTarget, "delivery-target", "127.0.0.1:10497", "delivery-service gRPC target")
	flag.StringVar(&cfg.receiptTarget, "receipt-target", "127.0.0.1:10499", "receipt-service gRPC target")
	registerTLSFlags("conversation-tls", "NEXUSIM_CONVERSATION_TLS", "conversation-service", &cfg.conversationTLS)
	registerTLSFlags("message-tls", "NEXUSIM_MESSAGE_TLS", "message-service", &cfg.messageTLS)
	registerTLSFlags("delivery-tls", "NEXUSIM_DELIVERY_TLS", "delivery-service", &cfg.deliveryTLS)
	registerTLSFlags("receipt-tls", "NEXUSIM_RECEIPT_TLS", "receipt-service", &cfg.receiptTLS)
	flag.StringVar(&cfg.resultDir, "result-dir", `H:\NexusIM\loadtest-results\receipt-smoke`, "result directory")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", "", "PostgreSQL DSN")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 3*time.Second, "per-request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 20*time.Second, "wait timeout")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 200*time.Millisecond, "poll interval")
	flag.StringVar(&cfg.tenantID, "tenant-id", "tenant-receipt-smoke", "tenant id")
	flag.StringVar(&cfg.conversationID, "conversation-id", "conv-receipt-smoke", "conversation id")
	flag.StringVar(&cfg.ownerUserID, "owner-user-id", "owner-1", "owner/sender user id")
	flag.StringVar(&cfg.receiverUserID, "receiver-user-id", "receipt-user-1", "receiver user id")
	flag.StringVar(&cfg.receiverDeviceID, "receiver-device-id", "receipt-device-1", "receiver device id")
	flag.StringVar(&cfg.deliveryGroup, "delivery-consumer-group", "", "delivery timeline consumer group")
	flag.StringVar(&cfg.receiptGroup, "receipt-consumer-group", "", "receipt delivery event consumer group")
	var kafkaBrokers string
	flag.StringVar(&kafkaBrokers, "kafka-brokers", "localhost:9092", "Kafka brokers for receipt event readback")
	flag.StringVar(&cfg.receiptEventsTopic, "receipt-events-topic", "im.receipt.events", "receipt events topic")
	flag.StringVar(&cfg.receiptEventsGroup, "receipt-events-consumer-group", "", "receipt event readback consumer group")
	flag.BoolVar(&cfg.verifiedAuthMetadata, "verified-auth-metadata", envBool(false, "NEXUSIM_RECEIPT_LOADTEST_VERIFIED_AUTH_METADATA"), "send gateway verified identity through user-facing gRPC metadata")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "delete existing rows for tenant before smoke")
	flag.Parse()
	if cfg.requestTimeout <= 0 {
		cfg.requestTimeout = 3 * time.Second
	}
	if cfg.waitTimeout <= 0 {
		cfg.waitTimeout = 20 * time.Second
	}
	if cfg.pollInterval <= 0 {
		cfg.pollInterval = 200 * time.Millisecond
	}
	cfg.kafkaBrokers = splitCSV(kafkaBrokers)
	return cfg
}

func registerTLSFlags(prefix string, envPrefix string, serviceName string, config *grpctls.Config) {
	flag.StringVar(&config.CAFile, prefix+"-ca-file", os.Getenv(envPrefix+"_CA_FILE"), "CA PEM for "+serviceName+" gRPC TLS")
	flag.StringVar(&config.ServerName, prefix+"-server-name", os.Getenv(envPrefix+"_SERVER_NAME"), "override server name for "+serviceName+" gRPC TLS")
	flag.StringVar(&config.ClientCertFile, prefix+"-client-cert-file", os.Getenv(envPrefix+"_CLIENT_CERT_FILE"), "client certificate PEM for "+serviceName+" gRPC mTLS")
	flag.StringVar(&config.ClientKeyFile, prefix+"-client-key-file", os.Getenv(envPrefix+"_CLIENT_KEY_FILE"), "client private key PEM for "+serviceName+" gRPC mTLS")
}
