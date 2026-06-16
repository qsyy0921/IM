package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type config struct {
	pgDSN                     string
	cleanup                   bool
	messageTenantID           string
	messageConversationPrefix string
	messageConversationCount  int
	messageVUs                int
	conversationTenantID      string
	conversationID            string
	conversationOwnerUserID   string
	deliveryTenantID          string
	deliveryConversationID    string
	deliveryUserID            string
	deliveryMessageCount      int
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "capacity seed failed: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, cfg.pgDSN)
	if err != nil {
		return fmt.Errorf("open postgres pool: %w", err)
	}
	defer pool.Close()
	if err := seed(ctx, pool, cfg); err != nil {
		return err
	}
	fmt.Printf(
		"OK   capacity seed ready: message tenant=%s conversations=%d vus=%d; memberchange tenant=%s conversation=%s; delivery tenant=%s conversation=%s items=%d\n",
		cfg.messageTenantID,
		cfg.messageConversationCount,
		cfg.messageVUs,
		cfg.conversationTenantID,
		cfg.conversationID,
		cfg.deliveryTenantID,
		cfg.deliveryConversationID,
		cfg.deliveryMessageCount,
	)
	return nil
}

func parseConfig(args []string) (config, error) {
	cfg := config{}
	flags := flag.NewFlagSet("capacityseed", flag.ContinueOnError)
	flags.StringVar(&cfg.pgDSN, "pg-dsn", "", "PostgreSQL DSN")
	flags.BoolVar(&cfg.cleanup, "cleanup", false, "delete existing fixture rows for the configured fixture tenants before seeding")
	flags.StringVar(&cfg.messageTenantID, "message-tenant-id", "tenant-capacity-message", "tenant id used by loadtest/sendmessage")
	flags.StringVar(&cfg.messageConversationPrefix, "message-conversation-prefix", "conv-capacity-message", "conversation prefix used by loadtest/sendmessage")
	flags.IntVar(&cfg.messageConversationCount, "message-conversation-count", 10, "number of message-service conversations to seed")
	flags.IntVar(&cfg.messageVUs, "message-vus", 2, "number of message-service VU sender users to seed per conversation")
	flags.StringVar(&cfg.conversationTenantID, "conversation-tenant-id", "tenant-capacity-conversation", "tenant id used by loadtest/memberchange")
	flags.StringVar(&cfg.conversationID, "conversation-id", "conv-capacity-memberchange", "conversation id used by loadtest/memberchange")
	flags.StringVar(&cfg.conversationOwnerUserID, "conversation-owner-user-id", "owner-1", "ACTIVE owner user for memberchange loadtest")
	flags.StringVar(&cfg.deliveryTenantID, "delivery-tenant-id", "tenant-capacity-delivery", "tenant id used by loadtest/delivery")
	flags.StringVar(&cfg.deliveryConversationID, "delivery-conversation-id", "conv-capacity-delivery", "conversation id used by loadtest/delivery")
	flags.StringVar(&cfg.deliveryUserID, "delivery-user-id", "delivery-user-1", "user id used by loadtest/delivery")
	flags.IntVar(&cfg.deliveryMessageCount, "delivery-message-count", 1, "number of user_inbox items to seed for loadtest/delivery")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if strings.TrimSpace(cfg.pgDSN) == "" {
		return config{}, errors.New("--pg-dsn is required")
	}
	if strings.TrimSpace(cfg.messageTenantID) == "" {
		return config{}, errors.New("--message-tenant-id is required")
	}
	if strings.TrimSpace(cfg.messageConversationPrefix) == "" {
		return config{}, errors.New("--message-conversation-prefix is required")
	}
	if cfg.messageConversationCount <= 0 {
		return config{}, errors.New("--message-conversation-count must be positive")
	}
	if cfg.messageVUs <= 0 {
		return config{}, errors.New("--message-vus must be positive")
	}
	if strings.TrimSpace(cfg.conversationTenantID) == "" {
		return config{}, errors.New("--conversation-tenant-id is required")
	}
	if strings.TrimSpace(cfg.conversationID) == "" {
		return config{}, errors.New("--conversation-id is required")
	}
	if strings.TrimSpace(cfg.conversationOwnerUserID) == "" {
		return config{}, errors.New("--conversation-owner-user-id is required")
	}
	if strings.TrimSpace(cfg.deliveryTenantID) == "" {
		return config{}, errors.New("--delivery-tenant-id is required")
	}
	if strings.TrimSpace(cfg.deliveryConversationID) == "" {
		return config{}, errors.New("--delivery-conversation-id is required")
	}
	if strings.TrimSpace(cfg.deliveryUserID) == "" {
		return config{}, errors.New("--delivery-user-id is required")
	}
	if cfg.deliveryMessageCount <= 0 {
		return config{}, errors.New("--delivery-message-count must be positive")
	}
	return cfg, nil
}

