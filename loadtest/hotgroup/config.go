package main

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
)

const (
	runnerModeFull           = "full"
	runnerModeSubscriberOnly = "subscriber-only"
)

type config struct {
	RunName                       string
	ResultRoot                    string
	ResultDir                     string
	RunnerMode                    string
	DryRun                        bool
	Cleanup                       bool
	RequestTimeout                time.Duration
	WaitTimeout                   time.Duration
	PollInterval                  time.Duration
	ConversationTarget            string
	MessageTarget                 string
	DeliveryTarget                string
	PushURL                       string
	ConversationTLS               grpctls.Config
	MessageTLS                    grpctls.Config
	DeliveryTLS                   grpctls.Config
	VerifiedAuthMetadata          bool
	PGDSN                         string
	TenantID                      string
	ConversationID                string
	GroupSize                     int
	SenderCount                   int
	OnlineRatio                   float64
	SlowClientRatio               float64
	ACKRatio                      float64
	MessageRate                   float64
	Duration                      time.Duration
	MessageCount                  int
	ExpectedFanoutMode            string
	RequireDeliveryOutboxDrain    bool
	PullLimit                     int32
	ReceiverSampleCount           int
	ConversationSubscriberCount   int
	SubscriberShardCount          int
	SubscriberShardIndex          int
	RequireConversationNotify     bool
	ConversationSignalSampleEvery int
}

