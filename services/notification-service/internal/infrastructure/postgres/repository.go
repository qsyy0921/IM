package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/notification-service/internal/types"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (repository *Repository) CreateNotificationRequest(
	ctx context.Context,
	command types.CreateNotificationRequestCommand,
	requestID string,
	destinationHash string,
	commandHash string,
) (types.NotificationRequest, error) {
	if repository.pool == nil {
		return types.NotificationRequest{}, types.NewDBWriteFailed("notification repository is not configured")
	}
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return types.NotificationRequest{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	existing, err := getRequestByIdempotency(ctx, tx, command.AuthContext.TenantID, command.RequesterService, command.IdempotencyKey)
	if err != nil {
		return types.NotificationRequest{}, err
	}
	if existing.RequestID != "" {
		if existing.CommandHash != commandHash {
			return types.NotificationRequest{}, types.NewAlreadyExists("idempotency key command mismatch")
		}
		if err := tx.Commit(ctx); err != nil {
			return types.NotificationRequest{}, types.NewDBReadFailed(err.Error())
		}
		return existing, nil
	}

	status := types.StatusAccepted
	var nextAttemptAt time.Time
	if !command.ScheduledAt.IsZero() && command.ScheduledAt.After(time.Now().UTC()) {
		status = types.StatusScheduled
		nextAttemptAt = command.ScheduledAt.UTC()
	}
	row := tx.QueryRow(ctx, `
INSERT INTO notification_requests (
	tenant_id,
	request_id,
	requester_service,
	requester_user_id,
	idempotency_key,
	command_hash,
	channel,
	recipient_ref,
	destination_hash,
	destination_masked,
	template_key,
	template_version,
	locale,
	priority,
	template_variables_json,
	secret_payload_ciphertext,
	secret_payload_key_version,
	secret_payload_expires_at,
	status,
	next_attempt_at,
	expires_at,
	correlation_id,
	causation_id,
	trace_id,
	created_at
) VALUES (
	$1, $2, $3, $4, $5, $6,
	$7, $8, $9, $10, $11, $12,
	$13, $14, $15::jsonb, $16, $17, $18,
	$19, $20, $21, $22, $23, $24, now()
)
RETURNING
	tenant_id,
	request_id,
	requester_service,
	requester_user_id,
	idempotency_key,
	command_hash,
	channel,
	recipient_ref,
	destination_hash,
	destination_masked,
	template_key,
	template_version,
	locale,
	priority,
	template_variables_json,
	secret_payload_ciphertext,
	secret_payload_key_version,
	secret_payload_expires_at,
	status,
	attempt_count,
	next_attempt_at,
	expires_at,
	last_failure_class,
	last_public_error,
	correlation_id,
	causation_id,
	trace_id,
	created_at,
	delivered_at,
	dead_lettered_at,
	canceled_at
`, command.AuthContext.TenantID,
		requestID,
		command.RequesterService,
		string(command.RequesterUserID),
		command.IdempotencyKey,
		commandHash,
		command.Channel,
		command.RecipientRef,
		destinationHash,
		command.DestinationMasked,
		command.TemplateKey,
		command.TemplateVersion,
		command.Locale,
		command.Priority,
		command.TemplateVariablesJSON,
		command.SecretPayloadCiphertext,
		command.SecretPayloadKeyVersion,
		nullableTime(command.SecretPayloadExpiresAt),
		status,
		nullableTime(nextAttemptAt),
		nullableTime(command.ExpiresAt),
		command.CorrelationID,
		command.CausationID,
		command.TraceID,
	)
	result, err := scanNotificationRequest(row)
	if err != nil {
		return types.NotificationRequest{}, types.NewDBWriteFailed(err.Error())
	}
	if err := insertAcceptedOutbox(ctx, tx, result); err != nil {
		return types.NotificationRequest{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.NotificationRequest{}, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func (repository *Repository) GetNotificationRequest(
	ctx context.Context,
	tenantID types.TenantID,
	requestID string,
) (types.NotificationRequest, error) {
	if repository.pool == nil {
		return types.NotificationRequest{}, types.NewDBReadFailed("notification repository is not configured")
	}
	row := repository.pool.QueryRow(ctx, selectRequestSQL()+`
WHERE tenant_id = $1
  AND request_id = $2
`, tenantID, strings.TrimSpace(requestID))
	result, err := scanNotificationRequest(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.NotificationRequest{}, types.NewNotFound("notification request not found")
		}
		return types.NotificationRequest{}, types.NewDBReadFailed(err.Error())
	}
	return result, nil
}

func (repository *Repository) CancelNotificationRequest(
	ctx context.Context,
	command types.CancelNotificationRequestCommand,
) (types.NotificationRequest, error) {
	if repository.pool == nil {
		return types.NotificationRequest{}, types.NewDBWriteFailed("notification repository is not configured")
	}
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return types.NotificationRequest{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	current, err := lockNotificationRequest(ctx, tx, command.AuthContext.TenantID, command.RequestID)
	if err != nil {
		return types.NotificationRequest{}, err
	}
	if current.Status == types.StatusCanceled {
		if err := tx.Commit(ctx); err != nil {
			return types.NotificationRequest{}, types.NewDBReadFailed(err.Error())
		}
		return current, nil
	}
	if current.Status == types.StatusDelivered || current.Status == types.StatusDLQ {
		return types.NotificationRequest{}, types.NewFailedPrecondition("notification request is terminal")
	}
	row := tx.QueryRow(ctx, `
UPDATE notification_requests
SET status = 'CANCELED',
    canceled_at = now(),
    last_public_error = ''
WHERE tenant_id = $1
  AND request_id = $2
RETURNING
	tenant_id,
	request_id,
	requester_service,
	requester_user_id,
	idempotency_key,
	command_hash,
	channel,
	recipient_ref,
	destination_hash,
	destination_masked,
	template_key,
	template_version,
	locale,
	priority,
	template_variables_json,
	secret_payload_ciphertext,
	secret_payload_key_version,
	secret_payload_expires_at,
	status,
	attempt_count,
	next_attempt_at,
	expires_at,
	last_failure_class,
	last_public_error,
	correlation_id,
	causation_id,
	trace_id,
	created_at,
	delivered_at,
	dead_lettered_at,
	canceled_at
`, command.AuthContext.TenantID, strings.TrimSpace(command.RequestID))
	result, err := scanNotificationRequest(row)
	if err != nil {
		return types.NotificationRequest{}, types.NewDBWriteFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return types.NotificationRequest{}, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

type requestScanner interface {
	Scan(dest ...any) error
}

func scanNotificationRequest(scanner requestScanner) (types.NotificationRequest, error) {
	var request types.NotificationRequest
	var requesterUserID string
	var secretExpiresAt *time.Time
	var nextAttemptAt *time.Time
	var expiresAt *time.Time
	var deliveredAt *time.Time
	var deadLetteredAt *time.Time
	var canceledAt *time.Time
	if err := scanner.Scan(
		&request.TenantID,
		&request.RequestID,
		&request.RequesterService,
		&requesterUserID,
		&request.IdempotencyKey,
		&request.CommandHash,
		&request.Channel,
		&request.RecipientRef,
		&request.DestinationHash,
		&request.DestinationMasked,
		&request.TemplateKey,
		&request.TemplateVersion,
		&request.Locale,
		&request.Priority,
		&request.TemplateVariablesJSON,
		&request.SecretPayloadCiphertext,
		&request.SecretPayloadKeyVersion,
		&secretExpiresAt,
		&request.Status,
		&request.AttemptCount,
		&nextAttemptAt,
		&expiresAt,
		&request.LastFailureClass,
		&request.LastPublicError,
		&request.CorrelationID,
		&request.CausationID,
		&request.TraceID,
		&request.CreatedAt,
		&deliveredAt,
		&deadLetteredAt,
		&canceledAt,
	); err != nil {
		return types.NotificationRequest{}, err
	}
	request.RequesterUserID = types.UserID(requesterUserID)
	request.SecretPayloadExpiresAt = valueTime(secretExpiresAt)
	request.NextAttemptAt = valueTime(nextAttemptAt)
	request.ExpiresAt = valueTime(expiresAt)
	request.DeliveredAt = valueTime(deliveredAt)
	request.DeadLetteredAt = valueTime(deadLetteredAt)
	request.CanceledAt = valueTime(canceledAt)
	return request, nil
}

func getRequestByIdempotency(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	requesterService string,
	idempotencyKey string,
) (types.NotificationRequest, error) {
	row := tx.QueryRow(ctx, selectRequestSQL()+`
WHERE tenant_id = $1
  AND requester_service = $2
  AND idempotency_key = $3
FOR UPDATE
`, tenantID, strings.TrimSpace(requesterService), strings.TrimSpace(idempotencyKey))
	result, err := scanNotificationRequest(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.NotificationRequest{}, nil
		}
		return types.NotificationRequest{}, types.NewDBReadFailed(err.Error())
	}
	return result, nil
}

func lockNotificationRequest(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	requestID string,
) (types.NotificationRequest, error) {
	row := tx.QueryRow(ctx, selectRequestSQL()+`
WHERE tenant_id = $1
  AND request_id = $2
FOR UPDATE
`, tenantID, strings.TrimSpace(requestID))
	result, err := scanNotificationRequest(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.NotificationRequest{}, types.NewNotFound("notification request not found")
		}
		return types.NotificationRequest{}, types.NewDBReadFailed(err.Error())
	}
	return result, nil
}

func insertAcceptedOutbox(ctx context.Context, tx pgx.Tx, request types.NotificationRequest) error {
	payload := map[string]any{
		"tenant_id":          string(request.TenantID),
		"request_id":         request.RequestID,
		"requester_service":  request.RequesterService,
		"channel":            request.Channel,
		"recipient_ref":      request.RecipientRef,
		"destination_masked": request.DestinationMasked,
		"template_key":       request.TemplateKey,
		"template_version":   request.TemplateVersion,
		"locale":             request.Locale,
		"priority":           request.Priority,
		"status":             request.Status,
		"correlation_id":     request.CorrelationID,
		"causation_id":       request.CausationID,
		"trace_id":           request.TraceID,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	eventID := request.RequestID + ":accepted"
	_, err = tx.Exec(ctx, `
INSERT INTO notification_outbox (
	event_id,
	tenant_id,
	request_id,
	event_type,
	event_version,
	partition_key,
	payload_json,
	status,
	created_at,
	updated_at
) VALUES (
	$1, $2, $3, 'notification.request.accepted.v1', 1, $4, $5::jsonb, 'PENDING', now(), now()
)
ON CONFLICT (tenant_id, event_id) DO NOTHING
`, eventID, request.TenantID, request.RequestID, string(request.TenantID)+":"+request.RequestID, string(payloadJSON))
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func selectRequestSQL() string {
	return `
SELECT
	tenant_id,
	request_id,
	requester_service,
	requester_user_id,
	idempotency_key,
	command_hash,
	channel,
	recipient_ref,
	destination_hash,
	destination_masked,
	template_key,
	template_version,
	locale,
	priority,
	template_variables_json,
	secret_payload_ciphertext,
	secret_payload_key_version,
	secret_payload_expires_at,
	status,
	attempt_count,
	next_attempt_at,
	expires_at,
	last_failure_class,
	last_public_error,
	correlation_id,
	causation_id,
	trace_id,
	created_at,
	delivered_at,
	dead_lettered_at,
	canceled_at
FROM notification_requests
`
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

func valueTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return value.UTC()
}
