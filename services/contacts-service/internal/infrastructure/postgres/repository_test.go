package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
		AuthContext: types.AuthContext{
			TenantID:  "tenant-contacts",
			UserID:    types.UserID(sender),
			DeviceID:  "device-1",
			RequestID: "request-" + key,
			TraceID:   "trace-" + key,
		},
		TargetUserID:   types.UserID(target),
		IdempotencyKey: key,
		Message:        message,
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

func listCommand(owner string, pageSize int, pageToken string) types.ListContactsCommand {
	return types.ListContactsCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-contacts",
			UserID:   types.UserID(owner),
		},
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

func insertContactEdge(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner string, contact string) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO contact_edges (
    tenant_id,
    owner_user_id,
    contact_user_id,
    status,
    source_request_id,
    version,
    created_at,
    updated_at
) VALUES ('tenant-contacts', $1, $2, 'ACTIVE', 'seed-request', 1, now(), now())
`, owner, contact)
	if err != nil {
		t.Fatalf("insert contact edge: %v", err)
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
	migrationPath := filepath.Join(root, "migrations", "postgres", "contacts", "000001_contacts_core.sql")
	sqlBytes, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read contacts migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
		t.Fatalf("apply contacts migration: %v", err)
	}
}

func resetContactsTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
TRUNCATE
    contacts_outbox,
    contact_command_idempotency,
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
