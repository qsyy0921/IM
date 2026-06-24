package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/action-executor/internal/types"
)

func TestProviderFailureRedrivePlanOutputIsLowSensitiveAndNonMutating(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "redrive-plan.json")
	reasonPath := filepath.Join(tempDir, "reason.txt")
	if err := os.WriteFile(reasonPath, []byte("operator verified provider recovery and wants a guarded redrive plan"), 0o600); err != nil {
		t.Fatalf("write reason: %v", err)
	}
	reasonHash, err := actionProviderFailureReasonSHA256FromEnvWithPath(reasonPath)
	if err != nil {
		t.Fatalf("hash reason: %v", err)
	}
	now := time.Date(2026, 6, 25, 10, 30, 0, 0, time.UTC)
	row := types.ProviderFailureAuditRow{
		TenantID:          "tenant-1",
		ProviderFailureID: "provider-failure-1",
		ExecutionID:       "execution-1",
		ResultID:          "result-1",
		ProposalID:        "proposal-1",
		ApprovalID:        "approval-1",
		PreparedAuditID:   "mcp-audit-1",
		UserID:            "user-sensitive-1",
		SkillID:           "skill-1",
		ToolName:          "conversation.note.create",
		ResourceType:      "conversation",
		ResourceID:        "conversation-sensitive-1",
		Classification:    "TOOL_PROVIDER_UNAVAILABLE",
		Status:            types.ProviderFailureStatusDLQ,
		Retryable:         false,
		RetryCount:        3,
		DeadLetteredAt:    &now,
		FailureRef:        "action-executor://executions/execution-1/provider-failures/provider-failure-1",
		CreatedAt:         now,
	}
	output := newProviderFailureOperatorOutput(
		"action-executor.provider-failure.redrive-plan",
		types.ProviderFailureAuditOptions{TenantID: "tenant-1", Status: types.ProviderFailureStatusDLQ, Limit: 50},
		[]types.ProviderFailureAuditRow{row},
		true,
		reasonHash,
	)
	output.OperatorNextStep = "review"
	if err := writeProviderFailureOperatorOutput(outputPath, output); err != nil {
		t.Fatalf("write output: %v", err)
	}
	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	rawString := string(raw)
	for _, forbidden := range []string{"operator verified provider recovery", "user-sensitive-1", "conversation-sensitive-1", row.FailureRef} {
		if strings.Contains(rawString, forbidden) {
			t.Fatalf("redrive plan leaked %q: %s", forbidden, rawString)
		}
	}
	var parsed providerFailureOperatorOutput
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if parsed.ExecutesTool || parsed.MutatesProviderFailure || !parsed.RequiresOperatorApproval || !parsed.DryRun {
		t.Fatalf("unexpected redrive safety flags: %+v", parsed)
	}
	if parsed.BatchID == "" || parsed.CandidateCount != 1 || len(parsed.RedriveRequirements) == 0 {
		t.Fatalf("unexpected redrive batch handoff fields: %+v", parsed)
	}
	if parsed.Counts.Total != 1 || parsed.Counts.DLQ != 1 || parsed.Rows[0].UserIDHash == "" || parsed.Rows[0].ResourceIDHash == "" || parsed.Rows[0].FailureRefHash == "" {
		t.Fatalf("unexpected redrive plan content: %+v", parsed)
	}
	if parsed.ReasonSHA256 != reasonHash || !parsed.ReasonPresent {
		t.Fatalf("unexpected reason hash: %+v", parsed)
	}
}

func TestProviderFailureReplayOperatorUIOutputRequiresFreshApprovalAndAudit(t *testing.T) {
	now := time.Date(2026, 6, 25, 11, 30, 0, 0, time.UTC)
	row := types.ProviderFailureAuditRow{
		TenantID:          "tenant-1",
		ProviderFailureID: "provider-failure-2",
		ExecutionID:       "execution-2",
		ResultID:          "result-2",
		ProposalID:        "proposal-old-2",
		ApprovalID:        "approval-old-2",
		PreparedAuditID:   "mcp-audit-old-2",
		UserID:            "user-sensitive-2",
		SkillID:           "skill-2",
		ToolName:          "conversation.profile.update",
		ResourceType:      "conversation",
		ResourceID:        "conversation-sensitive-2",
		Classification:    "TOOL_PROVIDER_UNAVAILABLE",
		Status:            types.ProviderFailureStatusDLQ,
		Retryable:         false,
		RetryCount:        3,
		DeadLetteredAt:    &now,
		FailureRef:        "action-executor://executions/execution-2/provider-failures/provider-failure-2",
		CreatedAt:         now,
	}
	output := newProviderFailureReplayOperatorUIOutput(
		types.ProviderFailureAuditOptions{TenantID: "tenant-1", Status: types.ProviderFailureStatusDLQ, Limit: 50},
		[]types.ProviderFailureAuditRow{row},
	)
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("encode output: %v", err)
	}
	rawString := string(encoded)
	for _, forbidden := range []string{"user-sensitive-2", "conversation-sensitive-2", row.FailureRef} {
		if strings.Contains(rawString, forbidden) {
			t.Fatalf("operator UI leaked %q: %s", forbidden, rawString)
		}
	}
	if output.Kind != "action-executor.provider-failure.replay-operator-ui" ||
		output.ExecutesTool ||
		output.MutatesProviderFailure ||
		!output.RequiresOperatorApproval ||
		!output.RequiresFreshApproval ||
		!output.RequiresPreparedAudit ||
		!output.RequiresNewInput ||
		!output.RequiresReasonSHA256 ||
		!output.DryRun {
		t.Fatalf("unexpected operator UI safety flags: %+v", output)
	}
	if output.BatchID == "" ||
		output.PermissionGate == "" ||
		output.AuditContract == "" ||
		output.EvalGate != "action-provider-replay-operator-ui-first-path" ||
		len(output.RedriveRequirements) == 0 {
		t.Fatalf("operator UI missing workflow fields: %+v", output)
	}
	if len(output.Rows) != 1 ||
		output.Rows[0].ReplayCandidateID == "" ||
		output.Rows[0].ReplayState != "AWAITING_FRESH_APPROVAL" ||
		output.Rows[0].UserIDHash == "" ||
		output.Rows[0].ResourceIDHash == "" ||
		output.Rows[0].FailureRefHash == "" {
		t.Fatalf("unexpected operator UI row: %+v", output.Rows)
	}
}

func TestProviderFailureOperatorRejectsInvalidStatusAndReasonFile(t *testing.T) {
	if err := validateProviderFailureAuditStatus("FAILED"); err == nil {
		t.Fatal("expected invalid status to fail closed")
	}
	t.Setenv("NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_AUDIT_LIMIT", "not-an-int")
	if _, err := providerFailureAuditOptionsFromEnv("NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_AUDIT"); err == nil {
		t.Fatal("expected invalid limit to fail closed")
	}
	t.Setenv("NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE_REASON_FILE", filepath.Join(t.TempDir(), "missing.txt"))
	if _, err := actionProviderFailureReasonSHA256FromEnv("NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE_REASON_FILE"); err == nil {
		t.Fatal("expected missing reason file to fail closed")
	}
}

func actionProviderFailureReasonSHA256FromEnvWithPath(path string) (string, error) {
	previous := os.Getenv("NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE_REASON_FILE")
	_ = os.Setenv("NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE_REASON_FILE", path)
	defer func() {
		_ = os.Setenv("NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE_REASON_FILE", previous)
	}()
	return actionProviderFailureReasonSHA256FromEnv("NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE_REASON_FILE")
}
