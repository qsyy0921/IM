package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/audit-service/internal/domain"
	"github.com/qsyy0921/IM/services/audit-service/internal/types"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (repository *Repository) AppendAuditRecord(
	ctx context.Context,
	prepared domain.PreparedRecord,
	auditID string,
) (types.AuditRecord, error) {
	if repository.pool == nil {
		return types.AuditRecord{}, types.NewDBWriteFailed("audit repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.AuditRecord{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockAuditAppend(ctx, tx, prepared.Command); err != nil {
		return types.AuditRecord{}, types.NewDBWriteFailed(err.Error())
	}

	existing, found, err := findExistingRecord(ctx, tx, prepared.Command)
	if err != nil {
		return types.AuditRecord{}, types.NewDBReadFailed(err.Error())
	}
	if found {
		if existing.CommandHash != prepared.CommandHash {
			return types.AuditRecord{}, types.NewAlreadyExists("audit idempotency conflict")
		}
		if err := tx.Commit(ctx); err != nil {
			return types.AuditRecord{}, types.NewDBReadFailed(err.Error())
		}
		return existing, nil
	}

	previousHash, err := lastRecordHash(ctx, tx, prepared.Command.AuthContext.TenantID, prepared.Command.AuditStream)
	if err != nil {
		return types.AuditRecord{}, types.NewDBReadFailed(err.Error())
	}
	recordHash := domain.RecordHash(
		previousHash,
		prepared.CanonicalJSONHash,
		prepared.Command.AuthContext.TenantID,
		prepared.Command.AuditStream,
		auditID,
	)

	row := tx.QueryRow(ctx, `
INSERT INTO audit_records (
	tenant_id,
	audit_id,
	audit_stream,
	source_service,
	source_event_id,
	record_type,
	actor_ref,
	subject_ref,
	resource_ref,
	action,
	outcome,
	reason_code,
	risk_level,
	occurred_at,
	attributes_json,
	canonical_json_hash,
	previous_record_hash,
	record_hash,
	idempotency_key,
	command_hash,
	correlation_id,
	causation_id,
	trace_id
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9,
	$10, $11, $12, $13, $14, $15::jsonb,
	$16, $17, $18, $19, $20, $21, $22, $23
)
RETURNING
	tenant_id,
	audit_id,
	audit_stream,
	source_service,
	source_event_id,
	record_type,
	actor_ref,
	subject_ref,
	resource_ref,
	action,
	outcome,
	reason_code,
	risk_level,
	occurred_at,
	ingested_at,
	attributes_json,
	canonical_json_hash,
	previous_record_hash,
	record_hash,
	idempotency_key,
	command_hash,
	correlation_id,
	causation_id,
	trace_id
`, prepared.Command.AuthContext.TenantID,
		auditID,
		prepared.Command.AuditStream,
		prepared.Command.SourceService,
		prepared.Command.SourceEventID,
		prepared.Command.RecordType,
		prepared.Command.ActorRef,
		prepared.Command.SubjectRef,
		prepared.Command.ResourceRef,
		prepared.Command.Action,
		prepared.Command.Outcome,
		prepared.Command.ReasonCode,
		prepared.Command.RiskLevel,
		prepared.Command.OccurredAt,
		prepared.Command.AttributesJSON,
		prepared.CanonicalJSONHash,
		previousHash,
		recordHash,
		prepared.Command.IdempotencyKey,
		prepared.CommandHash,
		prepared.Command.CorrelationID,
		prepared.Command.CausationID,
		prepared.Command.TraceID,
	)
	record, err := scanRecord(row)
	if err != nil {
		return types.AuditRecord{}, types.NewDBWriteFailed(err.Error())
	}
	if err := insertRecordAppendedOutbox(ctx, tx, record); err != nil {
		return types.AuditRecord{}, types.NewDBWriteFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return types.AuditRecord{}, types.NewDBWriteFailed(err.Error())
	}
	return record, nil
}

func (repository *Repository) QueryAuditRecords(
	ctx context.Context,
	command types.QueryAuditRecordsCommand,
	fetchLimit int,
) ([]types.AuditRecord, error) {
	if repository.pool == nil {
		return nil, types.NewDBReadFailed("audit repository is not configured")
	}
	if fetchLimit <= 0 {
		return nil, nil
	}

	args := []any{command.AuthContext.TenantID, fetchLimit}
	filters := []string{"tenant_id = $1"}
	if strings.TrimSpace(command.AuditStream) != "" {
		args = append(args, strings.TrimSpace(command.AuditStream))
		filters = append(filters, "audit_stream = $"+itoa(len(args)))
	}
	if strings.TrimSpace(command.RecordType) != "" {
		args = append(args, strings.TrimSpace(command.RecordType))
		filters = append(filters, "record_type = $"+itoa(len(args)))
	}
	if strings.TrimSpace(command.SourceService) != "" {
		args = append(args, strings.TrimSpace(command.SourceService))
		filters = append(filters, "source_service = $"+itoa(len(args)))
	}
	if strings.TrimSpace(command.AfterAuditID) != "" {
		args = append(args, strings.TrimSpace(command.AfterAuditID))
		filters = append(filters, "audit_id > $"+itoa(len(args)))
	}

	query := `
SELECT
	tenant_id,
	audit_id,
	audit_stream,
	source_service,
	source_event_id,
	record_type,
	actor_ref,
	subject_ref,
	resource_ref,
	action,
	outcome,
	reason_code,
	risk_level,
	occurred_at,
	ingested_at,
	attributes_json,
	canonical_json_hash,
	previous_record_hash,
	record_hash,
	idempotency_key,
	command_hash,
	correlation_id,
	causation_id,
	trace_id
FROM audit_records
WHERE ` + strings.Join(filters, "\n  AND ") + `
ORDER BY audit_id ASC
LIMIT $2
`
	rows, err := repository.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	records := make([]types.AuditRecord, 0, fetchLimit)
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return records, nil
}

func (repository *Repository) CreateAuditExport(
	ctx context.Context,
	prepared domain.PreparedExport,
	exportID string,
) (types.AuditExportJob, error) {
	if repository.pool == nil {
		return types.AuditExportJob{}, types.NewDBWriteFailed("audit repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.AuditExportJob{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	existing, found, err := findExistingExport(ctx, tx, prepared.Command.AuthContext.TenantID, prepared.Command.IdempotencyKey)
	if err != nil {
		return types.AuditExportJob{}, types.NewDBReadFailed(err.Error())
	}
	if found {
		if existing.CommandHash != prepared.CommandHash {
			return types.AuditExportJob{}, types.NewAlreadyExists("audit export idempotency conflict")
		}
		if err := tx.Commit(ctx); err != nil {
			return types.AuditExportJob{}, types.NewDBReadFailed(err.Error())
		}
		return existing, nil
	}

	row := tx.QueryRow(ctx, `
INSERT INTO audit_export_jobs (
	tenant_id,
	export_id,
	status,
	audit_stream,
	record_type,
	source_service,
	filter_hash,
	redaction_profile,
	requested_by_ref,
	idempotency_key,
	command_hash,
	correlation_id,
	causation_id,
	trace_id
) VALUES (
	$1, $2, 'PENDING', $3, $4, $5, $6, $7, $8,
	$9, $10, $11, $12, $13
)
RETURNING
	tenant_id,
	export_id,
	status,
	audit_stream,
	record_type,
	source_service,
	filter_hash,
	redaction_profile,
	requested_by_ref,
	requested_at,
	manifest_ref,
	record_count,
	completed_at,
	failed_at,
	public_error,
	idempotency_key,
	command_hash,
	correlation_id,
	causation_id,
	trace_id
`, prepared.Command.AuthContext.TenantID,
		exportID,
		prepared.Command.AuditStream,
		prepared.Command.RecordType,
		prepared.Command.SourceService,
		prepared.Command.FilterHash,
		prepared.Command.RedactionProfile,
		prepared.Command.RequestedByRef,
		prepared.Command.IdempotencyKey,
		prepared.CommandHash,
		prepared.Command.CorrelationID,
		prepared.Command.CausationID,
		prepared.Command.TraceID,
	)
	job, err := scanExportJob(row)
	if err != nil {
		return types.AuditExportJob{}, types.NewDBWriteFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return types.AuditExportJob{}, types.NewDBWriteFailed(err.Error())
	}
	return job, nil
}

func (repository *Repository) GetAuditExport(
	ctx context.Context,
	tenantID types.TenantID,
	exportID string,
) (types.AuditExportJob, error) {
	if repository.pool == nil {
		return types.AuditExportJob{}, types.NewDBReadFailed("audit repository is not configured")
	}
	row := repository.pool.QueryRow(ctx, `
SELECT
	tenant_id,
	export_id,
	status,
	audit_stream,
	record_type,
	source_service,
	filter_hash,
	redaction_profile,
	requested_by_ref,
	requested_at,
	manifest_ref,
	record_count,
	completed_at,
	failed_at,
	public_error,
	idempotency_key,
	command_hash,
	correlation_id,
	causation_id,
	trace_id
FROM audit_export_jobs
WHERE tenant_id = $1
  AND export_id = $2
`, tenantID, strings.TrimSpace(exportID))
	job, err := scanExportJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return types.AuditExportJob{}, types.NewNotFound("audit export not found")
	}
	if err != nil {
		return types.AuditExportJob{}, types.NewDBReadFailed(err.Error())
	}
	return job, nil
}

func (repository *Repository) VerifyAuditProof(
	ctx context.Context,
	tenantID types.TenantID,
	auditID string,
) (types.AuditProofVerification, error) {
	if repository.pool == nil {
		return types.AuditProofVerification{}, types.NewDBReadFailed("audit repository is not configured")
	}
	record, err := getRecord(ctx, repository.pool, tenantID, auditID)
	if err != nil {
		return types.AuditProofVerification{}, err
	}
	result := types.AuditProofVerification{
		AuditID:            record.AuditID,
		RecordHash:         record.RecordHash,
		PreviousRecordHash: record.PreviousRecordHash,
	}
	if record.PreviousRecordHash != "" {
		exists, err := recordHashExists(ctx, repository.pool, record.TenantID, record.AuditStream, record.PreviousRecordHash)
		if err != nil {
			return types.AuditProofVerification{}, err
		}
		if !exists {
			result.FailureReason = "MISSING_PREDECESSOR"
			return result, nil
		}
	}
	expected := domain.RecordHash(
		record.PreviousRecordHash,
		record.CanonicalJSONHash,
		record.TenantID,
		record.AuditStream,
		record.AuditID,
	)
	if expected != record.RecordHash {
		result.FailureReason = "HASH_MISMATCH"
		return result, nil
	}
	result.Valid = true
	return result, nil
}

func findExistingExport(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	idempotencyKey string,
) (types.AuditExportJob, bool, error) {
	row := tx.QueryRow(ctx, `
SELECT
	tenant_id,
	export_id,
	status,
	audit_stream,
	record_type,
	source_service,
	filter_hash,
	redaction_profile,
	requested_by_ref,
	requested_at,
	manifest_ref,
	record_count,
	completed_at,
	failed_at,
	public_error,
	idempotency_key,
	command_hash,
	correlation_id,
	causation_id,
	trace_id
FROM audit_export_jobs
WHERE tenant_id = $1
  AND idempotency_key = $2
LIMIT 1
FOR UPDATE
`, tenantID, idempotencyKey)
	job, err := scanExportJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return types.AuditExportJob{}, false, nil
	}
	if err != nil {
		return types.AuditExportJob{}, false, err
	}
	return job, true, nil
}

func findExistingRecord(
	ctx context.Context,
	tx pgx.Tx,
	command types.AppendAuditRecordCommand,
) (types.AuditRecord, bool, error) {
	row := tx.QueryRow(ctx, `
SELECT
	tenant_id,
	audit_id,
	audit_stream,
	source_service,
	source_event_id,
	record_type,
	actor_ref,
	subject_ref,
	resource_ref,
	action,
	outcome,
	reason_code,
	risk_level,
	occurred_at,
	ingested_at,
	attributes_json,
	canonical_json_hash,
	previous_record_hash,
	record_hash,
	idempotency_key,
	command_hash,
	correlation_id,
	causation_id,
	trace_id
FROM audit_records
WHERE tenant_id = $1
  AND (
	(source_service = $2 AND source_event_id = $3)
	OR idempotency_key = $4
  )
ORDER BY ingested_at ASC, audit_id ASC
LIMIT 1
FOR UPDATE
`, command.AuthContext.TenantID, command.SourceService, command.SourceEventID, command.IdempotencyKey)
	record, err := scanRecord(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return types.AuditRecord{}, false, nil
	}
	if err != nil {
		return types.AuditRecord{}, false, err
	}
	return record, true, nil
}

func lockAuditAppend(ctx context.Context, tx pgx.Tx, command types.AppendAuditRecordCommand) error {
	for _, key := range []string{
		strings.Join([]string{"audit-idempotency", string(command.AuthContext.TenantID), command.SourceService, command.SourceEventID, command.IdempotencyKey}, "|"),
		strings.Join([]string{"audit-stream", string(command.AuthContext.TenantID), command.AuditStream}, "|"),
	} {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key); err != nil {
			return err
		}
	}
	return nil
}

