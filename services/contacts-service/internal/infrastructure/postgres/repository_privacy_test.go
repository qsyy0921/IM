package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

func TestRepositoryContactPrivacyDefaultsAllowRequestsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	privacy, err := repository.GetContactPrivacy(ctx, getPrivacyCommand("bob"))
	if err != nil {
		t.Fatalf("get default contact privacy: %v", err)
	}
	if privacy.TenantID != "tenant-contacts" ||
		privacy.UserID != "bob" ||
		!privacy.Settings.AllowContactRequests ||
		len(privacy.Settings.ProfileVisibilityFields) != 4 ||
		privacy.Settings.Version != 0 ||
		privacy.Settings.UpdatedAtUnixMS != 0 ||
		privacy.Settings.PolicySource != types.ContactPrivacyPolicySourceSystemDefault {
		t.Fatalf("unexpected default privacy: %+v", privacy)
	}

	sendResult, err := repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-privacy-default", "hello"))
	if err != nil {
		t.Fatalf("send contact request with default privacy: %v", err)
	}
	if sendResult.Status != types.ContactRequestStatusPending {
		t.Fatalf("unexpected send result: %+v", sendResult)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestCreated, 1)
}

func TestRepositoryTenantContactPrivacyDefaultBlocksRequestsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)
	insertTenantPrivacyDefault(t, ctx, pool, false)

	privacy, err := repository.GetContactPrivacy(ctx, getPrivacyCommand("bob"))
	if err != nil {
		t.Fatalf("get tenant default contact privacy: %v", err)
	}
	if privacy.Settings.AllowContactRequests ||
		privacy.Settings.Version != 1 ||
		privacy.Settings.UpdatedAtUnixMS == 0 ||
		privacy.Settings.PolicySource != types.ContactPrivacyPolicySourceTenantDefault {
		t.Fatalf("unexpected tenant default privacy: %+v", privacy)
	}

	_, err = repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-tenant-default-denied", "hello"))
	if !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected tenant default permission denied, got %v", err)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestCreated, 0)
	assertContactRequestCount(t, ctx, pool, 0)

	open, err := repository.SetContactPrivacy(ctx, setPrivacyCommand("bob", true, "privacy-user-open"))
	if err != nil {
		t.Fatalf("set user privacy open: %v", err)
	}
	if !open.Settings.AllowContactRequests ||
		open.Settings.Version != 1 ||
		open.Settings.PolicySource != types.ContactPrivacyPolicySourceUser {
		t.Fatalf("unexpected user privacy override: %+v", open)
	}

	sendResult, err := repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-user-override-open", "hello again"))
	if err != nil {
		t.Fatalf("send after user privacy override: %v", err)
	}
	if sendResult.Status != types.ContactRequestStatusPending {
		t.Fatalf("unexpected send after user privacy override: %+v", sendResult)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestCreated, 1)
}

