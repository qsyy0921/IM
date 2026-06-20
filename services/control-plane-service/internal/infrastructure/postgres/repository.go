package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/control-plane-service/internal/domain"
	"github.com/qsyy0921/IM/services/control-plane-service/internal/types"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (repository *Repository) PublishConfigVersion(
	ctx context.Context,
	prepared domain.PreparedConfigVersion,
	eventID string,
) (types.ConfigVersion, error) {
	if repository.pool == nil {
		return types.ConfigVersion{}, types.NewDBWriteFailed("control-plane repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.ConfigVersion{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	command := prepared.Command
	if err := upsertBundle(ctx, tx, command); err != nil {
		return types.ConfigVersion{}, types.NewDBWriteFailed(err.Error())
	}
	existing, found, err := findExistingVersion(ctx, tx, command)
	if err != nil {
		return types.ConfigVersion{}, types.NewDBReadFailed(err.Error())
	}
	if found {
		if existing.CommandHash != prepared.CommandHash {
			return types.ConfigVersion{}, types.NewAlreadyExists("config version idempotency conflict")
		}
		if err := tx.Commit(ctx); err != nil {
			return types.ConfigVersion{}, types.NewDBReadFailed(err.Error())
		}
		return existing, nil
	}

	version, err := insertConfigVersion(ctx, tx, prepared)
	if err != nil {
		return types.ConfigVersion{}, types.NewDBWriteFailed(err.Error())
	}
	if err := insertControlOutbox(ctx, tx, eventID, version, "control.config.published.v1"); err != nil {
		return types.ConfigVersion{}, types.NewDBWriteFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return types.ConfigVersion{}, types.NewDBWriteFailed(err.Error())
	}
	return version, nil
}

func (repository *Repository) GetConfigSnapshot(
	ctx context.Context,
	command types.GetConfigSnapshotCommand,
) (types.ConfigSnapshot, error) {
	if repository.pool == nil {
		return types.ConfigSnapshot{}, types.NewDBReadFailed("control-plane repository is not configured")
	}
	now := time.Now().UTC()
	row := repository.pool.QueryRow(ctx, `
SELECT tenant_id, environment, config_kind, bundle_key, version, schema_version,
       payload_json::text, payload_checksum, effective_at, COALESCE(expires_at, 'epoch'::timestamptz)
FROM control_config_versions
WHERE tenant_id = $1
  AND environment = $2
  AND config_kind = $3
  AND bundle_key = $4
  AND status IN ('PUBLISHED', 'ACTIVE')
  AND effective_at <= $5
  AND (expires_at IS NULL OR expires_at > $5)
ORDER BY effective_at DESC, created_at DESC
LIMIT 1
`, string(command.AuthContext.TenantID), command.Environment, command.ConfigKind, command.BundleKey, now)
	var snapshot types.ConfigSnapshot
	if err := row.Scan(
		&snapshot.TenantID,
		&snapshot.Environment,
		&snapshot.ConfigKind,
		&snapshot.BundleKey,
		&snapshot.Version,
		&snapshot.SchemaVersion,
		&snapshot.PayloadJSON,
		&snapshot.PayloadChecksum,
		&snapshot.EffectiveAt,
		&snapshot.ExpiresAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.ConfigSnapshot{}, types.NewNotFound("config snapshot not found")
		}
		return types.ConfigSnapshot{}, types.NewDBReadFailed(err.Error())
	}
	snapshot.ServiceName = command.ServiceName
	snapshot.GeneratedAt = now
	snapshot.RolloutDecision = "MATCH"
	snapshot.NotModified = command.CurrentVersion != "" && command.CurrentVersion == snapshot.Version
	return snapshot, nil
}

func (repository *Repository) AckAppliedConfigVersion(
	ctx context.Context,
	command types.AckAppliedConfigVersionCommand,
	eventID string,
) (types.AppliedConfigVersion, error) {
	if repository.pool == nil {
		return types.AppliedConfigVersion{}, types.NewDBWriteFailed("control-plane repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.AppliedConfigVersion{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	appliedAt := time.Now().UTC()
	applied := types.AppliedConfigVersion{
		TenantID:       command.AuthContext.TenantID,
		Environment:    command.Environment,
		ServiceName:    command.ServiceName,
		InstanceRef:    command.InstanceRef,
		ConfigKind:     command.ConfigKind,
		BundleKey:      command.BundleKey,
		Version:        command.Version,
		ServiceVersion: command.ServiceVersion,
		Status:         command.Status,
		LastErrorClass: command.LastErrorClass,
		AppliedAt:      appliedAt,
	}
	if err := upsertAppliedAck(ctx, tx, applied); err != nil {
		return types.AppliedConfigVersion{}, types.NewDBWriteFailed(err.Error())
	}
	if err := insertAppliedOutbox(ctx, tx, eventID, applied); err != nil {
		return types.AppliedConfigVersion{}, types.NewDBWriteFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return types.AppliedConfigVersion{}, types.NewDBWriteFailed(err.Error())
	}
	return applied, nil
}

func upsertBundle(ctx context.Context, tx pgx.Tx, command types.PublishConfigVersionCommand) error {
	_, err := tx.Exec(ctx, `
INSERT INTO control_config_bundles (
    tenant_id, environment, config_kind, bundle_key, status, created_at, updated_at
) VALUES ($1, $2, $3, $4, 'ACTIVE', now(), now())
ON CONFLICT (tenant_id, environment, config_kind, bundle_key)
DO UPDATE SET updated_at = now()
`, string(command.AuthContext.TenantID), command.Environment, command.ConfigKind, command.BundleKey)
	return err
}

func findExistingVersion(
	ctx context.Context,
	tx pgx.Tx,
	command types.PublishConfigVersionCommand,
) (types.ConfigVersion, bool, error) {
	row := tx.QueryRow(ctx, `
SELECT tenant_id, environment, config_kind, bundle_key, version, schema_version,
       payload_json::text, payload_checksum, command_hash, status,
       effective_at, COALESCE(expires_at, 'epoch'::timestamptz), published_at,
       approval_ref, operator_ref, reason_ref, idempotency_key, correlation_id, causation_id, trace_id
FROM control_config_versions
WHERE tenant_id = $1
  AND environment = $2
  AND config_kind = $3
  AND bundle_key = $4
  AND (version = $5 OR idempotency_key = $6)
LIMIT 1
`, string(command.AuthContext.TenantID), command.Environment, command.ConfigKind, command.BundleKey, command.Version, command.IdempotencyKey)
	version, err := scanConfigVersion(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.ConfigVersion{}, false, nil
		}
		return types.ConfigVersion{}, false, err
	}
	return version, true, nil
}

func insertConfigVersion(ctx context.Context, tx pgx.Tx, prepared domain.PreparedConfigVersion) (types.ConfigVersion, error) {
	command := prepared.Command
	var expiresAt any
	if !command.ExpiresAt.IsZero() {
		expiresAt = command.ExpiresAt
	}
	row := tx.QueryRow(ctx, `
INSERT INTO control_config_versions (
    tenant_id, environment, config_kind, bundle_key, version, schema_version,
    payload_json, payload_checksum, command_hash, status, effective_at, expires_at,
    published_at, approval_ref, operator_ref, reason_ref, idempotency_key,
    correlation_id, causation_id, trace_id
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7::jsonb, $8, $9, 'ACTIVE', $10, $11,
    now(), $12, $13, $14, $15, $16, $17, $18
)
RETURNING tenant_id, environment, config_kind, bundle_key, version, schema_version,
       payload_json::text, payload_checksum, command_hash, status,
       effective_at, COALESCE(expires_at, 'epoch'::timestamptz), published_at,
       approval_ref, operator_ref, reason_ref, idempotency_key, correlation_id, causation_id, trace_id
`, string(command.AuthContext.TenantID), command.Environment, command.ConfigKind, command.BundleKey,
		command.Version, command.SchemaVersion, prepared.PayloadJSON, prepared.PayloadChecksum, prepared.CommandHash,
		command.EffectiveAt, expiresAt, command.ApprovalRef, command.OperatorRef, command.ReasonRef,
		command.IdempotencyKey, command.CorrelationID, command.CausationID, command.TraceID)
	return scanConfigVersion(row)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanConfigVersion(row rowScanner) (types.ConfigVersion, error) {
	var version types.ConfigVersion
	if err := row.Scan(
		&version.TenantID,
		&version.Environment,
		&version.ConfigKind,
		&version.BundleKey,
		&version.Version,
		&version.SchemaVersion,
		&version.PayloadJSON,
		&version.PayloadChecksum,
		&version.CommandHash,
		&version.Status,
		&version.EffectiveAt,
		&version.ExpiresAt,
		&version.PublishedAt,
		&version.ApprovalRef,
		&version.OperatorRef,
		&version.ReasonRef,
		&version.IdempotencyKey,
		&version.CorrelationID,
		&version.CausationID,
		&version.TraceID,
	); err != nil {
		return types.ConfigVersion{}, err
	}
	return version, nil
}

func upsertAppliedAck(ctx context.Context, tx pgx.Tx, applied types.AppliedConfigVersion) error {
	_, err := tx.Exec(ctx, `
INSERT INTO control_applied_acks (
    tenant_id, environment, service_name, instance_ref, config_kind, bundle_key,
    version, service_version, status, last_error_class, applied_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11, now()
)
ON CONFLICT (tenant_id, environment, service_name, instance_ref, config_kind, bundle_key)
DO UPDATE SET
    version = EXCLUDED.version,
    service_version = EXCLUDED.service_version,
    status = EXCLUDED.status,
    last_error_class = EXCLUDED.last_error_class,
    applied_at = EXCLUDED.applied_at,
    updated_at = now()
`, string(applied.TenantID), applied.Environment, applied.ServiceName, applied.InstanceRef, applied.ConfigKind,
		applied.BundleKey, applied.Version, applied.ServiceVersion, applied.Status, applied.LastErrorClass, applied.AppliedAt)
	return err
}

func insertControlOutbox(ctx context.Context, tx pgx.Tx, eventID string, version types.ConfigVersion, eventType string) error {
	payload := map[string]any{
		"tenant_id":        string(version.TenantID),
		"environment":      version.Environment,
		"config_kind":      version.ConfigKind,
		"bundle_key":       version.BundleKey,
		"version":          version.Version,
		"schema_version":   version.SchemaVersion,
		"checksum_present": version.PayloadChecksum != "",
		"operator_ref":     version.OperatorRef,
		"approval_ref":     version.ApprovalRef,
		"trace_id":         version.TraceID,
	}
	return insertOutbox(ctx, tx, eventID, version.TenantID, "config_version",
		fmt.Sprintf("%s:%s:%s:%s", version.Environment, version.ConfigKind, version.BundleKey, version.Version),
		eventType, string(version.TenantID)+":"+version.BundleKey, payload)
}

func insertAppliedOutbox(ctx context.Context, tx pgx.Tx, eventID string, applied types.AppliedConfigVersion) error {
	payload := map[string]any{
		"tenant_id":       string(applied.TenantID),
		"environment":     applied.Environment,
		"service_name":    applied.ServiceName,
		"config_kind":     applied.ConfigKind,
		"bundle_key":      applied.BundleKey,
		"version":         applied.Version,
		"service_version": applied.ServiceVersion,
		"status":          applied.Status,
		"has_error_class": applied.LastErrorClass != "",
	}
	return insertOutbox(ctx, tx, eventID, applied.TenantID, "applied_ack",
		fmt.Sprintf("%s:%s:%s:%s", applied.Environment, applied.ServiceName, applied.ConfigKind, applied.BundleKey),
		"control.config.applied.v1", string(applied.TenantID)+":"+applied.ServiceName, payload)
}

func insertOutbox(
	ctx context.Context,
	tx pgx.Tx,
	eventID string,
	tenantID types.TenantID,
	aggregateType string,
	aggregateID string,
	eventType string,
	partitionKey string,
	payload map[string]any,
) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
INSERT INTO control_outbox (
    event_id, tenant_id, aggregate_type, aggregate_id, event_type, event_version,
    partition_key, payload_json, status, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, 1, $6, $7::jsonb, 'PENDING', now(), now())
`, eventID, string(tenantID), aggregateType, aggregateID, eventType, partitionKey, string(encoded))
	return err
}
