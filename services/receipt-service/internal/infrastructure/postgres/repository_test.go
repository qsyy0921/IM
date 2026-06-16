package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

func TestRepositoryProjectDeliveryEventAndMarkReadIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	_, err := repository.ProjectDeliveryEvent(ctx, inboxCreatedCommand(1, "delivery-inbox-1"))
	if err != nil {
		t.Fatalf("project inbox item: %v", err)
	}
	_, err = repository.ProjectDeliveryEvent(ctx, ackRecordedCommand(1, "delivery-ack-1"))
	if err != nil {
		t.Fatalf("project ack: %v", err)
	}
	state, err := repository.GetReceiptState(ctx, getStateCommandBySeq(1))
	if err != nil {
		t.Fatalf("get receipt state after ack: %v", err)
	}
	if state.ReceivedUserCount != 1 || state.ReadUserCount != 0 {
		t.Fatalf("unexpected counts after ack: received=%d read=%d", state.ReceivedUserCount, state.ReadUserCount)
	}
	summary, err := repository.ListConversations(ctx, listConversationsCommand(10, ""))
	if err != nil {
		t.Fatalf("list conversations before read: %v", err)
	}
	assertConversationSummary(t, summary, 1, 1, 0)

	result, err := repository.MarkRead(ctx, markReadCommand(1))
	if err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if result.LastReadSeq != 1 {
		t.Fatalf("expected last_read_seq=1, got %d", result.LastReadSeq)
	}
	state, err = repository.GetReceiptState(ctx, getStateCommandByMessage("message-1"))
	if err != nil {
		t.Fatalf("get receipt state after read: %v", err)
	}
	if state.ReceivedUserCount != 1 || state.ReadUserCount != 1 {
		t.Fatalf("unexpected counts after read: received=%d read=%d", state.ReceivedUserCount, state.ReadUserCount)
	}
	summary, err = repository.ListConversations(ctx, listConversationsCommand(10, ""))
	if err != nil {
		t.Fatalf("list conversations after read: %v", err)
	}
	assertConversationSummary(t, summary, 1, 0, 1)
	assertReceiptOutboxCount(t, ctx, pool, "receipt.message.received.v1", 1)
	assertReceiptOutboxCount(t, ctx, pool, "receipt.message.read.v1", 1)
	assertReceiptOutboxPayload(t, ctx, pool, "receipt.message.received.v1", "message-1", "timeline-event-1", "device-1", 1)
	assertReceiptOutboxPayload(t, ctx, pool, "receipt.message.read.v1", "message-1", "timeline-event-1", "device-1", 1)

	result, err = repository.MarkRead(ctx, markReadCommand(1))
	if err != nil {
		t.Fatalf("repeat mark read should be idempotent: %v", err)
	}
	if result.LastReadSeq != 1 {
		t.Fatalf("expected idempotent read seq 1, got %d", result.LastReadSeq)
	}
	assertReceiptOutboxCount(t, ctx, pool, "receipt.message.read.v1", 1)
}

func TestRepositoryListReceiptStatesBatchIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	if _, err := repository.ProjectDeliveryEvent(ctx, inboxCreatedCommand(1, "delivery-inbox-1")); err != nil {
		t.Fatalf("project first inbox item: %v", err)
	}
	if _, err := repository.ProjectDeliveryEvent(ctx, ackRecordedCommand(1, "delivery-ack-1")); err != nil {
		t.Fatalf("project first ack: %v", err)
	}
	if _, err := repository.MarkRead(ctx, markReadCommand(1)); err != nil {
		t.Fatalf("mark first item read: %v", err)
	}
	if _, err := repository.ProjectDeliveryEvent(ctx, inboxCreatedCommand(2, "delivery-inbox-2")); err != nil {
		t.Fatalf("project second inbox item: %v", err)
	}

	result, err := repository.ListReceiptStates(ctx, types.ListReceiptStatesCommand{
		AuthContext:    getStateCommandBySeq(1).AuthContext,
		ConversationID: "conv-receipt",
		Items: []types.ReceiptStateQuery{
			{ConversationSeq: 2},
			{MessageID: "message-1"},
		},
	})
	if err != nil {
		t.Fatalf("list receipt states: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 receipt states, got %d: %+v", len(result.Items), result.Items)
	}
	if result.Items[0].ConversationSeq != 2 ||
		result.Items[0].MessageID != "message-2" ||
		result.Items[0].ReceivedUserCount != 0 ||
		result.Items[0].ReadUserCount != 0 {
		t.Fatalf("unexpected first batch item: %+v", result.Items[0])
	}
	if result.Items[1].ConversationSeq != 1 ||
		result.Items[1].MessageID != "message-1" ||
		result.Items[1].ReceivedUserCount != 1 ||
		result.Items[1].ReadUserCount != 1 {
		t.Fatalf("unexpected second batch item: %+v", result.Items[1])
	}
}

