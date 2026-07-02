package main

import (
	"context"
	"fmt"
	"os"
	"time"
)

func main() {
	if err := run(os.Args[1:], os.Getenv); err != nil {
		fmt.Fprintf(os.Stderr, "hotgroup loadtest failed: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, getenv func(string) string) error {
	cfg, err := parseConfig(args, getenv)
	if err != nil {
		return err
	}
	plan := buildUserPlan(cfg)
	messageCount := cfg.MessageCount
	if messageCount == 0 {
		messageCount = int(cfg.MessageRate * cfg.Duration.Seconds())
	}
	if messageCount <= 0 {
		messageCount = 1
	}
	cfg.MessageCount = messageCount
	if cfg.SendConcurrency == 0 {
		cfg.SendConcurrency = cfg.SenderCount
	}
	result := &summary{
		SchemaVersion:              2,
		RunName:                    cfg.RunName,
		Commit:                     shortCommit(),
		GitDirty:                   gitDirty(),
		GitStatusShort:             gitStatusShort(),
		RunnerMode:                 cfg.RunnerMode,
		DryRun:                     cfg.DryRun,
		SkipSetup:                  cfg.SkipSetup,
		VerifiedAuthMetadata:       cfg.VerifiedAuthMetadata,
		TenantID:                   cfg.TenantID,
		ConversationID:             cfg.ConversationID,
		GroupSize:                  cfg.GroupSize,
		SenderCount:                cfg.SenderCount,
		SendConcurrency:            cfg.SendConcurrency,
		OnlineRatio:                cfg.OnlineRatio,
		SlowClientRatio:            cfg.SlowClientRatio,
		ACKRatio:                   cfg.ACKRatio,
		MessageRate:                cfg.MessageRate,
		DurationSeconds:            cfg.Duration.Seconds(),
		MessageCount:               messageCount,
		ExpectedInboxRows:          int64(messageCount * cfg.GroupSize),
		ExpectedTimelineRows:       int64(messageCount),
		ExpectedFanoutMode:         cfg.ExpectedFanoutMode,
		RequireDeliveryOutboxDrain: cfg.RequireDeliveryOutboxDrain,
		UserPlan:                   plan,
		StartedAt:                  time.Now().UTC(),
	}
	if cfg.RunnerMode == runnerModeSubscriberOnly {
		result.ExpectedInboxRows = 0
		result.ExpectedTimelineRows = 0
	}
	runErr := execute(context.Background(), cfg, plan, result)
	if finishErr := finish(cfg, result, runErr); finishErr != nil {
		return finishErr
	}
	fmt.Printf("summary: %s\n", cfg.ResultDir)
	return nil
}

func execute(ctx context.Context, cfg config, plan userPlan, result *summary) error {
	if cfg.DryRun {
		result.Success = true
		return nil
	}
	if cfg.RunnerMode == runnerModeSubscriberOnly {
		return executeSubscriberOnly(ctx, cfg, plan, result)
	}
	pool, err := openPool(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	if cfg.Cleanup {
		if err := cleanupTenant(ctx, pool, cfg.TenantID); err != nil {
			return err
		}
	}
	clients, err := newServiceClients(cfg)
	if err != nil {
		return err
	}
	defer clients.close()
	if !cfg.SkipSetup {
		memberVersion, err := createGroup(ctx, cfg, clients, plan)
		if err != nil {
			return err
		}
		memberVersion, err = joinMembers(ctx, cfg, clients, plan, memberVersion)
		if err != nil {
			return err
		}
		_ = memberVersion
	}
	if _, err := waitForCount(ctx, pool, cfg.WaitTimeout, cfg.PollInterval, `
SELECT COUNT(*) FROM delivery_membership_projection
WHERE tenant_id = $1 AND conversation_id = $2 AND status = 'ACTIVE'
`, int64(cfg.GroupSize), cfg.TenantID, cfg.ConversationID); err != nil {
		return fmt.Errorf("wait delivery membership projection: %w", err)
	}
	fanoutMode, err := readConversationFanoutMode(ctx, pool, cfg)
	if err != nil {
		return err
	}
	result.ActualFanoutMode = fanoutMode
	result.ExpectedInboxRows = expectedInboxRowsForFanoutMode(result.MessageCount, cfg.GroupSize, fanoutMode)
	baseline, err := readProjectionBaseline(ctx, pool, cfg)
	if err != nil {
		return err
	}
	result.BaselineUserInboxRows = baseline.UserInboxRows
	result.BaselineDeliveryTimelineRows = baseline.DeliveryTimelineRows
	result.TargetUserInboxRows = baseline.UserInboxRows + result.ExpectedInboxRows
	result.TargetDeliveryTimelineRows = baseline.DeliveryTimelineRows + result.ExpectedTimelineRows
	if cfg.ExpectedFanoutMode != "" && fanoutMode != cfg.ExpectedFanoutMode {
		stats, statsErr := readPostgresStats(ctx, pool, cfg)
		if statsErr == nil {
			result.Postgres = &stats
		}
		return fmt.Errorf("fanout mode = %s, want %s", fanoutMode, cfg.ExpectedFanoutMode)
	}
	subscribers, pushStats, err := openConversationSubscribers(ctx, cfg, plan)
	result.Push = pushStats
	if err != nil {
		stats, statsErr := readPostgresStats(ctx, pool, cfg)
		if statsErr == nil {
			result.Postgres = &stats
		}
		return fmt.Errorf("open conversation subscribers: %w", err)
	}
	defer closeConversationSubscribers(subscribers)
	pushCollector := startConversationSignalCollection(ctx, cfg, subscribers, expectedConversationSignalsPerSubscriber(cfg, result.MessageCount))
	if pushCollector != nil {
		defer pushCollector.cancel()
	}
	result.Send = sendMessages(ctx, cfg, clients, plan, result.MessageCount)
	if result.Send.ErrorCount > 0 {
		stats, statsErr := readPostgresStats(ctx, pool, cfg)
		if statsErr == nil {
			result.Postgres = &stats
		}
		result.Success = false
		return fmt.Errorf("send errors: %d", result.Send.ErrorCount)
	}
	if result.ExpectedInboxRows > 0 {
		if _, err := waitForCount(ctx, pool, cfg.WaitTimeout, cfg.PollInterval, `
SELECT COUNT(*) FROM user_inbox
WHERE tenant_id = $1 AND conversation_id = $2
`, result.TargetUserInboxRows, cfg.TenantID, cfg.ConversationID); err != nil {
			stats, statsErr := readPostgresStats(ctx, pool, cfg)
			if statsErr == nil {
				result.Postgres = &stats
			}
			return fmt.Errorf("wait user inbox fanout: %w", err)
		}
	} else {
		if _, err := waitForCount(ctx, pool, cfg.WaitTimeout, cfg.PollInterval, `
SELECT COUNT(*) FROM delivery_timeline_items
WHERE tenant_id = $1 AND conversation_id = $2
`, result.TargetDeliveryTimelineRows, cfg.TenantID, cfg.ConversationID); err != nil {
			stats, statsErr := readPostgresStats(ctx, pool, cfg)
			if statsErr == nil {
				result.Postgres = &stats
			}
			return fmt.Errorf("wait delivery timeline read-fanout rows: %w", err)
		}
	}
	if pushCollector != nil {
		pushStats, err := pushCollector.wait(cfg, result.Push)
		result.Push = pushStats
		if err != nil {
			stats, statsErr := readPostgresStats(ctx, pool, cfg)
			if statsErr == nil {
				result.Postgres = &stats
			}
			return fmt.Errorf("wait conversation signals: %w", err)
		}
	}
	result.Receiver = pullAndAckSample(ctx, cfg, clients, plan, result.Send.MaxSeq)
	if cfg.RequireDeliveryOutboxDrain {
		if _, err := waitForZero(ctx, pool, cfg.WaitTimeout, cfg.PollInterval, `
SELECT COUNT(*) FROM delivery_outbox
WHERE tenant_id = $1 AND conversation_id = $2 AND status = 'PENDING'
`, cfg.TenantID, cfg.ConversationID); err != nil {
			stats, statsErr := readPostgresStats(ctx, pool, cfg)
			if statsErr == nil {
				result.Postgres = &stats
			}
			return fmt.Errorf("wait delivery outbox drain: %w", err)
		}
	}
	stats, err := readPostgresStats(ctx, pool, cfg)
	if err != nil {
		return err
	}
	result.Postgres = &stats
	result.ActualFanoutMode = stats.FanoutMode
	result.Success = result.Send.SuccessCount == result.MessageCount &&
		result.Receiver.PullErrorCount == 0 &&
		result.Postgres.UserInboxRows >= result.TargetUserInboxRows &&
		result.Postgres.DeliveryTimelineRows >= result.TargetDeliveryTimelineRows &&
		result.Postgres.MessageOutboxDLQ == 0 &&
		result.Postgres.DeliveryOutboxDLQ == 0 &&
		(!cfg.RequireConversationNotify ||
			result.Push.ConversationSignalCount >= expectedConversationSignalCount(cfg, result.MessageCount, result.Push.SubscriberCount)) &&
		(!cfg.RequireDeliveryOutboxDrain || result.Postgres.DeliveryOutboxPending == 0)
	if !result.Success {
		return fmt.Errorf("hotgroup validation failed")
	}
	return nil
}

func executeSubscriberOnly(ctx context.Context, cfg config, plan userPlan, result *summary) error {
	subscribers, pushStats, err := openConversationSubscribers(ctx, cfg, plan)
	result.Push = pushStats
	if err != nil {
		return fmt.Errorf("open conversation subscribers: %w", err)
	}
	defer closeConversationSubscribers(subscribers)
	pushCollector := startConversationSignalCollection(ctx, cfg, subscribers, expectedConversationSignalsPerSubscriber(cfg, result.MessageCount))
	if pushCollector != nil {
		defer pushCollector.cancel()
	}
	pushStats, err = pushCollector.wait(cfg, result.Push)
	result.Push = pushStats
	if err != nil {
		return fmt.Errorf("wait conversation signals: %w", err)
	}
	result.Success = !cfg.RequireConversationNotify ||
		result.Push.ConversationSignalCount >= expectedConversationSignalCount(cfg, result.MessageCount, result.Push.SubscriberCount)
	if !result.Success {
		return fmt.Errorf("hotgroup subscriber-only validation failed")
	}
	return nil
}

func expectedInboxRowsForFanoutMode(messageCount int, groupSize int, fanoutMode string) int64 {
	switch fanoutMode {
	case "READ_FANOUT", "BROADCAST_SIGNAL":
		return 0
	default:
		return int64(messageCount * groupSize)
	}
}