func TestRepositorySetTenantContactPrivacyDefaultIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	defaultResult, err := repository.GetTenantContactPrivacyDefault(ctx, types.GetTenantContactPrivacyDefaultCommand{
		TenantID: "tenant-contacts",
	})
	if err != nil {
		t.Fatalf("get system tenant privacy default: %v", err)
	}
	if !defaultResult.Settings.AllowContactRequests ||
		defaultResult.Settings.Version != 0 ||
		defaultResult.Settings.PolicySource != types.ContactPrivacyPolicySourceSystemDefault {
		t.Fatalf("unexpected system tenant default: %+v", defaultResult)
	}

	blocked, err := repository.SetTenantContactPrivacyDefault(ctx, types.SetTenantContactPrivacyDefaultCommand{
		TenantID:             "tenant-contacts",
		AllowContactRequests: false,
	})
	if err != nil {
		t.Fatalf("set tenant default blocked: %v", err)
	}
	if !blocked.Changed ||
		blocked.Settings.AllowContactRequests ||
		blocked.Settings.Version != 1 ||
		blocked.Settings.PolicySource != types.ContactPrivacyPolicySourceTenantDefault ||
		blocked.Settings.UpdatedAtUnixMS == 0 {
		t.Fatalf("unexpected blocked tenant default: %+v", blocked)
	}

	replay, err := repository.SetTenantContactPrivacyDefault(ctx, types.SetTenantContactPrivacyDefaultCommand{
		TenantID:             "tenant-contacts",
		AllowContactRequests: false,
	})
	if err != nil {
		t.Fatalf("set same tenant default: %v", err)
	}
	if replay.Changed || replay.Settings.Version != 1 {
		t.Fatalf("expected unchanged tenant default version, got %+v", replay)
	}

	open, err := repository.SetTenantContactPrivacyDefault(ctx, types.SetTenantContactPrivacyDefaultCommand{
		TenantID:             "tenant-contacts",
		AllowContactRequests: true,
	})
	if err != nil {
		t.Fatalf("set tenant default open: %v", err)
	}
	if !open.Changed ||
		!open.Settings.AllowContactRequests ||
		open.Settings.Version != 2 ||
		open.Settings.PolicySource != types.ContactPrivacyPolicySourceTenantDefault {
		t.Fatalf("unexpected open tenant default: %+v", open)
	}

	userPrivacy, err := repository.GetContactPrivacy(ctx, getPrivacyCommand("bob"))
	if err != nil {
		t.Fatalf("get user privacy from tenant default: %v", err)
	}
	if !userPrivacy.Settings.AllowContactRequests ||
		userPrivacy.Settings.Version != 2 ||
		userPrivacy.Settings.PolicySource != types.ContactPrivacyPolicySourceTenantDefault {
		t.Fatalf("unexpected user privacy from tenant default: %+v", userPrivacy)
	}
}

func TestRepositoryContactRequestSourceMetadataIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	command := sendCommand("alice", "bob", "send-source", "hello")
	command.SourceType = types.ContactRequestSourceTypeGroup
	command.SourceRef = " conversation:conv-1 "
	sendResult, err := repository.SendContactRequest(ctx, command)
	if err != nil {
		t.Fatalf("send contact request with source: %v", err)
	}
	if sendResult.SourceType != types.ContactRequestSourceTypeGroup || sendResult.SourceRef != "conversation:conv-1" {
		t.Fatalf("unexpected send source metadata: %+v", sendResult)
	}

	replay, err := repository.SendContactRequest(ctx, command)
	if err != nil {
		t.Fatalf("replay contact request with source: %v", err)
	}
	if !replay.IdempotentReplay ||
		replay.SourceType != types.ContactRequestSourceTypeGroup ||
		replay.SourceRef != "conversation:conv-1" {
		t.Fatalf("unexpected source replay: %+v", replay)
	}

	conflicting := sendCommand("alice", "bob", "send-source", "hello")
	conflicting.SourceType = types.ContactRequestSourceTypeSearch
	conflicting.SourceRef = "search:alice"
	if _, err := repository.SendContactRequest(ctx, conflicting); !errors.Is(err, types.ErrContactRequestConflict) {
		t.Fatalf("expected source metadata idempotency conflict, got %v", err)
	}

	incoming, err := repository.ListContactRequests(ctx, listContactRequestsCommand("bob", types.ContactRequestListDirectionIncoming, types.ContactRequestStatusPending, 10, ""))
	if err != nil {
		t.Fatalf("list incoming contact requests: %v", err)
	}
	if len(incoming.Requests) != 1 ||
		incoming.Requests[0].SourceType != types.ContactRequestSourceTypeGroup ||
		incoming.Requests[0].SourceRef != "conversation:conv-1" {
		t.Fatalf("unexpected listed source metadata: %+v", incoming)
	}

	var sourceType string
	var sourceRef string
	err = pool.QueryRow(ctx, `
SELECT source_type, source_ref
FROM contact_requests
WHERE tenant_id = 'tenant-contacts'
  AND request_id = $1
`, sendResult.RequestID).Scan(&sourceType, &sourceRef)
	if err != nil {
		t.Fatalf("query contact request source metadata: %v", err)
	}
	if sourceType != string(types.ContactRequestSourceTypeGroup) || sourceRef != "conversation:conv-1" {
		t.Fatalf("unexpected persisted source metadata: %s %s", sourceType, sourceRef)
	}

	var payloadSourceType string
	var payloadSourceRef string
	err = pool.QueryRow(ctx, `
SELECT payload_json->>'source_type', payload_json->>'source_ref'
FROM contacts_outbox
WHERE tenant_id = 'tenant-contacts'
  AND event_type = $1
`, eventTypeContactRequestCreated).Scan(&payloadSourceType, &payloadSourceRef)
	if err != nil {
		t.Fatalf("query contact request outbox source metadata: %v", err)
	}
	if payloadSourceType != string(types.ContactRequestSourceTypeGroup) || payloadSourceRef != "conversation:conv-1" {
		t.Fatalf("unexpected outbox source metadata: %s %s", payloadSourceType, payloadSourceRef)
	}
}

