package rpc

import (
	"context"
	"testing"
	"time"

	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestDeliveryClientAckDeliverySendsVerifiedMetadata(t *testing.T) {
	fake := &fakeDeliveryServiceClient{}
	client := NewDeliveryClient(fake, time.Second)
	result, err := client.AckDelivery(context.Background(), types.AckDeliveryCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-1",
			UserID:    "user-1",
			DeviceID:  "device-1",
			SessionID: "session-1",
			TraceID:   "trace-1",
			RequestID: "request-1",
		},
		ConversationID: "conv-1",
		ReceivedSeq:    42,
	})
	if err != nil {
		t.Fatalf("ack delivery: %v", err)
	}
	if result.LastReceivedSeq != 42 {
		t.Fatalf("last_received_seq=%d want=42", result.LastReceivedSeq)
	}
	assertDeliveryMetadataValue(t, fake.outgoingMetadata, deliveryMetadataTenantID, "tenant-1")
	assertDeliveryMetadataValue(t, fake.outgoingMetadata, deliveryMetadataUserID, "user-1")
	assertDeliveryMetadataValue(t, fake.outgoingMetadata, deliveryMetadataDeviceID, "device-1")
	assertDeliveryMetadataValue(t, fake.outgoingMetadata, deliveryMetadataSessionID, "session-1")
	assertDeliveryMetadataValue(t, fake.outgoingMetadata, deliveryMetadataTraceID, "trace-1")
	assertDeliveryMetadataValue(t, fake.outgoingMetadata, deliveryMetadataRequestID, "request-1")
	if fake.request.GetAuthContext().GetTenantId() != "tenant-1" ||
		fake.request.GetAuthContext().GetUserId() != "user-1" ||
		fake.request.GetAuthContext().GetDeviceId() != "device-1" {
		t.Fatalf("unexpected body auth context: %+v", fake.request.GetAuthContext())
	}
}

func TestDeliveryClientAckDeliveryDropsUnsafeCorrelationMetadata(t *testing.T) {
	fake := &fakeDeliveryServiceClient{}
	client := NewDeliveryClient(fake, time.Second)
	if _, err := client.AckDelivery(context.Background(), types.AckDeliveryCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-1",
			UserID:    "user-1",
			DeviceID:  "device-1",
			SessionID: "session-1",
			TraceID:   "trace user=user1@example.com",
			RequestID: "request-token=secret-token",
		},
		ConversationID: "conv-1",
		ReceivedSeq:    42,
	}); err != nil {
		t.Fatalf("ack delivery: %v", err)
	}
	assertDeliveryMetadataMissing(t, fake.outgoingMetadata, deliveryMetadataTraceID)
	assertDeliveryMetadataMissing(t, fake.outgoingMetadata, deliveryMetadataRequestID)
	if fake.request.GetAuthContext().GetTraceId() != "" ||
		fake.request.GetAuthContext().GetRequestId() != "" {
		t.Fatalf("unsafe correlation fields must not be forwarded: %+v", fake.request.GetAuthContext())
	}
}

type fakeDeliveryServiceClient struct {
	outgoingMetadata metadata.MD
	request          *deliveryv1.AckDeliveryRequest
}

func (client *fakeDeliveryServiceClient) PullInbox(
	context.Context,
	*deliveryv1.PullInboxRequest,
	...grpc.CallOption,
) (*deliveryv1.PullInboxResponse, error) {
	panic("unexpected PullInbox call")
}

func (client *fakeDeliveryServiceClient) AckDelivery(
	ctx context.Context,
	request *deliveryv1.AckDeliveryRequest,
	_ ...grpc.CallOption,
) (*deliveryv1.AckDeliveryResponse, error) {
	client.outgoingMetadata, _ = metadata.FromOutgoingContext(ctx)
	client.request = request
	return &deliveryv1.AckDeliveryResponse{
		TenantId:        request.GetAuthContext().GetTenantId(),
		UserId:          request.GetAuthContext().GetUserId(),
		DeviceId:        request.GetAuthContext().GetDeviceId(),
		ConversationId:  request.GetConversationId(),
		LastReceivedSeq: request.GetReceivedSeq(),
	}, nil
}

func (client *fakeDeliveryServiceClient) HideInboxItem(
	context.Context,
	*deliveryv1.HideInboxItemRequest,
	...grpc.CallOption,
) (*deliveryv1.HideInboxItemResponse, error) {
	panic("unexpected HideInboxItem call")
}

func assertDeliveryMetadataValue(t *testing.T, md metadata.MD, key string, want string) {
	t.Helper()
	values := md.Get(key)
	if len(values) != 1 || values[0] != want {
		t.Fatalf("metadata %s = %v, want [%s]", key, values, want)
	}
}

func assertDeliveryMetadataMissing(t *testing.T, md metadata.MD, key string) {
	t.Helper()
	if values := md.Get(key); len(values) != 0 {
		t.Fatalf("metadata %s = %v, want missing", key, values)
	}
}
