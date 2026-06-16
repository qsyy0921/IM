package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

func TestSanitizeContactsOutboxPublishErrorUsesStablePublicMessages(t *testing.T) {
	tests := []struct {
		name string
		err  error
		text string
		want string
	}{
		{
			name: "context canceled",
			err:  context.Canceled,
			text: "context canceled for user=user1@example.com token=secret-token",
			want: "contacts outbox publish canceled",
		},
		{
			name: "deadline exceeded",
			err:  context.DeadlineExceeded,
			text: "deadline exceeded while publishing request-token=secret-token",
			want: "contacts outbox publish timeout",
		},
		{
			name: "unsupported event",
			err:  errors.New("unsupported event_type=contact.future.v9 user=user1@example.com"),
			text: "unsupported event_type=contact.future.v9 user=user1@example.com",
			want: "contacts outbox publish unsupported event",
		},
		{
			name: "invalid payload",
			err:  errors.New("malformed json payload for user=user1@example.com token=secret-token"),
			text: "malformed json payload for user=user1@example.com token=secret-token",
			want: "contacts outbox publish invalid payload",
		},
		{
			name: "broker unavailable",
			err:  errors.New("kafka broker connection refused at 10.0.0.8 token=secret-token"),
			text: "kafka broker connection refused at 10.0.0.8 token=secret-token",
			want: "contacts outbox publish broker unavailable",
		},
		{
			name: "unknown raw error",
			err:  errors.New("provider body user=user1@example.com token=secret-token nonce=secret-nonce"),
			text: "provider body user=user1@example.com token=secret-token nonce=secret-nonce",
			want: "contacts outbox publish failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeContactsOutboxPublishError(tt.err); got != tt.want {
				t.Fatalf("sanitize publish error = %q, want %q", got, tt.want)
			}
			if got := sanitizeContactsOutboxStoredError(tt.text); got != tt.want {
				t.Fatalf("sanitize stored error = %q, want %q", got, tt.want)
			}
			for _, forbidden := range []string{"user1@example.com", "secret-token", "secret-nonce", "10.0.0.8"} {
				if strings.Contains(tt.want, forbidden) {
					t.Fatalf("stable contacts outbox error leaked sensitive text %q in %q", forbidden, tt.want)
				}
			}
		})
	}
	if got := sanitizeContactsOutboxStoredError("   "); got != "" {
		t.Fatalf("blank stored error = %q, want empty", got)
	}
}

func TestOutboxStoreProcessReadyBatchMarksPublishedIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)
	store := NewOutboxStore(pool, WithOutboxClock(func() time.Time {
		return time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	}))

	sendResult, err := repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-1", "hello"))
	if err != nil {
		t.Fatalf("send contact request: %v", err)
	}
	if _, err := repository.RespondContactRequest(ctx, respondCommand("bob", sendResult.RequestID, "respond-1", types.ContactDecisionAccept)); err != nil {
		t.Fatalf("accept contact request: %v", err)
	}

	var published []string
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for index, message := range messages {
			published = append(published, message.EventType)
			errs[index] = nil
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process first ready: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 1 || len(published) != 1 || published[0] != types.ContactEventRequestCreated {
		t.Fatalf("unexpected stats=%+v published=%v", stats, published)
	}
	stats, err = store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for index, message := range messages {
			published = append(published, message.EventType)
			errs[index] = nil
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process second ready: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 1 || len(published) != 2 || published[1] != types.ContactEventRequestAccepted {
		t.Fatalf("unexpected second stats=%+v published=%v", stats, published)
	}
	assertContactsOutboxStatusCount(t, ctx, pool, types.OutboxStatusPublished, 2)
	assertContactsOutboxStatusCount(t, ctx, pool, types.OutboxStatusPending, 0)
}

func TestOutboxStoreRetriesAndDLQIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)
	store := NewOutboxStore(pool)

	if _, err := repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-1", "hello")); err != nil {
		t.Fatalf("send contact request: %v", err)
	}
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		return []error{errors.New("kafka unavailable: broker body user=user1@example.com token=secret-token")}
	})
	if err != nil {
		t.Fatalf("process retry: %v", err)
	}
	if stats.Retried != 1 {
		t.Fatalf("expected retry stats, got %+v", stats)
	}
	assertContactsOutboxStatusCount(t, ctx, pool, types.OutboxStatusPending, 1)
	assertContactsOutboxLastError(t, ctx, pool, types.OutboxStatusPending, "contacts outbox publish broker unavailable", "user1@example.com", "secret-token", "broker body")

	_, err = pool.Exec(ctx, `
UPDATE contacts_outbox
SET next_retry_at = now() - interval '1 second',
    retry_count = 2
WHERE tenant_id = 'tenant-contacts'
`)
	if err != nil {
		t.Fatalf("make outbox ready for dlq: %v", err)
	}
	stats, err = store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		return []error{errors.New("malformed payload: provider body token=secret-token")}
	})
	if err != nil {
		t.Fatalf("process dlq: %v", err)
	}
	if stats.DeadLettered != 1 {
		t.Fatalf("expected dlq stats, got %+v", stats)
	}
	assertContactsOutboxStatusCount(t, ctx, pool, types.OutboxStatusDLQ, 1)
	assertContactsOutboxLastError(t, ctx, pool, types.OutboxStatusDLQ, "contacts outbox publish invalid payload", "malformed payload", "secret-token")
}

