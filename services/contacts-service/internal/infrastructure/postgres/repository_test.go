package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

func TestRepositorySendRespondAndListIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	sendResult, err := repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-1", "hello"))
	if err != nil {
		t.Fatalf("send contact request: %v", err)
	}
	if sendResult.Status != types.ContactRequestStatusPending || sendResult.RequestID == "" {
		t.Fatalf("unexpected send result: %+v", sendResult)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestCreated, 1)

	replay, err := repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-1", "hello"))
	if err != nil {
		t.Fatalf("send contact request replay: %v", err)
	}
	if !replay.IdempotentReplay || replay.RequestID != sendResult.RequestID {
		t.Fatalf("expected replay of %s, got %+v", sendResult.RequestID, replay)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestCreated, 1)

	_, err = repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-1", "changed"))
	if !errors.Is(err, types.ErrContactRequestConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
	_, err = repository.SendContactRequest(ctx, sendCommand("bob", "alice", "send-reverse", "hi"))
	if !errors.Is(err, types.ErrContactRequestConflict) {
		t.Fatalf("expected reverse pending conflict, got %v", err)
	}

	acceptResult, err := repository.RespondContactRequest(ctx, respondCommand("bob", sendResult.RequestID, "respond-1", types.ContactDecisionAccept))
	if err != nil {
		t.Fatalf("accept contact request: %v", err)
	}
	if acceptResult.Status != types.ContactRequestStatusAccepted {
		t.Fatalf("unexpected accept result: %+v", acceptResult)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestAccepted, 1)
	assertContactEdge(t, ctx, pool, "alice", "bob", types.ContactEdgeStatusActive, 1)
	assertContactEdge(t, ctx, pool, "bob", "alice", types.ContactEdgeStatusActive, 1)

	acceptReplay, err := repository.RespondContactRequest(ctx, respondCommand("bob", sendResult.RequestID, "respond-1", types.ContactDecisionAccept))
	if err != nil {
		t.Fatalf("accept contact request replay: %v", err)
	}
	if !acceptReplay.IdempotentReplay || acceptReplay.RequestID != sendResult.RequestID {
		t.Fatalf("expected accept replay, got %+v", acceptReplay)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestAccepted, 1)

	_, err = repository.RespondContactRequest(ctx, respondCommand("bob", sendResult.RequestID, "respond-decline", types.ContactDecisionDecline))
	if !errors.Is(err, types.ErrContactRequestConflict) {
		t.Fatalf("expected terminal opposite decision conflict, got %v", err)
	}
	_, err = repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-active", "again"))
	if !errors.Is(err, types.ErrContactAlreadyExists) {
		t.Fatalf("expected active contact exists, got %v", err)
	}

	aliceContacts, err := repository.ListContacts(ctx, listCommand("alice", 10, ""))
	if err != nil {
		t.Fatalf("list alice contacts: %v", err)
	}
	assertContactIDs(t, aliceContacts, "bob")
	bobState, err := repository.GetContactState(ctx, stateCommand("bob", "alice"))
	if err != nil {
		t.Fatalf("get bob contact state: %v", err)
	}
	if bobState.Status != types.ContactEdgeStatusActive || bobState.ContactUserID != "alice" {
		t.Fatalf("unexpected bob contact state: %+v", bobState)
	}
}

func TestRepositoryDeclineDoesNotCreateEdgesIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	sendResult, err := repository.SendContactRequest(ctx, sendCommand("alice", "carol", "send-decline", "hello"))
	if err != nil {
		t.Fatalf("send contact request: %v", err)
	}
	declineResult, err := repository.RespondContactRequest(ctx, respondCommand("carol", sendResult.RequestID, "respond-decline", types.ContactDecisionDecline))
	if err != nil {
		t.Fatalf("decline contact request: %v", err)
	}
	if declineResult.Status != types.ContactRequestStatusDeclined {
		t.Fatalf("unexpected decline result: %+v", declineResult)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestDeclined, 1)
	assertNoContactEdges(t, ctx, pool)
	_, err = repository.GetContactState(ctx, stateCommand("alice", "carol"))
	if !errors.Is(err, types.ErrContactRequestNotFound) {
		t.Fatalf("expected no contact state, got %v", err)
	}
}

func TestRepositoryCancelContactRequestIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	sendResult, err := repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-cancel", "hello"))
	if err != nil {
		t.Fatalf("send contact request: %v", err)
	}
	cancelResult, err := repository.CancelContactRequest(ctx, cancelCommand("alice", sendResult.RequestID, "cancel-1"))
	if err != nil {
		t.Fatalf("cancel contact request: %v", err)
	}
	if cancelResult.Status != types.ContactRequestStatusCanceled || cancelResult.SenderUserID != "alice" || cancelResult.ReceiverUserID != "bob" {
		t.Fatalf("unexpected cancel result: %+v", cancelResult)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestCanceled, 1)
	assertNoContactEdges(t, ctx, pool)

	replay, err := repository.CancelContactRequest(ctx, cancelCommand("alice", sendResult.RequestID, "cancel-1"))
	if err != nil {
		t.Fatalf("cancel replay: %v", err)
	}
	if !replay.IdempotentReplay || replay.Status != types.ContactRequestStatusCanceled || replay.RequestID != sendResult.RequestID {
		t.Fatalf("expected cancel replay, got %+v", replay)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestCanceled, 1)

	_, err = repository.RespondContactRequest(ctx, respondCommand("bob", sendResult.RequestID, "respond-after-cancel", types.ContactDecisionAccept))
	if !errors.Is(err, types.ErrContactRequestConflict) {
		t.Fatalf("expected accept after cancel conflict, got %v", err)
	}
	_, err = repository.CancelContactRequest(ctx, cancelCommand("bob", sendResult.RequestID, "cancel-by-receiver"))
	if !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected receiver cancel permission denied, got %v", err)
	}
	pendingIncoming, err := repository.ListContactRequests(ctx, listContactRequestsCommand("bob", types.ContactRequestListDirectionIncoming, types.ContactRequestStatusPending, 10, ""))
	if err != nil {
		t.Fatalf("list pending incoming: %v", err)
	}
	assertContactRequestIDs(t, pendingIncoming)
	canceledOutgoing, err := repository.ListContactRequests(ctx, listContactRequestsCommand("alice", types.ContactRequestListDirectionOutgoing, types.ContactRequestStatusCanceled, 10, ""))
	if err != nil {
		t.Fatalf("list canceled outgoing: %v", err)
	}
	assertContactRequestIDs(t, canceledOutgoing, sendResult.RequestID)
}

func TestRepositoryListContactRequestsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	aliceToBob, err := repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-alice-bob", "alice says hi"))
	if err != nil {
		t.Fatalf("send alice -> bob: %v", err)
	}
	carolToBob, err := repository.SendContactRequest(ctx, sendCommand("carol", "bob", "send-carol-bob", "carol says hi"))
	if err != nil {
		t.Fatalf("send carol -> bob: %v", err)
	}
	bobToDave, err := repository.SendContactRequest(ctx, sendCommand("bob", "dave", "send-bob-dave", "bob says hi"))
	if err != nil {
		t.Fatalf("send bob -> dave: %v", err)
	}
	setContactRequestCreatedAt(t, ctx, pool, aliceToBob.RequestID, time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC))
	setContactRequestCreatedAt(t, ctx, pool, carolToBob.RequestID, time.Date(2026, 6, 10, 8, 1, 0, 0, time.UTC))
	setContactRequestCreatedAt(t, ctx, pool, bobToDave.RequestID, time.Date(2026, 6, 10, 8, 2, 0, 0, time.UTC))

	firstIncoming, err := repository.ListContactRequests(ctx, listContactRequestsCommand("bob", types.ContactRequestListDirectionIncoming, types.ContactRequestStatusPending, 1, ""))
	if err != nil {
		t.Fatalf("list first incoming pending: %v", err)
	}
	assertContactRequestIDs(t, firstIncoming, carolToBob.RequestID)
	if firstIncoming.Direction != types.ContactRequestListDirectionIncoming || firstIncoming.Status != types.ContactRequestStatusPending {
		t.Fatalf("unexpected first incoming metadata: %+v", firstIncoming)
	}
	if firstIncoming.NextPageToken == "" {
		t.Fatal("expected incoming pending next_page_token")
	}
	secondIncoming, err := repository.ListContactRequests(ctx, listContactRequestsCommand("bob", types.ContactRequestListDirectionIncoming, types.ContactRequestStatusPending, 1, firstIncoming.NextPageToken))
	if err != nil {
		t.Fatalf("list second incoming pending: %v", err)
	}
	assertContactRequestIDs(t, secondIncoming, aliceToBob.RequestID)
	_, err = repository.ListContactRequests(ctx, listContactRequestsCommand("bob", types.ContactRequestListDirectionOutgoing, types.ContactRequestStatusPending, 1, firstIncoming.NextPageToken))
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected direction cursor mismatch, got %v", err)
	}
	_, err = repository.ListContactRequests(ctx, listContactRequestsCommand("bob", types.ContactRequestListDirectionIncoming, types.ContactRequestStatusAccepted, 1, firstIncoming.NextPageToken))
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected status cursor mismatch, got %v", err)
	}

	outgoing, err := repository.ListContactRequests(ctx, listContactRequestsCommand("bob", types.ContactRequestListDirectionOutgoing, types.ContactRequestStatusPending, 10, ""))
	if err != nil {
		t.Fatalf("list outgoing pending: %v", err)
	}
	assertContactRequestIDs(t, outgoing, bobToDave.RequestID)

	if _, err := repository.RespondContactRequest(ctx, respondCommand("bob", aliceToBob.RequestID, "respond-alice-bob", types.ContactDecisionAccept)); err != nil {
		t.Fatalf("accept alice -> bob: %v", err)
	}
	pendingAfterAccept, err := repository.ListContactRequests(ctx, listContactRequestsCommand("bob", types.ContactRequestListDirectionIncoming, types.ContactRequestStatusPending, 10, ""))
	if err != nil {
		t.Fatalf("list pending after accept: %v", err)
	}
	assertContactRequestIDs(t, pendingAfterAccept, carolToBob.RequestID)
	accepted, err := repository.ListContactRequests(ctx, listContactRequestsCommand("bob", types.ContactRequestListDirectionIncoming, types.ContactRequestStatusAccepted, 10, ""))
	if err != nil {
		t.Fatalf("list accepted incoming: %v", err)
	}
	assertContactRequestIDs(t, accepted, aliceToBob.RequestID)
	if len(accepted.Requests) != 1 || accepted.Requests[0].DecidedAtUnixMS == 0 || accepted.Requests[0].Message != "alice says hi" {
		t.Fatalf("unexpected accepted request item: %+v", accepted.Requests)
	}
}

func TestRepositoryUpdateContactRemarkIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)
	acceptContact(t, ctx, repository, "alice", "bob")

	result, err := repository.UpdateContactRemark(ctx, remarkCommand("alice", "bob", "remark-1", "Bob from school"))
	if err != nil {
		t.Fatalf("update contact remark: %v", err)
	}
	if result.Status != types.ContactEdgeStatusActive || result.Version != 2 || result.Remark != "Bob from school" {
		t.Fatalf("unexpected remark result: %+v", result)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRemarkUpdated, 1)

	replay, err := repository.UpdateContactRemark(ctx, remarkCommand("alice", "bob", "remark-1", "Bob from school"))
	if err != nil {
		t.Fatalf("update contact remark replay: %v", err)
	}
	if !replay.IdempotentReplay || replay.Version != result.Version || replay.Remark != result.Remark {
		t.Fatalf("expected remark replay, got %+v", replay)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRemarkUpdated, 1)

	aliceContacts, err := repository.ListContacts(ctx, listCommand("alice", 10, ""))
	if err != nil {
		t.Fatalf("list alice contacts: %v", err)
	}
	if len(aliceContacts.Contacts) != 1 || aliceContacts.Contacts[0].Remark != "Bob from school" {
		t.Fatalf("expected remark in list result, got %+v", aliceContacts.Contacts)
	}
	state, err := repository.GetContactState(ctx, stateCommand("alice", "bob"))
	if err != nil {
		t.Fatalf("get contact state: %v", err)
	}
	if state.Remark != "Bob from school" {
		t.Fatalf("expected remark in state, got %+v", state)
	}
}

func TestRepositoryUpdateContactGroupIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)
	acceptContact(t, ctx, repository, "alice", "bob")
	acceptContact(t, ctx, repository, "alice", "carol")

	result, err := repository.UpdateContactGroup(ctx, groupCommand("alice", "bob", "group-1", "school"))
	if err != nil {
		t.Fatalf("update contact group: %v", err)
	}
	if result.Status != types.ContactEdgeStatusActive || result.Version != 2 || result.GroupName != "school" {
		t.Fatalf("unexpected group result: %+v", result)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactGroupUpdated, 1)

	replay, err := repository.UpdateContactGroup(ctx, groupCommand("alice", "bob", "group-1", "school"))
	if err != nil {
		t.Fatalf("update contact group replay: %v", err)
	}
	if !replay.IdempotentReplay || replay.Version != result.Version || replay.GroupName != result.GroupName {
		t.Fatalf("expected group replay, got %+v", replay)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactGroupUpdated, 1)

	aliceContacts, err := repository.ListContacts(ctx, listGroupCommand("alice", 10, "", "school"))
	if err != nil {
		t.Fatalf("list alice grouped contacts: %v", err)
	}
	assertContactIDs(t, aliceContacts, "bob")
	if aliceContacts.Contacts[0].GroupName != "school" {
		t.Fatalf("expected group in list result, got %+v", aliceContacts.Contacts)
	}
	state, err := repository.GetContactState(ctx, stateCommand("alice", "bob"))
	if err != nil {
		t.Fatalf("get contact state: %v", err)
	}
	if state.GroupName != "school" {
		t.Fatalf("expected group in state, got %+v", state)
	}

	if _, err := repository.UpdateContactGroup(ctx, groupCommand("alice", "carol", "group-2", "school")); err != nil {
		t.Fatalf("update second contact group: %v", err)
	}
	paged, err := repository.ListContacts(ctx, listGroupCommand("alice", 1, "", "school"))
	if err != nil {
		t.Fatalf("list grouped first page: %v", err)
	}
	if paged.NextPageToken == "" {
		t.Fatal("expected grouped page token")
	}
	_, err = repository.ListContacts(ctx, listGroupCommand("alice", 1, paged.NextPageToken, "family"))
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected group-bound page token rejection, got %v", err)
	}
}

func TestRepositoryUpdateContactRemarkReplayUsesOriginalSnapshotIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)
	acceptContact(t, ctx, repository, "alice", "bob")

	first, err := repository.UpdateContactRemark(ctx, remarkCommand("alice", "bob", "remark-1", "Bob from school"))
	if err != nil {
		t.Fatalf("update first remark: %v", err)
	}
	second, err := repository.UpdateContactRemark(ctx, remarkCommand("alice", "bob", "remark-2", "Robert from work"))
	if err != nil {
		t.Fatalf("update second remark: %v", err)
	}
	if second.Remark != "Robert from work" || second.Version <= first.Version {
		t.Fatalf("unexpected second remark result: %+v", second)
	}

	replay, err := repository.UpdateContactRemark(ctx, remarkCommand("alice", "bob", "remark-1", "Bob from school"))
	if err != nil {
		t.Fatalf("replay first remark: %v", err)
	}
	if !replay.IdempotentReplay || replay.Remark != first.Remark || replay.Version != first.Version {
		t.Fatalf("expected original remark replay %+v, got %+v", first, replay)
	}
}

