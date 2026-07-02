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

func TestParseConfigRejectsNegativeSendConcurrency(t *testing.T) {
	_, err := parseConfig([]string{
		"--send-concurrency", "-1",
	}, func(string) string { return "" })
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestParseConfigRejectsSkipSetupWithCleanup(t *testing.T) {
	_, err := parseConfig([]string{
		"--skip-setup",
		"--cleanup",
	}, func(string) string { return "" })
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRunDryRunDefaultsSendConcurrencyToSenderCount(t *testing.T) {
	dir := t.TempDir()
	err := run([]string{
		"--dry-run",
		"--run-name", "hotgroup-send-concurrency-default",
		"--result-root", dir,
		"--tenant-id", "tenant-hot-test",
		"--conversation-id", "conv-hot-test",
		"--group-size", "12",
		"--sender-count", "4",
		"--message-count", "4",
	}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("run dry-run: %v", err)
	}
	summaryPath := filepath.Join(dir, "hotgroup-send-concurrency-default", "hotgroup-summary.json")
	content, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	var result summary
	if err := json.Unmarshal(content, &result); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if result.SendConcurrency != 4 {
		t.Fatalf("send concurrency = %d", result.SendConcurrency)
	}
}

func TestParseConfigSubscriberOnlyDoesNotRequirePostgres(t *testing.T) {
	cfg, err := parseConfig([]string{
		"--runner-mode", "subscriber-only",
		"--conversation-subscriber-count", "8",
		"--subscriber-shard-count", "2",
		"--subscriber-shard-index", "1",
		"--push-url", "ws://127.0.0.1:10498/ws",
		"--message-count", "10",
	}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("parse subscriber-only config: %v", err)
	}
	if cfg.RunnerMode != runnerModeSubscriberOnly {
		t.Fatalf("runner mode = %s", cfg.RunnerMode)
	}
	if cfg.PGDSN != "" {
		t.Fatalf("pg dsn should not be required in subscriber-only mode: %s", cfg.PGDSN)
	}
}

func TestConversationSignalSampleEveryAdjustsExpectedSignals(t *testing.T) {
	cfg, err := parseConfig([]string{
		"--runner-mode", "subscriber-only",
		"--conversation-subscriber-count", "8",
		"--subscriber-shard-count", "2",
		"--subscriber-shard-index", "1",
		"--push-url", "ws://127.0.0.1:10498/ws",
		"--message-count", "1000",
		"--conversation-signal-sample-every", "10",
	}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("parse sampled signal config: %v", err)
	}
	if cfg.ConversationSignalSampleEvery != 10 {
		t.Fatalf("sample every = %d", cfg.ConversationSignalSampleEvery)
	}
	if got, want := expectedConversationSignalsPerSubscriber(cfg, 1000), 100; got != want {
		t.Fatalf("expected per subscriber = %d, want %d", got, want)
	}
	if got, want := expectedConversationSignalCount(cfg, 1000, 4), 400; got != want {
		t.Fatalf("expected total = %d, want %d", got, want)
	}
	if got, want := expectedConversationSignalsPerSubscriber(cfg, 9), 0; got != want {
		t.Fatalf("expected small sampled run per subscriber = %d, want %d", got, want)
	}
}

func TestParseConfigRejectsInvalidSubscriberShard(t *testing.T) {
	_, err := parseConfig([]string{
		"--runner-mode", "subscriber-only",
		"--conversation-subscriber-count", "8",
		"--subscriber-shard-count", "2",
		"--subscriber-shard-index", "2",
		"--push-url", "ws://127.0.0.1:10498/ws",
	}, func(string) string { return "" })
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestShardReceiversIsDeterministic(t *testing.T) {
	cfg := config{
		TenantID:        "tenant-hot",
		ConversationID:  "conv-hot",
		GroupSize:       12,
		SenderCount:     1,
		OnlineRatio:     1,
		SlowClientRatio: 0,
	}
	plan := buildUserPlan(cfg)
	receivers := sampledReceivers(plan, 8)
	shard0 := shardReceivers(receivers, 3, 0)
	shard1 := shardReceivers(receivers, 3, 1)
	shard2 := shardReceivers(receivers, 3, 2)
	if got, want := len(shard0), 3; got != want {
		t.Fatalf("shard0 len = %d, want %d", got, want)
	}
	if got, want := len(shard1), 3; got != want {
		t.Fatalf("shard1 len = %d, want %d", got, want)
	}
	if got, want := len(shard2), 2; got != want {
		t.Fatalf("shard2 len = %d, want %d", got, want)
	}
	if shard0[0].UserID != "hot-user-000001" || shard1[0].UserID != "hot-user-000002" || shard2[0].UserID != "hot-user-000003" {
		t.Fatalf("unexpected shard starts: %s %s %s", shard0[0].UserID, shard1[0].UserID, shard2[0].UserID)
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
		"--skip-setup",
		"--send-concurrency", "2",
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
	if !result.SkipSetup {
		t.Fatal("expected skip_setup in summary")
	}
	if result.SchemaVersion != 2 {
		t.Fatalf("schema version = %d", result.SchemaVersion)
	}
	if result.RunnerMode != runnerModeFull {
		t.Fatalf("runner mode = %s", result.RunnerMode)
	}
	if result.SendConcurrency != 2 {
		t.Fatalf("send concurrency = %d", result.SendConcurrency)
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