func parseConfig(args []string, getenv func(string) string) (config, error) {
	now := time.Now().Format("20060102-150405")
	cfg := config{}
	flags := flag.NewFlagSet("hotgroup", flag.ContinueOnError)
	flags.StringVar(&cfg.RunName, "run-name", envString(getenv, "NEXUSIM_HOTGROUP_RUN_NAME", "hotgroup-"+now), "run name")
	flags.StringVar(&cfg.ResultRoot, "result-root", envString(getenv, "NEXUSIM_RESULT_ROOT", `H:\NexusIM\loadtest-results`), "result root directory")
	flags.StringVar(&cfg.RunnerMode, "runner-mode", envString(getenv, "NEXUSIM_HOTGROUP_RUNNER_MODE", runnerModeFull), "runner mode: full or subscriber-only")
	flags.BoolVar(&cfg.DryRun, "dry-run", envBool(getenv, "NEXUSIM_HOTGROUP_DRY_RUN", false), "write user model and summary without contacting services")
	flags.BoolVar(&cfg.Cleanup, "cleanup", envBool(getenv, "NEXUSIM_HOTGROUP_CLEANUP", false), "delete rows for the configured tenant before running; requires --pg-dsn")
	flags.DurationVar(&cfg.RequestTimeout, "request-timeout", envDuration(getenv, "NEXUSIM_REQUEST_TIMEOUT", 3*time.Second), "per-request timeout")
	flags.DurationVar(&cfg.WaitTimeout, "wait-timeout", envDuration(getenv, "NEXUSIM_WAIT_TIMEOUT", 60*time.Second), "max wait for async projections")
	flags.DurationVar(&cfg.PollInterval, "poll-interval", envDuration(getenv, "NEXUSIM_POLL_INTERVAL", 500*time.Millisecond), "async projection poll interval")
	flags.StringVar(&cfg.ConversationTarget, "conversation-target", envString(getenv, "NEXUSIM_CONVERSATION_TARGET", "127.0.0.1:13096"), "conversation-service gRPC target")
	flags.StringVar(&cfg.MessageTarget, "message-target", envString(getenv, "NEXUSIM_MESSAGE_TARGET", "127.0.0.1:13095"), "message-service gRPC target")
	flags.StringVar(&cfg.DeliveryTarget, "delivery-target", envString(getenv, "NEXUSIM_DELIVERY_TARGET", "127.0.0.1:13097"), "delivery-service gRPC target")
	flags.StringVar(&cfg.PushURL, "push-url", envString(getenv, "NEXUSIM_PUSH_URL", ""), "optional push-gateway WebSocket URL used for conversation signal verification")
	flags.BoolVar(&cfg.VerifiedAuthMetadata, "verified-auth-metadata", envBool(getenv, "NEXUSIM_HOTGROUP_VERIFIED_AUTH_METADATA", false), "send gateway verified identity through gRPC metadata")
	flags.StringVar(&cfg.PGDSN, "pg-dsn", envString(getenv, "NEXUSIM_PG_DSN", ""), "PostgreSQL DSN for cleanup, async waits and stats")
	flags.StringVar(&cfg.TenantID, "tenant-id", envString(getenv, "NEXUSIM_TENANT_ID", "tenant-hotgroup-"+now), "tenant id")
	flags.StringVar(&cfg.ConversationID, "conversation-id", envString(getenv, "NEXUSIM_CONVERSATION_ID", "conv-hotgroup-"+now), "conversation id")
	flags.IntVar(&cfg.GroupSize, "group-size", envInt(getenv, "NEXUSIM_HOTGROUP_GROUP_SIZE", 100), "total group member count including owner and senders")
	flags.IntVar(&cfg.SenderCount, "sender-count", envInt(getenv, "NEXUSIM_HOTGROUP_SENDER_COUNT", 5), "number of active senders")
	flags.Float64Var(&cfg.OnlineRatio, "online-ratio", envFloat(getenv, "NEXUSIM_HOTGROUP_ONLINE_RATIO", 0.2), "receiver online ratio")
	flags.Float64Var(&cfg.SlowClientRatio, "slow-client-ratio", envFloat(getenv, "NEXUSIM_HOTGROUP_SLOW_CLIENT_RATIO", 0.0), "slow client ratio among receivers; modeled in user plan only in v0.1")
	flags.Float64Var(&cfg.ACKRatio, "ack-ratio", envFloat(getenv, "NEXUSIM_HOTGROUP_ACK_RATIO", 0.8), "ratio of sampled receivers that ack after pull")
	flags.Float64Var(&cfg.MessageRate, "message-rate", envFloat(getenv, "NEXUSIM_HOTGROUP_MESSAGE_RATE", 10), "target message rate per second")
	flags.DurationVar(&cfg.Duration, "duration", envDuration(getenv, "NEXUSIM_DURATION", 60*time.Second), "traffic duration")
	flags.IntVar(&cfg.MessageCount, "message-count", envInt(getenv, "NEXUSIM_HOTGROUP_MESSAGE_COUNT", 0), "explicit total message count; zero derives from message-rate * duration")
	flags.StringVar(&cfg.ExpectedFanoutMode, "expect-fanout-mode", envString(getenv, "NEXUSIM_HOTGROUP_EXPECT_FANOUT_MODE", ""), "optional required conversation fanout mode after member promotion")
	flags.BoolVar(&cfg.RequireDeliveryOutboxDrain, "require-delivery-outbox-drain", envBool(getenv, "NEXUSIM_HOTGROUP_REQUIRE_DELIVERY_OUTBOX_DRAIN", false), "wait for delivery_outbox PENDING rows to drain before marking success")
	pullLimit := envInt(getenv, "NEXUSIM_HOTGROUP_PULL_LIMIT", 100)
	flags.IntVar(&pullLimit, "pull-limit", pullLimit, "PullInbox limit per sampled receiver")
	flags.IntVar(&cfg.ReceiverSampleCount, "receiver-sample-count", envInt(getenv, "NEXUSIM_HOTGROUP_RECEIVER_SAMPLE_COUNT", 10), "number of receivers used for PullInbox/AckDelivery sampling")
	flags.IntVar(&cfg.ConversationSubscriberCount, "conversation-subscriber-count", envInt(getenv, "NEXUSIM_HOTGROUP_CONVERSATION_SUBSCRIBER_COUNT", 0), "number of receivers that subscribe to conversation signals over WebSocket")
	flags.IntVar(&cfg.SubscriberShardCount, "subscriber-shard-count", envInt(getenv, "NEXUSIM_HOTGROUP_SUBSCRIBER_SHARD_COUNT", 1), "number of subscriber runner shards for conversation signal reading")
	flags.IntVar(&cfg.SubscriberShardIndex, "subscriber-shard-index", envInt(getenv, "NEXUSIM_HOTGROUP_SUBSCRIBER_SHARD_INDEX", 0), "zero-based subscriber runner shard index")
	flags.BoolVar(&cfg.RequireConversationNotify, "require-conversation-notify", envBool(getenv, "NEXUSIM_HOTGROUP_REQUIRE_CONVERSATION_NOTIFY", false), "require each WebSocket subscriber to receive at least one conversation signal")
	flags.IntVar(&cfg.ConversationSignalSampleEvery, "conversation-signal-sample-every", envInt(getenv, "NEXUSIM_HOTGROUP_CONVERSATION_SIGNAL_SAMPLE_EVERY", 1), "expected conversation signal sampling interval; 1 requires every message signal")
	registerTLSFlags(flags, "conversation-tls", "NEXUSIM_CONVERSATION_TLS", "conversation-service", &cfg.ConversationTLS, getenv)
	registerTLSFlags(flags, "message-tls", "NEXUSIM_MESSAGE_TLS", "message-service", &cfg.MessageTLS, getenv)
	registerTLSFlags(flags, "delivery-tls", "NEXUSIM_DELIVERY_TLS", "delivery-service", &cfg.DeliveryTLS, getenv)
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	cfg.PullLimit = int32(pullLimit)
	cfg.ResultDir = filepath.Join(cfg.ResultRoot, cfg.RunName)
	return cfg, cfg.validate()
}

