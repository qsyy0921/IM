package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

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

func TestRepositoryListConversationsExcludesMutedIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	sortTime := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	insertConversationSummary(t, ctx, pool, "conv-a", 11, sortTime)
	insertConversationSummary(t, ctx, pool, "conv-b", 12, sortTime.Add(-time.Minute))
	insertConversationSummary(t, ctx, pool, "conv-c", 13, sortTime.Add(-2*time.Minute))
	if _, err := repository.MuteConversation(ctx, muteConversationCommand("conv-b", true)); err != nil {
		t.Fatalf("mute conv-b: %v", err)
	}

	defaultList, err := repository.ListConversations(ctx, listConversationsCommand(10, ""))
	if err != nil {
		t.Fatalf("list default conversations: %v", err)
	}
	assertConversationIDs(t, defaultList, "conv-a", "conv-b", "conv-c")

	first, err := repository.ListConversations(ctx, listConversationsCommandExcludeMuted(1, ""))
	if err != nil {
		t.Fatalf("list first exclude-muted page: %v", err)
	}
	assertConversationIDs(t, first, "conv-a")
	if first.NextPageCursor == "" {
		t.Fatal("expected exclude-muted next cursor")
	}
	second, err := repository.ListConversations(ctx, listConversationsCommandExcludeMuted(1, first.NextPageCursor))
	if err != nil {
		t.Fatalf("list second exclude-muted page: %v", err)
	}
	assertConversationIDs(t, second, "conv-c")
	if second.NextPageCursor != "" {
		t.Fatalf("expected empty exclude-muted cursor on last page, got %q", second.NextPageCursor)
	}

	_, err = repository.ListConversations(ctx, listConversationsCommand(1, first.NextPageCursor))
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid cursor when exclude_muted changes, got %v", err)
	}

	conflicting := listConversationsCommandExcludeMuted(10, "")
	conflicting.MutedOnly = true
	_, err = repository.ListConversations(ctx, conflicting)
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument for muted_only + exclude_muted, got %v", err)
	}
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

func TestRepositoryListConversationsFiltersMultipleTagsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	sortTime := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	insertConversationSummary(t, ctx, pool, "conv-a", 11, sortTime)
	insertConversationSummary(t, ctx, pool, "conv-b", 12, sortTime.Add(-time.Minute))
	insertConversationSummary(t, ctx, pool, "conv-c", 13, sortTime.Add(-2*time.Minute))
	insertConversationSummary(t, ctx, pool, "conv-d", 14, sortTime.Add(-3*time.Minute))

	if _, err := repository.SetConversationTags(ctx, setConversationTagsCommand("conv-a", "work", "urgent", "vip")); err != nil {
		t.Fatalf("set conv-a tags: %v", err)
	}
	if _, err := repository.SetConversationTags(ctx, setConversationTagsCommand("conv-b", "work", "vip")); err != nil {
		t.Fatalf("set conv-b tags: %v", err)
	}
	if _, err := repository.SetConversationTags(ctx, setConversationTagsCommand("conv-c", "urgent", "vip")); err != nil {
		t.Fatalf("set conv-c tags: %v", err)
	}
	if _, err := repository.SetConversationTags(ctx, setConversationTagsCommand("conv-d", "work", "urgent")); err != nil {
		t.Fatalf("set conv-d tags: %v", err)
	}

	first, err := repository.ListConversations(ctx, listConversationsCommandWithTags(1, "", "urgent", "work"))
	if err != nil {
		t.Fatalf("list first multi-tag page: %v", err)
	}
	assertConversationIDs(t, first, "conv-a")
	if first.NextPageCursor == "" {
		t.Fatal("expected multi-tag next cursor")
	}
	second, err := repository.ListConversations(ctx, listConversationsCommandWithTags(1, first.NextPageCursor, "work", "urgent"))
	if err != nil {
		t.Fatalf("list second multi-tag page with reversed filters: %v", err)
	}
	assertConversationIDs(t, second, "conv-d")
	if second.NextPageCursor != "" {
		t.Fatalf("expected empty multi-tag cursor on last page, got %q", second.NextPageCursor)
	}

	_, err = repository.ListConversations(ctx, listConversationsCommandWithTags(1, first.NextPageCursor, "work"))
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid cursor when tag_filters changes, got %v", err)
	}

	withLegacyFilter := listConversationsCommandWithTags(10, "", "work", "urgent")
	withLegacyFilter.TagFilter = "vip"
	filtered, err := repository.ListConversations(ctx, withLegacyFilter)
	if err != nil {
		t.Fatalf("list combined legacy and multi-tag filters: %v", err)
	}
	assertConversationIDs(t, filtered, "conv-a")
}

