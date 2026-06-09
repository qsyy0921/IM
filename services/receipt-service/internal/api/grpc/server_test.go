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
	server := NewServer(fakeMarkRead{err: types.NewInvalidArgument("tenant_id is required")}, fakeGetReceiptState{}, fakeListReceiptStates{}, fakeListConversations{})
	_, err := server.MarkRead(context.Background(), &receiptv1.MarkReadRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
	}
}

func TestMarkReadSanitizesDBWriteError(t *testing.T) {
	server := NewServer(fakeMarkRead{err: types.NewDBWriteFailed("duplicate key value violates unique constraint receipt_outbox_event_id_key")}, fakeGetReceiptState{}, fakeListReceiptStates{}, fakeListConversations{})
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
	server := NewServer(fakeMarkRead{}, fakeGetReceiptState{}, fakeListReceiptStates{}, fakeListConversations{err: types.NewInvalidArgument("tenant_id is required")})
	_, err := server.ListConversations(context.Background(), &receiptv1.ListConversationsRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
	}
}

func TestListConversationsMapsSort(t *testing.T) {
	list := &fakeListConversationsCapture{}
	server := NewServer(fakeMarkRead{}, fakeGetReceiptState{}, fakeListReceiptStates{}, list)
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
	server := NewServer(fakeMarkRead{}, fakeGetReceiptState{}, fakeListReceiptStates{}, list)
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
		item.GetUpdatedAtUnixMs() != updatedAt.UnixMilli() {
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
	server := NewServer(fakeMarkRead{}, fakeGetReceiptState{}, list, fakeListConversations{})
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
