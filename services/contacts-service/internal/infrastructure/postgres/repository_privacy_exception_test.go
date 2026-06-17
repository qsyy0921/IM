package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

func TestRepositoryContactPrivacyExceptionAllowDenyIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	deny, err := repository.SetContactPrivacyException(ctx, setPrivacyExceptionCommand("bob", "alice", types.ContactPrivacyExceptionDecisionDeny, "privacy-exception-deny"))
	if err != nil {
		t.Fatalf("set deny privacy exception: %v", err)
	}
	if deny.Decision != types.ContactPrivacyExceptionDecisionDeny ||
		deny.Version != 1 ||
		deny.OwnerUserID != "bob" ||
		deny.OtherUserID != "alice" {
		t.Fatalf("unexpected deny privacy exception: %+v", deny)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactPrivacyExceptionUpdated, 1)

	replay, err := repository.SetContactPrivacyException(ctx, setPrivacyExceptionCommand("bob", "alice", types.ContactPrivacyExceptionDecisionDeny, "privacy-exception-deny"))
	if err != nil {
		t.Fatalf("replay deny privacy exception: %v", err)
	}
	if !replay.IdempotentReplay || replay.Version != deny.Version {
		t.Fatalf("unexpected privacy exception replay: %+v", replay)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactPrivacyExceptionUpdated, 1)

	_, err = repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-denied-by-exception", "hello"))
	if !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected privacy exception deny to block request, got %v", err)
	}
	assertContactRequestCount(t, ctx, pool, 0)

	closedPrivacy, err := repository.SetContactPrivacy(ctx, setPrivacyCommand("bob", false, "privacy-close-for-exception"))
	if err != nil {
		t.Fatalf("close bob privacy: %v", err)
	}
	if closedPrivacy.Settings.AllowContactRequests {
		t.Fatalf("expected bob privacy closed: %+v", closedPrivacy)
	}
	allow, err := repository.SetContactPrivacyException(ctx, setPrivacyExceptionCommand("bob", "carol", types.ContactPrivacyExceptionDecisionAllow, "privacy-exception-allow"))
	if err != nil {
		t.Fatalf("set allow privacy exception: %v", err)
	}
	if allow.Decision != types.ContactPrivacyExceptionDecisionAllow {
		t.Fatalf("unexpected allow privacy exception: %+v", allow)
	}
	allowedSend, err := repository.SendContactRequest(ctx, sendCommand("carol", "bob", "send-allowed-by-exception", "hello"))
	if err != nil {
		t.Fatalf("expected allow exception to bypass closed privacy: %v", err)
	}
	if allowedSend.Status != types.ContactRequestStatusPending {
		t.Fatalf("unexpected allowed send result: %+v", allowedSend)
	}

	sourceBlocked, err := repository.SetTenantContactRequestSourcePolicy(ctx, types.SetTenantContactRequestSourcePolicyCommand{
		TenantID:             "tenant-contacts",
		SourceType:           types.ContactRequestSourceTypeSearch,
		AllowContactRequests: false,
	})
	if err != nil {
		t.Fatalf("set search source policy blocked: %v", err)
	}
	if sourceBlocked.Policy.AllowContactRequests {
		t.Fatalf("expected search source policy blocked: %+v", sourceBlocked)
	}
	_, err = repository.SetContactPrivacyException(ctx, setPrivacyExceptionCommand("bob", "dave", types.ContactPrivacyExceptionDecisionAllow, "privacy-exception-allow-dave"))
	if err != nil {
		t.Fatalf("set allow privacy exception for dave: %v", err)
	}
	search := sendCommand("dave", "bob", "send-search-source-still-blocked", "hello from search")
	search.SourceType = types.ContactRequestSourceTypeSearch
	_, err = repository.SendContactRequest(ctx, search)
	if !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected source policy to still block allow exception, got %v", err)
	}

	var payloadDecision string
	err = pool.QueryRow(ctx, `
SELECT payload_json->>'decision'
FROM contacts_outbox
WHERE tenant_id = 'tenant-contacts'
  AND event_type = $1
  AND payload_json->>'owner_user_id' = 'bob'
  AND payload_json->>'other_user_id' = 'carol'
`, eventTypeContactPrivacyExceptionUpdated).Scan(&payloadDecision)
	if err != nil {
		t.Fatalf("query privacy exception outbox payload: %v", err)
	}
	if payloadDecision != "ALLOW" {
		t.Fatalf("unexpected privacy exception outbox decision: %q", payloadDecision)
	}
}