func TestRepositoryDeleteContactIsOwnerScopedIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)
	acceptContact(t, ctx, repository, "alice", "bob")

	result, err := repository.DeleteContact(ctx, deleteCommand("alice", "bob", "delete-1"))
	if err != nil {
		t.Fatalf("delete contact: %v", err)
	}
	if result.Status != types.ContactEdgeStatusDeleted || result.Version != 2 {
		t.Fatalf("unexpected delete result: %+v", result)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactEdgeDeleted, 1)
	assertContactEdge(t, ctx, pool, "alice", "bob", types.ContactEdgeStatusDeleted, 2)
	assertContactEdge(t, ctx, pool, "bob", "alice", types.ContactEdgeStatusActive, 1)

	aliceContacts, err := repository.ListContacts(ctx, listCommand("alice", 10, ""))
	if err != nil {
		t.Fatalf("list alice contacts: %v", err)
	}
	assertContactIDs(t, aliceContacts)
	bobContacts, err := repository.ListContacts(ctx, listCommand("bob", 10, ""))
	if err != nil {
		t.Fatalf("list bob contacts: %v", err)
	}
	assertContactIDs(t, bobContacts, "alice")

	replay, err := repository.DeleteContact(ctx, deleteCommand("alice", "bob", "delete-1"))
	if err != nil {
		t.Fatalf("delete replay: %v", err)
	}
	if !replay.IdempotentReplay || replay.Status != types.ContactEdgeStatusDeleted {
		t.Fatalf("expected delete replay, got %+v", replay)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactEdgeDeleted, 1)
}

func TestRepositoryBlockContactIsOwnerScopedIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)
	acceptContact(t, ctx, repository, "alice", "bob")

	result, err := repository.BlockContact(ctx, blockCommand("alice", "bob", "block-1", "spam"))
	if err != nil {
		t.Fatalf("block contact: %v", err)
	}
	if result.Status != types.ContactEdgeStatusBlocked || result.Version != 2 {
		t.Fatalf("unexpected block result: %+v", result)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactEdgeBlocked, 1)
	assertContactEdge(t, ctx, pool, "alice", "bob", types.ContactEdgeStatusBlocked, 2)
	assertContactEdge(t, ctx, pool, "bob", "alice", types.ContactEdgeStatusActive, 1)

	aliceContacts, err := repository.ListContacts(ctx, listCommand("alice", 10, ""))
	if err != nil {
		t.Fatalf("list alice contacts: %v", err)
	}
	assertContactIDs(t, aliceContacts)
	bobContacts, err := repository.ListContacts(ctx, listCommand("bob", 10, ""))
	if err != nil {
		t.Fatalf("list bob contacts: %v", err)
	}
	assertContactIDs(t, bobContacts, "alice")

	replay, err := repository.BlockContact(ctx, blockCommand("alice", "bob", "block-1", "spam"))
	if err != nil {
		t.Fatalf("block replay: %v", err)
	}
	if !replay.IdempotentReplay || replay.Status != types.ContactEdgeStatusBlocked {
		t.Fatalf("expected block replay, got %+v", replay)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactEdgeBlocked, 1)
}

func TestRepositorySendContactRequestBlockedEdgeDeniedIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)
	acceptContact(t, ctx, repository, "alice", "bob")
	if _, err := repository.BlockContact(ctx, blockCommand("alice", "bob", "block-1", "spam")); err != nil {
		t.Fatalf("block contact: %v", err)
	}
	if _, err := repository.DeleteContact(ctx, deleteCommand("alice", "bob", "delete-after-block")); !errors.Is(err, types.ErrContactNotFound) {
		t.Fatalf("expected blocked edge not deletable as active contact, got %v", err)
	}

	_, err := repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-blocked", "let me add you again"))
	if !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected permission denied for blocked contact request, got %v", err)
	}
	assertContactRequestCount(t, ctx, pool, 1)
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestCreated, 1)
}

func TestRepositoryRespondContactRequestBlockedEdgeDeniedIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	acceptContact(t, ctx, repository, "alice", "bob")
	if _, err := repository.DeleteContact(ctx, deleteCommand("alice", "bob", "delete-before-readd")); err != nil {
		t.Fatalf("delete alice contact edge: %v", err)
	}
	sendResult, err := repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-before-block", "hello again"))
	if err != nil {
		t.Fatalf("send contact request: %v", err)
	}
	if _, err := repository.BlockContact(ctx, blockCommand("bob", "alice", "block-before-accept", "spam")); err != nil {
		t.Fatalf("block contact: %v", err)
	}

	_, err = repository.RespondContactRequest(ctx, respondCommand("bob", sendResult.RequestID, "accept-after-block", types.ContactDecisionAccept))
	if !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected permission denied for blocked accept, got %v", err)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestCreated, 2)
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestAccepted, 1)
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactEdgeDeleted, 1)
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactEdgeBlocked, 1)
	assertContactEdge(t, ctx, pool, "alice", "bob", types.ContactEdgeStatusDeleted, 2)
	assertContactEdge(t, ctx, pool, "bob", "alice", types.ContactEdgeStatusBlocked, 2)
}

