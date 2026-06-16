package grpc

import (
	"context"
	"testing"
	"time"

	receiptv1 "github.com/qsyy0921/IM/api/proto/nexusim/receipt/v1"
	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestMarkReadMapsValidationError(t *testing.T) {
	server := NewServer(fakeMarkRead{err: types.NewInvalidArgument("tenant_id is required")}, fakeGetReceiptState{}, fakeListReceiptStates{}, fakeListConversations{}, fakeArchiveConversation{}, fakePinConversation{}, fakeMuteConversation{}, fakeSetConversationTags{}, fakeSetConversationDraft{})
	_, err := server.MarkRead(context.Background(), &receiptv1.MarkReadRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
	}
}

func TestMarkReadSanitizesDBWriteError(t *testing.T) {
	server := NewServer(fakeMarkRead{err: types.NewDBWriteFailed("duplicate key value violates unique constraint receipt_outbox_event_id_key")}, fakeGetReceiptState{}, fakeListReceiptStates{}, fakeListConversations{}, fakeArchiveConversation{}, fakePinConversation{}, fakeMuteConversation{}, fakeSetConversationTags{}, fakeSetConversationDraft{})
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
	server := NewServer(fakeMarkRead{}, fakeGetReceiptState{}, fakeListReceiptStates{}, fakeListConversations{err: types.NewInvalidArgument("tenant_id is required")}, fakeArchiveConversation{}, fakePinConversation{}, fakeMuteConversation{}, fakeSetConversationTags{}, fakeSetConversationDraft{})
	_, err := server.ListConversations(context.Background(), &receiptv1.ListConversationsRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
	}
}

func TestReceiptAuthMetadataOverridesBodyForAllUserCommands(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		metadataTenantID, "trusted-tenant",
		metadataUserID, "trusted-user",
		metadataDeviceID, "trusted-device",
		metadataSessionID, "trusted-session",
	))
	interceptor := VerifiedAuthUnaryInterceptor(true)
	_, err := interceptor(ctx, nil, &grpcgo.UnaryServerInfo{}, func(ctx context.Context, request any) (any, error) {
		mark := &fakeMarkReadCapture{result: types.MarkReadResult{
			TenantID:       "trusted-tenant",
			UserID:         "trusted-user",
			ConversationID: "conversation-1",
			LastReadSeq:    10,
		}}
		get := &fakeGetReceiptStateCapture{result: types.GetReceiptStateResult{
			ConversationID:  "conversation-1",
			ConversationSeq: 10,
			MessageID:       "message-10",
		}}
		listStates := &fakeListReceiptStatesCapture{}
		listConversations := &fakeListConversationsCapture{}
		archive := &fakeArchiveConversationCapture{}
		pin := &fakePinConversationCapture{}
		mute := &fakeMuteConversationCapture{}
		setTags := &fakeSetConversationTagsCapture{}
		setDraft := &fakeSetConversationDraftCapture{}
		server := NewServer(mark, get, listStates, listConversations, archive, pin, mute, setTags, setDraft)

		if _, err := server.MarkRead(ctx, &receiptv1.MarkReadRequest{
			AuthContext:    testSpoofedAuthContext(),
			ConversationId: "conversation-1",
			ReadSeq:        10,
		}); err != nil {
			t.Fatalf("mark read: %v", err)
		}
		if _, err := server.GetReceiptState(ctx, &receiptv1.GetReceiptStateRequest{
			AuthContext:    testSpoofedAuthContext(),
			ConversationId: "conversation-1",
			MessageId:      "message-10",
		}); err != nil {
			t.Fatalf("get receipt state: %v", err)
		}
		if _, err := server.ListReceiptStates(ctx, &receiptv1.ListReceiptStatesRequest{
			AuthContext:    testSpoofedAuthContext(),
			ConversationId: "conversation-1",
			Items:          []*receiptv1.ReceiptStateQuery{{MessageId: "message-10"}},
		}); err != nil {
			t.Fatalf("list receipt states: %v", err)
		}
		if _, err := server.ListConversations(ctx, &receiptv1.ListConversationsRequest{
			AuthContext: testSpoofedAuthContext(),
			Limit:       20,
		}); err != nil {
			t.Fatalf("list conversations: %v", err)
		}
		if _, err := server.ArchiveConversation(ctx, &receiptv1.ArchiveConversationRequest{
			AuthContext:    testSpoofedAuthContext(),
			ConversationId: "conversation-1",
			Archived:       true,
		}); err != nil {
			t.Fatalf("archive conversation: %v", err)
		}
		if _, err := server.PinConversation(ctx, &receiptv1.PinConversationRequest{
			AuthContext:    testSpoofedAuthContext(),
			ConversationId: "conversation-1",
			Pinned:         true,
		}); err != nil {
			t.Fatalf("pin conversation: %v", err)
		}
		if _, err := server.MuteConversation(ctx, &receiptv1.MuteConversationRequest{
			AuthContext:    testSpoofedAuthContext(),
			ConversationId: "conversation-1",
			Muted:          true,
		}); err != nil {
			t.Fatalf("mute conversation: %v", err)
		}
		if _, err := server.SetConversationTags(ctx, &receiptv1.SetConversationTagsRequest{
			AuthContext:    testSpoofedAuthContext(),
			ConversationId: "conversation-1",
			Tags:           []string{"work"},
		}); err != nil {
			t.Fatalf("set conversation tags: %v", err)
		}
		if _, err := server.SetConversationDraft(ctx, &receiptv1.SetConversationDraftRequest{
			AuthContext:    testSpoofedAuthContext(),
			ConversationId: "conversation-1",
			DraftText:      "draft",
		}); err != nil {
			t.Fatalf("set conversation draft: %v", err)
		}

		assertTrustedMetadataAuth(t, mark.command.AuthContext)
		assertTrustedMetadataAuth(t, get.command.AuthContext)
		assertTrustedMetadataAuth(t, listStates.command.AuthContext)
		assertTrustedMetadataAuth(t, listConversations.command.AuthContext)
		assertTrustedMetadataAuth(t, archive.command.AuthContext)
		assertTrustedMetadataAuth(t, pin.command.AuthContext)
		assertTrustedMetadataAuth(t, mute.command.AuthContext)
		assertTrustedMetadataAuth(t, setTags.command.AuthContext)
		assertTrustedMetadataAuth(t, setDraft.command.AuthContext)
		return nil, nil
	})
	if err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
}

