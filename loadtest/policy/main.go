package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc"
)

type config struct {
	target                 string
	policyTLS              grpctls.Config
	resultDir              string
	requestTimeout         time.Duration
	tenantID               string
	userID                 string
	deviceID               string
	sessionID              string
	conversationID         string
	messageID              string
	expectedAllowed        bool
	expectedPermissionVer  int64
	expectedClassification string
	expectedReason         string
}

type summary struct {
	Commit                 string             `json:"commit"`
	CommitFull             string             `json:"commit_full"`
	GitDirty               bool               `json:"git_dirty"`
	GitStatusShort         string             `json:"git_status_short,omitempty"`
	Target                 string             `json:"target"`
	PolicyTLSEnabled       bool               `json:"policy_tls_enabled"`
	ResultDir              string             `json:"result_dir"`
	TenantID               string             `json:"tenant_id"`
	UserID                 string             `json:"user_id"`
	ConversationID         string             `json:"conversation_id"`
	StartedAt              time.Time          `json:"started_at"`
	FinishedAt             time.Time          `json:"finished_at"`
	Success                bool               `json:"success"`
	Error                  string             `json:"error,omitempty"`
	ExpectedAllowed        bool               `json:"expected_allowed"`
	ExpectedPermissionVer  int64              `json:"expected_permission_version"`
	ExpectedClassification string             `json:"expected_classification"`
	ExpectedReason         string             `json:"expected_reason,omitempty"`
	Actions                []actionSummary    `json:"actions"`
	LatenciesMS            map[string]float64 `json:"latencies_ms"`
	Capacity               *capacitySummary   `json:"capacity_summary,omitempty"`
}

type actionSummary struct {
	Action            string  `json:"action"`
	MessageID         string  `json:"message_id,omitempty"`
	Allowed           bool    `json:"allowed"`
	PermissionVersion int64   `json:"permission_version"`
	Classification    string  `json:"classification"`
	Reason            string  `json:"reason,omitempty"`
	LatencyMS         float64 `json:"latency_ms"`
}

type capacitySummary struct {
	DurationSeconds    float64 `json:"duration_seconds"`
	ActionCount        int     `json:"action_count"`
	AllowedActionCount int     `json:"allowed_action_count"`
	DeniedActionCount  int     `json:"denied_action_count"`
	DecisionsPerSecond float64 `json:"decisions_per_second"`
	LatencyP95MS       float64 `json:"latency_p95_ms,omitempty"`
	LatencyP99MS       float64 `json:"latency_p99_ms,omitempty"`
	ExpectedAllowed    bool    `json:"expected_allowed"`
	PermissionVersion  int64   `json:"permission_version"`
	Classification     string  `json:"classification"`
}

func main() {
	cfg := parseConfig()
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseConfig() config {
	var cfg config
	flag.StringVar(&cfg.target, "target", "127.0.0.1:10800", "policy-service gRPC target")
	flag.StringVar(&cfg.policyTLS.CAFile, "policy-tls-ca-file", "", "CA PEM for policy-service gRPC TLS")
	flag.StringVar(&cfg.policyTLS.ServerName, "policy-tls-server-name", "", "server name for policy-service gRPC TLS")
	flag.StringVar(&cfg.policyTLS.ClientCertFile, "policy-tls-client-cert-file", "", "client certificate PEM for policy-service mTLS")
	flag.StringVar(&cfg.policyTLS.ClientKeyFile, "policy-tls-client-key-file", "", "client private key PEM for policy-service mTLS")
	flag.StringVar(&cfg.resultDir, "result-dir", "H:\\NexusIM\\loadtest-results\\policy-smoke", "result directory")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 5*time.Second, "per-request timeout")
	flag.StringVar(&cfg.tenantID, "tenant-id", "tenant-policy-smoke", "tenant id")
	flag.StringVar(&cfg.userID, "user-id", "policy-user", "user id")
	flag.StringVar(&cfg.deviceID, "device-id", "policy-device-1", "device id")
	flag.StringVar(&cfg.sessionID, "session-id", "policy-session-1", "session id")
	flag.StringVar(&cfg.conversationID, "conversation-id", "policy-conversation", "conversation id")
	flag.StringVar(&cfg.messageID, "message-id", "policy-message", "message id for edit/revoke/delete")
	flag.BoolVar(&cfg.expectedAllowed, "expected-allowed", true, "expected allowed value")
	flag.Int64Var(&cfg.expectedPermissionVer, "expected-permission-version", 1, "expected permission version")
	flag.StringVar(&cfg.expectedClassification, "expected-classification", "INTERNAL", "expected classification")
	flag.StringVar(&cfg.expectedReason, "expected-reason", "", "expected deny reason")
	flag.Parse()
	if cfg.requestTimeout <= 0 {
		cfg.requestTimeout = 5 * time.Second
	}
	return cfg
}

