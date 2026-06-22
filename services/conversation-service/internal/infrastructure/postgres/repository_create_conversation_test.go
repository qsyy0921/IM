package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

func TestRepositoryCreateDirectConversationWritesPublishableMemberBoundariesIntegration(t *testing.T) {
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	defer pool.Close()

	resetMemberChangeTables(t, ctx, pool)
	eventIDs := []types.EventID{"event-create-direct-1", "event-create-direct-2"}
	eventIndex := 0
	repository := NewRepository(pool, WithIDGenerators(
		func() (types.ChangeID, error) { return "unused-create-change", nil },
		func() (types.EventID, error) {
			eventID := eventIDs[eventIndex]
			eventIndex++
			return eventID, nil
		},
	))

	result, err := repository.CreateConversation(ctx, types.CreateConversationCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-create",
			UserID:    "user-a",
			RequestID: "request-create",
			TraceID:   "trace-create",
		},
		ConversationID:   "direct-create",
		ConversationType: types.ConversationTypeDirect,
		DirectPeerUserID: "user-b",
		IdempotencyKey:   "direct-create-idem",
	})
	if err != nil {
		t.Fatalf("create direct conversation: %v", err)
	}
	if result.BoundarySeq != 2 || result.MemberVersion != 2 || result.PermissionVersion != 2 {
		t.Fatalf("unexpected create result: %+v", result)
	}

	rows, err := pool.Query(ctx, `
SELECT
    aggregate_version,
    event_id,
    event_type,
    status,
    payload_json->>'change_id',
    payload_json->>'target_user_id',
    (payload_json->>'boundary_seq')::bigint
FROM message_outbox
WHERE tenant_id = 'tenant-create'
  AND conversation_id = 'direct-create'
ORDER BY aggregate_version
`)
	if err != nil {
		t.Fatalf("query outbox: %v", err)
	}
	defer rows.Close()

	type outboxRow struct {
		aggregateVersion int64
		eventID          string
		eventType        string
		status           string
		changeID         string
		targetUserID     string
		boundarySeq      int64
	}
	var got []outboxRow
	for rows.Next() {
		var row outboxRow
		if err := rows.Scan(
			&row.aggregateVersion,
			&row.eventID,
			&row.eventType,
			&row.status,
			&row.changeID,
			&row.targetUserID,
			&row.boundarySeq,
		); err != nil {
			t.Fatalf("scan outbox: %v", err)
		}
		got = append(got, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate outbox: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected two direct member boundary outbox rows, got %+v", got)
	}
	for index, row := range got {
		wantSeq := int64(index + 1)
		if row.aggregateVersion != wantSeq ||
			row.boundarySeq != wantSeq ||
			row.eventID != string(eventIDs[index]) ||
			row.changeID != string(eventIDs[index]) ||
			row.eventType != string(types.TimelineEventConversationMemberJoined) ||
			row.status != "PENDING" {
			t.Fatalf("unexpected outbox row %d: %+v", index, row)
		}
	}
	if got[0].targetUserID != "user-a" || got[1].targetUserID != "user-b" {
		t.Fatalf("unexpected direct member targets: %+v", got)
	}
}
