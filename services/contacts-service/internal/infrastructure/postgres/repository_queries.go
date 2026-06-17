package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/contacts-service/internal/domain"
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

func (r *Repository) ListContactRequests(
	ctx context.Context,
	command types.ListContactRequestsCommand,
) (types.ListContactRequestsResult, error) {
	if r.pool == nil {
		return types.ListContactRequestsResult{}, types.NewDBReadFailed("contacts repository is not configured")
	}
	direction := command.NormalizedDirection()
	status := command.NormalizedStatus()
	sourceTypeFilter := command.NormalizedSourceTypeFilter()
	riskLevelFilter := command.NormalizedRiskLevelFilter()
	reviewRequiredFilterSet := command.ReviewRequiredFilter != nil
	reviewRequiredFilter := false
	if command.ReviewRequiredFilter != nil {
		reviewRequiredFilter = *command.ReviewRequiredFilter
	}
	limit := domain.NormalizePageSize(command.PageSize)
	cursor, hasCursor, err := decodeContactRequestPageTokenFor(command, direction, status, sourceTypeFilter, riskLevelFilter, command.ReviewRequiredFilter, limit)
	if err != nil {
		return types.ListContactRequestsResult{}, err
	}

	userColumn := "receiver_user_id"
	if direction == types.ContactRequestListDirectionOutgoing {
		userColumn = "sender_user_id"
	}
	args := []any{
		command.AuthContext.TenantID,
		command.AuthContext.UserID,
		status,
		limit + 1,
		sourceTypeFilter,
		riskLevelFilter,
		reviewRequiredFilterSet,
		reviewRequiredFilter,
	}
	query := fmt.Sprintf(`
SELECT
    request_id,
    sender_user_id,
    receiver_user_id,
    status,
    message,
    source_type,
    source_ref,
    risk_level,
    review_required,
    created_at,
    updated_at,
    decided_at IS NOT NULL AS has_decided_at,
    COALESCE(decided_at, 'epoch'::timestamptz) AS decided_at
FROM contact_requests
WHERE tenant_id = $1
  AND %s = $2
  AND status = $3
  AND ($5 = '' OR source_type = $5)
  AND ($6 = '' OR risk_level = $6)
  AND ($7 = false OR review_required = $8)
`, userColumn)
	if hasCursor {
		query += `  AND (created_at < $9 OR (created_at = $9 AND request_id > $10))
`
		args = append(args, cursor.CreatedAt, cursor.RequestID)
	}
	query += `ORDER BY created_at DESC, request_id ASC
LIMIT $4
`
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return types.ListContactRequestsResult{}, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	type listedRequest struct {
		item      types.ContactRequestItem
		createdAt time.Time
	}
	listed := make([]listedRequest, 0, limit)
	for rows.Next() {
		var item types.ContactRequestItem
		var createdAt time.Time
		var updatedAt time.Time
		var decidedAt time.Time
		var hasDecidedAt bool
		if err := rows.Scan(
			&item.RequestID,
			&item.SenderUserID,
			&item.ReceiverUserID,
			&item.Status,
			&item.Message,
			&item.SourceType,
			&item.SourceRef,
			&item.RiskLevel,
			&item.ReviewRequired,
			&createdAt,
			&updatedAt,
			&hasDecidedAt,
			&decidedAt,
		); err != nil {
			return types.ListContactRequestsResult{}, types.NewDBReadFailed(err.Error())
		}
		item.CreatedAtUnixMS = createdAt.UnixMilli()
		item.UpdatedAtUnixMS = updatedAt.UnixMilli()
		if hasDecidedAt {
			item.DecidedAtUnixMS = decidedAt.UnixMilli()
		}
		listed = append(listed, listedRequest{item: item, createdAt: createdAt})
	}
	if err := rows.Err(); err != nil {
		return types.ListContactRequestsResult{}, types.NewDBReadFailed(err.Error())
	}

	nextToken := ""
	if len(listed) > limit {
		last := listed[limit-1]
		nextToken = encodeContactRequestPageToken(contactRequestPageCursor{
			Version:              2,
			TenantID:             command.AuthContext.TenantID,
			UserID:               command.AuthContext.UserID,
			Direction:            direction,
			Status:               status,
			SourceTypeFilter:     sourceTypeFilter,
			RiskLevelFilter:      riskLevelFilter,
			ReviewRequiredFilter: command.ReviewRequiredFilter,
			PageSize:             limit,
			CreatedAt:            last.createdAt,
			RequestID:            last.item.RequestID,
		})
		listed = listed[:limit]
	}
	items := make([]types.ContactRequestItem, 0, len(listed))
	for _, row := range listed {
		items = append(items, row.item)
	}
	return types.ListContactRequestsResult{
		TenantID:      command.AuthContext.TenantID,
		UserID:        command.AuthContext.UserID,
		Direction:     direction,
		Status:        status,
		Requests:      items,
		NextPageToken: nextToken,
	}, nil
}

