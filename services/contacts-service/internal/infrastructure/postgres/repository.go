package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