func TestReceiptAuthMetadataDoesNotRequireBodyAuthContext(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		metadataTenantID, "trusted-tenant",
		metadataUserID, "trusted-user",
		metadataDeviceID, "trusted-device",
		metadataTraceID, "trusted-trace",
		metadataRequestID, "trusted-request",
	))
	interceptor := VerifiedAuthUnaryInterceptor(true)
	_, err := interceptor(ctx, nil, &grpcgo.UnaryServerInfo{}, func(ctx context.Context, request any) (any, error) {
		list := &fakeListConversationsCapture{}
		server := NewServer(fakeMarkRead{}, fakeGetReceiptState{}, fakeListReceiptStates{}, list, fakeArchiveConversation{}, fakePinConversation{}, fakeMuteConversation{}, fakeSetConversationTags{}, fakeSetConversationDraft{})
		if _, err := server.ListConversations(ctx, &receiptv1.ListConversationsRequest{Limit: 10}); err != nil {
			t.Fatalf("list conversations: %v", err)
		}
		auth := list.command.AuthContext
		if auth.TenantID != "trusted-tenant" ||
			auth.UserID != "trusted-user" ||
			auth.DeviceID != "trusted-device" ||
			auth.TraceID != "trusted-trace" ||
			auth.RequestID != "trusted-request" {
			t.Fatalf("unexpected verified auth without body auth: %+v", auth)
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
}

func TestVerifiedAuthUnaryInterceptorRequiresTrustedIdentity(t *testing.T) {
	interceptor := VerifiedAuthUnaryInterceptor(true)
	_, err := interceptor(context.Background(), nil, &grpcgo.UnaryServerInfo{}, func(ctx context.Context, request any) (any, error) {
		t.Fatal("handler should not be called without verified auth")
		return nil, nil
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected unauthenticated, got %v (%v)", status.Code(err), err)
	}
}

func TestListConversationsMapsSort(t *testing.T) {
	tests := []struct {
		name string
		sort receiptv1.ConversationListSort
		want string
	}{
		{
			name: "updated",
			sort: receiptv1.ConversationListSort_CONVERSATION_LIST_SORT_UPDATED_AT_DESC,
			want: types.ConversationListSortUpdatedAtDesc,
		},
		{
			name: "pinned",
			sort: receiptv1.ConversationListSort_CONVERSATION_LIST_SORT_PINNED_UPDATED_AT_DESC,
			want: types.ConversationListSortPinnedUpdatedAtDesc,
		},
		{
			name: "unread",
			sort: receiptv1.ConversationListSort_CONVERSATION_LIST_SORT_UNREAD_UPDATED_AT_DESC,
			want: types.ConversationListSortUnreadUpdatedAtDesc,
		},
		{
			name: "draft",
			sort: receiptv1.ConversationListSort_CONVERSATION_LIST_SORT_DRAFT_UPDATED_AT_DESC,
			want: types.ConversationListSortDraftUpdatedAtDesc,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			list := &fakeListConversationsCapture{}
			server := NewServer(fakeMarkRead{}, fakeGetReceiptState{}, fakeListReceiptStates{}, list, fakeArchiveConversation{}, fakePinConversation{}, fakeMuteConversation{}, fakeSetConversationTags{}, fakeSetConversationDraft{})
			_, err := server.ListConversations(context.Background(), &receiptv1.ListConversationsRequest{
				AuthContext: &receiptv1.AuthContext{TenantId: "tenant-1", UserId: "user-1", DeviceId: "device-1"},
				Limit:       20,
				PageCursor:  "cursor-1",
				Sort:        test.sort,
				UnreadOnly:  true,
				PinnedOnly:  true,
				MutedOnly:   true,
				TagFilter:   "work",
				DraftOnly:   true,
			})
			if err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
			if list.command.Sort != test.want ||
				list.command.Limit != 20 ||
				list.command.PageCursor != "cursor-1" ||
				!list.command.UnreadOnly ||
				!list.command.PinnedOnly ||
				!list.command.MutedOnly ||
				list.command.TagFilter != "work" ||
				!list.command.DraftOnly {
				t.Fatalf("unexpected list command: %+v", list.command)
			}
		})
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
				Tags:                []string{"work", "urgent"},
			}},
			NextPageCursor: "next-cursor",
			ProjectionWatermark: types.ProjectionWatermark{
				Source:      "im.delivery.events",
				OffsetValue: 22,
				UpdatedAt:   updatedAt,
			},
		},
	}
	server := NewServer(fakeMarkRead{}, fakeGetReceiptState{}, fakeListReceiptStates{}, list, fakeArchiveConversation{}, fakePinConversation{}, fakeMuteConversation{}, fakeSetConversationTags{}, fakeSetConversationDraft{})
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
		item.GetArchived() ||
		len(item.GetTags()) != 2 ||
		item.GetTags()[0] != "work" ||
		item.GetTags()[1] != "urgent" {
		t.Fatalf("unexpected item: %+v", item)
	}
}

