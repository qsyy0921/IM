package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func finish(cfg config, result *summary, runErr error) error {
	result.FinishedAt = time.Now().UTC()
	if runErr != nil {
		result.Error = runErr.Error()
	}
	if err := os.MkdirAll(cfg.ResultDir, 0o755); err != nil {
		return fmt.Errorf("create result dir: %w", err)
	}
	if err := writeJSON(filepath.Join(cfg.ResultDir, "hotgroup-summary.json"), result); err != nil {
		return err
	}
	if err := writeUsers(filepath.Join(cfg.ResultDir, "users.jsonl"), result.UserPlan); err != nil {
		return err
	}
	if err := writeReport(filepath.Join(cfg.ResultDir, "hotgroup-report.md"), result); err != nil {
		return err
	}
	return runErr
}

func writeJSON(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func writeUsers(path string, plan userPlan) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create users jsonl: %w", err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	for _, user := range allMembers(plan) {
		if err := encoder.Encode(user); err != nil {
			return fmt.Errorf("write user model: %w", err)
		}
	}
	return nil
}

func writeReport(path string, result *summary) error {
	var builder strings.Builder
	builder.WriteString("# Hot Group Loadtest Report\n\n")
	builder.WriteString("本报告由 `loadtest/hotgroup` 生成。v0.1 只覆盖 GROUP 创建、批量成员 JOIN、SendMessage、PullInbox、AckDelivery 抽样和 PostgreSQL 统计；不包含 WebSocket notify storm。\n\n")
	fmt.Fprintf(&builder, "- run_name: `%s`\n", result.RunName)
	fmt.Fprintf(&builder, "- commit: `%s`\n", result.Commit)
	fmt.Fprintf(&builder, "- dry_run: `%t`\n", result.DryRun)
	fmt.Fprintf(&builder, "- success: `%t`\n", result.Success)
	if result.Error != "" {
		fmt.Fprintf(&builder, "- error: `%s`\n", result.Error)
	}
	fmt.Fprintf(&builder, "- tenant_id: `%s`\n", result.TenantID)
	fmt.Fprintf(&builder, "- conversation_id: `%s`\n", result.ConversationID)
	fmt.Fprintf(&builder, "- group_size: `%d`\n", result.GroupSize)
	fmt.Fprintf(&builder, "- sender_count: `%d`\n", result.SenderCount)
	fmt.Fprintf(&builder, "- message_count: `%d`\n", result.MessageCount)
	fmt.Fprintf(&builder, "- expected_inbox_rows: `%d`\n\n", result.ExpectedInboxRows)
	fmt.Fprintf(&builder, "- require_delivery_outbox_drain: `%t`\n\n", result.RequireDeliveryOutboxDrain)
	builder.WriteString("## User Model\n\n")
	fmt.Fprintf(&builder, "- owner: `%s`\n", result.UserPlan.Owner.UserID)
	fmt.Fprintf(&builder, "- senders: `%d`\n", len(result.UserPlan.Senders))
	fmt.Fprintf(&builder, "- receivers: `%d`\n", len(result.UserPlan.Receivers))
	fmt.Fprintf(&builder, "- online_fast: `%d`\n", result.UserPlan.OnlineFast)
	fmt.Fprintf(&builder, "- online_slow: `%d`\n", result.UserPlan.OnlineSlow)
	fmt.Fprintf(&builder, "- offline: `%d`\n\n", result.UserPlan.Offline)
	builder.WriteString("## Send\n\n")
	fmt.Fprintf(&builder, "- success: `%d`\n", result.Send.SuccessCount)
	fmt.Fprintf(&builder, "- errors: `%d`\n", result.Send.ErrorCount)
	fmt.Fprintf(&builder, "- p95_ms: `%.2f`\n", result.Send.LatencyP95MS)
	fmt.Fprintf(&builder, "- p99_ms: `%.2f`\n", result.Send.LatencyP99MS)
	fmt.Fprintf(&builder, "- max_seq: `%d`\n\n", result.Send.MaxSeq)
	builder.WriteString("## Receiver Sample\n\n")
	fmt.Fprintf(&builder, "- sampled_receivers: `%d`\n", result.Receiver.SampledReceivers)
	fmt.Fprintf(&builder, "- pull_success: `%d`\n", result.Receiver.PullSuccessCount)
	fmt.Fprintf(&builder, "- pull_errors: `%d`\n", result.Receiver.PullErrorCount)
	fmt.Fprintf(&builder, "- ack_success: `%d`\n", result.Receiver.AckSuccessCount)
	fmt.Fprintf(&builder, "- ack_errors: `%d`\n", result.Receiver.AckErrorCount)
	fmt.Fprintf(&builder, "- pull_p95_ms: `%.2f`\n\n", result.Receiver.PullLatencyP95MS)
	if result.Postgres != nil {
		builder.WriteString("## PostgreSQL Stats\n\n")
		fmt.Fprintf(&builder, "- active_conversation_members: `%d`\n", result.Postgres.ConversationMemberCount)
		fmt.Fprintf(&builder, "- active_delivery_membership: `%d`\n", result.Postgres.DeliveryMembershipActiveCount)
		fmt.Fprintf(&builder, "- message_log_count: `%d`\n", result.Postgres.MessageLogCount)
		fmt.Fprintf(&builder, "- user_inbox_rows: `%d`\n", result.Postgres.UserInboxRows)
		fmt.Fprintf(&builder, "- message_outbox_pending: `%d`\n", result.Postgres.MessageOutboxPending)
		fmt.Fprintf(&builder, "- delivery_outbox_pending: `%d`\n", result.Postgres.DeliveryOutboxPending)
		fmt.Fprintf(&builder, "- message_outbox_dlq: `%d`\n", result.Postgres.MessageOutboxDLQ)
		fmt.Fprintf(&builder, "- delivery_outbox_dlq: `%d`\n\n", result.Postgres.DeliveryOutboxDLQ)
	}
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func gitOutput(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func shortCommit() string {
	commit := gitOutput("rev-parse", "HEAD")
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

func gitStatusShort() string {
	return gitOutput("status", "--short")
}

func gitDirty() bool {
	return strings.TrimSpace(gitStatusShort()) != ""
}

func summarizeLatencies(values []float64) (float64, float64, float64) {
	if len(values) == 0 {
		return 0, 0, 0
	}
	copied := append([]float64(nil), values...)
	sort.Float64s(copied)
	var total float64
	for _, value := range copied {
		total += value
	}
	return total / float64(len(copied)), percentile(copied, 0.95), percentile(copied, 0.99)
}

func percentile(sorted []float64, quantile float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	index := int(math.Ceil(float64(len(sorted))*quantile)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}
