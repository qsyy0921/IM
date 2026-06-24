package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	memoryv1 "github.com/qsyy0921/IM/api/proto/nexusim/memory/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc"
)

const defaultResultRoot = `H:\NexusIM\loadtest-results`

type config struct {
	memoryTarget    string
	memoryTLS       grpctls.Config
	tenantID        string
	userID          string
	deviceID        string
	subjectUserID   string
	aggregateType   string
	aggregateKey    string
	minSupportCount int
	requestTimeout  time.Duration
	resultRoot      string
	runName         string
	execute         bool
}

type summary struct {
	SchemaVersion              int       `json:"schema_version"`
	Mode                       string    `json:"mode"`
	Executed                   bool      `json:"executed"`
	Commit                     string    `json:"commit"`
	CommitFull                 string    `json:"commit_full"`
	GitDirty                   bool      `json:"git_dirty"`
	GitStatusShort             string    `json:"git_status_short,omitempty"`
	ResultDir                  string    `json:"result_dir"`
	MemoryTarget               string    `json:"memory_target"`
	MemoryTLSEnabled           bool      `json:"memory_tls_enabled"`
	TenantID                   string    `json:"tenant_id"`
	UserID                     string    `json:"user_id"`
	DeviceID                   string    `json:"device_id"`
	SubjectUserID              string    `json:"subject_user_id"`
	AggregateType              string    `json:"aggregate_type"`
	AggregateKeyHash           string    `json:"aggregate_key_sha256"`
	MinSupportCount            int       `json:"min_support_count"`
	StartedAt                  time.Time `json:"started_at"`
	FinishedAt                 time.Time `json:"finished_at"`
	Success                    bool      `json:"success"`
	Error                      string    `json:"error,omitempty"`
	Active                     bool      `json:"active,omitempty"`
	SupportCount               int32     `json:"support_count,omitempty"`
	ProfileID                  string    `json:"profile_id,omitempty"`
	ProfileStatus              string    `json:"profile_status,omitempty"`
	ProfileReviewState         string    `json:"profile_review_state,omitempty"`
	SupportingMemoryCount      int       `json:"supporting_memory_count,omitempty"`
	SupportingMemoryIDHashes   []string  `json:"supporting_memory_id_hashes,omitempty"`
	SummaryTextSHA256          string    `json:"summary_text_sha256,omitempty"`
	SummaryTextLength          int       `json:"summary_text_length,omitempty"`
	ProfileUpdatedAtUnixMillis int64     `json:"profile_updated_at_unix_ms,omitempty"`
}

