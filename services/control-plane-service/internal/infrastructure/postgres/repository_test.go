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
	"github.com/qsyy0921/IM/services/control-plane-service/internal/domain"
	"github.com/qsyy0921/IM/services/control-plane-service/internal/types"
)

func TestRepositoryPublishSnapshotAckIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openControlTestPool(t)
	resetControlTables(t, ctx, pool)
	repository := NewRepository(pool)

	command := validPublishCommand()
	prepared, err := domain.PrepareConfigVersion(command)
	if err != nil {
		t.Fatalf("prepare config version: %v", err)
	}
	version, err := repository.PublishConfigVersion(ctx, prepared, "evt_1")
	if err != nil {
		t.Fatalf("publish config version: %v", err)
	}
	if version.PayloadChecksum == "" || strings.Contains(version.PayloadJSON, "secret") {
		t.Fatalf("unexpected version: %+v", version)
	}
	replay, err := repository.PublishConfigVersion(ctx, prepared, "evt_replay")
	if err != nil {
		t.Fatalf("publish replay: %v", err)
	}
	if replay.Version != version.Version || replay.PayloadChecksum != version.PayloadChecksum {
		t.Fatalf("unexpected replay: %+v", replay)
	}
	conflict := command
	conflict.PayloadJSON = `{"plans":{"tenant-free":{"requests_per_second":99,"burst":99}}}`
	conflictPrepared, err := domain.PrepareConfigVersion(conflict)
	if err != nil {
		t.Fatalf("prepare conflict: %v", err)
	}
	if _, err := repository.PublishConfigVersion(ctx, conflictPrepared, "evt_conflict"); !errors.Is(err, types.ErrAlreadyExists) {
		t.Fatalf("expected conflict, got %v", err)
	}

	snapshot, err := repository.GetConfigSnapshot(ctx, types.GetConfigSnapshotCommand{
		AuthContext:    types.AuthContext{TenantID: command.AuthContext.TenantID},
		Environment:    command.Environment,
		ServiceName:    "api-gateway",
		ConfigKind:     command.ConfigKind,
		BundleKey:      command.BundleKey,
		CurrentVersion: "",
	})
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshot.Version != command.Version || snapshot.PayloadChecksum != version.PayloadChecksum {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}

	applied, err := repository.AckAppliedConfigVersion(ctx, types.AckAppliedConfigVersionCommand{
		AuthContext:    types.AuthContext{TenantID: command.AuthContext.TenantID},
		Environment:    command.Environment,
		ServiceName:    "api-gateway",
		InstanceRef:    "api-gateway-1",
		ConfigKind:     command.ConfigKind,
		BundleKey:      command.BundleKey,
		Version:        command.Version,
		ServiceVersion: "local",
		Status:         types.AppliedStatusInSync,
	}, "evt_2")
	if err != nil {
		t.Fatalf("ack applied: %v", err)
	}
	if applied.Status != types.AppliedStatusInSync {
		t.Fatalf("unexpected applied ack: %+v", applied)
	}
	assertOutboxSafe(t, ctx, pool, string(command.AuthContext.TenantID), 2)
}

func TestRepositoryRollbackConfigVersionIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openControlTestPool(t)
	resetControlTables(t, ctx, pool)
	repository := NewRepository(pool)

	first := validPublishCommand()
	first.Version = "quota-v1"
	first.IdempotencyKey = "idem-v1"
	second := validPublishCommand()
	second.Version = "quota-v2"
	second.IdempotencyKey = "idem-v2"
	second.PayloadJSON = `{"plans":{"tenant-free":{"requests_per_second":30,"burst":60}}}`
	second.EffectiveAt = first.EffectiveAt.Add(time.Second)
	firstPrepared, err := domain.PrepareConfigVersion(first)
	if err != nil {
		t.Fatalf("prepare first: %v", err)
	}
	secondPrepared, err := domain.PrepareConfigVersion(second)
	if err != nil {
		t.Fatalf("prepare second: %v", err)
	}
	if _, err := repository.PublishConfigVersion(ctx, firstPrepared, "evt_publish_1"); err != nil {
		t.Fatalf("publish first: %v", err)
	}
	if _, err := repository.PublishConfigVersion(ctx, secondPrepared, "evt_publish_2"); err != nil {
		t.Fatalf("publish second: %v", err)
	}
	snapshot, err := repository.GetConfigSnapshot(ctx, types.GetConfigSnapshotCommand{
		AuthContext: types.AuthContext{TenantID: first.AuthContext.TenantID},
		Environment: first.Environment,
		ServiceName: "api-gateway",
		ConfigKind:  first.ConfigKind,
		BundleKey:   first.BundleKey,
	})
	if err != nil {
		t.Fatalf("get latest snapshot: %v", err)
	}
	if snapshot.Version != "quota-v2" {
		t.Fatalf("snapshot before rollback = %+v", snapshot)
	}

	rollback := types.RollbackConfigVersionCommand{
		AuthContext:    first.AuthContext,
		Environment:    first.Environment,
		ConfigKind:     first.ConfigKind,
		BundleKey:      first.BundleKey,
		TargetVersion:  first.Version,
		IdempotencyKey: "rollback-idem-1",
		OperatorRef:    "operator:test",
		ApprovalRef:    "approval:test",
		ReasonRef:      "reason:test",
	}
	preparedRollback, err := domain.PrepareConfigRollback(rollback)
	if err != nil {
		t.Fatalf("prepare rollback: %v", err)
	}
	rolledBack, replayed, err := repository.RollbackConfigVersion(ctx, preparedRollback, "evt_rollback_1")
	if err != nil {
		t.Fatalf("rollback config version: %v", err)
	}
	if replayed || rolledBack.Version != first.Version || rolledBack.Status != types.StatusActive {
		t.Fatalf("unexpected rollback result replayed=%v version=%+v", replayed, rolledBack)
	}
	replay, replayed, err := repository.RollbackConfigVersion(ctx, preparedRollback, "evt_rollback_replay")
	if err != nil {
		t.Fatalf("rollback replay: %v", err)
	}
	if !replayed || replay.Version != first.Version {
		t.Fatalf("unexpected rollback replay replayed=%v version=%+v", replayed, replay)
	}
	conflict := rollback
	conflict.TargetVersion = second.Version
	conflictPrepared, err := domain.PrepareConfigRollback(conflict)
	if err != nil {
		t.Fatalf("prepare conflict rollback: %v", err)
	}
	if _, _, err := repository.RollbackConfigVersion(ctx, conflictPrepared, "evt_rollback_conflict"); !errors.Is(err, types.ErrAlreadyExists) {
		t.Fatalf("expected rollback idempotency conflict, got %v", err)
	}

	snapshot, err = repository.GetConfigSnapshot(ctx, types.GetConfigSnapshotCommand{
		AuthContext: types.AuthContext{TenantID: first.AuthContext.TenantID},
		Environment: first.Environment,
		ServiceName: "api-gateway",
		ConfigKind:  first.ConfigKind,
		BundleKey:   first.BundleKey,
	})
	if err != nil {
		t.Fatalf("get snapshot after rollback: %v", err)
	}
	if snapshot.Version != first.Version || snapshot.PayloadChecksum != rolledBack.PayloadChecksum {
		t.Fatalf("unexpected snapshot after rollback: %+v", snapshot)
	}
	var secondStatus string
	if err := pool.QueryRow(ctx, `
SELECT status
FROM control_config_versions
WHERE tenant_id = $1 AND environment = $2 AND config_kind = $3 AND bundle_key = $4 AND version = $5
`, string(first.AuthContext.TenantID), first.Environment, first.ConfigKind, first.BundleKey, second.Version).Scan(&secondStatus); err != nil {
		t.Fatalf("read second status: %v", err)
	}
	if secondStatus != types.StatusRolledBack {
		t.Fatalf("second status = %s", secondStatus)
	}
	assertOutboxSafe(t, ctx, pool, string(first.AuthContext.TenantID), 3)
}

func validPublishCommand() types.PublishConfigVersionCommand {
	return types.PublishConfigVersionCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-control-test",
			UserID:   "operator-1",
		},
		Environment:    "local",
		ConfigKind:     types.ConfigKindAPIGatewayTenantQuota,
		BundleKey:      "api-gateway/default",
		Version:        "quota-v1",
		SchemaVersion:  "quota-v1",
		PayloadJSON:    `{"plans":{"tenant-free":{"requests_per_second":20,"burst":40}}}`,
		EffectiveAt:    time.Unix(1000, 0),
		IdempotencyKey: "idem-1",
		OperatorRef:    "operator:test",
		ApprovalRef:    "approval:test",
	}
}

func openControlTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is required for control-plane postgres integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	t.Cleanup(pool.Close)
	applyControlMigration(t, ctx, pool)
	return pool
}

func applyControlMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "control-plane")
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	for _, path := range files {
		sqlBytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", path, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("apply migration %s: %v", path, err)
		}
	}
}

func resetControlTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
TRUNCATE
    control_outbox,
    control_applied_acks,
    control_config_rollbacks,
    control_rollout_rules,
    control_config_versions,
    control_config_bundles
CASCADE
`)
	if err != nil {
		t.Fatalf("reset control tables: %v", err)
	}
}

func assertOutboxSafe(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID string, expected int) {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT payload_json::text FROM control_outbox WHERE tenant_id = $1`, tenantID)
	if err != nil {
		t.Fatalf("query control outbox: %v", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
		var payload string
		if err := rows.Scan(&payload); err != nil {
			t.Fatalf("scan outbox: %v", err)
		}
		for _, forbidden := range []string{"requests_per_second", "provider_token", "payload_json", "secret"} {
			if strings.Contains(payload, forbidden) {
				t.Fatalf("outbox payload leaked %q: %s", forbidden, payload)
			}
		}
	}
	if count != expected {
		t.Fatalf("expected %d outbox rows, got %d", expected, count)
	}
}