func TestRepositoryContactRequestDefaultSourceReplaysLegacyHashIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	command := sendCommand("alice", "bob", "send-legacy-source", "hello")
	legacyHash, err := commandHash(commandHashPayload{
		Kind:         commandTypeSendContactRequest,
		TenantID:     string(command.AuthContext.TenantID),
		UserID:       string(command.AuthContext.UserID),
		TargetUserID: string(command.TargetUserID),
		Message:      command.Message,
	})
	if err != nil {
		t.Fatalf("legacy command hash: %v", err)
	}
	_, err = pool.Exec(ctx, `
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
) VALUES ('legacy-request-1', 'tenant-contacts', 'alice', 'bob', 'PENDING', $1, $2, $3, now(), now())
`, command.IdempotencyKey, legacyHash, command.Message)
	if err != nil {
		t.Fatalf("insert legacy contact request: %v", err)
	}

	replay, err := repository.SendContactRequest(ctx, command)
	if err != nil {
		t.Fatalf("replay legacy contact request: %v", err)
	}
	if !replay.IdempotentReplay ||
		replay.RequestID != "legacy-request-1" ||
		replay.SourceType != types.ContactRequestSourceTypeDirect ||
		replay.SourceRef != "" {
		t.Fatalf("unexpected legacy replay: %+v", replay)
	}
	assertContactRequestCount(t, ctx, pool, 1)
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestCreated, 0)
}