func main() {
	cfg, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseArgs(args []string) (config, error) {
	var cfg config
	flags := flag.NewFlagSet("memory-profile-operator", flag.ContinueOnError)
	flags.StringVar(&cfg.memoryTarget, "memory-target", envOrDefault("NEXUSIM_MEMORY_GRPC_ADDR", "127.0.0.1:10580"), "memory-service gRPC target")
	registerTLSFlags(flags, "memory-tls", "NEXUSIM_MEMORY_TLS", "memory-service", &cfg.memoryTLS)
	flags.StringVar(&cfg.tenantID, "tenant-id", envOrDefault("NEXUSIM_TENANT_ID", "nexusim-local"), "tenant id")
	flags.StringVar(&cfg.userID, "user-id", "memory-profile-operator", "auth user id")
	flags.StringVar(&cfg.deviceID, "device-id", "memory-profile-operator-device", "auth device id")
	flags.StringVar(&cfg.subjectUserID, "subject-user-id", "", "profile subject user id; defaults to --user-id")
	flags.StringVar(&cfg.aggregateType, "aggregate-type", "SKILL", "profile aggregate type")
	flags.StringVar(&cfg.aggregateKey, "aggregate-key", "", "profile aggregate key")
	flags.IntVar(&cfg.minSupportCount, "min-support-count", 2, "minimum visible supporting PROFILE_SIGNAL memories")
	flags.DurationVar(&cfg.requestTimeout, "request-timeout", 5*time.Second, "request timeout")
	flags.StringVar(&cfg.resultRoot, "result-root", defaultResultRoot, "external result root for low-sensitive output")
	flags.StringVar(&cfg.runName, "run-name", "", "run name under result-root")
	flags.BoolVar(&cfg.execute, "execute", false, "execute recompute through memory-service; default is plan-only")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if strings.TrimSpace(cfg.subjectUserID) == "" {
		cfg.subjectUserID = cfg.userID
	}
	if strings.TrimSpace(cfg.runName) == "" {
		cfg.runName = "memory-profile-operator-" + time.Now().UTC().Format("20060102-150405")
	}
	return cfg, validateConfig(cfg)
}

func registerTLSFlags(flags *flag.FlagSet, prefix string, envPrefix string, serviceName string, config *grpctls.Config) {
	flags.StringVar(&config.CAFile, prefix+"-ca-file", os.Getenv(envPrefix+"_CA_FILE"), "CA PEM for "+serviceName+" gRPC TLS")
	flags.StringVar(&config.ServerName, prefix+"-server-name", os.Getenv(envPrefix+"_SERVER_NAME"), "override server name for "+serviceName+" gRPC TLS")
	flags.StringVar(&config.ClientCertFile, prefix+"-client-cert-file", os.Getenv(envPrefix+"_CLIENT_CERT_FILE"), "client certificate PEM for "+serviceName+" mTLS")
	flags.StringVar(&config.ClientKeyFile, prefix+"-client-key-file", os.Getenv(envPrefix+"_CLIENT_KEY_FILE"), "client private key PEM for "+serviceName+" mTLS")
}

func validateConfig(cfg config) error {
	if strings.TrimSpace(cfg.memoryTarget) == "" {
		return errors.New("--memory-target is required")
	}
	if strings.TrimSpace(cfg.tenantID) == "" {
		return errors.New("--tenant-id is required")
	}
	if strings.TrimSpace(cfg.userID) == "" {
		return errors.New("--user-id is required")
	}
	if strings.TrimSpace(cfg.deviceID) == "" {
		return errors.New("--device-id is required")
	}
	if strings.TrimSpace(cfg.subjectUserID) == "" {
		return errors.New("--subject-user-id is required")
	}
	if cfg.subjectUserID != cfg.userID {
		return errors.New("--subject-user-id must match --user-id until a policy-controlled operator path exists")
	}
	if strings.TrimSpace(cfg.aggregateType) == "" {
		return errors.New("--aggregate-type is required")
	}
	if strings.TrimSpace(cfg.aggregateKey) == "" {
		return errors.New("--aggregate-key is required")
	}
	if cfg.minSupportCount <= 0 || cfg.minSupportCount > 20 {
		return errors.New("--min-support-count must be between 1 and 20")
	}
	if cfg.requestTimeout <= 0 {
		return errors.New("--request-timeout must be positive")
	}
	if strings.TrimSpace(cfg.resultRoot) == "" {
		return errors.New("--result-root is required")
	}
	return nil
}

func run(cfg config) error {
	resultDir := filepath.Join(cfg.resultRoot, cfg.runName)
	if err := validateExternalResultDir(resultDir); err != nil {
		return err
	}
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		return fmt.Errorf("create result dir: %w", err)
	}

	result := newSummary(cfg, resultDir)
	defer func() {
		result.FinishedAt = time.Now().UTC()
		_ = writeSummary(resultDir, result)
	}()

	if !cfg.execute {
		result.Success = true
		return nil
	}

	dialOption, err := grpctls.DialOption(cfg.memoryTLS, "memory-tls")
	if err != nil {
		result.Error = "configure memory TLS: " + err.Error()
		return err
	}
	conn, err := grpc.NewClient("passthrough:///"+cfg.memoryTarget, dialOption)
	if err != nil {
		result.Error = "dial memory-service: " + err.Error()
		return fmt.Errorf("dial memory-service: %w", err)
	}
	defer conn.Close()

	requestCtx, cancel := context.WithTimeout(context.Background(), cfg.requestTimeout)
	defer cancel()
	response, err := memoryv1.NewMemoryServiceClient(conn).RecomputeProfileAggregate(requestCtx, recomputeRequest(cfg))
	if err != nil {
		result.Error = "recompute profile aggregate: " + err.Error()
		return fmt.Errorf("recompute profile aggregate: %w", err)
	}
	applyResponse(&result, response)
	result.Success = true
	return nil
}

