package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/media-service/internal/app"
	"github.com/qsyy0921/IM/services/media-service/internal/types"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (repository *Repository) CreateUploadSession(
	ctx context.Context,
	command types.CreateUploadSessionCommand,
	allocation app.UploadAllocation,
) (types.MediaAsset, types.UploadSession, error) {
	if repository.pool == nil {
		return types.MediaAsset{}, types.UploadSession{}, types.NewDBWriteFailed("media repository is not configured")
	}
	command = command.Normalized()
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return types.MediaAsset{}, types.UploadSession{}, types.NewDBWriteFailed(err.Error())
	}
	defer rollback(ctx, tx)

	existingAsset, existingSession, err := selectUploadSessionByIdempotency(
		ctx,
		tx,
		command.AuthContext.TenantID,
		command.AuthContext.UserID,
		command.IdempotencyKey,
	)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return types.MediaAsset{}, types.UploadSession{}, types.NewDBReadFailed(err.Error())
	}
	if err == nil {
		if existingSession.CommandHash != command.CommandHash() {
			return types.MediaAsset{}, types.UploadSession{}, types.NewAlreadyExists("idempotency key command mismatch")
		}
		if err := tx.Commit(ctx); err != nil {
			return types.MediaAsset{}, types.UploadSession{}, types.NewDBWriteFailed(err.Error())
		}
		return existingAsset, existingSession, nil
	}

	asset, err := insertMediaAsset(ctx, tx, command, allocation)
	if err != nil {
		return types.MediaAsset{}, types.UploadSession{}, types.NewDBWriteFailed(err.Error())
	}
	session, err := insertUploadSession(ctx, tx, command, allocation)
	if err != nil {
		return types.MediaAsset{}, types.UploadSession{}, types.NewDBWriteFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return types.MediaAsset{}, types.UploadSession{}, types.NewDBWriteFailed(err.Error())
	}
	return asset, session, nil
}

func (repository *Repository) GetAsset(
	ctx context.Context,
	tenantID types.TenantID,
	assetID string,
) (types.MediaAsset, error) {
	if repository.pool == nil {
		return types.MediaAsset{}, types.NewDBReadFailed("media repository is not configured")
	}
	row := repository.pool.QueryRow(ctx, selectAssetSQL()+`
WHERE tenant_id = $1
  AND asset_id = $2
`, tenantID, strings.TrimSpace(assetID))
	asset, err := scanAsset(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.MediaAsset{}, types.NewMediaAssetNotFound("media asset not found")
		}
		return types.MediaAsset{}, types.NewDBReadFailed(err.Error())
	}
	return asset, nil
}

