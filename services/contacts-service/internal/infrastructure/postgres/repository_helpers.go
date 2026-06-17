package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

type commandHashPayload struct {
	Kind                          string   `json:"kind"`
	TenantID                      string   `json:"tenant_id"`
	UserID                        string   `json:"user_id"`
	TargetUserID                  string   `json:"target_user_id,omitempty"`
	OtherUserID                   string   `json:"other_user_id,omitempty"`
	ContactUserID                 string   `json:"contact_user_id,omitempty"`
	RequestID                     string   `json:"request_id,omitempty"`
	Decision                      string   `json:"decision,omitempty"`
	Message                       string   `json:"message,omitempty"`
	SourceType                    string   `json:"source_type,omitempty"`
	SourceRef                     string   `json:"source_ref,omitempty"`
	Reason                        string   `json:"reason,omitempty"`
	Remark                        string   `json:"remark,omitempty"`
	GroupName                     string   `json:"group_name,omitempty"`
	AllowContactRequests          *bool    `json:"allow_contact_requests,omitempty"`
	AllowSearchContactRequests    *bool    `json:"allow_search_contact_requests,omitempty"`
	AllowProfileVisibility        *bool    `json:"allow_profile_visibility,omitempty"`
	UpdateProfileVisibilityFields bool     `json:"update_profile_visibility_fields,omitempty"`
	ProfileVisibilityFields       []string `json:"profile_visibility_fields,omitempty"`
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
	RiskLevel      types.ContactRequestRiskLevel
	ReviewRequired bool
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
		RiskLevel:        row.RiskLevel,
		ReviewRequired:   row.ReviewRequired,
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
SELECT request_id, tenant_id, sender_user_id, receiver_user_id, status, source_type, source_ref, risk_level, review_required
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
		&row.RiskLevel,
		&row.ReviewRequired,
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
SELECT request_id, tenant_id, sender_user_id, receiver_user_id, status, command_hash, source_type, source_ref, risk_level, review_required
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
		&row.RiskLevel,
		&row.ReviewRequired,
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
SELECT request_id, tenant_id, sender_user_id, receiver_user_id, status, source_type, source_ref, risk_level, review_required
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
		&row.RiskLevel,
		&row.ReviewRequired,
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
	sourcePolicy contactRequestSourcePolicyRow,
	requestID string,
	commandHash string,
	status types.ContactRequestStatus,
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
    risk_level,
    review_required,
    created_at,
    updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $13)
`, requestID, command.AuthContext.TenantID, command.AuthContext.UserID, command.TargetUserID, status, command.IdempotencyKey, commandHash, command.Message, command.NormalizedSourceType(), command.NormalizedSourceRef(), sourcePolicy.RiskLevel, sourcePolicy.ReviewRequired, now)
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

func pendingOrReviewContactRequestExists(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, first types.UserID, second types.UserID) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM contact_requests
    WHERE tenant_id = $1
      AND status IN ('PENDING', 'REVIEW_REQUIRED')
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
    decided_at = CASE WHEN $3 = 'PENDING' THEN NULL ELSE now() END,
    updated_at = now()
WHERE tenant_id = $1
  AND request_id = $2
`, request.TenantID, request.RequestID, status)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertContactRequestReviewAudit(
	ctx context.Context,
	tx pgx.Tx,
	request contactRequestRow,
	nextStatus types.ContactRequestStatus,
	command types.ReviewContactRequestCommand,
) error {
	_, err := tx.Exec(ctx, `
INSERT INTO contact_request_review_audit (
    tenant_id,
    request_id,
    previous_status,
    next_status,
    decision,
    operator,
	reason,
	source_type,
	risk_level,
	review_required,
	reviewed_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
`, request.TenantID, request.RequestID, request.Status, nextStatus, command.Decision, command.Operator, command.Reason, request.SourceType, request.RiskLevel, request.ReviewRequired)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
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

func commitReviewContactRequestResult(ctx context.Context, tx pgx.Tx, result types.ReviewContactRequestResult) (types.ReviewContactRequestResult, error) {
	if err := tx.Commit(ctx); err != nil {
		return types.ReviewContactRequestResult{}, types.NewDBWriteFailed(err.Error())
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

func reviewResultFromRequest(request contactRequestRow, nextStatus types.ContactRequestStatus, decision types.ContactRequestReviewDecision) types.ReviewContactRequestResult {
	return types.ReviewContactRequestResult{
		RequestID:      request.RequestID,
		TenantID:       request.TenantID,
		SenderUserID:   request.SenderUserID,
		ReceiverUserID: request.ReceiverUserID,
		PreviousStatus: request.Status,
		Status:         nextStatus,
		Decision:       decision,
		RiskLevel:      request.RiskLevel,
		ReviewRequired: request.ReviewRequired,
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