func execute(ctx context.Context, cfg config, client memoryv1.MemoryServiceClient) (summary, error) {
	result := newSummary(cfg, "")
	if !cfg.execute {
		result.Success = true
		return result, nil
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	response, err := client.RecomputeProfileAggregate(requestCtx, recomputeRequest(cfg))
	if err != nil {
		result.Error = "recompute profile aggregate: " + err.Error()
		return result, err
	}
	applyResponse(&result, response)
	result.Success = true
	return result, nil
}

func recomputeRequest(cfg config) *memoryv1.RecomputeProfileAggregateRequest {
	return &memoryv1.RecomputeProfileAggregateRequest{
		AuthContext: &memoryv1.AuthContext{
			TenantId:  cfg.tenantID,
			UserId:    cfg.userID,
			DeviceId:  cfg.deviceID,
			SessionId: "memory-profile-operator",
			TraceId:   "memory-profile-operator",
			RequestId: "memory-profile-operator",
		},
		SubjectUserId:   cfg.subjectUserID,
		AggregateType:   cfg.aggregateType,
		AggregateKey:    cfg.aggregateKey,
		MinSupportCount: int32(cfg.minSupportCount),
	}
}

func newSummary(cfg config, resultDir string) summary {
	gitStatus := gitOutput("status", "--short")
	return summary{
		SchemaVersion:    1,
		Mode:             "recompute-profile",
		Executed:         cfg.execute,
		Commit:           gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:       gitOutput("rev-parse", "HEAD"),
		GitDirty:         strings.TrimSpace(gitStatus) != "",
		GitStatusShort:   gitStatus,
		ResultDir:        resultDir,
		MemoryTarget:     cfg.memoryTarget,
		MemoryTLSEnabled: cfg.memoryTLS.Enabled(),
		TenantID:         cfg.tenantID,
		UserID:           cfg.userID,
		DeviceID:         cfg.deviceID,
		SubjectUserID:    cfg.subjectUserID,
		AggregateType:    cfg.aggregateType,
		AggregateKeyHash: sha256Hex(cfg.aggregateKey),
		MinSupportCount:  cfg.minSupportCount,
		StartedAt:        time.Now().UTC(),
	}
}

func applyResponse(result *summary, response *memoryv1.RecomputeProfileAggregateResponse) {
	result.Active = response.GetActive()
	result.SupportCount = response.GetSupportCount()
	item := response.GetItem()
	if item == nil {
		return
	}
	result.ProfileID = item.GetProfileId()
	result.ProfileStatus = item.GetStatus().String()
	result.ProfileReviewState = item.GetReviewState().String()
	result.SupportingMemoryCount = len(item.GetSupportingMemoryEventIds())
	for _, id := range item.GetSupportingMemoryEventIds() {
		result.SupportingMemoryIDHashes = append(result.SupportingMemoryIDHashes, sha256Hex(id))
	}
	result.SummaryTextSHA256 = sha256Hex(item.GetSummaryText())
	result.SummaryTextLength = len(item.GetSummaryText())
	result.ProfileUpdatedAtUnixMillis = item.GetUpdatedAtUnixMs()
}

func validateExternalResultDir(path string) error {
	clean := filepath.Clean(path)
	repoRoot := gitOutput("rev-parse", "--show-toplevel")
	if repoRoot != "" && strings.HasPrefix(strings.ToLower(clean), strings.ToLower(filepath.Clean(repoRoot))) {
		return fmt.Errorf("result dir must be outside repository: %s", clean)
	}
	return nil
}

func writeSummary(resultDir string, result summary) error {
	path := filepath.Join(resultDir, "memory-profile-operator-summary.json")
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, encoded, 0o644)
}

func gitOutput(args ...string) string {
	cmd := exec.Command("git", args...)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func sha256Hex(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func envOrDefault(name string, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
	}
	return value
}
