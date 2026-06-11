package grpc

import (
	"context"
	"testing"
	"time"

	receiptv1 "github.com/qsyy0921/IM/api/proto/nexusim/receipt/v1"
	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMarkReadMapsValidationError(t *testing.T) {
	server := NewServer(fakeMarkRead{err: types.NewInvalidArgument("tenant_id is required")}, fakeGetReceiptState{}, fakeListReceiptStates{}, fakeListConversations{}, fakeArchiveConversation{}, fakePinConversation{}, fakeMuteConversation{})
	_, err := server.MarkRead(context.Background(), &receiptv1.MarkReadRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
	}
}

func TestMarkReadSanitizesDBWriteError(t *testing.T) {
	server := NewServer(fakeMarkRead{err: types.NewDBWriteFailed("duplicate key value violates unique constraint receipt_outbox_event_id_key")}, fakeGetReceiptState{}, fakeListReceiptStates{}, fakeListConversations{}, fakeArchiveConversation{}, fakePinConversation{}, fakeMuteConversation{})
	_, err := server.MarkRead(context.Background(), &receiptv1.MarkReadRequest{
		AuthContext:    &receiptv1.AuthContext{TenantId: "tenant-1", UserId: "user-1", DeviceId: "device-1"},
		ConversationId: "conversation-1",
		ReadSeq:        1,
	})
	statusErr, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected grpc status, got %v", err)
	}
	if statusErr.Code() != codes.Unavailable {
		t.Fatalf("expected Unavailable, got %v", statusErr.Code())
	}
	if statusErr.Message() != "receipt write failed" {
		t.Fatalf("expected sanitized message, got %q", statusErr.Message())
	}
}

func TestListConversationsMapsValidationError(t *testing.T) {
	server := NewServer(fakeMarkRead{}, fakeGetReceiptState{}, fakeListReceiptStates{}, fakeListConversations{err: types.NewInvalidArgument("tenant_id is required")}, fakeArchiveConversation{}, fakePinConversation{}, fakeMuteConversation{})
	_, err := server.ListConversations(context.Background(), &receiptv1.ListConversationsRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
	}
}