func (repository *Repository) CompleteUpload(
	ctx context.Context,
	command types.CompleteUploadCommand,
	metadata types.ObjectMetadata,
) (types.MediaAsset, error) {
	if repository.pool == nil {
		return types.MediaAsset{}, types.NewDBWriteFailed("media repository is not configured")
	}
	command = command.Normalized()
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return types.MediaAsset{}, types.NewDBWriteFailed(err.Error())
	}
	defer rollback(ctx, tx)

	asset, session, err := selectUploadSessionForComplete(ctx, tx, command)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.MediaAsset{}, types.NewUploadSessionNotFound("upload session not found")
		}
		return types.MediaAsset{}, types.NewDBReadFailed(err.Error())
	}
	if asset.OwnerUserID != command.AuthContext.UserID {
		return types.MediaAsset{}, types.NewPermissionDenied("asset owner mismatch")
	}
	if session.Status == "COMPLETED" {
		if err := tx.Commit(ctx); err != nil {
			return types.MediaAsset{}, types.NewDBWriteFailed(err.Error())
		}
		return asset, nil
	}
	if session.Status != "PENDING" {
		return types.MediaAsset{}, types.NewFailedPrecondition("upload session is not pending")
	}
	if time.Now().After(session.ExpiresAt) {
		if _, err := tx.Exec(ctx, `
UPDATE media_upload_sessions SET status = 'EXPIRED' WHERE tenant_id = $1 AND upload_session_id = $2
`, command.AuthContext.TenantID, command.UploadSessionID); err != nil {
			return types.MediaAsset{}, types.NewDBWriteFailed(err.Error())
		}
		if _, err := tx.Exec(ctx, `
UPDATE media_assets SET status = 'EXPIRED' WHERE tenant_id = $1 AND asset_id = $2
`, command.AuthContext.TenantID, command.AssetID); err != nil {
			return types.MediaAsset{}, types.NewDBWriteFailed(err.Error())
		}
		if err := tx.Commit(ctx); err != nil {
			return types.MediaAsset{}, types.NewDBWriteFailed(err.Error())
		}
		return types.MediaAsset{}, types.NewFailedPrecondition("upload session expired")
	}
	if asset.SHA256 != metadata.SHA256 || asset.SizeBytes != metadata.SizeBytes {
		return types.MediaAsset{}, types.NewInvalidArgument("uploaded object metadata mismatch")
	}

	if _, err := tx.Exec(ctx, `
UPDATE media_upload_sessions
SET status = 'COMPLETED',
    completed_at = now()
WHERE tenant_id = $1
  AND upload_session_id = $2;
`, command.AuthContext.TenantID, command.UploadSessionID); err != nil {
		return types.MediaAsset{}, types.NewDBWriteFailed(err.Error())
	}

	scanStatus, thumbnailStatus, transcodeStatus := initialProcessingStatuses(asset.MediaKind)
	row := tx.QueryRow(ctx, `
UPDATE media_assets
SET status = 'PROCESSING',
    scan_status = $3,
    thumbnail_status = $4,
    transcode_status = $5,
    uploaded_at = COALESCE(uploaded_at, now())
WHERE tenant_id = $1
  AND asset_id = $2
RETURNING
	tenant_id,
	asset_id,
	owner_user_id,
	conversation_id,
	media_kind,
	content_type,
	file_name,
	size_bytes,
	sha256,
	object_key,
	status,
	scan_status,
	thumbnail_status,
	transcode_status,
	created_at,
	uploaded_at,
	ready_at,
	deleted_at
`, command.AuthContext.TenantID, command.AssetID, scanStatus, thumbnailStatus, transcodeStatus)
	updatedAsset, err := scanAsset(row)
	if err != nil {
		return types.MediaAsset{}, types.NewDBWriteFailed(err.Error())
	}
	if err := insertProcessingJobs(ctx, tx, updatedAsset); err != nil {
		return types.MediaAsset{}, types.NewDBWriteFailed(err.Error())
	}
	if err := insertMediaEvent(ctx, tx, updatedAsset, "media.asset.uploaded.v1", updatedAsset.AssetID+"-uploaded-v1", "UPLOADED"); err != nil {
		return types.MediaAsset{}, types.NewDBWriteFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return types.MediaAsset{}, types.NewDBWriteFailed(err.Error())
	}
	return updatedAsset, nil
}