func TestRepositoryReceiptStateCountsReceivedDevicesIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	if _, err := repository.ProjectDeliveryEvent(ctx, inboxCreatedCommand(1, "delivery-inbox-1")); err != nil {
		t.Fatalf("project inbox item: %v", err)
	}
	if _, err := repository.ProjectDeliveryEvent(ctx, ackRecordedCommandForDevice(1, "delivery-ack-device-1", "device-1")); err != nil {
		t.Fatalf("project first device ack: %v", err)
	}
	if _, err := repository.ProjectDeliveryEvent(ctx, ackRecordedCommandForDevice(1, "delivery-ack-device-2", "device-2")); err != nil {
		t.Fatalf("project second device ack: %v", err)
	}

	state, err := repository.GetReceiptState(ctx, getStateCommandBySeq(1))
	if err != nil {
		t.Fatalf("get receipt state: %v", err)
	}
	if state.ReceivedUserCount != 1 || len(state.Receivers) != 1 {
		t.Fatalf("unexpected user receipt state: %+v", state)
	}
	if state.Receivers[0].ReceivedDeviceCount != 2 {
		t.Fatalf("expected received_device_count=2, got %+v", state.Receivers[0])
	}
	if len(state.Receivers[0].ReceivedDevices) != 0 || state.Receivers[0].ReceivedDevicesTruncated {
		t.Fatalf("expected default receipt state to hide device details, got %+v", state.Receivers[0])
	}

	detailedCommand := getStateCommandBySeq(1)
	detailedCommand.IncludeReceivedDevices = true
	detailedCommand.ReceivedDeviceLimitHint = 2
	detailed, err := repository.GetReceiptState(ctx, detailedCommand)
	if err != nil {
		t.Fatalf("get detailed receipt state: %v", err)
	}
	assertReceivedDeviceIDs(t, detailed.Receivers[0], "device-1", "device-2")
	if detailed.Receivers[0].ReceivedDevicesTruncated {
		t.Fatalf("did not expect detailed device list to be truncated: %+v", detailed.Receivers[0])
	}

	batch, err := repository.ListReceiptStates(ctx, types.ListReceiptStatesCommand{
		AuthContext:             getStateCommandBySeq(1).AuthContext,
		ConversationID:          "conv-receipt",
		Items:                   []types.ReceiptStateQuery{{ConversationSeq: 1}},
		IncludeReceivedDevices:  true,
		ReceivedDeviceLimitHint: 1,
	})
	if err != nil {
		t.Fatalf("list receipt states: %v", err)
	}
	if batch.Items[0].Receivers[0].ReceivedDeviceCount != 2 {
		t.Fatalf("expected batch received_device_count=2, got %+v", batch.Items[0].Receivers[0])
	}
	if len(batch.Items[0].Receivers[0].ReceivedDevices) != 1 || !batch.Items[0].Receivers[0].ReceivedDevicesTruncated {
		t.Fatalf("expected batch device details to be limited and truncated, got %+v", batch.Items[0].Receivers[0])
	}
}

func TestRepositoryListReceiptStatesReturnsNotFoundForMissingItemIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	if _, err := repository.ProjectDeliveryEvent(ctx, inboxCreatedCommand(1, "delivery-inbox-1")); err != nil {
		t.Fatalf("project inbox item: %v", err)
	}
	_, err := repository.ListReceiptStates(ctx, types.ListReceiptStatesCommand{
		AuthContext:    getStateCommandBySeq(1).AuthContext,
		ConversationID: "conv-receipt",
		Items: []types.ReceiptStateQuery{
			{ConversationSeq: 1},
			{ConversationSeq: 99},
		},
	})
	if !errors.Is(err, types.ErrReceiptNotFound) {
		t.Fatalf("expected receipt not found, got %v", err)
	}
}

func TestRepositoryMarkReadRejectsOutOfRangeIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	_, err := repository.ProjectDeliveryEvent(ctx, inboxCreatedCommand(1, "delivery-inbox-1"))
	if err != nil {
		t.Fatalf("project inbox item: %v", err)
	}
	_, err = repository.MarkRead(ctx, markReadCommand(2))
	if !errors.Is(err, types.ErrReadOutOfVisibleRange) {
		t.Fatalf("expected visible range error, got %v", err)
	}
	_, err = repository.MarkRead(ctx, markReadCommand(1))
	if !errors.Is(err, types.ErrReadOutOfReceivedRange) {
		t.Fatalf("expected received range error, got %v", err)
	}
}

func TestRepositoryProjectsAckBeforeInboxIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	_, err := repository.ProjectDeliveryEvent(ctx, ackRecordedCommand(2, "delivery-ack-1"))
	if err != nil {
		t.Fatalf("project ack first: %v", err)
	}
	_, err = repository.ProjectDeliveryEvent(ctx, inboxCreatedCommand(1, "delivery-inbox-1"))
	if err != nil {
		t.Fatalf("project inbox item: %v", err)
	}
	state, err := repository.GetReceiptState(ctx, getStateCommandBySeq(1))
	if err != nil {
		t.Fatalf("get receipt state: %v", err)
	}
	if state.ReceivedUserCount != 1 {
		t.Fatalf("expected inbox projected as already received, got %d", state.ReceivedUserCount)
	}
	assertReceiptOutboxCount(t, ctx, pool, "receipt.message.received.v1", 1)
	assertReceiptOutboxPayload(t, ctx, pool, "receipt.message.received.v1", "message-1", "timeline-event-1", "device-1", 1)
}

func TestRepositoryListConversationsConcurrentInboxAndMarkReadIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	if _, err := repository.ProjectDeliveryEvent(ctx, inboxCreatedCommand(1, "delivery-inbox-1")); err != nil {
		t.Fatalf("project first inbox item: %v", err)
	}
	if _, err := repository.ProjectDeliveryEvent(ctx, ackRecordedCommand(1, "delivery-ack-1")); err != nil {
		t.Fatalf("project first ack: %v", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, err := repository.ProjectDeliveryEvent(ctx, inboxCreatedCommand(2, "delivery-inbox-2"))
		errs <- err
	}()
	go func() {
		defer wg.Done()
		<-start
		_, err := repository.MarkRead(ctx, markReadCommand(1))
		errs <- err
	}()
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent operation failed: %v", err)
		}
	}

	summary, err := repository.ListConversations(ctx, listConversationsCommand(10, ""))
	if err != nil {
		t.Fatalf("list conversations after concurrent update: %v", err)
	}
	assertConversationSummary(t, summary, 2, 1, 1)
}

