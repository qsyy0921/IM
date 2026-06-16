package main

import (
	"flag"
	"os"
	"strings"
	"time"

	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
)

type config struct {
	target                 string
	resultDir              string
	pgDSN                  string
	scenario               string
	action                 string
	requestTimeout         time.Duration
	tenantID               string
	userID                 string
	deviceID               string
	sessionID              string
	changeUserID           string
	changeDeviceID         string
	changeSessionID        string
	conversationID         string
	clientMsgID            string
	expectedPermissionVer  int64
	expectedClassification string
	expectedBaseClass      string
	expectedReason         string
	messageTLS             grpctls.Config
	verifiedMetadata       bool
	cleanup                bool
	seedPolicyRule         bool
	seedTenantPolicyRule   bool
	seedConversationRole   bool
	seedOwnershipOverride  bool
	expectPolicyAudit      bool
	expectedAuditRows      int64
}

func parseConfig() config {
	var cfg config
	flag.StringVar(&cfg.target, "target", "127.0.0.1:10495", "message-service gRPC target")
	registerTLSFlags("message-tls", "NEXUSIM_MESSAGE_TLS", "message-service", &cfg.messageTLS)
	flag.StringVar(&cfg.resultDir, "result-dir", "H:\\NexusIM\\loadtest-results\\policy-message-smoke", "result directory")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", "", "PostgreSQL DSN")
	flag.StringVar(&cfg.scenario, "scenario", "allow", "scenario: allow or deny")
	flag.StringVar(&cfg.action, "action", "send", "message action: send, edit, revoke, or delete")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 5*time.Second, "per-request timeout")
	flag.StringVar(&cfg.tenantID, "tenant-id", "tenant-policy-message-smoke", "tenant id")
	flag.StringVar(&cfg.userID, "user-id", "policy-message-user", "user id")
	flag.StringVar(&cfg.deviceID, "device-id", "policy-message-device-1", "device id")
	flag.StringVar(&cfg.sessionID, "session-id", "policy-message-session-1", "session id")
	flag.StringVar(&cfg.changeUserID, "change-user-id", "", "user id for edit/revoke/delete; defaults to --user-id")
	flag.StringVar(&cfg.changeDeviceID, "change-device-id", "", "device id for edit/revoke/delete; defaults to --device-id")
	flag.StringVar(&cfg.changeSessionID, "change-session-id", "", "session id for edit/revoke/delete; defaults to --session-id")
	flag.StringVar(&cfg.conversationID, "conversation-id", "policy-message-conversation", "conversation id")
	flag.StringVar(&cfg.clientMsgID, "client-msg-id", "policy-message-client-msg-1", "client message id")
	flag.Int64Var(&cfg.expectedPermissionVer, "expected-permission-version", 1, "expected permission version")
	flag.StringVar(&cfg.expectedClassification, "expected-classification", "INTERNAL", "expected classification")
	flag.StringVar(&cfg.expectedBaseClass, "expected-base-classification", "", "expected base SendMessage classification for mutation scenarios; defaults to --expected-classification or seeded SEND rule")
	flag.StringVar(&cfg.expectedReason, "expected-reason", "", "expected deny reason")
	flag.BoolVar(&cfg.verifiedMetadata, "verified-auth-metadata", envBool(false, "NEXUSIM_POLICY_INTEGRATION_VERIFIED_AUTH_METADATA", "NEXUSIM_MESSAGE_LOADTEST_VERIFIED_AUTH_METADATA"), "send gateway verified identity through message-service gRPC metadata")
	flag.BoolVar(&cfg.cleanup, "cleanup", false, "delete message rows for tenant before running")
	flag.BoolVar(&cfg.seedPolicyRule, "seed-policy-rule", false, "seed exact policy_message_action_rules row for this scenario")
	flag.BoolVar(&cfg.seedTenantPolicyRule, "seed-tenant-policy-rule", false, "seed tenant-level policy_tenant_message_action_rules row for this scenario")
	flag.BoolVar(&cfg.seedConversationRole, "seed-conversation-role-gate", false, "seed policy conversation role gate rule and member projection for this scenario")
	flag.BoolVar(&cfg.seedOwnershipOverride, "seed-ownership-override-rule", false, "seed policy message ownership override rule and member projection for this scenario")
	flag.BoolVar(&cfg.expectPolicyAudit, "expect-policy-audit", false, "validate the latest policy_decision_audit_outbox row")
	flag.Int64Var(&cfg.expectedAuditRows, "expected-audit-rows", 0, "expected policy_decision_audit_outbox row count when audit validation is enabled")
	flag.Parse()
	cfg.scenario = strings.ToLower(strings.TrimSpace(cfg.scenario))
	cfg.action = strings.ToLower(strings.TrimSpace(cfg.action))
	cfg.changeUserID = strings.TrimSpace(cfg.changeUserID)
	cfg.changeDeviceID = strings.TrimSpace(cfg.changeDeviceID)
	cfg.changeSessionID = strings.TrimSpace(cfg.changeSessionID)
	if cfg.changeUserID == "" {
		cfg.changeUserID = cfg.userID
	}
	if cfg.changeDeviceID == "" {
		cfg.changeDeviceID = cfg.deviceID
	}
	if cfg.changeSessionID == "" {
		cfg.changeSessionID = cfg.sessionID
	}
	if cfg.requestTimeout <= 0 {
		cfg.requestTimeout = 5 * time.Second
	}
	return cfg
}

func registerTLSFlags(prefix, envPrefix, serviceName string, config *grpctls.Config) {
	flag.StringVar(&config.CAFile, prefix+"-ca-file", os.Getenv(envPrefix+"_CA_FILE"), serviceName+" gRPC TLS CA file")
	flag.StringVar(&config.ServerName, prefix+"-server-name", os.Getenv(envPrefix+"_SERVER_NAME"), serviceName+" gRPC TLS server name")
	flag.StringVar(&config.ClientCertFile, prefix+"-client-cert-file", os.Getenv(envPrefix+"_CLIENT_CERT_FILE"), serviceName+" gRPC TLS client certificate file")
	flag.StringVar(&config.ClientKeyFile, prefix+"-client-key-file", os.Getenv(envPrefix+"_CLIENT_KEY_FILE"), serviceName+" gRPC TLS client key file")
}
