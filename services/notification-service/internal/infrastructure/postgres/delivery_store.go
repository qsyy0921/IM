package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/notification-service/internal/types"
)

type DeliveryStore struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

type DeliveryStoreOption func(*DeliveryStore)

func NewDeliveryStore(pool *pgxpool.Pool, opts ...DeliveryStoreOption) *DeliveryStore {
	store := &DeliveryStore{
		pool: pool,
		now:  func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(store)
	}
	return store
}

func WithDeliveryClock(clock func() time.Time) DeliveryStoreOption {
	return func(store *DeliveryStore) {
		if clock != nil {
			store.now = clock
		}
	}
}

func (store *DeliveryStore) ClaimReadyDeliveryRequests(ctx context.Context, limit int, providerID string) ([]types.DeliveryRequest, error) {
	if store == nil || store.pool == nil {
		return nil, types.NewDBWriteFailed("notification delivery store is not configured")
	}
	if limit <= 0 {
		limit = 50
	}
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		providerID = "local-noop"
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	rows, err := tx.Query(ctx, `
WITH candidates AS (
    SELECT tenant_id, request_id
    FROM notification_requests
    WHERE status IN ('ACCEPTED', 'SCHEDULED', 'RETRY_WAIT')
      AND (
        next_attempt_at IS NULL
        OR next_attempt_at <= now()
        OR (expires_at IS NOT NULL AND expires_at <= now())
      )
    ORDER BY created_at, request_id
    LIMIT $1
    FOR UPDATE SKIP LOCKED
)
UPDATE notification_requests current
SET status = 'SENDING',
    attempt_count = current.attempt_count + 1,
    next_attempt_at = NULL,
    last_failure_class = '',
    last_public_error = ''
FROM candidates
WHERE current.tenant_id = candidates.tenant_id
  AND current.request_id = candidates.request_id
RETURNING
	current.tenant_id,
	current.request_id,
	current.requester_service,
	current.requester_user_id,
	current.idempotency_key,
	current.command_hash,
	current.channel,
	current.recipient_ref,
	current.destination_hash,
	current.destination_masked,
	current.template_key,
	current.template_version,
	current.locale,
	current.priority,
	current.template_variables_json,
	current.secret_payload_ciphertext,
	current.secret_payload_key_version,
	current.secret_payload_expires_at,
	current.status,
	current.attempt_count,
	current.next_attempt_at,
	current.expires_at,
	current.last_failure_class,
	current.last_public_error,
	current.correlation_id,
	current.causation_id,
	current.trace_id,
	current.created_at,
	current.delivered_at,
	current.dead_lettered_at,
	current.canceled_at
`, limit)
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer rows.Close()
	requests := make([]types.DeliveryRequest, 0)
	for rows.Next() {
		request, err := scanNotificationRequest(rows)
		if err != nil {
			return nil, types.NewDBWriteFailed(err.Error())
		}
		requests = append(requests, types.DeliveryRequest{
			NotificationRequest:    request,
			AttemptNumber:          request.AttemptCount,
			ProviderID:             providerID,
			ProviderIdempotencyKey: providerIdempotencyKey(providerID, request),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	return requests, nil
}

func (store *DeliveryStore) MarkDeliverySucceeded(ctx context.Context, request types.DeliveryRequest, result types.DeliveryResult) error {
	if store == nil || store.pool == nil {
		return types.NewDBWriteFailed("notification delivery store is not configured")
	}
	now := store.now()
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	current, err := lockNotificationRequest(ctx, tx, request.TenantID, request.RequestID)
	if err != nil {
		return err
	}
	if current.Status == types.StatusDelivered {
		if err := tx.Commit(ctx); err != nil {
			return types.NewDBWriteFailed(err.Error())
		}
		return nil
	}
	if current.Status != types.StatusSending {
		return types.NewFailedPrecondition("notification request is not sending")
	}
	result.ProviderID = firstNonEmpty(result.ProviderID, request.ProviderID)
	if strings.TrimSpace(result.ProviderMessageIDHash) == "" {
		result.ProviderMessageIDHash = providerMessageHash(result.ProviderID, request)
	}
	if err := insertDeliveryAttempt(ctx, tx, request, result.ProviderID, result.ProviderMessageIDHash, types.AttemptStatusSucceeded, "", "", now, time.Time{}); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
UPDATE notification_requests
SET status = 'DELIVERED',
    delivered_at = $3,
    last_failure_class = '',
    last_public_error = '',
    next_attempt_at = NULL
WHERE tenant_id = $1
  AND request_id = $2
`, request.TenantID, request.RequestID, now); err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if err := insertDeliveryOutbox(ctx, tx, request, types.NotificationEventDeliverySucceeded, deliverySuccessPayload(request, result), request.AttemptNumber+1); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func (store *DeliveryStore) MarkDeliveryFailed(ctx context.Context, request types.DeliveryRequest, failure types.DeliveryFailure, maxAttempts int, retryBaseDelay time.Duration) (bool, error) {
	if store == nil || store.pool == nil {
		return false, types.NewDBWriteFailed("notification delivery store is not configured")
	}
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	if retryBaseDelay <= 0 {
		retryBaseDelay = time.Second
	}
	failure = normalizeDeliveryFailure(failure)
	now := store.now()
	deadLettered := failure.Permanent || request.AttemptNumber >= maxAttempts
	retryAfter := failure.RetryAfter
	if retryAfter.IsZero() && !deadLettered {
		retryAfter = now.Add(deliveryRetryDelay(retryBaseDelay, request.AttemptNumber))
	}

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return false, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	current, err := lockNotificationRequest(ctx, tx, request.TenantID, request.RequestID)
	if err != nil {
		return false, err
	}
	if current.Status != types.StatusSending {
		if types.IsTerminalStatus(current.Status) {
			if err := tx.Commit(ctx); err != nil {
				return false, types.NewDBWriteFailed(err.Error())
			}
			return current.Status == types.StatusDLQ, nil
		}
		return false, types.NewFailedPrecondition("notification request is not sending")
	}
	if err := insertDeliveryAttempt(ctx, tx, request, request.ProviderID, "", types.AttemptStatusFailed, failure.FailureClass, failure.PublicError, now, retryAfter); err != nil {
		return false, err
	}
	status := types.StatusRetryWait
	eventType := types.NotificationEventDeliveryFailed
	deadLetteredAt := any(nil)
	nextAttemptAt := any(retryAfter)
	if deadLettered {
		status = types.StatusDLQ
		eventType = types.NotificationEventDeliveryDeadLettered
		deadLetteredAt = now
		nextAttemptAt = nil
	}
	if _, err := tx.Exec(ctx, `
UPDATE notification_requests
SET status = $3,
    next_attempt_at = $4,
    dead_lettered_at = $5,
    last_failure_class = $6,
    last_public_error = $7
WHERE tenant_id = $1
  AND request_id = $2
`, request.TenantID, request.RequestID, status, nextAttemptAt, deadLetteredAt, failure.FailureClass, failure.PublicError); err != nil {
		return false, types.NewDBWriteFailed(err.Error())
	}
	if err := insertDeliveryOutbox(ctx, tx, request, eventType, deliveryFailurePayload(request, failure), request.AttemptNumber+1); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, types.NewDBWriteFailed(err.Error())
	}
	return deadLettered, nil
}

func insertDeliveryAttempt(
	ctx context.Context,
	tx pgx.Tx,
	request types.DeliveryRequest,
	providerID string,
	providerMessageIDHash string,
	status string,
	failureClass string,
	publicError string,
	finishedAt time.Time,
	retryAfter time.Time,
) error {
	attemptID := deliveryAttemptID(request)
	_, err := tx.Exec(ctx, `
INSERT INTO notification_delivery_attempts (
    tenant_id,
    attempt_id,
    request_id,
    provider_id,
    provider_message_id_hash,
    status,
    failure_class,
    public_error,
    started_at,
    finished_at,
    retry_after
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $9, $10
)
ON CONFLICT (tenant_id, attempt_id) DO NOTHING
`, request.TenantID, attemptID, request.RequestID, providerID, providerMessageIDHash, status, failureClass, publicError, finishedAt, nullableTime(retryAfter))
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertDeliveryOutbox(ctx context.Context, tx pgx.Tx, request types.DeliveryRequest, eventType string, payload map[string]any, eventVersion int) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	eventID := fmt.Sprintf("%s:%s:%d", request.RequestID, eventTypeSuffix(eventType), request.AttemptNumber)
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
	$1, $2, $3, $4, $5, $6, $7::jsonb, 'PENDING', now(), now()
)
ON CONFLICT (tenant_id, event_id) DO NOTHING
`, eventID, request.TenantID, request.RequestID, eventType, eventVersion, string(request.TenantID)+":"+request.RequestID, string(payloadJSON))
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func deliverySuccessPayload(request types.DeliveryRequest, result types.DeliveryResult) map[string]any {
	return map[string]any{
		"tenant_id":                string(request.TenantID),
		"request_id":               request.RequestID,
		"channel":                  request.Channel,
		"provider_id":              result.ProviderID,
		"provider_message_id_hash": result.ProviderMessageIDHash,
		"correlation_id":           request.CorrelationID,
		"causation_id":             request.CausationID,
		"trace_id":                 request.TraceID,
	}
}

func deliveryFailurePayload(request types.DeliveryRequest, failure types.DeliveryFailure) map[string]any {
	return map[string]any{
		"tenant_id":      string(request.TenantID),
		"request_id":     request.RequestID,
		"channel":        request.Channel,
		"failure_class":  failure.FailureClass,
		"public_error":   failure.PublicError,
		"correlation_id": request.CorrelationID,
		"causation_id":   request.CausationID,
		"trace_id":       request.TraceID,
	}
}

func deliveryAttemptID(request types.DeliveryRequest) string {
	return fmt.Sprintf("%s:attempt:%d", request.RequestID, request.AttemptNumber)
}

func eventTypeSuffix(eventType string) string {
	switch eventType {
	case types.NotificationEventDeliverySucceeded:
		return "delivery-succeeded"
	case types.NotificationEventDeliveryFailed:
		return "delivery-failed"
	case types.NotificationEventDeliveryDeadLettered:
		return "delivery-dead-lettered"
	default:
		return "delivery-event"
	}
}

func normalizeDeliveryFailure(failure types.DeliveryFailure) types.DeliveryFailure {
	if strings.TrimSpace(failure.FailureClass) == "" {
		failure.FailureClass = types.FailureClassProviderUnavailable
	}
	if strings.TrimSpace(failure.PublicError) == "" {
		failure.PublicError = types.PublicErrorProviderUnavailable
	}
	failure.FailureClass = sanitizeDeliveryText(failure.FailureClass, types.FailureClassProviderUnavailable)
	failure.PublicError = sanitizeDeliveryText(failure.PublicError, types.PublicErrorProviderUnavailable)
	return failure
}

func sanitizeDeliveryText(value string, defaultValue string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue
	}
	lowered := strings.ToLower(value)
	for _, marker := range []string{"authorization", "smtp", "provider_body", "provider_response", "reset_token", "challenge", "totp", "recovery", "secret"} {
		if strings.Contains(lowered, marker) {
			return defaultValue
		}
	}
	if len(value) > 128 {
		value = value[:128]
	}
	return value
}

func deliveryRetryDelay(base time.Duration, attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	exponent := attempt - 1
	if exponent > 10 {
		exponent = 10
	}
	return base * time.Duration(1<<exponent)
}

func providerIdempotencyKey(providerID string, request types.NotificationRequest) string {
	return providerID + ":" + string(request.TenantID) + ":" + request.RequestID + ":" + fmt.Sprintf("%d", request.AttemptCount)
}

func providerMessageHash(providerID string, request types.DeliveryRequest) string {
	digest := sha256.Sum256([]byte(providerID + ":" + string(request.TenantID) + ":" + request.RequestID + ":" + request.ProviderIdempotencyKey))
	return hex.EncodeToString(digest[:])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