func TestRepositoryTenantContactRequestSourcePolicyBlocksRequestsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	defaultPolicy, err := repository.GetTenantContactRequestSourcePolicy(ctx, types.GetTenantContactRequestSourcePolicyCommand{
		TenantID:   "tenant-contacts",
		SourceType: types.ContactRequestSourceTypeSearch,
	})
	if err != nil {
		t.Fatalf("get default source policy: %v", err)
	}
	if !defaultPolicy.Policy.AllowContactRequests ||
		defaultPolicy.Policy.Version != 0 ||
		defaultPolicy.Policy.SourceType != types.ContactRequestSourceTypeSearch ||
		defaultPolicy.Policy.RiskLevel != types.ContactRequestRiskLevelLow ||
		defaultPolicy.Policy.ReviewRequired {
		t.Fatalf("unexpected default source policy: %+v", defaultPolicy)
	}

	blocked, err := repository.SetTenantContactRequestSourcePolicy(ctx, types.SetTenantContactRequestSourcePolicyCommand{
		TenantID:             "tenant-contacts",
		SourceType:           types.ContactRequestSourceTypeSearch,
		AllowContactRequests: false,
	})
	if err != nil {
		t.Fatalf("set source policy blocked: %v", err)
	}
	if !blocked.Changed ||
		blocked.Policy.AllowContactRequests ||
		blocked.Policy.Version != 1 ||
		blocked.Policy.UpdatedAtUnixMS == 0 {
		t.Fatalf("unexpected blocked source policy: %+v", blocked)
	}

	searchCommand := sendCommand("alice", "bob", "send-search-blocked", "hello")
	searchCommand.SourceType = types.ContactRequestSourceTypeSearch
	_, err = repository.SendContactRequest(ctx, searchCommand)
	if !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected source policy permission denied, got %v", err)
	}
	assertContactRequestCount(t, ctx, pool, 0)
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestCreated, 0)

	direct, err := repository.SendContactRequest(ctx, sendCommand("carol", "bob", "send-direct-allowed", "hello"))
	if err != nil {
		t.Fatalf("send direct while search blocked: %v", err)
	}
	if direct.Status != types.ContactRequestStatusPending ||
		direct.SourceType != types.ContactRequestSourceTypeDirect {
		t.Fatalf("unexpected direct send result: %+v", direct)
	}

	unchanged, err := repository.SetTenantContactRequestSourcePolicy(ctx, types.SetTenantContactRequestSourcePolicyCommand{
		TenantID:             "tenant-contacts",
		SourceType:           types.ContactRequestSourceTypeSearch,
		AllowContactRequests: false,
	})
	if err != nil {
		t.Fatalf("set same source policy: %v", err)
	}
	if unchanged.Changed || unchanged.Policy.Version != 1 {
		t.Fatalf("expected unchanged source policy version, got %+v", unchanged)
	}

	opened, err := repository.SetTenantContactRequestSourcePolicy(ctx, types.SetTenantContactRequestSourcePolicyCommand{
		TenantID:             "tenant-contacts",
		SourceType:           types.ContactRequestSourceTypeSearch,
		AllowContactRequests: true,
		RiskLevel:            types.ContactRequestRiskLevelHigh,
		ReviewRequired:       true,
	})
	if err != nil {
		t.Fatalf("set source policy open: %v", err)
	}
	if !opened.Changed ||
		!opened.Policy.AllowContactRequests ||
		opened.Policy.RiskLevel != types.ContactRequestRiskLevelHigh ||
		!opened.Policy.ReviewRequired ||
		opened.Policy.Version != 2 {
		t.Fatalf("unexpected open source policy: %+v", opened)
	}

	allowedSearch := sendCommand("dave", "bob", "send-search-allowed", "hello")
	allowedSearch.SourceType = types.ContactRequestSourceTypeSearch
	searchResult, err := repository.SendContactRequest(ctx, allowedSearch)
	if err != nil {
		t.Fatalf("send search after source policy opened: %v", err)
	}
	if searchResult.Status != types.ContactRequestStatusReviewRequired ||
		searchResult.SourceType != types.ContactRequestSourceTypeSearch ||
		searchResult.RiskLevel != types.ContactRequestRiskLevelHigh ||
		!searchResult.ReviewRequired {
		t.Fatalf("unexpected search send result: %+v", searchResult)
	}
	if _, err := repository.RespondContactRequest(ctx, respondCommand("bob", searchResult.RequestID, "respond-before-review", types.ContactDecisionAccept)); !errors.Is(err, types.ErrContactRequestConflict) {
		t.Fatalf("expected review required request to reject receiver response, got %v", err)
	}
	pendingBeforeReview, err := repository.ListContactRequests(ctx, listContactRequestsCommand("bob", types.ContactRequestListDirectionIncoming, types.ContactRequestStatusPending, 10, ""))
	if err != nil {
		t.Fatalf("list pending incoming before review: %v", err)
	}
	assertContactRequestIDs(t, pendingBeforeReview, direct.RequestID)
	reviewIncoming, err := repository.ListContactRequests(ctx, listContactRequestsCommand("bob", types.ContactRequestListDirectionIncoming, types.ContactRequestStatusReviewRequired, 10, ""))
	if err != nil {
		t.Fatalf("list review required incoming requests: %v", err)
	}
	var foundRiskRequest bool
	for _, item := range reviewIncoming.Requests {
		if item.RequestID == searchResult.RequestID {
			foundRiskRequest = true
			if item.RiskLevel != types.ContactRequestRiskLevelHigh || !item.ReviewRequired {
				t.Fatalf("unexpected listed risk metadata: %+v", item)
			}
		}
	}
	if !foundRiskRequest {
		t.Fatalf("expected listed risk request %s in %+v", searchResult.RequestID, reviewIncoming.Requests)
	}
	reviewResult, err := repository.ReviewContactRequest(ctx, types.ReviewContactRequestCommand{
		TenantID:  "tenant-contacts",
		RequestID: searchResult.RequestID,
		Decision:  types.ContactRequestReviewDecisionApprove,
		Operator:  "operator-1",
		Reason:    "risk reviewed",
	})
	if err != nil {
		t.Fatalf("approve contact request review: %v", err)
	}
	if reviewResult.PreviousStatus != types.ContactRequestStatusReviewRequired ||
		reviewResult.Status != types.ContactRequestStatusPending ||
		reviewResult.Decision != types.ContactRequestReviewDecisionApprove {
		t.Fatalf("unexpected review result: %+v", reviewResult)
	}
	assertContactRequestReviewAuditCount(t, ctx, pool, searchResult.RequestID, 1)
	_, err = repository.ReviewContactRequest(ctx, types.ReviewContactRequestCommand{
		TenantID:  "tenant-contacts",
		RequestID: searchResult.RequestID,
		Decision:  types.ContactRequestReviewDecisionDecline,
		Operator:  "operator-2",
		Reason:    "attempt reverse decision after approve",
	})
	if !errors.Is(err, types.ErrContactRequestConflict) {
		t.Fatalf("expected reverse review decision after approve conflict, got %v", err)
	}
	assertContactRequestReviewAuditCount(t, ctx, pool, searchResult.RequestID, 1)
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestDeclined, 0)
	accepted, err := repository.RespondContactRequest(ctx, respondCommand("bob", searchResult.RequestID, "respond-after-review", types.ContactDecisionAccept))
	if err != nil {
		t.Fatalf("accept approved contact request: %v", err)
	}
	if accepted.Status != types.ContactRequestStatusAccepted {
		t.Fatalf("unexpected accepted reviewed request: %+v", accepted)
	}
	assertContactRequestRiskMetadata(t, ctx, pool, searchResult.RequestID, types.ContactRequestRiskLevelHigh, true)
	assertContactRequestCreatedPayloadRiskMetadata(t, ctx, pool, searchResult.RequestID, types.ContactRequestRiskLevelHigh, true)
	assertContactRequestCount(t, ctx, pool, 2)
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestCreated, 2)
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestAccepted, 1)
}

func TestRepositoryReviewContactRequestDeclineIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	if _, err := repository.SetTenantContactRequestSourcePolicy(ctx, types.SetTenantContactRequestSourcePolicyCommand{
		TenantID:             "tenant-contacts",
		SourceType:           types.ContactRequestSourceTypeInviteLink,
		AllowContactRequests: true,
		RiskLevel:            types.ContactRequestRiskLevelHigh,
		ReviewRequired:       true,
	}); err != nil {
		t.Fatalf("set invite source policy: %v", err)
	}
	command := sendCommand("alice", "bob", "send-review-decline", "hello")
	command.SourceType = types.ContactRequestSourceTypeInviteLink
	command.SourceRef = "invite:batch-1"
	sendResult, err := repository.SendContactRequest(ctx, command)
	if err != nil {
		t.Fatalf("send review required request: %v", err)
	}
	if sendResult.Status != types.ContactRequestStatusReviewRequired {
		t.Fatalf("expected review required request, got %+v", sendResult)
	}
	reviewResult, err := repository.ReviewContactRequest(ctx, types.ReviewContactRequestCommand{
		TenantID:  "tenant-contacts",
		RequestID: sendResult.RequestID,
		Decision:  types.ContactRequestReviewDecisionDecline,
		Operator:  "operator-1",
		Reason:    "source risk rejected",
	})
	if err != nil {
		t.Fatalf("decline contact request review: %v", err)
	}
	if reviewResult.PreviousStatus != types.ContactRequestStatusReviewRequired ||
		reviewResult.Status != types.ContactRequestStatusDeclined ||
		reviewResult.Decision != types.ContactRequestReviewDecisionDecline {
		t.Fatalf("unexpected declined review result: %+v", reviewResult)
	}
	assertContactRequestReviewAuditCount(t, ctx, pool, sendResult.RequestID, 1)
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestDeclined, 1)
	assertNoContactEdges(t, ctx, pool)
	_, err = repository.ReviewContactRequest(ctx, types.ReviewContactRequestCommand{
		TenantID:  "tenant-contacts",
		RequestID: sendResult.RequestID,
		Decision:  types.ContactRequestReviewDecisionApprove,
		Operator:  "operator-2",
		Reason:    "attempt reverse decision after decline",
	})
	if !errors.Is(err, types.ErrContactRequestConflict) {
		t.Fatalf("expected reverse review decision after decline conflict, got %v", err)
	}
	assertContactRequestReviewAuditCount(t, ctx, pool, sendResult.RequestID, 1)
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestDeclined, 1)

	_, err = repository.RespondContactRequest(ctx, respondCommand("bob", sendResult.RequestID, "respond-after-review-decline", types.ContactDecisionAccept))
	if !errors.Is(err, types.ErrContactRequestConflict) {
		t.Fatalf("expected response after review decline conflict, got %v", err)
	}
}

func TestRepositoryAuditContactRequestReviewsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	if _, err := repository.SetTenantContactRequestSourcePolicy(ctx, types.SetTenantContactRequestSourcePolicyCommand{
		TenantID:             "tenant-contacts",
		SourceType:           types.ContactRequestSourceTypeQRCode,
		AllowContactRequests: true,
		RiskLevel:            types.ContactRequestRiskLevelHigh,
		ReviewRequired:       true,
	}); err != nil {
		t.Fatalf("set qr code source policy: %v", err)
	}
	command := sendCommand("alice", "bob", "send-review-audit", "hello")
	command.SourceType = types.ContactRequestSourceTypeQRCode
	command.SourceRef = "qr:campaign-1"
	sendResult, err := repository.SendContactRequest(ctx, command)
	if err != nil {
		t.Fatalf("send review required request: %v", err)
	}
	if _, err := repository.ReviewContactRequest(ctx, types.ReviewContactRequestCommand{
		TenantID:  "tenant-contacts",
		RequestID: sendResult.RequestID,
		Decision:  types.ContactRequestReviewDecisionApprove,
		Operator:  "operator-audit",
		Reason:    "internal source risk reviewed",
	}); err != nil {
		t.Fatalf("review contact request: %v", err)
	}

	reviewRequired := true
	rows, err := repository.AuditContactRequestReviews(ctx, ContactRequestReviewAuditOptions{
		TenantID:       "tenant-contacts",
		RequestID:      sendResult.RequestID,
		Operator:       "operator-audit",
		Decision:       "approve",
		NextStatus:     "pending",
		SourceType:     "qr_code",
		RiskLevel:      "high",
		ReviewRequired: &reviewRequired,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("audit contact request reviews: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one review audit row, got %+v", rows)
	}
	row := rows[0]
	if row.TenantID != "tenant-contacts" ||
		row.RequestID != sendResult.RequestID ||
		row.PreviousStatus != string(types.ContactRequestStatusReviewRequired) ||
		row.NextStatus != string(types.ContactRequestStatusPending) ||
		row.Decision != string(types.ContactRequestReviewDecisionApprove) ||
		row.Operator != "operator-audit" ||
		row.SourceType != string(types.ContactRequestSourceTypeQRCode) ||
		row.RiskLevel != string(types.ContactRequestRiskLevelHigh) ||
		!row.ReasonPresent ||
		!row.ReviewRequired {
		t.Fatalf("unexpected review audit row: %+v", row)
	}

	reviewedAfter := row.ReviewedAt.Add(-time.Second)
	reviewedBefore := row.ReviewedAt.Add(time.Second)
	windowRows, err := repository.AuditContactRequestReviews(ctx, ContactRequestReviewAuditOptions{
		TenantID:       "tenant-contacts",
		ReviewedAfter:  &reviewedAfter,
		ReviewedBefore: &reviewedBefore,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("audit contact request reviews by reviewed_at window: %v", err)
	}
	if len(windowRows) != 1 || windowRows[0].AuditID != row.AuditID {
		t.Fatalf("expected review audit row inside reviewed_at window, got %+v", windowRows)
	}

	_, err = repository.AuditContactRequestReviews(ctx, ContactRequestReviewAuditOptions{
		TenantID:       "tenant-contacts",
		ReviewedAfter:  &reviewedBefore,
		ReviewedBefore: &reviewedBefore,
		Limit:          10,
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid reviewed_at window error, got %v", err)
	}

	declinedRows, err := repository.AuditContactRequestReviews(ctx, ContactRequestReviewAuditOptions{
		TenantID: "tenant-contacts",
		Decision: "DECLINE",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("audit declined contact request reviews: %v", err)
	}
	if len(declinedRows) != 0 {
		t.Fatalf("expected no declined review audit rows, got %+v", declinedRows)
	}
}
