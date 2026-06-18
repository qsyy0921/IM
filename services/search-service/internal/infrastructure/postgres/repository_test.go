package postgres

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/search-service/internal/types"
)

func TestRepositoryProjectAndSearchMessagesIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openSearchTestPool(t)
	resetSearchTables(t, ctx, pool)
	repository := NewRepository(pool)

	project(t, ctx, repository, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-1",
		EventID:           "event-member-join",
		EventType:         types.TimelineEventConversationMemberJoined,
		ConversationID:    "conv-1",
		ConversationSeq:   1,
		ConsumerGroup:     "search-test",
		Topic:             "conversation.timeline.events",
		PartitionID:       0,
		OffsetValue:       2,
		TargetUserID:      "user-1",
		MemberRole:        "MEMBER",
		MemberStatus:      types.SearchMemberStatusActive,
		MemberVersion:     1,
		PermissionVersion: 1,
	})
	project(t, ctx, repository, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-1",
		EventID:           "event-message-1",
		EventType:         types.TimelineEventMessagePersisted,
		ConversationID:    "conv-1",
		ConversationSeq:   2,
		ConsumerGroup:     "search-test",
		Topic:             "conversation.timeline.events",
		PartitionID:       0,
		OffsetValue:       3,
		MessageID:         "msg-1",
		SenderID:          "user-2",
		MessageType:       "TEXT",
		SearchableText:    "hello searchable world",
		PermissionVersion: 7,
	})

	items, projectionVersion, err := repository.SearchMessages(ctx, types.SearchMessagesCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		Query: "searchable",
		Limit: 10,
	}, 10)
	if err != nil {
		t.Fatalf("search messages: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one hit, got %d: %+v", len(items), items)
	}
	if items[0].MessageID != "msg-1" || items[0].VisibilityVersion != 7 {
		t.Fatalf("unexpected hit: %+v", items[0])
	}
	if len(items[0].HighlightRanges) != 1 {
		t.Fatalf("expected highlight range, got %+v", items[0].HighlightRanges)
	}
	if projectionVersion != 7 {
		t.Fatalf("unexpected projection version %d", projectionVersion)
	}

	strangerItems, _, err := repository.SearchMessages(ctx, types.SearchMessagesCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-stranger",
			DeviceID: "device-1",
		},
		Query: "searchable",
		Limit: 10,
	}, 10)
	if err != nil {
		t.Fatalf("search as stranger: %v", err)
	}
	if len(strangerItems) != 0 {
		t.Fatalf("stranger should not see hits: %+v", strangerItems)
	}

	var checkpoint int64
	if err := pool.QueryRow(ctx, `
SELECT offset_value
FROM search_projection_checkpoints
WHERE consumer_group = 'search-test'
  AND topic = 'conversation.timeline.events'
  AND partition_id = 0
`).Scan(&checkpoint); err != nil {
		t.Fatalf("query checkpoint: %v", err)
	}
	if checkpoint != 3 {
		t.Fatalf("unexpected checkpoint %d", checkpoint)
	}
}

