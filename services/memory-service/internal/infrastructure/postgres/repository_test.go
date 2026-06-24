package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
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
		ProjectMemory:     true,
		MemoryEventType:   types.MemoryEventTypeDecision,
		MemoryReviewState: types.MemoryReviewUnreviewed,
		MemoryConfidence:  0.55,
		FactText:          "ship the retrieval gateway after memory projection",
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

func TestRepositorySubmitAndReviewMemoryCandidateIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openMemoryTestPool(t)
	resetMemoryTables(t, ctx, pool)
	repository := NewRepository(pool)

	projectMemberJoined(t, ctx, repository, "tenant-1", "conv-candidate", "reviewer-1", 1)
	command := memoryCandidateCommand("tenant-1", "reviewer-1", "conv-candidate", "candidate-decision-1", "msg-candidate-1", 2)
	item, err := repository.SubmitMemoryCandidate(ctx, command)
	if err != nil {
		t.Fatalf("submit memory candidate: %v", err)
	}
	if item.MemoryEventID != command.CandidateID || item.Status != types.MemoryStatusPending || item.ReviewState != types.MemoryReviewNeedsReview {
		t.Fatalf("unexpected submitted item: %+v", item)
	}
	if len(item.SourceRefs) != 1 || item.SourceRefs[0].SourceID != "msg-candidate-1" {
		t.Fatalf("expected source ref to be preserved: %+v", item.SourceRefs)
	}

	approved, err := repository.ReviewMemoryCandidate(ctx, types.ReviewMemoryCandidateCommand{
		AuthContext:   types.AuthContext{TenantID: "tenant-1", UserID: "reviewer-1"},
		MemoryEventID: "candidate-decision-1",
		Decision:      types.MemoryReviewDecisionApprove,
	})
	if err != nil {
		t.Fatalf("approve memory candidate: %v", err)
	}
	if approved.Status != types.MemoryStatusActive || approved.ReviewState != types.MemoryReviewApproved {
		t.Fatalf("unexpected approved item: %+v", approved)
	}

	items, _, err := repository.QueryMemoryEvents(ctx, types.QueryMemoryEventsCommand{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "reviewer-1"},
		Query:       "evidence pack",
		Statuses:    []string{types.MemoryStatusActive},
		Limit:       10,
	}, 10)
	if err != nil {
		t.Fatalf("query approved candidate: %v", err)
	}
	if len(items) != 1 || items[0].MemoryEventID != "candidate-decision-1" {
		t.Fatalf("expected approved candidate to become active: %+v", items)
	}
}

func TestRepositorySubmitMemoryCandidateRequiresVisibleSourceIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openMemoryTestPool(t)
	resetMemoryTables(t, ctx, pool)
	repository := NewRepository(pool)

	projectMemberJoined(t, ctx, repository, "tenant-1", "conv-candidate", "member-1", 1)
	command := memoryCandidateCommand("tenant-1", "stranger-1", "conv-candidate", "candidate-hidden-1", "msg-hidden-1", 2)
	if _, err := repository.SubmitMemoryCandidate(ctx, command); err != types.ErrPermissionDenied {
		t.Fatalf("submit hidden candidate error = %v, want permission denied", err)
	}
}

func TestRepositoryReviewMemoryCandidateCanRejectIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openMemoryTestPool(t)
	resetMemoryTables(t, ctx, pool)
	repository := NewRepository(pool)

	projectMemberJoined(t, ctx, repository, "tenant-1", "conv-candidate", "reviewer-1", 1)
	command := memoryCandidateCommand("tenant-1", "reviewer-1", "conv-candidate", "candidate-reject-1", "msg-reject-1", 2)
	if _, err := repository.SubmitMemoryCandidate(ctx, command); err != nil {
		t.Fatalf("submit memory candidate: %v", err)
	}

	rejected, err := repository.ReviewMemoryCandidate(ctx, types.ReviewMemoryCandidateCommand{
		AuthContext:   types.AuthContext{TenantID: "tenant-1", UserID: "reviewer-1"},
		MemoryEventID: "candidate-reject-1",
		Decision:      types.MemoryReviewDecisionReject,
	})
	if err != nil {
		t.Fatalf("reject memory candidate: %v", err)
	}
	if rejected.Status != types.MemoryStatusRejected || rejected.ReviewState != types.MemoryReviewRejected {
		t.Fatalf("unexpected rejected item: %+v", rejected)
	}

	items, _, err := repository.QueryMemoryEvents(ctx, types.QueryMemoryEventsCommand{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "reviewer-1"},
		Query:       "evidence pack",
		Statuses:    []string{types.MemoryStatusRejected},
		Limit:       10,
	}, 10)
	if err != nil {
		t.Fatalf("query rejected candidate: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("rejected candidate should stay hidden from query: %+v", items)
	}
}

