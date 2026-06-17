package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

func TestRepositorySetContactPrivacyBlocksIncomingRequestsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	closed, err := repository.SetContactPrivacy(ctx, setPrivacyCommand("bob", false, "privacy-close"))
	if err != nil {
		t.Fatalf("set privacy closed: %v", err)
	}
	if closed.IdempotentReplay ||
		closed.UserID != "bob" ||
		closed.Settings.AllowContactRequests ||
		closed.Settings.Version != 1 ||
		closed.Settings.UpdatedAtUnixMS == 0 ||
		closed.Settings.PolicySource != types.ContactPrivacyPolicySourceUser {
		t.Fatalf("unexpected closed privacy result: %+v", closed)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactPrivacyUpdated, 1)

	replay, err := repository.SetContactPrivacy(ctx, setPrivacyCommand("bob", false, "privacy-close"))
	if err != nil {
		t.Fatalf("set privacy replay: %v", err)
	}
	if !replay.IdempotentReplay ||
		replay.Settings.AllowContactRequests ||
		replay.Settings.Version != closed.Settings.Version ||
		replay.Settings.UpdatedAtUnixMS != closed.Settings.UpdatedAtUnixMS ||
		replay.Settings.PolicySource != types.ContactPrivacyPolicySourceUser {
		t.Fatalf("unexpected privacy replay: %+v", replay)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactPrivacyUpdated, 1)

	_, err = repository.SetContactPrivacy(ctx, setPrivacyCommand("bob", true, "privacy-close"))
	if !errors.Is(err, types.ErrContactRequestConflict) {
		t.Fatalf("expected privacy idempotency conflict, got %v", err)
	}

	_, err = repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-privacy-denied", "hello"))
	if !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected permission denied from closed privacy, got %v", err)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestCreated, 0)
	assertContactRequestCount(t, ctx, pool, 0)

	open, err := repository.SetContactPrivacy(ctx, setPrivacyCommand("bob", true, "privacy-open"))
	if err != nil {
		t.Fatalf("set privacy open: %v", err)
	}
	if !open.Settings.AllowContactRequests ||
		open.Settings.Version != 2 ||
		open.Settings.PolicySource != types.ContactPrivacyPolicySourceUser {
		t.Fatalf("unexpected open privacy result: %+v", open)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactPrivacyUpdated, 2)

	sendResult, err := repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-privacy-open", "hello again"))
	if err != nil {
		t.Fatalf("send contact request after open privacy: %v", err)
	}
	if sendResult.Status != types.ContactRequestStatusPending {
		t.Fatalf("unexpected send after open privacy: %+v", sendResult)
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestCreated, 1)
	assertContactRequestCount(t, ctx, pool, 1)
}

func TestRepositoryContactPrivacyBlocksSearchRequestsOnlyIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	allowSearch := false
	result, err := repository.SetContactPrivacy(ctx, types.SetContactPrivacyCommand{
		AuthContext:                authContext("bob", "privacy-search-close"),
		AllowContactRequests:       true,
		AllowSearchContactRequests: &allowSearch,
		IdempotencyKey:             "privacy-search-close",
	})
	if err != nil {
		t.Fatalf("set search privacy closed: %v", err)
	}
	if !result.Settings.AllowContactRequests ||
		result.Settings.AllowSearchContactRequests ||
		result.Settings.Version != 1 ||
		result.Settings.PolicySource != types.ContactPrivacyPolicySourceUser {
		t.Fatalf("unexpected search privacy result: %+v", result)
	}

	search := sendCommand("alice", "bob", "send-search-denied", "hello from search")
	search.SourceType = types.ContactRequestSourceTypeSearch
	_, err = repository.SendContactRequest(ctx, search)
	if !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected search request denied by target privacy, got %v", err)
	}
	assertContactRequestCount(t, ctx, pool, 0)
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestCreated, 0)

	direct, err := repository.SendContactRequest(ctx, sendCommand("carol", "bob", "send-direct-allowed", "hello direct"))
	if err != nil {
		t.Fatalf("send direct request after search privacy closed: %v", err)
	}
	if direct.Status != types.ContactRequestStatusPending ||
		direct.SourceType != types.ContactRequestSourceTypeDirect {
		t.Fatalf("unexpected direct result: %+v", direct)
	}
	assertContactRequestCount(t, ctx, pool, 1)
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestCreated, 1)
}