func TestRepositoryProjectionEditLeaveAndTombstoneIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openSearchTestPool(t)
	resetSearchTables(t, ctx, pool)
	repository := NewRepository(pool)

	project(t, ctx, repository, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-1",
		EventID:           "event-member-join",
		EventType:         types.TimelineEventConversationMemberJoined,
		ConversationID:    "conv-1",
		ConversationSeq:   1,
		ConsumerGroup:     "search-test",
		Topic:             "conversation.timeline.events",
		PartitionID:       0,
		OffsetValue:       2,
		TargetUserID:      "user-1",
		MemberRole:        "MEMBER",
		PermissionVersion: 1,
	})
	project(t, ctx, repository, types.ProjectTimelineEventCommand{
		TenantID:        "tenant-1",
		EventID:         "event-message-1",
		EventType:       types.TimelineEventMessagePersisted,
		ConversationID:  "conv-1",
		ConversationSeq: 2,
		ConsumerGroup:   "search-test",
		Topic:           "conversation.timeline.events",
		PartitionID:     0,
		OffsetValue:     3,
		MessageID:       "msg-1",
		SenderID:        "user-2",
		MessageType:     "TEXT",
		SearchableText:  "old project wording",
	})
	project(t, ctx, repository, types.ProjectTimelineEventCommand{
		TenantID:        "tenant-1",
		EventID:         "event-message-1-edit",
		EventType:       types.TimelineEventMessageEdited,
		ConversationID:  "conv-1",
		ConversationSeq: 2,
		ConsumerGroup:   "search-test",
		Topic:           "conversation.timeline.events",
		PartitionID:     0,
		OffsetValue:     4,
		MessageID:       "msg-1",
		SenderID:        "user-2",
		MessageType:     "TEXT",
		SearchableText:  "new project wording",
	})

	assertSearchCount(t, ctx, repository, "user-1", "old", 0)
	assertSearchCount(t, ctx, repository, "user-1", "new", 1)

	project(t, ctx, repository, types.ProjectTimelineEventCommand{
		TenantID:        "tenant-1",
		EventID:         "event-member-left",
		EventType:       types.TimelineEventConversationMemberLeft,
		ConversationID:  "conv-1",
		ConversationSeq: 3,
		ConsumerGroup:   "search-test",
		Topic:           "conversation.timeline.events",
		PartitionID:     0,
		OffsetValue:     5,
		TargetUserID:    "user-1",
		MemberRole:      "MEMBER",
	})
	project(t, ctx, repository, types.ProjectTimelineEventCommand{
		TenantID:        "tenant-1",
		EventID:         "event-message-2",
		EventType:       types.TimelineEventMessagePersisted,
		ConversationID:  "conv-1",
		ConversationSeq: 4,
		ConsumerGroup:   "search-test",
		Topic:           "conversation.timeline.events",
		PartitionID:     0,
		OffsetValue:     6,
		MessageID:       "msg-2",
		SenderID:        "user-2",
		MessageType:     "TEXT",
		SearchableText:  "after leave invisible",
	})

	assertSearchCount(t, ctx, repository, "user-1", "new", 1)
	assertSearchCount(t, ctx, repository, "user-1", "invisible", 0)

	project(t, ctx, repository, types.ProjectTimelineEventCommand{
		TenantID:        "tenant-1",
		EventID:         "event-message-1-revoke",
		EventType:       types.TimelineEventMessageRevoked,
		ConversationID:  "conv-1",
		ConversationSeq: 5,
		ConsumerGroup:   "search-test",
		Topic:           "conversation.timeline.events",
		PartitionID:     0,
		OffsetValue:     7,
		MessageID:       "msg-1",
	})

	assertSearchCount(t, ctx, repository, "user-1", "new", 0)
}

