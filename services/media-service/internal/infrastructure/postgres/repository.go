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
	if session.Status == "COMPLETED" && asset.Status == types.AssetStatusReady {
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

	row := tx.QueryRow(ctx, `
UPDATE media_assets
SET status = 'READY',
    scan_status = 'PASSED',
    thumbnail_status = 'SKIPPED',
    transcode_status = 'SKIPPED',
    uploaded_at = COALESCE(uploaded_at, now()),
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
`, command.AuthContext.TenantID, command.AssetID)
	updatedAsset, err := scanAsset(row)
	if err != nil {
		return types.MediaAsset{}, types.NewDBWriteFailed(err.Error())
	}
	if err := insertMediaEvent(ctx, tx, updatedAsset, "media.asset.uploaded.v1", updatedAsset.AssetID+"-uploaded-v1", "UPLOADED"); err != nil {
		return types.MediaAsset{}, types.NewDBWriteFailed(err.Error())
	}
	if err := insertMediaEvent(ctx, tx, updatedAsset, "media.asset.ready.v1", updatedAsset.AssetID+"-ready-v1", "READY"); err != nil {
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
