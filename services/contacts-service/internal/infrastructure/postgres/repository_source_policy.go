package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

func (r *Repository) GetTenantContactRequestSourcePolicy(
	ctx context.Context,
	command types.GetTenantContactRequestSourcePolicyCommand,
) (types.GetTenantContactRequestSourcePolicyResult, error) {
	if r.pool == nil {
		return types.GetTenantContactRequestSourcePolicyResult{}, types.NewDBReadFailed("contacts repository is not configured")
	}
	row, err := getTenantContactRequestSourcePolicy(ctx, r.pool, command.TenantID, command.NormalizedSourceType())
	if err != nil {
		return types.GetTenantContactRequestSourcePolicyResult{}, err
	}
	return contactRequestSourcePolicyResultFromRow(row), nil
}

func (r *Repository) SetTenantContactRequestSourcePolicy(
	ctx context.Context,
	command types.SetTenantContactRequestSourcePolicyCommand,
) (types.SetTenantContactRequestSourcePolicyResult, error) {
	if r.pool == nil {
		return types.SetTenantContactRequestSourcePolicyResult{}, types.NewDBWriteFailed("contacts repository is not configured")
	}
	sourceType := command.NormalizedSourceType()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.SetTenantContactRequestSourcePolicyResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if err := lockTenantContactRequestSourcePolicy(ctx, tx, command.TenantID, sourceType); err != nil {
		return types.SetTenantContactRequestSourcePolicyResult{}, err
	}
	row, changed, err := upsertTenantContactRequestSourcePolicy(ctx, tx, command.TenantID, sourceType, command.AllowContactRequests)
	if err != nil {
		return types.SetTenantContactRequestSourcePolicyResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.SetTenantContactRequestSourcePolicyResult{}, types.NewDBWriteFailed(err.Error())
	}
	return setContactRequestSourcePolicyResultFromRow(row, changed), nil
}

type contactRequestSourcePolicyRow struct {
	TenantID             types.TenantID
	SourceType           types.ContactRequestSourceType
	AllowContactRequests bool
	Version              int64
	UpdatedAt            time.Time
}

func defaultContactRequestSourcePolicyRow(tenantID types.TenantID, sourceType types.ContactRequestSourceType) contactRequestSourcePolicyRow {
	return contactRequestSourcePolicyRow{
		TenantID:             tenantID,
		SourceType:           sourceType,
		AllowContactRequests: true,
	}
}

func getTenantContactRequestSourcePolicy(
	ctx context.Context,
	queryer interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	tenantID types.TenantID,
	sourceType types.ContactRequestSourceType,
) (contactRequestSourcePolicyRow, error) {
	var row contactRequestSourcePolicyRow
	err := queryer.QueryRow(ctx, `
SELECT tenant_id, source_type, allow_contact_requests, version, updated_at
FROM contact_tenant_request_source_policies
WHERE tenant_id = $1
  AND source_type = $2
`, tenantID, sourceType).Scan(&row.TenantID, &row.SourceType, &row.AllowContactRequests, &row.Version, &row.UpdatedAt)
	if err == pgx.ErrNoRows {
		return defaultContactRequestSourcePolicyRow(tenantID, sourceType), nil
	}
	if err != nil {
		return contactRequestSourcePolicyRow{}, types.NewDBReadFailed(err.Error())
	}
	return row, nil
}

func contactRequestSourceAllowed(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, sourceType types.ContactRequestSourceType) (bool, error) {
	row, err := getTenantContactRequestSourcePolicy(ctx, tx, tenantID, sourceType)
	if err != nil {
		return false, err
	}
	return row.AllowContactRequests, nil
}

func lockTenantContactRequestSourcePolicy(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, sourceType types.ContactRequestSourceType) error {
	key := fmt.Sprintf("%s\x1f%s\x1fcontacts_source_policy", tenantID, sourceType)
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func upsertTenantContactRequestSourcePolicy(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	sourceType types.ContactRequestSourceType,
	allowContactRequests bool,
) (contactRequestSourcePolicyRow, bool, error) {
	current, err := getTenantContactRequestSourcePolicy(ctx, tx, tenantID, sourceType)
	if err != nil {
		return contactRequestSourcePolicyRow{}, false, err
	}
	if current.Version > 0 && current.AllowContactRequests == allowContactRequests {
		return current, false, nil
	}
	var row contactRequestSourcePolicyRow
	if current.Version == 0 {
		err = tx.QueryRow(ctx, `
INSERT INTO contact_tenant_request_source_policies (
    tenant_id,
    source_type,
    allow_contact_requests,
    version,
    created_at,
    updated_at
) VALUES ($1, $2, $3, 1, now(), now())
RETURNING tenant_id, source_type, allow_contact_requests, version, updated_at
`, tenantID, sourceType, allowContactRequests).Scan(&row.TenantID, &row.SourceType, &row.AllowContactRequests, &row.Version, &row.UpdatedAt)
	} else {
		err = tx.QueryRow(ctx, `
UPDATE contact_tenant_request_source_policies
SET allow_contact_requests = $3,
    version = version + 1,
    updated_at = now()
WHERE tenant_id = $1
  AND source_type = $2
RETURNING tenant_id, source_type, allow_contact_requests, version, updated_at
`, tenantID, sourceType, allowContactRequests).Scan(&row.TenantID, &row.SourceType, &row.AllowContactRequests, &row.Version, &row.UpdatedAt)
	}
	if err != nil {
		return contactRequestSourcePolicyRow{}, false, types.NewDBWriteFailed(err.Error())
	}
	return row, true, nil
}

func contactRequestSourcePolicyResultFromRow(row contactRequestSourcePolicyRow) types.GetTenantContactRequestSourcePolicyResult {
	return types.GetTenantContactRequestSourcePolicyResult{
		TenantID: row.TenantID,
		Policy:   contactRequestSourcePolicyFromRow(row),
	}
}

func setContactRequestSourcePolicyResultFromRow(row contactRequestSourcePolicyRow, changed bool) types.SetTenantContactRequestSourcePolicyResult {
	return types.SetTenantContactRequestSourcePolicyResult{
		TenantID: row.TenantID,
		Policy:   contactRequestSourcePolicyFromRow(row),
		Changed:  changed,
	}
}

func contactRequestSourcePolicyFromRow(row contactRequestSourcePolicyRow) types.ContactRequestSourcePolicy {
	var updatedAtUnixMS int64
	if !row.UpdatedAt.IsZero() {
		updatedAtUnixMS = row.UpdatedAt.UnixMilli()
	}
	return types.ContactRequestSourcePolicy{
		SourceType:           row.SourceType,
		AllowContactRequests: row.AllowContactRequests,
		Version:              row.Version,
		UpdatedAtUnixMS:      updatedAtUnixMS,
	}
}
