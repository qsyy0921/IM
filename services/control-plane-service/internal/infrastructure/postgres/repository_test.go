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
	assertOutboxSafe(t, ctx, pool, string(command.AuthContext.TenantID))
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
	path := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "control-plane", "000001_control_plane_core.sql")
	sqlBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
}

func resetControlTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
TRUNCATE
    control_outbox,
    control_applied_acks,
    control_rollout_rules,
    control_config_versions,
    control_config_bundles
CASCADE
`)
	if err != nil {
		t.Fatalf("reset control tables: %v", err)
	}
}

func assertOutboxSafe(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID string) {
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
	if count != 2 {
		t.Fatalf("expected two outbox rows, got %d", count)
	}
}
