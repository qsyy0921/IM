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
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/contacts-service/internal/domain"
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

const (
	commandTypeSendContactRequest    = "SEND_CONTACT_REQUEST"
	commandTypeRespondContactRequest = "RESPOND_CONTACT_REQUEST"
	commandTypeCancelContactRequest  = "CANCEL_CONTACT_REQUEST"
	commandTypeDeleteContact         = "DELETE_CONTACT"
	commandTypeBlockContact          = "BLOCK_CONTACT"
	commandTypeUnblockContact        = "UNBLOCK_CONTACT"
	commandTypeUpdateContactRemark   = "UPDATE_CONTACT_REMARK"
	commandTypeUpdateContactGroup    = "UPDATE_CONTACT_GROUP"
	commandTypeSetContactPrivacy     = "SET_CONTACT_PRIVACY"

	eventTypeContactRequestCreated  = "contact.request.created.v1"
	eventTypeContactRequestAccepted = "contact.request.accepted.v1"
	eventTypeContactRequestDeclined = "contact.request.declined.v1"
	eventTypeContactRequestCanceled = "contact.request.canceled.v1"
	eventTypeContactEdgeDeleted     = "contact.edge.deleted.v1"
	eventTypeContactEdgeBlocked     = "contact.edge.blocked.v1"
	eventTypeContactEdgeUnblocked   = "contact.edge.unblocked.v1"
	eventTypeContactRemarkUpdated   = "contact.edge.remark_updated.v1"
	eventTypeContactGroupUpdated    = "contact.edge.group_updated.v1"
	eventTypeContactPrivacyUpdated  = "contact.privacy.updated.v1"

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
	commandHash, err := sendContactRequestCommandHash(command)
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
	if ok, err := blockedContactExists(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.TargetUserID); err != nil {
		return types.SendContactRequestResult{}, err
	} else if ok {
		return types.SendContactRequestResult{}, types.NewPermissionDenied("blocked contact edge exists")
	}
	if ok, err := pendingContactRequestExists(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.TargetUserID); err != nil {
		return types.SendContactRequestResult{}, err
	} else if ok {
		return types.SendContactRequestResult{}, types.NewContactRequestConflict("pending contact request already exists")
	}
	if allowed, err := contactRequestsAllowed(ctx, tx, command.AuthContext.TenantID, command.TargetUserID); err != nil {
		return types.SendContactRequestResult{}, err
	} else if !allowed {
		return types.SendContactRequestResult{}, types.NewPermissionDenied("target user does not accept contact requests")
	}
	if allowed, err := contactRequestSourceAllowed(ctx, tx, command.AuthContext.TenantID, command.NormalizedSourceType()); err != nil {
		return types.SendContactRequestResult{}, err
	} else if !allowed {
		return types.SendContactRequestResult{}, types.NewPermissionDenied("contact request source is not allowed")
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
	partitionKey := partitionKeyFor(command.AuthContext.TenantID, command.AuthContext.UserID, command.TargetUserID)
	aggregateVersion, err := nextContactOutboxAggregateVersion(ctx, tx, command.AuthContext.TenantID, partitionKey)
	if err != nil {
		return types.SendContactRequestResult{}, err
	}
	if err := insertContactOutbox(ctx, tx, contactOutboxInput{
		EventID:          eventID,
		TenantID:         command.AuthContext.TenantID,
		AggregateType:    "CONTACT_REQUEST",
		AggregateID:      requestID,
		AggregateVersion: aggregateVersion,
		EventType:        eventTypeContactRequestCreated,
		PartitionKey:     partitionKey,
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
			"source_type":      command.NormalizedSourceType(),
			"source_ref":       command.NormalizedSourceRef(),
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
		SourceType:     command.NormalizedSourceType(),
		SourceRef:      command.NormalizedSourceRef(),
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
			return types.RespondContactRequestResult{}, types.NewPermissionDenied("blocked contact edge exists")
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
	partitionKey := partitionKeyFor(request.TenantID, request.SenderUserID, request.ReceiverUserID)
	aggregateVersion, err := nextContactOutboxAggregateVersion(ctx, tx, request.TenantID, partitionKey)
	if err != nil {
		return types.RespondContactRequestResult{}, err
	}
	if err := insertContactOutbox(ctx, tx, contactOutboxInput{
		EventID:          eventID,
		TenantID:         request.TenantID,
		AggregateType:    "CONTACT_REQUEST",
		AggregateID:      request.RequestID,
		AggregateVersion: aggregateVersion,
		EventType:        eventTypeForDecision(command.Decision),
		PartitionKey:     partitionKey,
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

func (r *Repository) CancelContactRequest(
	ctx context.Context,
	command types.CancelContactRequestCommand,
) (types.CancelContactRequestResult, error) {
	if r.pool == nil {
		return types.CancelContactRequestResult{}, types.NewDBWriteFailed("contacts repository is not configured")
	}
	commandHash, err := commandHash(commandHashPayload{
		Kind:      commandTypeCancelContactRequest,
		TenantID:  string(command.AuthContext.TenantID),
		UserID:    string(command.AuthContext.UserID),
		RequestID: command.RequestID,
	})
	if err != nil {
		return types.CancelContactRequestResult{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.CancelContactRequestResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if err := lockIdempotencyKey(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey); err != nil {
		return types.CancelContactRequestResult{}, err
	}
	if existing, ok, err := findCommandIdempotency(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey); err != nil {
		return types.CancelContactRequestResult{}, err
	} else if ok {
		if existing.CommandType != commandTypeCancelContactRequest || existing.CommandHash != commandHash {
			return types.CancelContactRequestResult{}, types.NewContactRequestConflict("idempotency key conflict")
		}
		result, err := getCancelContactRequestResult(ctx, tx, command.AuthContext.TenantID, existing.ResultID)
		if err != nil {
			return types.CancelContactRequestResult{}, err
		}
		result.IdempotentReplay = true
		return commitCancelResult(ctx, tx, result)
	}
	request, err := lockContactRequest(ctx, tx, command.AuthContext.TenantID, command.RequestID)
	if err != nil {
		return types.CancelContactRequestResult{}, err
	}
	if request.SenderUserID != command.AuthContext.UserID {
		return types.CancelContactRequestResult{}, types.NewPermissionDenied("only request sender can cancel")
	}
	if err := lockContactPair(ctx, tx, request.TenantID, request.SenderUserID, request.ReceiverUserID); err != nil {
		return types.CancelContactRequestResult{}, err
	}
	if request.Status != types.ContactRequestStatusPending {
		if request.Status != types.ContactRequestStatusCanceled {
			return types.CancelContactRequestResult{}, types.NewContactRequestConflict("contact request already completed with a different status")
		}
		if err := insertCommandIdempotency(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey, commandTypeCancelContactRequest, commandHash, request.RequestID); err != nil {
			return types.CancelContactRequestResult{}, err
		}
		return commitCancelResult(ctx, tx, cancelResultFromRequest(request, true))
	}

	if err := updateContactRequestStatus(ctx, tx, request, types.ContactRequestStatusCanceled); err != nil {
		return types.CancelContactRequestResult{}, err
	}
	if err := insertCommandIdempotency(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey, commandTypeCancelContactRequest, commandHash, request.RequestID); err != nil {
		return types.CancelContactRequestResult{}, err
	}
	eventID, err := r.eventID()
	if err != nil {
		return types.CancelContactRequestResult{}, types.NewOutboxWriteFailed(err.Error())
	}
	partitionKey := partitionKeyFor(request.TenantID, request.SenderUserID, request.ReceiverUserID)
	aggregateVersion, err := nextContactOutboxAggregateVersion(ctx, tx, request.TenantID, partitionKey)
	if err != nil {
		return types.CancelContactRequestResult{}, err
	}
	if err := insertContactOutbox(ctx, tx, contactOutboxInput{
		EventID:          eventID,
		TenantID:         request.TenantID,
		AggregateType:    "CONTACT_REQUEST",
		AggregateID:      request.RequestID,
		AggregateVersion: aggregateVersion,
		EventType:        eventTypeContactRequestCanceled,
		PartitionKey:     partitionKey,
		CorrelationID:    command.AuthContext.RequestID,
		CausationID:      request.RequestID,
		TraceID:          command.AuthContext.TraceID,
		Payload:          responsePayload(request, types.ContactRequestStatusCanceled, 0, r.now()),
	}); err != nil {
		return types.CancelContactRequestResult{}, err
	}
	return commitCancelResult(ctx, tx, types.CancelContactRequestResult{
		RequestID:      request.RequestID,
		TenantID:       request.TenantID,
		SenderUserID:   request.SenderUserID,
		ReceiverUserID: request.ReceiverUserID,
		Status:         types.ContactRequestStatusCanceled,
	})
}

func (r *Repository) ListContactRequests(
	ctx context.Context,
	command types.ListContactRequestsCommand,
) (types.ListContactRequestsResult, error) {
	if r.pool == nil {
		return types.ListContactRequestsResult{}, types.NewDBReadFailed("contacts repository is not configured")
	}
	direction := command.NormalizedDirection()
	status := command.NormalizedStatus()
	limit := domain.NormalizePageSize(command.PageSize)
	cursor, hasCursor, err := decodeContactRequestPageTokenFor(command, direction, status, limit)
	if err != nil {
		return types.ListContactRequestsResult{}, err
	}

	userColumn := "receiver_user_id"
	if direction == types.ContactRequestListDirectionOutgoing {
		userColumn = "sender_user_id"
	}
	args := []any{command.AuthContext.TenantID, command.AuthContext.UserID, status, limit + 1}
	query := fmt.Sprintf(`
SELECT
    request_id,
    sender_user_id,
    receiver_user_id,
    status,
    message,
    source_type,
    source_ref,
    created_at,
    updated_at,
    decided_at IS NOT NULL AS has_decided_at,
    COALESCE(decided_at, 'epoch'::timestamptz) AS decided_at
FROM contact_requests
WHERE tenant_id = $1
  AND %s = $2
  AND status = $3
`, userColumn)
	if hasCursor {
		query += `  AND (created_at < $5 OR (created_at = $5 AND request_id > $6))
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
			Version:   1,
			TenantID:  command.AuthContext.TenantID,
			UserID:    command.AuthContext.UserID,
			Direction: direction,
			Status:    status,
			PageSize:  limit,
			CreatedAt: last.createdAt,
			RequestID: last.item.RequestID,
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

func (r *Repository) DeleteContact(
	ctx context.Context,
	command types.DeleteContactCommand,
) (types.DeleteContactResult, error) {
	if r.pool == nil {
		return types.DeleteContactResult{}, types.NewDBWriteFailed("contacts repository is not configured")
	}
	commandHash, err := commandHash(commandHashPayload{
		Kind:          commandTypeDeleteContact,
		TenantID:      string(command.AuthContext.TenantID),
		UserID:        string(command.AuthContext.UserID),
		ContactUserID: string(command.ContactUserID),
	})
	if err != nil {
		return types.DeleteContactResult{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.DeleteContactResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if err := lockIdempotencyKey(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey); err != nil {
		return types.DeleteContactResult{}, err
	}
	if existing, ok, err := findCommandIdempotency(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey); err != nil {
		return types.DeleteContactResult{}, err
	} else if ok {
		if existing.CommandType != commandTypeDeleteContact || existing.CommandHash != commandHash {
			return types.DeleteContactResult{}, types.NewContactRequestConflict("idempotency key conflict")
		}
		row, err := contactEdgeRowFromIdempotencyResult(existing)
		if err != nil {
			return types.DeleteContactResult{}, err
		}
		return commitDeleteContactResult(ctx, tx, deleteContactResultFromEdge(row, true))
	}
	if err := lockContactPair(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.ContactUserID); err != nil {
		return types.DeleteContactResult{}, err
	}
	row, err := lockContactEdge(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.ContactUserID)
	if err != nil {
		return types.DeleteContactResult{}, err
	}
	if row.Status != types.ContactEdgeStatusActive {
		return types.DeleteContactResult{}, types.NewContactNotFound("active contact edge not found")
	}
	updated, err := updateContactEdgeStatus(ctx, tx, row, types.ContactEdgeStatusDeleted)
	if err != nil {
		return types.DeleteContactResult{}, err
	}
	resultJSON, err := edgeResultJSON(updated)
	if err != nil {
		return types.DeleteContactResult{}, err
	}
	if err := insertCommandIdempotencyWithResult(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey, commandTypeDeleteContact, commandHash, contactEdgeID(command.AuthContext.UserID, command.ContactUserID), resultJSON); err != nil {
		return types.DeleteContactResult{}, err
	}
	if err := r.insertEdgeOutbox(ctx, tx, edgeOutboxInput{
		TenantID:       command.AuthContext.TenantID,
		OwnerUserID:    command.AuthContext.UserID,
		ContactUserID:  command.ContactUserID,
		EventType:      eventTypeContactEdgeDeleted,
		CorrelationID:  command.AuthContext.RequestID,
		CausationID:    command.AuthContext.RequestID,
		TraceID:        command.AuthContext.TraceID,
		PreviousStatus: row.Status,
		Edge:           updated,
	}); err != nil {
		return types.DeleteContactResult{}, err
	}
	return commitDeleteContactResult(ctx, tx, deleteContactResultFromEdge(updated, false))
}

func (r *Repository) BlockContact(
	ctx context.Context,
	command types.BlockContactCommand,
) (types.BlockContactResult, error) {
	if r.pool == nil {
		return types.BlockContactResult{}, types.NewDBWriteFailed("contacts repository is not configured")
	}
	commandHash, err := commandHash(commandHashPayload{
		Kind:          commandTypeBlockContact,
		TenantID:      string(command.AuthContext.TenantID),
		UserID:        string(command.AuthContext.UserID),
		ContactUserID: string(command.ContactUserID),
		Reason:        command.Reason,
	})
	if err != nil {
		return types.BlockContactResult{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.BlockContactResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if err := lockIdempotencyKey(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey); err != nil {
		return types.BlockContactResult{}, err
	}
	if existing, ok, err := findCommandIdempotency(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey); err != nil {
		return types.BlockContactResult{}, err
	} else if ok {
		if existing.CommandType != commandTypeBlockContact || existing.CommandHash != commandHash {
			return types.BlockContactResult{}, types.NewContactRequestConflict("idempotency key conflict")
		}
		row, err := contactEdgeRowFromIdempotencyResult(existing)
		if err != nil {
			return types.BlockContactResult{}, err
		}
		return commitBlockContactResult(ctx, tx, blockContactResultFromEdge(row, true))
	}
	if err := lockContactPair(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.ContactUserID); err != nil {
		return types.BlockContactResult{}, err
	}
	row, err := lockContactEdge(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.ContactUserID)
	if err != nil {
		return types.BlockContactResult{}, err
	}
	if row.Status != types.ContactEdgeStatusActive {
		return types.BlockContactResult{}, types.NewContactNotFound("active contact edge not found")
	}
	updated, err := updateContactEdgeStatus(ctx, tx, row, types.ContactEdgeStatusBlocked)
	if err != nil {
		return types.BlockContactResult{}, err
	}
	resultJSON, err := edgeResultJSON(updated)
	if err != nil {
		return types.BlockContactResult{}, err
	}
	if err := insertCommandIdempotencyWithResult(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey, commandTypeBlockContact, commandHash, contactEdgeID(command.AuthContext.UserID, command.ContactUserID), resultJSON); err != nil {
		return types.BlockContactResult{}, err
	}
	if err := r.insertEdgeOutbox(ctx, tx, edgeOutboxInput{
		TenantID:       command.AuthContext.TenantID,
		OwnerUserID:    command.AuthContext.UserID,
		ContactUserID:  command.ContactUserID,
		EventType:      eventTypeContactEdgeBlocked,
		CorrelationID:  command.AuthContext.RequestID,
		CausationID:    command.AuthContext.RequestID,
		TraceID:        command.AuthContext.TraceID,
		PreviousStatus: row.Status,
		Edge:           updated,
		Reason:         command.Reason,
	}); err != nil {
		return types.BlockContactResult{}, err
	}
	return commitBlockContactResult(ctx, tx, blockContactResultFromEdge(updated, false))
}

func (r *Repository) UnblockContact(
	ctx context.Context,
	command types.UnblockContactCommand,
) (types.UnblockContactResult, error) {
	if r.pool == nil {
		return types.UnblockContactResult{}, types.NewDBWriteFailed("contacts repository is not configured")
	}
	commandHash, err := commandHash(commandHashPayload{
		Kind:          commandTypeUnblockContact,
		TenantID:      string(command.AuthContext.TenantID),
		UserID:        string(command.AuthContext.UserID),
		ContactUserID: string(command.ContactUserID),
	})
	if err != nil {
		return types.UnblockContactResult{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.UnblockContactResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if err := lockIdempotencyKey(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey); err != nil {
		return types.UnblockContactResult{}, err
	}
	if existing, ok, err := findCommandIdempotency(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey); err != nil {
		return types.UnblockContactResult{}, err
	} else if ok {
		if existing.CommandType != commandTypeUnblockContact || existing.CommandHash != commandHash {
			return types.UnblockContactResult{}, types.NewContactRequestConflict("idempotency key conflict")
		}
		row, err := contactEdgeRowFromIdempotencyResult(existing)
		if err != nil {
			return types.UnblockContactResult{}, err
		}
		return commitUnblockContactResult(ctx, tx, unblockContactResultFromEdge(row, true))
	}
	if err := lockContactPair(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.ContactUserID); err != nil {
		return types.UnblockContactResult{}, err
	}
	row, err := lockContactEdge(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.ContactUserID)
	if err != nil {
		return types.UnblockContactResult{}, err
	}
	if row.Status != types.ContactEdgeStatusBlocked {
		return types.UnblockContactResult{}, types.NewContactNotFound("blocked contact edge not found")
	}
	updated, err := updateContactEdgeStatus(ctx, tx, row, types.ContactEdgeStatusActive)
	if err != nil {
		return types.UnblockContactResult{}, err
	}
	resultJSON, err := edgeResultJSON(updated)
	if err != nil {
		return types.UnblockContactResult{}, err
	}
	if err := insertCommandIdempotencyWithResult(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey, commandTypeUnblockContact, commandHash, contactEdgeID(command.AuthContext.UserID, command.ContactUserID), resultJSON); err != nil {
		return types.UnblockContactResult{}, err
	}
	if err := r.insertEdgeOutbox(ctx, tx, edgeOutboxInput{
		TenantID:       command.AuthContext.TenantID,
		OwnerUserID:    command.AuthContext.UserID,
		ContactUserID:  command.ContactUserID,
		EventType:      eventTypeContactEdgeUnblocked,
		CorrelationID:  command.AuthContext.RequestID,
		CausationID:    command.AuthContext.RequestID,
		TraceID:        command.AuthContext.TraceID,
		PreviousStatus: row.Status,
		Edge:           updated,
	}); err != nil {
		return types.UnblockContactResult{}, err
	}
	return commitUnblockContactResult(ctx, tx, unblockContactResultFromEdge(updated, false))
}

func (r *Repository) UpdateContactRemark(
	ctx context.Context,
	command types.UpdateContactRemarkCommand,
) (types.UpdateContactRemarkResult, error) {
	if r.pool == nil {
		return types.UpdateContactRemarkResult{}, types.NewDBWriteFailed("contacts repository is not configured")
	}
	commandHash, err := commandHash(commandHashPayload{
		Kind:          commandTypeUpdateContactRemark,
		TenantID:      string(command.AuthContext.TenantID),
		UserID:        string(command.AuthContext.UserID),
		ContactUserID: string(command.ContactUserID),
		Remark:        command.Remark,
	})
	if err != nil {
		return types.UpdateContactRemarkResult{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.UpdateContactRemarkResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if err := lockIdempotencyKey(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey); err != nil {
		return types.UpdateContactRemarkResult{}, err
	}
	if existing, ok, err := findCommandIdempotency(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey); err != nil {
		return types.UpdateContactRemarkResult{}, err
	} else if ok {
		if existing.CommandType != commandTypeUpdateContactRemark || existing.CommandHash != commandHash {
			return types.UpdateContactRemarkResult{}, types.NewContactRequestConflict("idempotency key conflict")
		}
		row, err := contactEdgeRowFromIdempotencyResult(existing)
		if err != nil {
			return types.UpdateContactRemarkResult{}, err
		}
		return commitUpdateContactRemarkResult(ctx, tx, updateContactRemarkResultFromEdge(row, true))
	}
	if err := lockContactPair(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.ContactUserID); err != nil {
		return types.UpdateContactRemarkResult{}, err
	}
	row, err := lockContactEdge(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.ContactUserID)
	if err != nil {
		return types.UpdateContactRemarkResult{}, err
	}
	if row.Status != types.ContactEdgeStatusActive {
		return types.UpdateContactRemarkResult{}, types.NewContactNotFound("active contact edge not found")
	}
	if row.Remark == command.Remark {
		resultJSON, err := edgeResultJSON(row)
		if err != nil {
			return types.UpdateContactRemarkResult{}, err
		}
		if err := insertCommandIdempotencyWithResult(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey, commandTypeUpdateContactRemark, commandHash, contactEdgeID(command.AuthContext.UserID, command.ContactUserID), resultJSON); err != nil {
			return types.UpdateContactRemarkResult{}, err
		}
		return commitUpdateContactRemarkResult(ctx, tx, updateContactRemarkResultFromEdge(row, false))
	}
	updated, err := updateContactEdgeRemark(ctx, tx, row, command.Remark)
	if err != nil {
		return types.UpdateContactRemarkResult{}, err
	}
	resultJSON, err := edgeResultJSON(updated)
	if err != nil {
		return types.UpdateContactRemarkResult{}, err
	}
	if err := insertCommandIdempotencyWithResult(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey, commandTypeUpdateContactRemark, commandHash, contactEdgeID(command.AuthContext.UserID, command.ContactUserID), resultJSON); err != nil {
		return types.UpdateContactRemarkResult{}, err
	}
	if err := r.insertEdgeOutbox(ctx, tx, edgeOutboxInput{
		TenantID:      command.AuthContext.TenantID,
		OwnerUserID:   command.AuthContext.UserID,
		ContactUserID: command.ContactUserID,
		EventType:     eventTypeContactRemarkUpdated,
		CorrelationID: command.AuthContext.RequestID,
		CausationID:   command.AuthContext.RequestID,
		TraceID:       command.AuthContext.TraceID,
		Edge:          updated,
	}); err != nil {
		return types.UpdateContactRemarkResult{}, err
	}
	return commitUpdateContactRemarkResult(ctx, tx, updateContactRemarkResultFromEdge(updated, false))
}

func (r *Repository) UpdateContactGroup(
	ctx context.Context,
	command types.UpdateContactGroupCommand,
) (types.UpdateContactGroupResult, error) {
	if r.pool == nil {
		return types.UpdateContactGroupResult{}, types.NewDBWriteFailed("contacts repository is not configured")
	}
	groupName := command.NormalizedGroupName()
	commandHash, err := commandHash(commandHashPayload{
		Kind:          commandTypeUpdateContactGroup,
		TenantID:      string(command.AuthContext.TenantID),
		UserID:        string(command.AuthContext.UserID),
		ContactUserID: string(command.ContactUserID),
		GroupName:     groupName,
	})
	if err != nil {
		return types.UpdateContactGroupResult{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.UpdateContactGroupResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if err := lockIdempotencyKey(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey); err != nil {
		return types.UpdateContactGroupResult{}, err
	}
	if existing, ok, err := findCommandIdempotency(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey); err != nil {
		return types.UpdateContactGroupResult{}, err
	} else if ok {
		if existing.CommandType != commandTypeUpdateContactGroup || existing.CommandHash != commandHash {
			return types.UpdateContactGroupResult{}, types.NewContactRequestConflict("idempotency key conflict")
		}
		row, err := contactEdgeRowFromIdempotencyResult(existing)
		if err != nil {
			return types.UpdateContactGroupResult{}, err
		}
		return commitUpdateContactGroupResult(ctx, tx, updateContactGroupResultFromEdge(row, true))
	}
	if err := lockContactPair(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.ContactUserID); err != nil {
		return types.UpdateContactGroupResult{}, err
	}
	row, err := lockContactEdge(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.ContactUserID)
	if err != nil {
		return types.UpdateContactGroupResult{}, err
	}
	if row.Status != types.ContactEdgeStatusActive {
		return types.UpdateContactGroupResult{}, types.NewContactNotFound("active contact edge not found")
	}
	if row.GroupName == groupName {
		resultJSON, err := edgeResultJSON(row)
		if err != nil {
			return types.UpdateContactGroupResult{}, err
		}
		if err := insertCommandIdempotencyWithResult(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey, commandTypeUpdateContactGroup, commandHash, contactEdgeID(command.AuthContext.UserID, command.ContactUserID), resultJSON); err != nil {
			return types.UpdateContactGroupResult{}, err
		}
		return commitUpdateContactGroupResult(ctx, tx, updateContactGroupResultFromEdge(row, false))
	}
	updated, err := updateContactEdgeGroup(ctx, tx, row, groupName)
	if err != nil {
		return types.UpdateContactGroupResult{}, err
	}
	resultJSON, err := edgeResultJSON(updated)
	if err != nil {
		return types.UpdateContactGroupResult{}, err
	}
	if err := insertCommandIdempotencyWithResult(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.IdempotencyKey, commandTypeUpdateContactGroup, commandHash, contactEdgeID(command.AuthContext.UserID, command.ContactUserID), resultJSON); err != nil {
		return types.UpdateContactGroupResult{}, err
	}
	if err := r.insertEdgeOutbox(ctx, tx, edgeOutboxInput{
		TenantID:      command.AuthContext.TenantID,
		OwnerUserID:   command.AuthContext.UserID,
		ContactUserID: command.ContactUserID,
		EventType:     eventTypeContactGroupUpdated,
		CorrelationID: command.AuthContext.RequestID,
		CausationID:   command.AuthContext.RequestID,
		TraceID:       command.AuthContext.TraceID,
		Edge:          updated,
	}); err != nil {
		return types.UpdateContactGroupResult{}, err
	}
	return commitUpdateContactGroupResult(ctx, tx, updateContactGroupResultFromEdge(updated, false))
}

type commandHashPayload struct {
	Kind                 string `json:"kind"`
	TenantID             string `json:"tenant_id"`
	UserID               string `json:"user_id"`
	TargetUserID         string `json:"target_user_id,omitempty"`
	ContactUserID        string `json:"contact_user_id,omitempty"`
	RequestID            string `json:"request_id,omitempty"`
	Decision             string `json:"decision,omitempty"`
	Message              string `json:"message,omitempty"`
	SourceType           string `json:"source_type,omitempty"`
	SourceRef            string `json:"source_ref,omitempty"`
	Reason               string `json:"reason,omitempty"`
	Remark               string `json:"remark,omitempty"`
	GroupName            string `json:"group_name,omitempty"`
	AllowContactRequests *bool  `json:"allow_contact_requests,omitempty"`
}

func commandHash(payload commandHashPayload) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", types.NewInvalidArgument("invalid command payload")
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func sendContactRequestCommandHash(command types.SendContactRequestCommand) (string, error) {
	payload := commandHashPayload{
		Kind:         commandTypeSendContactRequest,
		TenantID:     string(command.AuthContext.TenantID),
		UserID:       string(command.AuthContext.UserID),
		TargetUserID: string(command.TargetUserID),
		Message:      command.Message,
	}
	sourceType := command.NormalizedSourceType()
	sourceRef := command.NormalizedSourceRef()
	if sourceType != types.ContactRequestSourceTypeDirect || sourceRef != "" {
		payload.SourceType = string(sourceType)
		payload.SourceRef = sourceRef
	}
	return commandHash(payload)
}

type commandIdempotency struct {
	CommandType string
	CommandHash string
	ResultID    string
	ResultJSON  []byte
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
SELECT command_type, command_hash, result_id, result_json
FROM contact_command_idempotency
WHERE tenant_id = $1
  AND user_id = $2
  AND idempotency_key = $3
FOR UPDATE
`, tenantID, userID, idempotencyKey).Scan(&existing.CommandType, &existing.CommandHash, &existing.ResultID, &existing.ResultJSON)
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
	return insertCommandIdempotencyWithResult(ctx, tx, tenantID, userID, idempotencyKey, commandType, commandHash, resultID, nil)
}

func insertCommandIdempotencyWithResult(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	userID types.UserID,
	idempotencyKey string,
	commandType string,
	commandHash string,
	resultID string,
	resultJSON []byte,
) error {
	if len(resultJSON) == 0 {
		resultJSON = []byte(`{}`)
	}
	_, err := tx.Exec(ctx, `
INSERT INTO contact_command_idempotency (
    tenant_id,
    user_id,
    idempotency_key,
    command_type,
    command_hash,
    result_id,
    result_json,
    created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, now())
`, tenantID, userID, idempotencyKey, commandType, commandHash, resultID, resultJSON)
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
	SourceType     types.ContactRequestSourceType
	SourceRef      string
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
		SourceType:       row.SourceType,
		SourceRef:        row.SourceRef,
	}
}

func getRespondContactRequestResult(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, requestID string) (types.RespondContactRequestResult, error) {
	row, err := getContactRequest(ctx, tx, tenantID, requestID)
	if err != nil {
		return types.RespondContactRequestResult{}, err
	}
	return respondResultFromRequest(row, false), nil
}

func getCancelContactRequestResult(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, requestID string) (types.CancelContactRequestResult, error) {
	row, err := getContactRequest(ctx, tx, tenantID, requestID)
	if err != nil {
		return types.CancelContactRequestResult{}, err
	}
	return cancelResultFromRequest(row, false), nil
}

func getContactRequest(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, requestID string) (contactRequestRow, error) {
	var row contactRequestRow
	err := tx.QueryRow(ctx, `
SELECT request_id, tenant_id, sender_user_id, receiver_user_id, status, source_type, source_ref
FROM contact_requests
WHERE tenant_id = $1
  AND request_id = $2
`, tenantID, requestID).Scan(
		&row.RequestID,
		&row.TenantID,
		&row.SenderUserID,
		&row.ReceiverUserID,
		&row.Status,
		&row.SourceType,
		&row.SourceRef,
	)
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
SELECT request_id, tenant_id, sender_user_id, receiver_user_id, status, command_hash, source_type, source_ref
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
		&row.SourceType,
		&row.SourceRef,
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
SELECT request_id, tenant_id, sender_user_id, receiver_user_id, status, source_type, source_ref
FROM contact_requests
WHERE tenant_id = $1
  AND request_id = $2
FOR UPDATE
`, tenantID, requestID).Scan(
		&row.RequestID,
		&row.TenantID,
		&row.SenderUserID,
		&row.ReceiverUserID,
		&row.Status,
		&row.SourceType,
		&row.SourceRef,
	)
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
    source_type,
    source_ref,
    created_at,
    updated_at
) VALUES ($1, $2, $3, $4, 'PENDING', $5, $6, $7, $8, $9, $10, $10)
`, requestID, command.AuthContext.TenantID, command.AuthContext.UserID, command.TargetUserID, command.IdempotencyKey, commandHash, command.Message, command.NormalizedSourceType(), command.NormalizedSourceRef(), now)
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

type edgeOutboxInput struct {
	TenantID       types.TenantID
	OwnerUserID    types.UserID
	ContactUserID  types.UserID
	EventType      string
	CorrelationID  string
	CausationID    string
	TraceID        string
	PreviousStatus types.ContactEdgeStatus
	Edge           contactEdgeRow
	Reason         string
}

func (r *Repository) insertEdgeOutbox(ctx context.Context, tx pgx.Tx, input edgeOutboxInput) error {
	eventID, err := r.eventID()
	if err != nil {
		return types.NewOutboxWriteFailed(err.Error())
	}
	partitionKey := partitionKeyFor(input.TenantID, input.OwnerUserID, input.ContactUserID)
	aggregateVersion, err := nextContactOutboxAggregateVersion(ctx, tx, input.TenantID, partitionKey)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"tenant_id":       input.TenantID,
		"owner_user_id":   input.OwnerUserID,
		"contact_user_id": input.ContactUserID,
		"status":          input.Edge.Status,
		"edge_version":    input.Edge.Version,
		"remark":          input.Edge.Remark,
		"group_name":      input.Edge.GroupName,
		"occurred_at":     r.now().Format(time.RFC3339Nano),
	}
	if input.PreviousStatus != "" {
		payload["previous_status"] = input.PreviousStatus
	}
	if input.Reason != "" {
		payload["reason"] = input.Reason
	}
	return insertContactOutbox(ctx, tx, contactOutboxInput{
		EventID:          eventID,
		TenantID:         input.TenantID,
		AggregateType:    "CONTACT_EDGE",
		AggregateID:      contactEdgeID(input.OwnerUserID, input.ContactUserID),
		AggregateVersion: aggregateVersion,
		EventType:        input.EventType,
		PartitionKey:     partitionKey,
		CorrelationID:    input.CorrelationID,
		CausationID:      input.CausationID,
		TraceID:          input.TraceID,
		Payload:          payload,
	})
}

func nextContactOutboxAggregateVersion(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, partitionKey string) (int64, error) {
	var version int64
	err := tx.QueryRow(ctx, `
SELECT COALESCE(MAX(aggregate_version), 0) + 1
FROM contacts_outbox
WHERE tenant_id = $1
  AND partition_key = $2
`, tenantID, partitionKey).Scan(&version)
	if err != nil {
		return 0, types.NewDBReadFailed(err.Error())
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

func commitCancelResult(ctx context.Context, tx pgx.Tx, result types.CancelContactRequestResult) (types.CancelContactRequestResult, error) {
	if err := tx.Commit(ctx); err != nil {
		return types.CancelContactRequestResult{}, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func commitDeleteContactResult(ctx context.Context, tx pgx.Tx, result types.DeleteContactResult) (types.DeleteContactResult, error) {
	if err := tx.Commit(ctx); err != nil {
		return types.DeleteContactResult{}, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func commitBlockContactResult(ctx context.Context, tx pgx.Tx, result types.BlockContactResult) (types.BlockContactResult, error) {
	if err := tx.Commit(ctx); err != nil {
		return types.BlockContactResult{}, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func commitUnblockContactResult(ctx context.Context, tx pgx.Tx, result types.UnblockContactResult) (types.UnblockContactResult, error) {
	if err := tx.Commit(ctx); err != nil {
		return types.UnblockContactResult{}, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func commitUpdateContactRemarkResult(ctx context.Context, tx pgx.Tx, result types.UpdateContactRemarkResult) (types.UpdateContactRemarkResult, error) {
	if err := tx.Commit(ctx); err != nil {
		return types.UpdateContactRemarkResult{}, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func commitUpdateContactGroupResult(ctx context.Context, tx pgx.Tx, result types.UpdateContactGroupResult) (types.UpdateContactGroupResult, error) {
	if err := tx.Commit(ctx); err != nil {
		return types.UpdateContactGroupResult{}, types.NewDBWriteFailed(err.Error())
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

func cancelResultFromRequest(request contactRequestRow, replay bool) types.CancelContactRequestResult {
	return types.CancelContactRequestResult{
		RequestID:        request.RequestID,
		TenantID:         request.TenantID,
		SenderUserID:     request.SenderUserID,
		ReceiverUserID:   request.ReceiverUserID,
		Status:           request.Status,
		IdempotentReplay: replay,
	}
}

func deleteContactResultFromEdge(row contactEdgeRow, replay bool) types.DeleteContactResult {
	return types.DeleteContactResult{
		TenantID:         row.TenantID,
		OwnerUserID:      row.OwnerUserID,
		ContactUserID:    row.ContactUserID,
		Status:           row.Status,
		SourceRequestID:  row.SourceRequestID,
		Version:          row.Version,
		IdempotentReplay: replay,
	}
}

func blockContactResultFromEdge(row contactEdgeRow, replay bool) types.BlockContactResult {
	return types.BlockContactResult{
		TenantID:         row.TenantID,
		OwnerUserID:      row.OwnerUserID,
		ContactUserID:    row.ContactUserID,
		Status:           row.Status,
		SourceRequestID:  row.SourceRequestID,
		Version:          row.Version,
		IdempotentReplay: replay,
	}
}

func unblockContactResultFromEdge(row contactEdgeRow, replay bool) types.UnblockContactResult {
	return types.UnblockContactResult{
		TenantID:         row.TenantID,
		OwnerUserID:      row.OwnerUserID,
		ContactUserID:    row.ContactUserID,
		Status:           row.Status,
		SourceRequestID:  row.SourceRequestID,
		Version:          row.Version,
		IdempotentReplay: replay,
	}
}

func updateContactRemarkResultFromEdge(row contactEdgeRow, replay bool) types.UpdateContactRemarkResult {
	return types.UpdateContactRemarkResult{
		TenantID:         row.TenantID,
		OwnerUserID:      row.OwnerUserID,
		ContactUserID:    row.ContactUserID,
		Status:           row.Status,
		SourceRequestID:  row.SourceRequestID,
		Version:          row.Version,
		Remark:           row.Remark,
		IdempotentReplay: replay,
	}
}

func updateContactGroupResultFromEdge(row contactEdgeRow, replay bool) types.UpdateContactGroupResult {
	return types.UpdateContactGroupResult{
		TenantID:         row.TenantID,
		OwnerUserID:      row.OwnerUserID,
		ContactUserID:    row.ContactUserID,
		Status:           row.Status,
		SourceRequestID:  row.SourceRequestID,
		Version:          row.Version,
		GroupName:        row.GroupName,
		IdempotentReplay: replay,
	}
}

type contactEdgeResultSnapshot struct {
	TenantID        types.TenantID          `json:"tenant_id"`
	OwnerUserID     types.UserID            `json:"owner_user_id"`
	ContactUserID   types.UserID            `json:"contact_user_id"`
	Status          types.ContactEdgeStatus `json:"status"`
	SourceRequestID string                  `json:"source_request_id"`
	Version         int64                   `json:"version"`
	Remark          string                  `json:"remark"`
	GroupName       string                  `json:"group_name"`
}

func edgeResultJSON(row contactEdgeRow) ([]byte, error) {
	raw, err := json.Marshal(contactEdgeResultSnapshot{
		TenantID:        row.TenantID,
		OwnerUserID:     row.OwnerUserID,
		ContactUserID:   row.ContactUserID,
		Status:          row.Status,
		SourceRequestID: row.SourceRequestID,
		Version:         row.Version,
		Remark:          row.Remark,
		GroupName:       row.GroupName,
	})
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	return raw, nil
}

func contactEdgeRowFromIdempotencyResult(existing commandIdempotency) (contactEdgeRow, error) {
	var snapshot contactEdgeResultSnapshot
	if len(existing.ResultJSON) == 0 || string(existing.ResultJSON) == "{}" {
		return contactEdgeRow{}, types.NewDBReadFailed("contact edge idempotency result snapshot missing")
	}
	if err := json.Unmarshal(existing.ResultJSON, &snapshot); err != nil {
		return contactEdgeRow{}, types.NewDBReadFailed(err.Error())
	}
	if snapshot.TenantID == "" || snapshot.OwnerUserID == "" || snapshot.ContactUserID == "" || snapshot.Status == "" || snapshot.Version <= 0 {
		return contactEdgeRow{}, types.NewDBReadFailed("contact edge idempotency result snapshot incomplete")
	}
	return contactEdgeRow{
		TenantID:        snapshot.TenantID,
		OwnerUserID:     snapshot.OwnerUserID,
		ContactUserID:   snapshot.ContactUserID,
		Status:          snapshot.Status,
		SourceRequestID: snapshot.SourceRequestID,
		Version:         snapshot.Version,
		Remark:          snapshot.Remark,
		GroupName:       snapshot.GroupName,
	}, nil
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
	Query         string         `json:"query,omitempty"`
	GroupName     string         `json:"group_name,omitempty"`
	ContactUserID string         `json:"contact_user_id"`
}

type contactRequestPageCursor struct {
	Version   int                               `json:"v"`
	TenantID  types.TenantID                    `json:"tenant_id"`
	UserID    types.UserID                      `json:"user_id"`
	Direction types.ContactRequestListDirection `json:"direction"`
	Status    types.ContactRequestStatus        `json:"status"`
	PageSize  int                               `json:"page_size"`
	CreatedAt time.Time                         `json:"created_at"`
	RequestID string                            `json:"request_id"`
}

func decodeContactRequestPageTokenFor(
	command types.ListContactRequestsCommand,
	direction types.ContactRequestListDirection,
	status types.ContactRequestStatus,
	pageSize int,
) (contactRequestPageCursor, bool, error) {
	value := command.PageToken
	if value == "" {
		return contactRequestPageCursor{}, false, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return contactRequestPageCursor{}, false, types.NewInvalidArgument("invalid page_token")
	}
	var cursor contactRequestPageCursor
	if err := json.Unmarshal(raw, &cursor); err != nil {
		return contactRequestPageCursor{}, false, types.NewInvalidArgument("invalid page_token")
	}
	if cursor.Version != 1 || cursor.RequestID == "" || cursor.CreatedAt.IsZero() {
		return contactRequestPageCursor{}, false, types.NewInvalidArgument("invalid page_token")
	}
	if cursor.TenantID != command.AuthContext.TenantID ||
		cursor.UserID != command.AuthContext.UserID ||
		cursor.Direction != direction ||
		cursor.Status != status ||
		cursor.PageSize != pageSize {
		return contactRequestPageCursor{}, false, types.NewInvalidArgument("invalid page_token")
	}
	return cursor, true, nil
}

func encodeContactRequestPageToken(cursor contactRequestPageCursor) string {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
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
		cursor.PageSize != pageSize ||
		cursor.Query != command.NormalizedQuery() ||
		cursor.GroupName != command.NormalizedGroupName() {
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

func likePatternForSearchQuery(query string) string {
	query = strings.ReplaceAll(query, `\`, `\\`)
	query = strings.ReplaceAll(query, `%`, `\%`)
	query = strings.ReplaceAll(query, `_`, `\_`)
	return "%" + query + "%"
}

func partitionKeyFor(tenantID types.TenantID, first types.UserID, second types.UserID) string {
	return fmt.Sprintf("%s:%s", tenantID, canonicalPair(first, second))
}

func contactEdgeID(ownerUserID types.UserID, contactUserID types.UserID) string {
	return fmt.Sprintf("%s:%s", ownerUserID, contactUserID)
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