func (repository *Repository) RecordAccessAudit(ctx context.Context, audit types.AccessAudit) error {
	if repository.pool == nil {
		return types.NewDBWriteFailed("media repository is not configured")
	}
	_, err := repository.pool.Exec(ctx, `
INSERT INTO media_access_audit (
	tenant_id,
	audit_id,
	asset_id,
	user_id,
	conversation_id,
	message_id,
	variant,
	decision,
	decision_source,
	request_id,
	created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
`, audit.TenantID,
		audit.AuditID,
		audit.AssetID,
		audit.UserID,
		audit.ConversationID,
		audit.MessageID,
		audit.Variant,
		audit.Decision,
		audit.DecisionSource,
		audit.RequestID,
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func (repository *Repository) DeleteMediaAsset(
	ctx context.Context,
	command types.DeleteMediaAssetCommand,
) (types.MediaAsset, error) {
	if repository.pool == nil {
		return types.MediaAsset{}, types.NewDBWriteFailed("media repository is not configured")
	}
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return types.MediaAsset{}, types.NewDBWriteFailed(err.Error())
	}
	defer rollback(ctx, tx)

	row := tx.QueryRow(ctx, selectAssetSQL()+`
WHERE tenant_id = $1
  AND asset_id = $2
FOR UPDATE
`, command.AuthContext.TenantID, strings.TrimSpace(command.AssetID))
	asset, err := scanAsset(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.MediaAsset{}, types.NewMediaAssetNotFound("media asset not found")
		}
		return types.MediaAsset{}, types.NewDBReadFailed(err.Error())
	}
	if asset.OwnerUserID != command.AuthContext.UserID {
		return types.MediaAsset{}, types.NewPermissionDenied("asset owner mismatch")
	}
	if asset.Status != types.AssetStatusDeleted {
		row = tx.QueryRow(ctx, `
UPDATE media_assets
SET status = 'DELETED',
    deleted_at = COALESCE(deleted_at, now())
WHERE tenant_id = $1
  AND asset_id = $2
RETURNING
	tenant_id,
	asset_id,
	owner_user_id,
	conversation_id,
	media_kind,
	content_type,
	file_name,
	size_bytes,
	sha256,
	object_key,
	status,
	scan_status,
	thumbnail_status,
	transcode_status,
	created_at,
	uploaded_at,
	ready_at,
	deleted_at
`, command.AuthContext.TenantID, asset.AssetID)
		asset, err = scanAsset(row)
		if err != nil {
			return types.MediaAsset{}, types.NewDBWriteFailed(err.Error())
		}
	}
	eventID := asset.AssetID + "-deleted-" + strings.TrimSpace(command.DeleteRequestID)
	if err := insertMediaEvent(ctx, tx, asset, "media.asset.deleted.v1", eventID, "DELETED"); err != nil {
		return types.MediaAsset{}, types.NewDBWriteFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return types.MediaAsset{}, types.NewDBWriteFailed(err.Error())
	}
	return asset, nil
}

func (repository *Repository) ClaimProcessingJobs(ctx context.Context, limit int) ([]types.ProcessingJob, error) {
	if repository.pool == nil {
		return nil, types.NewDBReadFailed("media repository is not configured")
	}
	if limit <= 0 {
		limit = 50
	}
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer rollback(ctx, tx)

	rows, err := tx.Query(ctx, `
WITH ready AS (
	SELECT j.tenant_id, j.job_id
	FROM media_processing_jobs j
	JOIN media_assets a
	  ON a.tenant_id = j.tenant_id
	 AND a.asset_id = j.asset_id
	WHERE j.status IN ('PENDING', 'FAILED')
	  AND (j.next_retry_at IS NULL OR j.next_retry_at <= now())
	  AND j.dead_lettered_at IS NULL
	  AND a.status = 'PROCESSING'
	ORDER BY j.created_at, j.job_id
	LIMIT $1
	FOR UPDATE OF j SKIP LOCKED
),
claimed AS (
	UPDATE media_processing_jobs j
	SET status = 'RUNNING',
	    attempt_count = j.attempt_count + 1,
	    next_retry_at = NULL,
	    updated_at = now()
	FROM ready
	WHERE j.tenant_id = ready.tenant_id
	  AND j.job_id = ready.job_id
	RETURNING
		j.tenant_id,
		j.job_id,
		j.asset_id,
		j.job_type,
		j.status,
		j.attempt_count,
		j.next_retry_at,
		j.last_error,
		j.dead_lettered_at,
		j.created_at,
		j.updated_at
)
SELECT
	c.tenant_id,
	c.job_id,
	c.asset_id,
	c.job_type,
	c.status,
	c.attempt_count,
	c.next_retry_at,
	c.last_error,
	c.dead_lettered_at,
	c.created_at,
	c.updated_at,
	a.tenant_id,
	a.asset_id,
	a.owner_user_id,
	a.conversation_id,
	a.media_kind,
	a.content_type,
	a.file_name,
	a.size_bytes,
	a.sha256,
	a.object_key,
	a.status,
	a.scan_status,
	a.thumbnail_status,
	a.transcode_status,
	a.created_at,
	a.uploaded_at,
	a.ready_at,
	a.deleted_at
FROM claimed c
JOIN media_assets a
  ON a.tenant_id = c.tenant_id
 AND a.asset_id = c.asset_id
ORDER BY c.created_at, c.job_id
`, limit)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	jobs := make([]types.ProcessingJob, 0)
	for rows.Next() {
		job, err := scanProcessingJob(rows)
		if err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	return jobs, nil
}

func (repository *Repository) MarkProcessingJobSucceeded(ctx context.Context, job types.ProcessingJob) (types.MediaAsset, error) {
	if repository.pool == nil {
		return types.MediaAsset{}, types.NewDBWriteFailed("media repository is not configured")
	}
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return types.MediaAsset{}, types.NewDBWriteFailed(err.Error())
	}
	defer rollback(ctx, tx)

	locked, err := lockProcessingJob(ctx, tx, job.TenantID, job.JobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.MediaAsset{}, types.NewFailedPrecondition("processing job not found")
		}
		return types.MediaAsset{}, types.NewDBReadFailed(err.Error())
	}
	if locked.Status == types.ProcessingJobStatusSucceeded {
		asset, err := selectAssetForUpdate(ctx, tx, locked.TenantID, locked.AssetID)
		if err != nil {
			return types.MediaAsset{}, types.NewDBReadFailed(err.Error())
		}
		if err := tx.Commit(ctx); err != nil {
			return types.MediaAsset{}, types.NewDBWriteFailed(err.Error())
		}
		return asset, nil
	}
	if locked.Status != types.ProcessingJobStatusRunning {
		return types.MediaAsset{}, types.NewFailedPrecondition("processing job is not running")
	}
	if _, err := tx.Exec(ctx, `
UPDATE media_processing_jobs
SET status = 'SUCCEEDED',
    last_error = '',
    updated_at = now()
WHERE tenant_id = $1
  AND job_id = $2
`, locked.TenantID, locked.JobID); err != nil {
		return types.MediaAsset{}, types.NewDBWriteFailed(err.Error())
	}
	asset, err := updateAssetProcessingStatus(ctx, tx, locked, types.ProcessingStatusPassed)
	if err != nil {
		return types.MediaAsset{}, types.NewDBWriteFailed(err.Error())
	}
	asset, err = maybeMarkAssetReady(ctx, tx, asset)
	if err != nil {
		return types.MediaAsset{}, types.NewDBWriteFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return types.MediaAsset{}, types.NewDBWriteFailed(err.Error())
	}
	return asset, nil
}