func TestRepositoryContactPrivacyExceptionManagementIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	for _, input := range []struct {
		other    string
		decision types.ContactPrivacyExceptionDecision
		key      string
	}{
		{other: "alice", decision: types.ContactPrivacyExceptionDecisionDeny, key: "privacy-exception-deny-alice"},
		{other: "carol", decision: types.ContactPrivacyExceptionDecisionAllow, key: "privacy-exception-allow-carol"},
		{other: "dave", decision: types.ContactPrivacyExceptionDecisionAllow, key: "privacy-exception-allow-dave"},
	} {
		if _, err := repository.SetContactPrivacyException(ctx, setPrivacyExceptionCommand("bob", input.other, input.decision, input.key)); err != nil {
			t.Fatalf("set privacy exception %s: %v", input.other, err)
		}
	}

	first, err := repository.ListContactPrivacyExceptions(ctx, listPrivacyExceptionsCommand("bob", 1, ""))
	if err != nil {
		t.Fatalf("list privacy exceptions first page: %v", err)
	}
	if len(first.Exceptions) != 1 || first.Exceptions[0].OtherUserID != "alice" || first.NextPageToken == "" {
		t.Fatalf("unexpected first privacy exception page: %+v", first)
	}
	second, err := repository.ListContactPrivacyExceptions(ctx, listPrivacyExceptionsCommand("bob", 1, first.NextPageToken))
	if err != nil {
		t.Fatalf("list privacy exceptions second page: %v", err)
	}
	if len(second.Exceptions) != 1 || second.Exceptions[0].OtherUserID != "carol" || second.Exceptions[0].Decision != types.ContactPrivacyExceptionDecisionAllow {
		t.Fatalf("unexpected second privacy exception page: %+v", second)
	}
	_, err = repository.ListContactPrivacyExceptions(ctx, listPrivacyExceptionsCommand("alice", 1, first.NextPageToken))
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected privacy exception page token to be owner-scoped, got %v", err)
	}

	deleted, err := repository.DeleteContactPrivacyException(ctx, deletePrivacyExceptionCommand("bob", "carol", "privacy-exception-delete-carol"))
	if err != nil {
		t.Fatalf("delete privacy exception: %v", err)
	}
	if !deleted.Deleted || deleted.OwnerUserID != "bob" || deleted.OtherUserID != "carol" || deleted.IdempotentReplay {
		t.Fatalf("unexpected privacy exception delete result: %+v", deleted)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactPrivacyExceptionDeleted, 1)

	replay, err := repository.DeleteContactPrivacyException(ctx, deletePrivacyExceptionCommand("bob", "carol", "privacy-exception-delete-carol"))
	if err != nil {
		t.Fatalf("replay privacy exception delete: %v", err)
	}
	if !replay.IdempotentReplay || !replay.Deleted {
		t.Fatalf("unexpected privacy exception delete replay: %+v", replay)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactPrivacyExceptionDeleted, 1)

	remaining, err := repository.ListContactPrivacyExceptions(ctx, listPrivacyExceptionsCommand("bob", 10, ""))
	if err != nil {
		t.Fatalf("list privacy exceptions after delete: %v", err)
	}
	if len(remaining.Exceptions) != 2 {
		t.Fatalf("expected two remaining privacy exceptions, got %+v", remaining)
	}
	for _, item := range remaining.Exceptions {
		if item.OtherUserID == "carol" {
			t.Fatalf("deleted privacy exception still listed: %+v", remaining)
		}
	}

	_, err = repository.DeleteContactPrivacyException(ctx, deletePrivacyExceptionCommand("bob", "carol", "privacy-exception-delete-missing"))
	if !errors.Is(err, types.ErrContactNotFound) {
		t.Fatalf("expected missing privacy exception delete to be not found, got %v", err)
	}

	closedPrivacy, err := repository.SetContactPrivacy(ctx, setPrivacyCommand("bob", false, "privacy-close-after-delete"))
	if err != nil {
		t.Fatalf("close bob privacy after delete: %v", err)
	}
	if closedPrivacy.Settings.AllowContactRequests {
		t.Fatalf("expected bob privacy closed: %+v", closedPrivacy)
	}
	_, err = repository.SendContactRequest(ctx, sendCommand("carol", "bob", "send-after-privacy-exception-delete", "hello"))
	if !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected deleted allow exception to inherit closed privacy, got %v", err)
	}

	var previousVersion int64
	err = pool.QueryRow(ctx, `
SELECT (payload_json->>'previous_exception_version')::bigint
FROM contacts_outbox
WHERE tenant_id = 'tenant-contacts'
  AND event_type = $1
  AND payload_json->>'owner_user_id' = 'bob'
  AND payload_json->>'other_user_id' = 'carol'
`, eventTypeContactPrivacyExceptionDeleted).Scan(&previousVersion)
	if err != nil {
		t.Fatalf("query privacy exception deleted outbox payload: %v", err)
	}
	if previousVersion != 1 {
		t.Fatalf("unexpected deleted privacy exception previous version: %d", previousVersion)
	}
}
