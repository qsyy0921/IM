package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/qsyy0921/IM/services/contacts-service/internal/domain"
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

func (r *Repository) SetContactPrivacyException(
	ctx context.Context,
	command types.SetContactPrivacyExceptionCommand,
) (types.SetContactPrivacyExceptionResult, error) {
	if r.pool == nil {
		return types.SetContactPrivacyExceptionResult{}, types.NewDBWriteFailed("contacts repository is not configured")
	}
	decision := types.NormalizeContactPrivacyExceptionDecision(command.Decision)
	if decision == "" {
		return types.SetContactPrivacyExceptionResult{}, types.NewInvalidArgument("decision is invalid")
	}
	commandHash, err := commandHash(commandHashPayload{
		Kind:        commandTypeSetPrivacyException,
		TenantID:    string(command.AuthContext.TenantID),
		UserID:      string(command.AuthContext.UserID),
		OtherUserID: string(command.OtherUserID),
		Decision:    string(decision),
	})
	if err != nil {
		return types.SetContactPrivacyExceptionResult{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.SetContactPrivacyExceptionResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if err := lockIdempotencyKey(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey); err != nil {
		return types.SetContactPrivacyExceptionResult{}, err
	}
	if existing, ok, err := findCommandIdempotency(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey); err != nil {
		return types.SetContactPrivacyExceptionResult{}, err
	} else if ok {
		if existing.CommandType != commandTypeSetPrivacyException || existing.CommandHash != commandHash {
			return types.SetContactPrivacyExceptionResult{}, types.NewContactRequestConflict("idempotency key conflict")
		}
		row, err := contactPrivacyExceptionRowFromIdempotencyResult(existing)
		if err != nil {
			return types.SetContactPrivacyExceptionResult{}, err
		}
		return commitSetContactPrivacyExceptionResult(ctx, tx, setContactPrivacyExceptionResultFromRow(row, true))
	}
	if err := lockContactPrivacyException(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.OtherUserID); err != nil {
		return types.SetContactPrivacyExceptionResult{}, err
	}
	row, changed, err := upsertContactPrivacyException(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.OtherUserID, decision)
	if err != nil {
		return types.SetContactPrivacyExceptionResult{}, err
	}
	resultJSON, err := contactPrivacyExceptionResultJSON(row)
	if err != nil {
		return types.SetContactPrivacyExceptionResult{}, err
	}
	if err := insertCommandIdempotencyWithResult(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey, commandTypeSetPrivacyException, commandHash, string(command.OtherUserID), resultJSON); err != nil {
		return types.SetContactPrivacyExceptionResult{}, err
	}
	if changed {
		if err := r.insertPrivacyExceptionOutbox(ctx, tx, privacyExceptionOutboxInput{
			TenantID:      command.AuthContext.TenantID,
			OwnerUserID:   command.AuthContext.UserID,
			OtherUserID:   command.OtherUserID,
			CorrelationID: command.AuthContext.RequestID,
			CausationID:   command.AuthContext.RequestID,
			TraceID:       command.AuthContext.TraceID,
			Exception:     row,
		}); err != nil {
			return types.SetContactPrivacyExceptionResult{}, err
		}
	}
	return commitSetContactPrivacyExceptionResult(ctx, tx, setContactPrivacyExceptionResultFromRow(row, false))
}

func (r *Repository) ListContactPrivacyExceptions(
	ctx context.Context,
	command types.ListContactPrivacyExceptionsCommand,
) (types.ListContactPrivacyExceptionsResult, error) {
	if r.pool == nil {
		return types.ListContactPrivacyExceptionsResult{}, types.NewDBReadFailed("contacts repository is not configured")
	}
	limit := domain.NormalizePageSize(command.PageSize)
	cursor, hasCursor, err := decodeContactPrivacyExceptionPageTokenFor(command, limit)
	if err != nil {
		return types.ListContactPrivacyExceptionsResult{}, err
	}
	args := []any{command.AuthContext.TenantID, command.AuthContext.UserID, limit + 1}
	query := `
SELECT other_user_id, decision, version, updated_at
FROM contact_privacy_exceptions
WHERE tenant_id = $1
  AND owner_user_id = $2
`
	if hasCursor {
		args = append(args, cursor.OtherUserID)
		query += fmt.Sprintf(`  AND other_user_id > $%d
`, len(args))
	}
	query += `ORDER BY other_user_id ASC
LIMIT $3
`
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return types.ListContactPrivacyExceptionsResult{}, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()
	items := make([]types.ContactPrivacyExceptionItem, 0, limit)
	for rows.Next() {
		var item types.ContactPrivacyExceptionItem
		var updatedAt time.Time
		if err := rows.Scan(&item.OtherUserID, &item.Decision, &item.Version, &updatedAt); err != nil {
			return types.ListContactPrivacyExceptionsResult{}, types.NewDBReadFailed(err.Error())
		}
		item.Decision = types.NormalizeContactPrivacyExceptionDecision(item.Decision)
		if item.Decision == "" {
			return types.ListContactPrivacyExceptionsResult{}, types.NewDBReadFailed("contact privacy exception decision is invalid")
		}
		item.UpdatedAtUnixMS = updatedAt.UnixMilli()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return types.ListContactPrivacyExceptionsResult{}, types.NewDBReadFailed(err.Error())
	}
	nextToken := ""
	if len(items) > limit {
		last := items[limit-1]
		nextToken = encodeContactPrivacyExceptionPageToken(contactPrivacyExceptionPageCursor{
			Version:     1,
			TenantID:    command.AuthContext.TenantID,
			OwnerUserID: command.AuthContext.UserID,
			PageSize:    limit,
			OtherUserID: string(last.OtherUserID),
		})
		items = items[:limit]
	}
	return types.ListContactPrivacyExceptionsResult{
		TenantID:      command.AuthContext.TenantID,
		OwnerUserID:   command.AuthContext.UserID,
		Exceptions:    items,
		NextPageToken: nextToken,
	}, nil
}

func (r *Repository) DeleteContactPrivacyException(
	ctx context.Context,
	command types.DeleteContactPrivacyExceptionCommand,
) (types.DeleteContactPrivacyExceptionResult, error) {
	if r.pool == nil {
		return types.DeleteContactPrivacyExceptionResult{}, types.NewDBWriteFailed("contacts repository is not configured")
	}
	commandHash, err := commandHash(commandHashPayload{
		Kind:        commandTypeDeletePrivacyException,
		TenantID:    string(command.AuthContext.TenantID),
		UserID:      string(command.AuthContext.UserID),
		OtherUserID: string(command.OtherUserID),
	})
	if err != nil {
		return types.DeleteContactPrivacyExceptionResult{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.DeleteContactPrivacyExceptionResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if err := lockIdempotencyKey(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey); err != nil {
		return types.DeleteContactPrivacyExceptionResult{}, err
	}
	if existing, ok, err := findCommandIdempotency(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey); err != nil {
		return types.DeleteContactPrivacyExceptionResult{}, err
	} else if ok {
		if existing.CommandType != commandTypeDeletePrivacyException || existing.CommandHash != commandHash {
			return types.DeleteContactPrivacyExceptionResult{}, types.NewContactRequestConflict("idempotency key conflict")
		}
		result, err := deleteContactPrivacyExceptionResultFromIdempotencyResult(existing)
		if err != nil {
			return types.DeleteContactPrivacyExceptionResult{}, err
		}
		result.IdempotentReplay = true
		return commitDeleteContactPrivacyExceptionResult(ctx, tx, result)
	}
	if err := lockContactPrivacyException(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.OtherUserID); err != nil {
		return types.DeleteContactPrivacyExceptionResult{}, err
	}
	row, err := deleteContactPrivacyException(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.OtherUserID)
	if err != nil {
		return types.DeleteContactPrivacyExceptionResult{}, err
	}
	result := types.DeleteContactPrivacyExceptionResult{
		TenantID:    row.TenantID,
		OwnerUserID: row.OwnerUserID,
		OtherUserID: row.OtherUserID,
		Deleted:     true,
	}
	resultJSON, err := deleteContactPrivacyExceptionResultJSON(result)
	if err != nil {
		return types.DeleteContactPrivacyExceptionResult{}, err
	}
	if err := insertCommandIdempotencyWithResult(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey, commandTypeDeletePrivacyException, commandHash, string(command.OtherUserID), resultJSON); err != nil {
		return types.DeleteContactPrivacyExceptionResult{}, err
	}
	if err := r.insertPrivacyExceptionDeletedOutbox(ctx, tx, privacyExceptionOutboxInput{
		TenantID:      command.AuthContext.TenantID,
		OwnerUserID:   command.AuthContext.UserID,
		OtherUserID:   command.OtherUserID,
		CorrelationID: command.AuthContext.RequestID,
		CausationID:   command.AuthContext.RequestID,
		TraceID:       command.AuthContext.TraceID,
		Exception:     row,
	}); err != nil {
		return types.DeleteContactPrivacyExceptionResult{}, err
	}
	return commitDeleteContactPrivacyExceptionResult(ctx, tx, result)
}