func TestRepositoryListConversationsFiltersLastSourceEventTypeIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	sortTime := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	insertConversationSummary(t, ctx, pool, "conv-a", 11, sortTime)
	insertConversationSummary(t, ctx, pool, "conv-b", 12, sortTime.Add(-time.Minute))
	insertConversationSummary(t, ctx, pool, "conv-c", 13, sortTime.Add(-2*time.Minute))
	setConversationSummaryLastSourceEventType(t, ctx, pool, "conv-a", types.SourceEventMessageDeleted)
	setConversationSummaryLastSourceEventType(t, ctx, pool, "conv-c", types.SourceEventMessageDeleted)

	first, err := repository.ListConversations(ctx, listConversationsCommandWithLastSourceEventType(1, "", types.SourceEventMessageDeleted))
	if err != nil {
		t.Fatalf("list first source event page: %v", err)
	}
	assertConversationIDs(t, first, "conv-a")
	if first.NextPageCursor == "" {
		t.Fatal("expected source event next cursor")
	}
	second, err := repository.ListConversations(ctx, listConversationsCommandWithLastSourceEventType(1, first.NextPageCursor, types.SourceEventMessageDeleted))
	if err != nil {
		t.Fatalf("list second source event page: %v", err)
	}
	assertConversationIDs(t, second, "conv-c")
	if second.NextPageCursor != "" {
		t.Fatalf("expected empty source event cursor on last page, got %q", second.NextPageCursor)
	}

	_, err = repository.ListConversations(ctx, listConversationsCommandWithLastSourceEventType(1, first.NextPageCursor, types.SourceEventMessagePersisted))
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid cursor when last source event filter changes, got %v", err)
	}

	persisted, err := repository.ListConversations(ctx, listConversationsCommandWithLastSourceEventType(10, "", types.SourceEventMessagePersisted))
	if err != nil {
		t.Fatalf("list persisted source event conversations: %v", err)
	}
	assertConversationIDs(t, persisted, "conv-b")
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

func TestRepositoryListConversationsFiltersDraftOnlyIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	sortTime := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	insertConversationSummary(t, ctx, pool, "conv-a", 11, sortTime)
	insertConversationSummary(t, ctx, pool, "conv-b", 12, sortTime.Add(-time.Minute))
	insertConversationSummary(t, ctx, pool, "conv-c", 13, sortTime.Add(-2*time.Minute))

	if _, err := repository.SetConversationDraft(ctx, setConversationDraftCommand("conv-a", "draft-a")); err != nil {
		t.Fatalf("set conv-a draft: %v", err)
	}
	if _, err := repository.SetConversationDraft(ctx, setConversationDraftCommand("conv-c", "draft-c")); err != nil {
		t.Fatalf("set conv-c draft: %v", err)
	}

	first, err := repository.ListConversations(ctx, listConversationsCommandDraftOnly(1, ""))
	if err != nil {
		t.Fatalf("list first draft page: %v", err)
	}
	assertConversationIDs(t, first, "conv-a")
	assertConversationDraft(t, first.Items[0], "draft-a", true)
	if first.NextPageCursor == "" {
		t.Fatal("expected draft-only next cursor")
	}

	second, err := repository.ListConversations(ctx, listConversationsCommandDraftOnly(1, first.NextPageCursor))
	if err != nil {
		t.Fatalf("list second draft page: %v", err)
	}
	assertConversationIDs(t, second, "conv-c")
	assertConversationDraft(t, second.Items[0], "draft-c", true)
	if second.NextPageCursor != "" {
		t.Fatalf("expected empty draft-only cursor on last page, got %q", second.NextPageCursor)
	}

	_, err = repository.ListConversations(ctx, listConversationsCommand(1, first.NextPageCursor))
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid cursor when draft_only changes, got %v", err)
	}

	if _, err := repository.SetConversationDraft(ctx, setConversationDraftCommand("conv-a", "")); err != nil {
		t.Fatalf("clear conv-a draft: %v", err)
	}
	filtered, err := repository.ListConversations(ctx, listConversationsCommandDraftOnly(10, ""))
	if err != nil {
		t.Fatalf("list draft-only after clear: %v", err)
	}
	assertConversationIDs(t, filtered, "conv-c")
}