func (r *Repository) ListContacts(
	ctx context.Context,
	command types.ListContactsCommand,
) (types.ListContactsResult, error) {
	if r.pool == nil {
		return types.ListContactsResult{}, types.NewDBReadFailed("contacts repository is not configured")
	}
	limit := domain.NormalizePageSize(command.PageSize)
	cursor, hasCursor, err := decodePageTokenFor(command, limit)
	if err != nil {
		return types.ListContactsResult{}, err
	}
	args := []any{command.AuthContext.TenantID, command.AuthContext.UserID, limit + 1}
	searchQuery := command.NormalizedQuery()
	groupName := command.NormalizedGroupName()
	query := `
SELECT
    contact_user_id,
    status,
    version,
    source_request_id,
    remark,
    group_name,
    created_at,
    updated_at
FROM contact_edges
WHERE tenant_id = $1
  AND owner_user_id = $2
  AND status = 'ACTIVE'
`
	if searchQuery != "" {
		args = append(args, likePatternForSearchQuery(searchQuery))
		query += fmt.Sprintf(`  AND (contact_user_id ILIKE $%d ESCAPE '\' OR remark ILIKE $%d ESCAPE '\' OR group_name ILIKE $%d ESCAPE '\')
`, len(args), len(args), len(args))
	}
	if groupName != "" {
		args = append(args, groupName)
		query += fmt.Sprintf(`  AND group_name = $%d
`, len(args))
	}
	if hasCursor {
		args = append(args, cursor.ContactUserID)
		query += fmt.Sprintf(`  AND contact_user_id > $%d
`, len(args))
	}
	query += `ORDER BY contact_user_id ASC
LIMIT $3
`
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return types.ListContactsResult{}, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()
	items := make([]types.ContactItem, 0, limit)
	for rows.Next() {
		var item types.ContactItem
		var createdAt time.Time
		var updatedAt time.Time
		if err := rows.Scan(&item.ContactUserID, &item.Status, &item.Version, &item.SourceRequestID, &item.Remark, &item.GroupName, &createdAt, &updatedAt); err != nil {
			return types.ListContactsResult{}, types.NewDBReadFailed(err.Error())
		}
		item.CreatedAtUnixMS = createdAt.UnixMilli()
		item.UpdatedAtUnixMS = updatedAt.UnixMilli()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return types.ListContactsResult{}, types.NewDBReadFailed(err.Error())
	}
	nextToken := ""
	if len(items) > limit {
		last := items[limit-1]
		nextToken = encodePageToken(contactPageCursor{
			Version:       1,
			TenantID:      command.AuthContext.TenantID,
			OwnerUserID:   command.AuthContext.UserID,
			PageSize:      limit,
			Query:         searchQuery,
			GroupName:     groupName,
			ContactUserID: string(last.ContactUserID),
		})
		items = items[:limit]
	}
	return types.ListContactsResult{
		TenantID:      command.AuthContext.TenantID,
		OwnerUserID:   command.AuthContext.UserID,
		Contacts:      items,
		NextPageToken: nextToken,
	}, nil
}

func (r *Repository) GetContactState(
	ctx context.Context,
	command types.GetContactStateCommand,
) (types.GetContactStateResult, error) {
	if r.pool == nil {
		return types.GetContactStateResult{}, types.NewDBReadFailed("contacts repository is not configured")
	}
	var result types.GetContactStateResult
	err := r.pool.QueryRow(ctx, `
SELECT
    tenant_id,
    owner_user_id,
    contact_user_id,
    status,
    source_request_id,
    version,
    remark,
    group_name
FROM contact_edges
WHERE tenant_id = $1
  AND owner_user_id = $2
  AND contact_user_id = $3
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.OtherUserID).Scan(
		&result.TenantID,
		&result.OwnerUserID,
		&result.ContactUserID,
		&result.Status,
		&result.SourceRequestID,
		&result.Version,
		&result.Remark,
		&result.GroupName,
	)
	if err == pgx.ErrNoRows {
		return types.GetContactStateResult{}, types.NewContactRequestNotFound("contact state not found")
	}
	if err != nil {
		return types.GetContactStateResult{}, types.NewDBReadFailed(err.Error())
	}
	return result, nil
}