func TestRepositoryProjectsOwnerTransferAndBoundaryCancelIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openSearchTestPool(t)
	resetSearchTables(t, ctx, pool)
	repository := NewRepository(pool)

	project(t, ctx, repository, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-1",
		EventID:           "event-owner-join",
		EventType:         types.TimelineEventConversationMemberJoined,
		ConversationID:    "conv-1",
		ConversationSeq:   1,
		ConsumerGroup:     "search-test",
		Topic:             "conversation.timeline.events",
		PartitionID:       0,
		OffsetValue:       2,
		TargetUserID:      "owner-1",
		MemberRole:        types.SearchMemberRoleOwner,
		MemberVersion:     1,
		PermissionVersion: 1,
	})
	project(t, ctx, repository, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-1",
		EventID:           "event-member-join",
		EventType:         types.TimelineEventConversationMemberJoined,
		ConversationID:    "conv-1",
		ConversationSeq:   2,
		ConsumerGroup:     "search-test",
		Topic:             "conversation.timeline.events",
		PartitionID:       0,
		OffsetValue:       3,
		TargetUserID:      "user-2",
		MemberRole:        types.SearchMemberRoleMember,
		MemberVersion:     2,
		PermissionVersion: 2,
	})
	project(t, ctx, repository, types.ProjectTimelineEventCommand{
		TenantID:            "tenant-1",
		EventID:             "event-owner-transfer",
		EventType:           types.TimelineEventConversationMemberOwnerTransferred,
		ConversationID:      "conv-1",
		ConversationSeq:     3,
		ConsumerGroup:       "search-test",
		Topic:               "conversation.timeline.events",
		PartitionID:         0,
		OffsetValue:         4,
		PreviousOwnerUserID: "owner-1",
		PreviousOwnerRole:   types.SearchMemberRoleAdmin,
		NewOwnerUserID:      "user-2",
		NewOwnerRole:        types.SearchMemberRoleOwner,
		MemberVersion:       3,
		PermissionVersion:   3,
	})

	assertMembershipRole(t, ctx, pool, "owner-1", types.SearchMemberRoleAdmin)
	assertMembershipRole(t, ctx, pool, "user-2", types.SearchMemberRoleOwner)

	project(t, ctx, repository, types.ProjectTimelineEventCommand{
		TenantID:          "tenant-1",
		EventID:           "event-boundary-cancel",
		EventType:         types.TimelineEventConversationMemberBoundaryCancelled,
		ConversationID:    "conv-1",
		ConversationSeq:   4,
		ConsumerGroup:     "search-test",
		Topic:             "conversation.timeline.events",
		PartitionID:       0,
		OffsetValue:       5,
		TargetUserID:      "user-3",
		MemberRole:        types.SearchMemberRoleMember,
		MemberVersion:     4,
		PermissionVersion: 4,
	})

	var memberCount int
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM search_membership_projection
WHERE tenant_id = 'tenant-1'
  AND conversation_id = 'conv-1'
`).Scan(&memberCount); err != nil {
		t.Fatalf("query member count: %v", err)
	}
	if memberCount != 2 {
		t.Fatalf("boundary cancel should not create membership rows, got %d", memberCount)
	}
	var checkpoint int64
	if err := pool.QueryRow(ctx, `
SELECT offset_value
FROM search_projection_checkpoints
WHERE consumer_group = 'search-test'
  AND topic = 'conversation.timeline.events'
  AND partition_id = 0
`).Scan(&checkpoint); err != nil {
		t.Fatalf("query checkpoint: %v", err)
	}
	if checkpoint != 5 {
		t.Fatalf("expected boundary cancel to advance checkpoint, got %d", checkpoint)
	}
}

func project(
	t *testing.T,
	ctx context.Context,
	repository *Repository,
	command types.ProjectTimelineEventCommand,
) {
	t.Helper()
	if _, err := repository.ProjectTimelineEvent(ctx, command); err != nil {
		t.Fatalf("project %s: %v", command.EventID, err)
	}
}

func assertSearchCount(
	t *testing.T,
	ctx context.Context,
	repository *Repository,
	userID types.UserID,
	query string,
	want int,
) {
	t.Helper()
	items, _, err := repository.SearchMessages(ctx, types.SearchMessagesCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   userID,
			DeviceID: "device-1",
		},
		Query: query,
		Limit: 10,
	}, 10)
	if err != nil {
		t.Fatalf("search %q: %v", query, err)
	}
	if len(items) != want {
		t.Fatalf("search %q got %d hits, want %d: %+v", query, len(items), want, items)
	}
}

func assertMembershipRole(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID types.UserID, want string) {
	t.Helper()
	var role string
	if err := pool.QueryRow(ctx, `
SELECT role
FROM search_membership_projection
WHERE tenant_id = 'tenant-1'
  AND conversation_id = 'conv-1'
  AND user_id = $1
`, userID).Scan(&role); err != nil {
		t.Fatalf("query membership role for %s: %v", userID, err)
	}
	if role != want {
		t.Fatalf("membership role for %s got %q, want %q", userID, role, want)
	}
}

func openSearchTestPool(t *testing.T) *pgxpool.Pool {
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

func resetSearchTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	migrationPath := filepath.Join(repoRoot(t), "migrations", "postgres", "search", "000001_search_core.sql")
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `
TRUNCATE
	search_projection_checkpoints,
	search_membership_projection,
	search_message_documents
`); err != nil {
		t.Fatalf("truncate search tables: %v", err)
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