func TestRepositoryUnblockContactRestoresOwnerScopedEdgeIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)
	acceptContact(t, ctx, repository, "alice", "bob")
	if _, err := repository.BlockContact(ctx, blockCommand("alice", "bob", "block-1", "spam")); err != nil {
		t.Fatalf("block contact: %v", err)
	}

	result, err := repository.UnblockContact(ctx, unblockCommand("alice", "bob", "unblock-1"))
	if err != nil {
		t.Fatalf("unblock contact: %v", err)
	}
	if result.Status != types.ContactEdgeStatusActive || result.Version != 3 {
		t.Fatalf("unexpected unblock result: %+v", result)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactEdgeUnblocked, 1)
	assertContactEdge(t, ctx, pool, "alice", "bob", types.ContactEdgeStatusActive, 3)
	assertContactEdge(t, ctx, pool, "bob", "alice", types.ContactEdgeStatusActive, 1)

	aliceContacts, err := repository.ListContacts(ctx, listCommand("alice", 10, ""))
	if err != nil {
		t.Fatalf("list alice contacts: %v", err)
	}
	assertContactIDs(t, aliceContacts, "bob")

	replay, err := repository.UnblockContact(ctx, unblockCommand("alice", "bob", "unblock-1"))
	if err != nil {
		t.Fatalf("unblock replay: %v", err)
	}
	if !replay.IdempotentReplay || replay.Status != types.ContactEdgeStatusActive || replay.Version != result.Version {
		t.Fatalf("expected unblock replay, got %+v", replay)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactEdgeUnblocked, 1)
}

func TestRepositoryContactOutboxVersionsStayMonotonicAfterReAddIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	firstSend, err := repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-1", "hello"))
	if err != nil {
		t.Fatalf("first send: %v", err)
	}
	if _, err := repository.RespondContactRequest(ctx, respondCommand("bob", firstSend.RequestID, "respond-1", types.ContactDecisionAccept)); err != nil {
		t.Fatalf("first accept: %v", err)
	}
	if _, err := repository.DeleteContact(ctx, deleteCommand("alice", "bob", "delete-1")); err != nil {
		t.Fatalf("delete contact: %v", err)
	}
	secondSend, err := repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-2", "hello again"))
	if err != nil {
		t.Fatalf("second send: %v", err)
	}
	if _, err := repository.RespondContactRequest(ctx, respondCommand("bob", secondSend.RequestID, "respond-2", types.ContactDecisionAccept)); err != nil {
		t.Fatalf("second accept: %v", err)
	}

	versions := contactOutboxVersionsForPair(t, ctx, pool, "alice", "bob")
	want := []int64{1, 2, 3, 4, 5}
	if len(versions) != len(want) {
		t.Fatalf("expected versions %v, got %v", want, versions)
	}
	for index := range want {
		if versions[index] != want[index] {
			t.Fatalf("expected versions %v, got %v", want, versions)
		}
	}
}

func TestRepositoryListContactsPaginationIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	insertContactEdge(t, ctx, pool, "alice", "bob")
	insertContactEdge(t, ctx, pool, "alice", "carol")
	insertContactEdge(t, ctx, pool, "alice", "dave")

	first, err := repository.ListContacts(ctx, listCommand("alice", 1, ""))
	if err != nil {
		t.Fatalf("list first page: %v", err)
	}
	assertContactIDs(t, first, "bob")
	if first.NextPageToken == "" {
		t.Fatal("expected next page token")
	}
	second, err := repository.ListContacts(ctx, listCommand("alice", 1, first.NextPageToken))
	if err != nil {
		t.Fatalf("list second page: %v", err)
	}
	assertContactIDs(t, second, "carol")
	_, err = repository.ListContacts(ctx, listCommand("bob", 1, first.NextPageToken))
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected page token owner mismatch, got %v", err)
	}
	_, err = repository.ListContacts(ctx, listCommand("alice", 2, first.NextPageToken))
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected page token size mismatch, got %v", err)
	}
	_, err = repository.ListContacts(ctx, listCommand("alice", 10, "not-base64"))
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid page token, got %v", err)
	}
}

func TestRepositoryListContactsSearchIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	insertContactEdgeWithRemark(t, ctx, pool, "alice", "bob", "work friend")
	insertContactEdgeWithRemark(t, ctx, pool, "alice", "carol", "School Friend")
	insertContactEdgeWithRemark(t, ctx, pool, "alice", "dave", "gym")
	insertContactEdgeWithRemark(t, ctx, pool, "bob", "alice", "not visible to alice search")
	insertContactEdgeWithRemark(t, ctx, pool, "alice", "erin", "school alumni")
	insertContactEdgeWithGroup(t, ctx, pool, "alice", "frank", "project phoenix")
	setContactEdgeStatus(t, ctx, pool, "alice", "erin", types.ContactEdgeStatusBlocked)

	byID, err := repository.ListContacts(ctx, listSearchCommand("alice", 10, "", "BO"))
	if err != nil {
		t.Fatalf("search by id: %v", err)
	}
	assertContactIDs(t, byID, "bob")

	byRemark, err := repository.ListContacts(ctx, listSearchCommand("alice", 10, "", "school"))
	if err != nil {
		t.Fatalf("search by remark: %v", err)
	}
	assertContactIDs(t, byRemark, "carol")

	byGroup, err := repository.ListContacts(ctx, listSearchCommand("alice", 10, "", "phoenix"))
	if err != nil {
		t.Fatalf("search by group_name: %v", err)
	}
	assertContactIDs(t, byGroup, "frank")

	paged, err := repository.ListContacts(ctx, listSearchCommand("alice", 1, "", "friend"))
	if err != nil {
		t.Fatalf("search first page: %v", err)
	}
	assertContactIDs(t, paged, "bob")
	if paged.NextPageToken == "" {
		t.Fatal("expected search next page token")
	}
	next, err := repository.ListContacts(ctx, listSearchCommand("alice", 1, paged.NextPageToken, "friend"))
	if err != nil {
		t.Fatalf("search second page: %v", err)
	}
	assertContactIDs(t, next, "carol")
	_, err = repository.ListContacts(ctx, listSearchCommand("alice", 1, paged.NextPageToken, "school"))
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected query-bound page token, got %v", err)
	}
}