func (repository *Repository) MarkProcessingJobFailed(
	ctx context.Context,
	job types.ProcessingJob,
	cause error,
	maxAttempts int,
	retryDelay time.Duration,
) (bool, error) {
	if repository.pool == nil {
		return false, types.NewDBWriteFailed("media repository is not configured")
	}
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	if retryDelay <= 0 {
		retryDelay = time.Second
	}
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, types.NewDBWriteFailed(err.Error())
	}
	defer rollback(ctx, tx)

	locked, err := lockProcessingJob(ctx, tx, job.TenantID, job.JobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, types.NewFailedPrecondition("processing job not found")
		}
		return false, types.NewDBReadFailed(err.Error())
	}
	if locked.Status != types.ProcessingJobStatusRunning {
		return false, types.NewFailedPrecondition("processing job is not running")
	}
	publicError := sanitizeProcessingError(cause)
	deadLettered := locked.AttemptCount >= maxAttempts
	if deadLettered {
		_, err = tx.Exec(ctx, `
UPDATE media_processing_jobs
SET status = 'DLQ',
    last_error = $3,
    dead_lettered_at = now(),
    updated_at = now()
WHERE tenant_id = $1
  AND job_id = $2
`, locked.TenantID, locked.JobID, publicError)
	} else {
		_, err = tx.Exec(ctx, `
UPDATE media_processing_jobs
SET status = 'FAILED',
    next_retry_at = $3,
    last_error = $4,
    updated_at = now()
WHERE tenant_id = $1
  AND job_id = $2
`, locked.TenantID, locked.JobID, time.Now().Add(retryDelay), publicError)
	}
	if err != nil {
		return false, types.NewDBWriteFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return false, types.NewDBWriteFailed(err.Error())
	}
	return deadLettered, nil
}