func run(cfg config) error {
	if err := os.MkdirAll(cfg.resultDir, 0o755); err != nil {
		return fmt.Errorf("create result dir: %w", err)
	}
	started := time.Now().UTC()
	s := summary{
		Commit:                 gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:             gitOutput("rev-parse", "HEAD"),
		GitStatusShort:         gitOutput("status", "--short"),
		Target:                 cfg.target,
		PolicyTLSEnabled:       cfg.policyTLS.Enabled(),
		ResultDir:              cfg.resultDir,
		TenantID:               cfg.tenantID,
		UserID:                 cfg.userID,
		ConversationID:         cfg.conversationID,
		StartedAt:              started,
		ExpectedAllowed:        cfg.expectedAllowed,
		ExpectedPermissionVer:  cfg.expectedPermissionVer,
		ExpectedClassification: cfg.expectedClassification,
		ExpectedReason:         cfg.expectedReason,
		LatenciesMS:            map[string]float64{},
	}
	s.GitDirty = strings.TrimSpace(s.GitStatusShort) != ""
	defer func() {
		s.FinishedAt = time.Now().UTC()
		s.Capacity = buildCapacitySummary(s)
		_ = writeSummary(cfg.resultDir, s)
	}()

	dialOption, err := grpctls.DialOption(cfg.policyTLS, "policy-tls")
	if err != nil {
		s.Error = fmt.Sprintf("configure policy-service TLS: %v", err)
		return fmt.Errorf("configure policy-service TLS: %w", err)
	}
	conn, err := grpc.NewClient(cfg.target, dialOption)
	if err != nil {
		s.Error = fmt.Sprintf("dial policy-service: %v", err)
		return fmt.Errorf("dial policy-service: %w", err)
	}
	defer conn.Close()
	client := policyv1.NewPolicyServiceClient(conn)

	actions := []struct {
		name      string
		proto     policyv1.MessageAction
		messageID string
	}{
		{name: "SEND", proto: policyv1.MessageAction_MESSAGE_ACTION_SEND},
		{name: "EDIT", proto: policyv1.MessageAction_MESSAGE_ACTION_EDIT, messageID: cfg.messageID},
		{name: "REVOKE", proto: policyv1.MessageAction_MESSAGE_ACTION_REVOKE, messageID: cfg.messageID},
		{name: "DELETE", proto: policyv1.MessageAction_MESSAGE_ACTION_DELETE, messageID: cfg.messageID},
	}
	for _, action := range actions {
		result, err := callPolicy(cfg, client, action.name, action.proto, action.messageID)
		if err != nil {
			s.Error = err.Error()
			return err
		}
		if err := validateAction(cfg, action.name, action.proto, action.messageID, result.response); err != nil {
			s.Error = err.Error()
			return err
		}
		actionResult := actionSummary{
			Action:            action.name,
			MessageID:         action.messageID,
			Allowed:           result.response.GetAllowed(),
			PermissionVersion: result.response.GetPermissionVersion(),
			Classification:    result.response.GetClassification(),
			Reason:            result.response.GetReason(),
			LatencyMS:         result.latencyMS,
		}
		s.Actions = append(s.Actions, actionResult)
		s.LatenciesMS[strings.ToLower(action.name)] = result.latencyMS
	}
	s.Success = true
	return nil
}