func seed(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin seed transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if cfg.cleanup {
		if err := cleanupTenant(ctx, tx, cfg.messageTenantID); err != nil {
			return err
		}
		if cfg.conversationTenantID != cfg.messageTenantID {
			if err := cleanupTenant(ctx, tx, cfg.conversationTenantID); err != nil {
				return err
			}
		}
		if err := cleanupDeliveryTenant(ctx, tx, cfg.deliveryTenantID); err != nil {
			return err
		}
	}
	if err := seedMessageFixtures(ctx, tx, cfg); err != nil {
		return err
	}
	if err := seedMemberChangeFixture(ctx, tx, cfg); err != nil {
		return err
	}
	if err := seedDeliveryFixture(ctx, tx, cfg); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit seed transaction: %w", err)
	}
	return nil
}

func cleanupTenant(ctx context.Context, tx pgx.Tx, tenantID string) error {
	statements := []string{
		`DELETE FROM message_command_idempotency WHERE tenant_id = $1`,
		`DELETE FROM message_change_history WHERE tenant_id = $1`,
		`DELETE FROM timeline_gap_markers WHERE tenant_id = $1`,
		`DELETE FROM seq_allocation_journal WHERE tenant_id = $1`,
		`DELETE FROM message_outbox WHERE tenant_id = $1`,
		`DELETE FROM conversation_timeline_events WHERE tenant_id = $1`,
		`DELETE FROM message_log WHERE tenant_id = $1`,
		`DELETE FROM conversation_seq WHERE tenant_id = $1`,
		`DELETE FROM member_change_saga WHERE tenant_id = $1`,
		`DELETE FROM conversation_members WHERE tenant_id = $1`,
		`DELETE FROM conversations WHERE tenant_id = $1`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement, tenantID); err != nil {
			return fmt.Errorf("cleanup tenant %s: %w", tenantID, err)
		}
	}
	return nil
}

func cleanupDeliveryTenant(ctx context.Context, tx pgx.Tx, tenantID string) error {
	statements := []string{
		`DELETE FROM delivery_outbox WHERE tenant_id = $1`,
		`DELETE FROM device_delivery_cursors WHERE tenant_id = $1`,
		`DELETE FROM delivery_membership_projection WHERE tenant_id = $1`,
		`DELETE FROM user_inbox WHERE tenant_id = $1`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement, tenantID); err != nil {
			return fmt.Errorf("cleanup delivery tenant %s: %w", tenantID, err)
		}
	}
	return nil
}

func seedMessageFixtures(ctx context.Context, tx pgx.Tx, cfg config) error {
	for index := 0; index < cfg.messageConversationCount; index++ {
		conversationID := fmt.Sprintf("%s-%d", cfg.messageConversationPrefix, index)
		if err := upsertConversation(ctx, tx, cfg.messageTenantID, conversationID, "GROUP", "WRITE_FANOUT"); err != nil {
			return err
		}
		if err := upsertConversationSeq(ctx, tx, cfg.messageTenantID, conversationID, 0); err != nil {
			return err
		}
		if err := upsertMember(ctx, tx, cfg.messageTenantID, conversationID, "owner-1", "OWNER", 1); err != nil {
			return err
		}
		for vu := 0; vu < cfg.messageVUs; vu++ {
			userID := fmt.Sprintf("user-%d", vu)
			if err := upsertMember(ctx, tx, cfg.messageTenantID, conversationID, userID, "MEMBER", 1); err != nil {
				return err
			}
		}
	}
	return nil
}

func seedMemberChangeFixture(ctx context.Context, tx pgx.Tx, cfg config) error {
	if err := upsertConversation(ctx, tx, cfg.conversationTenantID, cfg.conversationID, "GROUP", "WRITE_FANOUT"); err != nil {
		return err
	}
	if err := upsertConversationSeq(ctx, tx, cfg.conversationTenantID, cfg.conversationID, 0); err != nil {
		return err
	}
	return upsertMember(ctx, tx, cfg.conversationTenantID, cfg.conversationID, cfg.conversationOwnerUserID, "OWNER", 1)
}

