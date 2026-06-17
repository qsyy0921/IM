package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

func upsertAcceptedContactEdges(ctx context.Context, tx pgx.Tx, request contactRequestRow) (int64, error) {
	firstVersion, err := upsertContactEdge(ctx, tx, request.TenantID, request.SenderUserID, request.ReceiverUserID, request.RequestID)
	if err != nil {
		return 0, err
	}
	secondVersion, err := upsertContactEdge(ctx, tx, request.TenantID, request.ReceiverUserID, request.SenderUserID, request.RequestID)
	if err != nil {
		return 0, err
	}
	if secondVersion > firstVersion {
		return secondVersion, nil
	}
	return firstVersion, nil
}

func upsertContactEdge(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	ownerUserID types.UserID,
	contactUserID types.UserID,
	requestID string,
) (int64, error) {
	var version int64
	err := tx.QueryRow(ctx, `
INSERT INTO contact_edges (
    tenant_id,
    owner_user_id,
    contact_user_id,
    status,
    source_request_id,
    version,
    created_at,
    updated_at
) VALUES ($1, $2, $3, 'ACTIVE', $4, 1, now(), now())
ON CONFLICT (tenant_id, owner_user_id, contact_user_id) DO UPDATE
SET status = 'ACTIVE',
    source_request_id = EXCLUDED.source_request_id,
    version = contact_edges.version + 1,
    updated_at = now()
RETURNING version
`, tenantID, ownerUserID, contactUserID, requestID).Scan(&version)
	if err != nil {
		return 0, types.NewDBWriteFailed(err.Error())
	}
	return version, nil
}

type contactEdgeRow struct {
	TenantID        types.TenantID
	OwnerUserID     types.UserID
	ContactUserID   types.UserID
	Status          types.ContactEdgeStatus
	SourceRequestID string
	Version         int64
	Remark          string
	GroupName       string
}

func getContactEdge(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	ownerUserID types.UserID,
	contactUserID types.UserID,
) (contactEdgeRow, error) {
	return scanContactEdge(ctx, tx, `
SELECT tenant_id, owner_user_id, contact_user_id, status, source_request_id, version, remark, group_name
FROM contact_edges
WHERE tenant_id = $1
  AND owner_user_id = $2
  AND contact_user_id = $3
`, tenantID, ownerUserID, contactUserID)
}

func lockContactEdge(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	ownerUserID types.UserID,
	contactUserID types.UserID,
) (contactEdgeRow, error) {
	return scanContactEdge(ctx, tx, `
SELECT tenant_id, owner_user_id, contact_user_id, status, source_request_id, version, remark, group_name
FROM contact_edges
WHERE tenant_id = $1
  AND owner_user_id = $2
  AND contact_user_id = $3
FOR UPDATE
`, tenantID, ownerUserID, contactUserID)
}

func scanContactEdge(
	ctx context.Context,
	tx pgx.Tx,
	query string,
	tenantID types.TenantID,
	ownerUserID types.UserID,
	contactUserID types.UserID,
) (contactEdgeRow, error) {
	var row contactEdgeRow
	err := tx.QueryRow(ctx, query, tenantID, ownerUserID, contactUserID).Scan(
		&row.TenantID,
		&row.OwnerUserID,
		&row.ContactUserID,
		&row.Status,
		&row.SourceRequestID,
		&row.Version,
		&row.Remark,
		&row.GroupName,
	)
	if err == pgx.ErrNoRows {
		return contactEdgeRow{}, types.NewContactNotFound("contact edge not found")
	}
	if err != nil {
		return contactEdgeRow{}, types.NewDBReadFailed(err.Error())
	}
	return row, nil
}

func updateContactEdgeStatus(
	ctx context.Context,
	tx pgx.Tx,
	row contactEdgeRow,
	status types.ContactEdgeStatus,
) (contactEdgeRow, error) {
	var updated contactEdgeRow
	err := tx.QueryRow(ctx, `
UPDATE contact_edges
SET status = $4,
    version = version + 1,
    updated_at = now()
WHERE tenant_id = $1
  AND owner_user_id = $2
  AND contact_user_id = $3
RETURNING tenant_id, owner_user_id, contact_user_id, status, source_request_id, version, remark, group_name
`, row.TenantID, row.OwnerUserID, row.ContactUserID, status).Scan(
		&updated.TenantID,
		&updated.OwnerUserID,
		&updated.ContactUserID,
		&updated.Status,
		&updated.SourceRequestID,
		&updated.Version,
		&updated.Remark,
		&updated.GroupName,
	)
	if err != nil {
		return contactEdgeRow{}, types.NewDBWriteFailed(err.Error())
	}
	return updated, nil
}

func updateContactEdgeRemark(
	ctx context.Context,
	tx pgx.Tx,
	row contactEdgeRow,
	remark string,
) (contactEdgeRow, error) {
	var updated contactEdgeRow
	err := tx.QueryRow(ctx, `
UPDATE contact_edges
SET remark = $4,
    version = version + 1,
    updated_at = now()
WHERE tenant_id = $1
  AND owner_user_id = $2
  AND contact_user_id = $3
RETURNING tenant_id, owner_user_id, contact_user_id, status, source_request_id, version, remark, group_name
`, row.TenantID, row.OwnerUserID, row.ContactUserID, remark).Scan(
		&updated.TenantID,
		&updated.OwnerUserID,
		&updated.ContactUserID,
		&updated.Status,
		&updated.SourceRequestID,
		&updated.Version,
		&updated.Remark,
		&updated.GroupName,
	)
	if err != nil {
		return contactEdgeRow{}, types.NewDBWriteFailed(err.Error())
	}
	return updated, nil
}

func updateContactEdgeGroup(
	ctx context.Context,
	tx pgx.Tx,
	row contactEdgeRow,
	groupName string,
) (contactEdgeRow, error) {
	var updated contactEdgeRow
	err := tx.QueryRow(ctx, `
UPDATE contact_edges
SET group_name = $4,
    version = version + 1,
    updated_at = now()
WHERE tenant_id = $1
  AND owner_user_id = $2
  AND contact_user_id = $3
RETURNING tenant_id, owner_user_id, contact_user_id, status, source_request_id, version, remark, group_name
`, row.TenantID, row.OwnerUserID, row.ContactUserID, groupName).Scan(
		&updated.TenantID,
		&updated.OwnerUserID,
		&updated.ContactUserID,
		&updated.Status,
		&updated.SourceRequestID,
		&updated.Version,
		&updated.Remark,
		&updated.GroupName,
	)
	if err != nil {
		return contactEdgeRow{}, types.NewDBWriteFailed(err.Error())
	}
	return updated, nil
}