func TestRepositoryMessageChangeEventsDoNotIncreaseUnreadIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	if _, err := repository.ProjectDeliveryEvent(ctx, inboxCreatedCommand(1, "delivery-inbox-1")); err != nil {
		t.Fatalf("project persisted inbox item: %v", err)
	}
	if _, err := repository.ProjectDeliveryEvent(ctx, ackRecordedCommand(1, "delivery-ack-1")); err != nil {
		t.Fatalf("project ack: %v", err)
	}
	if _, err := repository.MarkRead(ctx, markReadCommand(1)); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	for _, eventType := range []string{
		types.SourceEventMessageEdited,
		types.SourceEventMessageRevoked,
		types.SourceEventMessageDeleted,
	} {
		seq := int64(2)
		if eventType == types.SourceEventMessageRevoked {
			seq = 3
		}
		if eventType == types.SourceEventMessageDeleted {
			seq = 4
		}
		command := inboxCreatedCommand(seq, "delivery-"+eventType)
		command.SourceEventID = fmt.Sprintf("timeline-change-%d", seq)
		command.SourceEventType = eventType
		command.MessageID = "message-1"
		if _, err := repository.ProjectDeliveryEvent(ctx, command); err != nil {
			t.Fatalf("project %s: %v", eventType, err)
		}
	}

	summary, err := repository.ListConversations(ctx, listConversationsCommand(10, ""))
	if err != nil {
		t.Fatalf("list conversations after message changes: %v", err)
	}
	assertConversationSummaryWithMessage(t, summary, 4, "message-1", types.SourceEventMessageDeleted, 0, 1)
	assertReceiptOutboxCount(t, ctx, pool, "receipt.message.received.v1", 1)
	assertReceiptOutboxCount(t, ctx, pool, "receipt.message.read.v1", 1)
	assertReceiptStateCount(t, ctx, pool, 1)
}

func TestRepositoryListConversationsPaginatesByStableCursorIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	sortTime := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	insertConversationSummary(t, ctx, pool, "conv-a", 11, sortTime)
	insertConversationSummary(t, ctx, pool, "conv-b", 12, sortTime)
	insertConversationSummary(t, ctx, pool, "conv-c", 13, sortTime.Add(-time.Minute))

	first, err := repository.ListConversations(ctx, listConversationsCommand(1, ""))
	if err != nil {
		t.Fatalf("list first page: %v", err)
	}
	assertConversationIDs(t, first, "conv-a")
	if first.NextPageCursor == "" {
		t.Fatal("expected next cursor for first page")
	}

	second, err := repository.ListConversations(ctx, listConversationsCommand(1, first.NextPageCursor))
	if err != nil {
		t.Fatalf("list second page: %v", err)
	}
	assertConversationIDs(t, second, "conv-b")
	if second.NextPageCursor == "" {
		t.Fatal("expected next cursor for second page")
	}

	third, err := repository.ListConversations(ctx, listConversationsCommand(1, second.NextPageCursor))
	if err != nil {
		t.Fatalf("list third page: %v", err)
	}
	assertConversationIDs(t, third, "conv-c")
	if third.NextPageCursor != "" {
		t.Fatalf("expected empty next cursor on last page, got %q", third.NextPageCursor)
	}
}

func TestRepositoryListConversationsFiltersUnreadOnlyIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	sortTime := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	insertConversationSummary(t, ctx, pool, "conv-a", 11, sortTime)
	insertConversationSummary(t, ctx, pool, "conv-b", 12, sortTime.Add(-time.Minute))
	insertConversationSummary(t, ctx, pool, "conv-c", 13, sortTime.Add(-2*time.Minute))
	setConversationUnread(t, ctx, pool, "conv-b", 0)

	firstCommand := listConversationsCommandUnreadOnly(1, "")
	first, err := repository.ListConversations(ctx, firstCommand)
	if err != nil {
		t.Fatalf("list first unread page: %v", err)
	}
	assertConversationIDs(t, first, "conv-a")
	if first.NextPageCursor == "" {
		t.Fatal("expected next cursor for unread first page")
	}

	second, err := repository.ListConversations(ctx, listConversationsCommandUnreadOnly(1, first.NextPageCursor))
	if err != nil {
		t.Fatalf("list second unread page: %v", err)
	}
	assertConversationIDs(t, second, "conv-c")
	if second.NextPageCursor != "" {
		t.Fatalf("expected empty next cursor on unread last page, got %q", second.NextPageCursor)
	}

	_, err = repository.ListConversations(ctx, listConversationsCommand(1, first.NextPageCursor))
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid cursor when unread_only changes, got %v", err)
	}
}

func TestRepositoryListConversationsSortsUnreadFirstIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	sortTime := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	insertConversationSummary(t, ctx, pool, "conv-read-new", 11, sortTime.Add(3*time.Minute))
	insertConversationSummary(t, ctx, pool, "conv-unread-new", 12, sortTime.Add(2*time.Minute))
	insertConversationSummary(t, ctx, pool, "conv-unread-old", 13, sortTime.Add(time.Minute))
	insertConversationSummary(t, ctx, pool, "conv-read-old", 14, sortTime)
	setConversationUnread(t, ctx, pool, "conv-read-new", 0)
	setConversationUnread(t, ctx, pool, "conv-read-old", 0)

	firstCommand := listConversationsCommandUnreadFirst(1, "")
	first, err := repository.ListConversations(ctx, firstCommand)
	if err != nil {
		t.Fatalf("list first unread-first page: %v", err)
	}
	assertConversationIDs(t, first, "conv-unread-new")
	if first.NextPageCursor == "" {
		t.Fatal("expected unread-first next cursor")
	}
	second, err := repository.ListConversations(ctx, listConversationsCommandUnreadFirst(1, first.NextPageCursor))
	if err != nil {
		t.Fatalf("list second unread-first page: %v", err)
	}
	assertConversationIDs(t, second, "conv-unread-old")
	if second.NextPageCursor == "" {
		t.Fatal("expected unread-first cursor after unread boundary")
	}
	third, err := repository.ListConversations(ctx, listConversationsCommandUnreadFirst(1, second.NextPageCursor))
	if err != nil {
		t.Fatalf("list third unread-first page: %v", err)
	}
	assertConversationIDs(t, third, "conv-read-new")
	if third.NextPageCursor == "" {
		t.Fatal("expected unread-first cursor after read boundary")
	}
	fourth, err := repository.ListConversations(ctx, listConversationsCommandUnreadFirst(1, third.NextPageCursor))
	if err != nil {
		t.Fatalf("list fourth unread-first page: %v", err)
	}
	assertConversationIDs(t, fourth, "conv-read-old")
	if fourth.NextPageCursor != "" {
		t.Fatalf("expected empty unread-first cursor on last page, got %q", fourth.NextPageCursor)
	}

	_, err = repository.ListConversations(ctx, listConversationsCommand(1, first.NextPageCursor))
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid cursor when unread-first sort changes, got %v", err)
	}
}

