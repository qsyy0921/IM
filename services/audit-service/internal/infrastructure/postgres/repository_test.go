package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/audit-service/internal/domain"
	"github.com/qsyy0921/IM/services/audit-service/internal/types"
)

func TestRepositoryAppendQueryAndVerifyIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openAuditTestPool(t)
	resetAuditTables(t, ctx, pool)
	repository := NewRepository(pool)

	firstPrepared := prepareAuditRecord(t, "identity-service", "event-1", "idem-1", `{"session_key":"session-1"}`)
	first, err := repository.AppendAuditRecord(ctx, firstPrepared, "aud_1")
	if err != nil {
		t.Fatalf("append first audit record: %v", err)
	}
	if first.PreviousRecordHash != "" || first.RecordHash == "" || first.CanonicalJSONHash == "" {
		t.Fatalf("unexpected first hash fields: %+v", first)
	}

	replay, err := repository.AppendAuditRecord(ctx, firstPrepared, "aud_should_not_win")
	if err != nil {
		t.Fatalf("append replay: %v", err)
	}
	if replay.AuditID != first.AuditID {
		t.Fatalf("replay returned different record: %+v", replay)
	}

	conflictPrepared := prepareAuditRecord(t, "identity-service", "event-1", "idem-1", `{"device_key":"device-2"}`)
	if _, err := repository.AppendAuditRecord(ctx, conflictPrepared, "aud_conflict"); !errors.Is(err, types.ErrAlreadyExists) {
		t.Fatalf("expected conflict, got %v", err)
	}

	secondPrepared := prepareAuditRecord(t, "agent-service", "event-2", "idem-2", `{"proposal_id":"proposal-1"}`)
	second, err := repository.AppendAuditRecord(ctx, secondPrepared, "aud_2")
	if err != nil {
		t.Fatalf("append second audit record: %v", err)
	}
	if second.PreviousRecordHash != first.RecordHash {
		t.Fatalf("expected second record to chain to first hash, got %+v first=%s", second, first.RecordHash)
	}

	query, err := repository.QueryAuditRecords(ctx, types.QueryAuditRecordsCommand{
		AuthContext: validAuth(),
		AuditStream: "security",
	}, 10)
	if err != nil {
		t.Fatalf("query audit records: %v", err)
	}
	if len(query) != 2 || query[0].AuditID != "aud_1" || query[1].AuditID != "aud_2" {
		t.Fatalf("unexpected query result: %+v", query)
	}

	proof, err := repository.VerifyAuditProof(ctx, "tenant-audit-test", "aud_2")
	if err != nil {
		t.Fatalf("verify proof: %v", err)
	}
	if !proof.Valid || proof.FailureReason != "" || proof.PreviousRecordHash != first.RecordHash {
		t.Fatalf("unexpected proof: %+v", proof)
	}
	assertAuditOutboxLowSensitive(t, ctx, pool)
}

func TestRepositoryVerifyAuditProofDetectsTamperIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openAuditTestPool(t)
	resetAuditTables(t, ctx, pool)
	repository := NewRepository(pool)

	prepared := prepareAuditRecord(t, "identity-service", "event-1", "idem-1", `{"session_key":"session-1"}`)
	if _, err := repository.AppendAuditRecord(ctx, prepared, "aud_1"); err != nil {
		t.Fatalf("append audit record: %v", err)
	}
	if _, err := pool.Exec(ctx, `
UPDATE audit_records
SET record_hash = 'tampered'
WHERE tenant_id = 'tenant-audit-test'
  AND audit_id = 'aud_1'
`); err != nil {
		t.Fatalf("tamper record: %v", err)
	}
	proof, err := repository.VerifyAuditProof(ctx, "tenant-audit-test", "aud_1")
	if err != nil {
		t.Fatalf("verify proof: %v", err)
	}
	if proof.Valid || proof.FailureReason != "HASH_MISMATCH" {
		t.Fatalf("expected hash mismatch, got %+v", proof)
	}
}

