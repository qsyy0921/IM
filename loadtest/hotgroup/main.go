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
	result := &summary{
		SchemaVersion:              1,
		RunName:                    cfg.RunName,
		Commit:                     shortCommit(),
		GitDirty:                   gitDirty(),
		GitStatusShort:             gitStatusShort(),
		DryRun:                     cfg.DryRun,
		VerifiedAuthMetadata:       cfg.VerifiedAuthMetadata,
		TenantID:                   cfg.TenantID,
		ConversationID:             cfg.ConversationID,
		GroupSize:                  cfg.GroupSize,
		SenderCount:                cfg.SenderCount,
		OnlineRatio:                cfg.OnlineRatio,
		SlowClientRatio:            cfg.SlowClientRatio,
		ACKRatio:                   cfg.ACKRatio,
		MessageRate:                cfg.MessageRate,
		DurationSeconds:            cfg.Duration.Seconds(),
		MessageCount:               messageCount,
		ExpectedInboxRows:          int64(messageCount * cfg.GroupSize),
		RequireDeliveryOutboxDrain: cfg.RequireDeliveryOutboxDrain,
		UserPlan:                   plan,
		StartedAt:                  time.Now().UTC(),
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
	memberVersion, err := createGroup(ctx, cfg, clients, plan)
	if err != nil {
		return err
	}
	memberVersion, err = joinMembers(ctx, cfg, clients, plan, memberVersion)
	if err != nil {
		return err
	}
	_ = memberVersion
	if _, err := waitForCount(ctx, pool, cfg.WaitTimeout, cfg.PollInterval, `
SELECT COUNT(*) FROM delivery_membership_projection
WHERE tenant_id = $1 AND conversation_id = $2 AND status = 'ACTIVE'
`, int64(cfg.GroupSize), cfg.TenantID, cfg.ConversationID); err != nil {
		return fmt.Errorf("wait delivery membership projection: %w", err)
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
	if _, err := waitForCount(ctx, pool, cfg.WaitTimeout, cfg.PollInterval, `
SELECT COUNT(*) FROM user_inbox
WHERE tenant_id = $1 AND conversation_id = $2
`, result.ExpectedInboxRows, cfg.TenantID, cfg.ConversationID); err != nil {
		stats, statsErr := readPostgresStats(ctx, pool, cfg)
		if statsErr == nil {
			result.Postgres = &stats
		}
		return fmt.Errorf("wait user inbox fanout: %w", err)
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
	result.Success = result.Send.SuccessCount == result.MessageCount &&
		result.Receiver.PullErrorCount == 0 &&
		result.Postgres.UserInboxRows >= result.ExpectedInboxRows &&
		result.Postgres.MessageOutboxDLQ == 0 &&
		result.Postgres.DeliveryOutboxDLQ == 0 &&
		(!cfg.RequireDeliveryOutboxDrain || result.Postgres.DeliveryOutboxPending == 0)
	if !result.Success {
		return fmt.Errorf("hotgroup validation failed")
	}
	return nil
}