func TestRepositoryListConversationsFiltersPinnedAndMutedIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	sortTime := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	insertConversationSummary(t, ctx, pool, "conv-a", 11, sortTime)
	insertConversationSummary(t, ctx, pool, "conv-b", 12, sortTime.Add(-time.Minute))
	insertConversationSummary(t, ctx, pool, "conv-c", 13, sortTime.Add(-2*time.Minute))
	insertConversationSummary(t, ctx, pool, "conv-d", 14, sortTime.Add(-3*time.Minute))

	if _, err := repository.PinConversation(ctx, pinConversationCommand("conv-a", true)); err != nil {
		t.Fatalf("pin conv-a: %v", err)
	}
	if _, err := repository.PinConversation(ctx, pinConversationCommand("conv-c", true)); err != nil {
		t.Fatalf("pin conv-c: %v", err)
	}
	if _, err := repository.MuteConversation(ctx, muteConversationCommand("conv-c", true)); err != nil {
		t.Fatalf("mute conv-c: %v", err)
	}
	if _, err := repository.MuteConversation(ctx, muteConversationCommand("conv-d", true)); err != nil {
		t.Fatalf("mute conv-d: %v", err)
	}

	pinnedFirst, err := repository.ListConversations(ctx, listConversationsCommandPinnedOnly(1, ""))
	if err != nil {
		t.Fatalf("list first pinned page: %v", err)
	}
	assertConversationIDs(t, pinnedFirst, "conv-a")
	if pinnedFirst.NextPageCursor == "" {
		t.Fatal("expected pinned next cursor")
	}
	pinnedSecond, err := repository.ListConversations(ctx, listConversationsCommandPinnedOnly(1, pinnedFirst.NextPageCursor))
	if err != nil {
		t.Fatalf("list second pinned page: %v", err)
	}
	assertConversationIDs(t, pinnedSecond, "conv-c")
	if pinnedSecond.NextPageCursor != "" {
		t.Fatalf("expected empty pinned cursor on last page, got %q", pinnedSecond.NextPageCursor)
	}
	_, err = repository.ListConversations(ctx, listConversationsCommandMutedOnly(1, pinnedFirst.NextPageCursor))
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid cursor when pinned_only changes, got %v", err)
	}

	muted, err := repository.ListConversations(ctx, listConversationsCommandMutedOnly(10, ""))
	if err != nil {
		t.Fatalf("list muted conversations: %v", err)
	}
	assertConversationIDs(t, muted, "conv-c", "conv-d")

	pinnedMuted := listConversationsCommandPinnedOnly(10, "")
	pinnedMuted.MutedOnly = true
	intersection, err := repository.ListConversations(ctx, pinnedMuted)
	if err != nil {
		t.Fatalf("list pinned and muted conversations: %v", err)
	}
	assertConversationIDs(t, intersection, "conv-c")
}

func TestRepositoryListConversationsFiltersTagsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	sortTime := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	insertConversationSummary(t, ctx, pool, "conv-a", 11, sortTime)
	insertConversationSummary(t, ctx, pool, "conv-b", 12, sortTime.Add(-time.Minute))
	insertConversationSummary(t, ctx, pool, "conv-c", 13, sortTime.Add(-2*time.Minute))

	if _, err := repository.SetConversationTags(ctx, setConversationTagsCommand("conv-a", "work", "urgent", "work")); err != nil {
		t.Fatalf("set conv-a tags: %v", err)
	}
	if _, err := repository.SetConversationTags(ctx, setConversationTagsCommand("conv-c", "work")); err != nil {
		t.Fatalf("set conv-c tags: %v", err)
	}

	workFirst, err := repository.ListConversations(ctx, listConversationsCommandWithTag(1, "", "work"))
	if err != nil {
		t.Fatalf("list first tagged page: %v", err)
	}
	assertConversationIDs(t, workFirst, "conv-a")
	assertConversationTags(t, workFirst.Items[0], "work", "urgent")
	if workFirst.NextPageCursor == "" {
		t.Fatal("expected tagged next cursor")
	}
	workSecond, err := repository.ListConversations(ctx, listConversationsCommandWithTag(1, workFirst.NextPageCursor, "work"))
	if err != nil {
		t.Fatalf("list second tagged page: %v", err)
	}
	assertConversationIDs(t, workSecond, "conv-c")
	if workSecond.NextPageCursor != "" {
		t.Fatalf("expected empty tagged cursor on last page, got %q", workSecond.NextPageCursor)
	}

	_, err = repository.ListConversations(ctx, listConversationsCommandWithTag(1, workFirst.NextPageCursor, "urgent"))
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid cursor when tag_filter changes, got %v", err)
	}

	urgent, err := repository.ListConversations(ctx, listConversationsCommandWithTag(10, "", "urgent"))
	if err != nil {
		t.Fatalf("list urgent conversations: %v", err)
	}
	assertConversationIDs(t, urgent, "conv-a")

	cleared, err := repository.SetConversationTags(ctx, setConversationTagsCommand("conv-a"))
	if err != nil {
		t.Fatalf("clear conv-a tags: %v", err)
	}
	if len(cleared.Conversation.Tags) != 0 {
		t.Fatalf("expected cleared tags, got %+v", cleared.Conversation)
	}
	urgent, err = repository.ListConversations(ctx, listConversationsCommandWithTag(10, "", "urgent"))
	if err != nil {
		t.Fatalf("list urgent after clear: %v", err)
	}
	if len(urgent.Items) != 0 {
		t.Fatalf("expected no urgent conversations after clear, got %+v", urgent.Items)
	}
}

func TestRepositorySetConversationDraftIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	sortTime := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	insertConversationSummary(t, ctx, pool, "conv-a", 11, sortTime)
	insertConversationSummary(t, ctx, pool, "conv-b", 12, sortTime.Add(time.Minute))

	set, err := repository.SetConversationDraft(ctx, setConversationDraftCommand("conv-a", "hello draft"))
	if err != nil {
		t.Fatalf("set draft: %v", err)
	}
	if set.Conversation.DraftText != "hello draft" || set.Conversation.DraftUpdatedAt.IsZero() {
		t.Fatalf("unexpected draft after set: %+v", set.Conversation)
	}

	list, err := repository.ListConversations(ctx, listConversationsCommand(10, ""))
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	assertConversationIDs(t, list, "conv-b", "conv-a")
	assertConversationDraft(t, list.Items[1], "hello draft", true)

	cleared, err := repository.SetConversationDraft(ctx, setConversationDraftCommand("conv-a", ""))
	if err != nil {
		t.Fatalf("clear draft: %v", err)
	}
	assertConversationDraft(t, cleared.Conversation, "", false)
}

func TestRepositoryListConversationsRejectsInvalidCursorIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	_, err := repository.ListConversations(ctx, listConversationsCommand(10, "not-a-valid-cursor"))
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}

	mismatchedCursor := encodeTestListCursor(t, map[string]any{
		"v":               1,
		"sort":            "other_sort",
		"sort_updated_at": time.Now().UTC(),
		"conversation_id": "conv-a",
	})
	_, err = repository.ListConversations(ctx, listConversationsCommand(10, mismatchedCursor))
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument for mismatched cursor sort, got %v", err)
	}
}

func TestRepositoryArchiveConversationFiltersListIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	if _, err := repository.ProjectDeliveryEvent(ctx, inboxCreatedCommand(1, "delivery-inbox-1")); err != nil {
		t.Fatalf("project inbox item: %v", err)
	}
	archiveResult, err := repository.ArchiveConversation(ctx, archiveConversationCommand(true))
	if err != nil {
		t.Fatalf("archive conversation: %v", err)
	}
	if !archiveResult.Conversation.Archived {
		t.Fatalf("expected archived conversation in response: %+v", archiveResult)
	}

	defaultList, err := repository.ListConversations(ctx, listConversationsCommand(10, ""))
	if err != nil {
		t.Fatalf("list default conversations: %v", err)
	}
	assertConversationIDs(t, defaultList)

	archivedList, err := repository.ListConversations(ctx, listConversationsCommandIncludingArchived(10, ""))
	if err != nil {
		t.Fatalf("list including archived conversations: %v", err)
	}
	assertConversationSummaryWithArchive(t, archivedList, 1, true)

	if _, err := repository.ProjectDeliveryEvent(ctx, inboxCreatedCommand(2, "delivery-inbox-2")); err != nil {
		t.Fatalf("project inbox while archived: %v", err)
	}
	defaultList, err = repository.ListConversations(ctx, listConversationsCommand(10, ""))
	if err != nil {
		t.Fatalf("list default conversations after new event: %v", err)
	}
	assertConversationIDs(t, defaultList)
	unreadDefault := listConversationsCommandUnreadOnly(10, "")
	unreadDefaultList, err := repository.ListConversations(ctx, unreadDefault)
	if err != nil {
		t.Fatalf("list default unread conversations after new event: %v", err)
	}
	assertConversationIDs(t, unreadDefaultList)

	archivedList, err = repository.ListConversations(ctx, listConversationsCommandIncludingArchived(10, ""))
	if err != nil {
		t.Fatalf("list including archived conversations after new event: %v", err)
	}
	assertConversationSummaryWithArchive(t, archivedList, 2, true)
	unreadArchived := listConversationsCommandIncludingArchived(10, "")
	unreadArchived.UnreadOnly = true
	unreadArchivedList, err := repository.ListConversations(ctx, unreadArchived)
	if err != nil {
		t.Fatalf("list included archived unread conversations after new event: %v", err)
	}
	assertConversationSummaryWithArchive(t, unreadArchivedList, 2, true)

	unarchiveResult, err := repository.ArchiveConversation(ctx, archiveConversationCommand(false))
	if err != nil {
		t.Fatalf("unarchive conversation: %v", err)
	}
	if unarchiveResult.Conversation.Archived {
		t.Fatalf("expected unarchived conversation in response: %+v", unarchiveResult)
	}
	defaultList, err = repository.ListConversations(ctx, listConversationsCommand(10, ""))
	if err != nil {
		t.Fatalf("list default conversations after unarchive: %v", err)
	}
	assertConversationSummaryWithArchive(t, defaultList, 2, false)
}

func TestRepositoryArchiveConversationRejectsUnknownSummaryIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	_, err := repository.ArchiveConversation(ctx, archiveConversationCommand(true))
	if !errors.Is(err, types.ErrConversationNotFound) {
		t.Fatalf("expected conversation not found, got %v", err)
	}
}

