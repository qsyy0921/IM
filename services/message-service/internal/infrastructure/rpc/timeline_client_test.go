package rpc

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	timelinev1 "github.com/qsyy0921/IM/api/proto/nexusim/timeline/v1"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestTimelineClientAllocateSeqBlock(t *testing.T) {
	fake := &fakeTimelineServer{
		response: &timelinev1.AllocateSeqBlockResponse{
			TenantId:         "tenant-1",
			ConversationId:   "conv-1",
			StartSeq:         42,
			EndSeq:           42,
			BlockSize:        1,
			SequencerEpoch:   3,
			LeaseId:          "lease-1",
			ExpiresAtUnixMs:  time.Now().Add(time.Minute).UnixMilli(),
			IdempotentReplay: false,
		},
	}
	client, cleanup := newBufconnTimelineClient(t, fake)
	defer cleanup()

	result, err := client.AllocateSeqBlock(context.Background(), timelineCommand(), 42)
	if err != nil {
		t.Fatalf("allocate seq block: %v", err)
	}
	if result.StartSeq != 42 || result.EndSeq != 42 || result.Epoch != 3 {
		t.Fatalf("unexpected seq block: %+v", result)
	}
	if result.LeaseID != "lease-1" || result.ExpiresAt.IsZero() {
		t.Fatalf("expected lease metadata, got %+v", result)
	}
	if fake.request.GetTenantId() != "tenant-1" ||
		fake.request.GetConversationId() != "conv-1" ||
		fake.request.GetRequesterId() != timelineRequesterID ||
		fake.request.GetBlockSize() != 1 ||
		fake.request.GetIdempotencyKey() == "" ||
		fake.request.GetMinimumStartSeq() != 42 {
		t.Fatalf("unexpected request: %+v", fake.request)
	}
}

func TestTimelineClientPropagatesTraceAndRequestMetadata(t *testing.T) {
	fake := &fakeTimelineServer{
		response: &timelinev1.AllocateSeqBlockResponse{
			TenantId:        "tenant-1",
			ConversationId:  "conv-1",
			StartSeq:        1,
			EndSeq:          1,
			BlockSize:       1,
			SequencerEpoch:  1,
			LeaseId:         "lease-1",
			ExpiresAtUnixMs: time.Now().Add(time.Minute).UnixMilli(),
		},
	}
	client, cleanup := newBufconnTimelineClient(t, fake)
	defer cleanup()

	_, err := client.AllocateSeqBlock(context.Background(), timelineCommand(), 1)
	if err != nil {
		t.Fatalf("allocate seq block: %v", err)
	}
	if fake.incomingMetadata.Get(timelineMetadataTraceID)[0] != "trace-1" ||
		fake.incomingMetadata.Get(timelineMetadataRequestID)[0] != "request-1" {
		t.Fatalf("expected trace/request metadata, got %v", fake.incomingMetadata)
	}
	if len(fake.incomingMetadata.Get("authorization")) != 0 {
		t.Fatalf("unexpected auth metadata leak: %v", fake.incomingMetadata)
	}
}