func TestRepositoryVerifyAuditProofDetectsMissingPredecessorIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openAuditTestPool(t)
	resetAuditTables(t, ctx, pool)
	repository := NewRepository(pool)

	firstPrepared := prepareAuditRecord(t, "identity-service", "event-1", "idem-1", `{"session_key":"session-1"}`)
	first, err := repository.AppendAuditRecord(ctx, firstPrepared, "aud_1")
	if err != nil {
		t.Fatalf("append first audit record: %v", err)
	}
	secondPrepared := prepareAuditRecord(t, "agent-service", "event-2", "idem-2", `{"proposal_id":"proposal-1"}`)
	if _, err := repository.AppendAuditRecord(ctx, secondPrepared, "aud_2"); err != nil {
		t.Fatalf("append second audit record: %v", err)
	}
	if _, err := pool.Exec(ctx, `
DELETE FROM audit_records
WHERE tenant_id = 'tenant-audit-test'
  AND audit_id = 'aud_1'
`); err != nil {
		t.Fatalf("delete predecessor: %v", err)
	}
	proof, err := repository.VerifyAuditProof(ctx, "tenant-audit-test", "aud_2")
	if err != nil {
		t.Fatalf("verify proof: %v", err)
	}
	if proof.Valid || proof.FailureReason != "MISSING_PREDECESSOR" || proof.PreviousRecordHash != first.RecordHash {
		t.Fatalf("expected missing predecessor, got %+v first=%s", proof, first.RecordHash)
	}
}

func prepareAuditRecord(t *testing.T, sourceService, sourceEventID, idempotencyKey, attributes string) domain.PreparedRecord {
	t.Helper()
	prepared, err := domain.PrepareRecord(types.AppendAuditRecordCommand{
		AuthContext:    validAuth(),
		AuditStream:    "security",
		SourceService:  sourceService,
		SourceEventID:  sourceEventID,
		RecordType:     "IDENTITY_AUTH",
		ActorRef:       "user:user-1",
		ResourceRef:    "session:session-1",
		Action:         "LOGIN",
		Outcome:        "SUCCEEDED",
		ReasonCode:     "OK",
		RiskLevel:      "LOW",
		OccurredAt:     time.Unix(100, 0).UTC(),
		AttributesJSON: attributes,
		IdempotencyKey: idempotencyKey,
		CorrelationID:  "corr-1",
	})
	if err != nil {
		t.Fatalf("prepare audit record: %v", err)
	}
	return prepared
}

func validAuth() types.AuthContext {
	return types.AuthContext{
		TenantID: "tenant-audit-test",
		UserID:   "user-1",
		DeviceID: "device-1",
	}
}

func assertAuditOutboxLowSensitive(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var payload string
	if err := pool.QueryRow(ctx, `
SELECT payload_json::text
FROM audit_outbox
WHERE tenant_id = 'tenant-audit-test'
  AND event_type = 'audit.record.appended.v1'
ORDER BY created_at ASC
LIMIT 1
`).Scan(&payload); err != nil {
		t.Fatalf("read audit outbox payload: %v", err)
	}
	for _, forbidden := range []string{"session-1", "provider body", "raw_prompt", "password"} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("audit outbox payload leaked forbidden value %q: %s", forbidden, payload)
		}
	}
	if !strings.Contains(payload, "aud_1") || !strings.Contains(payload, "record_hash") {
		t.Fatalf("audit outbox payload missing low-sensitive refs: %s", payload)
	}
}

func openAuditTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	t.Cleanup(pool.Close)
	applyAuditMigrations(t, context.Background(), pool)
	return pool
}

func applyAuditMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "audit")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migration dir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("read migration %s: %v", entry.Name(), err)
		}
		if _, err := pool.Exec(ctx, string(content)); err != nil {
			t.Fatalf("apply migration %s: %v", entry.Name(), err)
		}
	}
}

func resetAuditTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
TRUNCATE
	audit_outbox,
	audit_hash_segments,
	audit_records
`); err != nil {
		t.Fatalf("reset audit tables: %v", err)
	}
}