func TestRepositoryListConversationsSortsDraftFirstIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	sortTime := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	draftTime := time.Date(2026, 6, 10, 13, 0, 0, 0, time.UTC)
	insertConversationSummary(t, ctx, pool, "conv-no-draft-new", 11, sortTime.Add(time.Minute))
	insertConversationSummary(t, ctx, pool, "conv-draft-new", 12, sortTime)
	insertConversationSummary(t, ctx, pool, "conv-draft-old", 13, sortTime.Add(-time.Minute))
	insertConversationSummary(t, ctx, pool, "conv-no-draft-old", 14, sortTime.Add(-2*time.Minute))
	setConversationDraftAt(t, ctx, pool, "conv-draft-new", "draft-new", draftTime)
	setConversationDraftAt(t, ctx, pool, "conv-draft-old", "draft-old", draftTime.Add(-time.Minute))

	first, err := repository.ListConversations(ctx, listConversationsCommandDraftFirst(2, ""))
	if err != nil {
		t.Fatalf("list first draft-first page: %v", err)
	}
	assertConversationIDs(t, first, "conv-draft-new", "conv-draft-old")
	if first.NextPageCursor == "" {
		t.Fatal("expected draft-first cursor after first page")
	}

	second, err := repository.ListConversations(ctx, listConversationsCommandDraftFirst(2, first.NextPageCursor))
	if err != nil {
		t.Fatalf("list second draft-first page: %v", err)
	}
	assertConversationIDs(t, second, "conv-no-draft-new", "conv-no-draft-old")
	if second.NextPageCursor != "" {
		t.Fatalf("expected empty draft-first cursor on last page, got %q", second.NextPageCursor)
	}

	_, err = repository.ListConversations(ctx, listConversationsCommand(2, first.NextPageCursor))
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid cursor when draft-first sort changes, got %v", err)
	}
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

func TestRepositoryListConversationsFiltersArchivedOnlyIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	repository := NewRepository(pool)

	sortTime := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	insertConversationSummary(t, ctx, pool, "conv-a", 11, sortTime)
	insertConversationSummary(t, ctx, pool, "conv-b", 12, sortTime.Add(-time.Minute))
	insertConversationSummary(t, ctx, pool, "conv-c", 13, sortTime.Add(-2*time.Minute))
	if _, err := repository.ArchiveConversation(ctx, archiveConversationCommandForConversation("conv-a", true)); err != nil {
		t.Fatalf("archive conv-a: %v", err)
	}
	if _, err := repository.ArchiveConversation(ctx, archiveConversationCommandForConversation("conv-c", true)); err != nil {
		t.Fatalf("archive conv-c: %v", err)
	}

	first, err := repository.ListConversations(ctx, listConversationsCommandArchivedOnly(1, ""))
	if err != nil {
		t.Fatalf("list first archived-only page: %v", err)
	}
	assertConversationIDs(t, first, "conv-a")
	if !first.Items[0].Archived {
		t.Fatalf("expected first archived conversation, got %+v", first.Items[0])
	}
	if first.NextPageCursor == "" {
		t.Fatal("expected archived-only cursor")
	}

	second, err := repository.ListConversations(ctx, listConversationsCommandArchivedOnly(1, first.NextPageCursor))
	if err != nil {
		t.Fatalf("list second archived-only page: %v", err)
	}
	assertConversationIDs(t, second, "conv-c")
	if !second.Items[0].Archived {
		t.Fatalf("expected second archived conversation, got %+v", second.Items[0])
	}
	if second.NextPageCursor != "" {
		t.Fatalf("expected empty archived-only cursor on last page, got %q", second.NextPageCursor)
	}

	_, err = repository.ListConversations(ctx, listConversationsCommandIncludingArchived(1, first.NextPageCursor))
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid cursor when archived_only changes, got %v", err)
	}

	archivedIncluded := listConversationsCommandArchivedOnly(10, "")
	archivedIncluded.IncludeArchived = true
	included, err := repository.ListConversations(ctx, archivedIncluded)
	if err != nil {
		t.Fatalf("list archived-only with include_archived: %v", err)
	}
	assertConversationIDs(t, included, "conv-a", "conv-c")
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