func (cfg config) validate() error {
	if strings.TrimSpace(cfg.RunName) == "" {
		return errors.New("--run-name is required")
	}
	if strings.TrimSpace(cfg.ResultRoot) == "" {
		return errors.New("--result-root is required")
	}
	if cfg.RequestTimeout <= 0 {
		return errors.New("--request-timeout must be positive")
	}
	if cfg.WaitTimeout <= 0 {
		return errors.New("--wait-timeout must be positive")
	}
	if cfg.PollInterval <= 0 {
		return errors.New("--poll-interval must be positive")
	}
	switch cfg.RunnerMode {
	case runnerModeFull, runnerModeSubscriberOnly:
	default:
		return errors.New("--runner-mode must be full or subscriber-only")
	}
	if !cfg.DryRun && cfg.RunnerMode != runnerModeSubscriberOnly && strings.TrimSpace(cfg.PGDSN) == "" {
		return errors.New("--pg-dsn is required for non-dry-run hotgroup loadtest")
	}
	if cfg.RunnerMode == runnerModeSubscriberOnly && cfg.Cleanup {
		return errors.New("--cleanup cannot be used with --runner-mode subscriber-only")
	}
	if strings.TrimSpace(cfg.TenantID) == "" {
		return errors.New("--tenant-id is required")
	}
	if strings.TrimSpace(cfg.ConversationID) == "" {
		return errors.New("--conversation-id is required")
	}
	if cfg.GroupSize < 2 {
		return errors.New("--group-size must be at least 2")
	}
	if cfg.SenderCount <= 0 {
		return errors.New("--sender-count must be positive")
	}
	if cfg.SenderCount >= cfg.GroupSize {
		return errors.New("--sender-count must be smaller than --group-size")
	}
	if cfg.OnlineRatio < 0 || cfg.OnlineRatio > 1 {
		return errors.New("--online-ratio must be between 0 and 1")
	}
	if cfg.SlowClientRatio < 0 || cfg.SlowClientRatio > 1 {
		return errors.New("--slow-client-ratio must be between 0 and 1")
	}
	if cfg.ACKRatio < 0 || cfg.ACKRatio > 1 {
		return errors.New("--ack-ratio must be between 0 and 1")
	}
	if cfg.MessageRate <= 0 {
		return errors.New("--message-rate must be positive")
	}
	if cfg.Duration <= 0 {
		return errors.New("--duration must be positive")
	}
	if cfg.MessageCount < 0 {
		return errors.New("--message-count must be greater than or equal to zero")
	}
	switch cfg.ExpectedFanoutMode {
	case "", "WRITE_FANOUT", "HYBRID_FANOUT", "READ_FANOUT", "BROADCAST_SIGNAL":
	default:
		return errors.New("--expect-fanout-mode must be WRITE_FANOUT, HYBRID_FANOUT, READ_FANOUT or BROADCAST_SIGNAL")
	}
	if cfg.PullLimit <= 0 {
		return errors.New("--pull-limit must be positive")
	}
	if cfg.ReceiverSampleCount <= 0 {
		return errors.New("--receiver-sample-count must be positive")
	}
	if cfg.ConversationSubscriberCount < 0 {
		return errors.New("--conversation-subscriber-count must be greater than or equal to zero")
	}
	if cfg.SubscriberShardCount <= 0 {
		return errors.New("--subscriber-shard-count must be positive")
	}
	if cfg.SubscriberShardIndex < 0 || cfg.SubscriberShardIndex >= cfg.SubscriberShardCount {
		return errors.New("--subscriber-shard-index must be greater than or equal to zero and smaller than --subscriber-shard-count")
	}
	if cfg.ConversationSubscriberCount > 0 && strings.TrimSpace(cfg.PushURL) == "" {
		return errors.New("--push-url is required when --conversation-subscriber-count is positive")
	}
	if cfg.RequireConversationNotify && cfg.ConversationSubscriberCount == 0 {
		return errors.New("--require-conversation-notify requires --conversation-subscriber-count")
	}
	if cfg.ConversationSignalSampleEvery <= 0 {
		return errors.New("--conversation-signal-sample-every must be positive")
	}
	if cfg.RunnerMode == runnerModeSubscriberOnly && cfg.ConversationSubscriberCount == 0 {
		return errors.New("--runner-mode subscriber-only requires --conversation-subscriber-count")
	}
	return nil
}

func registerTLSFlags(flags *flag.FlagSet, prefix string, envPrefix string, serviceName string, config *grpctls.Config, getenv func(string) string) {
	flags.StringVar(&config.CAFile, prefix+"-ca-file", envString(getenv, envPrefix+"_CA_FILE", ""), "CA PEM for "+serviceName+" gRPC TLS")
	flags.StringVar(&config.ServerName, prefix+"-server-name", envString(getenv, envPrefix+"_SERVER_NAME", ""), "override server name for "+serviceName+" gRPC TLS")
	flags.StringVar(&config.ClientCertFile, prefix+"-client-cert-file", envString(getenv, envPrefix+"_CLIENT_CERT_FILE", ""), "client certificate PEM for "+serviceName+" gRPC mTLS")
	flags.StringVar(&config.ClientKeyFile, prefix+"-client-key-file", envString(getenv, envPrefix+"_CLIENT_KEY_FILE", ""), "client private key PEM for "+serviceName+" gRPC mTLS")
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
	if err != nil {
		return defaultValue
	}
	return parsed
}

func envFloat(getenv func(string) string, name string, defaultValue float64) float64 {
	value := strings.TrimSpace(getenv(name))
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
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
	if err != nil {
		return defaultValue
	}
	return parsed
}

func getenv(name string) string {
	return os.Getenv(name)
}
