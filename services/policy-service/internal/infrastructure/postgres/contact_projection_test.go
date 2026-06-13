package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

func TestProjectionRepositoryProjectsContactEdgesIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	repository := NewProjectionRepository(pool)

	accepted := contactAcceptedCommand("contact-accepted-1", 1)
	result, err := repository.ProjectContactEvent(ctx, accepted)
	if err != nil {
		t.Fatalf("project accepted contact: %v", err)
	}
	if result.ProjectedEdges != 2 {
		t.Fatalf("expected two active edges, got %+v", result)
	}
	assertProjectedEdge(t, ctx, pool, "alice", "bob", types.ContactEdgeStatusActive, 1)
	assertProjectedEdge(t, ctx, pool, "bob", "alice", types.ContactEdgeStatusActive, 1)

	blocked := contactEdgeCommand(types.ContactEventEdgeBlocked, "contact-blocked-1", "alice", "bob", types.ContactEdgeStatusBlocked, 2)
	result, err = repository.ProjectContactEvent(ctx, blocked)
	if err != nil {
		t.Fatalf("project blocked contact: %v", err)
	}
	if result.ProjectedEdges != 1 {
		t.Fatalf("expected one blocked edge, got %+v", result)
	}
	assertProjectedEdge(t, ctx, pool, "alice", "bob", types.ContactEdgeStatusBlocked, 2)
	assertProjectedEdge(t, ctx, pool, "bob", "alice", types.ContactEdgeStatusActive, 1)

	staleAccepted := contactAcceptedCommand("contact-accepted-stale", 1)
	result, err = repository.ProjectContactEvent(ctx, staleAccepted)
	if err != nil {
		t.Fatalf("project stale accepted contact: %v", err)
	}
	if result.ProjectedEdges != 0 {
		t.Fatalf("expected stale accepted contact to be a no-op, got %+v", result)
	}
	assertProjectedEdge(t, ctx, pool, "alice", "bob", types.ContactEdgeStatusBlocked, 2)
	assertProjectedEdge(t, ctx, pool, "bob", "alice", types.ContactEdgeStatusActive, 1)

	unblocked := contactEdgeCommand(types.ContactEventEdgeUnblocked, "contact-unblocked-1", "alice", "bob", types.ContactEdgeStatusActive, 3)
	if _, err := repository.ProjectContactEvent(ctx, unblocked); err != nil {
		t.Fatalf("project unblocked contact: %v", err)
	}
	assertProjectedEdge(t, ctx, pool, "alice", "bob", types.ContactEdgeStatusActive, 3)

	deleted := contactEdgeCommand(types.ContactEventEdgeDeleted, "contact-deleted-1", "bob", "alice", types.ContactEdgeStatusDeleted, 4)
	if _, err := repository.ProjectContactEvent(ctx, deleted); err != nil {
		t.Fatalf("project deleted contact: %v", err)
	}
	assertProjectedEdge(t, ctx, pool, "bob", "alice", types.ContactEdgeStatusDeleted, 4)
	assertPolicyCheckpoint(t, ctx, pool, "policy-contact-test", types.ContactEventEdgeDeleted, 16)
}

func TestProjectionRepositoryCheckpointsRequestEventsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	repository := NewProjectionRepository(pool)

	result, err := repository.ProjectContactEvent(ctx, types.ProjectContactEventCommand{
		TenantID:       "tenant-policy",
		EventID:        "contact-request-created-1",
		EventType:      types.ContactEventRequestCreated,
		SenderUserID:   "alice",
		ReceiverUserID: "bob",
		Status:         "PENDING",
		ConsumerGroup:  "policy-contact-test",
		Topic:          types.ContactEventRequestCreated,
		PartitionID:    3,
		OffsetValue:    42,
	})
	if err != nil {
		t.Fatalf("project request event: %v", err)
	}
	if result.ProjectedEdges != 0 {
		t.Fatalf("request event should not update edges: %+v", result)
	}
	assertPolicyCheckpoint(t, ctx, pool, "policy-contact-test", types.ContactEventRequestCreated, 42)
}

func contactAcceptedCommand(eventID string, version int64) types.ProjectContactEventCommand {
	return types.ProjectContactEventCommand{
		TenantID:       "tenant-policy",
		EventID:        eventID,
		EventType:      types.ContactEventRequestAccepted,
		SenderUserID:   "alice",
		ReceiverUserID: "bob",
		Status:         "ACCEPTED",
		EdgeVersion:    version,
		ConsumerGroup:  "policy-contact-test",
		Topic:          types.ContactEventRequestAccepted,
		PartitionID:    3,
		OffsetValue:    10 + version,
	}
}

func contactEdgeCommand(eventType string, eventID string, owner string, contact string, status string, version int64) types.ProjectContactEventCommand {
	return types.ProjectContactEventCommand{
		TenantID:      "tenant-policy",
		EventID:       eventID,
		EventType:     eventType,
		OwnerUserID:   types.UserID(owner),
		ContactUserID: types.UserID(contact),
		Status:        status,
		EdgeVersion:   version,
		ConsumerGroup: "policy-contact-test",
		Topic:         eventType,
		PartitionID:   3,
		OffsetValue:   12 + version,
	}
}

func assertProjectedEdge(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner string, contact string, status string, version int64) {
	t.Helper()
	var gotStatus string
	var gotVersion int64
	err := pool.QueryRow(ctx, `
SELECT status, edge_version
FROM policy_contact_edges_projection
WHERE tenant_id = 'tenant-policy'
  AND owner_user_id = $1
  AND contact_user_id = $2
`, owner, contact).Scan(&gotStatus, &gotVersion)
	if err != nil {
		t.Fatalf("query projected edge %s->%s: %v", owner, contact, err)
	}
	if gotStatus != status || gotVersion != version {
		t.Fatalf("unexpected edge %s->%s status/version: got %s/%d want %s/%d", owner, contact, gotStatus, gotVersion, status, version)
	}
}

func assertPolicyCheckpoint(t *testing.T, ctx context.Context, pool *pgxpool.Pool, group string, topic string, offset int64) {
	t.Helper()
	var got int64
	err := pool.QueryRow(ctx, `
SELECT offset_value
FROM policy_kafka_checkpoints
WHERE consumer_group = $1
  AND topic = $2
  AND partition_id = 3
`, group, topic).Scan(&got)
	if err != nil {
		t.Fatalf("query policy checkpoint: %v", err)
	}
	if got != offset {
		t.Fatalf("unexpected checkpoint offset: got %d want %d", got, offset)
	}
}