func TestOutboxStoreBlocksHigherVersionByPartitionKeyIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)
	store := NewOutboxStore(pool)

	sendResult, err := repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-1", "hello"))
	if err != nil {
		t.Fatalf("send contact request: %v", err)
	}
	if _, err := repository.RespondContactRequest(ctx, respondCommand("bob", sendResult.RequestID, "respond-1", types.ContactDecisionAccept)); err != nil {
		t.Fatalf("accept contact request: %v", err)
	}
	_, err = pool.Exec(ctx, `
UPDATE contacts_outbox
SET status = 'DLQ'
WHERE event_type = $1
`, types.ContactEventRequestCreated)
	if err != nil {
		t.Fatalf("mark created dlq: %v", err)
	}
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		t.Fatalf("publish should not be called while lower version is DLQ: %+v", messages)
		return nil
	})
	if err != nil {
		t.Fatalf("process ready: %v", err)
	}
	if stats.Fetched != 0 {
		t.Fatalf("expected no fetched messages, got %+v", stats)
	}
}

func TestOutboxStoreRepairDLQEventUnblocksPartitionIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)
	store := NewOutboxStore(pool)

	sendResult, err := repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-1", "hello"))
	if err != nil {
		t.Fatalf("send contact request: %v", err)
	}
	if _, err := repository.RespondContactRequest(ctx, respondCommand("bob", sendResult.RequestID, "respond-1", types.ContactDecisionAccept)); err != nil {
		t.Fatalf("accept contact request: %v", err)
	}
	createdEventID := contactsOutboxEventID(t, ctx, pool, types.ContactEventRequestCreated)
	_, err = pool.Exec(ctx, `
UPDATE contacts_outbox
SET status = 'DLQ',
    retry_count = 3,
    last_error = 'kafka unavailable: broker body user=user1@example.com token=secret-token',
    dead_lettered_at = now()
WHERE event_id = $1
`, createdEventID)
	if err != nil {
		t.Fatalf("mark created dlq: %v", err)
	}

	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		t.Fatalf("publish should not be called while lower version is DLQ: %+v", messages)
		return nil
	})
	if err != nil {
		t.Fatalf("process blocked ready: %v", err)
	}
	if stats.Fetched != 0 {
		t.Fatalf("expected no fetched messages before repair, got %+v", stats)
	}

	repairStats, err := store.RepairDLQEvents(ctx, []string{createdEventID, createdEventID, "missing-event"}, "operator retried after kafka recovery")
	if err != nil {
		t.Fatalf("repair dlq: %v", err)
	}
	if repairStats.Requested != 2 || repairStats.Repaired != 1 || repairStats.Skipped != 1 {
		t.Fatalf("unexpected repair stats: %+v", repairStats)
	}
	assertContactsOutboxRepairAudit(t, ctx, pool, createdEventID, "operator retried after kafka recovery", "contacts outbox publish broker unavailable", "user1@example.com", "secret-token", "broker body")

	var published []string
	stats, err = store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for index, message := range messages {
			published = append(published, message.EventType)
			errs[index] = nil
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process repaired ready: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 1 || len(published) != 1 || published[0] != types.ContactEventRequestCreated {
		t.Fatalf("unexpected first publish after repair stats=%+v published=%v", stats, published)
	}
	stats, err = store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for index, message := range messages {
			published = append(published, message.EventType)
			errs[index] = nil
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process unblocked ready: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 1 || len(published) != 2 || published[1] != types.ContactEventRequestAccepted {
		t.Fatalf("unexpected second publish after repair stats=%+v published=%v", stats, published)
	}
	assertContactsOutboxStatusCount(t, ctx, pool, types.OutboxStatusPublished, 2)
}

func TestOutboxStoreAuditOutboxReturnsLatestRowsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	if _, err := repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-1", "hello")); err != nil {
		t.Fatalf("send contact request: %v", err)
	}
	if _, err := repository.SendContactRequest(ctx, sendCommand("carol", "dave", "send-2", "hi")); err != nil {
		t.Fatalf("send second contact request: %v", err)
	}

	rows, err := NewOutboxStore(pool).AuditOutbox(ctx, OutboxAuditOptions{
		TenantID: "tenant-contacts",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("audit contacts outbox: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two rows, got %d", len(rows))
	}
	if rows[0].EventType != types.ContactEventRequestCreated || rows[0].AggregateVersion != 1 || rows[0].Status != types.OutboxStatusPending {
		t.Fatalf("unexpected latest contacts outbox row: %+v", rows[0])
	}
	if rows[1].EventType != types.ContactEventRequestCreated || rows[1].AggregateVersion != 1 || rows[1].Status != types.OutboxStatusPending {
		t.Fatalf("unexpected older contacts outbox row: %+v", rows[1])
	}
	if !rows[0].CreatedAt.After(rows[1].CreatedAt) && rows[0].ID <= rows[1].ID {
		t.Fatalf("expected latest row first, got row0=%+v row1=%+v", rows[0], rows[1])
	}
}