func TestRepositoryPinConversationSortsPinnedFirstIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	sortTime := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	insertConversationSummary(t, ctx, pool, "conv-a", 11, sortTime)
	insertConversationSummary(t, ctx, pool, "conv-b", 12, sortTime.Add(-time.Minute))
	insertConversationSummary(t, ctx, pool, "conv-c", 13, sortTime.Add(-2*time.Minute))

	pinResult, err := repository.PinConversation(ctx, pinConversationCommand("conv-c", true))
	if err != nil {
		t.Fatalf("pin conversation: %v", err)
	}
	if !pinResult.Conversation.Pinned {
		t.Fatalf("expected pinned conversation response: %+v", pinResult)
	}

	defaultList, err := repository.ListConversations(ctx, listConversationsCommand(10, ""))
	if err != nil {
		t.Fatalf("list pinned conversations: %v", err)
	}
	assertConversationIDs(t, defaultList, "conv-c", "conv-a", "conv-b")
	if !defaultList.Items[0].Pinned {
		t.Fatalf("expected first item pinned: %+v", defaultList.Items)
	}

	updatedSort := listConversationsCommand(10, "")
	updatedSort.Sort = types.ConversationListSortUpdatedAtDesc
	updatedList, err := repository.ListConversations(ctx, updatedSort)
	if err != nil {
		t.Fatalf("list updated conversations: %v", err)
	}
	assertConversationIDs(t, updatedList, "conv-a", "conv-b", "conv-c")

	first, err := repository.ListConversations(ctx, listConversationsCommand(1, ""))
	if err != nil {
		t.Fatalf("list first pinned page: %v", err)
	}
	assertConversationIDs(t, first, "conv-c")
	second, err := repository.ListConversations(ctx, listConversationsCommand(1, first.NextPageCursor))
	if err != nil {
		t.Fatalf("list second pinned page: %v", err)
	}
	assertConversationIDs(t, second, "conv-a")
	third, err := repository.ListConversations(ctx, listConversationsCommand(1, second.NextPageCursor))
	if err != nil {
		t.Fatalf("list third pinned page: %v", err)
	}
	assertConversationIDs(t, third, "conv-b")

	unpinResult, err := repository.PinConversation(ctx, pinConversationCommand("conv-c", false))
	if err != nil {
		t.Fatalf("unpin conversation: %v", err)
	}
	if unpinResult.Conversation.Pinned {
		t.Fatalf("expected unpinned conversation response: %+v", unpinResult)
	}
}

func TestRepositoryPinConversationRejectsUnknownSummaryIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	_, err := repository.PinConversation(ctx, pinConversationCommand("conv-receipt", true))
	if !errors.Is(err, types.ErrConversationNotFound) {
		t.Fatalf("expected conversation not found, got %v", err)
	}
}

func TestRepositoryMuteConversationUpdatesListPreferenceIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	insertConversationSummary(t, ctx, pool, "conv-receipt", 11, time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC))

	muteResult, err := repository.MuteConversation(ctx, muteConversationCommand("conv-receipt", true))
	if err != nil {
		t.Fatalf("mute conversation: %v", err)
	}
	if !muteResult.Conversation.Muted {
		t.Fatalf("expected muted conversation response: %+v", muteResult)
	}

	list, err := repository.ListConversations(ctx, listConversationsCommand(10, ""))
	if err != nil {
		t.Fatalf("list muted conversations: %v", err)
	}
	assertConversationIDs(t, list, "conv-receipt")
	if !list.Items[0].Muted || list.Items[0].UnreadCount != 1 {
		t.Fatalf("expected muted flag without unread change: %+v", list.Items[0])
	}

	unmuteResult, err := repository.MuteConversation(ctx, muteConversationCommand("conv-receipt", false))
	if err != nil {
		t.Fatalf("unmute conversation: %v", err)
	}
	if unmuteResult.Conversation.Muted {
		t.Fatalf("expected unmuted conversation response: %+v", unmuteResult)
	}
}

func TestRepositoryMuteConversationRejectsUnknownSummaryIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	_, err := repository.MuteConversation(ctx, muteConversationCommand("conv-receipt", true))
	if !errors.Is(err, types.ErrConversationNotFound) {
		t.Fatalf("expected conversation not found, got %v", err)
	}
}

func inboxCreatedCommand(seq int64, eventID string) types.ProjectDeliveryEventCommand {
	return types.ProjectDeliveryEventCommand{
		TenantID:        "tenant-receipt",
		EventID:         eventID,
		EventType:       types.DeliveryEventInboxItemCreated,
		UserID:          "receiver-1",
		ConversationID:  "conv-receipt",
		ConversationSeq: seq,
		SourceEventID:   fmt.Sprintf("timeline-event-%d", seq),
		SourceEventType: types.SourceEventMessagePersisted,
		MessageID:       fmt.Sprintf("message-%d", seq),
		SenderID:        "sender-1",
		ConsumerGroup:   "receipt-test",
		Topic:           "im.delivery.events",
		PartitionID:     0,
		OffsetValue:     seq,
	}
}

func ackRecordedCommand(receivedSeq int64, eventID string) types.ProjectDeliveryEventCommand {
	return ackRecordedCommandForDevice(receivedSeq, eventID, "device-1")
}

func ackRecordedCommandForDevice(receivedSeq int64, eventID string, deviceID string) types.ProjectDeliveryEventCommand {
	return types.ProjectDeliveryEventCommand{
		TenantID:        "tenant-receipt",
		EventID:         eventID,
		EventType:       types.DeliveryEventAckRecorded,
		UserID:          "receiver-1",
		DeviceID:        deviceID,
		ConversationID:  "conv-receipt",
		LastReceivedSeq: receivedSeq,
		ConsumerGroup:   "receipt-test",
		Topic:           "im.delivery.events",
		PartitionID:     0,
		OffsetValue:     receivedSeq + 10,
	}
}

