package main

import (
	"flag"
	"os"
	"strings"
	"time"

	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
)

type config struct {
	conversationTarget string
	messageTarget      string
	deliveryTarget     string
	receiptTarget      string
	conversationTLS    grpctls.Config
	messageTLS         grpctls.Config
	deliveryTLS        grpctls.Config
	receiptTLS         grpctls.Config
	pushTLS            grpctls.Config
	pushURL            string
	resultDir          string
	pgDSN              string
	policyKafkaBrokers []string
	policyTopic        string
	policyReadbackMin  int64

	tenantID       string
	conversationID string
	senderUserID   string
	receiverUserID string
	receiverDevice string

	requestTimeout time.Duration
	waitTimeout    time.Duration
	pollInterval   time.Duration
	cleanup        bool

	verifiedAuthMetadata  bool
	gatewayFacade         bool
	gatewayAuthMode       string
	gatewayAuthHMACSecret string
	gatewayAuthAudience   string
	gatewayAuthTokenTTL   time.Duration
	pushAuthMode          string
	pushAuthHMACSecret    string
	pushAuthTokenTTL      time.Duration
}

func registerTLSFlags(prefix string, envPrefix string, serviceName string, config *grpctls.Config) {
	flag.StringVar(&config.CAFile, prefix+"-ca-file", os.Getenv(envPrefix+"_CA_FILE"), "CA PEM for "+serviceName+" gRPC TLS")
	flag.StringVar(&config.ServerName, prefix+"-server-name", os.Getenv(envPrefix+"_SERVER_NAME"), "override server name for "+serviceName+" gRPC TLS")
	flag.StringVar(&config.ClientCertFile, prefix+"-client-cert-file", os.Getenv(envPrefix+"_CLIENT_CERT_FILE"), "client certificate PEM for "+serviceName+" gRPC mTLS")
	flag.StringVar(&config.ClientKeyFile, prefix+"-client-key-file", os.Getenv(envPrefix+"_CLIENT_KEY_FILE"), "client private key PEM for "+serviceName+" gRPC mTLS")
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
