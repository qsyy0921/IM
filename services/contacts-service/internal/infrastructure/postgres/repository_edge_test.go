package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

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