func TestRepositoryContactPrivacyProfileVisibilityIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	defaultPrivacy, err := repository.GetContactPrivacy(ctx, getPrivacyCommand("bob"))
	if err != nil {
		t.Fatalf("get default profile privacy: %v", err)
	}
	if !defaultPrivacy.Settings.AllowProfileVisibility ||
		len(defaultPrivacy.Settings.ProfileVisibilityFields) != 4 ||
		defaultPrivacy.Settings.PolicySource != types.ContactPrivacyPolicySourceSystemDefault {
		t.Fatalf("unexpected default profile privacy: %+v", defaultPrivacy)
	}

	allowProfile := false
	closed, err := repository.SetContactPrivacy(ctx, types.SetContactPrivacyCommand{
		AuthContext:            authContext("bob", "privacy-profile-close"),
		AllowContactRequests:   true,
		AllowProfileVisibility: &allowProfile,
		IdempotencyKey:         "privacy-profile-close",
	})
	if err != nil {
		t.Fatalf("set profile privacy closed: %v", err)
	}
	if closed.Settings.AllowProfileVisibility ||
		len(closed.Settings.ProfileVisibilityFields) != 0 ||
		!closed.Settings.AllowContactRequests ||
		!closed.Settings.AllowSearchContactRequests ||
		closed.Settings.Version != 1 ||
		closed.Settings.PolicySource != types.ContactPrivacyPolicySourceUser {
		t.Fatalf("unexpected closed profile privacy: %+v", closed)
	}

	replay, err := repository.SetContactPrivacy(ctx, types.SetContactPrivacyCommand{
		AuthContext:            authContext("bob", "privacy-profile-close"),
		AllowContactRequests:   true,
		AllowProfileVisibility: &allowProfile,
		IdempotencyKey:         "privacy-profile-close",
	})
	if err != nil {
		t.Fatalf("set profile privacy replay: %v", err)
	}
	if !replay.IdempotentReplay ||
		replay.Settings.AllowProfileVisibility ||
		len(replay.Settings.ProfileVisibilityFields) != 0 ||
		replay.Settings.Version != closed.Settings.Version {
		t.Fatalf("unexpected profile privacy replay: %+v", replay)
	}

	var payloadProfileVisibility bool
	err = pool.QueryRow(ctx, `
SELECT (payload_json->>'allow_profile_visibility')::boolean
FROM contacts_outbox
WHERE tenant_id = 'tenant-contacts'
  AND event_type = $1
`, eventTypeContactPrivacyUpdated).Scan(&payloadProfileVisibility)
	if err != nil {
		t.Fatalf("query profile privacy outbox payload: %v", err)
	}
	if payloadProfileVisibility {
		t.Fatalf("expected profile visibility false in privacy outbox payload")
	}
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactPrivacyUpdated, 1)

	allowProfile = true
	fields := []types.ContactProfileVisibilityField{
		types.ContactProfileVisibilityFieldDisplayName,
		types.ContactProfileVisibilityFieldStatusMessage,
	}
	fieldResult, err := repository.SetContactPrivacy(ctx, types.SetContactPrivacyCommand{
		AuthContext:                   authContext("bob", "privacy-profile-fields"),
		AllowContactRequests:          true,
		AllowProfileVisibility:        &allowProfile,
		UpdateProfileVisibilityFields: true,
		ProfileVisibilityFields:       fields,
		IdempotencyKey:                "privacy-profile-fields",
	})
	if err != nil {
		t.Fatalf("set profile privacy fields: %v", err)
	}
	if !fieldResult.Settings.AllowProfileVisibility ||
		len(fieldResult.Settings.ProfileVisibilityFields) != 2 ||
		fieldResult.Settings.ProfileVisibilityFields[1] != types.ContactProfileVisibilityFieldStatusMessage {
		t.Fatalf("unexpected profile visibility fields result: %+v", fieldResult)
	}
	var payloadProfileFields []string
	err = pool.QueryRow(ctx, `
SELECT array_agg(value::text ORDER BY ordinality)
FROM contacts_outbox, jsonb_array_elements_text(payload_json->'profile_visibility_fields') WITH ORDINALITY AS fields(value, ordinality)
WHERE tenant_id = 'tenant-contacts'
  AND event_type = $1
  AND payload_json->>'user_id' = 'bob'
  AND (payload_json->>'allow_profile_visibility')::boolean = true
`, eventTypeContactPrivacyUpdated).Scan(&payloadProfileFields)
	if err != nil {
		t.Fatalf("query profile privacy fields outbox payload: %v", err)
	}
	if len(payloadProfileFields) != 2 ||
		payloadProfileFields[0] != "DISPLAY_NAME" ||
		payloadProfileFields[1] != "STATUS_MESSAGE" {
		t.Fatalf("unexpected profile visibility fields payload: %+v", payloadProfileFields)
	}
}