func TestOutboxStoreAuditOutboxFiltersStatusAndEventTypeIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)

	sendResult, err := repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-1", "hello"))
	if err != nil {
		t.Fatalf("send contact request: %v", err)
	}
	if _, err := repository.RespondContactRequest(ctx, respondCommand("bob", sendResult.RequestID, "respond-1", types.ContactDecisionAccept)); err != nil {
		t.Fatalf("accept contact request: %v", err)
	}
	createdEventID := contactsOutboxEventID(t, ctx, pool, types.ContactEventRequestCreated)
	_, err = pool.Exec(ctx, `
UPDATE contacts_outbox
SET status = 'DLQ',
    retry_count = 2,
    last_error = 'kafka unavailable: broker body user=user1@example.com token=secret-token',
    dead_lettered_at = now()
WHERE event_id = $1
`, createdEventID)
	if err != nil {
		t.Fatalf("mark outbox dlq: %v", err)
	}

	rows, err := NewOutboxStore(pool).AuditOutbox(ctx, OutboxAuditOptions{
		TenantID:  "tenant-contacts",
		Status:    types.OutboxStatusDLQ,
		EventType: types.ContactEventRequestCreated,
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("audit filtered contacts outbox: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if rows[0].EventID != createdEventID || rows[0].Status != types.OutboxStatusDLQ || rows[0].LastError != "contacts outbox publish broker unavailable" {
		t.Fatalf("unexpected filtered contacts outbox row: %+v", rows[0])
	}
	assertContactsOutboxErrorDoesNotContain(t, rows[0].LastError, "user1@example.com", "secret-token", "broker body")
}

func TestOutboxStoreAuditOutboxRepairsReturnsLatestRowsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO contacts_outbox_repair_audit (
    event_id, tenant_id, previous_status, previous_retry_count, previous_last_error, previous_dead_lettered_at, repair_reason, repaired_at
) VALUES
    ('event-11', 'tenant-a', 'DLQ', 1, 'malformed payload user=user1@example.com', now() - interval '1 minute', 'manual audit', now() - interval '1 minute'),
    ('event-12', 'tenant-a', 'DLQ', 2, 'kafka unavailable token=secret-token', now() - interval '2 minutes', 'provider recovered', now())
`)
	if err != nil {
		t.Fatalf("seed contacts outbox repair audit: %v", err)
	}

	store := NewOutboxStore(pool)
	rows, err := store.AuditOutboxRepairs(ctx, OutboxRepairAuditOptions{
		TenantID: "tenant-a",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("audit contacts outbox repairs: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two rows, got %d", len(rows))
	}
	if rows[0].EventID != "event-12" || rows[0].Reason != "provider recovered" || rows[0].PreviousRetryCount != 2 {
		t.Fatalf("unexpected latest contacts outbox repair audit row: %+v", rows[0])
	}
	if rows[0].PreviousLastError != "contacts outbox publish broker unavailable" {
		t.Fatalf("unexpected latest sanitized repair error: %q", rows[0].PreviousLastError)
	}
	assertContactsOutboxErrorDoesNotContain(t, rows[0].PreviousLastError, "secret-token")
	if rows[1].EventID != "event-11" || rows[1].Reason != "manual audit" || rows[1].PreviousRetryCount != 1 {
		t.Fatalf("unexpected older contacts outbox repair audit row: %+v", rows[1])
	}
	if rows[1].PreviousLastError != "contacts outbox publish invalid payload" {
		t.Fatalf("unexpected older sanitized repair error: %q", rows[1].PreviousLastError)
	}
	assertContactsOutboxErrorDoesNotContain(t, rows[1].PreviousLastError, "user1@example.com")
}

func TestOutboxStoreAuditOutboxRepairsFiltersEventAndTenantIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO contacts_outbox_repair_audit (
    event_id, tenant_id, previous_status, previous_retry_count, previous_last_error, previous_dead_lettered_at, repair_reason, repaired_at
) VALUES
    ('event-21', 'tenant-b', 'DLQ', 1, 'malformed payload user=user1@example.com', now() - interval '1 minute', 'manual audit', now()),
    ('event-22', 'tenant-c', 'DLQ', 2, 'provider unavailable', now() - interval '2 minutes', 'provider recovered', now() - interval '1 minute')
`)
	if err != nil {
		t.Fatalf("seed contacts outbox repair audit: %v", err)
	}

	store := NewOutboxStore(pool)
	rows, err := store.AuditOutboxRepairs(ctx, OutboxRepairAuditOptions{
		EventID:  "event-21",
		TenantID: "tenant-b",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("audit contacts outbox repairs with filters: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if rows[0].EventID != "event-21" || rows[0].TenantID != "tenant-b" {
		t.Fatalf("unexpected filtered contacts outbox repair audit row: %+v", rows[0])
	}
	if rows[0].PreviousLastError != "contacts outbox publish invalid payload" {
		t.Fatalf("unexpected sanitized filtered repair error: %q", rows[0].PreviousLastError)
	}
	assertContactsOutboxErrorDoesNotContain(t, rows[0].PreviousLastError, "user1@example.com")
}

func TestOutboxStoreAuditOutboxRepairsFiltersRepairedAtIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)

	base := time.Date(2026, 6, 16, 2, 0, 0, 0, time.UTC)
	_, err := pool.Exec(ctx, `
INSERT INTO contacts_outbox_repair_audit (
    event_id, tenant_id, previous_status, previous_retry_count, previous_last_error, previous_dead_lettered_at, repair_reason, repaired_at
) VALUES
    ('event-window-old', 'tenant-window', 'DLQ', 1, 'publish failed', $1::timestamptz, 'manual old', $2::timestamptz),
    ('event-window-hit', 'tenant-window', 'DLQ', 2, 'provider unavailable', $1::timestamptz, 'manual hit', $3::timestamptz),
    ('event-window-new', 'tenant-window', 'DLQ', 3, 'provider unavailable', $1::timestamptz, 'manual new', $4::timestamptz)
`, base, base.Add(-time.Hour), base, base.Add(time.Hour))
	if err != nil {
		t.Fatalf("seed contacts outbox repair audit window rows: %v", err)
	}

	store := NewOutboxStore(pool)
	rows, err := store.AuditOutboxRepairs(ctx, OutboxRepairAuditOptions{
		TenantID:       "tenant-window",
		RepairedAfter:  ptrTime(base.Add(-30 * time.Minute)),
		RepairedBefore: ptrTime(base.Add(30 * time.Minute)),
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("audit contacts outbox repairs by repaired_at: %v", err)
	}
	if len(rows) != 1 || rows[0].EventID != "event-window-hit" {
		t.Fatalf("unexpected repaired_at window rows: %+v", rows)
	}

	_, err = store.AuditOutboxRepairs(ctx, OutboxRepairAuditOptions{
		TenantID:       "tenant-window",
		RepairedAfter:  ptrTime(base),
		RepairedBefore: ptrTime(base),
		Limit:          10,
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid repaired_at window, got %v", err)
	}
}

func TestOutboxStoreCleanupOutboxRepairsDeletesOnlyExpiredRowsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO contacts_outbox_repair_audit (
    event_id, tenant_id, previous_status, previous_retry_count, previous_last_error, previous_dead_lettered_at, repair_reason, repaired_at
) VALUES
    ('cleanup-event-1', 'tenant-cleanup', 'DLQ', 1, 'publish failed', now() - interval '1 minute', 'manual audit', now() - interval '10 days'),
    ('cleanup-event-2', 'tenant-cleanup', 'DLQ', 2, 'provider unavailable', now() - interval '2 minutes', 'provider recovered', now() - interval '1 day')
`)
	if err != nil {
		t.Fatalf("seed contacts outbox cleanup rows: %v", err)
	}

	stats, err := NewOutboxStore(pool).CleanupOutboxRepairs(ctx, OutboxRepairCleanupOptions{
		TenantID: "tenant-cleanup",
		Cutoff:   time.Now().UTC().Add(-7 * 24 * time.Hour),
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("cleanup contacts outbox repairs: %v", err)
	}
	if stats.Deleted != 1 {
		t.Fatalf("unexpected deleted count: %+v", stats)
	}
	assertContactsOutboxRepairAuditCount(t, ctx, pool, "tenant-cleanup", 1)
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

func TestOutboxStoreCleanupOutboxRepairsHonorsBatchLimitIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO contacts_outbox_repair_audit (
    event_id, tenant_id, previous_status, previous_retry_count, previous_last_error, previous_dead_lettered_at, repair_reason, repaired_at
) VALUES
    ('cleanup-event-11', 'tenant-limit', 'DLQ', 1, 'publish failed', now() - interval '1 minute', 'manual audit', now() - interval '10 days'),
    ('cleanup-event-12', 'tenant-limit', 'DLQ', 2, 'provider unavailable', now() - interval '2 minutes', 'provider recovered', now() - interval '9 days')
`)
	if err != nil {
		t.Fatalf("seed contacts outbox cleanup rows: %v", err)
	}

	stats, err := NewOutboxStore(pool).CleanupOutboxRepairs(ctx, OutboxRepairCleanupOptions{
		TenantID: "tenant-limit",
		Cutoff:   time.Now().UTC().Add(-7 * 24 * time.Hour),
		Limit:    1,
	})
	if err != nil {
		t.Fatalf("cleanup contacts outbox repairs with batch limit: %v", err)
	}
	if stats.Deleted != 1 {
		t.Fatalf("unexpected deleted count: %+v", stats)
	}
	assertContactsOutboxRepairAuditCount(t, ctx, pool, "tenant-limit", 1)
}