func TestRepositoryConcurrentSendIdempotencyIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	results := make(chan types.SendContactRequestResult, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-concurrent", "hello"))
			errs <- err
			results <- result
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	close(results)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent send failed: %v", err)
		}
	}
	requestIDs := map[string]bool{}
	replayCount := 0
	for result := range results {
		requestIDs[result.RequestID] = true
		if result.IdempotentReplay {
			replayCount++
		}
	}
	if len(requestIDs) != 1 {
		t.Fatalf("expected one request id, got %v", requestIDs)
	}
	if replayCount == 0 {
		t.Fatal("expected at least one idempotent replay")
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestCreated, 1)
}

func sendCommand(sender string, target string, key string, message string) types.SendContactRequestCommand {
	return types.SendContactRequestCommand{
		AuthContext:    authContext(sender, key),
		TargetUserID:   types.UserID(target),
		IdempotencyKey: key,
		Message:        message,
	}
}

func authContext(user string, key string) types.AuthContext {
	return types.AuthContext{
		TenantID:  "tenant-contacts",
		UserID:    types.UserID(user),
		DeviceID:  "device-1",
		RequestID: "request-" + key,
		TraceID:   "trace-" + key,
	}
}

func getPrivacyCommand(user string) types.GetContactPrivacyCommand {
	return types.GetContactPrivacyCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-contacts",
			UserID:    types.UserID(user),
			DeviceID:  "device-1",
			RequestID: "request-get-privacy-" + user,
			TraceID:   "trace-get-privacy-" + user,
		},
	}
}

func setPrivacyCommand(user string, allow bool, key string) types.SetContactPrivacyCommand {
	return types.SetContactPrivacyCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-contacts",
			UserID:    types.UserID(user),
			DeviceID:  "device-1",
			RequestID: "request-" + key,
			TraceID:   "trace-" + key,
		},
		AllowContactRequests: allow,
		IdempotencyKey:       key,
	}
}

func respondCommand(receiver string, requestID string, key string, decision types.ContactDecision) types.RespondContactRequestCommand {
	return types.RespondContactRequestCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-contacts",
			UserID:    types.UserID(receiver),
			DeviceID:  "device-1",
			RequestID: "request-" + key,
			TraceID:   "trace-" + key,
		},
		RequestID:      requestID,
		Decision:       decision,
		IdempotencyKey: key,
	}
}

func cancelCommand(sender string, requestID string, key string) types.CancelContactRequestCommand {
	return types.CancelContactRequestCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-contacts",
			UserID:    types.UserID(sender),
			DeviceID:  "device-1",
			RequestID: "request-" + key,
			TraceID:   "trace-" + key,
		},
		RequestID:      requestID,
		IdempotencyKey: key,
	}
}

func deleteCommand(owner string, contact string, key string) types.DeleteContactCommand {
	return types.DeleteContactCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-contacts",
			UserID:    types.UserID(owner),
			DeviceID:  "device-1",
			RequestID: "request-" + key,
			TraceID:   "trace-" + key,
		},
		ContactUserID:  types.UserID(contact),
		IdempotencyKey: key,
	}
}

func blockCommand(owner string, contact string, key string, reason string) types.BlockContactCommand {
	return types.BlockContactCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-contacts",
			UserID:    types.UserID(owner),
			DeviceID:  "device-1",
			RequestID: "request-" + key,
			TraceID:   "trace-" + key,
		},
		ContactUserID:  types.UserID(contact),
		IdempotencyKey: key,
		Reason:         reason,
	}
}

func unblockCommand(owner string, contact string, key string) types.UnblockContactCommand {
	return types.UnblockContactCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-contacts",
			UserID:    types.UserID(owner),
			DeviceID:  "device-1",
			RequestID: "request-" + key,
			TraceID:   "trace-" + key,
		},
		ContactUserID:  types.UserID(contact),
		IdempotencyKey: key,
	}
}

func remarkCommand(owner string, contact string, key string, remark string) types.UpdateContactRemarkCommand {
	return types.UpdateContactRemarkCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-contacts",
			UserID:    types.UserID(owner),
			DeviceID:  "device-1",
			RequestID: "request-" + key,
			TraceID:   "trace-" + key,
		},
		ContactUserID:  types.UserID(contact),
		IdempotencyKey: key,
		Remark:         remark,
	}
}