func lastRecordHash(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, stream string) (string, error) {
	var recordHash string
	err := tx.QueryRow(ctx, `
SELECT record_hash
FROM audit_records
WHERE tenant_id = $1
  AND audit_stream = $2
ORDER BY ingested_at DESC, audit_id DESC
LIMIT 1
FOR UPDATE
`, tenantID, stream).Scan(&recordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return recordHash, err
}

func insertRecordAppendedOutbox(ctx context.Context, tx pgx.Tx, record types.AuditRecord) error {
	payload := map[string]any{
		"audit_id":        record.AuditID,
		"audit_stream":    record.AuditStream,
		"source_service":  record.SourceService,
		"source_event_id": record.SourceEventID,
		"record_type":     record.RecordType,
		"record_hash":     record.RecordHash,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
INSERT INTO audit_outbox (
	event_id,
	tenant_id,
	aggregate_type,
	aggregate_id,
	event_type,
	event_version,
	partition_key,
	payload_json
) VALUES ($1, $2, 'audit_record', $3, 'audit.record.appended.v1', 1, $4, $5::jsonb)
`, record.AuditID+".appended", record.TenantID, record.AuditID, string(record.TenantID)+":"+record.AuditStream, string(encoded))
	return err
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func getRecord(ctx context.Context, db querier, tenantID types.TenantID, auditID string) (types.AuditRecord, error) {
	row := db.QueryRow(ctx, `
SELECT
	tenant_id,
	audit_id,
	audit_stream,
	source_service,
	source_event_id,
	record_type,
	actor_ref,
	subject_ref,
	resource_ref,
	action,
	outcome,
	reason_code,
	risk_level,
	occurred_at,
	ingested_at,
	attributes_json,
	canonical_json_hash,
	previous_record_hash,
	record_hash,
	idempotency_key,
	command_hash,
	correlation_id,
	causation_id,
	trace_id
FROM audit_records
WHERE tenant_id = $1
  AND audit_id = $2
`, tenantID, strings.TrimSpace(auditID))
	record, err := scanRecord(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return types.AuditRecord{}, types.NewNotFound("audit record not found")
	}
	if err != nil {
		return types.AuditRecord{}, types.NewDBReadFailed(err.Error())
	}
	return record, nil
}

func recordHashExists(
	ctx context.Context,
	db *pgxpool.Pool,
	tenantID types.TenantID,
	stream string,
	recordHash string,
) (bool, error) {
	var exists bool
	if err := db.QueryRow(ctx, `
SELECT EXISTS (
	SELECT 1
	FROM audit_records
	WHERE tenant_id = $1
	  AND audit_stream = $2
	  AND record_hash = $3
)
`, tenantID, stream, recordHash).Scan(&exists); err != nil {
		return false, types.NewDBReadFailed(err.Error())
	}
	return exists, nil
}

type recordScanner interface {
	Scan(dest ...any) error
}

func scanRecord(scanner recordScanner) (types.AuditRecord, error) {
	var record types.AuditRecord
	var attributesRaw []byte
	if err := scanner.Scan(
		&record.TenantID,
		&record.AuditID,
		&record.AuditStream,
		&record.SourceService,
		&record.SourceEventID,
		&record.RecordType,
		&record.ActorRef,
		&record.SubjectRef,
		&record.ResourceRef,
		&record.Action,
		&record.Outcome,
		&record.ReasonCode,
		&record.RiskLevel,
		&record.OccurredAt,
		&record.IngestedAt,
		&attributesRaw,
		&record.CanonicalJSONHash,
		&record.PreviousRecordHash,
		&record.RecordHash,
		&record.IdempotencyKey,
		&record.CommandHash,
		&record.CorrelationID,
		&record.CausationID,
		&record.TraceID,
	); err != nil {
		return types.AuditRecord{}, err
	}
	record.AttributesJSON = string(attributesRaw)
	return record, nil
}

func scanExportJob(scanner recordScanner) (types.AuditExportJob, error) {
	var job types.AuditExportJob
	var completedAt sql.NullTime
	var failedAt sql.NullTime
	if err := scanner.Scan(
		&job.TenantID,
		&job.ExportID,
		&job.Status,
		&job.AuditStream,
		&job.RecordType,
		&job.SourceService,
		&job.FilterHash,
		&job.RedactionProfile,
		&job.RequestedByRef,
		&job.RequestedAt,
		&job.ManifestRef,
		&job.RecordCount,
		&completedAt,
		&failedAt,
		&job.PublicError,
		&job.IdempotencyKey,
		&job.CommandHash,
		&job.CorrelationID,
		&job.CausationID,
		&job.TraceID,
	); err != nil {
		return types.AuditExportJob{}, err
	}
	if completedAt.Valid {
		job.CompletedAt = completedAt.Time
	}
	if failedAt.Valid {
		job.FailedAt = failedAt.Time
	}
	return job, nil
}

func itoa(value int) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	var buffer [20]byte
	index := len(buffer)
	for value > 0 {
		index--
		buffer[index] = digits[value%10]
		value /= 10
	}
	return string(buffer[index:])
}
