package rpc

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestConversationClientGetSendContext(t *testing.T) {
	client, cleanup := newBufconnConversationClient(t, &fakeConversationServer{
		response: &conversationv1.GetSendContextResponse{
			TenantId:            "tenant-1",
			ConversationId:      "conv-1",
			MemberVersion:       5,
			PermissionVersion:   7,
			ConversationMode:    conversationv1.ConversationMode_CONVERSATION_MODE_LOCAL_ROW_LOCK,
			FanoutMode:          conversationv1.FanoutMode_FANOUT_MODE_WRITE_FANOUT,
			FanoutPolicyVersion: 3,
			CurrentSeqShard:     "local",
		},
	})
	defer cleanup()

	result, err := client.GetSendContext(context.Background(), conversationCommand())
	if err != nil {
		t.Fatalf("get send context: %v", err)
	}
	if result.MemberVersion != 5 ||
		result.PermissionVersion != 7 ||
		result.ConversationMode != types.ConversationModeLocalRowLock ||
		result.FanoutMode != types.FanoutModeWriteFanout ||
		result.FanoutPolicyVersion != 3 ||
		result.CurrentSeqShard != "local" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestConversationClientMapsNotFound(t *testing.T) {
	client, cleanup := newBufconnConversationClient(t, &fakeConversationServer{
		err: status.Error(codes.NotFound, "conversation not found"),
	})
	defer cleanup()

	_, err := client.GetSendContext(context.Background(), conversationCommand())
	if !errors.Is(err, types.ErrConversationNotFound) {
		t.Fatalf("expected conversation not found, got %v", err)
	}
}

func TestConversationClientMapsFanoutModes(t *testing.T) {
	cases := []struct {
		name string
		in   conversationv1.FanoutMode
		want types.FanoutMode
	}{
		{name: "write fanout", in: conversationv1.FanoutMode_FANOUT_MODE_WRITE_FANOUT, want: types.FanoutModeWriteFanout},
		{name: "hybrid fanout", in: conversationv1.FanoutMode_FANOUT_MODE_HYBRID_FANOUT, want: types.FanoutModeHybridFanout},
		{name: "read fanout", in: conversationv1.FanoutMode_FANOUT_MODE_READ_FANOUT, want: types.FanoutModeReadFanout},
		{name: "broadcast signal", in: conversationv1.FanoutMode_FANOUT_MODE_BROADCAST_SIGNAL, want: types.FanoutModeBroadcastSignal},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, cleanup := newBufconnConversationClient(t, &fakeConversationServer{
				response: &conversationv1.GetSendContextResponse{
					ConversationMode:  conversationv1.ConversationMode_CONVERSATION_MODE_LOCAL_ROW_LOCK,
					FanoutMode:        tc.in,
					PermissionVersion: 1,
					TenantId:          "tenant-1",
					ConversationId:    "conv-1",
					CurrentSeqShard:   "local",
				},
			})
			defer cleanup()

			result, err := client.GetSendContext(context.Background(), conversationCommand())
			if err != nil {
				t.Fatalf("get send context: %v", err)
			}
			if result.FanoutMode != tc.want {
				t.Fatalf("unexpected fanout mode: got %q want %q", result.FanoutMode, tc.want)
			}
		})
	}
}

func TestConversationClientRejectsInvalidResponseContract(t *testing.T) {
	cases := []struct {
		name     string
		response *conversationv1.GetSendContextResponse
	}{
		{
			name: "mismatched tenant",
			response: &conversationv1.GetSendContextResponse{
				TenantId:          "other-tenant",
				ConversationId:    "conv-1",
				ConversationMode:  conversationv1.ConversationMode_CONVERSATION_MODE_LOCAL_ROW_LOCK,
				FanoutMode:        conversationv1.FanoutMode_FANOUT_MODE_WRITE_FANOUT,
				CurrentSeqShard:   "local",
				PermissionVersion: 1,
			},
		},
		{
			name: "unspecified conversation mode",
			response: &conversationv1.GetSendContextResponse{
				TenantId:          "tenant-1",
				ConversationId:    "conv-1",
				FanoutMode:        conversationv1.FanoutMode_FANOUT_MODE_WRITE_FANOUT,
				CurrentSeqShard:   "local",
				PermissionVersion: 1,
			},
		},
		{
			name: "unspecified fanout mode",
			response: &conversationv1.GetSendContextResponse{
				TenantId:          "tenant-1",
				ConversationId:    "conv-1",
				ConversationMode:  conversationv1.ConversationMode_CONVERSATION_MODE_LOCAL_ROW_LOCK,
				CurrentSeqShard:   "local",
				PermissionVersion: 1,
			},
		},
		{
			name: "empty seq shard",
			response: &conversationv1.GetSendContextResponse{
				TenantId:          "tenant-1",
				ConversationId:    "conv-1",
				ConversationMode:  conversationv1.ConversationMode_CONVERSATION_MODE_LOCAL_ROW_LOCK,
				FanoutMode:        conversationv1.FanoutMode_FANOUT_MODE_WRITE_FANOUT,
				PermissionVersion: 1,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, cleanup := newBufconnConversationClient(t, &fakeConversationServer{response: tc.response})
			defer cleanup()

			_, err := client.GetSendContext(context.Background(), conversationCommand())
			if !errors.Is(err, types.ErrDependencyUnavailable) {
				t.Fatalf("expected dependency unavailable, got %v", err)
			}
		})
	}
}

func newBufconnConversationClient(t *testing.T, server conversationv1.ConversationServiceServer) (ConversationClient, func()) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpcgo.NewServer()
	conversationv1.RegisterConversationServiceServer(grpcServer, server)
	go func() {
		if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpcgo.ErrServerStopped) {
			t.Errorf("serve bufconn grpc: %v", err)
		}
	}()

	conn, err := grpcgo.NewClient(
		"passthrough:///bufnet",
		grpcgo.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpcgo.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn grpc: %v", err)
	}
	return NewConversationClient(conversationv1.NewConversationServiceClient(conn), time.Second), func() {
		_ = conn.Close()
		grpcServer.Stop()
	}
}

func conversationCommand() types.SendMessageCommand {
	return types.SendMessageCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			TraceID:  "trace-1",
		},
		ConversationID: "conv-1",
	}
}

type fakeConversationServer struct {
	conversationv1.UnimplementedConversationServiceServer
	response *conversationv1.GetSendContextResponse
	err      error
}

func (f *fakeConversationServer) GetSendContext(
	context.Context,
	*conversationv1.GetSendContextRequest,
) (*conversationv1.GetSendContextResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.response, nil
}