func TestRepositoryQueryMemoryEventsAtConversationSeqIntegration(t *testing.T) {
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
	seedRuntimeMemoryWindow(t, ctx, pool, "tenant-1", "conv-1")

	items, _, err := repository.QueryMemoryEvents(ctx, types.QueryMemoryEventsCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID:    "conv-1",
		Query:             "runtime memory",
		Statuses:          []string{types.MemoryStatusActive},
		AtConversationSeq: 15,
		Limit:             10,
	}, 10)
	if err != nil {
		t.Fatalf("query memory events at seq 15: %v", err)
	}
	byID := memoryEventsByID(items)
	if len(items) != 2 {
		t.Fatalf("expected current and replacement active events, got %d: %+v", len(items), items)
	}
	if _, ok := byID["mem-current"]; !ok {
		t.Fatalf("current memory should be visible at seq 15: %+v", items)
	}
	replacement, ok := byID["mem-replacement"]
	if !ok {
		t.Fatalf("replacement memory should be visible at seq 15: %+v", items)
	}
	if len(replacement.SourceRefs) != 1 || replacement.SourceRefs[0].ConversationSeq != 13 {
		t.Fatalf("replacement should keep source ref: %+v", replacement.SourceRefs)
	}
	if len(replacement.SupersedesEventIDs) != 1 || replacement.SupersedesEventIDs[0] != "mem-superseded" {
		t.Fatalf("replacement should preserve supersession link: %+v", replacement.SupersedesEventIDs)
	}
	if _, ok := byID["mem-expired"]; ok {
		t.Fatalf("expired memory should be hidden at seq 15: %+v", items)
	}
	if _, ok := byID["mem-superseded"]; ok {
		t.Fatalf("superseded memory should be hidden from active query: %+v", items)
	}

	items, _, err = repository.QueryMemoryEvents(ctx, types.QueryMemoryEventsCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID:    "conv-1",
		Query:             "runtime memory",
		Statuses:          []string{types.MemoryStatusActive},
		AtConversationSeq: 25,
		Limit:             10,
	}, 10)
	if err != nil {
		t.Fatalf("query memory events at seq 25: %v", err)
	}
	byID = memoryEventsByID(items)
	if len(items) != 1 {
		t.Fatalf("expected only replacement active event at seq 25, got %d: %+v", len(items), items)
	}
	if _, ok := byID["mem-replacement"]; !ok {
		t.Fatalf("replacement memory should remain visible at seq 25: %+v", items)
	}
	if _, ok := byID["mem-current"]; ok {
		t.Fatalf("current memory should expire after valid_to_seq: %+v", items)
	}
}