func groupCommand(owner string, contact string, key string, groupName string) types.UpdateContactGroupCommand {
	return types.UpdateContactGroupCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-contacts",
			UserID:    types.UserID(owner),
			DeviceID:  "device-1",
			RequestID: "request-" + key,
			TraceID:   "trace-" + key,
		},
		ContactUserID:  types.UserID(contact),
		IdempotencyKey: key,
		GroupName:      groupName,
	}
}

func acceptContact(t *testing.T, ctx context.Context, repository *Repository, sender string, receiver string) string {
	t.Helper()
	keySuffix := sender + "-" + receiver
	sendResult, err := repository.SendContactRequest(ctx, sendCommand(sender, receiver, "send-"+keySuffix, "hello"))
	if err != nil {
		t.Fatalf("send contact request: %v", err)
	}
	if _, err := repository.RespondContactRequest(ctx, respondCommand(receiver, sendResult.RequestID, "respond-"+keySuffix, types.ContactDecisionAccept)); err != nil {
		t.Fatalf("accept contact request: %v", err)
	}
	return sendResult.RequestID
}

func listCommand(owner string, pageSize int, pageToken string) types.ListContactsCommand {
	return listSearchCommand(owner, pageSize, pageToken, "")
}

func listSearchCommand(owner string, pageSize int, pageToken string, query string) types.ListContactsCommand {
	return types.ListContactsCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-contacts",
			UserID:   types.UserID(owner),
		},
		PageSize:  pageSize,
		PageToken: pageToken,
		Query:     query,
	}
}

func listGroupCommand(owner string, pageSize int, pageToken string, groupName string) types.ListContactsCommand {
	command := listSearchCommand(owner, pageSize, pageToken, "")
	command.GroupName = groupName
	return command
}

func listContactRequestsCommand(
	user string,
	direction types.ContactRequestListDirection,
	status types.ContactRequestStatus,
	pageSize int,
	pageToken string,
) types.ListContactRequestsCommand {
	return types.ListContactRequestsCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-contacts",
			UserID:   types.UserID(user),
		},
		Direction: direction,
		Status:    status,
		PageSize:  pageSize,
		PageToken: pageToken,
	}
}

func stateCommand(owner string, other string) types.GetContactStateCommand {
	return types.GetContactStateCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-contacts",
			UserID:   types.UserID(owner),
		},
		OtherUserID: types.UserID(other),
	}
}

func newTestRepository(pool *pgxpool.Pool) *Repository {
	var requestCounter int
	var eventCounter int
	return NewRepository(
		pool,
		WithClock(func() time.Time {
			return time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
		}),
		WithIDGenerators(
			func() (string, error) {
				requestCounter++
				return "contact_req_test_" + string(rune('a'+requestCounter-1)), nil
			},
			func() (string, error) {
				eventCounter++
				return "evt_contact_test_" + string(rune('a'+eventCounter-1)), nil
			},
		),
	)
}

func assertContactIDs(t *testing.T, result types.ListContactsResult, want ...types.UserID) {
	t.Helper()
	if len(result.Contacts) != len(want) {
		t.Fatalf("expected %d contacts, got %d: %+v", len(want), len(result.Contacts), result.Contacts)
	}
	for index, userID := range want {
		if result.Contacts[index].ContactUserID != userID {
			t.Fatalf("expected contact %d = %s, got %+v", index, userID, result.Contacts[index])
		}
	}
}

func assertContactRequestIDs(t *testing.T, result types.ListContactRequestsResult, want ...string) {
	t.Helper()
	if len(result.Requests) != len(want) {
		t.Fatalf("expected %d contact requests, got %d: %+v", len(want), len(result.Requests), result.Requests)
	}
	for index, requestID := range want {
		if result.Requests[index].RequestID != requestID {
			t.Fatalf("expected request %d = %s, got %+v", index, requestID, result.Requests[index])
		}
	}
}

func assertContactsOutboxCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventType string, want int) {
	t.Helper()
	var got int
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM contacts_outbox
WHERE tenant_id = 'tenant-contacts'
  AND event_type = $1
`, eventType).Scan(&got)
	if err != nil {
		t.Fatalf("count contacts outbox: %v", err)
	}
	if got != want {
		t.Fatalf("expected %d contacts outbox rows for %s, got %d", want, eventType, got)
	}
}

func assertContactRequestCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int) {
	t.Helper()
	var got int
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM contact_requests
WHERE tenant_id = 'tenant-contacts'
`).Scan(&got)
	if err != nil {
		t.Fatalf("count contact requests: %v", err)
	}
	if got != want {
		t.Fatalf("expected %d contact requests, got %d", want, got)
	}
}

func assertContactEdge(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner string, contact string, status types.ContactEdgeStatus, version int64) {
	t.Helper()
	var gotStatus types.ContactEdgeStatus
	var gotVersion int64
	err := pool.QueryRow(ctx, `
SELECT status, version
FROM contact_edges
WHERE tenant_id = 'tenant-contacts'
  AND owner_user_id = $1
  AND contact_user_id = $2
`, owner, contact).Scan(&gotStatus, &gotVersion)
	if err != nil {
		t.Fatalf("query contact edge %s -> %s: %v", owner, contact, err)
	}
	if gotStatus != status || gotVersion != version {
		t.Fatalf("unexpected contact edge %s -> %s: status=%s version=%d", owner, contact, gotStatus, gotVersion)
	}
}

