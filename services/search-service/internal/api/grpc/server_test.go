package grpc

import (
	"context"
	"testing"
	"time"

	searchv1 "github.com/qsyy0921/IM/api/proto/nexusim/search/v1"
	"github.com/qsyy0921/IM/services/search-service/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeSearchMessagesExecutor struct {
	command types.SearchMessagesCommand
	result  types.SearchMessagesResult
	err     error
}

func (executor *fakeSearchMessagesExecutor) Execute(
	_ context.Context,
	command types.SearchMessagesCommand,
) (types.SearchMessagesResult, error) {
	executor.command = command
	return executor.result, executor.err
}

func TestSearchMessagesMapsRequestAndResponse(t *testing.T) {
	executor := &fakeSearchMessagesExecutor{
		result: types.SearchMessagesResult{
			Items: []types.SearchMessageHit{
				{
					ConversationID:    "conv-1",
					MessageID:         "msg-1",
					ConversationSeq:   10,
					SourceEventID:     "event-1",
					SenderID:          "user-2",
					MessageType:       "TEXT",
					Snippet:           "hello world",
					HighlightRanges:   []types.HighlightRange{{Start: 0, End: 5}},
					OccurredAt:        time.UnixMilli(1234),
					VisibilityVersion: 7,
				},
			},
			NextCursor:        "msg-1",
			ProjectionVersion: 12,
		},
	}
	server := NewServer(executor)
	response, err := server.SearchMessages(context.Background(), &searchv1.SearchMessagesRequest{
		AuthContext: &searchv1.AuthContext{
			TenantId: "tenant-1",
			UserId:   "user-1",
			DeviceId: "device-1",
		},
		Query:          "hello",
		ConversationId: "conv-1",
		Limit:          20,
	})
	if err != nil {
		t.Fatalf("search messages: %v", err)
	}
	if executor.command.AuthContext.TenantID != "tenant-1" || executor.command.Query != "hello" {
		t.Fatalf("unexpected command: %+v", executor.command)
	}
	if len(response.GetItems()) != 1 {
		t.Fatalf("expected one item, got %d", len(response.GetItems()))
	}
	item := response.GetItems()[0]
	if item.GetMessageId() != "msg-1" || item.GetVisibilityVersion() != 7 {
		t.Fatalf("unexpected item: %+v", item)
	}
	if response.GetNextCursor() != "msg-1" || response.GetProjectionVersion() != 12 {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestSearchMessagesPermissionDeniedMapping(t *testing.T) {
	server := NewServer(&fakeSearchMessagesExecutor{err: types.ErrPermissionDenied})
	_, err := server.SearchMessages(context.Background(), &searchv1.SearchMessagesRequest{
		AuthContext: &searchv1.AuthContext{
			TenantId: "tenant-1",
			UserId:   "user-1",
			DeviceId: "device-1",
		},
		Query: "hello",
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected permission denied, got %v", err)
	}
}