func TestGetReceiptStateMapsReceivedDeviceDetails(t *testing.T) {
	updatedAt := time.Date(2026, 6, 10, 12, 30, 0, 0, time.UTC)
	get := &fakeGetReceiptStateCapture{
		result: types.GetReceiptStateResult{
			ConversationID:    "conversation-1",
			ConversationSeq:   11,
			MessageID:         "message-11",
			ReceivedUserCount: 1,
			VisibilityMode:    types.ReceiptVisibilityDetailed,
			Receivers: []types.ReceiptUserState{{
				UserID:                   "user-2",
				ReceivedSeq:              11,
				ReceivedDeviceCount:      2,
				ReceivedDevicesTruncated: true,
				ReceivedDevices: []types.ReceivedDeviceState{{
					DeviceID:        "device-2",
					LastReceivedSeq: 12,
					UpdatedAt:       updatedAt,
				}},
			}},
		},
	}
	server := NewServer(fakeMarkRead{}, get, fakeListReceiptStates{}, fakeListConversations{}, fakeArchiveConversation{}, fakePinConversation{}, fakeMuteConversation{}, fakeSetConversationTags{}, fakeSetConversationDraft{})
	response, err := server.GetReceiptState(context.Background(), &receiptv1.GetReceiptStateRequest{
		AuthContext:            &receiptv1.AuthContext{TenantId: "tenant-1", UserId: "user-1", DeviceId: "device-1"},
		ConversationId:         "conversation-1",
		MessageId:              "message-11",
		IncludeReceivedDevices: true,
		ReceivedDeviceLimit:    1,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !get.command.IncludeReceivedDevices || get.command.ReceivedDeviceLimitHint != 1 {
		t.Fatalf("unexpected get command: %+v", get.command)
	}
	receiver := response.GetReceivers()[0]
	if receiver.GetReceivedDeviceCount() != 2 ||
		!receiver.GetReceivedDevicesTruncated() ||
		len(receiver.GetReceivedDevices()) != 1 ||
		receiver.GetReceivedDevices()[0].GetDeviceId() != "device-2" ||
		receiver.GetReceivedDevices()[0].GetLastReceivedSeq() != 12 ||
		receiver.GetReceivedDevices()[0].GetUpdatedAtUnixMs() != updatedAt.UnixMilli() {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestListReceiptStatesMapsRequestAndResponse(t *testing.T) {
	updatedAt := time.Date(2026, 6, 10, 12, 30, 0, 0, time.UTC)
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
					UserID:              "user-2",
					ReceivedSeq:         11,
					ReadSeq:             11,
					ReceivedDeviceCount: 2,
					ReceivedDevices: []types.ReceivedDeviceState{{
						DeviceID:        "device-2",
						LastReceivedSeq: 12,
						UpdatedAt:       updatedAt,
					}},
				}},
			}},
		},
	}
	server := NewServer(fakeMarkRead{}, fakeGetReceiptState{}, list, fakeListConversations{}, fakeArchiveConversation{}, fakePinConversation{}, fakeMuteConversation{}, fakeSetConversationTags{}, fakeSetConversationDraft{})
	response, err := server.ListReceiptStates(context.Background(), &receiptv1.ListReceiptStatesRequest{
		AuthContext:    &receiptv1.AuthContext{TenantId: "tenant-1", UserId: "user-1", DeviceId: "device-1"},
		ConversationId: "conversation-1",
		Items: []*receiptv1.ReceiptStateQuery{
			{MessageId: "message-11"},
			{ConversationSeq: 12},
		},
		IncludeReceivedDevices: true,
		ReceivedDeviceLimit:    5,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if list.command.ConversationID != "conversation-1" ||
		len(list.command.Items) != 2 ||
		list.command.Items[0].MessageID != "message-11" ||
		list.command.Items[1].ConversationSeq != 12 ||
		!list.command.IncludeReceivedDevices ||
		list.command.ReceivedDeviceLimitHint != 5 {
		t.Fatalf("unexpected command: %+v", list.command)
	}
	if len(response.GetItems()) != 1 ||
		response.GetItems()[0].GetConversationSeq() != 11 ||
		response.GetItems()[0].GetReadUserCount() != 1 ||
		response.GetItems()[0].GetReceivers()[0].GetReceivedDeviceCount() != 2 ||
		response.GetItems()[0].GetReceivers()[0].GetReceivedDevices()[0].GetDeviceId() != "device-2" {
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
	server := NewServer(fakeMarkRead{}, fakeGetReceiptState{}, fakeListReceiptStates{}, fakeListConversations{}, archive, fakePinConversation{}, fakeMuteConversation{}, fakeSetConversationTags{}, fakeSetConversationDraft{})
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
	server := NewServer(fakeMarkRead{}, fakeGetReceiptState{}, fakeListReceiptStates{}, fakeListConversations{}, fakeArchiveConversation{}, pin, fakeMuteConversation{}, fakeSetConversationTags{}, fakeSetConversationDraft{})
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
	server := NewServer(fakeMarkRead{}, fakeGetReceiptState{}, fakeListReceiptStates{}, fakeListConversations{}, fakeArchiveConversation{}, fakePinConversation{}, mute, fakeSetConversationTags{}, fakeSetConversationDraft{})
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

func TestSetConversationTagsMapsRequestAndResponse(t *testing.T) {
	setTags := &fakeSetConversationTagsCapture{
		result: types.SetConversationTagsResult{
			Conversation: types.ConversationSummary{
				ConversationID: "conversation-1",
				LastVisibleSeq: 7,
				LastMessageID:  "message-7",
				Tags:           []string{"work", "urgent"},
			},
		},
	}
	server := NewServer(fakeMarkRead{}, fakeGetReceiptState{}, fakeListReceiptStates{}, fakeListConversations{}, fakeArchiveConversation{}, fakePinConversation{}, fakeMuteConversation{}, setTags, fakeSetConversationDraft{})
	response, err := server.SetConversationTags(context.Background(), &receiptv1.SetConversationTagsRequest{
		AuthContext:    &receiptv1.AuthContext{TenantId: "tenant-1", UserId: "user-1", DeviceId: "device-1"},
		ConversationId: "conversation-1",
		Tags:           []string{"work", "urgent"},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if setTags.command.ConversationID != "conversation-1" ||
		len(setTags.command.Tags) != 2 ||
		setTags.command.Tags[0] != "work" ||
		setTags.command.Tags[1] != "urgent" {
		t.Fatalf("unexpected set tags command: %+v", setTags.command)
	}
	if response.GetConversation().GetConversationId() != "conversation-1" ||
		len(response.GetConversation().GetTags()) != 2 ||
		response.GetConversation().GetTags()[1] != "urgent" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestSetConversationDraftMapsRequestAndResponse(t *testing.T) {
	setDraft := &fakeSetConversationDraftCapture{
		result: types.SetConversationDraftResult{
			Conversation: types.ConversationSummary{
				ConversationID: "conversation-1",
				LastVisibleSeq: 7,
				LastMessageID:  "message-7",
				DraftText:      "hello draft",
			},
		},
	}
	server := NewServer(fakeMarkRead{}, fakeGetReceiptState{}, fakeListReceiptStates{}, fakeListConversations{}, fakeArchiveConversation{}, fakePinConversation{}, fakeMuteConversation{}, fakeSetConversationTags{}, setDraft)
	response, err := server.SetConversationDraft(context.Background(), &receiptv1.SetConversationDraftRequest{
		AuthContext:    &receiptv1.AuthContext{TenantId: "tenant-1", UserId: "user-1", DeviceId: "device-1"},
		ConversationId: "conversation-1",
		DraftText:      "hello draft",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if setDraft.command.ConversationID != "conversation-1" ||
		setDraft.command.DraftText != "hello draft" {
		t.Fatalf("unexpected set draft command: %+v", setDraft.command)
	}
	if response.GetConversation().GetConversationId() != "conversation-1" ||
		response.GetConversation().GetDraftText() != "hello draft" {
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

type fakeMarkReadCapture struct {
	command types.MarkReadCommand
	result  types.MarkReadResult
}

func (fake *fakeMarkReadCapture) Execute(_ context.Context, command types.MarkReadCommand) (types.MarkReadResult, error) {
	fake.command = command
	return fake.result, nil
}

type fakeGetReceiptState struct{}

func (fakeGetReceiptState) Execute(context.Context, types.GetReceiptStateCommand) (types.GetReceiptStateResult, error) {
	return types.GetReceiptStateResult{}, nil
}

type fakeGetReceiptStateCapture struct {
	command types.GetReceiptStateCommand
	result  types.GetReceiptStateResult
}

func (fake *fakeGetReceiptStateCapture) Execute(_ context.Context, command types.GetReceiptStateCommand) (types.GetReceiptStateResult, error) {
	fake.command = command
	return fake.result, nil
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

type fakeSetConversationTags struct{}

func (fakeSetConversationTags) Execute(context.Context, types.SetConversationTagsCommand) (types.SetConversationTagsResult, error) {
	return types.SetConversationTagsResult{}, nil
}

type fakeSetConversationTagsCapture struct {
	command types.SetConversationTagsCommand
	result  types.SetConversationTagsResult
}

func (fake *fakeSetConversationTagsCapture) Execute(_ context.Context, command types.SetConversationTagsCommand) (types.SetConversationTagsResult, error) {
	fake.command = command
	return fake.result, nil
}

type fakeSetConversationDraft struct{}

func (fakeSetConversationDraft) Execute(context.Context, types.SetConversationDraftCommand) (types.SetConversationDraftResult, error) {
	return types.SetConversationDraftResult{}, nil
}

type fakeSetConversationDraftCapture struct {
	command types.SetConversationDraftCommand
	result  types.SetConversationDraftResult
}

func (fake *fakeSetConversationDraftCapture) Execute(_ context.Context, command types.SetConversationDraftCommand) (types.SetConversationDraftResult, error) {
	fake.command = command
	return fake.result, nil
}

func testSpoofedAuthContext() *receiptv1.AuthContext {
	return &receiptv1.AuthContext{
		TenantId:  "spoofed-tenant",
		UserId:    "spoofed-user",
		DeviceId:  "spoofed-device",
		SessionId: "spoofed-session",
		TraceId:   "body-trace",
		RequestId: "body-request",
	}
}

func assertTrustedMetadataAuth(t *testing.T, auth types.AuthContext) {
	t.Helper()
	if auth.TenantID != "trusted-tenant" ||
		auth.UserID != "trusted-user" ||
		auth.DeviceID != "trusted-device" ||
		auth.SessionID != "trusted-session" ||
		auth.TraceID != "body-trace" ||
		auth.RequestID != "body-request" {
		t.Fatalf("unexpected verified auth: %+v", auth)
	}
}