func TestRepositoryListProfileAggregatesRequiresActiveSupportingMemoryIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openMemoryTestPool(t)
	resetMemoryTables(t, ctx, pool)
	repository := NewRepository(pool)
	seedRuntimeMemoryWindow(t, ctx, pool, "tenant-1", "conv-1")
	if _, err := pool.Exec(ctx, `
INSERT INTO memory_structured_events (
	tenant_id,
	memory_event_id,
	scope_type,
	scope_id,
	conversation_id,
	topic,
	event_type,
	status,
	review_state,
	fact_text,
	actor_user_ids,
	audience_user_ids,
	valid_from_seq,
	valid_to_seq,
	supersedes_event_ids,
	contradicts_event_ids,
	confidence,
	visibility_version,
	extraction_version,
	source_projection_version
) VALUES
($1, 'mem-deleted-support', 'CONVERSATION', $2, $2, 'runtime-memory', 'PROFILE_SIGNAL', 'DELETED', 'APPROVED', 'deleted support should not keep an active profile visible', '["user-1"]'::jsonb, '[]'::jsonb, 14, NULL, '[]'::jsonb, '[]'::jsonb, 0.9000, 1, 'test-v1', 30)
`, "tenant-1", "conv-1"); err != nil {
		t.Fatalf("seed deleted support: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO memory_profile_aggregates (
	tenant_id,
	profile_id,
	subject_user_id,
	aggregate_type,
	aggregate_key,
	status,
	review_state,
	summary_text,
	supporting_memory_event_ids,
	confidence,
	updated_by_memory_event_id
) VALUES
($1, 'profile-active', 'user-1', 'SKILL', 'phoenix-launch', 'ACTIVE', 'APPROVED', 'reviewed multi-source profile with active supporting evidence', '["mem-current", "mem-replacement"]'::jsonb, 0.9100, 'mem-replacement'),
($1, 'profile-stale-support', 'user-1', 'SKILL', 'deleted-support', 'ACTIVE', 'APPROVED', 'profile with deleted support must not be returned as active', '["mem-deleted-support"]'::jsonb, 0.9200, 'mem-deleted-support')
`, "tenant-1"); err != nil {
		t.Fatalf("seed profiles: %v", err)
	}

	items, err := repository.ListProfileAggregates(ctx, types.ListProfileAggregatesCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		SubjectUserID: "user-1",
		AggregateType: "SKILL",
		Statuses:      []string{types.MemoryStatusActive},
		Limit:         10,
	}, 10)
	if err != nil {
		t.Fatalf("list profile aggregates: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected only active profile with active support, got %d: %+v", len(items), items)
	}
	if items[0].ProfileID != "profile-active" {
		t.Fatalf("unexpected profile returned: %+v", items[0])
	}
	if len(items[0].SupportingMemoryEventIDs) != 2 {
		t.Fatalf("expected profile supporting memory ids to be preserved: %+v", items[0].SupportingMemoryEventIDs)
	}
}

func TestRepositoryRecomputeProfileAggregateIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openMemoryTestPool(t)
	resetMemoryTables(t, ctx, pool)
	repository := NewRepository(pool)
	seedProfileSignalMemories(t, ctx, pool, "tenant-1", "conv-1", "user-1")

	item, supportCount, active, err := repository.RecomputeProfileAggregate(ctx, types.RecomputeProfileAggregateCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		SubjectUserID:   "user-1",
		AggregateType:   types.ProfileAggregateTypeSkill,
		AggregateKey:    "phoenix-launch",
		MinSupportCount: 2,
	})
	if err != nil {
		t.Fatalf("recompute profile aggregate: %v", err)
	}
	if !active || supportCount != 2 {
		t.Fatalf("expected active profile with two supports, active=%v support_count=%d item=%+v", active, supportCount, item)
	}
	if item.Status != types.MemoryStatusActive || item.ReviewState != types.MemoryReviewApproved {
		t.Fatalf("unexpected recomputed profile state: %+v", item)
	}
	if item.AggregateType != types.ProfileAggregateTypeSkill || item.AggregateKey != "phoenix-launch" {
		t.Fatalf("unexpected aggregate identity: %+v", item)
	}
	if len(item.SupportingMemoryEventIDs) != 2 {
		t.Fatalf("expected two supporting memory ids: %+v", item.SupportingMemoryEventIDs)
	}

	items, err := repository.ListProfileAggregates(ctx, types.ListProfileAggregatesCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		SubjectUserID: "user-1",
		AggregateType: types.ProfileAggregateTypeSkill,
		Statuses:      []string{types.MemoryStatusActive},
		Limit:         10,
	}, 10)
	if err != nil {
		t.Fatalf("list recomputed profile: %v", err)
	}
	if len(items) != 1 || items[0].ProfileID != item.ProfileID {
		t.Fatalf("expected recomputed profile to be visible: %+v", items)
	}

	if _, err := pool.Exec(ctx, `
UPDATE memory_structured_events
SET status = 'DELETED', review_state = 'REJECTED', updated_at = now()
WHERE tenant_id = $1
  AND memory_event_id = 'profile-signal-2'
`, "tenant-1"); err != nil {
		t.Fatalf("delete profile support: %v", err)
	}
	archived, supportCount, active, err := repository.RecomputeProfileAggregate(ctx, types.RecomputeProfileAggregateCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		SubjectUserID:   "user-1",
		AggregateType:   types.ProfileAggregateTypeSkill,
		AggregateKey:    "phoenix-launch",
		MinSupportCount: 2,
	})
	if err != nil {
		t.Fatalf("recompute after support delete: %v", err)
	}
	if active || supportCount != 1 {
		t.Fatalf("expected inactive profile after support delete, active=%v support_count=%d item=%+v", active, supportCount, archived)
	}
	if archived.ProfileID != item.ProfileID || archived.Status != types.MemoryStatusArchived {
		t.Fatalf("expected existing profile to be archived: %+v", archived)
	}

	items, err = repository.ListProfileAggregates(ctx, types.ListProfileAggregatesCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		SubjectUserID: "user-1",
		AggregateType: types.ProfileAggregateTypeSkill,
		Statuses:      []string{types.MemoryStatusActive},
		Limit:         10,
	}, 10)
	if err != nil {
		t.Fatalf("list archived profile: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("archived profile should not be returned as active: %+v", items)
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
		ProjectMemory:     true,
		MemoryEventType:   types.MemoryEventTypeStatus,
		MemoryReviewState: types.MemoryReviewUnreviewed,
		MemoryConfidence:  0.55,
		FactText:          "status: this memory should be revoked",
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

func seedRuntimeMemoryWindow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID string, conversationID string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
INSERT INTO memory_structured_events (
	tenant_id,
	memory_event_id,
	scope_type,
	scope_id,
	conversation_id,
	topic,
	event_type,
	status,
	review_state,
	fact_text,
	actor_user_ids,
	audience_user_ids,
	valid_from_seq,
	valid_to_seq,
	supersedes_event_ids,
	contradicts_event_ids,
	confidence,
	visibility_version,
	extraction_version,
	source_projection_version
) VALUES
($1, 'mem-expired', 'CONVERSATION', $2, $2, 'runtime-memory', 'DECISION', 'ACTIVE', 'APPROVED', 'expired runtime memory decision', '["user-2"]'::jsonb, '[]'::jsonb, 2, 5, '[]'::jsonb, '[]'::jsonb, 0.9000, 1, 'test-v1', 5),
($1, 'mem-current', 'CONVERSATION', $2, $2, 'runtime-memory', 'DECISION', 'ACTIVE', 'APPROVED', 'current runtime memory decision', '["user-2"]'::jsonb, '[]'::jsonb, 10, 20, '[]'::jsonb, '[]'::jsonb, 0.9100, 1, 'test-v1', 20),
($1, 'mem-superseded', 'CONVERSATION', $2, $2, 'runtime-memory', 'DECISION', 'SUPERSEDED', 'APPROVED', 'old runtime memory decision', '["user-2"]'::jsonb, '[]'::jsonb, 6, 12, '[]'::jsonb, '[]'::jsonb, 0.8000, 1, 'test-v1', 12),
($1, 'mem-replacement', 'CONVERSATION', $2, $2, 'runtime-memory', 'DECISION', 'ACTIVE', 'APPROVED', 'replacement runtime memory decision', '["user-3"]'::jsonb, '[]'::jsonb, 13, NULL, '["mem-superseded"]'::jsonb, '[]'::jsonb, 0.9500, 1, 'test-v1', 30)
`, tenantID, conversationID); err != nil {
		t.Fatalf("seed runtime memory events: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO memory_event_source_refs (
	tenant_id,
	memory_event_id,
	source_ref_id,
	source_type,
	source_id,
	source_event_id,
	conversation_id,
	conversation_seq,
	occurred_at
) VALUES
($1, 'mem-expired', 'source-expired', 'MESSAGE', 'msg-expired', 'event-expired', $2, 2, now()),
($1, 'mem-current', 'source-current', 'MESSAGE', 'msg-current', 'event-current', $2, 10, now()),
($1, 'mem-superseded', 'source-superseded', 'MESSAGE', 'msg-superseded', 'event-superseded', $2, 6, now()),
($1, 'mem-replacement', 'source-replacement', 'MESSAGE', 'msg-replacement', 'event-replacement', $2, 13, now())
`, tenantID, conversationID); err != nil {
		t.Fatalf("seed runtime memory source refs: %v", err)
	}
}

func seedProfileSignalMemories(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID string, conversationID string, userID string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
INSERT INTO memory_membership_projection (
	tenant_id,
	conversation_id,
	user_id,
	role,
	status,
	join_seq,
	leave_seq,
	member_version,
	permission_version,
	updated_by_event_id
) VALUES ($1, $2, $3, 'MEMBER', 'ACTIVE', 1, NULL, 1, 1, 'member-seed')
`, tenantID, conversationID, userID); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO memory_structured_events (
	tenant_id,
	memory_event_id,
	scope_type,
	scope_id,
	conversation_id,
	topic,
	event_type,
	status,
	review_state,
	fact_text,
	actor_user_ids,
	audience_user_ids,
	valid_from_seq,
	valid_to_seq,
	supersedes_event_ids,
	contradicts_event_ids,
	confidence,
	visibility_version,
	extraction_version,
	source_projection_version
) VALUES
($1, 'profile-signal-1', 'CONVERSATION', $2, $2, 'phoenix-launch', 'PROFILE_SIGNAL', 'ACTIVE', 'APPROVED', 'user coordinates phoenix launch rollout plans', jsonb_build_array($3::text), '[]'::jsonb, 2, NULL, '[]'::jsonb, '[]'::jsonb, 0.8700, 1, 'profile-test-v1', 2),
($1, 'profile-signal-2', 'CONVERSATION', $2, $2, 'phoenix-launch', 'PROFILE_SIGNAL', 'ACTIVE', 'APPROVED', 'user resolves phoenix launch blockers across groups', jsonb_build_array($3::text), '[]'::jsonb, 3, NULL, '[]'::jsonb, '[]'::jsonb, 0.9300, 1, 'profile-test-v1', 3)
`, tenantID, conversationID, userID); err != nil {
		t.Fatalf("seed profile signals: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO memory_event_source_refs (
	tenant_id,
	memory_event_id,
	source_ref_id,
	source_type,
	source_id,
	source_event_id,
	conversation_id,
	conversation_seq,
	occurred_at
) VALUES
($1, 'profile-signal-1', 'source-profile-1', 'MESSAGE', 'msg-profile-1', 'event-profile-1', $2, 2, now()),
($1, 'profile-signal-2', 'source-profile-2', 'MESSAGE', 'msg-profile-2', 'event-profile-2', $2, 3, now())
`, tenantID, conversationID); err != nil {
		t.Fatalf("seed profile signal refs: %v", err)
	}
}