func TestOutboxStoreCleanupOutboxRepairsFiltersEventAndTenantIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO contacts_outbox_repair_audit (
    event_id, tenant_id, previous_status, previous_retry_count, previous_last_error, previous_dead_lettered_at, repair_reason, repaired_at
) VALUES
    ('cleanup-event-21', 'tenant-filter', 'DLQ', 1, 'publish failed', now() - interval '1 minute', 'manual audit', now() - interval '10 days'),
    ('cleanup-event-22', 'tenant-other', 'DLQ', 2, 'provider unavailable', now() - interval '2 minutes', 'provider recovered', now() - interval '10 days')
`)
	if err != nil {
		t.Fatalf("seed contacts outbox cleanup rows: %v", err)
	}

	stats, err := NewOutboxStore(pool).CleanupOutboxRepairs(ctx, OutboxRepairCleanupOptions{
		EventID:  "cleanup-event-21",
		TenantID: "tenant-filter",
		Cutoff:   time.Now().UTC().Add(-7 * 24 * time.Hour),
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("cleanup contacts outbox repairs with filters: %v", err)
	}
	if stats.Deleted != 1 {
		t.Fatalf("unexpected deleted count: %+v", stats)
	}
	rows, err := NewOutboxStore(pool).AuditOutboxRepairs(ctx, OutboxRepairAuditOptions{Limit: 10})
	if err != nil {
		t.Fatalf("audit contacts outbox repairs after cleanup: %v", err)
	}
	if len(rows) != 1 || rows[0].EventID != "cleanup-event-22" || rows[0].TenantID != "tenant-other" {
		t.Fatalf("unexpected remaining contacts outbox repair audit rows: %+v", rows)
	}
}

func assertContactsOutboxRepairAudit(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID string, wantReason string, wantPreviousError string, forbidden ...string) {
	t.Helper()
	var reason string
	var previousStatus string
	var previousRetryCount int
	var previousError string
	err := pool.QueryRow(ctx, `
