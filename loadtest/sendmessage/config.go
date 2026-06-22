package main

import (
	"errors"
	"flag"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
)

type config struct {
	Target               string
	VUs                  int
	Duration             time.Duration
	ResultDir            string
	RequestTimeout       time.Duration
	StatsWait            time.Duration
	TenantID             string
	ConversationPrefix   string
	ConversationCount    int
	PGDSN                string
	ServiceMetricsURL    string
	RelayMetricsURL      string
	RetryOverloaded      bool
	MaxRetries           int
	RetryJitter          time.Duration
	MessageTLS           grpctls.Config
	VerifiedAuthMetadata bool
}

func parseConfig(args []string, getenv func(string) string) (config, error) {
	defaultResultDir := filepath.Join(`H:\NexusIM\loadtest-results`, time.Now().Format("20060102-150405"))
	cfg := config{}
	flags := flag.NewFlagSet("sendmessage", flag.ContinueOnError)
	flags.StringVar(&cfg.Target, "target", envString(getenv, "NEXUSIM_TARGET", "127.0.0.1:10495"), "gRPC target or comma-separated targets, such as 127.0.0.1:10495 or 127.0.0.1:10495,127.0.0.1:10501")
	flags.IntVar(&cfg.VUs, "vus", envInt(getenv, "NEXUSIM_VUS", 10), "virtual users")
	flags.DurationVar(&cfg.Duration, "duration", envDuration(getenv, "NEXUSIM_DURATION", 30*time.Second), "test duration")
	flags.StringVar(&cfg.ResultDir, "result-dir", envString(getenv, "NEXUSIM_RESULT_DIR", defaultResultDir), "result output directory")
	flags.DurationVar(&cfg.RequestTimeout, "request-timeout", envDuration(getenv, "NEXUSIM_REQUEST_TIMEOUT", 2*time.Second), "per-request timeout")
	flags.DurationVar(&cfg.StatsWait, "stats-wait", envDuration(getenv, "NEXUSIM_STATS_WAIT", 0), "wait after traffic before reading external stats")
	flags.StringVar(&cfg.TenantID, "tenant-id", envString(getenv, "NEXUSIM_TENANT_ID", "tenant-loadtest-"+time.Now().Format("20060102150405")), "tenant id")
	flags.StringVar(&cfg.ConversationPrefix, "conversation-prefix", envString(getenv, "NEXUSIM_CONVERSATION_PREFIX", "conv-loadtest"), "conversation id prefix")
	flags.IntVar(&cfg.ConversationCount, "conversation-count", envInt(getenv, "NEXUSIM_CONVERSATION_COUNT", 1), "number of conversations to spread requests across")
	flags.StringVar(&cfg.PGDSN, "pg-dsn", envString(getenv, "NEXUSIM_PG_DSN", ""), "optional PostgreSQL DSN for outbox stats")
	flags.StringVar(&cfg.ServiceMetricsURL, "service-metrics-url", envString(getenv, "NEXUSIM_SERVICE_METRICS_URL", ""), "optional message-service gRPC process metrics URL or comma-separated URLs")
	flags.StringVar(&cfg.RelayMetricsURL, "relay-metrics-url", envString(getenv, "NEXUSIM_RELAY_METRICS_URL", ""), "optional message-service relay process metrics URL or comma-separated URLs")
	flags.BoolVar(&cfg.RetryOverloaded, "retry-overloaded", envBool(getenv, "NEXUSIM_RETRY_OVERLOADED", false), "retry SERVICE_OVERLOADED using gRPC RetryInfo")
	flags.IntVar(&cfg.MaxRetries, "max-retries", envInt(getenv, "NEXUSIM_MAX_RETRIES", 0), "max retry attempts for one logical request when --retry-overloaded is enabled")
	flags.DurationVar(&cfg.RetryJitter, "retry-jitter", envDurationAllowZero(getenv, "NEXUSIM_RETRY_JITTER", 0), "max deterministic jitter added to overload retry delay")
	flags.BoolVar(&cfg.VerifiedAuthMetadata, "verified-auth-metadata", envBool(getenv, "NEXUSIM_SENDMESSAGE_VERIFIED_AUTH_METADATA", false), "send gateway verified identity through message-service gRPC metadata")
	flags.StringVar(&cfg.MessageTLS.CAFile, "message-tls-ca-file", envString(getenv, "NEXUSIM_MESSAGE_TLS_CA_FILE", ""), "CA PEM for message-service gRPC TLS")
	flags.StringVar(&cfg.MessageTLS.ServerName, "message-tls-server-name", envString(getenv, "NEXUSIM_MESSAGE_TLS_SERVER_NAME", ""), "override server name for message-service gRPC TLS")
	flags.StringVar(&cfg.MessageTLS.ClientCertFile, "message-tls-client-cert-file", envString(getenv, "NEXUSIM_MESSAGE_TLS_CLIENT_CERT_FILE", ""), "client certificate PEM for message-service gRPC mTLS")
	flags.StringVar(&cfg.MessageTLS.ClientKeyFile, "message-tls-client-key-file", envString(getenv, "NEXUSIM_MESSAGE_TLS_CLIENT_KEY_FILE", ""), "client private key PEM for message-service gRPC mTLS")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if cfg.Target == "" {
		return config{}, errors.New("target is required")
	}
	if cfg.VUs <= 0 {
		return config{}, errors.New("vus must be positive")
	}
	if cfg.Duration <= 0 {
		return config{}, errors.New("duration must be positive")
	}
	if cfg.ResultDir == "" {
		return config{}, errors.New("result-dir is required")
	}
	if cfg.RequestTimeout <= 0 {
		return config{}, errors.New("request-timeout must be positive")
	}
	if cfg.ConversationCount <= 0 {
		return config{}, errors.New("conversation-count must be positive")
	}
	if cfg.MaxRetries < 0 {
		return config{}, errors.New("max-retries must be greater than or equal to zero")
	}
	if cfg.RetryJitter < 0 {
		return config{}, errors.New("retry-jitter must be greater than or equal to zero")
	}
	return cfg, nil
}

func envString(getenv func(string) string, name string, defaultValue string) string {
	value := strings.TrimSpace(getenv(name))
	if value == "" {
		return defaultValue
	}
	return value
}

func envInt(getenv func(string) string, name string, defaultValue int) int {
	value := strings.TrimSpace(getenv(name))
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return defaultValue
	}
	return parsed
}

func envBool(getenv func(string) string, name string, defaultValue bool) bool {
	value := strings.TrimSpace(getenv(name))
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func envDuration(getenv func(string) string, name string, defaultValue time.Duration) time.Duration {
	value := strings.TrimSpace(getenv(name))
	if value == "" {
		return defaultValue
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return defaultValue
	}
	return parsed
}

func envDurationAllowZero(getenv func(string) string, name string, defaultValue time.Duration) time.Duration {
	value := strings.TrimSpace(getenv(name))
	if value == "" {
		return defaultValue
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed < 0 {
		return defaultValue
	}
	return parsed
}