func selectUploadSessionByIdempotency(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	userID types.UserID,
	idempotencyKey string,
) (types.MediaAsset, types.UploadSession, error) {
	row := tx.QueryRow(ctx, selectAssetAndSessionSQL()+`
WHERE s.tenant_id = $1
  AND s.owner_user_id = $2
  AND s.idempotency_key = $3
FOR UPDATE OF s
`, tenantID, userID, strings.TrimSpace(idempotencyKey))
	return scanAssetAndSession(row)
}

func selectUploadSessionForComplete(
	ctx context.Context,
	tx pgx.Tx,
	command types.CompleteUploadCommand,
) (types.MediaAsset, types.UploadSession, error) {
	row := tx.QueryRow(ctx, selectAssetAndSessionSQL()+`
WHERE s.tenant_id = $1
  AND s.upload_session_id = $2
  AND s.asset_id = $3
FOR UPDATE OF s, a
`, command.AuthContext.TenantID, command.UploadSessionID, command.AssetID)
	return scanAssetAndSession(row)
}

func insertMediaAsset(
	ctx context.Context,
	tx pgx.Tx,
	command types.CreateUploadSessionCommand,
	allocation app.UploadAllocation,
) (types.MediaAsset, error) {
	row := tx.QueryRow(ctx, `
INSERT INTO media_assets (
	tenant_id,
	asset_id,
	owner_user_id,
	conversation_id,
	media_kind,
	content_type,
	file_name,
	size_bytes,
	sha256,
	object_key,
	status,
	scan_status,
	thumbnail_status,
	transcode_status,
	created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'UPLOAD_PENDING', 'PENDING', 'PENDING', 'PENDING', now())
RETURNING
	tenant_id,
	asset_id,
	owner_user_id,
	conversation_id,
	media_kind,
	content_type,
	file_name,
	size_bytes,
	sha256,
	object_key,
	status,
	scan_status,
	thumbnail_status,
	transcode_status,
	created_at,
	uploaded_at,
	ready_at,
	deleted_at
`, command.AuthContext.TenantID,
		allocation.AssetID,
		command.AuthContext.UserID,
		command.ConversationID,
		command.MediaKind,
		command.ContentType,
		command.FileName,
		command.SizeBytes,
		command.SHA256,
		allocation.ObjectKey,
	)
	return scanAsset(row)
}

func insertUploadSession(
	ctx context.Context,
	tx pgx.Tx,
	command types.CreateUploadSessionCommand,
	allocation app.UploadAllocation,
) (types.UploadSession, error) {
	row := tx.QueryRow(ctx, `
INSERT INTO media_upload_sessions (
	tenant_id,
	upload_session_id,
	asset_id,
	owner_user_id,
	idempotency_key,
	command_hash,
	status,
	expires_at,
	created_at
) VALUES ($1, $2, $3, $4, $5, $6, 'PENDING', $7, now())
RETURNING
	tenant_id,
	upload_session_id,
	asset_id,
	owner_user_id,
	idempotency_key,
	command_hash,
	status,
	expires_at,
	completed_at,
	created_at
`, command.AuthContext.TenantID,
		allocation.UploadSessionID,
		allocation.AssetID,
		command.AuthContext.UserID,
		command.IdempotencyKey,
		command.CommandHash(),
		allocation.ExpiresAt,
	)
	return scanSession(row)
}