func insertConversationSummary(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	conversationID string,
	lastVisibleSeq int64,
	sortUpdatedAt time.Time,
) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO user_conversation_summaries (
    tenant_id,
    user_id,
    conversation_id,
    last_visible_seq,
    last_message_id,
    last_sender_id,
    last_source_event_type,
    last_read_seq,
    unread_count,
    sort_updated_at,
    updated_at
) VALUES (
    'tenant-receipt',
    'receiver-1',
    $1,
    $2,
    $3,
    'sender-1',
    'message.persisted.v1',
    0,
    1,
    $4,
    $4
)
`, conversationID, lastVisibleSeq, fmt.Sprintf("message-%d", lastVisibleSeq), sortUpdatedAt)
	if err != nil {
		t.Fatalf("insert conversation summary %s: %v", conversationID, err)
	}
}

func encodeTestListCursor(t *testing.T, value map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode test cursor: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func markReadCommand(seq int64) types.MarkReadCommand {
	return types.MarkReadCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-receipt",
			UserID:   "receiver-1",
			DeviceID: "device-1",
		},
		ConversationID: "conv-receipt",
		ReadSeq:        seq,
	}
}

func listConversationsCommand(limit int, cursor string) types.ListConversationsCommand {
	return types.ListConversationsCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-receipt",
			UserID:   "receiver-1",
			DeviceID: "device-1",
		},
		Limit:      limit,
		PageCursor: cursor,
	}
}

func listConversationsCommandIncludingArchived(limit int, cursor string) types.ListConversationsCommand {
	command := listConversationsCommand(limit, cursor)
	command.IncludeArchived = true
	return command
}

func listConversationsCommandUnreadOnly(limit int, cursor string) types.ListConversationsCommand {
	command := listConversationsCommand(limit, cursor)
	command.UnreadOnly = true
	return command
}

func listConversationsCommandUnreadFirst(limit int, cursor string) types.ListConversationsCommand {
	command := listConversationsCommand(limit, cursor)
	command.Sort = types.ConversationListSortUnreadUpdatedAtDesc
	return command
}

func listConversationsCommandPinnedOnly(limit int, cursor string) types.ListConversationsCommand {
	command := listConversationsCommand(limit, cursor)
	command.PinnedOnly = true
	return command
}

func listConversationsCommandMutedOnly(limit int, cursor string) types.ListConversationsCommand {
	command := listConversationsCommand(limit, cursor)
	command.MutedOnly = true
	return command
}

func listConversationsCommandWithTag(limit int, cursor string, tag string) types.ListConversationsCommand {
	command := listConversationsCommand(limit, cursor)
	command.TagFilter = tag
	return command
}

func setConversationUnread(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	conversationID string,
	unreadCount int64,
) {
	t.Helper()
	_, err := pool.Exec(ctx, `
UPDATE user_conversation_summaries
SET unread_count = $1
WHERE tenant_id = 'tenant-receipt'
  AND user_id = 'receiver-1'
  AND conversation_id = $2
`, unreadCount, conversationID)
	if err != nil {
		t.Fatalf("set conversation unread %s: %v", conversationID, err)
	}
}

func archiveConversationCommand(archived bool) types.ArchiveConversationCommand {
	return types.ArchiveConversationCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-receipt",
			UserID:   "receiver-1",
			DeviceID: "device-1",
		},
		ConversationID: "conv-receipt",
		Archived:       archived,
	}
}

func pinConversationCommand(conversationID string, pinned bool) types.PinConversationCommand {
	return types.PinConversationCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-receipt",
			UserID:   "receiver-1",
			DeviceID: "device-1",
		},
		ConversationID: types.ConversationID(conversationID),
		Pinned:         pinned,
	}
}

func muteConversationCommand(conversationID string, muted bool) types.MuteConversationCommand {
	return types.MuteConversationCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-receipt",
			UserID:   "receiver-1",
			DeviceID: "device-1",
		},
		ConversationID: types.ConversationID(conversationID),
		Muted:          muted,
	}
}

func setConversationTagsCommand(conversationID string, tags ...string) types.SetConversationTagsCommand {
	return types.SetConversationTagsCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-receipt",
			UserID:   "receiver-1",
			DeviceID: "device-1",
		},
		ConversationID: types.ConversationID(conversationID),
		Tags:           tags,
	}
}

func setConversationDraftCommand(conversationID string, draftText string) types.SetConversationDraftCommand {
	return types.SetConversationDraftCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-receipt",
			UserID:   "receiver-1",
			DeviceID: "device-1",
		},
		ConversationID: types.ConversationID(conversationID),
		DraftText:      draftText,
	}
}

func getStateCommandBySeq(seq int64) types.GetReceiptStateCommand {
	return types.GetReceiptStateCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-receipt",
			UserID:   "sender-1",
			DeviceID: "device-sender",
		},
		ConversationID:  "conv-receipt",
		ConversationSeq: seq,
	}
}

func getStateCommandByMessage(messageID string) types.GetReceiptStateCommand {
	command := getStateCommandBySeq(0)
	command.MessageID = messageID
	return command
}

func assertConversationIDs(t *testing.T, result types.ListConversationsResult, want ...types.ConversationID) {
	t.Helper()
	if len(result.Items) != len(want) {
		t.Fatalf("expected %d conversation summaries, got %d: %+v", len(want), len(result.Items), result.Items)
	}
	for index, conversationID := range want {
		if result.Items[index].ConversationID != conversationID {
			t.Fatalf("expected item %d conversation_id=%s, got %s", index, conversationID, result.Items[index].ConversationID)
		}
	}
}

func assertConversationSummary(
	t *testing.T,
	result types.ListConversationsResult,
	wantLastVisibleSeq int64,
	wantUnread int64,
	wantLastReadSeq int64,
) {
	t.Helper()
	assertConversationSummaryWithMessage(
		t,
		result,
		wantLastVisibleSeq,
		fmt.Sprintf("message-%d", wantLastVisibleSeq),
		types.SourceEventMessagePersisted,
		wantUnread,
		wantLastReadSeq,
	)
}

func assertConversationSummaryWithMessage(
	t *testing.T,
	result types.ListConversationsResult,
	wantLastVisibleSeq int64,
	wantLastMessageID string,
	wantLastSourceEventType string,
	wantUnread int64,
	wantLastReadSeq int64,
) {
	t.Helper()
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 conversation summary, got %d: %+v", len(result.Items), result.Items)
	}
	item := result.Items[0]
	if item.ConversationID != "conv-receipt" ||
		item.LastVisibleSeq != wantLastVisibleSeq ||
		item.UnreadCount != wantUnread ||
		item.LastReadSeq != wantLastReadSeq {
		t.Fatalf("unexpected conversation summary: %+v", item)
	}
	if item.LastMessageID != wantLastMessageID {
		t.Fatalf("unexpected last_message_id: %+v", item)
	}
	if item.LastSourceEventType != wantLastSourceEventType {
		t.Fatalf("unexpected last_source_event_type: %+v", item)
	}
}

func assertConversationSummaryWithArchive(
	t *testing.T,
	result types.ListConversationsResult,
	wantLastVisibleSeq int64,
	wantArchived bool,
) {
	t.Helper()
	assertConversationSummary(t, result, wantLastVisibleSeq, wantLastVisibleSeq, 0)
	if result.Items[0].Archived != wantArchived {
		t.Fatalf("expected archived=%v, got %+v", wantArchived, result.Items[0])
	}
}

func assertConversationTags(t *testing.T, item types.ConversationSummary, want ...string) {
	t.Helper()
	if len(item.Tags) != len(want) {
		t.Fatalf("expected %d tags, got %d: %+v", len(want), len(item.Tags), item)
	}
	for index, tag := range want {
		if item.Tags[index] != tag {
			t.Fatalf("expected tag %d=%s, got %+v", index, tag, item.Tags)
		}
	}
}

func assertConversationDraft(t *testing.T, item types.ConversationSummary, wantText string, wantUpdated bool) {
	t.Helper()
	if item.DraftText != wantText {
		t.Fatalf("expected draft_text=%q, got %+v", wantText, item)
	}
	if item.DraftUpdatedAt.IsZero() == wantUpdated {
		t.Fatalf("expected draft_updated_at set=%v, got %+v", wantUpdated, item)
	}
}

func assertReceiptOutboxCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventType string, want int) {
	t.Helper()
	var got int
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM receipt_outbox
WHERE tenant_id = 'tenant-receipt'
  AND event_type = $1
`, eventType).Scan(&got)
	if err != nil {
		t.Fatalf("count receipt outbox: %v", err)
	}
	if got != want {
		t.Fatalf("expected %d outbox rows for %s, got %d", want, eventType, got)
	}
}

func assertReceiptStateCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int) {
	t.Helper()
	var got int
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM message_receipt_states
WHERE tenant_id = 'tenant-receipt'
`).Scan(&got)
	if err != nil {
		t.Fatalf("count receipt states: %v", err)
	}
	if got != want {
		t.Fatalf("expected %d receipt states, got %d", want, got)
	}
}

func assertReceivedDeviceIDs(t *testing.T, receiver types.ReceiptUserState, want ...string) {
	t.Helper()
	if len(receiver.ReceivedDevices) != len(want) {
		t.Fatalf("expected %d received device details, got %d: %+v", len(want), len(receiver.ReceivedDevices), receiver)
	}
	got := make(map[string]bool, len(receiver.ReceivedDevices))
	for _, device := range receiver.ReceivedDevices {
		if device.DeviceID == "" || device.LastReceivedSeq == 0 || device.UpdatedAt.IsZero() {
			t.Fatalf("unexpected empty received device detail: %+v", device)
		}
		got[device.DeviceID] = true
	}
	for _, deviceID := range want {
		if !got[deviceID] {
			t.Fatalf("missing received device %s in %+v", deviceID, receiver.ReceivedDevices)
		}
	}
}

func assertReceiptOutboxPayload(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	eventType string,
	wantMessageID string,
	wantSourceEventID string,
	wantDeviceID string,
	wantCursorSeq int64,
) {
	t.Helper()
	var raw []byte
	err := pool.QueryRow(ctx, `
SELECT payload_json
FROM receipt_outbox
WHERE tenant_id = 'tenant-receipt'
  AND event_type = $1
ORDER BY id DESC
LIMIT 1
`, eventType).Scan(&raw)
	if err != nil {
		t.Fatalf("query receipt outbox payload: %v", err)
	}
	var payload struct {
		MessageID     string `json:"message_id"`
		SourceEventID string `json:"source_event_id"`
		DeviceID      string `json:"device_id"`
		CursorSeq     int64  `json:"cursor_seq"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode receipt outbox payload: %v", err)
	}
	if payload.MessageID != wantMessageID ||
		payload.SourceEventID != wantSourceEventID ||
		payload.DeviceID != wantDeviceID ||
		payload.CursorSeq != wantCursorSeq {
		t.Fatalf("unexpected payload: %+v", payload)
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
	applyReceiptMigration(t, ctx, pool)
	return pool
}

func applyReceiptMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	root := findRepoRoot(t)
	for _, name := range []string{
		"000001_receipt_core.sql",
		"000002_conversation_summary.sql",
		"000003_receipt_source_event_type.sql",
		"000004_conversation_summary_source_event_type.sql",
		"000005_conversation_archive.sql",
		"000006_conversation_pin.sql",
		"000007_conversation_mute.sql",
		"000008_conversation_unread_filter.sql",
		"000009_receipt_outbox_repair_audit.sql",
		"000010_device_received_cursor_lookup.sql",
		"000011_conversation_tags.sql",
		"000012_conversation_draft.sql",
	} {
		migrationPath := filepath.Join(root, "migrations", "postgres", "receipt", name)
		sqlBytes, err := os.ReadFile(migrationPath)
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
}

func resetReceiptTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
TRUNCATE
    conversation_summary_checkpoints,
    user_conversation_summaries,
    receipt_outbox_repair_audit,
    receipt_outbox,
    receipt_kafka_checkpoints,
    message_receipt_states,
    user_read_cursors,
    user_received_cursors,
    device_received_cursors,
    receipt_inbox_projection
RESTART IDENTITY
`)
	if err != nil {
		t.Fatalf("reset receipt tables: %v", err)
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
