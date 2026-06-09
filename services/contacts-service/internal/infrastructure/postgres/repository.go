package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/contacts-service/internal/domain"
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

const (
	commandTypeSendContactRequest    = "SEND_CONTACT_REQUEST"
	commandTypeRespondContactRequest = "RESPOND_CONTACT_REQUEST"

	eventTypeContactRequestCreated  = "contact.request.created.v1"
	eventTypeContactRequestAccepted = "contact.request.accepted.v1"
	eventTypeContactRequestDeclined = "contact.request.declined.v1"

	contactsOutboxEventVersion   = "1.0.0"
	contactsOutboxMappingVersion = 1
)

type Repository struct {
	pool      *pgxpool.Pool
	now       func() time.Time
	requestID func() (string, error)
	eventID   func() (string, error)
}

type RepositoryOption func(*Repository)

func NewRepository(pool *pgxpool.Pool, opts ...RepositoryOption) *Repository {
	repository := &Repository{
		pool: pool,
		now:  func() time.Time { return time.Now().UTC() },
		requestID: func() (string, error) {
			id, err := newID("contact_req")
			if err != nil {
				return "", err
			}
			return id, nil
		},
		eventID: func() (string, error) {
			return newID("evt_contact")
		},
	}
	for _, opt := range opts {
		opt(repository)
	}
	return repository
}

func WithClock(clock func() time.Time) RepositoryOption {
	return func(repository *Repository) {
		if clock != nil {
			repository.now = clock
		}
	}
}

func WithIDGenerators(requestID func() (string, error), eventID func() (string, error)) RepositoryOption {
	return func(repository *Repository) {
		if requestID != nil {
			repository.requestID = requestID
		}
		if eventID != nil {
			repository.eventID = eventID
		}
	}
}

