package grpc

import (
	"context"
	"testing"

	receiptv1 "github.com/qsyy0921/IM/api/proto/nexusim/receipt/v1"
	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMarkReadMapsValidationError(t *testing.T) {
	server := NewServer(fakeMarkRead{err: types.NewInvalidArgument("tenant_id is required")}, fakeGetReceiptState{}, fakeListConversations{})
	_, err := server.MarkRead(context.Background(), &receiptv1.MarkReadRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
	}
}

func TestMarkReadSanitizesDBWriteError(t *testing.T) {
	server := NewServer(fakeMarkRead{err: types.NewDBWriteFailed("duplicate key value violates unique constraint receipt_outbox_event_id_key")}, fakeGetReceiptState{}, fakeListConversations{})
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
	server := NewServer(fakeMarkRead{}, fakeGetReceiptState{}, fakeListConversations{err: types.NewInvalidArgument("tenant_id is required")})
	_, err := server.ListConversations(context.Background(), &receiptv1.ListConversationsRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
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

type fakeListConversations struct {
	err error
}

func (fake fakeListConversations) Execute(context.Context, types.ListConversationsCommand) (types.ListConversationsResult, error) {
	if fake.err != nil {
		return types.ListConversationsResult{}, fake.err
	}
	return types.ListConversationsResult{}, nil
}
