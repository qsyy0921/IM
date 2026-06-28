package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildUserPlanSplitsRolesAndOnlineModes(t *testing.T) {
	cfg := config{
		TenantID:        "tenant-hot",
		ConversationID:  "conv-hot",
		GroupSize:       10,
		SenderCount:     2,
		OnlineRatio:     0.5,
		SlowClientRatio: 0.25,
	}
	plan := buildUserPlan(cfg)
	if plan.Owner.UserID != "hot-owner-000001" {
		t.Fatalf("unexpected owner: %s", plan.Owner.UserID)
	}
	if len(plan.Senders) != 2 {
		t.Fatalf("sender count = %d", len(plan.Senders))
	}
	if len(plan.Receivers) != 7 {
		t.Fatalf("receiver count = %d", len(plan.Receivers))
	}
	if got := len(allMembers(plan)); got != 10 {
		t.Fatalf("all members = %d", got)
	}
	if plan.OnlineFast+plan.OnlineSlow+plan.Offline != len(plan.Receivers) {
		t.Fatalf("online split does not match receivers: %+v", plan)
	}
}

func TestParseConfigRejectsSenderCountAtLeastGroupSize(t *testing.T) {
	_, err := parseConfig([]string{
		"--group-size", "5",
		"--sender-count", "5",
	}, func(string) string { return "" })
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRunDryRunWritesSummaryAndUsers(t *testing.T) {
	dir := t.TempDir()
	err := run([]string{
		"--dry-run",
		"--run-name", "hotgroup-test",
		"--result-root", dir,
		"--tenant-id", "tenant-hot-test",
		"--conversation-id", "conv-hot-test",
		"--group-size", "12",
		"--sender-count", "3",
		"--message-count", "4",
	}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("run dry-run: %v", err)
	}
	summaryPath := filepath.Join(dir, "hotgroup-test", "hotgroup-summary.json")
	content, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	var result summary
	if err := json.Unmarshal(content, &result); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if !result.Success || !result.DryRun {
		t.Fatalf("unexpected dry-run result: success=%v dry_run=%v error=%s", result.Success, result.DryRun, result.Error)
	}
	if result.ExpectedInboxRows != 48 {
		t.Fatalf("expected inbox rows = %d", result.ExpectedInboxRows)
	}
	usersPath := filepath.Join(dir, "hotgroup-test", "users.jsonl")
	if _, err := os.Stat(usersPath); err != nil {
		t.Fatalf("users.jsonl missing: %v", err)
	}
	reportPath := filepath.Join(dir, "hotgroup-test", "hotgroup-report.md")
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatalf("report missing: %v", err)
	}
}