func (r *Repository) SendContactRequest(
	ctx context.Context,
	command types.SendContactRequestCommand,
) (types.SendContactRequestResult, error) {
	if r.pool == nil {
		return types.SendContactRequestResult{}, types.NewDBWriteFailed("contacts repository is not configured")
	}
	commandHash, err := commandHash(commandHashPayload{
		Kind:         commandTypeSendContactRequest,
		TenantID:     string(command.AuthContext.TenantID),
		UserID:       string(command.AuthContext.UserID),
		TargetUserID: string(command.TargetUserID),
		Message:      command.Message,
	})
	if err != nil {
		return types.SendContactRequestResult{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.SendContactRequestResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := lockIdempotencyKey(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey); err != nil {
		return types.SendContactRequestResult{}, err
	}
	if existing, ok, err := findContactRequestByIdempotency(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey); err != nil {
		return types.SendContactRequestResult{}, err
	} else if ok {
		if existing.CommandHash != commandHash {
			return types.SendContactRequestResult{}, types.NewContactRequestConflict("idempotency key conflict")
		}
		return commitSendResult(ctx, tx, sendResultFromRequest(existing, true))
	}
	if err := lockContactPair(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.TargetUserID); err != nil {
		return types.SendContactRequestResult{}, err
	}
	if ok, err := activeContactExists(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.TargetUserID); err != nil {
		return types.SendContactRequestResult{}, err
	} else if ok {
		return types.SendContactRequestResult{}, types.NewContactAlreadyExists("contact already exists")
	}
	if ok, err := pendingContactRequestExists(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.TargetUserID); err != nil {
		return types.SendContactRequestResult{}, err
	} else if ok {
		return types.SendContactRequestResult{}, types.NewContactRequestConflict("pending contact request already exists")
	}

	requestID, err := r.requestID()
	if err != nil {
		return types.SendContactRequestResult{}, types.NewDBWriteFailed(err.Error())
	}
	occurredAt := r.now()
	if err := insertContactRequest(ctx, tx, command, requestID, commandHash, occurredAt); err != nil {
		return types.SendContactRequestResult{}, err
	}
	eventID, err := r.eventID()
	if err != nil {
		return types.SendContactRequestResult{}, types.NewOutboxWriteFailed(err.Error())
	}
	if err := insertContactOutbox(ctx, tx, contactOutboxInput{
		EventID:          eventID,
		TenantID:         command.AuthContext.TenantID,
		AggregateType:    "CONTACT_REQUEST",
		AggregateID:      requestID,
		AggregateVersion: 1,
		EventType:        eventTypeContactRequestCreated,
		PartitionKey:     partitionKeyFor(command.AuthContext.TenantID, command.AuthContext.UserID, command.TargetUserID),
		CorrelationID:    command.AuthContext.RequestID,
		CausationID:      command.AuthContext.RequestID,
		TraceID:          command.AuthContext.TraceID,
		Payload: map[string]any{
			"tenant_id":        command.AuthContext.TenantID,
			"request_id":       requestID,
			"sender_user_id":   command.AuthContext.UserID,
			"receiver_user_id": command.TargetUserID,
			"status":           types.ContactRequestStatusPending,
			"message":          command.Message,
			"occurred_at":      occurredAt.Format(time.RFC3339Nano),
		},
	}); err != nil {
		return types.SendContactRequestResult{}, err
	}
	return commitSendResult(ctx, tx, types.SendContactRequestResult{
		RequestID:      requestID,
		TenantID:       command.AuthContext.TenantID,
		SenderUserID:   command.AuthContext.UserID,
		ReceiverUserID: command.TargetUserID,
		Status:         types.ContactRequestStatusPending,
	})
}

func (r *Repository) RespondContactRequest(
	ctx context.Context,
	command types.RespondContactRequestCommand,
) (types.RespondContactRequestResult, error) {
	if r.pool == nil {
		return types.RespondContactRequestResult{}, types.NewDBWriteFailed("contacts repository is not configured")
	}
	commandHash, err := commandHash(commandHashPayload{
		Kind:      commandTypeRespondContactRequest,
		TenantID:  string(command.AuthContext.TenantID),
		UserID:    string(command.AuthContext.UserID),
		RequestID: command.RequestID,
		Decision:  string(command.Decision),
	})
	if err != nil {
		return types.RespondContactRequestResult{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.RespondContactRequestResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if err := lockIdempotencyKey(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey); err != nil {
		return types.RespondContactRequestResult{}, err
	}
	if existing, ok, err := findCommandIdempotency(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey); err != nil {
		return types.RespondContactRequestResult{}, err
	} else if ok {
		if existing.CommandType != commandTypeRespondContactRequest || existing.CommandHash != commandHash {
			return types.RespondContactRequestResult{}, types.NewContactRequestConflict("idempotency key conflict")
		}
		result, err := getRespondContactRequestResult(ctx, tx, command.AuthContext.TenantID, existing.ResultID)
		if err != nil {
			return types.RespondContactRequestResult{}, err
		}
		result.IdempotentReplay = true
		return commitRespondResult(ctx, tx, result)
	}
	request, err := lockContactRequest(ctx, tx, command.AuthContext.TenantID, command.RequestID)
	if err != nil {
		return types.RespondContactRequestResult{}, err
	}
	if request.ReceiverUserID != command.AuthContext.UserID {
		return types.RespondContactRequestResult{}, types.NewPermissionDenied("only request receiver can respond")
	}
	if err := lockContactPair(ctx, tx, request.TenantID, request.SenderUserID, request.ReceiverUserID); err != nil {
		return types.RespondContactRequestResult{}, err
	}
	expectedStatus := requestStatusForDecision(command.Decision)
	if request.Status != types.ContactRequestStatusPending {
		if request.Status != expectedStatus {
			return types.RespondContactRequestResult{}, types.NewContactRequestConflict("contact request already completed with a different status")
		}
		if err := insertCommandIdempotency(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey, commandTypeRespondContactRequest, commandHash, request.RequestID); err != nil {
			return types.RespondContactRequestResult{}, err
		}
		return commitRespondResult(ctx, tx, respondResultFromRequest(request, true))
	}

	if command.Decision == types.ContactDecisionAccept {
		if ok, err := blockedContactExists(ctx, tx, request.TenantID, request.SenderUserID, request.ReceiverUserID); err != nil {
			return types.RespondContactRequestResult{}, err
		} else if ok {
			return types.RespondContactRequestResult{}, types.NewContactRequestConflict("blocked contact edge exists")
		}
	}
	if err := updateContactRequestStatus(ctx, tx, request, expectedStatus); err != nil {
		return types.RespondContactRequestResult{}, err
	}
	edgeVersion := int64(0)
	if command.Decision == types.ContactDecisionAccept {
		edgeVersion, err = upsertAcceptedContactEdges(ctx, tx, request)
		if err != nil {
			return types.RespondContactRequestResult{}, err
		}
	}
	if err := insertCommandIdempotency(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey, commandTypeRespondContactRequest, commandHash, request.RequestID); err != nil {
		return types.RespondContactRequestResult{}, err
	}
	eventID, err := r.eventID()
	if err != nil {
		return types.RespondContactRequestResult{}, types.NewOutboxWriteFailed(err.Error())
	}
	if err := insertContactOutbox(ctx, tx, contactOutboxInput{
		EventID:          eventID,
		TenantID:         request.TenantID,
		AggregateType:    "CONTACT_REQUEST",
		AggregateID:      request.RequestID,
		AggregateVersion: 2,
		EventType:        eventTypeForDecision(command.Decision),
		PartitionKey:     partitionKeyFor(request.TenantID, request.SenderUserID, request.ReceiverUserID),
		CorrelationID:    command.AuthContext.RequestID,
		CausationID:      request.RequestID,
		TraceID:          command.AuthContext.TraceID,
		Payload:          responsePayload(request, expectedStatus, edgeVersion, r.now()),
	}); err != nil {
		return types.RespondContactRequestResult{}, err
	}
	return commitRespondResult(ctx, tx, types.RespondContactRequestResult{
		RequestID:      request.RequestID,
		TenantID:       request.TenantID,
		SenderUserID:   request.SenderUserID,
		ReceiverUserID: request.ReceiverUserID,
		Status:         expectedStatus,
	})
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
	query := `
SELECT
    contact_user_id,
    status,
    version,
    source_request_id,
    created_at,
    updated_at
FROM contact_edges
WHERE tenant_id = $1
  AND owner_user_id = $2
  AND status = 'ACTIVE'
`
	if hasCursor {
		query += `  AND contact_user_id > $4
`
		args = append(args, cursor.ContactUserID)
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
		if err := rows.Scan(&item.ContactUserID, &item.Status, &item.Version, &item.SourceRequestID, &createdAt, &updatedAt); err != nil {
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
    version
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
	)
	if err == pgx.ErrNoRows {
		return types.GetContactStateResult{}, types.NewContactRequestNotFound("contact state not found")
	}
	if err != nil {
		return types.GetContactStateResult{}, types.NewDBReadFailed(err.Error())
	}
	return result, nil
}

type commandHashPayload struct {
	Kind         string `json:"kind"`
	TenantID     string `json:"tenant_id"`
	UserID       string `json:"user_id"`
	TargetUserID string `json:"target_user_id,omitempty"`
	RequestID    string `json:"request_id,omitempty"`
	Decision     string `json:"decision,omitempty"`
	Message      string `json:"message,omitempty"`
}

func commandHash(payload commandHashPayload) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", types.NewInvalidArgument("invalid command payload")
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

type commandIdempotency struct {
	CommandType string
	CommandHash string
	ResultID    string
}

func findCommandIdempotency(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	userID types.UserID,
	idempotencyKey string,
) (commandIdempotency, bool, error) {
	var existing commandIdempotency
	err := tx.QueryRow(ctx, `
SELECT command_type, command_hash, result_id
FROM contact_command_idempotency
WHERE tenant_id = $1
  AND user_id = $2
  AND idempotency_key = $3
FOR UPDATE
`, tenantID, userID, idempotencyKey).Scan(&existing.CommandType, &existing.CommandHash, &existing.ResultID)
	if err == pgx.ErrNoRows {
		return commandIdempotency{}, false, nil
	}
	if err != nil {
		return commandIdempotency{}, false, types.NewDBReadFailed(err.Error())
	}
	return existing, true, nil
}

func insertCommandIdempotency(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	userID types.UserID,
	idempotencyKey string,
	commandType string,
	commandHash string,
	resultID string,
) error {
	_, err := tx.Exec(ctx, `
INSERT INTO contact_command_idempotency (
    tenant_id,
    user_id,
    idempotency_key,
    command_type,
    command_hash,
    result_id,
    created_at
) VALUES ($1, $2, $3, $4, $5, $6, now())
`, tenantID, userID, idempotencyKey, commandType, commandHash, resultID)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

type contactRequestRow struct {
	RequestID      string
	TenantID       types.TenantID
	SenderUserID   types.UserID
	ReceiverUserID types.UserID
	Status         types.ContactRequestStatus
	CommandHash    string
}

func getContactRequestResult(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, requestID string) (types.SendContactRequestResult, error) {
	row, err := getContactRequest(ctx, tx, tenantID, requestID)
	if err != nil {
		return types.SendContactRequestResult{}, err
	}
	return sendResultFromRequest(row, false), nil
}

func sendResultFromRequest(row contactRequestRow, replay bool) types.SendContactRequestResult {
	return types.SendContactRequestResult{
		RequestID:        row.RequestID,
		TenantID:         row.TenantID,
		SenderUserID:     row.SenderUserID,
		ReceiverUserID:   row.ReceiverUserID,
		Status:           row.Status,
		IdempotentReplay: replay,
	}
}

func getRespondContactRequestResult(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, requestID string) (types.RespondContactRequestResult, error) {
	row, err := getContactRequest(ctx, tx, tenantID, requestID)
	if err != nil {
		return types.RespondContactRequestResult{}, err
	}
	return respondResultFromRequest(row, false), nil
}

func getContactRequest(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, requestID string) (contactRequestRow, error) {
	var row contactRequestRow
	err := tx.QueryRow(ctx, `
SELECT request_id, tenant_id, sender_user_id, receiver_user_id, status
FROM contact_requests
WHERE tenant_id = $1
  AND request_id = $2
`, tenantID, requestID).Scan(&row.RequestID, &row.TenantID, &row.SenderUserID, &row.ReceiverUserID, &row.Status)
	if err == pgx.ErrNoRows {
		return contactRequestRow{}, types.NewContactRequestNotFound("contact request not found")
	}
	if err != nil {
		return contactRequestRow{}, types.NewDBReadFailed(err.Error())
	}
	return row, nil
}

func findContactRequestByIdempotency(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	senderUserID types.UserID,
	idempotencyKey string,
) (contactRequestRow, bool, error) {
	var row contactRequestRow
	err := tx.QueryRow(ctx, `
SELECT request_id, tenant_id, sender_user_id, receiver_user_id, status, command_hash
FROM contact_requests
WHERE tenant_id = $1
  AND sender_user_id = $2
  AND idempotency_key = $3
FOR UPDATE
`, tenantID, senderUserID, idempotencyKey).Scan(
		&row.RequestID,
		&row.TenantID,
		&row.SenderUserID,
		&row.ReceiverUserID,
		&row.Status,
		&row.CommandHash,
	)
	if err == pgx.ErrNoRows {
		return contactRequestRow{}, false, nil
	}
	if err != nil {
		return contactRequestRow{}, false, types.NewDBReadFailed(err.Error())
	}
	return row, true, nil
}

func lockContactRequest(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, requestID string) (contactRequestRow, error) {
	var row contactRequestRow
	err := tx.QueryRow(ctx, `
SELECT request_id, tenant_id, sender_user_id, receiver_user_id, status
FROM contact_requests
WHERE tenant_id = $1
  AND request_id = $2
FOR UPDATE
`, tenantID, requestID).Scan(&row.RequestID, &row.TenantID, &row.SenderUserID, &row.ReceiverUserID, &row.Status)
	if err == pgx.ErrNoRows {
		return contactRequestRow{}, types.NewContactRequestNotFound("contact request not found")
	}
	if err != nil {
		return contactRequestRow{}, types.NewDBReadFailed(err.Error())
	}
	return row, nil
}

func insertContactRequest(
	ctx context.Context,
	tx pgx.Tx,
	command types.SendContactRequestCommand,
	requestID string,
	commandHash string,
	now time.Time,
) error {
	_, err := tx.Exec(ctx, `
INSERT INTO contact_requests (
    request_id,
    tenant_id,
    sender_user_id,
    receiver_user_id,
    status,
    idempotency_key,
    command_hash,
    message,
    created_at,
    updated_at
) VALUES ($1, $2, $3, $4, 'PENDING', $5, $6, $7, $8, $8)
`, requestID, command.AuthContext.TenantID, command.AuthContext.UserID, command.TargetUserID, command.IdempotencyKey, commandHash, command.Message, now)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func activeContactExists(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, first types.UserID, second types.UserID) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM contact_edges
    WHERE tenant_id = $1
      AND owner_user_id = $2
      AND contact_user_id = $3
      AND status = 'ACTIVE'
)
`, tenantID, first, second).Scan(&exists)
	if err != nil {
		return false, types.NewDBReadFailed(err.Error())
	}
	return exists, nil
}

func blockedContactExists(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, first types.UserID, second types.UserID) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM contact_edges
    WHERE tenant_id = $1
      AND (
          (owner_user_id = $2 AND contact_user_id = $3)
          OR (owner_user_id = $3 AND contact_user_id = $2)
      )
      AND status = 'BLOCKED'
)
`, tenantID, first, second).Scan(&exists)
	if err != nil {
		return false, types.NewDBReadFailed(err.Error())
	}
	return exists, nil
}

func pendingContactRequestExists(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, first types.UserID, second types.UserID) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM contact_requests
    WHERE tenant_id = $1
      AND status = 'PENDING'
      AND LEAST(sender_user_id, receiver_user_id) = LEAST($2::text, $3::text)
      AND GREATEST(sender_user_id, receiver_user_id) = GREATEST($2::text, $3::text)
)
`, tenantID, first, second).Scan(&exists)
	if err != nil {
		return false, types.NewDBReadFailed(err.Error())
	}
	return exists, nil
}

func updateContactRequestStatus(ctx context.Context, tx pgx.Tx, request contactRequestRow, status types.ContactRequestStatus) error {
	_, err := tx.Exec(ctx, `
UPDATE contact_requests
SET status = $3,
    decided_at = now(),
    updated_at = now()
WHERE tenant_id = $1
  AND request_id = $2
`, request.TenantID, request.RequestID, status)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

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

type contactOutboxInput struct {
	EventID          string
	TenantID         types.TenantID
	AggregateType    string
	AggregateID      string
	AggregateVersion int64
	EventType        string
	PartitionKey     string
	CorrelationID    string
	CausationID      string
	TraceID          string
	Payload          map[string]any
}

func insertContactOutbox(ctx context.Context, tx pgx.Tx, input contactOutboxInput) error {
	payloadBytes, err := json.Marshal(input.Payload)
	if err != nil {
		return types.NewOutboxWriteFailed(err.Error())
	}
	_, err = tx.Exec(ctx, `
INSERT INTO contacts_outbox (
    event_id,
    tenant_id,
    aggregate_type,
    aggregate_id,
    aggregate_version,
    event_type,
    event_version,
    mapping_version,
    partition_key,
    producer,
    correlation_id,
    causation_id,
    trace_id,
    payload_json,
    status,
    available_at,
    created_at,
    updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'contacts-service', $10, $11, $12, $13, 'PENDING', now(), now(), now())
`, input.EventID, input.TenantID, input.AggregateType, input.AggregateID, input.AggregateVersion, input.EventType, contactsOutboxEventVersion, contactsOutboxMappingVersion, input.PartitionKey, input.CorrelationID, input.CausationID, input.TraceID, payloadBytes)
	if err != nil {
		return types.NewOutboxWriteFailed(err.Error())
	}
	return nil
}

func lockIdempotencyKey(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, idempotencyKey string) error {
	key := fmt.Sprintf("%s\x1f%s\x1f%s\x1fcontacts_idempotency", tenantID, userID, idempotencyKey)
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func lockContactPair(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, first types.UserID, second types.UserID) error {
	key := fmt.Sprintf("%s\x1f%s\x1fcontacts_pair", tenantID, canonicalPair(first, second))
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func commitSendResult(ctx context.Context, tx pgx.Tx, result types.SendContactRequestResult) (types.SendContactRequestResult, error) {
	if err := tx.Commit(ctx); err != nil {
		return types.SendContactRequestResult{}, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func commitRespondResult(ctx context.Context, tx pgx.Tx, result types.RespondContactRequestResult) (types.RespondContactRequestResult, error) {
	if err := tx.Commit(ctx); err != nil {
		return types.RespondContactRequestResult{}, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func respondResultFromRequest(request contactRequestRow, replay bool) types.RespondContactRequestResult {
	return types.RespondContactRequestResult{
		RequestID:        request.RequestID,
		TenantID:         request.TenantID,
		SenderUserID:     request.SenderUserID,
		ReceiverUserID:   request.ReceiverUserID,
		Status:           request.Status,
		IdempotentReplay: replay,
	}
}

func requestStatusForDecision(decision types.ContactDecision) types.ContactRequestStatus {
	if decision == types.ContactDecisionAccept {
		return types.ContactRequestStatusAccepted
	}
	return types.ContactRequestStatusDeclined
}

func eventTypeForDecision(decision types.ContactDecision) string {
	if decision == types.ContactDecisionAccept {
		return eventTypeContactRequestAccepted
	}
	return eventTypeContactRequestDeclined
}

func responsePayload(request contactRequestRow, status types.ContactRequestStatus, edgeVersion int64, occurredAt time.Time) map[string]any {
	payload := map[string]any{
		"tenant_id":        request.TenantID,
		"request_id":       request.RequestID,
		"sender_user_id":   request.SenderUserID,
		"receiver_user_id": request.ReceiverUserID,
		"status":           status,
		"occurred_at":      occurredAt.Format(time.RFC3339Nano),
	}
	if status == types.ContactRequestStatusAccepted {
		payload["edge_version"] = edgeVersion
	}
	return payload
}

type contactPageCursor struct {
	Version       int            `json:"v"`
	TenantID      types.TenantID `json:"tenant_id"`
	OwnerUserID   types.UserID   `json:"owner_user_id"`
	PageSize      int            `json:"page_size"`
	ContactUserID string         `json:"contact_user_id"`
}

func decodePageTokenFor(command types.ListContactsCommand, pageSize int) (contactPageCursor, bool, error) {
	value := command.PageToken
	if value == "" {
		return contactPageCursor{}, false, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return contactPageCursor{}, false, types.NewInvalidArgument("invalid page_token")
	}
	var cursor contactPageCursor
	if err := json.Unmarshal(raw, &cursor); err != nil {
		return contactPageCursor{}, false, types.NewInvalidArgument("invalid page_token")
	}
	if cursor.Version != 1 || cursor.ContactUserID == "" {
		return contactPageCursor{}, false, types.NewInvalidArgument("invalid page_token")
	}
	if cursor.TenantID != command.AuthContext.TenantID ||
		cursor.OwnerUserID != command.AuthContext.UserID ||
		cursor.PageSize != pageSize {
		return contactPageCursor{}, false, types.NewInvalidArgument("invalid page_token")
	}
	return cursor, true, nil
}

func encodePageToken(cursor contactPageCursor) string {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func partitionKeyFor(tenantID types.TenantID, first types.UserID, second types.UserID) string {
	return fmt.Sprintf("%s:%s", tenantID, canonicalPair(first, second))
}

func canonicalPair(first types.UserID, second types.UserID) string {
	values := []string{string(first), string(second)}
	sort.Strings(values)
	return values[0] + ":" + values[1]
}

func newID(prefix string) (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(bytes[:]), nil
}
