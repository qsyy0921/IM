package postgres

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

func TestRepositoryCreateMemberChangeRejoinClearsLeaveSeqIntegration(t *testing.T) {
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

	resetConversationTables(t, ctx, pool)
	_, err = pool.Exec(ctx, `
INSERT INTO conversations (
    tenant_id, conversation_id, conversation_type, status, conversation_mode,
    fanout_mode, fanout_policy_version, member_version, permission_version, current_seq_shard
) VALUES ('tenant-rejoin', 'conv-rejoin', 'GROUP', 'ACTIVE', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 3, 5, 7, 'local');
INSERT INTO conversation_seq (tenant_id, conversation_id, current_seq)
VALUES ('tenant-rejoin', 'conv-rejoin', 10);
INSERT INTO conversation_members (
    tenant_id, conversation_id, user_id, role, status, join_seq, leave_seq,
    member_version, permission_version
) VALUES
    ('tenant-rejoin', 'conv-rejoin', 'owner-1', 'OWNER', 'ACTIVE', 1, NULL, 5, 7),
    ('tenant-rejoin', 'conv-rejoin', 'target-1', 'MEMBER', 'LEFT', 2, 8, 4, 6);
`)
	if err != nil {
		t.Fatalf("seed rejoin conversation: %v", err)
	}

	repository := NewRepository(
		pool,
		WithIDGenerators(
			func() (types.ChangeID, error) { return "change-rejoin-1", nil },
			func() (types.EventID, error) { return "event-rejoin-1", nil },
		),
	)
	result, err := repository.CreateMemberChange(ctx, types.CreateMemberChangeCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-rejoin",
			UserID:   "owner-1",
		},
		ConversationID:        "conv-rejoin",
		TargetUserID:          "target-1",
		ChangeType:            types.MemberChangeTypeJoin,
		TargetRole:            types.MemberRoleMember,
		ExpectedMemberVersion: 5,
		IdempotencyKey:        "idem-rejoin-1",
		ConflictPolicy:        types.MemberChangeConflictPolicyReject,
		Reason:                "rejoin target",
	})
	if err != nil {
		t.Fatalf("create rejoin member change: %v", err)
	}
	if result.BoundarySeq != 11 {
		t.Fatalf("boundary seq = %d, want 11", result.BoundarySeq)
	}

	var status types.MemberStatus
	var joinSeq sql.NullInt64
	var leaveSeq sql.NullInt64
	if err := pool.QueryRow(ctx, `
SELECT status, join_seq, leave_seq
FROM conversation_members
WHERE tenant_id = 'tenant-rejoin'
  AND conversation_id = 'conv-rejoin'
  AND user_id = 'target-1'
`).Scan(&status, &joinSeq, &leaveSeq); err != nil {
		t.Fatalf("query rejoined member: %v", err)
	}
	if status != types.MemberStatusActive || !joinSeq.Valid || joinSeq.Int64 != 11 || leaveSeq.Valid {
		t.Fatalf("unexpected rejoined member window: status=%s join=%v leave=%v", status, joinSeq, leaveSeq)
	}

	rows, err := repository.AuditMemberWindows(ctx, MemberWindowAuditOptions{
		TenantID:       "tenant-rejoin",
		ConversationID: "conv-rejoin",
		UserID:         "target-1",
		IssueClass:     "ACTIVE_WITH_LEAVE_SEQ",
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("audit member windows after rejoin: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rejoined member should not have active-with-leave issue: %+v", rows)
	}
}