func insertMediaEvent(
	ctx context.Context,
	tx pgx.Tx,
	asset types.MediaAsset,
	eventType string,
	eventID string,
	status string,
) error {
	payload := map[string]any{
		"tenant_id":       string(asset.TenantID),
		"asset_id":        asset.AssetID,
		"conversation_id": asset.ConversationID,
		"media_kind":      asset.MediaKind,
		"content_type":    asset.ContentType,
		"size_bytes":      asset.SizeBytes,
		"sha256":          asset.SHA256,
		"status":          status,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
INSERT INTO media_outbox (
	event_id,
	tenant_id,
	asset_id,
	event_type,
	event_version,
	partition_key,
	payload_json,
	status,
	created_at,
	updated_at
) VALUES ($1, $2, $3, $4, 1, $5, $6::jsonb, 'PENDING', now(), now())
ON CONFLICT (tenant_id, event_id) DO NOTHING
`, eventID, asset.TenantID, asset.AssetID, eventType, fmt.Sprintf("%s:%s", asset.TenantID, asset.AssetID), string(payloadJSON))
	return err
}

func initialProcessingStatuses(mediaKind string) (scanStatus string, thumbnailStatus string, transcodeStatus string) {
	scanStatus = types.ProcessingStatusPending
	thumbnailStatus = types.ProcessingStatusSkipped
	transcodeStatus = types.ProcessingStatusSkipped
	switch mediaKind {
	case types.MediaKindImage:
		thumbnailStatus = types.ProcessingStatusPending
	case types.MediaKindVideo:
		thumbnailStatus = types.ProcessingStatusPending
		transcodeStatus = types.ProcessingStatusPending
	case types.MediaKindVoice:
		transcodeStatus = types.ProcessingStatusPending
	}
	return scanStatus, thumbnailStatus, transcodeStatus
}

func insertProcessingJobs(ctx context.Context, tx pgx.Tx, asset types.MediaAsset) error {
	jobTypes := []string{types.ProcessingJobTypeScan}
	if asset.ThumbnailStatus == types.ProcessingStatusPending {
		jobTypes = append(jobTypes, types.ProcessingJobTypeThumbnail)
	}
	if asset.TranscodeStatus == types.ProcessingStatusPending {
		jobTypes = append(jobTypes, types.ProcessingJobTypeTranscode)
	}
	for _, jobType := range jobTypes {
		jobID := fmt.Sprintf("%s-%s-v1", asset.AssetID, strings.ToLower(jobType))
		if _, err := tx.Exec(ctx, `
INSERT INTO media_processing_jobs (
	tenant_id,
	job_id,
	asset_id,
	job_type,
	status,
	created_at,
	updated_at
) VALUES ($1, $2, $3, $4, 'PENDING', now(), now())
ON CONFLICT (tenant_id, job_id) DO NOTHING
`, asset.TenantID, jobID, asset.AssetID, jobType); err != nil {
			return err
		}
	}
	return nil
}

func lockProcessingJob(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, jobID string) (types.ProcessingJob, error) {
	row := tx.QueryRow(ctx, `
SELECT
	tenant_id,
	job_id,
	asset_id,
	job_type,
	status,
	attempt_count,
	next_retry_at,
	last_error,
	dead_lettered_at,
	created_at,
	updated_at
FROM media_processing_jobs
WHERE tenant_id = $1
  AND job_id = $2
FOR UPDATE
`, tenantID, strings.TrimSpace(jobID))
	return scanProcessingJobOnly(row)
}

func selectAssetForUpdate(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, assetID string) (types.MediaAsset, error) {
	row := tx.QueryRow(ctx, selectAssetSQL()+`
WHERE tenant_id = $1
  AND asset_id = $2
FOR UPDATE
`, tenantID, strings.TrimSpace(assetID))
	return scanAsset(row)
}

func updateAssetProcessingStatus(
	ctx context.Context,
	tx pgx.Tx,
	job types.ProcessingJob,
	status string,
) (types.MediaAsset, error) {
	var setClause string
	switch job.JobType {
	case types.ProcessingJobTypeScan:
		setClause = "scan_status = $3"
	case types.ProcessingJobTypeThumbnail:
		setClause = "thumbnail_status = $3"
	case types.ProcessingJobTypeTranscode:
		setClause = "transcode_status = $3"
	default:
		return types.MediaAsset{}, fmt.Errorf("unsupported processing job type %q", job.JobType)
	}
	row := tx.QueryRow(ctx, `
UPDATE media_assets
SET `+setClause+`
WHERE tenant_id = $1
  AND asset_id = $2
RETURNING
	tenant_id,
	asset_id,
	owner_user_id,
	conversation_id,
	media_kind,
	content_type,
	file_name,
	size_bytes,
	sha256,
	object_key,
	status,
	scan_status,
	thumbnail_status,
	transcode_status,
	created_at,
	uploaded_at,
	ready_at,
	deleted_at
`, job.TenantID, job.AssetID, status)
	return scanAsset(row)
}

func maybeMarkAssetReady(ctx context.Context, tx pgx.Tx, asset types.MediaAsset) (types.MediaAsset, error) {
	if asset.Status != types.AssetStatusProcessing ||
		!isProcessingTerminal(asset.ScanStatus) ||
		!isProcessingTerminal(asset.ThumbnailStatus) ||
		!isProcessingTerminal(asset.TranscodeStatus) {
		return asset, nil
	}
	row := tx.QueryRow(ctx, `
UPDATE media_assets
SET status = 'READY',
    ready_at = COALESCE(ready_at, now())
WHERE tenant_id = $1
  AND asset_id = $2
RETURNING
	tenant_id,
	asset_id,
	owner_user_id,
	conversation_id,
	media_kind,
	content_type,
	file_name,
	size_bytes,
	sha256,
	object_key,
	status,
	scan_status,
	thumbnail_status,
	transcode_status,
	created_at,
	uploaded_at,
	ready_at,
	deleted_at
`, asset.TenantID, asset.AssetID)
	updated, err := scanAsset(row)
	if err != nil {
		return types.MediaAsset{}, err
	}
	if err := insertMediaEvent(ctx, tx, updated, "media.asset.ready.v1", updated.AssetID+"-ready-v1", "READY"); err != nil {
		return types.MediaAsset{}, err
	}
	return updated, nil
}

func isProcessingTerminal(status string) bool {
	return status == types.ProcessingStatusPassed || status == types.ProcessingStatusSkipped
}

func sanitizeProcessingError(err error) string {
	if err == nil {
		return "media processing failed"
	}
	return "media processing failed"
}

func selectAssetSQL() string {
	return `
SELECT
	tenant_id,
	asset_id,
	owner_user_id,
	conversation_id,
	media_kind,
	content_type,
	file_name,
	size_bytes,
	sha256,
	object_key,
	status,
	scan_status,
	thumbnail_status,
	transcode_status,
	created_at,
	uploaded_at,
	ready_at,
	deleted_at
FROM media_assets
`
}

func selectAssetAndSessionSQL() string {
	return `
SELECT
	a.tenant_id,
	a.asset_id,
	a.owner_user_id,
	a.conversation_id,
	a.media_kind,
	a.content_type,
	a.file_name,
	a.size_bytes,
	a.sha256,
	a.object_key,
	a.status,
	a.scan_status,
	a.thumbnail_status,
	a.transcode_status,
	a.created_at,
	a.uploaded_at,
	a.ready_at,
	a.deleted_at,
	s.tenant_id,
	s.upload_session_id,
	s.asset_id,
	s.owner_user_id,
	s.idempotency_key,
	s.command_hash,
	s.status,
	s.expires_at,
	s.completed_at,
	s.created_at
FROM media_upload_sessions s
JOIN media_assets a
  ON a.tenant_id = s.tenant_id
 AND a.asset_id = s.asset_id
`
}

type scanner interface {
	Scan(dest ...any) error
}

func scanAssetAndSession(row scanner) (types.MediaAsset, types.UploadSession, error) {
	var asset types.MediaAsset
	var session types.UploadSession
	var uploadedAt sql.NullTime
	var readyAt sql.NullTime
	var deletedAt sql.NullTime
	var completedAt sql.NullTime
	if err := row.Scan(
		&asset.TenantID,
		&asset.AssetID,
		&asset.OwnerUserID,
		&asset.ConversationID,
		&asset.MediaKind,
		&asset.ContentType,
		&asset.FileName,
		&asset.SizeBytes,
		&asset.SHA256,
		&asset.ObjectKey,
		&asset.Status,
		&asset.ScanStatus,
		&asset.ThumbnailStatus,
		&asset.TranscodeStatus,
		&asset.CreatedAt,
		&uploadedAt,
		&readyAt,
		&deletedAt,
		&session.TenantID,
		&session.UploadSessionID,
		&session.AssetID,
		&session.OwnerUserID,
		&session.IdempotencyKey,
		&session.CommandHash,
		&session.Status,
		&session.ExpiresAt,
		&completedAt,
		&session.CreatedAt,
	); err != nil {
		return types.MediaAsset{}, types.UploadSession{}, err
	}
	asset.UploadedAt = nullTime(uploadedAt)
	asset.ReadyAt = nullTime(readyAt)
	asset.DeletedAt = nullTime(deletedAt)
	session.CompletedAt = nullTime(completedAt)
	return asset, session, nil
}

func scanAsset(row scanner) (types.MediaAsset, error) {
	var asset types.MediaAsset
	var uploadedAt sql.NullTime
	var readyAt sql.NullTime
	var deletedAt sql.NullTime
	if err := row.Scan(
		&asset.TenantID,
		&asset.AssetID,
		&asset.OwnerUserID,
		&asset.ConversationID,
		&asset.MediaKind,
		&asset.ContentType,
		&asset.FileName,
		&asset.SizeBytes,
		&asset.SHA256,
		&asset.ObjectKey,
		&asset.Status,
		&asset.ScanStatus,
		&asset.ThumbnailStatus,
		&asset.TranscodeStatus,
		&asset.CreatedAt,
		&uploadedAt,
		&readyAt,
		&deletedAt,
	); err != nil {
		return types.MediaAsset{}, err
	}
	asset.UploadedAt = nullTime(uploadedAt)
	asset.ReadyAt = nullTime(readyAt)
	asset.DeletedAt = nullTime(deletedAt)
	return asset, nil
}

func scanProcessingJob(row scanner) (types.ProcessingJob, error) {
	var job types.ProcessingJob
	var asset types.MediaAsset
	var nextRetryAt sql.NullTime
	var deadLetteredAt sql.NullTime
	var uploadedAt sql.NullTime
	var readyAt sql.NullTime
	var deletedAt sql.NullTime
	if err := row.Scan(
		&job.TenantID,
		&job.JobID,
		&job.AssetID,
		&job.JobType,
		&job.Status,
		&job.AttemptCount,
		&nextRetryAt,
		&job.LastError,
		&deadLetteredAt,
		&job.CreatedAt,
		&job.UpdatedAt,
		&asset.TenantID,
		&asset.AssetID,
		&asset.OwnerUserID,
		&asset.ConversationID,
		&asset.MediaKind,
		&asset.ContentType,
		&asset.FileName,
		&asset.SizeBytes,
		&asset.SHA256,
		&asset.ObjectKey,
		&asset.Status,
		&asset.ScanStatus,
		&asset.ThumbnailStatus,
		&asset.TranscodeStatus,
		&asset.CreatedAt,
		&uploadedAt,
		&readyAt,
		&deletedAt,
	); err != nil {
		return types.ProcessingJob{}, err
	}
	job.NextRetryAt = nullTime(nextRetryAt)
	job.DeadLetteredAt = nullTime(deadLetteredAt)
	asset.UploadedAt = nullTime(uploadedAt)
	asset.ReadyAt = nullTime(readyAt)
	asset.DeletedAt = nullTime(deletedAt)
	job.Asset = asset
	return job, nil
}

func scanProcessingJobOnly(row scanner) (types.ProcessingJob, error) {
	var job types.ProcessingJob
	var nextRetryAt sql.NullTime
	var deadLetteredAt sql.NullTime
	if err := row.Scan(
		&job.TenantID,
		&job.JobID,
		&job.AssetID,
		&job.JobType,
		&job.Status,
		&job.AttemptCount,
		&nextRetryAt,
		&job.LastError,
		&deadLetteredAt,
		&job.CreatedAt,
		&job.UpdatedAt,
	); err != nil {
		return types.ProcessingJob{}, err
	}
	job.NextRetryAt = nullTime(nextRetryAt)
	job.DeadLetteredAt = nullTime(deadLetteredAt)
	return job, nil
}

func scanSession(row scanner) (types.UploadSession, error) {
	var session types.UploadSession
	var completedAt sql.NullTime
	if err := row.Scan(
		&session.TenantID,
		&session.UploadSessionID,
		&session.AssetID,
		&session.OwnerUserID,
		&session.IdempotencyKey,
		&session.CommandHash,
		&session.Status,
		&session.ExpiresAt,
		&completedAt,
		&session.CreatedAt,
	); err != nil {
		return types.UploadSession{}, err
	}
	session.CompletedAt = nullTime(completedAt)
	return session, nil
}

func nullTime(value sql.NullTime) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func rollback(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(ctx)
}