type callResult struct {
	response  *policyv1.CheckMessageActionResponse
	latencyMS float64
}

func callPolicy(
	cfg config,
	client policyv1.PolicyServiceClient,
	actionName string,
	action policyv1.MessageAction,
	messageID string,
) (callResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.requestTimeout)
	defer cancel()
	started := time.Now()
	response, err := client.CheckMessageAction(ctx, &policyv1.CheckMessageActionRequest{
		AuthContext: &policyv1.AuthContext{
			TenantId:  cfg.tenantID,
			UserId:    cfg.userID,
			DeviceId:  cfg.deviceID,
			SessionId: cfg.sessionID,
			TraceId:   "trace-policy-smoke",
			RequestId: "policy-smoke-" + strings.ToLower(actionName),
		},
		ConversationId: cfg.conversationID,
		Action:         action,
		MessageId:      messageID,
	})
	if err != nil {
		return callResult{}, fmt.Errorf("check %s policy: %w", actionName, err)
	}
	return callResult{response: response, latencyMS: float64(time.Since(started).Microseconds()) / 1000.0}, nil
}

func validateAction(
	cfg config,
	actionName string,
	action policyv1.MessageAction,
	messageID string,
	response *policyv1.CheckMessageActionResponse,
) error {
	if response == nil {
		return fmt.Errorf("%s returned empty response", actionName)
	}
	if response.GetTenantId() != cfg.tenantID ||
		response.GetUserId() != cfg.userID ||
		response.GetConversationId() != cfg.conversationID ||
		response.GetMessageId() != messageID ||
		response.GetAction() != action {
		return fmt.Errorf("%s returned mismatched response", actionName)
	}
	if response.GetAllowed() != cfg.expectedAllowed {
		return fmt.Errorf("%s allowed=%v, expected %v", actionName, response.GetAllowed(), cfg.expectedAllowed)
	}
	if response.GetPermissionVersion() != cfg.expectedPermissionVer {
		return fmt.Errorf("%s permission_version=%d, expected %d", actionName, response.GetPermissionVersion(), cfg.expectedPermissionVer)
	}
	if response.GetClassification() != cfg.expectedClassification {
		return fmt.Errorf("%s classification=%q, expected %q", actionName, response.GetClassification(), cfg.expectedClassification)
	}
	if response.GetReason() != cfg.expectedReason {
		return fmt.Errorf("%s reason=%q, expected %q", actionName, response.GetReason(), cfg.expectedReason)
	}
	return nil
}

func writeSummary(resultDir string, s summary) error {
	path := filepath.Join(resultDir, "policy-summary.json")
	bytes, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(bytes, '\n'), 0o644)
}

func buildCapacitySummary(s summary) *capacitySummary {
	duration := s.FinishedAt.Sub(s.StartedAt).Seconds()
	if duration <= 0 {
		return nil
	}
	allowed := 0
	for _, action := range s.Actions {
		if action.Allowed {
			allowed++
		}
	}
	actionCount := len(s.Actions)
	return &capacitySummary{
		DurationSeconds:    duration,
		ActionCount:        actionCount,
		AllowedActionCount: allowed,
		DeniedActionCount:  actionCount - allowed,
		DecisionsPerSecond: ratePerSecond(actionCount, duration),
		LatencyP95MS:       latencyQuantile(s.LatenciesMS, 0.95),
		LatencyP99MS:       latencyQuantile(s.LatenciesMS, 0.99),
		ExpectedAllowed:    s.ExpectedAllowed,
		PermissionVersion:  s.ExpectedPermissionVer,
		Classification:     s.ExpectedClassification,
	}
}

func ratePerSecond(count int, durationSeconds float64) float64 {
	if count <= 0 || durationSeconds <= 0 {
		return 0
	}
	return float64(count) / durationSeconds
}

func latencyQuantile(values map[string]float64, quantile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, 0, len(values))
	for _, value := range values {
		sorted = append(sorted, value)
	}
	sort.Float64s(sorted)
	index := int(math.Ceil(quantile*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func gitOutput(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
