package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/contacts-service/internal/domain"
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

const (
	commandTypeSendContactRequest     = "SEND_CONTACT_REQUEST"
	commandTypeRespondContactRequest  = "RESPOND_CONTACT_REQUEST"
	commandTypeCancelContactRequest   = "CANCEL_CONTACT_REQUEST"
	commandTypeDeleteContact          = "DELETE_CONTACT"
	commandTypeBlockContact           = "BLOCK_CONTACT"
	commandTypeUnblockContact         = "UNBLOCK_CONTACT"
	commandTypeUpdateContactRemark    = "UPDATE_CONTACT_REMARK"
	commandTypeUpdateContactGroup     = "UPDATE_CONTACT_GROUP"
	commandTypeSetContactPrivacy      = "SET_CONTACT_PRIVACY"
	commandTypeSetPrivacyException    = "SET_CONTACT_PRIVACY_EXCEPTION"
	commandTypeDeletePrivacyException = "DELETE_CONTACT_PRIVACY_EXCEPTION"

	eventTypeContactRequestCreated          = "contact.request.created.v1"
	eventTypeContactRequestAccepted         = "contact.request.accepted.v1"
	eventTypeContactRequestDeclined         = "contact.request.declined.v1"
	eventTypeContactRequestCanceled         = "contact.request.canceled.v1"
	eventTypeContactEdgeDeleted             = "contact.edge.deleted.v1"
	eventTypeContactEdgeBlocked             = "contact.edge.blocked.v1"
	eventTypeContactEdgeUnblocked           = "contact.edge.unblocked.v1"
	eventTypeContactRemarkUpdated           = "contact.edge.remark_updated.v1"
	eventTypeContactGroupUpdated            = "contact.edge.group_updated.v1"
	eventTypeContactPrivacyUpdated          = "contact.privacy.updated.v1"
	eventTypeContactPrivacyExceptionUpdated = "contact.privacy_exception.updated.v1"
	eventTypeContactPrivacyExceptionDeleted = "contact.privacy_exception.deleted.v1"

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
	if ok, err := pendingOrReviewContactRequestExists(ctx, tx, command.AuthContext.TenantID, command.AuthContext.UserID, command.TargetUserID); err != nil {
		return types.SendContactRequestResult{}, err
	} else if ok {
		return types.SendContactRequestResult{}, types.NewContactRequestConflict("pending contact request already exists")
	}
	exceptionDecision, hasException, err := getContactPrivacyExceptionDecision(
		ctx,
		tx,
		command.AuthContext.TenantID,
		command.TargetUserID,
		command.AuthContext.UserID,
	)
	if err != nil {
		return types.SendContactRequestResult{}, err
	}
	if hasException && exceptionDecision == types.ContactPrivacyExceptionDecisionDeny {
		return types.SendContactRequestResult{}, types.NewPermissionDenied("target user denied this contact request")
	}
	if !hasException || exceptionDecision != types.ContactPrivacyExceptionDecisionAllow {
		allowed, err := contactRequestsAllowed(ctx, tx, command.AuthContext.TenantID, command.TargetUserID, command.NormalizedSourceType())
		if err != nil {
			return types.SendContactRequestResult{}, err
		}
		if !allowed {
			return types.SendContactRequestResult{}, types.NewPermissionDenied("target user does not accept contact requests")
		}
	}
	sourcePolicy, err := getTenantContactRequestSourcePolicy(ctx, tx, command.AuthContext.TenantID, command.NormalizedSourceType())
	if err != nil {
		return types.SendContactRequestResult{}, err
	}
	if !sourcePolicy.AllowContactRequests {
		return types.SendContactRequestResult{}, types.NewPermissionDenied("contact request source is not allowed")
	}

	requestID, err := r.requestID()
	if err != nil {
		return types.SendContactRequestResult{}, types.NewDBWriteFailed(err.Error())
	}
	occurredAt := r.now()
	initialStatus := types.ContactRequestStatusPending
	if sourcePolicy.ReviewRequired {
		initialStatus = types.ContactRequestStatusReviewRequired
	}
	if err := insertContactRequest(ctx, tx, command, sourcePolicy, requestID, commandHash, initialStatus, occurredAt); err != nil {
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
			"status":           initialStatus,
			"message":          command.Message,
			"source_type":      command.NormalizedSourceType(),
			"source_ref":       command.NormalizedSourceRef(),
			"risk_level":       sourcePolicy.RiskLevel,
			"review_required":  sourcePolicy.ReviewRequired,
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
		Status:         initialStatus,
		SourceType:     command.NormalizedSourceType(),
		SourceRef:      command.NormalizedSourceRef(),
		RiskLevel:      sourcePolicy.RiskLevel,
		ReviewRequired: sourcePolicy.ReviewRequired,
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
		if request.Status == types.ContactRequestStatusReviewRequired {
			return types.RespondContactRequestResult{}, types.NewContactRequestConflict("contact request requires operator review")
		}
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
	if request.Status != types.ContactRequestStatusPending && request.Status != types.ContactRequestStatusReviewRequired {
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

func (r *Repository) ReviewContactRequest(
	ctx context.Context,
	command types.ReviewContactRequestCommand,
) (types.ReviewContactRequestResult, error) {
	if r.pool == nil {
		return types.ReviewContactRequestResult{}, types.NewDBWriteFailed("contacts repository is not configured")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.ReviewContactRequestResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	request, err := lockContactRequest(ctx, tx, command.TenantID, command.RequestID)
	if err != nil {
		return types.ReviewContactRequestResult{}, err
	}
	if request.Status != types.ContactRequestStatusReviewRequired {
		if command.Decision == types.ContactRequestReviewDecisionApprove &&
			request.Status == types.ContactRequestStatusPending &&
			request.ReviewRequired {
			return commitReviewContactRequestResult(ctx, tx, reviewResultFromRequest(request, request.Status, command.Decision))
		}
		if command.Decision == types.ContactRequestReviewDecisionDecline &&
			request.Status == types.ContactRequestStatusDeclined &&
			request.ReviewRequired {
			return commitReviewContactRequestResult(ctx, tx, reviewResultFromRequest(request, request.Status, command.Decision))
		}
		return types.ReviewContactRequestResult{}, types.NewContactRequestConflict("contact request is not awaiting review")
	}
	nextStatus := types.ContactRequestStatusPending
	if command.Decision == types.ContactRequestReviewDecisionDecline {
		nextStatus = types.ContactRequestStatusDeclined
	}
	if err := updateContactRequestStatus(ctx, tx, request, nextStatus); err != nil {
		return types.ReviewContactRequestResult{}, err
	}
	if err := insertContactRequestReviewAudit(ctx, tx, request, nextStatus, command); err != nil {
		return types.ReviewContactRequestResult{}, err
	}
	if command.Decision == types.ContactRequestReviewDecisionDecline {
		eventID, err := r.eventID()
		if err != nil {
			return types.ReviewContactRequestResult{}, types.NewOutboxWriteFailed(err.Error())
		}
		partitionKey := partitionKeyFor(request.TenantID, request.SenderUserID, request.ReceiverUserID)
		aggregateVersion, err := nextContactOutboxAggregateVersion(ctx, tx, request.TenantID, partitionKey)
		if err != nil {
			return types.ReviewContactRequestResult{}, err
		}
		if err := insertContactOutbox(ctx, tx, contactOutboxInput{
			EventID:          eventID,
			TenantID:         request.TenantID,
			AggregateType:    "CONTACT_REQUEST",
			AggregateID:      request.RequestID,
			AggregateVersion: aggregateVersion,
			EventType:        eventTypeContactRequestDeclined,
			PartitionKey:     partitionKey,
			CorrelationID:    command.Operator,
			CausationID:      request.RequestID,
			Payload:          responsePayload(request, types.ContactRequestStatusDeclined, 0, r.now()),
		}); err != nil {
			return types.ReviewContactRequestResult{}, err
		}
	}
	return commitReviewContactRequestResult(ctx, tx, reviewResultFromRequest(request, nextStatus, command.Decision))
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