func TestTimelineClientMapsIdempotencyConflict(t *testing.T) {
	client, cleanup := newBufconnTimelineClient(t, &fakeTimelineServer{
		err: status.Error(codes.Aborted, "idempotency conflict"),
	})
	defer cleanup()

	_, err := client.AllocateSeqBlock(context.Background(), timelineCommand(), 1)
	if !errors.Is(err, types.ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
}

func TestTimelineClientRejectsInvalidResponseContract(t *testing.T) {
	cases := []struct {
		name     string
		response *timelinev1.AllocateSeqBlockResponse
	}{
		{
			name: "mismatched tenant",
			response: &timelinev1.AllocateSeqBlockResponse{
				TenantId:        "other-tenant",
				ConversationId:  "conv-1",
				StartSeq:        1,
				EndSeq:          1,
				BlockSize:       1,
				SequencerEpoch:  1,
				LeaseId:         "lease-1",
				ExpiresAtUnixMs: time.Now().Add(time.Minute).UnixMilli(),
			},
		},
		{
			name: "range block not supported by first stage",
			response: &timelinev1.AllocateSeqBlockResponse{
				TenantId:        "tenant-1",
				ConversationId:  "conv-1",
				StartSeq:        1,
				EndSeq:          10,
				BlockSize:       10,
				SequencerEpoch:  1,
				LeaseId:         "lease-1",
				ExpiresAtUnixMs: time.Now().Add(time.Minute).UnixMilli(),
			},
		},
		{
			name: "missing lease",
			response: &timelinev1.AllocateSeqBlockResponse{
				TenantId:        "tenant-1",
				ConversationId:  "conv-1",
				StartSeq:        1,
				EndSeq:          1,
				BlockSize:       1,
				SequencerEpoch:  1,
				ExpiresAtUnixMs: time.Now().Add(time.Minute).UnixMilli(),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, cleanup := newBufconnTimelineClient(t, &fakeTimelineServer{response: tc.response})
			defer cleanup()

			_, err := client.AllocateSeqBlock(context.Background(), timelineCommand(), 1)
			if !errors.Is(err, types.ErrSequencerUnavailable) {
				t.Fatalf("expected sequencer unavailable, got %v", err)
			}
		})
	}
}

func TestTimelineClientCachesSeqBlock(t *testing.T) {
	fake := &fakeTimelineServer{
		response: &timelinev1.AllocateSeqBlockResponse{
			TenantId:         "tenant-1",
			ConversationId:   "conv-1",
			StartSeq:         100,
			EndSeq:           102,
			BlockSize:        3,
			SequencerEpoch:   7,
			LeaseId:          "lease-cached",
			ExpiresAtUnixMs:  time.Now().Add(time.Minute).UnixMilli(),
			IdempotentReplay: false,
		},
	}
	client, cleanup := newBufconnTimelineClientWithConfig(t, fake, 3)
	defer cleanup()

	first, err := client.AllocateSeqBlock(context.Background(), timelineCommand(), 100)
	if err != nil {
		t.Fatalf("allocate first seq: %v", err)
	}
	second, err := client.AllocateSeqBlock(context.Background(), timelineCommand(), 100)
	if err != nil {
		t.Fatalf("allocate second seq: %v", err)
	}
	third, err := client.AllocateSeqBlock(context.Background(), timelineCommand(), 100)
	if err != nil {
		t.Fatalf("allocate third seq: %v", err)
	}
	if first.StartSeq != 100 || second.StartSeq != 101 || third.StartSeq != 102 {
		t.Fatalf("unexpected cached seqs: first=%+v second=%+v third=%+v", first, second, third)
	}
	if fake.calls != 1 || fake.request.GetBlockSize() != 3 {
		t.Fatalf("expected one remote block allocation, calls=%d request=%+v", fake.calls, fake.request)
	}
}

func newBufconnTimelineClient(t *testing.T, server timelinev1.TimelineServiceServer) (TimelineClient, func()) {
	return newBufconnTimelineClientWithConfig(t, server, 1)
}

func newBufconnTimelineClientWithConfig(t *testing.T, server timelinev1.TimelineServiceServer, blockSize int) (TimelineClient, func()) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpcgo.NewServer()
	timelinev1.RegisterTimelineServiceServer(grpcServer, server)
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
	return NewTimelineClientWithConfig(timelinev1.NewTimelineServiceClient(conn), time.Second, blockSize, 0), func() {
		_ = conn.Close()
		grpcServer.Stop()
	}
}

func timelineCommand() types.SendMessageCommand {
	return types.SendMessageCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-1",
			UserID:    "user-1",
			DeviceID:  "device-1",
			TraceID:   "trace-1",
			RequestID: "request-1",
		},
		ConversationID: "conv-1",
		ClientMsgID:    "client-1",
		MessageType:    types.MessageTypeText,
		PayloadJSON:    []byte(`{"text":"hello"}`),
	}
}

type fakeTimelineServer struct {
	timelinev1.UnimplementedTimelineServiceServer
	incomingMetadata metadata.MD
	request          *timelinev1.AllocateSeqBlockRequest
	response         *timelinev1.AllocateSeqBlockResponse
	err              error
	calls            int
}

func (f *fakeTimelineServer) AllocateSeqBlock(
	ctx context.Context,
	request *timelinev1.AllocateSeqBlockRequest,
) (*timelinev1.AllocateSeqBlockResponse, error) {
	f.calls++
	f.incomingMetadata, _ = metadata.FromIncomingContext(ctx)
	f.request = request
	if f.err != nil {
		return nil, f.err
	}
	return f.response, nil
}
