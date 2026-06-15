package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

func (r *Repository) GetContactPrivacy(
	ctx context.Context,
	command types.GetContactPrivacyCommand,
) (types.GetContactPrivacyResult, error) {
	if r.pool == nil {
		return types.GetContactPrivacyResult{}, types.NewDBReadFailed("contacts repository is not configured")
	}
	row, err := getContactPrivacySettings(ctx, r.pool, command.AuthContext.TenantID, command.AuthContext.UserID)
	if err != nil {
		return types.GetContactPrivacyResult{}, err
	}
	return contactPrivacyResultFromRow(row), nil
}

func (r *Repository) SetContactPrivacy(
	ctx context.Context,
	command types.SetContactPrivacyCommand,
) (types.SetContactPrivacyResult, error) {
	if r.pool == nil {
		return types.SetContactPrivacyResult{}, types.NewDBWriteFailed("contacts repository is not configured")
	}
	commandHash, err := commandHash(commandHashPayload{
		Kind:                 commandTypeSetContactPrivacy,
		TenantID:             string(command.AuthContext.TenantID),
		UserID:               string(command.AuthContext.UserID),
		AllowContactRequests: &command.AllowContactRequests,
	})
	if err != nil {
		return types.SetContactPrivacyResult{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.SetContactPrivacyResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if err := lockIdempotencyKey(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey); err != nil {
		return types.SetContactPrivacyResult{}, err
	}
	if existing, ok, err := findCommandIdempotency(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey); err != nil {
		return types.SetContactPrivacyResult{}, err
	} else if ok {
		if existing.CommandType != commandTypeSetContactPrivacy || existing.CommandHash != commandHash {
			return types.SetContactPrivacyResult{}, types.NewContactRequestConflict("idempotency key conflict")
		}
		row, err := contactPrivacyRowFromIdempotencyResult(existing)
		if err != nil {
			return types.SetContactPrivacyResult{}, err
		}
		return commitSetContactPrivacyResult(ctx, tx, setContactPrivacyResultFromRow(row, true))
	}
	if err := lockContactPrivacySettings(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID); err != nil {
		return types.SetContactPrivacyResult{}, err
	}
	row, changed, err := upsertContactPrivacySettings(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.AllowContactRequests)
	if err != nil {
		return types.SetContactPrivacyResult{}, err
	}
	resultJSON, err := contactPrivacyResultJSON(row)
	if err != nil {
		return types.SetContactPrivacyResult{}, err
	}
	if err := insertCommandIdempotencyWithResult(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey, commandTypeSetContactPrivacy, commandHash, string(command.AuthContext.UserID), resultJSON); err != nil {
		return types.SetContactPrivacyResult{}, err
	}
	if changed {
		if err := r.insertPrivacyOutbox(ctx, tx, privacyOutboxInput{
			TenantID:      command.AuthContext.TenantID,
			UserID:        command.AuthContext.UserID,
			CorrelationID: command.AuthContext.RequestID,
			CausationID:   command.AuthContext.RequestID,
			TraceID:       command.AuthContext.TraceID,
			Privacy:       row,
		}); err != nil {
			return types.SetContactPrivacyResult{}, err
		}
	}
	return commitSetContactPrivacyResult(ctx, tx, setContactPrivacyResultFromRow(row, false))
}

func (r *Repository) GetTenantContactPrivacyDefault(
	ctx context.Context,
	command types.GetTenantContactPrivacyDefaultCommand,
) (types.GetTenantContactPrivacyDefaultResult, error) {
	if r.pool == nil {
		return types.GetTenantContactPrivacyDefaultResult{}, types.NewDBReadFailed("contacts repository is not configured")
	}
	row, err := getTenantContactPrivacyDefault(ctx, r.pool, command.TenantID)
	if err != nil {
		return types.GetTenantContactPrivacyDefaultResult{}, err
	}
	return tenantContactPrivacyDefaultResultFromRow(row), nil
}

func (r *Repository) SetTenantContactPrivacyDefault(
	ctx context.Context,
	command types.SetTenantContactPrivacyDefaultCommand,
) (types.SetTenantContactPrivacyDefaultResult, error) {
	if r.pool == nil {
		return types.SetTenantContactPrivacyDefaultResult{}, types.NewDBWriteFailed("contacts repository is not configured")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.SetTenantContactPrivacyDefaultResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if err := lockTenantContactPrivacyDefault(ctx, tx, command.TenantID); err != nil {
		return types.SetTenantContactPrivacyDefaultResult{}, err
	}
	row, changed, err := upsertTenantContactPrivacyDefault(ctx, tx, command.TenantID, command.AllowContactRequests)
	if err != nil {
		return types.SetTenantContactPrivacyDefaultResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.SetTenantContactPrivacyDefaultResult{}, types.NewDBWriteFailed(err.Error())
	}
	return setTenantContactPrivacyDefaultResultFromRow(row, changed), nil
}

type contactPrivacyRow struct {
	TenantID             types.TenantID
	UserID               types.UserID
	AllowContactRequests bool
	Version              int64
	UpdatedAt            time.Time
	PolicySource         types.ContactPrivacyPolicySource
}

func defaultContactPrivacyRow(tenantID types.TenantID, userID types.UserID) contactPrivacyRow {
	return contactPrivacyRow{
		TenantID:             tenantID,
		UserID:               userID,
		AllowContactRequests: true,
		PolicySource:         types.ContactPrivacyPolicySourceSystemDefault,
	}
}

func tenantDefaultContactPrivacyRow(tenantID types.TenantID, userID types.UserID, allowContactRequests bool, version int64, updatedAt time.Time) contactPrivacyRow {
	return contactPrivacyRow{
		TenantID:             tenantID,
		UserID:               userID,
		AllowContactRequests: allowContactRequests,
		Version:              version,
		UpdatedAt:            updatedAt,
		PolicySource:         types.ContactPrivacyPolicySourceTenantDefault,
	}
}

func getContactPrivacySettings(
	ctx context.Context,
	queryer interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	tenantID types.TenantID,
	userID types.UserID,
) (contactPrivacyRow, error) {
	row, ok, err := getUserContactPrivacySettings(ctx, queryer, tenantID, userID)
	if err != nil || ok {
		return row, err
	}
	return getTenantDefaultContactPrivacySettings(ctx, queryer, tenantID, userID)
}

func getUserContactPrivacySettings(
	ctx context.Context,
	queryer interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	tenantID types.TenantID,
	userID types.UserID,
) (contactPrivacyRow, bool, error) {
	var row contactPrivacyRow
	err := queryer.QueryRow(ctx, `
SELECT tenant_id, user_id, allow_contact_requests, version, updated_at
FROM contact_privacy_settings
WHERE tenant_id = $1
  AND user_id = $2
`, tenantID, userID).Scan(&row.TenantID, &row.UserID, &row.AllowContactRequests, &row.Version, &row.UpdatedAt)
	if err == pgx.ErrNoRows {
		return contactPrivacyRow{}, false, nil
	}
	if err != nil {
		return contactPrivacyRow{}, false, types.NewDBReadFailed(err.Error())
	}
	row.PolicySource = types.ContactPrivacyPolicySourceUser
	return row, true, nil
}

func getTenantDefaultContactPrivacySettings(
	ctx context.Context,
	queryer interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	tenantID types.TenantID,
	userID types.UserID,
) (contactPrivacyRow, error) {
	var allowContactRequests bool
	var version int64
	var updatedAt time.Time
	err := queryer.QueryRow(ctx, `
SELECT allow_contact_requests, version, updated_at
FROM contact_tenant_privacy_defaults
WHERE tenant_id = $1
`, tenantID).Scan(&allowContactRequests, &version, &updatedAt)
	if err == pgx.ErrNoRows {
		return defaultContactPrivacyRow(tenantID, userID), nil
	}
	if err != nil {
		return contactPrivacyRow{}, types.NewDBReadFailed(err.Error())
	}
	return tenantDefaultContactPrivacyRow(tenantID, userID, allowContactRequests, version, updatedAt), nil
}

func getTenantContactPrivacyDefault(
	ctx context.Context,
	queryer interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	tenantID types.TenantID,
) (contactPrivacyRow, error) {
	return getTenantDefaultContactPrivacySettings(ctx, queryer, tenantID, "")
}

func contactRequestsAllowed(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID) (bool, error) {
	row, err := getContactPrivacySettings(ctx, tx, tenantID, userID)
	if err != nil {
		return false, err
	}
	return row.AllowContactRequests, nil
}

func lockTenantContactPrivacyDefault(ctx context.Context, tx pgx.Tx, tenantID types.TenantID) error {
	key := fmt.Sprintf("%s\x1fcontacts_tenant_privacy_default", tenantID)
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func lockContactPrivacySettings(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID) error {
	key := fmt.Sprintf("%s\x1f%s\x1fcontacts_privacy", tenantID, userID)
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func upsertContactPrivacySettings(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	userID types.UserID,
	allowContactRequests bool,
) (contactPrivacyRow, bool, error) {
	current, ok, err := getUserContactPrivacySettings(ctx, tx, tenantID, userID)
	if err != nil {
		return contactPrivacyRow{}, false, err
	}
	if ok && current.AllowContactRequests == allowContactRequests {
		return current, false, nil
	}
	var row contactPrivacyRow
	if !ok {
		err = tx.QueryRow(ctx, `
INSERT INTO contact_privacy_settings (
    tenant_id,
    user_id,
    allow_contact_requests,
    version,
    created_at,
    updated_at
) VALUES ($1, $2, $3, 1, now(), now())
RETURNING tenant_id, user_id, allow_contact_requests, version, updated_at
`, tenantID, userID, allowContactRequests).Scan(&row.TenantID, &row.UserID, &row.AllowContactRequests, &row.Version, &row.UpdatedAt)
	} else {
		err = tx.QueryRow(ctx, `
UPDATE contact_privacy_settings
SET allow_contact_requests = $3,
    version = version + 1,
    updated_at = now()
WHERE tenant_id = $1
  AND user_id = $2
RETURNING tenant_id, user_id, allow_contact_requests, version, updated_at
`, tenantID, userID, allowContactRequests).Scan(&row.TenantID, &row.UserID, &row.AllowContactRequests, &row.Version, &row.UpdatedAt)
	}
	if err != nil {
		return contactPrivacyRow{}, false, types.NewDBWriteFailed(err.Error())
	}
	row.PolicySource = types.ContactPrivacyPolicySourceUser
	return row, true, nil
}

func upsertTenantContactPrivacyDefault(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	allowContactRequests bool,
) (contactPrivacyRow, bool, error) {
	current, err := getTenantContactPrivacyDefault(ctx, tx, tenantID)
	if err != nil {
		return contactPrivacyRow{}, false, err
	}
	if current.PolicySource == types.ContactPrivacyPolicySourceTenantDefault &&
		current.AllowContactRequests == allowContactRequests {
		return current, false, nil
	}
	var row contactPrivacyRow
	if current.PolicySource != types.ContactPrivacyPolicySourceTenantDefault {
		err = tx.QueryRow(ctx, `
INSERT INTO contact_tenant_privacy_defaults (
    tenant_id,
    allow_contact_requests,
    version,
    created_at,
    updated_at
) VALUES ($1, $2, 1, now(), now())
RETURNING tenant_id, allow_contact_requests, version, updated_at
`, tenantID, allowContactRequests).Scan(&row.TenantID, &row.AllowContactRequests, &row.Version, &row.UpdatedAt)
	} else {
		err = tx.QueryRow(ctx, `
UPDATE contact_tenant_privacy_defaults
SET allow_contact_requests = $2,
    version = version + 1,
    updated_at = now()
WHERE tenant_id = $1
RETURNING tenant_id, allow_contact_requests, version, updated_at
`, tenantID, allowContactRequests).Scan(&row.TenantID, &row.AllowContactRequests, &row.Version, &row.UpdatedAt)
	}
	if err != nil {
		return contactPrivacyRow{}, false, types.NewDBWriteFailed(err.Error())
	}
	row.PolicySource = types.ContactPrivacyPolicySourceTenantDefault
	return row, true, nil
}

func contactPrivacyResultFromRow(row contactPrivacyRow) types.GetContactPrivacyResult {
	return types.GetContactPrivacyResult{
		TenantID: row.TenantID,
		UserID:   row.UserID,
		Settings: contactPrivacySettingsFromRow(row),
	}
}

func setContactPrivacyResultFromRow(row contactPrivacyRow, replay bool) types.SetContactPrivacyResult {
	return types.SetContactPrivacyResult{
		TenantID:         row.TenantID,
		UserID:           row.UserID,
		Settings:         contactPrivacySettingsFromRow(row),
		IdempotentReplay: replay,
	}
}

func tenantContactPrivacyDefaultResultFromRow(row contactPrivacyRow) types.GetTenantContactPrivacyDefaultResult {
	return types.GetTenantContactPrivacyDefaultResult{
		TenantID: row.TenantID,
		Settings: contactPrivacySettingsFromRow(row),
	}
}

func setTenantContactPrivacyDefaultResultFromRow(row contactPrivacyRow, changed bool) types.SetTenantContactPrivacyDefaultResult {
	return types.SetTenantContactPrivacyDefaultResult{
		TenantID: row.TenantID,
		Settings: contactPrivacySettingsFromRow(row),
		Changed:  changed,
	}
}

func contactPrivacySettingsFromRow(row contactPrivacyRow) types.ContactPrivacySettings {
	var updatedAtUnixMS int64
	if !row.UpdatedAt.IsZero() {
		updatedAtUnixMS = row.UpdatedAt.UnixMilli()
	}
	return types.ContactPrivacySettings{
		AllowContactRequests: row.AllowContactRequests,
		Version:              row.Version,
		UpdatedAtUnixMS:      updatedAtUnixMS,
		PolicySource:         row.PolicySource,
	}
}

type privacyOutboxInput struct {
	TenantID      types.TenantID
	UserID        types.UserID
	CorrelationID string
	CausationID   string
	TraceID       string
	Privacy       contactPrivacyRow
}

func (r *Repository) insertPrivacyOutbox(ctx context.Context, tx pgx.Tx, input privacyOutboxInput) error {
	eventID, err := r.eventID()
	if err != nil {
		return types.NewOutboxWriteFailed(err.Error())
	}
	partitionKey := fmt.Sprintf("%s:%s", input.TenantID, input.UserID)
	aggregateVersion, err := nextContactOutboxAggregateVersion(ctx, tx, input.TenantID, partitionKey)
	if err != nil {
		return err
	}
	return insertContactOutbox(ctx, tx, contactOutboxInput{
		EventID:          eventID,
		TenantID:         input.TenantID,
		AggregateType:    "CONTACT_PRIVACY",
		AggregateID:      string(input.UserID),
		AggregateVersion: aggregateVersion,
		EventType:        eventTypeContactPrivacyUpdated,
		PartitionKey:     partitionKey,
		CorrelationID:    input.CorrelationID,
		CausationID:      input.CausationID,
		TraceID:          input.TraceID,
		Payload: map[string]any{
			"tenant_id":              input.TenantID,
			"user_id":                input.UserID,
			"allow_contact_requests": input.Privacy.AllowContactRequests,
			"privacy_version":        input.Privacy.Version,
			"occurred_at":            r.now().Format(time.RFC3339Nano),
		},
	})
}

func commitSetContactPrivacyResult(ctx context.Context, tx pgx.Tx, result types.SetContactPrivacyResult) (types.SetContactPrivacyResult, error) {
	if err := tx.Commit(ctx); err != nil {
		return types.SetContactPrivacyResult{}, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

type contactPrivacyResultSnapshot struct {
	TenantID             types.TenantID `json:"tenant_id"`
	UserID               types.UserID   `json:"user_id"`
	AllowContactRequests bool           `json:"allow_contact_requests"`
	Version              int64          `json:"version"`
	UpdatedAtUnixMS      int64          `json:"updated_at_unix_ms"`
	PolicySource         string         `json:"policy_source"`
}

func contactPrivacyResultJSON(row contactPrivacyRow) ([]byte, error) {
	settings := contactPrivacySettingsFromRow(row)
	raw, err := json.Marshal(contactPrivacyResultSnapshot{
		TenantID:             row.TenantID,
		UserID:               row.UserID,
		AllowContactRequests: settings.AllowContactRequests,
		Version:              settings.Version,
		UpdatedAtUnixMS:      settings.UpdatedAtUnixMS,
		PolicySource:         string(settings.PolicySource),
	})
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	return raw, nil
}

func contactPrivacyRowFromIdempotencyResult(existing commandIdempotency) (contactPrivacyRow, error) {
	var snapshot contactPrivacyResultSnapshot
	if len(existing.ResultJSON) == 0 || string(existing.ResultJSON) == "{}" {
		return contactPrivacyRow{}, types.NewDBReadFailed("contact privacy idempotency result snapshot missing")
	}
	if err := json.Unmarshal(existing.ResultJSON, &snapshot); err != nil {
		return contactPrivacyRow{}, types.NewDBReadFailed(err.Error())
	}
	if snapshot.TenantID == "" || snapshot.UserID == "" || snapshot.Version <= 0 || snapshot.UpdatedAtUnixMS <= 0 {
		return contactPrivacyRow{}, types.NewDBReadFailed("contact privacy idempotency result snapshot incomplete")
	}
	return contactPrivacyRow{
		TenantID:             snapshot.TenantID,
		UserID:               snapshot.UserID,
		AllowContactRequests: snapshot.AllowContactRequests,
		Version:              snapshot.Version,
		UpdatedAt:            time.UnixMilli(snapshot.UpdatedAtUnixMS).UTC(),
		PolicySource:         contactPrivacyPolicySourceFromSnapshot(snapshot.PolicySource),
	}, nil
}

func contactPrivacyPolicySourceFromSnapshot(source string) types.ContactPrivacyPolicySource {
	switch types.ContactPrivacyPolicySource(source) {
	case types.ContactPrivacyPolicySourceTenantDefault,
		types.ContactPrivacyPolicySourceSystemDefault:
		return types.ContactPrivacyPolicySource(source)
	default:
		return types.ContactPrivacyPolicySourceUser
	}
}
