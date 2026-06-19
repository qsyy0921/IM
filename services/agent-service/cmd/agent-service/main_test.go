package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/agent-service/internal/infrastructure/postgres"
)

func TestValidateAgentServiceMode(t *testing.T) {
	for _, mode := range []string{"noop", "grpc", "approval-outbox-relay", "proposal-approval-audit", "proposal-approval-approve"} {
		if err := validateAgentServiceMode(mode); err != nil {
			t.Fatalf("mode %s should be valid: %v", mode, err)
		}
	}
	if err := validateAgentServiceMode("bad"); err == nil {
		t.Fatal("expected invalid mode error")
	}
}

func TestValidateAgentDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	for _, addr := range []string{"", "localhost:11922", "127.0.0.1:11922", "172.30.80.25:11922"} {
		if err := validateAgentDebugListenerConfig(addr, false); err != nil {
			t.Fatalf("addr %s should be accepted: %v", addr, err)
		}
	}
}

func TestValidateAgentDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	if err := validateAgentDebugListenerConfig("8.8.8.8:11922", false); err == nil {
		t.Fatal("expected public listener rejection")
	}
}

func TestValidateAgentDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateAgentDebugListenerConfig("8.8.8.8:11922", true); err != nil {
		t.Fatalf("expected public listener override: %v", err)
	}
}

func TestAgentProposalProviderFromEnvAllowsPythonWorkerMode(t *testing.T) {
	t.Setenv("NEXUSIM_AGENT_PROPOSAL_PROVIDER_MODE", "python-worker")
	t.Setenv("NEXUSIM_AGENT_PYTHON_BIN", "python")
	t.Setenv("NEXUSIM_AGENT_PYTHON_WORKER_SCRIPT", "ai/python/scripts/run_candidate_worker.py")

	provider, err := agentProposalProviderFromEnv()
	if err != nil {
		t.Fatalf("python worker provider should be valid: %v", err)
	}
	if provider == nil {
		t.Fatal("expected provider")
	}
}

func TestAgentProposalProviderFromEnvRejectsUnknownMode(t *testing.T) {
	t.Setenv("NEXUSIM_AGENT_PROPOSAL_PROVIDER_MODE", "unsafe-direct-provider")

	if _, err := agentProposalProviderFromEnv(); err == nil {
		t.Fatal("expected unsupported provider mode error")
	}
}

func TestNewDebugHandlerExposesMetrics(t *testing.T) {
	server := http.Server{Handler: newDebugHandler()}
	request, err := http.NewRequest(http.MethodGet, "/metrics", nil)
	if err != nil {
		t.Fatal(err)
	}
	recorder := &responseRecorder{header: http.Header{}}
	server.Handler.ServeHTTP(recorder, request)
	if recorder.status != 0 && recorder.status != http.StatusOK {
		t.Fatalf("unexpected status %d", recorder.status)
	}
	body := string(recorder.body)
	if body != "nexusim_agent_service_info 1\n" {
		t.Fatalf("unexpected body %q", body)
	}
}

func TestProposalApprovalDryRunDefaultsToTrue(t *testing.T) {
	t.Setenv("NEXUSIM_AGENT_PROPOSAL_APPROVAL_DRY_RUN", "")
	value, err := proposalApprovalDryRunFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !value {
		t.Fatal("expected approval operator dry-run default")
	}
	t.Setenv("NEXUSIM_AGENT_PROPOSAL_APPROVAL_DRY_RUN", "false")
	value, err = proposalApprovalDryRunFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if value {
		t.Fatal("expected explicit false to disable dry-run")
	}
}

func TestAgentOperatorReasonFromFile(t *testing.T) {
	reasonPath := filepath.Join(t.TempDir(), "reason.txt")
	if err := os.WriteFile(reasonPath, []byte(" approve grounded proposal "), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NEXUSIM_AGENT_TEST_REASON_FILE", reasonPath)
	reason, err := agentOperatorReasonFromEnv("NEXUSIM_AGENT_TEST_REASON", "NEXUSIM_AGENT_TEST_REASON_FILE", "fallback")
	if err != nil {
		t.Fatal(err)
	}
	if reason != "approve grounded proposal" {
		t.Fatalf("unexpected reason %q", reason)
	}
}

func TestWriteProposalApprovalOutputIsLowSensitive(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "approval.json")
	now := time.Date(2026, 6, 19, 1, 2, 3, 0, time.UTC)
	candidate := postgresinfra.AgentProposalApprovalAuditRow{
		TenantID:              "tenant-1",
		ProposalID:            "proposal-1",
		UserID:                "user-1",
		ConversationID:        "conv-1",
		SkillID:               "nexusim.local.echo",
		PreparedAuditID:       "mcp-audit-1",
		ToolName:              "nexusim.local.echo",
		ResourceType:          "conversation",
		ResourceID:            "conv-1",
		RiskLevel:             "LOW",
		Status:                "PROPOSED",
		RequiresApproval:      true,
		Allowed:               true,
		PermissionVersion:     1,
		Classification:        "TOOL_ALLOWED",
		DecisionSource:        "policy",
		EvidencePackID:        "pack-1",
		GeneratedByLLM:        false,
		ApprovalReasonPresent: true,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	err := writeProposalApprovalApproveOutput(outputPath, true, proposalApprovalRequest{
		TenantID:              "tenant-1",
		ProposalID:            "proposal-1",
		ApprovedByUserID:      "approver-1",
		ApprovalReasonPresent: true,
	}, &candidate, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, forbidden := range []string{"objective", "proposal_text", "citations_json", "approve grounded proposal", "reason\": \"", "EvidencePack"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("approval output leaked forbidden content %q: %s", forbidden, text)
		}
	}
	for _, required := range []string{"proposal-1", "approver-1", "approval_reason_present"} {
		if !strings.Contains(text, required) {
			t.Fatalf("approval output missing %q: %s", required, text)
		}
	}
}

type responseRecorder struct {
	header http.Header
	status int
	body   string
}

func (recorder *responseRecorder) Header() http.Header {
	return recorder.header
}

func (recorder *responseRecorder) WriteHeader(statusCode int) {
	recorder.status = statusCode
}

func (recorder *responseRecorder) Write(bytes []byte) (int, error) {
	recorder.body += string(bytes)
	return len(bytes), nil
}

var _ http.ResponseWriter = (*responseRecorder)(nil)
var _ io.Writer = (*responseRecorder)(nil)