SELECT repair_reason, previous_status, previous_retry_count, previous_last_error
FROM contacts_outbox_repair_audit
WHERE tenant_id = 'tenant-contacts'
  AND event_id = $1
`, eventID).Scan(&reason, &previousStatus, &previousRetryCount, &previousError)
	if err != nil {
		t.Fatalf("query contacts outbox repair audit: %v", err)
	}
	if reason != wantReason || previousStatus != types.OutboxStatusDLQ || previousRetryCount != 3 || previousError != wantPreviousError {
		t.Fatalf(
			"unexpected repair audit reason=%q previous_status=%q previous_retry_count=%d previous_error=%q",
			reason,
			previousStatus,
			previousRetryCount,
			previousError,
		)
	}
	assertContactsOutboxErrorDoesNotContain(t, previousError, forbidden...)
}

func assertContactsOutboxLastError(t *testing.T, ctx context.Context, pool *pgxpool.Pool, status string, want string, forbidden ...string) {
	t.Helper()
	var lastError string
	err := pool.QueryRow(ctx, `
SELECT last_error
FROM contacts_outbox
WHERE tenant_id = 'tenant-contacts'
  AND status = $1
LIMIT 1
`, status).Scan(&lastError)
	if err != nil {
		t.Fatalf("query contacts outbox last_error: %v", err)
	}
	if lastError != want {
		t.Fatalf("unexpected contacts outbox last_error: got %q want %q", lastError, want)
	}
	assertContactsOutboxErrorDoesNotContain(t, lastError, forbidden...)
}

func assertContactsOutboxErrorDoesNotContain(t *testing.T, lastError string, forbidden ...string) {
	t.Helper()
	for _, text := range forbidden {
		if text != "" && strings.Contains(lastError, text) {
			t.Fatalf("contacts outbox error leaked %q: %q", text, lastError)
		}
	}
}

func assertContactsOutboxRepairAuditCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID string, want int) {
	t.Helper()
	var got int
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM contacts_outbox_repair_audit
WHERE tenant_id = $1
`, tenantID).Scan(&got)
	if err != nil {
		t.Fatalf("count contacts outbox repair audit: %v", err)
	}
	if got != want {
		t.Fatalf("expected %d contacts outbox repair audit rows, got %d", want, got)
	}
}

func assertContactsOutboxStatusCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, status string, want int) {
	t.Helper()
	var got int
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM contacts_outbox
WHERE tenant_id = 'tenant-contacts'
  AND status = $1
`, status).Scan(&got)
	if err != nil {
		t.Fatalf("count contacts outbox status: %v", err)
	}
	if got != want {
		t.Fatalf("expected %d contacts outbox rows with status %s, got %d", want, status, got)
	}
}

func contactsOutboxEventID(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventType string) string {
	t.Helper()
	var eventID string
	err := pool.QueryRow(ctx, `
SELECT event_id
FROM contacts_outbox
WHERE tenant_id = 'tenant-contacts'
  AND event_type = $1
`, eventType).Scan(&eventID)
	if err != nil {
		t.Fatalf("query contacts outbox event id: %v", err)
	}
	return eventID
}
