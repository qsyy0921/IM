package grpc

import (
	"context"
	"errors"
	"strings"
	"testing"

	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGetSendContextConvertsResponse(t *testing.T) {
	executor := &fakeGetSendContextExecutor{
		result: types.ConversationSendContext{
			TenantID:            "tenant-1",
			ConversationID:      "conv-1",
			MemberVersion:       5,
			PermissionVersion:   7,
			ConversationMode:    types.ConversationModeLocalRowLock,
			FanoutMode:          types.FanoutModeWriteFanout,
			FanoutPolicyVersion: 3,
			CurrentSeqShard:     "local",
		},
	}
	server := NewServer(executor)

	response, err := server.GetSendContext(context.Background(), &conversationv1.GetSendContextRequest{
		TenantId:       "tenant-1",
		ConversationId: "conv-1",
		UserId:         "user-1",
		TraceId:        "trace-1",
	})
	if err != nil {
		t.Fatalf("get send context: %v", err)
	}
	if executor.command.TenantID != "tenant-1" ||
		executor.command.ConversationID != "conv-1" ||
		executor.command.UserID != "user-1" ||
		executor.command.TraceID != "trace-1" {
		t.Fatalf("unexpected command: %+v", executor.command)
	}
	if response.GetConversationMode() != conversationv1.ConversationMode_CONVERSATION_MODE_LOCAL_ROW_LOCK ||
		response.GetFanoutMode() != conversationv1.FanoutMode_FANOUT_MODE_WRITE_FANOUT ||
		response.GetMemberVersion() != 5 ||
		response.GetPermissionVersion() != 7 ||
		response.GetFanoutPolicyVersion() != 3 ||
		response.GetCurrentSeqShard() != "local" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestGetSendContextMapsErrors(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		code    codes.Code
		message string
	}{
		{name: "invalid argument", err: types.NewInvalidArgument("tenant_id is required"), code: codes.InvalidArgument, message: "invalid argument"},
		{name: "not found", err: types.NewConversationNotFound("missing"), code: codes.NotFound, message: "conversation not found"},
		{name: "member inactive", err: types.NewMemberNotActive("left"), code: codes.PermissionDenied, message: "conversation member is not active"},
		{name: "db read", err: types.NewDBReadFailed("select conversations timeout"), code: codes.Unavailable, message: "conversation read failed"},
		{name: "unknown", err: errors.New("boom"), code: codes.Internal, message: "conversation service internal error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := NewServer(&fakeGetSendContextExecutor{err: tc.err})

			_, err := server.GetSendContext(context.Background(), &conversationv1.GetSendContextRequest{
				TenantId:       "tenant-1",
				ConversationId: "conv-1",
				UserId:         "user-1",
			})
			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("expected status error, got %v", err)
			}
			if st.Code() != tc.code {
				t.Fatalf("expected %s, got %s", tc.code, st.Code())
			}
			if st.Message() != tc.message {
				t.Fatalf("expected stable message %q, got %q", tc.message, st.Message())
			}
			if strings.Contains(st.Message(), "select conversations") || strings.Contains(st.Message(), "boom") {
				t.Fatalf("status message leaked internal detail: %q", st.Message())
			}
		})
	}
}

func TestGetSendContextMapsValidationErrors(t *testing.T) {
	cases := []struct {
		name    string
		request *conversationv1.GetSendContextRequest
	}{
		{
			name: "missing tenant",
			request: &conversationv1.GetSendContextRequest{
				ConversationId: "conv-1",
				UserId:         "user-1",
			},
		},
		{
			name: "missing conversation",
			request: &conversationv1.GetSendContextRequest{
				TenantId: "tenant-1",
				UserId:   "user-1",
			},
		},
		{
			name: "missing user",
			request: &conversationv1.GetSendContextRequest{
				TenantId:       "tenant-1",
				ConversationId: "conv-1",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewServer(&fakeGetSendContextExecutor{validate: true}).GetSendContext(context.Background(), tc.request)
			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("expected status error, got %v", err)
			}
			if st.Code() != codes.InvalidArgument {
				t.Fatalf("expected invalid argument, got %s", st.Code())
			}
			if st.Message() != "invalid argument" {
				t.Fatalf("expected stable message, got %q", st.Message())
			}
		})
	}
}

func TestGetSendContextRejectsNilRequest(t *testing.T) {
	_, err := NewServer(&fakeGetSendContextExecutor{}).GetSendContext(context.Background(), nil)
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Fatalf("expected invalid argument, got %s", st.Code())
	}
}

type fakeGetSendContextExecutor struct {
	result   types.ConversationSendContext
	err      error
	command  types.GetSendContextCommand
	validate bool
}

func (f *fakeGetSendContextExecutor) Execute(
	_ context.Context,
	command types.GetSendContextCommand,
) (types.ConversationSendContext, error) {
	f.command = command
	if f.validate {
		if err := command.Validate(); err != nil {
			return types.ConversationSendContext{}, err
		}
	}
	return f.result, f.err
}