func seedDeliveryFixture(ctx context.Context, tx pgx.Tx, cfg config) error {
	if _, err := tx.Exec(ctx, `
INSERT INTO delivery_membership_projection (
    tenant_id, conversation_id, user_id, role, status, join_seq, leave_seq,
    member_version, permission_version, updated_by_event_id, updated_at
)
VALUES ($1, $2, $3, 'MEMBER', 'ACTIVE', 1, NULL, 1, 1, $4, now())
ON CONFLICT (tenant_id, conversation_id, user_id) DO UPDATE SET
    role = EXCLUDED.role,
    status = EXCLUDED.status,
    join_seq = EXCLUDED.join_seq,
    leave_seq = EXCLUDED.leave_seq,
    member_version = EXCLUDED.member_version,
    permission_version = EXCLUDED.permission_version,
    updated_by_event_id = EXCLUDED.updated_by_event_id,
    updated_at = now()
`, cfg.deliveryTenantID, cfg.deliveryConversationID, cfg.deliveryUserID, "capacity-seed-membership"); err != nil {
		return fmt.Errorf("seed delivery membership projection: %w", err)
	}
	for seq := 1; seq <= cfg.deliveryMessageCount; seq++ {
		eventID := fmt.Sprintf("capacity-seed-delivery-event-%d", seq)
		messageID := fmt.Sprintf("capacity-seed-message-%d", seq)
		if _, err := tx.Exec(ctx, `
INSERT INTO user_inbox (
    tenant_id, user_id, conversation_id, conversation_seq, event_id, event_type,
    message_id, sender_id, payload_json, fanout_mode, permission_version
)
VALUES ($1, $2, $3, $4, $5, 'message.persisted.v1', $6, 'capacity-seed-sender', $7::jsonb, 'WRITE_FANOUT', 1)
ON CONFLICT (tenant_id, user_id, conversation_id, conversation_seq) DO UPDATE SET
    event_id = EXCLUDED.event_id,
    event_type = EXCLUDED.event_type,
    message_id = EXCLUDED.message_id,
    sender_id = EXCLUDED.sender_id,
    payload_json = EXCLUDED.payload_json,
    fanout_mode = EXCLUDED.fanout_mode,
    permission_version = EXCLUDED.permission_version
`, cfg.deliveryTenantID, cfg.deliveryUserID, cfg.deliveryConversationID, seq, eventID, messageID, fmt.Sprintf(`{"seed":true,"seq":%d}`, seq)); err != nil {
			return fmt.Errorf("seed delivery inbox item %d: %w", seq, err)
		}
	}
	return nil
}

func upsertConversation(ctx context.Context, tx pgx.Tx, tenantID string, conversationID string, conversationType string, fanoutMode string) error {
	if _, err := tx.Exec(ctx, `
INSERT INTO conversations (
    tenant_id, conversation_id, conversation_type, status, conversation_mode,
    fanout_mode, fanout_policy_version, member_version, permission_version, current_seq_shard
)
VALUES ($1, $2, $3, 'ACTIVE', 'LOCAL_ROW_LOCK', $4, 1, 1, 1, 'local')
ON CONFLICT (tenant_id, conversation_id) DO UPDATE SET
    conversation_type = EXCLUDED.conversation_type,
    status = 'ACTIVE',
    conversation_mode = 'LOCAL_ROW_LOCK',
    fanout_mode = EXCLUDED.fanout_mode,
    fanout_policy_version = 1,
    member_version = GREATEST(conversations.member_version, 1),
    permission_version = GREATEST(conversations.permission_version, 1),
    current_seq_shard = 'local',
    updated_at = now()
`, tenantID, conversationID, conversationType, fanoutMode); err != nil {
		return fmt.Errorf("upsert conversation %s/%s: %w", tenantID, conversationID, err)
	}
	return nil
}

func upsertConversationSeq(ctx context.Context, tx pgx.Tx, tenantID string, conversationID string, currentSeq int64) error {
	if _, err := tx.Exec(ctx, `
INSERT INTO conversation_seq (tenant_id, conversation_id, current_seq)
VALUES ($1, $2, $3)
ON CONFLICT (tenant_id, conversation_id) DO UPDATE SET
    current_seq = GREATEST(conversation_seq.current_seq, EXCLUDED.current_seq),
    updated_at = now()
`, tenantID, conversationID, currentSeq); err != nil {
		return fmt.Errorf("upsert conversation seq %s/%s: %w", tenantID, conversationID, err)
	}
	return nil
}

func upsertMember(ctx context.Context, tx pgx.Tx, tenantID string, conversationID string, userID string, role string, version int64) error {
	if _, err := tx.Exec(ctx, `
INSERT INTO conversation_members (
    tenant_id, conversation_id, user_id, role, status, join_seq, leave_seq,
    member_version, permission_version
)
VALUES ($1, $2, $3, $4, 'ACTIVE', 0, NULL, $5, $5)
ON CONFLICT (tenant_id, conversation_id, user_id) DO UPDATE SET
    role = EXCLUDED.role,
    status = 'ACTIVE',
    leave_seq = NULL,
    member_version = GREATEST(conversation_members.member_version, EXCLUDED.member_version),
    permission_version = GREATEST(conversation_members.permission_version, EXCLUDED.permission_version),
    updated_at = now()
`, tenantID, conversationID, userID, role, version); err != nil {
		return fmt.Errorf("upsert member %s/%s/%s: %w", tenantID, conversationID, userID, err)
	}
	return nil
}