func TestRepositoryTenantProfilePrivacyDefaultInheritanceIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	allowTenantProfile := false
	tenantDefault, err := repository.SetTenantContactPrivacyDefault(ctx, types.SetTenantContactPrivacyDefaultCommand{
		TenantID:               "tenant-contacts",
		AllowContactRequests:   true,
		AllowProfileVisibility: &allowTenantProfile,
	})
	if err != nil {
		t.Fatalf("set tenant profile privacy default: %v", err)
	}
	if tenantDefault.Settings.AllowProfileVisibility ||
		len(tenantDefault.Settings.ProfileVisibilityFields) != 0 ||
		tenantDefault.Settings.PolicySource != types.ContactPrivacyPolicySourceTenantDefault {
		t.Fatalf("unexpected tenant profile privacy default: %+v", tenantDefault)
	}

	inherited, err := repository.GetContactPrivacy(ctx, getPrivacyCommand("bob"))
	if err != nil {
		t.Fatalf("get inherited profile privacy: %v", err)
	}
	if inherited.Settings.AllowProfileVisibility ||
		len(inherited.Settings.ProfileVisibilityFields) != 0 ||
		inherited.Settings.PolicySource != types.ContactPrivacyPolicySourceTenantDefault ||
		inherited.Settings.Version != tenantDefault.Settings.Version {
		t.Fatalf("unexpected inherited profile privacy: %+v", inherited)
	}

	allowUserProfile := true
	userOverride, err := repository.SetContactPrivacy(ctx, types.SetContactPrivacyCommand{
		AuthContext:            authContext("bob", "privacy-profile-open"),
		AllowContactRequests:   true,
		AllowProfileVisibility: &allowUserProfile,
		IdempotencyKey:         "privacy-profile-open",
	})
	if err != nil {
		t.Fatalf("set user profile privacy open: %v", err)
	}
	if !userOverride.Settings.AllowProfileVisibility ||
		len(userOverride.Settings.ProfileVisibilityFields) != 4 ||
		userOverride.Settings.PolicySource != types.ContactPrivacyPolicySourceUser {
		t.Fatalf("unexpected user profile privacy override: %+v", userOverride)
	}

	tenantFields, err := repository.SetTenantContactPrivacyDefault(ctx, types.SetTenantContactPrivacyDefaultCommand{
		TenantID:                      "tenant-contacts",
		AllowContactRequests:          true,
		AllowProfileVisibility:        &allowUserProfile,
		UpdateProfileVisibilityFields: true,
		ProfileVisibilityFields: []types.ContactProfileVisibilityField{
			types.ContactProfileVisibilityFieldDisplayName,
			types.ContactProfileVisibilityFieldAvatar,
		},
	})
	if err != nil {
		t.Fatalf("set tenant profile privacy fields: %v", err)
	}
	if len(tenantFields.Settings.ProfileVisibilityFields) != 2 ||
		tenantFields.Settings.ProfileVisibilityFields[1] != types.ContactProfileVisibilityFieldAvatar {
		t.Fatalf("unexpected tenant profile visibility fields: %+v", tenantFields)
	}
}

func TestRepositoryTenantPrivacyDefaultBlocksSearchUnlessUserOverridesIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	allowTenantSearch := false
	tenantDefault, err := repository.SetTenantContactPrivacyDefault(ctx, types.SetTenantContactPrivacyDefaultCommand{
		TenantID:                   "tenant-contacts",
		AllowContactRequests:       true,
		AllowSearchContactRequests: &allowTenantSearch,
	})
	if err != nil {
		t.Fatalf("set tenant search privacy default: %v", err)
	}
	if !tenantDefault.Settings.AllowContactRequests ||
		tenantDefault.Settings.AllowSearchContactRequests ||
		tenantDefault.Settings.PolicySource != types.ContactPrivacyPolicySourceTenantDefault {
		t.Fatalf("unexpected tenant search privacy default: %+v", tenantDefault)
	}

	search := sendCommand("alice", "bob", "send-tenant-search-denied", "hello from search")
	search.SourceType = types.ContactRequestSourceTypeSearch
	_, err = repository.SendContactRequest(ctx, search)
	if !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected search request denied by tenant privacy default, got %v", err)
	}

	allowUserSearch := true
	userOverride, err := repository.SetContactPrivacy(ctx, types.SetContactPrivacyCommand{
		AuthContext:                authContext("bob", "privacy-user-search-open"),
		AllowContactRequests:       true,
		AllowSearchContactRequests: &allowUserSearch,
		IdempotencyKey:             "privacy-user-search-open",
	})
	if err != nil {
		t.Fatalf("set user search privacy open: %v", err)
	}
	if !userOverride.Settings.AllowSearchContactRequests ||
		userOverride.Settings.PolicySource != types.ContactPrivacyPolicySourceUser {
		t.Fatalf("unexpected user search override: %+v", userOverride)
	}

	allowed := sendCommand("alice", "bob", "send-user-search-allowed", "hello again")
	allowed.SourceType = types.ContactRequestSourceTypeSearch
	sendResult, err := repository.SendContactRequest(ctx, allowed)
	if err != nil {
		t.Fatalf("send search request after user override: %v", err)
	}
	if sendResult.Status != types.ContactRequestStatusPending ||
		sendResult.SourceType != types.ContactRequestSourceTypeSearch {
		t.Fatalf("unexpected search send result: %+v", sendResult)
	}
	assertContactRequestCount(t, ctx, pool, 1)
	assertContactsOutboxCount(t, ctx, pool, eventTypeContactRequestCreated, 1)
}
