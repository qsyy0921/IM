package grpc

import (
	"context"
	"testing"

	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakePullInboxExecutor struct {
	result types.PullInboxResult
	err    error
}

func (executor fakePullInboxExecutor) Execute(
	context.Context,
	types.PullInboxCommand,
) (types.PullInboxResult, error) {
	return executor.result, executor.err
}

type fakeAckDeliveryExecutor struct {
	result types.AckDeliveryResult
	err    error
}

func (executor fakeAckDeliveryExecutor) Execute(
	context.Context,
	types.AckDeliveryCommand,
) (types.AckDeliveryResult, error) {
	return executor.result, executor.err
}

func TestAckDeliveryMapsOutOfVisibleRange(t *testing.T) {
	server := NewServer(
		fakePullInboxExecutor{},
		fakeAckDeliveryExecutor{err: types.NewAckOutOfVisibleRange("too high")},
	)
	_, err := server.AckDelivery(context.Background(), &deliveryv1.AckDeliveryRequest{
		AuthContext: &deliveryv1.AuthContext{
			TenantId: "tenant-1",
			UserId:   "user-1",
			DeviceId: "device-1",
		},
		ConversationId: "conv-1",
		ReceivedSeq:    100,
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected failed precondition, got %v", err)
	}
}