func assertNoContactEdges(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var got int
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM contact_edges
WHERE tenant_id = 'tenant-contacts'
`).Scan(&got)
	if err != nil {
		t.Fatalf("count contact edges: %v", err)
	}
	if got != 0 {
		t.Fatalf("expected no contact edges, got %d", got)
	}
}

func contactOutboxVersionsForPair(t *testing.T, ctx context.Context, pool *pgxpool.Pool, first string, second string) []int64 {
	t.Helper()
	rows, err := pool.Query(ctx, `
SELECT aggregate_version
FROM contacts_outbox
WHERE tenant_id = 'tenant-contacts'
  AND partition_key = $1
ORDER BY aggregate_version ASC
`, partitionKeyFor("tenant-contacts", types.UserID(first), types.UserID(second)))
	if err != nil {
		t.Fatalf("query contact outbox versions: %v", err)
	}
	defer rows.Close()
	var versions []int64
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			t.Fatalf("scan contact outbox version: %v", err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate contact outbox versions: %v", err)
	}
	return versions
}

func insertContactEdge(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner string, contact string) {
	insertContactEdgeWithRemark(t, ctx, pool, owner, contact, "")
}

func insertContactEdgeWithRemark(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner string, contact string, remark string) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO contact_edges (
    tenant_id,
    owner_user_id,
    contact_user_id,
    status,
    remark,
    source_request_id,
    version,
    created_at,
    updated_at
) VALUES ('tenant-contacts', $1, $2, 'ACTIVE', $3, 'seed-request', 1, now(), now())
`, owner, contact, remark)
	if err != nil {
		t.Fatalf("insert contact edge: %v", err)
	}
}

func insertContactEdgeWithGroup(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner string, contact string, groupName string) {
	t.Helper()
	insertContactEdgeWithRemark(t, ctx, pool, owner, contact, "")
	_, err := pool.Exec(ctx, `
UPDATE contact_edges
SET group_name = $3,
    updated_at = now()
WHERE tenant_id = 'tenant-contacts'
  AND owner_user_id = $1
  AND contact_user_id = $2
`, owner, contact, groupName)
	if err != nil {
		t.Fatalf("set contact edge group_name: %v", err)
	}
}

func setContactEdgeStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner string, contact string, status types.ContactEdgeStatus) {
	t.Helper()
	_, err := pool.Exec(ctx, `
UPDATE contact_edges
SET status = $3,
    updated_at = now()
WHERE tenant_id = 'tenant-contacts'
  AND owner_user_id = $1
  AND contact_user_id = $2
`, owner, contact, status)
	if err != nil {
		t.Fatalf("set contact edge status: %v", err)
	}
}

func setContactRequestCreatedAt(t *testing.T, ctx context.Context, pool *pgxpool.Pool, requestID string, createdAt time.Time) {
	t.Helper()
	_, err := pool.Exec(ctx, `
UPDATE contact_requests
SET created_at = $2,
    updated_at = $2
WHERE tenant_id = 'tenant-contacts'
  AND request_id = $1
`, requestID, createdAt)
	if err != nil {
		t.Fatalf("set contact request created_at: %v", err)
	}
}

func insertTenantPrivacyDefault(t *testing.T, ctx context.Context, pool *pgxpool.Pool, allowContactRequests bool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO contact_tenant_privacy_defaults (
    tenant_id,
    allow_contact_requests,
    version,
    created_at,
    updated_at
) VALUES ('tenant-contacts', $1, 1, now(), now())
`, allowContactRequests)
	if err != nil {
		t.Fatalf("insert tenant privacy default: %v", err)
	}
}

func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	t.Cleanup(pool.Close)
	applyContactsMigration(t, ctx, pool)
	return pool
}

func applyContactsMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	root := findRepoRoot(t)
	migrationDir := filepath.Join(root, "migrations", "postgres", "contacts")
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		t.Fatalf("read contacts migrations: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".sql" {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		sqlBytes, err := os.ReadFile(filepath.Join(migrationDir, name))
		if err != nil {
			t.Fatalf("read contacts migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("apply contacts migration %s: %v", name, err)
		}
	}
}

func resetContactsTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
TRUNCATE
    contacts_outbox_repair_audit,
    contacts_outbox,
    contact_command_idempotency,
    contact_privacy_settings,
    contact_tenant_privacy_defaults,
    contact_tenant_request_source_policies,
    contact_edges,
    contact_requests
RESTART IDENTITY
`)
	if err != nil {
		t.Fatalf("reset contacts tables: %v", err)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("repo root not found")
		}
		wd = parent
	}
}

func TestDecodePageTokenRejectsMalformedPayload(t *testing.T) {
	raw, err := json.Marshal(contactPageCursor{
		Version:       1,
		TenantID:      "tenant-other",
		OwnerUserID:   "alice",
		PageSize:      1,
		ContactUserID: "bob",
	})
	if err != nil {
		t.Fatalf("marshal cursor: %v", err)
	}
	_, _, err = decodePageTokenFor(listCommand("alice", 1, base64.RawURLEncoding.EncodeToString(raw)), 1)
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid cursor tenant, got %v", err)
	}
}
