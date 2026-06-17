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
	"sync"
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
	duration               time.Duration
	vus                    int
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
	RequestedDurationSec   float64            `json:"requested_duration_seconds,omitempty"`
	VUs                    int                `json:"vus,omitempty"`
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
	ActionCount            int                `json:"action_count"`
	AllowedActionCount     int                `json:"allowed_action_count"`
	DeniedActionCount      int                `json:"denied_action_count"`
	Capacity               *capacitySummary   `json:"capacity_summary,omitempty"`

	latencySamplesMS []float64
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
	flag.DurationVar(&cfg.duration, "duration", 0, "capacity run duration; zero runs one action set")
	flag.IntVar(&cfg.vus, "vus", 1, "virtual users for duration mode")
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
	if cfg.duration < 0 {
		cfg.duration = 0
	}
	if cfg.vus <= 0 {
		cfg.vus = 1
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
		RequestedDurationSec:   cfg.duration.Seconds(),
		VUs:                    cfg.vus,
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

	if cfg.duration > 0 {
		if err := runCapacityMode(cfg, client, &s); err != nil {
			s.Error = err.Error()
			return err
		}
	} else {
		if err := runOnce(cfg, client, &s); err != nil {
			s.Error = err.Error()
			return err
		}
	}
	s.Success = true
	return nil
}

type policyAction struct {
	name      string
	proto     policyv1.MessageAction
	messageID string
}

func policyActions(cfg config) []policyAction {
	return []policyAction{
		{name: "SEND", proto: policyv1.MessageAction_MESSAGE_ACTION_SEND},
		{name: "EDIT", proto: policyv1.MessageAction_MESSAGE_ACTION_EDIT, messageID: cfg.messageID},
		{name: "REVOKE", proto: policyv1.MessageAction_MESSAGE_ACTION_REVOKE, messageID: cfg.messageID},
		{name: "DELETE", proto: policyv1.MessageAction_MESSAGE_ACTION_DELETE, messageID: cfg.messageID},
	}
}

func runOnce(cfg config, client policyv1.PolicyServiceClient, s *summary) error {
	for _, action := range policyActions(cfg) {
		actionResult, err := executeAction(context.Background(), cfg, client, action, strings.ToLower(action.name))
		if err != nil {
			return err
		}
		recordAction(s, actionResult)
	}
	return nil
}

func runCapacityMode(cfg config, client policyv1.PolicyServiceClient, s *summary) error {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.duration)
	defer cancel()

	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup
	for workerID := 0; workerID < cfg.vus; workerID++ {
		workerID := workerID
		wg.Add(1)
		go func() {
			defer wg.Done()
			iteration := 0
			for {
				if ctx.Err() != nil {
					return
				}
				for _, action := range policyActions(cfg) {
					if ctx.Err() != nil {
						return
					}
					requestSuffix := fmt.Sprintf("vu-%d-%d-%s", workerID, iteration, strings.ToLower(action.name))
					actionResult, err := executeAction(ctx, cfg, client, action, requestSuffix)
					if err != nil {
						if ctx.Err() != nil {
							return
						}
						mu.Lock()
						if firstErr == nil {
							firstErr = err
							cancel()
						}
						mu.Unlock()
						return
					}
					mu.Lock()
					recordAction(s, actionResult)
					mu.Unlock()
				}
				iteration++
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	if s.ActionCount == 0 {
		return fmt.Errorf("policy capacity run produced no actions")
	}
	return nil
}

func executeAction(
	ctx context.Context,
	cfg config,
	client policyv1.PolicyServiceClient,
	action policyAction,
	requestSuffix string,
) (actionSummary, error) {
	result, err := callPolicy(ctx, cfg, client, action.name, action.proto, action.messageID, requestSuffix)
	if err != nil {
		return actionSummary{}, err
	}
	if err := validateAction(cfg, action.name, action.proto, action.messageID, result.response); err != nil {
		return actionSummary{}, err
	}
	return actionSummary{
		Action:            action.name,
		MessageID:         action.messageID,
		Allowed:           result.response.GetAllowed(),
		PermissionVersion: result.response.GetPermissionVersion(),
		Classification:    result.response.GetClassification(),
		Reason:            result.response.GetReason(),
		LatencyMS:         result.latencyMS,
	}, nil
}

func recordAction(s *summary, action actionSummary) {
	const maxActionSamples = 100
	if len(s.Actions) < maxActionSamples {
		s.Actions = append(s.Actions, action)
	}
	s.LatenciesMS[strings.ToLower(action.Action)] = action.LatencyMS
	s.latencySamplesMS = append(s.latencySamplesMS, action.LatencyMS)
	s.ActionCount++
	if action.Allowed {
		s.AllowedActionCount++
	} else {
		s.DeniedActionCount++
	}
}

type callResult struct {
	response  *policyv1.CheckMessageActionResponse
	latencyMS float64
}

func callPolicy(
	parent context.Context,
	cfg config,
	client policyv1.PolicyServiceClient,
	actionName string,
	action policyv1.MessageAction,
	messageID string,
	requestSuffix string,
) (callResult, error) {
	ctx, cancel := context.WithTimeout(parent, cfg.requestTimeout)
	defer cancel()
	if strings.TrimSpace(requestSuffix) == "" {
		requestSuffix = strings.ToLower(actionName)
	}
	started := time.Now()
	response, err := client.CheckMessageAction(ctx, &policyv1.CheckMessageActionRequest{
		AuthContext: &policyv1.AuthContext{
			TenantId:  cfg.tenantID,
			UserId:    cfg.userID,
			DeviceId:  cfg.deviceID,
			SessionId: cfg.sessionID,
			TraceId:   "trace-policy-smoke",
			RequestId: "policy-smoke-" + requestSuffix,
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
	actionCount := s.ActionCount
	allowed := s.AllowedActionCount
	denied := s.DeniedActionCount
	if actionCount == 0 {
		for _, action := range s.Actions {
			if action.Allowed {
				allowed++
			} else {
				denied++
			}
		}
		actionCount = len(s.Actions)
	}
	latencySamples := s.latencySamplesMS
	if len(latencySamples) == 0 {
		latencySamples = make([]float64, 0, len(s.LatenciesMS))
		for _, value := range s.LatenciesMS {
			latencySamples = append(latencySamples, value)
		}
	}
	return &capacitySummary{
		DurationSeconds:    duration,
		ActionCount:        actionCount,
		AllowedActionCount: allowed,
		DeniedActionCount:  denied,
		DecisionsPerSecond: ratePerSecond(actionCount, duration),
		LatencyP95MS:       latencyQuantile(latencySamples, 0.95),
		LatencyP99MS:       latencyQuantile(latencySamples, 0.99),
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

func latencyQuantile(values []float64, quantile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, 0, len(values))
	sorted = append(sorted, values...)
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