func memoryEventsByID(items []types.StructuredMemoryEvent) map[string]types.StructuredMemoryEvent {
	byID := make(map[string]types.StructuredMemoryEvent, len(items))
	for _, item := range items {
		byID[item.MemoryEventID] = item
	}
	return byID
}

func memoryCandidateCommand(tenantID string, reviewerID string, conversationID string, candidateID string, messageID string, conversationSeq int64) types.SubmitMemoryCandidateCommand {
	factText := "decision: keep evidence pack source-backed"
	return types.SubmitMemoryCandidateCommand{
		AuthContext: types.AuthContext{
			TenantID: types.TenantID(tenantID),
			UserID:   types.UserID(reviewerID),
			DeviceID: "device-reviewer",
		},
		CandidateID:    candidateID,
		Scope:          types.MemoryScopeConversation,
		ScopeID:        conversationID,
		ConversationID: types.ConversationID(conversationID),
		Topic:          "architecture",
		EventType:      types.MemoryEventTypeDecision,
		FactText:       factText,
		FactSHA256:     normalizedTestFactSHA256(factText),
		ActorUserIDs:   []string{"speaker-1"},
		SourceRefs: []types.SourceRef{
			{
				SourceType:      types.MemorySourceTypeMessage,
				SourceID:        messageID,
				SourceEventID:   "timeline-" + messageID,
				ConversationID:  types.ConversationID(conversationID),
				ConversationSeq: conversationSeq,
				OccurredAt:      time.Unix(1000+conversationSeq, 0).UTC(),
			},
		},
		ValidFromSeq:      conversationSeq,
		Confidence:        0.79,
		VisibilityVersion: 3,
		ExtractionVersion: "memory-extraction-candidate-v1",
	}
}

func normalizedTestFactSHA256(value string) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func projectMemberJoined(t *testing.T, ctx context.Context, repository *Repository, tenantID string, conversationID string, userID string, seq int64) {
	t.Helper()
	projectMemory(t, ctx, repository, types.ProjectTimelineEventCommand{
		TenantID:          types.TenantID(tenantID),
		EventID:           "event-member-join-" + userID,
		EventType:         types.TimelineEventConversationMemberJoined,
		ConversationID:    types.ConversationID(conversationID),
		ConversationSeq:   seq,
		ConsumerGroup:     "memory-test",
		Topic:             "conversation.timeline.events",
		PartitionID:       0,
		OffsetValue:       seq + 1,
		TargetUserID:      types.UserID(userID),
		MemberRole:        types.MemoryMemberRoleMember,
		MemberStatus:      types.MemoryMemberStatusActive,
		MemberVersion:     seq,
		PermissionVersion: seq,
	})
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
