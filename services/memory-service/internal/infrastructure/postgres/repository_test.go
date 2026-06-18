package postgres

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/memory-service/internal/types"
)

func TestRepositoryProjectAndQueryMemoryEventsIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openMemoryTestPool(t)
	resetMemoryTables(t, ctx, pool)
	repository := NewRepository(pool)

	projectMemory(t, ctx, repository, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-1",
		EventID:           "event-member-join",
		EventType:         types.TimelineEventConversationMemberJoined,
		ConversationID:    "conv-1",
		ConversationSeq:   1,
		ConsumerGroup:     "memory-test",
		Topic:             "conversation.timeline.events",
		PartitionID:       0,
		OffsetValue:       2,
		TargetUserID:      "user-1",
		MemberRole:        types.MemoryMemberRoleMember,
		MemberStatus:      types.MemoryMemberStatusActive,
		MemberVersion:     1,
		PermissionVersion: 1,
	})
	projectMemory(t, ctx, repository, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-1",
		EventID:           "event-message-1",
		EventType:         types.TimelineEventMessagePersisted,
		ConversationID:    "conv-1",
		ConversationSeq:   2,
		ConsumerGroup:     "memory-test",
		Topic:             "conversation.timeline.events",
		PartitionID:       0,
		OffsetValue:       3,
		MessageID:         "msg-1",
		SenderID:          "user-2",
		FactText:          "decision: ship the retrieval gateway after memory projection",
		PermissionVersion: 7,
	})

	items, projectionVersion, err := repository.QueryMemoryEvents(ctx, types.QueryMemoryEventsCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		Query:    "retrieval gateway",
		Statuses: []string{types.MemoryStatusPending},
		Limit:    10,
	}, 10)
	if err != nil {
		t.Fatalf("query memory events: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one memory event, got %d: %+v", len(items), items)
	}
	if items[0].MemoryEventID != "event-message-1" || items[0].Status != types.MemoryStatusPending {
		t.Fatalf("unexpected memory event: %+v", items[0])
	}
	if len(items[0].SourceRefs) != 1 || items[0].SourceRefs[0].SourceID != "msg-1" {
		t.Fatalf("expected source ref to msg-1: %+v", items[0].SourceRefs)
	}
	if projectionVersion != 2 {
		t.Fatalf("unexpected projection version %d", projectionVersion)
	}

	strangerItems, _, err := repository.QueryMemoryEvents(ctx, types.QueryMemoryEventsCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-stranger",
			DeviceID: "device-1",
		},
		Query:    "retrieval gateway",
		Statuses: []string{types.MemoryStatusPending},
		Limit:    10,
	}, 10)
	if err != nil {
		t.Fatalf("query memory events as stranger: %v", err)
	}
	if len(strangerItems) != 0 {
		t.Fatalf("stranger should not see memory events: %+v", strangerItems)
	}
}

func TestRepositoryTombstonesMemoryByMessageIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openMemoryTestPool(t)
	resetMemoryTables(t, ctx, pool)
	repository := NewRepository(pool)

	projectMemory(t, ctx, repository, types.ProjectTimelineEventCommand{
		TenantID:        "tenant-1",
		EventID:         "event-member-join",
		EventType:       types.TimelineEventConversationMemberJoined,
		ConversationID:  "conv-1",
		ConversationSeq: 1,
		ConsumerGroup:   "memory-test",
		Topic:           "conversation.timeline.events",
		PartitionID:     0,
		OffsetValue:     2,
		TargetUserID:    "user-1",
		MemberRole:      types.MemoryMemberRoleMember,
	})
	projectMemory(t, ctx, repository, types.ProjectTimelineEventCommand{
		TenantID:        "tenant-1",
		EventID:         "event-message-1",
		EventType:       types.TimelineEventMessagePersisted,
		ConversationID:  "conv-1",
		ConversationSeq: 2,
		ConsumerGroup:   "memory-test",
		Topic:           "conversation.timeline.events",
		PartitionID:     0,
		OffsetValue:     3,
		MessageID:       "msg-1",
		SenderID:        "user-2",
		FactText:        "status: this memory should be revoked",
	})
	projectMemory(t, ctx, repository, types.ProjectTimelineEventCommand{
		TenantID:        "tenant-1",
		EventID:         "event-message-1-revoked",
		EventType:       types.TimelineEventMessageRevoked,
		ConversationID:  "conv-1",
		ConversationSeq: 3,
		ConsumerGroup:   "memory-test",
		Topic:           "conversation.timeline.events",
		PartitionID:     0,
		OffsetValue:     4,
		MessageID:       "msg-1",
	})

	items, _, err := repository.QueryMemoryEvents(ctx, types.QueryMemoryEventsCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		Query:    "revoked",
		Statuses: []string{types.MemoryStatusPending},
		Limit:    10,
	}, 10)
	if err != nil {
		t.Fatalf("query memory events after revoke: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("revoked memory should be hidden: %+v", items)
	}
}

func projectMemory(t *testing.T, ctx context.Context, repository *Repository, command types.ProjectTimelineEventCommand) {
	t.Helper()
	if _, err := repository.ProjectTimelineEvent(ctx, command); err != nil {
		t.Fatalf("project %s: %v", command.EventID, err)
	}
}

func openMemoryTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func resetMemoryTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	migrationPath := filepath.Join(repoRoot(t), "migrations", "postgres", "memory", "000001_memory_core.sql")
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `
TRUNCATE
	memory_projection_checkpoints,
	memory_graph_edges,
	memory_event_source_refs,
	memory_profile_aggregates,
	memory_membership_projection,
	memory_structured_events
`); err != nil {
		t.Fatalf("truncate memory tables: %v", err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("repository root not found from %s", dir)
		}
		dir = parent
	}
}