func TestListConversationsMapsSort(t *testing.T) {
	list := &fakeListConversationsCapture{}
	server := NewServer(fakeMarkRead{}, fakeGetReceiptState{}, fakeListReceiptStates{}, list, fakeArchiveConversation{}, fakePinConversation{}, fakeMuteConversation{})
	_, err := server.ListConversations(context.Background(), &receiptv1.ListConversationsRequest{
		AuthContext: &receiptv1.AuthContext{TenantId: "tenant-1", UserId: "user-1", DeviceId: "device-1"},
		Limit:       20,
		PageCursor:  "cursor-1",
		Sort:        receiptv1.ConversationListSort_CONVERSATION_LIST_SORT_UPDATED_AT_DESC,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if list.command.Sort != types.ConversationListSortUpdatedAtDesc ||
		list.command.Limit != 20 ||
		list.command.PageCursor != "cursor-1" {
		t.Fatalf("unexpected list command: %+v", list.command)
	}
}

func TestListConversationsMapsResponse(t *testing.T) {
	updatedAt := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	list := &fakeListConversationsCapture{
		result: types.ListConversationsResult{
			Items: []types.ConversationSummary{{
				ConversationID:      "conversation-1",
				LastVisibleSeq:      12,
				LastMessageID:       "message-12",
				LastSenderID:        "sender-1",
				LastSourceEventType: types.SourceEventMessageEdited,
				UnreadCount:         0,
				LastReadSeq:         12,
				UpdatedAt:           updatedAt,
			}},
			NextPageCursor: "next-cursor",
			ProjectionWatermark: types.ProjectionWatermark{
				Source:      "im.delivery.events",
				OffsetValue: 22,
				UpdatedAt:   updatedAt,
			},
		},
	}
	server := NewServer(fakeMarkRead{}, fakeGetReceiptState{}, fakeListReceiptStates{}, list, fakeArchiveConversation{}, fakePinConversation{}, fakeMuteConversation{})
	response, err := server.ListConversations(context.Background(), &receiptv1.ListConversationsRequest{
		AuthContext: &receiptv1.AuthContext{TenantId: "tenant-1", UserId: "user-1", DeviceId: "device-1"},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if response.GetNextPageCursor() != "next-cursor" ||
		response.GetProjectionWatermark().GetOffsetValue() != 22 ||
		len(response.GetItems()) != 1 {
		t.Fatalf("unexpected response: %+v", response)
	}
	item := response.GetItems()[0]
	if item.GetConversationId() != "conversation-1" ||
		item.GetLastSourceEventType() != types.SourceEventMessageEdited ||
		item.GetUpdatedAtUnixMs() != updatedAt.UnixMilli() ||
		item.GetArchived() {
		t.Fatalf("unexpected item: %+v", item)
	}
}

func TestListReceiptStatesMapsRequestAndResponse(t *testing.T) {
	list := &fakeListReceiptStatesCapture{
		result: types.ListReceiptStatesResult{
			Items: []types.GetReceiptStateResult{{
				ConversationID:    "conversation-1",
				ConversationSeq:   11,
				MessageID:         "message-11",
				ReceivedUserCount: 2,
				ReadUserCount:     1,
				VisibilityMode:    types.ReceiptVisibilityDetailed,
				Receivers: []types.ReceiptUserState{{
					UserID:      "user-2",
					ReceivedSeq: 11,
					ReadSeq:     11,
				}},
			}},
		},
	}
	server := NewServer(fakeMarkRead{}, fakeGetReceiptState{}, list, fakeListConversations{}, fakeArchiveConversation{}, fakePinConversation{}, fakeMuteConversation{})
	response, err := server.ListReceiptStates(context.Background(), &receiptv1.ListReceiptStatesRequest{
		AuthContext:    &receiptv1.AuthContext{TenantId: "tenant-1", UserId: "user-1", DeviceId: "device-1"},
		ConversationId: "conversation-1",
		Items: []*receiptv1.ReceiptStateQuery{
			{MessageId: "message-11"},
			{ConversationSeq: 12},
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if list.command.ConversationID != "conversation-1" ||
		len(list.command.Items) != 2 ||
		list.command.Items[0].MessageID != "message-11" ||
		list.command.Items[1].ConversationSeq != 12 {
		t.Fatalf("unexpected command: %+v", list.command)
	}
	if len(response.GetItems()) != 1 ||
		response.GetItems()[0].GetConversationSeq() != 11 ||
		response.GetItems()[0].GetReadUserCount() != 1 {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestArchiveConversationMapsRequestAndResponse(t *testing.T) {
	archive := &fakeArchiveConversationCapture{
		result: types.ArchiveConversationResult{
			Conversation: types.ConversationSummary{
				ConversationID: "conversation-1",
				LastVisibleSeq: 7,
				LastMessageID:  "message-7",
				Archived:       true,
			},
		},
	}
	server := NewServer(fakeMarkRead{}, fakeGetReceiptState{}, fakeListReceiptStates{}, fakeListConversations{}, archive, fakePinConversation{}, fakeMuteConversation{})
	response, err := server.ArchiveConversation(context.Background(), &receiptv1.ArchiveConversationRequest{
		AuthContext:    &receiptv1.AuthContext{TenantId: "tenant-1", UserId: "user-1", DeviceId: "device-1"},
		ConversationId: "conversation-1",
		Archived:       true,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if archive.command.ConversationID != "conversation-1" || !archive.command.Archived {
		t.Fatalf("unexpected command: %+v", archive.command)
	}
	if response.GetConversation().GetConversationId() != "conversation-1" ||
		!response.GetConversation().GetArchived() {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestPinConversationMapsRequestAndResponse(t *testing.T) {
	pin := &fakePinConversationCapture{
		result: types.PinConversationResult{
			Conversation: types.ConversationSummary{
				ConversationID: "conversation-1",
				LastVisibleSeq: 7,
				LastMessageID:  "message-7",
				Pinned:         true,
			},
		},
	}
	server := NewServer(fakeMarkRead{}, fakeGetReceiptState{}, fakeListReceiptStates{}, fakeListConversations{}, fakeArchiveConversation{}, pin, fakeMuteConversation{})
	response, err := server.PinConversation(context.Background(), &receiptv1.PinConversationRequest{
		AuthContext:    &receiptv1.AuthContext{TenantId: "tenant-1", UserId: "user-1", DeviceId: "device-1"},
		ConversationId: "conversation-1",
		Pinned:         true,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if pin.command.ConversationID != "conversation-1" || !pin.command.Pinned {
		t.Fatalf("unexpected command: %+v", pin.command)
	}
	if response.GetConversation().GetConversationId() != "conversation-1" ||
		!response.GetConversation().GetPinned() {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestMuteConversationMapsRequestAndResponse(t *testing.T) {
	mute := &fakeMuteConversationCapture{
		result: types.MuteConversationResult{
			Conversation: types.ConversationSummary{
				ConversationID: "conversation-1",
				LastVisibleSeq: 7,
				LastMessageID:  "message-7",
				Muted:          true,
			},
		},
	}
	server := NewServer(fakeMarkRead{}, fakeGetReceiptState{}, fakeListReceiptStates{}, fakeListConversations{}, fakeArchiveConversation{}, fakePinConversation{}, mute)
	response, err := server.MuteConversation(context.Background(), &receiptv1.MuteConversationRequest{
		AuthContext:    &receiptv1.AuthContext{TenantId: "tenant-1", UserId: "user-1", DeviceId: "device-1"},
		ConversationId: "conversation-1",
		Muted:          true,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if mute.command.ConversationID != "conversation-1" || !mute.command.Muted {
		t.Fatalf("unexpected mute command: %+v", mute.command)
	}
	if response.GetConversation().GetConversationId() != "conversation-1" ||
		!response.GetConversation().GetMuted() {
		t.Fatalf("unexpected response: %+v", response)
	}
}

type fakeMarkRead struct {
	err error
}

func (fake fakeMarkRead) Execute(context.Context, types.MarkReadCommand) (types.MarkReadResult, error) {
	if fake.err != nil {
		return types.MarkReadResult{}, fake.err
	}
	return types.MarkReadResult{}, nil
}

type fakeGetReceiptState struct{}

func (fakeGetReceiptState) Execute(context.Context, types.GetReceiptStateCommand) (types.GetReceiptStateResult, error) {
	return types.GetReceiptStateResult{}, nil
}

type fakeListReceiptStates struct{}

func (fakeListReceiptStates) Execute(context.Context, types.ListReceiptStatesCommand) (types.ListReceiptStatesResult, error) {
	return types.ListReceiptStatesResult{}, nil
}

type fakeListReceiptStatesCapture struct {
	command types.ListReceiptStatesCommand
	result  types.ListReceiptStatesResult
}

func (fake *fakeListReceiptStatesCapture) Execute(_ context.Context, command types.ListReceiptStatesCommand) (types.ListReceiptStatesResult, error) {
	fake.command = command
	return fake.result, nil
}

type fakeListConversations struct {
	err error
}

func (fake fakeListConversations) Execute(context.Context, types.ListConversationsCommand) (types.ListConversationsResult, error) {
	if fake.err != nil {
		return types.ListConversationsResult{}, fake.err
	}
	return types.ListConversationsResult{}, nil
}

type fakeListConversationsCapture struct {
	command types.ListConversationsCommand
	result  types.ListConversationsResult
}

func (fake *fakeListConversationsCapture) Execute(_ context.Context, command types.ListConversationsCommand) (types.ListConversationsResult, error) {
	fake.command = command
	return fake.result, nil
}

type fakeArchiveConversation struct{}

func (fakeArchiveConversation) Execute(context.Context, types.ArchiveConversationCommand) (types.ArchiveConversationResult, error) {
	return types.ArchiveConversationResult{}, nil
}

type fakeArchiveConversationCapture struct {
	command types.ArchiveConversationCommand
	result  types.ArchiveConversationResult
}

func (fake *fakeArchiveConversationCapture) Execute(_ context.Context, command types.ArchiveConversationCommand) (types.ArchiveConversationResult, error) {
	fake.command = command
	return fake.result, nil
}

type fakePinConversation struct{}

func (fakePinConversation) Execute(context.Context, types.PinConversationCommand) (types.PinConversationResult, error) {
	return types.PinConversationResult{}, nil
}

type fakePinConversationCapture struct {
	command types.PinConversationCommand
	result  types.PinConversationResult
}

func (fake *fakePinConversationCapture) Execute(_ context.Context, command types.PinConversationCommand) (types.PinConversationResult, error) {
	fake.command = command
	return fake.result, nil
}

type fakeMuteConversation struct{}

func (fakeMuteConversation) Execute(context.Context, types.MuteConversationCommand) (types.MuteConversationResult, error) {
	return types.MuteConversationResult{}, nil
}

type fakeMuteConversationCapture struct {
	command types.MuteConversationCommand
	result  types.MuteConversationResult
}

func (fake *fakeMuteConversationCapture) Execute(_ context.Context, command types.MuteConversationCommand) (types.MuteConversationResult, error) {
	fake.command = command
	return fake.result, nil
}
